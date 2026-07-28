package web

import (
	"net/http"
	"net/http/httptest"
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
