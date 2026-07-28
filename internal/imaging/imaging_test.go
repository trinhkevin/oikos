package imaging

import (
	"context"
	"fmt"
	"os"
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

// TestMultiFrameSourceUsesFirstFrameOnly guards against a class of bug where
// ImageMagick's `-format` string is applied once per frame/image with no
// separator inserted between them. For a multi-frame/multi-image source
// (an animated GIF, a multi-page TIFF, or — the realistic case for guest
// photo uploads — a Portrait-mode HEIC that stores an auxiliary depth map
// alongside the primary photo), naive parsing of `identify`'s output can
// silently misparse, and a convert command with no frame selector can
// write multiple numbered output files instead of a single dstPath.
//
// This test builds a synthetic 2-frame GIF whose frames have different
// sizes (100x50, then 200x100) and confirms both ConvertAndStrip and the
// underlying identify logic report frame 0's dimensions only, and that
// exactly one output file is produced.
func TestMultiFrameSourceUsesFirstFrameOnly(t *testing.T) {
	requireMagick(t)
	ctx := context.Background()
	dir := t.TempDir()

	src := filepath.Join(dir, "multiframe.gif")
	if err := exec.CommandContext(ctx, "magick",
		"-delay", "10",
		"-size", "100x50", "xc:red",
		"-size", "200x100", "xc:blue",
		src,
	).Run(); err != nil {
		t.Fatalf("generating multi-frame test fixture: %v", err)
	}

	dst := filepath.Join(dir, "out.jpg")
	result, err := ConvertAndStrip(ctx, src, dst)
	if err != nil {
		t.Fatalf("ConvertAndStrip: %v", err)
	}
	if result.Width != 100 || result.Height != 50 {
		t.Errorf("Result = %+v, want 100x50 (frame 0's dimensions, not frame 1's 200x100 or a misparse of both concatenated)", result)
	}

	// Confirm exactly one output file was written — a frame selector
	// omission would cause ImageMagick to write "out-0.jpg", "out-1.jpg"
	// instead of the single dst path this function's contract promises.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading temp dir: %v", err)
	}
	var outputFiles []string
	for _, e := range entries {
		if e.Name() != "multiframe.gif" {
			outputFiles = append(outputFiles, e.Name())
		}
	}
	if len(outputFiles) != 1 || outputFiles[0] != "out.jpg" {
		t.Errorf("expected exactly one output file %q, got %v", "out.jpg", outputFiles)
	}

	if _, err := os.Stat(dst); err != nil {
		t.Errorf("expected %s to exist: %v", dst, err)
	}
}
