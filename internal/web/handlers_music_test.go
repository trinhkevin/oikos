package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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
