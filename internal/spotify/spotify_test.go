package spotify

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
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
	// The scope actually matters: deleting user-modify-playback-state
	// from Scopes would leave queue-add silently failing with a 403,
	// discoverable only by redoing the whole SSH-tunnel OAuth dance.
	// url.Values.Encode() space-separates and then percent-encodes the
	// scope string, so a space becomes "+".
	if !strings.Contains(u, "scope=") {
		t.Fatalf("AuthURL = %q, want it to contain a scope param", u)
	}
	if !strings.Contains(u, "user-modify-playback-state") {
		t.Errorf("AuthURL = %q, want it to request the user-modify-playback-state scope (required for queue-add)", u)
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

// TestNowPlayingCachesNothingPlaying proves the "nothing playing" result
// (204) is itself a cache hit on the second call, not just a cache miss
// that happens to also return nil. Without a cache-freshness signal
// independent of "is the cached value nil", every page load with nothing
// playing would re-hit the API, defeating the whole point of the cache.
func TestNowPlayingCachesNothingPlaying(t *testing.T) {
	var calls int32
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"access_token": "t", "expires_in": 3600})
	})
	mux.HandleFunc("/me/player/currently-playing", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusNoContent)
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()
	c, _ := newTestClient(t, ts) // nowPlayingTTL = time.Minute

	track, err := c.NowPlaying(context.Background())
	if err != nil {
		t.Fatalf("NowPlaying: %v", err)
	}
	if track != nil {
		t.Fatalf("track = %+v, want nil", track)
	}

	if _, err := c.NowPlaying(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("now-playing endpoint called %d times, want 1 (second call should hit the nothing-playing cache)", got)
	}
}

// TestNowPlayingItemNullTreatedAsNothingPlaying covers a real Spotify
// response shape: HTTP 200 with "item": null (e.g. an ad on a free
// account). This must be treated the same as 204, not decoded into a
// zero-value Track and rendered as a blank "Now playing" line.
func TestNowPlayingItemNullTreatedAsNothingPlaying(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"access_token": "t", "expires_in": 3600})
	})
	mux.HandleFunc("/me/player/currently-playing", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"item": null}`))
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()
	c, _ := newTestClient(t, ts)

	track, err := c.NowPlaying(context.Background())
	if err != nil {
		t.Fatalf("NowPlaying: %v", err)
	}
	if track != nil {
		t.Errorf("track = %+v, want nil for a 200 with item:null", track)
	}
}

// TestGetAccessTokenRefreshRateLimitedMapsToErrRateLimited proves a 429
// from the token endpoint's refresh path maps to ErrRateLimited, not the
// generic ErrTokenInvalid the old code returned for every failure there.
func TestGetAccessTokenRefreshRateLimitedMapsToErrRateLimited(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()
	c, _ := newTestClient(t, ts)

	_, err := c.NowPlaying(context.Background())
	if !errors.Is(err, ErrRateLimited) {
		t.Fatalf("err = %v, want ErrRateLimited", err)
	}
	if errors.Is(err, ErrTokenInvalid) {
		t.Errorf("err = %v, should not also be ErrTokenInvalid", err)
	}
}

// TestGetAccessTokenRefreshServerErrorMapsToErrUpstream proves a 5xx from
// the token endpoint's refresh path maps to ErrUpstream (transient), not
// ErrTokenInvalid (which tells the host to redo the whole OAuth setup).
func TestGetAccessTokenRefreshServerErrorMapsToErrUpstream(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()
	c, _ := newTestClient(t, ts)

	_, err := c.NowPlaying(context.Background())
	if !errors.Is(err, ErrUpstream) {
		t.Fatalf("err = %v, want ErrUpstream", err)
	}
	if errors.Is(err, ErrTokenInvalid) {
		t.Errorf("err = %v, should not be ErrTokenInvalid for a transient 503", err)
	}
}

// TestGetAccessTokenRefreshInvalidGrantMapsToErrTokenInvalid proves
// Spotify's actual invalid/revoked-grant response (400) still maps to
// ErrTokenInvalid, so the host is correctly told to redo authorization.
func TestGetAccessTokenRefreshInvalidGrantMapsToErrTokenInvalid(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":"invalid_grant"}`))
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()
	c, _ := newTestClient(t, ts)

	_, err := c.NowPlaying(context.Background())
	if !errors.Is(err, ErrTokenInvalid) {
		t.Fatalf("err = %v, want ErrTokenInvalid", err)
	}
}

// TestGetAccessTokenSingleFlightOnConcurrentRefresh proves concurrent
// callers hitting an expired/missing cached token collapse into exactly
// one refresh call, rather than each independently refreshing with the
// same refresh token (which risks a later response clobbering an earlier
// rotated token, silently locking the host out).
func TestGetAccessTokenSingleFlightOnConcurrentRefresh(t *testing.T) {
	var tokenCalls int32
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&tokenCalls, 1)
		time.Sleep(20 * time.Millisecond) // widen the race window
		json.NewEncoder(w).Encode(map[string]any{
			"access_token": "test-access-token", "refresh_token": "test-refresh-token", "expires_in": 3600,
		})
	})
	mux.HandleFunc("/me/player/currently-playing", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()
	c, _ := newTestClient(t, ts)

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := c.NowPlaying(context.Background()); err != nil {
				t.Errorf("NowPlaying: %v", err)
			}
		}()
	}
	wg.Wait()

	if got := atomic.LoadInt32(&tokenCalls); got != 1 {
		t.Errorf("token endpoint called %d times, want 1 (refresh should be single-flighted)", got)
	}
}

// TestGetAccessTokenPersistsRotatedRefreshToken proves the
// rotation-persistence behavior end to end: when a refresh response
// includes a new refresh_token, SaveTokens is actually called with it.
func TestGetAccessTokenPersistsRotatedRefreshToken(t *testing.T) {
	ts, _ := testSpotifyServer(t) // its /token handler always returns refresh_token: "test-refresh-token"
	defer ts.Close()
	c, store := newTestClient(t, ts)
	store.refreshToken = "old-refresh-token"
	store.saveCalls = 0

	if _, err := c.NowPlaying(context.Background()); err != nil {
		t.Fatalf("NowPlaying: %v", err)
	}

	if store.saveCalls != 1 {
		t.Errorf("SaveTokens called %d times, want 1", store.saveCalls)
	}
	if store.refreshToken != "test-refresh-token" {
		t.Errorf("stored refresh token = %q, want the rotated value test-refresh-token", store.refreshToken)
	}
}
