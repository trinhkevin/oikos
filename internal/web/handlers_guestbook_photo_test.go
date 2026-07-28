// internal/web/handlers_guestbook_photo_test.go
package web

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"
)

func requireMagickForGuestbookTest(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("magick"); err != nil {
		t.Skip("magick not found on PATH — skipping guest book photo integration test")
	}
}

// realJPEGBytes returns a small but genuinely valid JPEG — the fake
// Converter seam used elsewhere in this codebase doesn't apply at the
// handler level (Server always wires the real ImageMagick-shelling
// Converter), so this test needs bytes real enough for `magick` to
// actually decode and convert, not just JPEG magic-number bytes.
func realJPEGBytes(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 20, 10))
	for y := 0; y < 10; y++ {
		for x := 0; x < 20; x++ {
			img.Set(x, y, color.RGBA{R: 200, G: 40, B: 40, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatalf("encoding fixture JPEG: %v", err)
	}
	return buf.Bytes()
}

func multipartGuestbookRequest(t *testing.T, name, message string, photo []byte, photoFilename string) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	if err := mw.WriteField("name", name); err != nil {
		t.Fatal(err)
	}
	if err := mw.WriteField("message", message); err != nil {
		t.Fatal(err)
	}
	if photo != nil {
		part, err := mw.CreateFormFile("photo", photoFilename)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := part.Write(photo); err != nil {
			t.Fatal(err)
		}
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/guestbook", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	return req
}

// TestGuestbookCreateWithPhotoLinksAndRendersThumbnail is the feature
// test for item 8: a guest book entry submitted with a real photo must
// (a) run it through the shared photos ingest pipeline (stripped JPEG +
// thumbnail on disk), (b) persist the resulting Photo.ID as the entry's
// PhotoID, and (c) render a thumbnail linking to the full image in the
// guest book entries fragment.
func TestGuestbookCreateWithPhotoLinksAndRendersThumbnail(t *testing.T) {
	requireMagickForGuestbookTest(t)

	cfg, _ := testConfigWithPhotos(t)
	s := newTestServerWithPhotosConfig(t, cfg)

	req := multipartGuestbookRequest(t, "Jamie", "Loved the cats!", realJPEGBytes(t), "cat.jpg")
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Loved the cats!") {
		t.Fatalf("body = %q, want the new entry's message", body)
	}
	if !strings.Contains(body, "/uploads/") {
		t.Errorf("body = %q, want a thumbnail linking into /uploads/", body)
	}
	if !strings.Contains(body, `alt="Guest book photo"`) {
		t.Errorf("body = %q, want the guest book photo's alt text", body)
	}

	entries, err := s.guestbookStore.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("entries = %+v, want exactly one", entries)
	}
	if entries[0].PhotoID == nil {
		t.Fatal("entries[0].PhotoID = nil, want it set to the ingested photo's id")
	}

	photo, err := s.photosStore.Get(context.Background(), *entries[0].PhotoID)
	if err != nil {
		t.Fatalf("photosStore.Get: %v", err)
	}
	if photo.Source != "guestbook" {
		t.Errorf("photo.Source = %q, want %q", photo.Source, "guestbook")
	}

	// The linked photo must also show up in the main gallery — the
	// spec says photos.Store.List doesn't filter by source, so a
	// guestbook photo is still a gallery photo.
	gallery, err := s.photosStore.List(context.Background(), 60, 0)
	if err != nil {
		t.Fatalf("photosStore.List: %v", err)
	}
	found := false
	for _, p := range gallery {
		if p.ID == photo.ID {
			found = true
		}
	}
	if !found {
		t.Error("expected the guestbook photo to also appear in the main gallery listing")
	}
}

// TestGuestbookCreateWithoutPhotoStillWorks is the regression test:
// this feature must not break the existing photo-less path.
func TestGuestbookCreateWithoutPhotoStillWorks(t *testing.T) {
	cfg, _ := testConfigWithPhotos(t)
	s := newTestServerWithPhotosConfig(t, cfg)

	req := multipartGuestbookRequest(t, "Alex", "No photo, just a note", nil, "")
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "No photo, just a note") {
		t.Fatalf("body = %q, want the new entry's message", body)
	}
	if strings.Contains(body, "/uploads/") {
		t.Errorf("body = %q, want no thumbnail for a photo-less entry", body)
	}

	entries, err := s.guestbookStore.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("entries = %+v, want exactly one", entries)
	}
	if entries[0].PhotoID != nil {
		t.Errorf("entries[0].PhotoID = %v, want nil for a photo-less submission", *entries[0].PhotoID)
	}
}

// TestGuestbookCreateWithBadPhotoSurfacesFriendlyError proves a
// rejected photo (not an image) surfaces the same friendly error
// mapping handlers_photos.go uses, retargeted to the form, rather than
// a raw store/ingest error or a silently-dropped photo.
func TestGuestbookCreateWithBadPhotoSurfacesFriendlyError(t *testing.T) {
	cfg, _ := testConfigWithPhotos(t)
	s := newTestServerWithPhotosConfig(t, cfg)

	req := multipartGuestbookRequest(t, "Jamie", "Nice try", []byte("not an image at all"), "notes.txt")
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (validation error still renders a fragment)", rec.Code)
	}
	if got := rec.Header().Get("HX-Retarget"); got != "#guestbook-form" {
		t.Errorf("HX-Retarget = %q, want #guestbook-form", got)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "isn&#39;t a photo") {
		t.Errorf("body = %q, want the shared not-an-image message", body)
	}

	entries, err := s.guestbookStore.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("entries = %+v, want nothing recorded when the photo is rejected", entries)
	}
}
