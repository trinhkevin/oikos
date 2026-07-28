// internal/web/static_files_test.go
package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"homesite/internal/photos"
)

// TestUploadsServesHiddenPhotoAsNotFound proves the security fix: a
// photo an owner has marked hidden must actually stop being fetchable
// via /uploads/, not just disappear from the /photos gallery listing.
func TestUploadsServesHiddenPhotoAsNotFound(t *testing.T) {
	cfg, uploadsDir := testConfigWithPhotos(t)
	s := newTestServerWithPhotosConfig(t, cfg)

	relPath := filepath.Join("2026-07", "hidden.jpg")
	fullPath := filepath.Join(uploadsDir, relPath)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fullPath, []byte("fake-jpeg-bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := s.photosStore.Insert(context.Background(), photos.Photo{
		FileID: "hidden", Path: relPath, ThumbPath: relPath + "_thumb",
		Source: "gallery", CreatedAt: "2026-07-27T12:00:00Z", ClientIP: "1.1.1.1", Hidden: true,
	}); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/uploads/"+relPath, nil)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404 for a hidden photo's path", rec.Code)
	}
}

// TestUploadsServesVisiblePhotoNormally is the companion regression
// test: a photo with no hidden row (or hidden=0) must still be
// servable — the hidden-check must fail OPEN for anything not
// explicitly moderated.
func TestUploadsServesVisiblePhotoNormally(t *testing.T) {
	cfg, uploadsDir := testConfigWithPhotos(t)
	s := newTestServerWithPhotosConfig(t, cfg)

	relPath := filepath.Join("2026-07", "visible.jpg")
	fullPath := filepath.Join(uploadsDir, relPath)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fullPath, []byte("fake-jpeg-bytes"), 0o644); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/uploads/"+relPath, nil)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 for an untracked/visible file", rec.Code)
	}
}

// TestUploadsAndMediaRefuseDirectoryListings proves the bare
// http.FileServer directory-listing behavior — which used to let a
// guest browse /uploads/2026-07/ and discover an in-flight _src
// filename before its deferred cleanup ran — is gone for both mounts.
func TestUploadsAndMediaRefuseDirectoryListings(t *testing.T) {
	cfg, uploadsDir := testConfigWithPhotos(t)
	s := newTestServerWithPhotosConfig(t, cfg)

	monthDir := filepath.Join(uploadsDir, "2026-07")
	if err := os.MkdirAll(monthDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(monthDir, "somefile.jpg"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	for _, path := range []string{"/uploads/", "/uploads/2026-07/", "/media/", "/media/cats/"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		s.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Errorf("GET %s status = %d, want 404 (no directory listing)", path, rec.Code)
		}
	}
}
