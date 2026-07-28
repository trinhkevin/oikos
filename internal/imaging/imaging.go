// Package imaging shells out to the ImageMagick 7 `magick` CLI to convert
// guest-uploaded photos (including HEIC/HEIF, which Go cannot decode
// natively) into metadata-stripped JPEGs, generate thumbnails, and read
// image dimensions.
package imaging

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"time"
)

type Result struct {
	Width  int
	Height int
}

const execTimeout = 20 * time.Second

// ConvertAndStrip converts src (any ImageMagick-readable format,
// including HEIC/HEIF) into a JPEG at dstPath with all metadata removed
// via -strip — this is where GPS coordinates, device identifiers, and
// timestamps embedded in guest photos are discarded before the file
// ever touches the gallery or the guest book. The `[0]` scene selector
// on the source pins the operation to the first image/frame only — a
// no-op for ordinary single-frame photos, but load-bearing for sources
// that can contain more than one image in a single file (an animated
// GIF, a multi-page TIFF, or a Portrait-mode HEIC, which commonly
// stores an auxiliary depth map alongside the primary photo in the
// same container). Without it, a multi-image source could produce
// multiple numbered output files instead of the single dstPath this
// function's contract promises.
func ConvertAndStrip(ctx context.Context, srcPath, dstPath string) (Result, error) {
	ctx, cancel := context.WithTimeout(ctx, execTimeout)
	defer cancel()

	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, "magick", srcPath+"[0]", "-auto-orient", "-strip", dstPath)
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return Result{}, fmt.Errorf("imaging: convert %s: %w: %s", srcPath, err, stderr.String())
	}
	return identify(ctx, dstPath)
}

// Thumbnail writes a resized copy of srcPath to dstPath whose longest
// edge is longEdge pixels, preserving aspect ratio and stripping
// metadata. The trailing `>` in the geometry means "shrink only" — a
// photo already smaller than longEdge is not upscaled. As in
// ConvertAndStrip, the `[0]` scene selector on the source pins the
// operation to the first image/frame only, so a multi-image source
// still produces exactly one thumbnail file.
func Thumbnail(ctx context.Context, srcPath, dstPath string, longEdge int) error {
	ctx, cancel := context.WithTimeout(ctx, execTimeout)
	defer cancel()

	geometry := fmt.Sprintf("%dx%d>", longEdge, longEdge)
	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, "magick", srcPath+"[0]", "-auto-orient", "-strip", "-resize", geometry, dstPath)
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("imaging: thumbnail %s: %w: %s", srcPath, err, stderr.String())
	}
	return nil
}

// identify reads the pixel dimensions of the first image/frame in path.
// The `[0]` scene selector prevents `-format "%w %h"` from being applied
// once per frame with no separator for multi-image sources (an animated
// GIF, multi-page TIFF, or Portrait-mode HEIC with an embedded depth
// map) — without it, the output could be several concatenated "W H"
// groups (e.g. "100 50200 60") that fmt.Sscanf would silently misparse.
func identify(ctx context.Context, path string) (Result, error) {
	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, "magick", "identify", "-format", "%w %h", path+"[0]")
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return Result{}, fmt.Errorf("imaging: identify %s: %w: %s", path, err, stderr.String())
	}
	var w, h int
	if _, err := fmt.Sscanf(string(out), "%d %d", &w, &h); err != nil {
		return Result{}, fmt.Errorf("imaging: parsing identify output %q: %w", out, err)
	}
	return Result{Width: w, Height: h}, nil
}
