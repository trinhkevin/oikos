// internal/spotify/spotify.go
package spotify

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

var (
	ErrNoActiveDevice = errors.New("no active playback device")
	ErrTokenInvalid   = errors.New("spotify authorization is invalid or missing")
	ErrRateLimited    = errors.New("spotify rate limited the request")
	ErrUpstream       = errors.New("spotify request failed")
)

// Scopes requested during the one-time host authorization. Read-currently-
// playing and read-playback-state back the Now Playing display; modify-
// playback-state backs queue-add.
const Scopes = "user-read-currently-playing user-read-playback-state user-modify-playback-state"

type Track struct {
	URI     string
	Name    string
	Artists string
}

// TokenStore persists the durable refresh token across restarts. The
// access token is never persisted — it's short-lived and cheap to
// re-derive via refresh.
type TokenStore interface {
	LoadRefreshToken(ctx context.Context) (string, error) // "", nil if none saved yet
	SaveTokens(ctx context.Context, refreshToken string, updatedAt time.Time) error
}

// Client is the one integration in this codebase that authenticates as a
// user, not as the app — reading what's playing and controlling the
// queue are both scoped to a real Spotify account with no app-only
// equivalent.
type Client struct {
	clientID      string
	clientSecret  string
	redirectURI   string
	httpClient    *http.Client
	tokens        TokenStore
	nowPlayingTTL time.Duration

	authURL    string
	tokenURL   string
	apiBaseURL string

	mu           sync.Mutex
	accessToken  string
	accessExpiry time.Time
	nowPlaying   *Track
	nowPlayingAt time.Time
}

func New(clientID, clientSecret, redirectURI string, tokens TokenStore, nowPlayingTTL time.Duration) *Client {
	return &Client{
		clientID: clientID, clientSecret: clientSecret, redirectURI: redirectURI,
		httpClient:    &http.Client{Timeout: 10 * time.Second},
		tokens:        tokens,
		nowPlayingTTL: nowPlayingTTL,
		authURL:       "https://accounts.spotify.com/authorize",
		tokenURL:      "https://accounts.spotify.com/api/token",
		apiBaseURL:    "https://api.spotify.com/v1",
	}
}

// AuthURL builds the URL the host visits, via the loopback tunnel, to
// begin the one-time authorization. state is a random value the caller
// must verify unchanged on callback, to prevent CSRF.
func (c *Client) AuthURL(state string) string {
	v := url.Values{
		"client_id":     {c.clientID},
		"response_type": {"code"},
		"redirect_uri":  {c.redirectURI},
		"scope":         {Scopes},
		"state":         {state},
	}
	return c.authURL + "?" + v.Encode()
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
}

func (c *Client) postForm(ctx context.Context, endpoint string, form url.Values) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(c.clientID, c.clientSecret)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, body)
	}
	return body, nil
}

// ExchangeCode trades an authorization code from the OAuth callback for
// tokens and persists the refresh token. Called exactly once, during the
// host's one-time setup (or again if the refresh token is ever revoked).
func (c *Client) ExchangeCode(ctx context.Context, code string) error {
	body, err := c.postForm(ctx, c.tokenURL, url.Values{
		"grant_type":   {"authorization_code"},
		"code":         {code},
		"redirect_uri": {c.redirectURI},
	})
	if err != nil {
		return fmt.Errorf("spotify: exchanging code: %w", err)
	}
	var resp tokenResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return fmt.Errorf("spotify: decoding token exchange response: %w", err)
	}
	if resp.RefreshToken == "" {
		return fmt.Errorf("spotify: token exchange returned no refresh token")
	}

	c.mu.Lock()
	c.accessToken = resp.AccessToken
	c.accessExpiry = time.Now().Add(time.Duration(resp.ExpiresIn-30) * time.Second)
	c.mu.Unlock()

	return c.tokens.SaveTokens(ctx, resp.RefreshToken, time.Now().UTC())
}

