package imaging

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"testing"
)

func requireMagick(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("magick"); err != nil {
		t.Skip("magick not found on PATH — skipping ImageMagick integration test")
	}
}

func TestConvertAndStripProducesJPEGWithoutEXIF(t *testing.T) {
	requireMagick(t)
	ctx := context.Background()
	dir := t.TempDir()

	src := filepath.Join(dir, "src.png")
	if err := exec.CommandContext(ctx, "magick",
		"-size", "100x50", "xc:red",
		"-set", "exif:GPSLatitude", "37/1,46/1,2540/100",
		src,
	).Run(); err != nil {
		t.Fatalf("generating test fixture with fake GPS EXIF: %v", err)
	}

	dst := filepath.Join(dir, "out.jpg")
	result, err := ConvertAndStrip(ctx, src, dst)
	if err != nil {
		t.Fatalf("ConvertAndStrip: %v", err)
	}
	if result.Width != 100 || result.Height != 50 {
		t.Errorf("Result = %+v, want 100x50", result)
	}

	out, err := exec.CommandContext(ctx, "magick", "identify", "-format", "%[EXIF:GPSLatitude]", dst).Output()
	if err != nil {
		t.Fatalf("identify on output: %v", err)
	}
	if len(out) != 0 {
		t.Errorf("expected no GPS EXIF in output, got %q", out)
	}
}

func TestThumbnailResizesToLongEdge(t *testing.T) {
	requireMagick(t)
	ctx := context.Background()
	dir := t.TempDir()

	src := filepath.Join(dir, "src.jpg")
	if err := exec.CommandContext(ctx, "magick", "-size", "1200x600", "xc:blue", src).Run(); err != nil {
		t.Fatalf("generating test fixture: %v", err)
	}

	dst := filepath.Join(dir, "thumb.jpg")
	if err := Thumbnail(ctx, src, dst, 400); err != nil {
		t.Fatalf("Thumbnail: %v", err)
	}

	out, err := exec.CommandContext(ctx, "magick", "identify", "-format", "%w %h", dst).Output()
	if err != nil {
		t.Fatalf("identify on thumbnail: %v", err)
	}
	var w, h int
	if _, err := fmt.Sscanf(string(out), "%d %d", &w, &h); err != nil {
		t.Fatalf("parsing identify output %q: %v", out, err)
	}
	if w != 400 || h != 200 {
		t.Errorf("thumbnail = %dx%d, want 400x200 (long edge 400, 2:1 aspect preserved)", w, h)
	}
}
