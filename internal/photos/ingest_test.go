package photos

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"homesite/internal/config"
	"homesite/internal/imaging"
)

type fakeConverter struct {
	convertCalled, thumbnailCalled bool
}

func (f *fakeConverter) ConvertAndStrip(ctx context.Context, src, dst string) (imaging.Result, error) {
	f.convertCalled = true
	if err := os.WriteFile(dst, []byte("fake-jpeg-bytes"), 0o644); err != nil {
		return imaging.Result{}, err
	}
	return imaging.Result{Width: 800, Height: 600}, nil
}

func (f *fakeConverter) Thumbnail(ctx context.Context, src, dst string, longEdge int) error {
	f.thumbnailCalled = true
	return os.WriteFile(dst, []byte("fake-thumb-bytes"), 0o644)
}

func testPhotosConfig() config.PhotosConfig {
	return config.PhotosConfig{
		MaxFileBytes:  1024 * 1024,
		MaxTotalBytes: 10 * 1024 * 1024,
		ThumbLongEdge: 400,
	}
}

func TestIngestSucceedsForValidJPEG(t *testing.T) {
	uploadsDir := t.TempDir()
	conv := &fakeConverter{}
	ing := &Ingester{store: newTestStore(t), cfg: testPhotosConfig(), uploadsDir: uploadsDir, conv: conv}

	jpeg := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 'J', 'F', 'I', 'F', 0, 1, 2, 3, 4, 5}
	photo, err := ing.Ingest(context.Background(), bytes.NewReader(jpeg), "gallery", "192.168.1.20")
	if err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	if !conv.convertCalled || !conv.thumbnailCalled {
		t.Error("expected both ConvertAndStrip and Thumbnail to be called")
	}
	if photo.Width != 800 || photo.Height != 600 {
		t.Errorf("photo dims = %dx%d, want 800x600", photo.Width, photo.Height)
	}
	if _, err := os.Stat(filepath.Join(uploadsDir, photo.Path)); err != nil {
		t.Errorf("expected final image on disk: %v", err)
	}
	if _, err := os.Stat(filepath.Join(uploadsDir, photo.ThumbPath)); err != nil {
		t.Errorf("expected thumbnail on disk: %v", err)
	}
}

func TestIngestRejectsNonImage(t *testing.T) {
	uploadsDir := t.TempDir()
	ing := &Ingester{store: newTestStore(t), cfg: testPhotosConfig(), uploadsDir: uploadsDir, conv: &fakeConverter{}}

	_, err := ing.Ingest(context.Background(), bytes.NewReader([]byte("not an image")), "gallery", "192.168.1.20")
	if !errors.Is(err, ErrNotAnImage) {
		t.Fatalf("err = %v, want ErrNotAnImage", err)
	}
}

func TestIngestRejectsHugeNonImageAsNotAnImageNotTooLarge(t *testing.T) {
	// The brief is explicit: sniff runs before the size check, so a huge
	// non-image upload reports "not an image," never "too large" — the
	// size limit is about image files, not an excuse to skip sniffing.
	uploadsDir := t.TempDir()
	cfg := testPhotosConfig()
	cfg.MaxFileBytes = 8 // tiny, so the huge non-image definitely exceeds it too
	ing := &Ingester{store: newTestStore(t), cfg: cfg, uploadsDir: uploadsDir, conv: &fakeConverter{}}

	hugeNonImage := bytes.Repeat([]byte("not an image at all, just text"), 1000)
	_, err := ing.Ingest(context.Background(), bytes.NewReader(hugeNonImage), "gallery", "192.168.1.20")
	if !errors.Is(err, ErrNotAnImage) {
		t.Fatalf("err = %v, want ErrNotAnImage (sniff must run before the size check)", err)
	}
}

func TestIngestRejectsOversizedFile(t *testing.T) {
	uploadsDir := t.TempDir()
	cfg := testPhotosConfig()
	cfg.MaxFileBytes = 8 // tiny, to trigger the limit deterministically
	ing := &Ingester{store: newTestStore(t), cfg: cfg, uploadsDir: uploadsDir, conv: &fakeConverter{}}

	jpeg := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 'J', 'F', 'I', 'F', 0, 1, 2, 3, 4, 5}
	_, err := ing.Ingest(context.Background(), bytes.NewReader(jpeg), "gallery", "192.168.1.20")
	if !errors.Is(err, ErrTooLarge) {
		t.Fatalf("err = %v, want ErrTooLarge", err)
	}
}

