// internal/web/handlers_music.go
package web

import (
	"errors"
	"log"
	"net/http"
	"time"

	"homesite/internal/spotify"
	"homesite/views"
)

func (s *Server) handleMusicPage(w http.ResponseWriter, r *http.Request) {
	nowPlaying, err := s.spotifyClient.NowPlaying(r.Context())
	nowPlayingErr := err != nil
	if err != nil {
		log.Printf("web: now-playing error: %v", err)
	}

	recent, err := s.requestStore.Recent(r.Context(), 10)
	if err != nil {
		log.Printf("web: recent requests error: %v", err)
	}

	render(w, r, views.MusicPage(nowPlaying, nowPlayingErr, recent))
}

func (s *Server) handleMusicSearch(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	if q == "" {
		render(w, r, views.SearchResults(nil))
		return
	}
	tracks, err := s.spotifyClient.Search(r.Context(), q)
	if err != nil {
		log.Printf("web: search error: %v", err)
		render(w, r, views.SearchResultsError(spotifyErrorMessage(err)))
		return
	}
	render(w, r, views.SearchResults(tracks))
}

func (s *Server) handleMusicRequest(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "could not parse form", http.StatusBadRequest)
		return
	}
	uri := r.PostFormValue("uri")
	name := r.PostFormValue("name")
	artist := r.PostFormValue("artist")
	nickname := r.PostFormValue("nickname")
	ip := clientIP(r)

	err := s.spotifyClient.QueueTrack(r.Context(), uri)
	status := "queued"
	message := "Added to the queue!"
	if err != nil {
		status = "failed"
		message = spotifyErrorMessage(err)
		log.Printf("web: queue error: %v", err)
	}

	if _, insertErr := s.requestStore.Insert(r.Context(), spotify.SongRequest{
		TrackURI: uri, TrackName: name, ArtistName: artist, RequestedBy: nickname,
		CreatedAt: time.Now().UTC().Format(time.RFC3339), ClientIP: ip, Status: status,
	}); insertErr != nil {
		log.Printf("web: recording song request: %v", insertErr)
	}

	render(w, r, views.RequestResult(err == nil, message))
}

func spotifyErrorMessage(err error) string {
	switch {
	case errors.Is(err, spotify.ErrNoActiveDevice):
		return "Nothing's playing yet — ask the host to start the music 🎵"
	case errors.Is(err, spotify.ErrTokenInvalid):
		return "Song requests are down right now"
	case errors.Is(err, spotify.ErrRateLimited):
		return "Spotify's throttling us — try again in a minute"
	default:
		return "Couldn't reach Spotify, try again"
	}
}
