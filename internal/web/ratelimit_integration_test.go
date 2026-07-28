// internal/web/ratelimit_integration_test.go
package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestGuestbookRateLimitTriggersAfterConfiguredMax(t *testing.T) {
	s := newTestServer(t) // testConfig() now sets GuestbookPerWindow: 2

	post := func(message string) *httptest.ResponseRecorder {
		form := url.Values{"name": {"Alex"}, "message": {message}}
		req := httptest.NewRequest(http.MethodPost, "/guestbook", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.RemoteAddr = "192.168.1.55:4321"
		rec := httptest.NewRecorder()
		s.ServeHTTP(rec, req)
		return rec
	}

	post("first")
	post("second")
	rec := post("third — should be blocked")

	if got := rec.Header().Get("HX-Retarget"); got != "#guestbook-rl-message" {
		t.Errorf("HX-Retarget = %q, want #guestbook-rl-message", got)
	}
	if !strings.Contains(rec.Body.String(), "You've had your turn") {
		t.Errorf("body = %q, want the rate-limit message", rec.Body.String())
	}
}

// TestGuestbookRateLimitPersistsAcrossCookieClear proves the specific
// property called out in the spec: a guest cannot reset their rate-limit
// budget by clearing cookies (or by never accepting cookies in the first
// place). No request in this test ever sends or receives a cookie — the
// limiter is keyed by client IP alone — yet the budget still runs out at
// the configured max because all three requests share the same simulated
// IP, exactly as a real guest's DHCP-assigned LAN address would.
func TestGuestbookRateLimitPersistsAcrossCookieClear(t *testing.T) {
	s := newTestServer(t) // testConfig() sets GuestbookPerWindow: 2

	post := func(message string) *httptest.ResponseRecorder {
		form := url.Values{"name": {"Alex"}, "message": {message}}
		req := httptest.NewRequest(http.MethodPost, "/guestbook", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.RemoteAddr = "192.168.1.77:9999"
		// Deliberately never attach a cookie, simulating a guest whose
		// browser cleared cookies (or never accepted them) between
		// requests.
		rec := httptest.NewRecorder()
		s.ServeHTTP(rec, req)
		return rec
	}

	post("first")
	post("second")
	rec := post("third — should still be blocked despite no cookie ever being sent")

	if got := rec.Header().Get("HX-Retarget"); got != "#guestbook-rl-message" {
		t.Errorf("HX-Retarget = %q, want #guestbook-rl-message (limit should persist across cookie-less requests)", got)
	}
	if !strings.Contains(rec.Body.String(), "You've had your turn") {
		t.Errorf("body = %q, want the rate-limit message", rec.Body.String())
	}
}
