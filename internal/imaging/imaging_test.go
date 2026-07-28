package imaging

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func requireMagick(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("magick"); err != nil {
		t.Skip("magick not found on PATH — skipping ImageMagick integration test")
	}
}

// metadataMarker is embedded via ImageMagick's `comment` text field — a
// real, verifiably-embedded value that actually round-trips through a
// PNG-to-JPEG conversion (unlike `-set exif:GPSLatitude` on a PNG
// source, which only sets an internal property string visible to
// `identify -verbose`/`%c`-style introspection on the SOURCE file and
// is never serialized into a real EXIF profile in the first place — so
// asserting its absence from a JPEG output proves nothing about
// -strip's behavior; it's already absent with or without it. Verified
// manually before writing this test: `magick identify -format
// "%[EXIF:GPSLatitude]"` on a converted JPEG returns "unknown image
// property" in both the -strip and non--strip cases for that
// property, whereas `-set comment` genuinely survives a conversion
// without -strip and is removed by it).
const metadataMarker = "SECRET-MARKER-do-not-leak-e3f1a9"

func buildMarkedFixture(t *testing.T, ctx context.Context, dir string) string {
	t.Helper()
	src := filepath.Join(dir, "src.png")
	if err := exec.CommandContext(ctx, "magick",
		"-size", "100x50", "xc:red",
		"-set", "comment", metadataMarker,
		src,
	).Run(); err != nil {
		t.Fatalf("generating test fixture with embedded comment marker: %v", err)
	}

	// Confirm the marker is actually present on the SOURCE before
	// testing anything against it — a test that asserts the absence of
	// something that was never present proves nothing.
	out, err := exec.CommandContext(ctx, "magick", "identify", "-format", "%c", src).Output()
	if err != nil {
		t.Fatalf("identify on source fixture: %v", err)
	}
	if !strings.Contains(string(out), metadataMarker) {
		t.Fatalf("fixture setup broken: source %s has no comment marker (identify -format %%c = %q)", src, out)
	}
	return src
}

func TestConvertAndStripProducesJPEGWithoutEXIF(t *testing.T) {
	requireMagick(t)
	ctx := context.Background()
	dir := t.TempDir()

	src := buildMarkedFixture(t, ctx, dir)

	dst := filepath.Join(dir, "out.jpg")
	result, err := ConvertAndStrip(ctx, src, dst)
	if err != nil {
		t.Fatalf("ConvertAndStrip: %v", err)
	}
	if result.Width != 100 || result.Height != 50 {
		t.Errorf("Result = %+v, want 100x50", result)
	}

	out, err := exec.CommandContext(ctx, "magick", "identify", "-format", "%c", dst).Output()
	if err != nil {
		t.Fatalf("identify on output: %v", err)
	}
	if strings.Contains(string(out), metadataMarker) {
		t.Errorf("expected the comment marker to be stripped from ConvertAndStrip's output, got %q", out)
	}
}

// TestConvertAndStripNegativeControlMarkerSurvivesWithoutStrip proves
// the test above can actually fail: run the exact same conversion
// MINUS -strip and confirm the marker DOES survive. Without this, a
// future regression that silently drops -strip from ConvertAndStrip
// would not be caught by the test above if the marker happened to be
// lost for some unrelated reason (e.g. the marker mechanism itself
// stopped working) — this proves the marker mechanism is sound and the
// positive test is actually exercising -strip's behavior, not some
// other accidental metadata loss.
func TestConvertAndStripNegativeControlMarkerSurvivesWithoutStrip(t *testing.T) {
	requireMagick(t)
	ctx := context.Background()
	dir := t.TempDir()

	src := buildMarkedFixture(t, ctx, dir)

	dst := filepath.Join(dir, "out-nostrip.jpg")
	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, "magick", src+"[0]", "-auto-orient", dst) // deliberately no -strip
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("conversion without -strip: %v: %s", err, stderr.String())
	}

	out, err := exec.CommandContext(ctx, "magick", "identify", "-format", "%c", dst).Output()
	if err != nil {
		t.Fatalf("identify on output: %v", err)
	}
	if !strings.Contains(string(out), metadataMarker) {
		t.Fatalf("negative control failed: marker did not survive a conversion WITHOUT -strip (got %q) — the marker mechanism itself is broken, so the positive test above proves nothing", out)
	}
}

// TestThumbnailProducesJPEGWithoutEXIF is Thumbnail's own coverage for
// metadata stripping — before this test, only ConvertAndStrip had any
// such coverage, leaving a regression that dropped -strip from
// Thumbnail specifically undetected.
func TestThumbnailProducesJPEGWithoutEXIF(t *testing.T) {
	requireMagick(t)
	ctx := context.Background()
	dir := t.TempDir()

	src := buildMarkedFixture(t, ctx, dir)

	dst := filepath.Join(dir, "thumb.jpg")
	if err := Thumbnail(ctx, src, dst, 400); err != nil {
		t.Fatalf("Thumbnail: %v", err)
	}

	out, err := exec.CommandContext(ctx, "magick", "identify", "-format", "%c", dst).Output()
	if err != nil {
		t.Fatalf("identify on thumbnail output: %v", err)
	}
	if strings.Contains(string(out), metadataMarker) {
		t.Errorf("expected the comment marker to be stripped from Thumbnail's output, got %q", out)
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
