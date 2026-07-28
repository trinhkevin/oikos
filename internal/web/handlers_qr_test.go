package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"homesite/internal/config"
)

func testConfigWithWiFi() *config.Config {
	cfg := testConfig()
	cfg.WiFi = config.WiFiConfig{SSID: "Brivin Net", Password: "letmein123", Auth: "WPA"}
	cfg.Site.URL = "http://home.arpa"
	cfg.Site.FallbackURL = "http://192.168.1.50"
	return cfg
}

func TestWiFiPageRendersCredentials(t *testing.T) {
	s := New(testConfigWithWiFi())
	req := httptest.NewRequest(http.MethodGet, "/wifi", nil)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Brivin Net") || !strings.Contains(body, "letmein123") {
		t.Error("expected page to show SSID and password as selectable text")
	}
	if !strings.Contains(body, "/wifi/qr.png") {
		t.Error("expected page to reference the QR image route")
	}
}

func TestWiFiQRPngServesImage(t *testing.T) {
	s := New(testConfigWithWiFi())
	req := httptest.NewRequest(http.MethodGet, "/wifi/qr.png", nil)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "image/png" {
		t.Errorf("Content-Type = %q, want image/png", ct)
	}
	if rec.Body.Len() == 0 {
		t.Fatal("expected non-empty PNG body")
	}
}

func TestSharePageRendersURLAndFallback(t *testing.T) {
	s := New(testConfigWithWiFi())
	req := httptest.NewRequest(http.MethodGet, "/share", nil)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "home.arpa") {
		t.Error("expected page to show the primary URL")
	}
	if !strings.Contains(body, "192.168.1.50") {
		t.Error("expected page to show the resolver-bypass fallback")
	}
}

func TestShareQRPngServesImage(t *testing.T) {
	s := New(testConfigWithWiFi())
	req := httptest.NewRequest(http.MethodGet, "/share/qr.png", nil)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "image/png" {
		t.Errorf("Content-Type = %q, want image/png", ct)
	}
}
