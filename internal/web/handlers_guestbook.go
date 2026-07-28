package web

import (
	"context"
	"errors"
	"log"
	"net/http"

	"homesite/internal/guestbook"
	"homesite/views"
)

func (s *Server) handleGuestbookPage(w http.ResponseWriter, r *http.Request) {
	entries, err := s.guestbookStore.List(r.Context())
	if err != nil {
		log.Printf("web: listing guestbook: %v", err)
	}
	render(w, r, views.GuestbookPage(s.enrichGuestbookEntries(r.Context(), entries)))
}

func (s *Server) handleGuestbookCreate(w http.ResponseWriter, r *http.Request) {
	// The real form always posts multipart/form-data (it has an
	// optional file field), but a plain urlencoded POST with no photo
	// is also accepted gracefully: ParseMultipartForm calls ParseForm
	// internally regardless, so name/message are already populated by
	// the time it returns http.ErrNotMultipart for a non-multipart
	// body — only a genuinely malformed request body is a 400.
	if err := r.ParseMultipartForm(multipartMemoryLimit); err != nil && !errors.Is(err, http.ErrNotMultipart) {
		http.Error(w, "could not parse form", http.StatusBadRequest)
		return
	}
	name := r.PostFormValue("name")
	message := r.PostFormValue("message")

	// The photo is optional — Guest Book and Upload Photos share the
	// same storage backend and ingest pipeline (spec), but a guest book
	// entry attaches at most one photo, linked via PhotoID. No error
	// from FormFile means a file was actually provided.
	var photoID *int64
	if file, fh, err := r.FormFile("photo"); err == nil {
		defer file.Close()
		photo, ingestErr := s.photosIngester.Ingest(r.Context(), file, "guestbook", clientIP(r))
		if ingestErr != nil {
			log.Printf("web: guestbook photo upload rejected (%s): %v", fh.Filename, ingestErr)
			w.Header().Set("HX-Retarget", "#guestbook-form")
			w.Header().Set("HX-Reswap", "outerHTML")
			render(w, r, views.GuestbookForm(&views.GuestbookFormError{
				Message:  uploadErrorMessage(ingestErr),
				Name:     name,
				Message_: message,
			}))
			return
		}
		photoID = &photo.ID
	}

	_, err := s.guestbookStore.Create(r.Context(), guestbook.Entry{
		Name: name, Message: message, PhotoID: photoID, ClientIP: clientIP(r),
	})
	if err != nil {
		// The form's own hx-target/hx-swap ("#guestbook-entries" /
		// "afterbegin") are correct for the success case, but this
		// validation-error fragment is a whole <form>, not a new entry —
		// left alone, htmx would prepend it into the entries list instead
		// of updating the form the guest is looking at. Retarget/reswap
		// this one response so it replaces #guestbook-form in place.
		w.Header().Set("HX-Retarget", "#guestbook-form")
		w.Header().Set("HX-Reswap", "outerHTML")
		render(w, r, views.GuestbookForm(&views.GuestbookFormError{
			Message:  guestbookErrorMessage(err),
			Name:     name,
			Message_: message,
		}))
		return
	}

	entries, err := s.guestbookStore.List(r.Context())
	if err != nil {
		log.Printf("web: listing guestbook after create: %v", err)
	}
	// The newest entry is entries[0] (List orders DESC by created_at);
	// hx-swap="afterbegin" on #guestbook-entries means only that one new
	// entry should be sent back, not the whole list re-rendered.
	if len(entries) > 0 {
		render(w, r, views.GuestbookEntries(s.enrichGuestbookEntries(r.Context(), entries[:1])))
	}
}

// enrichGuestbookEntries loads the linked Photo (path/thumb_path) for
// every entry that has a PhotoID, so the template can render a
// thumbnail — guestbook.Store.List only returns the bare PhotoID, not
// photo details. A photo lookup failure is logged and the entry is
// rendered without its photo rather than failing the whole page.
func (s *Server) enrichGuestbookEntries(ctx context.Context, entries []guestbook.Entry) []views.GuestbookEntryView {
	out := make([]views.GuestbookEntryView, len(entries))
	for i, e := range entries {
		out[i] = views.GuestbookEntryView{Entry: e}
		if e.PhotoID != nil {
			photo, err := s.photosStore.Get(ctx, *e.PhotoID)
			if err != nil {
				log.Printf("web: loading guestbook photo %d: %v", *e.PhotoID, err)
				continue
			}
			out[i].Photo = &photo
		}
	}
	return out
}

func guestbookErrorMessage(err error) string {
	switch {
	case errors.Is(err, guestbook.ErrNameRequired):
		return "please enter a name"
	case errors.Is(err, guestbook.ErrNameTooLong):
		return "name is too long — 40 characters max"
	case errors.Is(err, guestbook.ErrMessageRequired):
		return "please enter a message"
	case errors.Is(err, guestbook.ErrMessageTooLong):
		return "message is too long — 500 characters max"
	default:
		return "couldn't save that — try again"
	}
}