// getAccessToken returns a valid access token, refreshing via the stored
// refresh token if the cached one is missing or expired.
func (c *Client) getAccessToken(ctx context.Context) (string, error) {
	c.mu.Lock()
	if c.accessToken != "" && time.Now().Before(c.accessExpiry) {
		tok := c.accessToken
		c.mu.Unlock()
		return tok, nil
	}
	c.mu.Unlock()

	refreshToken, err := c.tokens.LoadRefreshToken(ctx)
	if err != nil {
		return "", fmt.Errorf("spotify: loading refresh token: %w", err)
	}
	if refreshToken == "" {
		return "", fmt.Errorf("spotify: no refresh token saved yet: %w", ErrTokenInvalid)
	}

	body, err := c.postForm(ctx, c.tokenURL, url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
	})
	if err != nil {
		return "", fmt.Errorf("spotify: refreshing token: %w: %w", err, ErrTokenInvalid)
	}
	var resp tokenResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", fmt.Errorf("spotify: decoding refresh response: %w", err)
	}

	c.mu.Lock()
	c.accessToken = resp.AccessToken
	c.accessExpiry = time.Now().Add(time.Duration(resp.ExpiresIn-30) * time.Second)
	c.mu.Unlock()

	// Spotify may rotate the refresh token; persist it if a new one came back.
	if resp.RefreshToken != "" {
		if err := c.tokens.SaveTokens(ctx, resp.RefreshToken, time.Now().UTC()); err != nil {
			return "", fmt.Errorf("spotify: saving rotated refresh token: %w", err)
		}
	}

	return resp.AccessToken, nil
}

// NowPlaying returns the currently-playing track, cached for
// nowPlayingTTL so a room full of guests loading the page doesn't
// hammer the API. Returns (nil, nil) when nothing is playing — that is
// not an error condition.
func (c *Client) NowPlaying(ctx context.Context) (*Track, error) {
	c.mu.Lock()
	if c.nowPlaying != nil && time.Now().Before(c.nowPlayingAt.Add(c.nowPlayingTTL)) {
		np := c.nowPlaying
		c.mu.Unlock()
		return np, nil
	}
	c.mu.Unlock()

	token, err := c.getAccessToken(ctx)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.apiBaseURL+"/me/player/currently-playing", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("spotify: now-playing request: %w: %w", err, ErrUpstream)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNoContent {
		c.mu.Lock()
		c.nowPlaying = nil
		c.nowPlayingAt = time.Now()
		c.mu.Unlock()
		return nil, nil
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, ErrRateLimited
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("spotify: now-playing status %d: %w", resp.StatusCode, ErrUpstream)
	}

	var body struct {
		Item struct {
			Name    string `json:"name"`
			URI     string `json:"uri"`
			Artists []struct {
				Name string `json:"name"`
			} `json:"artists"`
		} `json:"item"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("spotify: decoding now-playing: %w", err)
	}

	names := make([]string, 0, len(body.Item.Artists))
	for _, a := range body.Item.Artists {
		names = append(names, a.Name)
	}
	track := &Track{URI: body.Item.URI, Name: body.Item.Name, Artists: strings.Join(names, ", ")}

	c.mu.Lock()
	c.nowPlaying = track
	c.nowPlayingAt = time.Now()
	c.mu.Unlock()

	return track, nil
}

// Search returns up to 8 matching tracks for a guest's query.
func (c *Client) Search(ctx context.Context, query string) ([]Track, error) {
	token, err := c.getAccessToken(ctx)
	if err != nil {
		return nil, err
	}

	endpoint := fmt.Sprintf("%s/search?q=%s&type=track&limit=8", c.apiBaseURL, url.QueryEscape(query))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("spotify: search request: %w: %w", err, ErrUpstream)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, ErrRateLimited
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("spotify: search status %d: %w", resp.StatusCode, ErrUpstream)
	}

	var body struct {
		Tracks struct {
			Items []struct {
				URI     string `json:"uri"`
				Name    string `json:"name"`
				Artists []struct {
					Name string `json:"name"`
				} `json:"artists"`
			} `json:"items"`
		} `json:"tracks"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("spotify: decoding search response: %w", err)
	}

	tracks := make([]Track, 0, len(body.Tracks.Items))
	for _, item := range body.Tracks.Items {
		names := make([]string, 0, len(item.Artists))
		for _, a := range item.Artists {
			names = append(names, a.Name)
		}
		tracks = append(tracks, Track{URI: item.URI, Name: item.Name, Artists: strings.Join(names, ", ")})
	}
	return tracks, nil
}

// QueueTrack adds uri to the active device's playback queue — the
// unmoderated part of "unmoderated queue requests." No approval step
// happens before this call.
func (c *Client) QueueTrack(ctx context.Context, uri string) error {
	token, err := c.getAccessToken(ctx)
	if err != nil {
		return err
	}

	endpoint := fmt.Sprintf("%s/me/player/queue?uri=%s", c.apiBaseURL, url.QueryEscape(uri))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("spotify: queue request: %w: %w", err, ErrUpstream)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusNoContent, http.StatusOK:
		return nil
	case http.StatusNotFound:
		return ErrNoActiveDevice
	case http.StatusTooManyRequests:
		return ErrRateLimited
	default:
		return fmt.Errorf("spotify: queue status %d: %w", resp.StatusCode, ErrUpstream)
	}
}
