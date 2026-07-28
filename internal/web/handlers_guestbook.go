package web

import (
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
	render(w, r, views.GuestbookPage(entries))
}

func (s *Server) handleGuestbookCreate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "could not parse form", http.StatusBadRequest)
		return
	}
	name := r.PostFormValue("name")
	message := r.PostFormValue("message")

	_, err := s.guestbookStore.Create(r.Context(), guestbook.Entry{
		Name: name, Message: message, ClientIP: clientIP(r),
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
		render(w, r, views.GuestbookEntries(entries[:1]))
	}
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
