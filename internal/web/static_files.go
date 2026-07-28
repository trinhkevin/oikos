// internal/web/static_files.go
package web

import (
	"log"
	"net/http"
	"path/filepath"
	"strings"
)

// handleMedia serves cat photos from content/cats/. Directory listings
// are refused outright (no legitimate request needs one, and the bare
// http.FileServer this used to be wired to directly would otherwise
// serve one for any directory path).
func (s *Server) handleMedia(w http.ResponseWriter, r *http.Request) {
	rel := strings.TrimPrefix(r.URL.Path, "/media/")
	if rel == "" || strings.HasSuffix(rel, "/") {
		http.NotFound(w, r)
		return
	}
	http.StripPrefix("/media/", http.FileServer(http.Dir(filepath.Join(s.cfg.ContentDir, "cats")))).ServeHTTP(w, r)
}

// handleUploads serves guest-uploaded photos and thumbnails from
// UploadsDir. Two things a bare http.FileServer would get wrong:
//  1. Directory listings — this tree is guest-writable, and a listing
//     would expose e.g. the ephemeral upload's on-disk layout.
//  2. Moderation — a photo (or guest book entry) an owner has hidden
//     must actually stop being servable, not just disappear from the
//     gallery/entries list while remaining fetchable by anyone who
//     already has (or guesses) its URL.
func (s *Server) handleUploads(w http.ResponseWriter, r *http.Request) {
	rel := strings.TrimPrefix(r.URL.Path, "/uploads/")
	if rel == "" || strings.HasSuffix(rel, "/") {
		http.NotFound(w, r)
		return
	}
	hidden, err := s.photosStore.IsHidden(r.Context(), rel)
	if err != nil {
		log.Printf("web: checking photo hidden status for %s: %v", rel, err)
	}
	if hidden {
		http.NotFound(w, r)
		return
	}
	http.StripPrefix("/uploads/", http.FileServer(http.Dir(s.cfg.UploadsDir))).ServeHTTP(w, r)
}
