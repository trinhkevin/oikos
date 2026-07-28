// internal/web/handlers_spotify_auth.go
package web

import (
	"crypto/rand"
	"encoding/hex"
	"log"
	"net/http"
)

// randomToken generates a random hex string, used here for the OAuth
// state parameter that guards /spotify/callback against CSRF. This is
// deliberately not the same concept as the rate limiter's per-guest
// identity: Task 16 removed a cookie-keyed rate-limit token because it
// let guests reset their budget by clearing cookies. The OAuth state
// cookie below has no such property to protect — it's a short-lived,
// single-use value checked once on callback, not a durable identity — so
// reusing the concept here doesn't reintroduce that bug.
func randomToken() string {
	var b [16]byte
	rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

func (s *Server) handleSpotifyLogin(w http.ResponseWriter, r *http.Request) {
	if !isLoopback(r) {
		http.NotFound(w, r)
		return
	}
	state := randomToken()
	http.SetCookie(w, &http.Cookie{
		Name: "spotify_oauth_state", Value: state, Path: "/spotify", MaxAge: 300, HttpOnly: true,
	})
	http.Redirect(w, r, s.spotifyClient.AuthURL(state), http.StatusFound)
}

func (s *Server) handleSpotifyCallback(w http.ResponseWriter, r *http.Request) {
	if !isLoopback(r) {
		http.NotFound(w, r)
		return
	}
	cookie, err := r.Cookie("spotify_oauth_state")
	if err != nil || r.URL.Query().Get("state") != cookie.Value {
		http.Error(w, "state mismatch — go back to /spotify/login and try again", http.StatusBadRequest)
		return
	}
	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "authorization was not granted: "+r.URL.Query().Get("error"), http.StatusBadRequest)
		return
	}
	if err := s.spotifyClient.ExchangeCode(r.Context(), code); err != nil {
		log.Printf("web: spotify code exchange failed: %v", err)
		http.Error(w, "authorization failed — check the server logs", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte("<h1>Spotify connected</h1><p>You can close this tab.</p>"))
}
