package web

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"homesite/internal/config"
)

func testConfig() *config.Config {
	return &config.Config{
		Site:       config.SiteConfig{Title: "Brivin Household"},
		ContentDir: "../../content",
		DataDir:    "../../data",
		UploadsDir: "../../uploads",
	}
}

func TestHealthzReturnsOK(t *testing.T) {
	s := New(testConfig())
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestStaticServesHTMX(t *testing.T) {
	s := New(testConfig())
	req := httptest.NewRequest(http.MethodGet, "/static/htmx.min.js", nil)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if rec.Body.Len() == 0 {
		t.Fatal("expected non-empty htmx.min.js body")
	}
}
