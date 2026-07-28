// internal/web/handlers_qr.go
package web

import (
	"fmt"
	"log"
	"net/http"

	"homesite/internal/qr"
	"homesite/views"
)

func (s *Server) handleWiFiPage(w http.ResponseWriter, r *http.Request) {
	primary := fmt.Sprintf("%s / %s", s.cfg.WiFi.SSID, s.cfg.WiFi.Password)
	render(w, r, views.QRPage("Wi-Fi", "/wifi/qr.png", primary, ""))
}

func (s *Server) handleWiFiQRPng(w http.ResponseWriter, r *http.Request) {
	payload := qr.WiFiPayload(s.cfg.WiFi.SSID, s.cfg.WiFi.Password, s.cfg.WiFi.Auth, s.cfg.WiFi.Hidden)
	writePNG(w, payload)
}

func (s *Server) handleSharePage(w http.ResponseWriter, r *http.Request) {
	render(w, r, views.QRPage("Share", "/share/qr.png", s.cfg.Site.URL, s.cfg.Site.FallbackURL))
}

func (s *Server) handleShareQRPng(w http.ResponseWriter, r *http.Request) {
	payload := qr.URLPayload(s.cfg.Site.URL)
	writePNG(w, payload)
}

func writePNG(w http.ResponseWriter, payload string) {
	b, err := qr.PNG(payload)
	if err != nil {
		log.Printf("web: qr render error: %v", err)
		http.Error(w, "could not render QR code", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Write(b)
}
