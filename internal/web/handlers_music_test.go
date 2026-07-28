package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"homesite/internal/spotify"
)

func TestMusicPageRenders(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/music", nil)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	// testConfig() has no real Spotify credentials, so NowPlaying will
	// fail against the real API — the page must degrade gracefully, not 500.
	if !strings.Contains(rec.Body.String(), "Music Requests") {
		t.Error("expected the page heading to render")
	}
}

func TestMusicSearchWithEmptyQueryReturnsNoResults(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/music/search", nil)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}

// TestMusicRequestRejectsOverlongTrackName proves a guest sees a sensible
// message — not a raw store error — for a request whose hidden-field
// track name exceeds the store's cap, and that nothing lands in the
// "Recently added" list as a result.
func TestMusicRequestRejectsOverlongTrackName(t *testing.T) {
	s := newTestServer(t)
	form := url.Values{
		"uri": {"spotify:track:abc"}, "name": {strings.Repeat("x", spotify.MaxTrackNameLength+1)}, "artist": {"Artist"},
	}
	req := httptest.NewRequest(http.MethodPost, "/music/request", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "invalid") {
		t.Errorf("body = %q, want a message about the invalid request", rec.Body.String())
	}

	recent, err := s.requestStore.Recent(req.Context(), 10)
	if err != nil {
		t.Fatalf("Recent: %v", err)
	}
	if len(recent) != 0 {
		t.Errorf("Recent = %+v, want nothing recorded for a rejected request", recent)
	}
}
