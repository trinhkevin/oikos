package web

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"testing"

	"homesite/internal/config"
	"homesite/internal/store"
)

func testConfig() *config.Config {
	return &config.Config{
		Site:       config.SiteConfig{Title: "Brivin Household"},
		Limits:     config.LimitsConfig{GuestbookPerWindow: 2, PhotoUploadsPerWindow: 30, SongRequestsPerWindow: 3, WindowMinutes: 15},
		ContentDir: "../../content",
		DataDir:    "../../data",
		UploadsDir: "../../uploads",
	}
}

func newTestServer(t *testing.T) *Server {
	t.Helper()
	db, err := store.OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return New(testConfig(), db)
}

func mustOpenMemoryDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := store.OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestHealthzReturnsOK(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestStaticServesHTMX(t *testing.T) {
	s := newTestServer(t)
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
