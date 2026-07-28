package spotify

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// fakeTokenStore is an in-memory TokenStore for tests, avoiding any real
// database dependency in this package's own tests.
type fakeTokenStore struct {
	refreshToken string
	saveCalls    int
}

func (f *fakeTokenStore) LoadRefreshToken(ctx context.Context) (string, error) {
	return f.refreshToken, nil
}
func (f *fakeTokenStore) SaveTokens(ctx context.Context, refreshToken string, updatedAt time.Time) error {
	f.refreshToken = refreshToken
	f.saveCalls++
	return nil
}

func TestAuthURLIncludesScopesAndState(t *testing.T) {
	c := New("id", "secret", "http://127.0.0.1:8080/spotify/callback", &fakeTokenStore{}, time.Second)
	u := c.AuthURL("test-state-123")
	if !strings.Contains(u, "state=test-state-123") {
		t.Errorf("AuthURL = %q, want it to contain the state param", u)
	}
	if !strings.Contains(u, "client_id=id") {
		t.Errorf("AuthURL = %q, want it to contain client_id", u)
	}
}

// testSpotifyServer fakes the accounts token endpoint and the three API.
func testSpotifyServer(t *testing.T) (*httptest.Server, *[]string) {
	t.Helper()
	var calls []string
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, "token:"+r.FormValue("grant_type"))
		json.NewEncoder(w).Encode(map[string]any{
			"access_token": "test-access-token", "refresh_token": "test-refresh-token", "expires_in": 3600,
		})
	})
	mux.HandleFunc("/me/player/currently-playing", func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, "now-playing")
		json.NewEncoder(w).Encode(map[string]any{
			"item": map[string]any{
				"name": "Song A", "uri": "spotify:track:abc",
				"artists": []map[string]any{{"name": "Artist One"}},
			},
		})
	})
	mux.HandleFunc("/search", func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, "search:"+r.URL.Query().Get("q"))
		json.NewEncoder(w).Encode(map[string]any{
			"tracks": map[string]any{"items": []map[string]any{
				{"name": "Song B", "uri": "spotify:track:def", "artists": []map[string]any{{"name": "Artist Two"}}},
			}},
		})
	})
	mux.HandleFunc("/me/player/queue", func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, "queue:"+r.URL.Query().Get("uri"))
		w.WriteHeader(http.StatusNoContent)
	})
	return httptest.NewServer(mux), &calls
}

func newTestClient(t *testing.T, ts *httptest.Server) (*Client, *fakeTokenStore) {
	t.Helper()
	store := &fakeTokenStore{refreshToken: "existing-refresh-token"}
	c := New("id", "secret", "http://127.0.0.1:8080/spotify/callback", store, time.Minute)
	c.tokenURL = ts.URL + "/token"
	c.apiBaseURL = ts.URL
	return c, store
}

func TestExchangeCodeSavesRefreshToken(t *testing.T) {
	ts, _ := testSpotifyServer(t)
	defer ts.Close()
	c, store := newTestClient(t, ts)
	store.refreshToken = "" // no token yet, this call is the initial authorization

	if err := c.ExchangeCode(context.Background(), "auth-code-123"); err != nil {
		t.Fatalf("ExchangeCode: %v", err)
	}
	if store.refreshToken != "test-refresh-token" {
		t.Errorf("stored refresh token = %q, want test-refresh-token", store.refreshToken)
	}
}

func TestNowPlayingFetchesAndCaches(t *testing.T) {
	ts, calls := testSpotifyServer(t)
	defer ts.Close()
	c, _ := newTestClient(t, ts)

	track, err := c.NowPlaying(context.Background())
	if err != nil {
		t.Fatalf("NowPlaying: %v", err)
	}
	if track == nil || track.Name != "Song A" || track.Artists != "Artist One" {
		t.Fatalf("track = %+v", track)
	}

	if _, err := c.NowPlaying(context.Background()); err != nil {
		t.Fatal(err)
	}
	nowPlayingCalls := 0
	for _, call := range *calls {
		if call == "now-playing" {
			nowPlayingCalls++
		}
	}
	if nowPlayingCalls != 1 {
		t.Errorf("now-playing endpoint called %d times, want 1 (second call should hit cache)", nowPlayingCalls)
	}
}

func TestSearchReturnsTracks(t *testing.T) {
	ts, _ := testSpotifyServer(t)
	defer ts.Close()
	c, _ := newTestClient(t, ts)

	tracks, err := c.Search(context.Background(), "song b")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(tracks) != 1 || tracks[0].Name != "Song B" || tracks[0].URI != "spotify:track:def" {
		t.Fatalf("tracks = %+v", tracks)
	}
}

func TestQueueTrackSucceeds(t *testing.T) {
	ts, calls := testSpotifyServer(t)
	defer ts.Close()
	c, _ := newTestClient(t, ts)

	if err := c.QueueTrack(context.Background(), "spotify:track:abc"); err != nil {
		t.Fatalf("QueueTrack: %v", err)
	}
	found := false
	for _, call := range *calls {
		if call == "queue:spotify:track:abc" {
			found = true
		}
	}
	if !found {
		t.Error("expected a queue call with the requested URI")
	}
}

func TestQueueTrackNoActiveDeviceReturnsSentinel(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"access_token": "t", "expires_in": 3600})
	})
	mux.HandleFunc("/me/player/queue", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()
	c, _ := newTestClient(t, ts)

	err := c.QueueTrack(context.Background(), "spotify:track:abc")
	if !errors.Is(err, ErrNoActiveDevice) {
		t.Fatalf("err = %v, want ErrNoActiveDevice", err)
	}
}

func TestQueueTrackRateLimitedReturnsSentinel(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"access_token": "t", "expires_in": 3600})
	})
	mux.HandleFunc("/me/player/queue", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()
	c, _ := newTestClient(t, ts)

	err := c.QueueTrack(context.Background(), "spotify:track:abc")
	if !errors.Is(err, ErrRateLimited) {
		t.Fatalf("err = %v, want ErrRateLimited", err)
	}
}

func TestNowPlayingNoTrackReturnsNilNotError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"access_token": "t", "expires_in": 3600})
	})
	mux.HandleFunc("/me/player/currently-playing", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()
	c, _ := newTestClient(t, ts)

	track, err := c.NowPlaying(context.Background())
	if err != nil {
		t.Fatalf("NowPlaying: %v", err)
	}
	if track != nil {
		t.Errorf("track = %+v, want nil (nothing playing)", track)
	}
}
