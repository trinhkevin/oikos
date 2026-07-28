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
	ErrNoActiveDevice   = errors.New("no active playback device")
	ErrTokenInvalid     = errors.New("spotify authorization is invalid or missing")
	ErrRateLimited      = errors.New("spotify rate limited the request")
	ErrUpstream         = errors.New("spotify request failed")
	ErrPremiumRequired  = errors.New("spotify premium is required for this action")
	ErrPlaybackRejected = errors.New("spotify rejected the playback command")
)

// Scopes requested during the one-time host authorization. Read-currently-
// playing and read-playback-state back the Now Playing display; modify-
// playback-state backs queue-add.
const Scopes = "user-read-currently-playing user-read-playback-state user-modify-playback-state"

type Track struct {
	URI         string
	Name        string
	Artists     string
	AlbumArtURL string
}

// albumImage is Spotify's shape for one entry in an album's "images"
// array — always ordered largest-first (typically 640/300/64px).
type albumImage struct {
	URL string `json:"url"`
}

// pickAlbumArt returns a mid-sized image URL when available, falling
// back to whatever's present. Guests' phones don't need the 640px
// original for a thumbnail, and always taking index 0 would mean
// re-downloading the largest asset every poll.
func pickAlbumArt(images []albumImage) string {
	switch {
	case len(images) >= 2:
		return images[1].URL
	case len(images) == 1:
		return images[0].URL
	default:
		return ""
	}
}

// trackItem is Spotify's track object shape, shared verbatim across
// now-playing, search, and queue responses.
type trackItem struct {
	Name    string `json:"name"`
	URI     string `json:"uri"`
	Artists []struct {
		Name string `json:"name"`
	} `json:"artists"`
	Album struct {
		Images []albumImage `json:"images"`
	} `json:"album"`
}

func (t trackItem) toTrack() Track {
	names := make([]string, 0, len(t.Artists))
	for _, a := range t.Artists {
		names = append(names, a.Name)
	}
	return Track{
		URI:         t.URI,
		Name:        t.Name,
		Artists:     strings.Join(names, ", "),
		AlbumArtURL: pickAlbumArt(t.Album.Images),
	}
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

	mu               sync.Mutex
	accessToken      string
	accessExpiry     time.Time
	nowPlaying       *Track
	nowPlayingCached bool // true once a fetch has populated nowPlaying, even when it's nil (nothing playing)
	nowPlayingAt     time.Time
	queue            []Track
	queueCached      bool
	queueAt          time.Time
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

// statusError carries the HTTP status code of a non-200 response from
// Spotify's accounts/token endpoint, so callers can distinguish a
// rate-limited or transient failure from an actually-invalid grant
// instead of treating every non-200 the same way.
type statusError struct {
	status int
	body   []byte
}

func (e *statusError) Error() string {
	return fmt.Sprintf("status %d: %s", e.status, e.body)
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
		return nil, &statusError{status: resp.StatusCode, body: body}
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
//
// The lock is held across the entire refresh sequence, including the
// network round-trip — deliberately, not an oversight. Without this,
// concurrent callers arriving right as the access token expires (several
// guests loading /music at once, or right after a restart) would each
// independently refresh with the same refresh token and each
// independently call SaveTokens; depending on Spotify's rotation timing,
// a later response could clobber an earlier one that was actually still
// valid, silently locking the host out. Holding the lock serializes
// refreshes so only the first caller through actually hits the network;
// everyone behind it sees the freshly-cached token once it's their turn.
func (c *Client) getAccessToken(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.accessToken != "" && time.Now().Before(c.accessExpiry) {
		return c.accessToken, nil
	}

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
		return "", fmt.Errorf("spotify: refreshing token: %w: %w", err, classifyTokenError(err))
	}
	var resp tokenResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", fmt.Errorf("spotify: decoding refresh response: %w", err)
	}

	c.accessToken = resp.AccessToken
	c.accessExpiry = time.Now().Add(time.Duration(resp.ExpiresIn-30) * time.Second)

	// Spotify may rotate the refresh token; persist it if a new one came back.
	if resp.RefreshToken != "" {
		if err := c.tokens.SaveTokens(ctx, resp.RefreshToken, time.Now().UTC()); err != nil {
			return "", fmt.Errorf("spotify: saving rotated refresh token: %w", err)
		}
	}

	return resp.AccessToken, nil
}

