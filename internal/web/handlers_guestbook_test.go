package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"homesite/internal/guestbook"
)

func guestbookEntryFixture(name, message string) guestbook.Entry {
	return guestbook.Entry{Name: name, Message: message, ClientIP: "192.168.1.30"}
}

func TestGuestbookPageRendersEntries(t *testing.T) {
	s := newTestServer(t)
	if _, err := s.guestbookStore.Create(t.Context(), guestbookEntryFixture("Alex", "Loved the negroni")); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/guestbook", nil)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Loved the negroni") {
		t.Error("expected the seeded entry to render")
	}
}

func TestGuestbookCreateSucceeds(t *testing.T) {
	s := newTestServer(t)
	form := url.Values{"name": {"Jamie"}, "message": {"Great cats"}}
	req := httptest.NewRequest(http.MethodPost, "/guestbook", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Great cats") {
		t.Error("expected the new entry in the response fragment")
	}
}

func TestGuestbookCreateRejectsEmptyMessagePreservingName(t *testing.T) {
	s := newTestServer(t)
	form := url.Values{"name": {"Jamie"}, "message": {""}}
	req := httptest.NewRequest(http.MethodPost, "/guestbook", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (validation error still renders a fragment)", rec.Code)
	}
	body := rec.Body.String()
	// value="Jamie" specifically (not just "Jamie" appearing anywhere,
	// which a successfully-created entry's rendered name would also
	// satisfy) — this can only come from the form's preserved input
	// attribute on the validation-failure branch.
	if !strings.Contains(body, `value="Jamie"`) {
		t.Errorf("body = %q, want the typed name preserved as a form field value (value=\"Jamie\")", body)
	}
	// The exact validation error text, not just the substring "message"
	// (which the textarea's own name="message" attribute would also
	// satisfy on the success path).
	if !strings.Contains(body, "please enter a message") {
		t.Errorf("body = %q, want the specific message-required error text", body)
	}
	// This fragment must be the form itself re-rendered, not a new
	// guestbook entry — the success path never contains this element.
	if !strings.Contains(body, `id="guestbook-form"`) {
		t.Errorf("body = %q, want the guestbook form fragment, not an entry", body)
	}
}

func TestGuestbookCreateValidationFailureRetargetsToForm(t *testing.T) {
	s := newTestServer(t)
	form := url.Values{"name": {"Jamie"}, "message": {""}}
	req := httptest.NewRequest(http.MethodPost, "/guestbook", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)

	if got := rec.Header().Get("HX-Retarget"); got != "#guestbook-form" {
		t.Errorf("HX-Retarget = %q, want #guestbook-form", got)
	}
	if got := rec.Header().Get("HX-Reswap"); got != "outerHTML" {
		t.Errorf("HX-Reswap = %q, want outerHTML", got)
	}
}
