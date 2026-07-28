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
	s := newTestServer(t)
	s.cfg = testConfigWithWiFi()
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
	// SSID and password must render as two distinct labeled lines, not
	// one ambiguous concatenated string (which would be unparseable if
	// either value contained a space or slash).
	if !strings.Contains(body, "Network: Brivin Net") {
		t.Errorf("body = %q, want a distinct \"Network: <ssid>\" line", body)
	}
	if !strings.Contains(body, "Password: letmein123") {
		t.Errorf("body = %q, want a distinct \"Password: <password>\" line", body)
	}
	if !strings.Contains(body, "/wifi/qr.png") {
		t.Error("expected page to reference the QR image route")
	}
	if !strings.Contains(body, `alt="Wi-Fi QR code"`) {
		t.Errorf("body = %q, want a Wi-Fi-specific alt text on the QR image", body)
	}
}

func TestWiFiQRPngServesImage(t *testing.T) {
	s := newTestServer(t)
	s.cfg = testConfigWithWiFi()
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
	s := newTestServer(t)
	s.cfg = testConfigWithWiFi()
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
	if !strings.Contains(body, `alt="Share QR code"`) {
		t.Errorf("body = %q, want a Share-specific alt text on the QR image", body)
	}
}

func TestShareQRPngServesImage(t *testing.T) {
	s := newTestServer(t)
	s.cfg = testConfigWithWiFi()
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