// classifyTokenError maps a token-endpoint failure to the sentinel that
// best describes it, so a transient blip (429, 5xx, timeout) doesn't get
// conflated with an actually-invalid or revoked refresh token. Only a
// genuine invalid-grant response (400/401 — Spotify's actual rejection
// of a bad refresh token) maps to ErrTokenInvalid, which tells the host
// to redo the whole one-time OAuth setup; everything else is transient.
func classifyTokenError(err error) error {
	var se *statusError
	if errors.As(err, &se) {
		switch {
		case se.status == http.StatusTooManyRequests:
			return ErrRateLimited
		case se.status == http.StatusBadRequest || se.status == http.StatusUnauthorized:
			return ErrTokenInvalid
		default:
			return ErrUpstream
		}
	}
	// Not an HTTP-status failure at all (connection refused, timeout,
	// context cancellation, etc.) — transient by nature.
	return ErrUpstream
}

// NowPlaying returns the currently-playing track, cached for
// nowPlayingTTL so a room full of guests loading the page doesn't
// hammer the API. Returns (nil, nil) when nothing is playing — that is
// not an error condition.
func (c *Client) NowPlaying(ctx context.Context) (*Track, error) {
	c.mu.Lock()
	if c.nowPlayingCached && time.Now().Before(c.nowPlayingAt.Add(c.nowPlayingTTL)) {
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
		c.nowPlayingCached = true
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
		Item trackItem `json:"item"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("spotify: decoding now-playing: %w", err)
	}

	if body.Item.URI == "" {
		// Spotify can return HTTP 200 with "item": null in some legitimate
		// states (e.g. an ad on a free account, certain playback contexts)
		// — decodes to a zero-value Item, not an error. Treat exactly like
		// 204: nothing playing, and cache it as such.
		c.mu.Lock()
		c.nowPlaying = nil
		c.nowPlayingCached = true
		c.nowPlayingAt = time.Now()
		c.mu.Unlock()
		return nil, nil
	}

	track := body.Item.toTrack()

	c.mu.Lock()
	c.nowPlaying = &track
	c.nowPlayingCached = true
	c.nowPlayingAt = time.Now()
	c.mu.Unlock()

	return &track, nil
}

// Queue returns the tracks Spotify will play next, cached alongside
// NowPlaying on the same short TTL — both are polled together by the
// Music page, so a room full of guests loading it doesn't double the
// hammering the now-playing cache already exists to prevent.
func (c *Client) Queue(ctx context.Context) ([]Track, error) {
	c.mu.Lock()
	if c.queueCached && time.Now().Before(c.queueAt.Add(c.nowPlayingTTL)) {
		q := c.queue
		c.mu.Unlock()
		return q, nil
	}
	c.mu.Unlock()

	token, err := c.getAccessToken(ctx)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.apiBaseURL+"/me/player/queue", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("spotify: queue request: %w: %w", err, ErrUpstream)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNoContent {
		c.mu.Lock()
		c.queue = nil
		c.queueCached = true
		c.queueAt = time.Now()
		c.mu.Unlock()
		return nil, nil
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, ErrRateLimited
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("spotify: queue status %d: %w", resp.StatusCode, ErrUpstream)
	}

	var body struct {
		Queue []trackItem `json:"queue"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("spotify: decoding queue: %w", err)
	}

	tracks := make([]Track, 0, len(body.Queue))
	for _, item := range body.Queue {
		tracks = append(tracks, item.toTrack())
	}

	c.mu.Lock()
	c.queue = tracks
	c.queueCached = true
	c.queueAt = time.Now()
	c.mu.Unlock()

	return tracks, nil
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
			Items []trackItem `json:"items"`
		} `json:"tracks"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("spotify: decoding search response: %w", err)
	}

	tracks := make([]Track, 0, len(body.Tracks.Items))
	for _, item := range body.Tracks.Items {
		tracks = append(tracks, item.toTrack())
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
		// Invalidate the cached queue so the next Queue() call re-fetches
		// from Spotify instead of serving a snapshot from before this
		// track was added — the Music page immediately re-displays the
		// queue after a successful add, and it must show the new track.
		c.mu.Lock()
		c.queueCached = false
		c.mu.Unlock()
		return nil
	case http.StatusNotFound:
		return ErrNoActiveDevice
	case http.StatusTooManyRequests:
		return ErrRateLimited
	case http.StatusForbidden:
		// Spotify returns 403 for several distinct playback-command
		// rejections (most commonly the host's account not having
		// Premium, which is required for every playback-modification
		// endpoint including queue-add) — the body's "reason" field is
		// the only way to tell them apart, and logging it here is what
		// was missing when this first got reported as an opaque
		// "failed" with nothing actionable in the logs.
		body, _ := io.ReadAll(resp.Body)
		if strings.Contains(string(body), "PREMIUM_REQUIRED") {
			return fmt.Errorf("spotify: queue rejected: %s: %w", body, ErrPremiumRequired)
		}
		return fmt.Errorf("spotify: queue rejected: %s: %w", body, ErrPlaybackRejected)
	default:
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("spotify: queue status %d: %s: %w", resp.StatusCode, body, ErrUpstream)
	}
}
