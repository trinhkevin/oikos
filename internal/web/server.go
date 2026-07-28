// internal/web/server.go
package web

import (
	"io/fs"
	"net/http"

	homesite "homesite"
	"homesite/internal/config"
	"homesite/internal/content"
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
}

func New(cfg *config.Config) *Server {
	cache := content.NewCache()
	s := &Server{
		mux:           http.NewServeMux(),
		cfg:           cfg,
		menuLoader:    content.NewMenuLoader(cache),
		catLoader:     content.NewCatLoader(cache, cfg.ContentDir+"/cats"),
		welcomeLoader: content.NewWelcomeLoader(cache),
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

	// Photo store and ingester are deliberately NOT constructed here yet —
	// they need a live *sql.DB, which main.go opens once at startup in
	// Task 15 alongside the guest book (same database). Until then, tests
	// construct a Server via New and set s.photosStore/s.photosIngester
	// directly. This is a deliberate, temporary seam that Task 15 closes.

	s.mux.HandleFunc("GET /healthz", s.handleHealthz)
	s.registerPageRoutes()

	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}
