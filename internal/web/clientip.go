// internal/web/clientip.go
package web

import (
	"net"
	"net/http"
)

// clientIP extracts the caller's IP from RemoteAddr, stripping the port.
// This is a LAN-only site with no reverse proxy in front of it, so
// RemoteAddr is always the guest's real address — no X-Forwarded-For
// handling is needed or trusted.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
