// internal/web/server.go
package web

import (
	"database/sql"
	"io/fs"
	"net/http"
	"path/filepath"
	"time"

	homesite "homesite"
	"homesite/internal/config"
	"homesite/internal/content"
	"homesite/internal/guestbook"
	"homesite/internal/photos"
	"homesite/internal/spotify"
)

type Server struct {
	mux                *http.ServeMux
	cfg                *config.Config
	menuLoader         *content.MenuLoader
	catLoader          *content.CatLoader
	welcomeLoader      *content.WelcomeLoader
	photosStore        *photos.Store
	photosIngester     *photos.Ingester
	guestbookStore     *guestbook.Store
	guestbookLimiter   *rateLimiter
	photosLimiter      *rateLimiter
	spotifyClient      *spotify.Client
	requestStore       *spotify.RequestStore
	songRequestLimiter *rateLimiter
}

func New(cfg *config.Config, db *sql.DB) *Server {
	cache := content.NewCache()
	photosStore := photos.NewStore(db)

	s := &Server{
		mux:            http.NewServeMux(),
		cfg:            cfg,
		menuLoader:     content.NewMenuLoader(cache),
		catLoader:      content.NewCatLoader(cache, filepath.Join(cfg.ContentDir, "cats")),
		welcomeLoader:  content.NewWelcomeLoader(cache),
		photosStore:    photosStore,
		photosIngester: photos.NewIngester(photosStore, cfg.Photos, cfg.UploadsDir),
		guestbookStore: guestbook.NewStore(db),
	}

	windowDur := time.Duration(cfg.Limits.WindowMinutes) * time.Minute
	s.guestbookLimiter = newRateLimiter(cfg.Limits.GuestbookPerWindow, windowDur)
	s.photosLimiter = newRateLimiter(cfg.Limits.PhotoUploadsPerWindow, windowDur)

	tokenStore := spotify.NewSQLTokenStore(db)
	s.spotifyClient = spotify.New(
		cfg.Spotify.ClientID, cfg.Spotify.ClientSecret, cfg.Spotify.RedirectURI,
		tokenStore, time.Duration(cfg.Spotify.NowPlayingCacheSeconds)*time.Second,
	)
	s.requestStore = spotify.NewRequestStore(db)
	s.songRequestLimiter = newRateLimiter(cfg.Limits.SongRequestsPerWindow, windowDur)

	staticSub, err := fs.Sub(homesite.StaticFS, "static")
	if err != nil {
		panic("web: static assets missing from embed: " + err.Error())
	}
	s.mux.Handle("/static/", http.StripPrefix("/static/", http.FileServerFS(staticSub)))

	s.mux.HandleFunc("GET /media/", s.handleMedia)
	s.mux.HandleFunc("GET /uploads/", s.handleUploads)

	s.mux.HandleFunc("GET /healthz", s.handleHealthz)
	s.registerPageRoutes()

	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}
