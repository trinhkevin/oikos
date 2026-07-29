// internal/web/handlers_admin_test.go
package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"homesite/internal/config"
	"homesite/internal/guestbook"
	"homesite/internal/photos"
	"homesite/internal/store"
)

func newTestServerWithAdmin(t *testing.T) *Server {
	t.Helper()
	cfg, _ := testConfigWithPhotos(t)
	cfg.Admin = config.AdminConfig{Username: "ktrinh", Password: "password"}
	db, err := store.OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return New(cfg, db)
}

func TestAdminDashboardRequiresAuth(t *testing.T) {
	s := newTestServerWithAdmin(t)
	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 with no credentials", rec.Code)
	}
}

func TestAdminDashboardRejectsWrongCredentials(t *testing.T) {
	s := newTestServerWithAdmin(t)
	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	req.SetBasicAuth("ktrinh", "not-the-password")
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 with wrong password", rec.Code)
	}
}

// TestAdminDisabledWhenNoCredentialsConfigured proves an empty
// Admin.Username/Password (the default for every deployment that
// hasn't opted in) fails closed rather than accepting an empty-string
// match.
func TestAdminDisabledWhenNoCredentialsConfigured(t *testing.T) {
	s := newTestServer(t) // testConfig() sets no Admin credentials
	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	req.SetBasicAuth("", "")
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 when admin has no configured credentials", rec.Code)
	}
}

func TestAdminDashboardRendersWithCorrectCredentials(t *testing.T) {
	s := newTestServerWithAdmin(t)
	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	req.SetBasicAuth("ktrinh", "password")
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 with correct credentials", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Moderation") {
		t.Error("expected the dashboard heading to render")
	}
}

func insertTestPhoto(t *testing.T, s *Server) int64 {
	t.Helper()
	id, err := s.photosStore.Insert(context.Background(), photos.Photo{
		FileID: "test-file", Path: "2026-07/test.jpg", ThumbPath: "2026-07/test_thumb.jpg",
		ByteSize: 100, Width: 10, Height: 10, Source: "gallery",
		CreatedAt: time.Now().UTC().Format(time.RFC3339), ClientIP: "127.0.0.1",
	})
	if err != nil {
		t.Fatalf("inserting test photo: %v", err)
	}
	return id
}

func adminRequest(method, path, user, pass string) *http.Request {
	req := httptest.NewRequest(method, path, nil)
	req.SetBasicAuth(user, pass)
	return req
}

func TestAdminPhotoArchiveAndUnarchive(t *testing.T) {
	s := newTestServerWithAdmin(t)
	id := insertTestPhoto(t, s)

	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, adminRequest(http.MethodPost, hidePath(id), "ktrinh", "password"))
	if rec.Code != http.StatusOK {
		t.Fatalf("archive status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Archived") {
		t.Errorf("body = %q, want the Archived badge after hiding", rec.Body.String())
	}
	p, err := s.photosStore.Get(context.Background(), id)
	if err != nil || !p.Hidden {
		t.Fatalf("photo hidden = %v, err = %v; want hidden=true", p.Hidden, err)
	}

	rec = httptest.NewRecorder()
	s.ServeHTTP(rec, adminRequest(http.MethodPost, unhidePath(id), "ktrinh", "password"))
	if rec.Code != http.StatusOK {
		t.Fatalf("unarchive status = %d, want 200", rec.Code)
	}
	p, err = s.photosStore.Get(context.Background(), id)
	if err != nil || p.Hidden {
		t.Fatalf("photo hidden = %v, err = %v; want hidden=false after unarchiving", p.Hidden, err)
	}
}

// TestAdminPhotoDeleteUnlinksGuestbookEntry proves deleting a photo that
// a guest book entry still points to succeeds (by clearing that
// reference first) instead of failing on the enforced photo_id foreign
// key — the whole reason ClearPhotoID exists.
func TestAdminPhotoDeleteUnlinksGuestbookEntry(t *testing.T) {
	s := newTestServerWithAdmin(t)
	photoID := insertTestPhoto(t, s)
	entryID, err := s.guestbookStore.Create(context.Background(), guestbook.Entry{
		Name: "Guest", Message: "Hi!", PhotoID: &photoID, ClientIP: "127.0.0.1",
	})
	if err != nil {
		t.Fatalf("creating guestbook entry: %v", err)
	}

	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, adminRequest(http.MethodPost, deletePhotoPath(photoID), "ktrinh", "password"))
	if rec.Code != http.StatusOK {
		t.Fatalf("delete status = %d, want 200 (should unlink, not fail on FK)", rec.Code)
	}

	if _, err := s.photosStore.Get(context.Background(), photoID); err == nil {
		t.Error("expected the photo row to be gone after delete")
	}
	entries, err := s.guestbookStore.ListForModeration(context.Background())
	if err != nil {
		t.Fatalf("listing entries: %v", err)
	}
	for _, e := range entries {
		if e.ID == entryID && e.PhotoID != nil {
			t.Errorf("entry %d still references deleted photo %d", entryID, photoID)
		}
	}
}

func TestAdminGuestbookHideAndDelete(t *testing.T) {
	s := newTestServerWithAdmin(t)
	entryID, err := s.guestbookStore.Create(context.Background(), guestbook.Entry{
		Name: "Guest", Message: "Hello", ClientIP: "127.0.0.1",
	})
	if err != nil {
		t.Fatalf("creating guestbook entry: %v", err)
	}

	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, adminRequest(http.MethodPost, hideEntryPath(entryID), "ktrinh", "password"))
	if rec.Code != http.StatusOK {
		t.Fatalf("hide status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Hidden") {
		t.Errorf("body = %q, want the Hidden badge", rec.Body.String())
	}

	rec = httptest.NewRecorder()
	s.ServeHTTP(rec, adminRequest(http.MethodPost, deleteEntryPath(entryID), "ktrinh", "password"))
	if rec.Code != http.StatusOK {
		t.Fatalf("delete status = %d, want 200", rec.Code)
	}
	entries, err := s.guestbookStore.ListForModeration(context.Background())
	if err != nil {
		t.Fatalf("listing entries: %v", err)
	}
	for _, e := range entries {
		if e.ID == entryID {
			t.Error("expected the entry to be gone after delete")
		}
	}
}

func hidePath(id int64) string        { return "/admin/photos/" + strconv.FormatInt(id, 10) + "/hide" }
func unhidePath(id int64) string      { return "/admin/photos/" + strconv.FormatInt(id, 10) + "/unhide" }
func deletePhotoPath(id int64) string { return "/admin/photos/" + strconv.FormatInt(id, 10) + "/delete" }
func hideEntryPath(id int64) string   { return "/admin/guestbook/" + strconv.FormatInt(id, 10) + "/hide" }
func deleteEntryPath(id int64) string {
	return "/admin/guestbook/" + strconv.FormatInt(id, 10) + "/delete"
}
