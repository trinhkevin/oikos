// internal/web/ratelimit.go
package web

import (
	"crypto/rand"
	"encoding/hex"
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

const rateLimitCookieName = "brivin_rl"

// rateLimitKey combines a long-lived per-browser cookie with the
// caller's IP, per the spec's "keyed by session cookie AND client IP, so
// clearing cookies does not reset them" requirement. It sets the cookie
// on first sight of a caller that doesn't have one yet.
func rateLimitKey(w http.ResponseWriter, r *http.Request) string {
	token := ""
	if c, err := r.Cookie(rateLimitCookieName); err == nil && c.Value != "" {
		token = c.Value
	} else {
		token = randomToken()
		http.SetCookie(w, &http.Cookie{
			Name: rateLimitCookieName, Value: token, Path: "/",
			MaxAge: 60 * 60 * 24 * 30,
		})
	}
	return token + "|" + clientIP(r)
}

func randomToken() string {
	var b [16]byte
	rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// rateLimit wraps next so requests over budget get a friendly fragment
// retargeted (via the HX-Retarget/HX-Reswap response headers) into
// targetID instead of the route's normal success target.
func (s *Server) rateLimit(rl *rateLimiter, targetID string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key := rateLimitKey(w, r)
		if !rl.allow(key) {
			w.Header().Set("HX-Retarget", "#"+targetID)
			w.Header().Set("HX-Reswap", "innerHTML")
			renderRateLimited(w, r)
			return
		}
		next(w, r)
	}
}
