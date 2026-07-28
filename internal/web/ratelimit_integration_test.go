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
// budget by clearing cookies. Requests 1 and 2 genuinely carry a session
// cookie (simulating normal browser behavior); request 3 deliberately
// omits it (simulating a cleared/refused cookie jar) while keeping the
// same simulated IP — exactly as a real guest's DHCP-assigned LAN
// address would stay stable across requests regardless of cookie state.
// The limit must still trigger on request 3 because Task 16's fix keys
// the limiter by IP alone, with no cookie in the key to reset.
func TestGuestbookRateLimitPersistsAcrossCookieClear(t *testing.T) {
	s := newTestServer(t) // testConfig() sets GuestbookPerWindow: 2

	const ip = "192.168.1.77:9999"
	sessionCookie := &http.Cookie{Name: "session", Value: "guest-session-abc123"}

	post := func(message string, withCookie bool) *httptest.ResponseRecorder {
		form := url.Values{"name": {"Alex"}, "message": {message}}
		req := httptest.NewRequest(http.MethodPost, "/guestbook", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.RemoteAddr = ip
		if withCookie {
			req.AddCookie(sessionCookie)
		}
		rec := httptest.NewRecorder()
		s.ServeHTTP(rec, req)
		return rec
	}

	post("first", true)
	post("second", true)
	// Request 3 explicitly omits the cookie carried across requests 1
	// and 2 — simulating the guest's browser clearing cookies between
	// requests — but keeps the same IP.
	rec := post("third — should still be blocked despite the cookie being cleared", false)

	if got := rec.Header().Get("HX-Retarget"); got != "#guestbook-rl-message" {
		t.Errorf("HX-Retarget = %q, want #guestbook-rl-message (limit should persist across a cleared cookie)", got)
	}
	if !strings.Contains(rec.Body.String(), "You've had your turn") {
		t.Errorf("body = %q, want the rate-limit message", rec.Body.String())
	}
}
