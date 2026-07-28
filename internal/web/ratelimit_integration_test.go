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
	var cookie *http.Cookie

	post := func(message string) *httptest.ResponseRecorder {
		form := url.Values{"name": {"Alex"}, "message": {message}}
		req := httptest.NewRequest(http.MethodPost, "/guestbook", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		if cookie != nil {
			req.AddCookie(cookie)
		}
		rec := httptest.NewRecorder()
		s.ServeHTTP(rec, req)
		for _, c := range rec.Result().Cookies() {
			if c.Name == rateLimitCookieName {
				cookie = c
			}
		}
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
