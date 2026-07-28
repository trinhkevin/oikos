// internal/web/ratelimit_test.go
package web

import (
	"testing"
	"time"
)

func TestRateLimiterAllowsUpToMax(t *testing.T) {
	rl := newRateLimiter(2, time.Minute)
	if !rl.allow("k") {
		t.Fatal("1st request should be allowed")
	}
	if !rl.allow("k") {
		t.Fatal("2nd request should be allowed")
	}
	if rl.allow("k") {
		t.Fatal("3rd request should be denied")
	}
}

func TestRateLimiterIsPerKey(t *testing.T) {
	rl := newRateLimiter(1, time.Minute)
	if !rl.allow("a") {
		t.Fatal("first key's 1st request should be allowed")
	}
	if !rl.allow("b") {
		t.Fatal("different key should have its own budget")
	}
}

func TestRateLimiterExpiresOldHits(t *testing.T) {
	rl := newRateLimiter(1, 10*time.Millisecond)
	if !rl.allow("k") {
		t.Fatal("1st request should be allowed")
	}
	time.Sleep(20 * time.Millisecond)
	if !rl.allow("k") {
		t.Fatal("request after the window elapsed should be allowed again")
	}
}
