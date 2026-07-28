// internal/web/handlers_music.go
package web

import (
	"context"
	"errors"
	"log"
	"net/http"
	"time"

	"homesite/internal/spotify"
	"homesite/views"
)

// fetchQueue centralizes the queue-error logging so every caller (the
// initial page render, the standalone queue-panel route, and the
// post-request refresh) degrades the same way on a Spotify error.
func (s *Server) fetchQueue(ctx context.Context) []spotify.Track {
	queue, err := s.spotifyClient.Queue(ctx)
	if err != nil {
		log.Printf("web: queue error: %v", err)
	}
	return queue
}

func (s *Server) handleMusicPage(w http.ResponseWriter, r *http.Request) {
	nowPlaying, err := s.spotifyClient.NowPlaying(r.Context())
	nowPlayingErr := err != nil
	if err != nil {
		log.Printf("web: now-playing error: %v", err)
	}
	queue := s.fetchQueue(r.Context())
	render(w, r, views.MusicPage(nowPlaying, nowPlayingErr, queue))
}

// handleMusicNowPlayingFragment backs the Music page's live-updating
// now-playing section: HTMX polls this on an interval and swaps in the
// returned fragment, so guests see the current track change without
// ever reloading the page. The queue panel is deliberately not part of
// this poll — it would otherwise silently kick a guest out of an
// in-progress search every few seconds.
func (s *Server) handleMusicNowPlayingFragment(w http.ResponseWriter, r *http.Request) {
	nowPlaying, err := s.spotifyClient.NowPlaying(r.Context())
	nowPlayingErr := err != nil
	if err != nil {
		log.Printf("web: now-playing error: %v", err)
	}
	render(w, r, views.NowPlayingFragment(nowPlaying, nowPlayingErr))
}

// handleMusicQueueFragment renders the queue panel's default (non-
// search) state. It backs both the "✕ close search" button and the
// out-of-band refresh after a song is successfully queued.
func (s *Server) handleMusicQueueFragment(w http.ResponseWriter, r *http.Request) {
	render(w, r, views.QueueView(s.fetchQueue(r.Context())))
}

// minSearchQueryLength guards against firing a Spotify search on every
// keystroke of a 300ms debounce: a single guest typing generates a
// burst of near-useless 0-1 character queries that would otherwise all
// reach the API. There's no rate limit or minimum-length guard on this
// route today, so this is the cheap mitigation for most of that risk.
const minSearchQueryLength = 2

func (s *Server) handleMusicSearch(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	if len(q) < minSearchQueryLength {
		render(w, r, views.SearchViewEmpty())
		return
	}
	tracks, err := s.spotifyClient.Search(r.Context(), q)
	if err != nil {
		log.Printf("web: search error: %v", err)
		render(w, r, views.SearchViewError(spotifyErrorMessage(err)))
		return
	}
	if len(tracks) == 0 {
		render(w, r, views.SearchViewEmpty())
		return
	}
	render(w, r, views.SearchView(tracks))
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

	// name/artist come from hidden form fields that the search-results UI
	// populates from Spotify's own (always short) metadata, but a request
	// can be crafted directly — reject oversized values before ever
	// touching Spotify's queue or the database. RequestStore.Insert
	// enforces the same cap independently as a domain-layer backstop.
	if len(name) > spotify.MaxTrackNameLength || len(artist) > spotify.MaxArtistNameLength {
		render(w, r, views.RequestResult(false, "That request looks invalid — try searching again"))
		return
	}

	err := s.spotifyClient.QueueTrack(r.Context(), uri)
	status := "queued"
	message := "Added to the queue!"
	if err != nil {
		status = "failed"
		message = spotifyErrorMessage(err)
		log.Printf("web: queue error: %v", err)
	}

	success := err == nil
	if _, insertErr := s.requestStore.Insert(r.Context(), spotify.SongRequest{
		TrackURI: uri, TrackName: name, ArtistName: artist, RequestedBy: nickname,
		CreatedAt: time.Now().UTC().Format(time.RFC3339), ClientIP: ip, Status: status,
	}); insertErr != nil {
		if errors.Is(insertErr, spotify.ErrTrackNameTooLong) || errors.Is(insertErr, spotify.ErrArtistNameTooLong) {
			success = false
			message = "That request looks invalid — try searching again"
		}
		log.Printf("web: recording song request: %v", insertErr)
	}

	if success {
		// Refresh the queue panel back to the queue view (out-of-band)
		// alongside the toast — this is what returns a guest to the queue
		// after queueing from within a search, per the "queue a song to
		// show the queue again" requirement. A failed request leaves the
		// search results in place instead, so the guest can retry.
		render(w, r, views.RequestResultWithQueueRefresh(success, message, s.fetchQueue(r.Context())))
		return
	}
	render(w, r, views.RequestResult(success, message))
}

func spotifyErrorMessage(err error) string {
	switch {
	case errors.Is(err, spotify.ErrNoActiveDevice):
		return "Nothing's playing yet — ask the host to start the music 🎵"
	case errors.Is(err, spotify.ErrTokenInvalid):
		return "Song requests are down right now"
	case errors.Is(err, spotify.ErrRateLimited):
		return "Spotify's throttling us — try again in a minute"
	case errors.Is(err, spotify.ErrPremiumRequired):
		return "Queueing needs Spotify Premium on the host's account"
	case errors.Is(err, spotify.ErrPlaybackRejected):
		return "Spotify rejected that — try again once something's playing"
	default:
		return "Couldn't reach Spotify, try again"
	}
}
