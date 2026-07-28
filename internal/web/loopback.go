// internal/web/loopback.go
package web

import (
	"net"
	"net/http"
)

// isLoopback reports whether r originated from 127.0.0.1 or ::1. Used to
// gate /spotify/login and /spotify/callback, which share the site's one
// port with every guest-facing route but must never be guest-reachable.
func isLoopback(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
