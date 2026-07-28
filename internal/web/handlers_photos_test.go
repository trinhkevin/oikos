package web

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"homesite/internal/config"
	"homesite/internal/photos"
	"homesite/internal/store"
)

func testConfigWithPhotos(t *testing.T) (*config.Config, string) {
	t.Helper()
	uploadsDir := t.TempDir()
	cfg := testConfig()
	cfg.UploadsDir = uploadsDir
	cfg.Photos = config.PhotosConfig{
		MaxFileBytes: 1024 * 1024, MaxTotalBytes: 100 * 1024 * 1024, ThumbLongEdge: 400,
	}
	return cfg, uploadsDir
}

// newTestServerWithPhotos builds a Server via the current single-argument
// New(cfg) and then wires photosStore/photosIngester directly. New doesn't
// open a database yet (that requires a *sql.DB, threaded through starting
// in Task 15), so tests construct the store/ingester themselves against an
// in-memory database — this is a deliberate, temporary seam documented in
// the Task 14 brief's Step 5.
func newTestServerWithPhotos(t *testing.T) *Server {
	t.Helper()
	cfg, _ := testConfigWithPhotos(t)
	return newTestServerWithPhotosConfig(t, cfg)
}

func newTestServerWithPhotosConfig(t *testing.T, cfg *config.Config) *Server {
	t.Helper()
	db, err := store.OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	s := New(cfg)
	s.photosStore = photos.NewStore(db)
	s.photosIngester = photos.NewIngester(s.photosStore, cfg.Photos, cfg.UploadsDir)
	return s
}

func multipartJPEGRequest(t *testing.T, filename string, body []byte) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	part, err := mw.CreateFormFile("photos", filename)
	if err != nil {
		t.Fatal(err)
	}
	part.Write(body)
	mw.Close()

	req := httptest.NewRequest(http.MethodPost, "/photos", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	return req
}

func TestPhotosPageRendersEmptyGallery(t *testing.T) {
	s := newTestServerWithPhotos(t)
	req := httptest.NewRequest(http.MethodGet, "/photos", nil)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Upload Photos") {
		t.Error("expected page heading")
	}
}

func TestPhotosUploadRejectsNonImage(t *testing.T) {
	// magick isn't invoked on a rejected file, so this test needs no
	// real ImageMagick install — Sniff rejects it before Ingest ever
	// calls the converter.
	s := newTestServerWithPhotos(t)
	req := multipartJPEGRequest(t, "notes.txt", []byte("just some text"))
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (fragment renders even on per-file failure)", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "0 of 1 uploaded") {
		t.Errorf("body = %q, want a 0-of-1 result", body)
	}
	// templ HTML-escapes text nodes, so the apostrophe in "isn't" renders
	// as "&#39;" — match the escaped form actually produced.
	if !strings.Contains(body, "isn&#39;t a photo") {
		t.Errorf("body = %q, want the not-an-image message", body)
	}
}

func TestPhotosPageShowsDiskWarningPastThreshold(t *testing.T) {
	cfg, uploadsDir := testConfigWithPhotos(t)
	cfg.Photos.MaxTotalBytes = 1000
	cfg.Photos.WarnAtPercent = 85
	writeFileForTest(t, uploadsDir+"/existing.dat", make([]byte, 900)) // 90% full

	s := newTestServerWithPhotosConfig(t, cfg)

	req := httptest.NewRequest(http.MethodGet, "/photos", nil)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)

	if !strings.Contains(rec.Body.String(), "almost full") {
		t.Error("expected the disk warning banner at 90% usage with an 85% threshold")
	}
}

func TestPhotosPageHidesDiskWarningBelowThreshold(t *testing.T) {
	cfg, uploadsDir := testConfigWithPhotos(t)
	cfg.Photos.MaxTotalBytes = 1000
	cfg.Photos.WarnAtPercent = 85
	writeFileForTest(t, uploadsDir+"/existing.dat", make([]byte, 100)) // 10% full

	s := newTestServerWithPhotosConfig(t, cfg)

	req := httptest.NewRequest(http.MethodGet, "/photos", nil)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)

	if strings.Contains(rec.Body.String(), "almost full") {
		t.Error("did not expect the disk warning banner at 10% usage with an 85% threshold")
	}
}

func writeFileForTest(t *testing.T, path string, b []byte) {
	t.Helper()
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
}
