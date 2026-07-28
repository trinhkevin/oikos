package web

import (
	"log"
	"net/http"

	"homesite/views"
)

func (s *Server) registerPageRoutes() {
	s.mux.HandleFunc("GET /{$}", s.handleWelcome)
	s.mux.HandleFunc("GET /coffee", s.handleMenu(s.cfg.ContentDir+"/coffee.yaml"))
	s.mux.HandleFunc("GET /cocktails", s.handleMenu(s.cfg.ContentDir+"/cocktails.yaml"))
	s.mux.HandleFunc("GET /refreshments", s.handleMenu(s.cfg.ContentDir+"/refreshments.yaml"))
	s.mux.HandleFunc("GET /cats", s.handleCatsIndex)
	s.mux.HandleFunc("GET /cats/{slug}", s.handleCatDetail)
	s.mux.HandleFunc("GET /wifi", s.handleWiFiPage)
	s.mux.HandleFunc("GET /wifi/qr.png", s.handleWiFiQRPng)
	s.mux.HandleFunc("GET /share", s.handleSharePage)
	s.mux.HandleFunc("GET /share/qr.png", s.handleShareQRPng)
	s.mux.HandleFunc("GET /photos", s.handlePhotosPage)
	s.mux.HandleFunc("POST /photos", s.rateLimit(s.photosLimiter, "photos-rl-message", s.handlePhotosUpload))
	s.mux.HandleFunc("GET /guestbook", s.handleGuestbookPage)
	s.mux.HandleFunc("POST /guestbook", s.rateLimit(s.guestbookLimiter, "guestbook-rl-message", s.handleGuestbookCreate))
}

func (s *Server) handleWelcome(w http.ResponseWriter, r *http.Request) {
	html, err := s.welcomeLoader.Load(s.cfg.ContentDir + "/welcome.md")
	if err != nil {
		log.Printf("web: welcome content error: %v", err)
	}
	render(w, r, views.Welcome(html))
}

func (s *Server) handleMenu(path string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		menu, err := s.menuLoader.Load(path)
		if err != nil {
			log.Printf("web: menu content error (%s): %v", path, err)
		}
		render(w, r, views.MenuPage(menu))
	}
}

func (s *Server) handleCatsIndex(w http.ResponseWriter, r *http.Request) {
	cats, err := s.catLoader.LoadAll()
	if err != nil {
		log.Printf("web: cats index error: %v", err)
	}
	render(w, r, views.CatsIndex(cats))
}

func (s *Server) handleCatDetail(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	cat, err := s.catLoader.LoadOne(slug)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	render(w, r, views.CatDetail(cat))
}
