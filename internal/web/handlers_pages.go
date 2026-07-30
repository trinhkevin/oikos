package web

import (
	"log"
	"net/http"

	"homesite/views"
)

func (s *Server) registerPageRoutes() {
	s.mux.HandleFunc("GET /{$}", s.handleWelcome)
	s.mux.HandleFunc("GET /cocktails", s.handleMenu(s.cfg.ContentDir+"/drinks.yaml"))
	s.mux.HandleFunc("GET /refreshments", s.handleMenu(s.cfg.ContentDir+"/food.yaml"))
	s.mux.HandleFunc("GET /entertainment", s.handleEntertainment)
	s.mux.HandleFunc("GET /cats", s.handleCatsIndex)
	s.mux.HandleFunc("GET /recipes", s.handleRecipesIndex)
	s.mux.HandleFunc("GET /recipes/{slug}", s.handleRecipeDetail)
	s.mux.HandleFunc("GET /wifi", s.handleWiFiPage)
	s.mux.HandleFunc("GET /wifi/qr.png", s.handleWiFiQRPng)
	s.mux.HandleFunc("GET /share", s.handleSharePage)
	s.mux.HandleFunc("GET /share/qr.png", s.handleShareQRPng)
	s.mux.HandleFunc("GET /photos", s.handlePhotosPage)
	s.mux.HandleFunc("POST /photos", s.rateLimit(s.photosLimiter, "photos-rl-message", s.handlePhotosUpload))
	s.mux.HandleFunc("GET /guestbook", s.handleGuestbookPage)
	s.mux.HandleFunc("POST /guestbook", s.rateLimit(s.guestbookLimiter, "guestbook-rl-message", s.handleGuestbookCreate))
	s.mux.HandleFunc("GET /music", s.handleMusicPage)
	s.mux.HandleFunc("GET /music/now-playing", s.handleMusicNowPlayingFragment)
	s.mux.HandleFunc("GET /music/queue", s.handleMusicQueueFragment)
	s.mux.HandleFunc("GET /music/search", s.handleMusicSearch)
	s.mux.HandleFunc("POST /music/request", s.rateLimit(s.songRequestLimiter, "music-request-rl-message", s.handleMusicRequest))
	s.mux.HandleFunc("GET /spotify/login", s.handleSpotifyLogin)
	s.mux.HandleFunc("GET /spotify/callback", s.handleSpotifyCallback)

	s.mux.HandleFunc("GET /admin", s.requireAdminAuth(s.handleAdminDashboard))
	s.mux.HandleFunc("POST /admin/photos/{id}/hide", s.requireAdminAuth(s.handleAdminPhotoHide))
	s.mux.HandleFunc("POST /admin/photos/{id}/unhide", s.requireAdminAuth(s.handleAdminPhotoUnhide))
	s.mux.HandleFunc("POST /admin/photos/{id}/delete", s.requireAdminAuth(s.handleAdminPhotoDelete))
	s.mux.HandleFunc("POST /admin/guestbook/{id}/hide", s.requireAdminAuth(s.handleAdminGuestbookHide))
	s.mux.HandleFunc("POST /admin/guestbook/{id}/unhide", s.requireAdminAuth(s.handleAdminGuestbookUnhide))
	s.mux.HandleFunc("POST /admin/guestbook/{id}/delete", s.requireAdminAuth(s.handleAdminGuestbookDelete))
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

func (s *Server) handleEntertainment(w http.ResponseWriter, r *http.Request) {
	render(w, r, views.EntertainmentPage())
}

func (s *Server) handleCatsIndex(w http.ResponseWriter, r *http.Request) {
	cats, err := s.catLoader.LoadAll()
	if err != nil {
		log.Printf("web: cats index error: %v", err)
	}
	render(w, r, views.CatsIndex(cats))
}

func (s *Server) handleRecipesIndex(w http.ResponseWriter, r *http.Request) {
	recipes, err := s.recipeLoader.LoadAll()
	if err != nil {
		log.Printf("web: recipes index error: %v", err)
	}
	render(w, r, views.RecipesIndex(recipes))
}

func (s *Server) handleRecipeDetail(w http.ResponseWriter, r *http.Request) {
	recipe, err := s.recipeLoader.LoadOne(r.PathValue("slug"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	render(w, r, views.RecipeDetail(recipe))
}
