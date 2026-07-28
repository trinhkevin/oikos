// internal/web/ratelimit.go
package web

import (
	"net/http"
	"sync"
	"time"
)

type rateLimiter struct {
	mu     sync.Mutex
	hits   map[string][]time.Time
	max    int
	window time.Duration
}

func newRateLimiter(max int, window time.Duration) *rateLimiter {
	return &rateLimiter{hits: make(map[string][]time.Time), max: max, window: window}
}

// allow records a hit for key and reports whether it's within budget.
// Restart-resets-everything is an accepted tradeoff (see spec) — this
// is an in-memory map, not a persisted counter.
func (rl *rateLimiter) allow(key string) bool {
	now := time.Now()
	cutoff := now.Add(-rl.window)

	rl.mu.Lock()
	defer rl.mu.Unlock()

	kept := rl.hits[key][:0]
	for _, t := range rl.hits[key] {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	if len(kept) >= rl.max {
		rl.hits[key] = kept
		return false
	}
	rl.hits[key] = append(kept, now)
	return true
}

// rateLimit wraps next so requests over budget get a friendly fragment
// retargeted (via the HX-Retarget/HX-Reswap response headers) into
// targetID instead of the route's normal success target.
//
// The limiter is keyed by client IP alone, not by a cookie. This is a
// LAN-only site: guests join the household's WiFi directly and each gets
// an individually DHCP-assigned address from the router — there's no
// shared NAT gateway collapsing multiple guests onto one apparent IP the
// way there would be on the public internet. So the IP is already a
// stable, reliable per-guest identifier here, and — critically — using it
// alone means clearing cookies (or never accepting them) cannot reset a
// caller's budget, since there's no cookie in the key to reset.
func (s *Server) rateLimit(rl *rateLimiter, targetID string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key := clientIP(r)
		if !rl.allow(key) {
			w.Header().Set("HX-Retarget", "#"+targetID)
			w.Header().Set("HX-Reswap", "innerHTML")
			renderRateLimited(w, r)
			return
		}
		next(w, r)
	}
}
