// internal/web/handlers_admin.go
package web

import (
	"crypto/subtle"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"homesite/views"
)

// requireAdminAuth gates every /admin route behind HTTP Basic Auth.
// Empty configured credentials mean admin is disabled — fail closed,
// not an accidental open door from an empty-string match.
func (s *Server) requireAdminAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		validUser := s.cfg.Admin.Username != "" && subtle.ConstantTimeCompare([]byte(user), []byte(s.cfg.Admin.Username)) == 1
		validPass := s.cfg.Admin.Password != "" && subtle.ConstantTimeCompare([]byte(pass), []byte(s.cfg.Admin.Password)) == 1
		if !ok || !validUser || !validPass {
			w.Header().Set("WWW-Authenticate", `Basic realm="Brivin Household Moderation"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

func (s *Server) handleAdminDashboard(w http.ResponseWriter, r *http.Request) {
	photoList, err := s.photosStore.ListForModeration(r.Context())
	if err != nil {
		log.Printf("web: admin photo list error: %v", err)
	}
	entries, err := s.guestbookStore.ListForModeration(r.Context())
	if err != nil {
		log.Printf("web: admin guestbook list error: %v", err)
	}
	render(w, r, views.AdminPage(photoList, entries))
}

func (s *Server) parseAdminID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return 0, false
	}
	return id, true
}

func (s *Server) handleAdminPhotoHide(w http.ResponseWriter, r *http.Request) {
	id, ok := s.parseAdminID(w, r)
	if !ok {
		return
	}
	if err := s.photosStore.SetHidden(r.Context(), id, true); err != nil {
		log.Printf("web: admin archive photo %d: %v", id, err)
		http.Error(w, "could not archive photo", http.StatusInternalServerError)
		return
	}
	p, err := s.photosStore.Get(r.Context(), id)
	if err != nil {
		log.Printf("web: admin re-fetch photo %d: %v", id, err)
		http.Error(w, "archived, but could not refresh the row", http.StatusInternalServerError)
		return
	}
	render(w, r, views.AdminPhotoItem(p))
}

func (s *Server) handleAdminPhotoUnhide(w http.ResponseWriter, r *http.Request) {
	id, ok := s.parseAdminID(w, r)
	if !ok {
		return
	}
	if err := s.photosStore.SetHidden(r.Context(), id, false); err != nil {
		log.Printf("web: admin unarchive photo %d: %v", id, err)
		http.Error(w, "could not unarchive photo", http.StatusInternalServerError)
		return
	}
	p, err := s.photosStore.Get(r.Context(), id)
	if err != nil {
		log.Printf("web: admin re-fetch photo %d: %v", id, err)
		http.Error(w, "unarchived, but could not refresh the row", http.StatusInternalServerError)
		return
	}
	render(w, r, views.AdminPhotoItem(p))
}

// handleAdminPhotoDelete permanently removes a photo: unlink any guest
// book entry pointing at it first (photo_id is an enforced foreign key —
// deleting a still-referenced row would otherwise fail outright), then
// delete the DB row, then best-effort remove its files from disk. The
// response is empty, which HTMX's outerHTML swap turns into "this item
// is now gone" without a page reload.
func (s *Server) handleAdminPhotoDelete(w http.ResponseWriter, r *http.Request) {
	id, ok := s.parseAdminID(w, r)
	if !ok {
		return
	}
	if err := s.guestbookStore.ClearPhotoID(r.Context(), id); err != nil {
		log.Printf("web: admin unlink guestbook photo %d: %v", id, err)
		http.Error(w, "could not delete photo", http.StatusInternalServerError)
		return
	}
	p, err := s.photosStore.Delete(r.Context(), id)
	if err != nil {
		log.Printf("web: admin delete photo %d: %v", id, err)
		http.Error(w, "could not delete photo", http.StatusInternalServerError)
		return
	}
	for _, rel := range []string{p.Path, p.ThumbPath} {
		if rel == "" {
			continue
		}
		if err := os.Remove(filepath.Join(s.cfg.UploadsDir, rel)); err != nil && !os.IsNotExist(err) {
			log.Printf("web: admin removing file %s for deleted photo %d: %v", rel, id, err)
		}
	}
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleAdminGuestbookHide(w http.ResponseWriter, r *http.Request) {
	id, ok := s.parseAdminID(w, r)
	if !ok {
		return
	}
	if err := s.guestbookStore.SetHidden(r.Context(), id, true); err != nil {
		log.Printf("web: admin hide guestbook entry %d: %v", id, err)
		http.Error(w, "could not hide entry", http.StatusInternalServerError)
		return
	}
	s.renderAdminGuestbookItem(w, r, id)
}

func (s *Server) handleAdminGuestbookUnhide(w http.ResponseWriter, r *http.Request) {
	id, ok := s.parseAdminID(w, r)
	if !ok {
		return
	}
	if err := s.guestbookStore.SetHidden(r.Context(), id, false); err != nil {
		log.Printf("web: admin unhide guestbook entry %d: %v", id, err)
		http.Error(w, "could not unhide entry", http.StatusInternalServerError)
		return
	}
	s.renderAdminGuestbookItem(w, r, id)
}

func (s *Server) handleAdminGuestbookDelete(w http.ResponseWriter, r *http.Request) {
	id, ok := s.parseAdminID(w, r)
	if !ok {
		return
	}
	if err := s.guestbookStore.Delete(r.Context(), id); err != nil {
		log.Printf("web: admin delete guestbook entry %d: %v", id, err)
		http.Error(w, "could not delete entry", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// renderAdminGuestbookItem re-fetches one entry by scanning the full
// moderation list — guestbook.Store has no single-entry Get today, and
// this list is small enough (a household party, not a stadium) that
// adding one isn't worth it yet.
func (s *Server) renderAdminGuestbookItem(w http.ResponseWriter, r *http.Request, id int64) {
	entries, err := s.guestbookStore.ListForModeration(r.Context())
	if err != nil {
		log.Printf("web: admin re-list guestbook after update: %v", err)
		http.Error(w, "updated, but could not refresh the row", http.StatusInternalServerError)
		return
	}
	for _, e := range entries {
		if e.ID == id {
			render(w, r, views.AdminGuestbookItem(e))
			return
		}
	}
	w.WriteHeader(http.StatusOK)
}
