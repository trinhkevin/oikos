package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSpotifyLoginRejectsNonLoopback(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/spotify/login", nil)
	req.RemoteAddr = "203.0.113.5:54321" // a real, non-loopback address
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 for a non-loopback caller", rec.Code)
	}
}

func TestSpotifyLoginRedirectsForLoopback(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/spotify/login", nil)
	req.RemoteAddr = "127.0.0.1:54321"
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302 redirect to Spotify's authorize endpoint", rec.Code)
	}
	loc := rec.Header().Get("Location")
	if loc == "" {
		t.Fatal("expected a Location header pointing at Spotify's authorize endpoint")
	}
	// Not just "some URL came back" — it must actually request the
	// scopes the queue-add and now-playing features depend on.
	if !strings.Contains(loc, "scope=") {
		t.Fatalf("Location = %q, want it to contain a scope param", loc)
	}
	for _, want := range []string{"user-read-currently-playing", "user-read-playback-state", "user-modify-playback-state"} {
		if !strings.Contains(loc, want) {
			t.Errorf("Location = %q, want it to contain scope %q", loc, want)
		}
	}
}

func TestSpotifyCallbackRejectsNonLoopback(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/spotify/callback?code=abc&state=xyz", nil)
	req.RemoteAddr = "203.0.113.5:54321"
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 for a non-loopback caller", rec.Code)
	}
}
