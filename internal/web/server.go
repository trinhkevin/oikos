// internal/web/server.go
package web

import (
	"database/sql"
	"io/fs"
	"net/http"

	homesite "homesite"
	"homesite/internal/config"
	"homesite/internal/content"
	"homesite/internal/guestbook"
	"homesite/internal/photos"
)

type Server struct {
	mux            *http.ServeMux
	cfg            *config.Config
	menuLoader     *content.MenuLoader
	catLoader      *content.CatLoader
	welcomeLoader  *content.WelcomeLoader
	photosStore    *photos.Store
	photosIngester *photos.Ingester
	guestbookStore *guestbook.Store
}

func New(cfg *config.Config, db *sql.DB) *Server {
	cache := content.NewCache()
	photosStore := photos.NewStore(db)

	s := &Server{
		mux:            http.NewServeMux(),
		cfg:            cfg,
		menuLoader:     content.NewMenuLoader(cache),
		catLoader:      content.NewCatLoader(cache, cfg.ContentDir+"/cats"),
		welcomeLoader:  content.NewWelcomeLoader(cache),
		photosStore:    photosStore,
		photosIngester: photos.NewIngester(photosStore, cfg.Photos, cfg.UploadsDir),
		guestbookStore: guestbook.NewStore(db),
	}

	staticSub, err := fs.Sub(homesite.StaticFS, "static")
	if err != nil {
		panic("web: static assets missing from embed: " + err.Error())
	}
	s.mux.Handle("/static/", http.StripPrefix("/static/", http.FileServerFS(staticSub)))

	catsPhotoDir := http.Dir(cfg.ContentDir + "/cats")
	s.mux.Handle("/media/", http.StripPrefix("/media/", http.FileServer(catsPhotoDir)))

	uploadsDir := http.Dir(cfg.UploadsDir)
	s.mux.Handle("/uploads/", http.StripPrefix("/uploads/", http.FileServer(uploadsDir)))

	s.mux.HandleFunc("GET /healthz", s.handleHealthz)
	s.registerPageRoutes()

	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}
