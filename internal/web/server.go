// internal/web/server.go
package web

import (
	"io/fs"
	"net/http"

	homesite "homesite"
	"homesite/internal/config"
	"homesite/internal/content"
)

type Server struct {
	mux           *http.ServeMux
	cfg           *config.Config
	menuLoader    *content.MenuLoader
	catLoader     *content.CatLoader
	welcomeLoader *content.WelcomeLoader
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

	s.mux.HandleFunc("GET /healthz", s.handleHealthz)
	s.registerPageRoutes()

	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}