func TestIngestRejectsWhenDiskCapReached(t *testing.T) {
	uploadsDir := t.TempDir()
	writeFile(t, filepath.Join(uploadsDir, "existing.dat"), make([]byte, 1000))

	cfg := testPhotosConfig()
	cfg.MaxTotalBytes = 500 // already exceeded by the existing file above
	ing := &Ingester{store: newTestStore(t), cfg: cfg, uploadsDir: uploadsDir, conv: &fakeConverter{}}

	jpeg := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 'J', 'F', 'I', 'F', 0, 1, 2, 3, 4, 5}
	_, err := ing.Ingest(context.Background(), bytes.NewReader(jpeg), "gallery", "192.168.1.20")
	if !errors.Is(err, ErrDiskFull) {
		t.Fatalf("err = %v, want ErrDiskFull", err)
	}
}

func TestIngestWrapsConverterFailure(t *testing.T) {
	uploadsDir := t.TempDir()
	failing := failingConverter{}
	ing := &Ingester{store: newTestStore(t), cfg: testPhotosConfig(), uploadsDir: uploadsDir, conv: failing}

	jpeg := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 'J', 'F', 'I', 'F', 0, 1, 2, 3, 4, 5}
	_, err := ing.Ingest(context.Background(), bytes.NewReader(jpeg), "gallery", "192.168.1.20")
	if !errors.Is(err, ErrProcessing) {
		t.Fatalf("err = %v, want ErrProcessing", err)
	}
}

// pathInspectingConverter records the srcPath it's called with, so the
// test can assert it — the regression coverage for the security fix
// that relocated the temp source file outside the served uploads tree
// (it used to live inside destDir, browsable via /uploads/ until the
// deferred cleanup ran).
type pathInspectingConverter struct {
	convertSrcPath, thumbnailSrcPath string
}

func (f *pathInspectingConverter) ConvertAndStrip(ctx context.Context, src, dst string) (imaging.Result, error) {
	f.convertSrcPath = src
	if err := os.WriteFile(dst, []byte("fake-jpeg-bytes"), 0o644); err != nil {
		return imaging.Result{}, err
	}
	return imaging.Result{Width: 800, Height: 600}, nil
}

func (f *pathInspectingConverter) Thumbnail(ctx context.Context, src, dst string, longEdge int) error {
	f.thumbnailSrcPath = src
	return os.WriteFile(dst, []byte("fake-thumb-bytes"), 0o644)
}

func TestIngestWritesTempSourceOutsideUploadsDir(t *testing.T) {
	uploadsDir := t.TempDir()
	conv := &pathInspectingConverter{}
	ing := &Ingester{store: newTestStore(t), cfg: testPhotosConfig(), uploadsDir: uploadsDir, conv: conv}

	jpeg := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 'J', 'F', 'I', 'F', 0, 1, 2, 3, 4, 5}
	if _, err := ing.Ingest(context.Background(), bytes.NewReader(jpeg), "gallery", "192.168.1.20"); err != nil {
		t.Fatalf("Ingest: %v", err)
	}

	absUploadsDir, err := filepath.Abs(uploadsDir)
	if err != nil {
		t.Fatal(err)
	}

	for name, path := range map[string]string{
		"ConvertAndStrip's srcPath": conv.convertSrcPath,
		"Thumbnail's srcPath":       conv.thumbnailSrcPath,
	} {
		if path == "" {
			t.Fatalf("%s was never recorded — converter not called?", name)
		}
		absPath, err := filepath.Abs(path)
		if err != nil {
			t.Fatal(err)
		}
		rel, err := filepath.Rel(absUploadsDir, absPath)
		if err != nil {
			t.Fatal(err)
		}
		// filepath.Rel returns a path starting with ".." (or being an
		// absolute path on some platforms) whenever absPath falls
		// outside absUploadsDir. Anything else means it's inside —
		// exactly the bug this test guards against.
		if rel == "." || (!strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != "..") {
			t.Errorf("%s = %q, is INSIDE uploadsDir %q (rel=%q) — the raw guest upload must never be reachable via /uploads/", name, path, uploadsDir, rel)
		}
	}
}

type failingConverter struct{}

func (failingConverter) ConvertAndStrip(ctx context.Context, src, dst string) (imaging.Result, error) {
	return imaging.Result{}, errors.New("boom")
}
func (failingConverter) Thumbnail(ctx context.Context, src, dst string, longEdge int) error {
	return errors.New("boom")
}
