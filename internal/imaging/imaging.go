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
// ever touches the gallery or the guest book.
func ConvertAndStrip(ctx context.Context, srcPath, dstPath string) (Result, error) {
	ctx, cancel := context.WithTimeout(ctx, execTimeout)
	defer cancel()

	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, "magick", srcPath, "-auto-orient", "-strip", dstPath)
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return Result{}, fmt.Errorf("imaging: convert %s: %w: %s", srcPath, err, stderr.String())
	}
	return identify(ctx, dstPath)
}

// Thumbnail writes a resized copy of srcPath to dstPath whose longest
// edge is longEdge pixels, preserving aspect ratio and stripping
// metadata. The trailing `>` in the geometry means "shrink only" — a
// photo already smaller than longEdge is not upscaled.
func Thumbnail(ctx context.Context, srcPath, dstPath string, longEdge int) error {
	ctx, cancel := context.WithTimeout(ctx, execTimeout)
	defer cancel()

	geometry := fmt.Sprintf("%dx%d>", longEdge, longEdge)
	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, "magick", srcPath, "-auto-orient", "-strip", "-resize", geometry, dstPath)
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("imaging: thumbnail %s: %w: %s", srcPath, err, stderr.String())
	}
	return nil
}

func identify(ctx context.Context, path string) (Result, error) {
	out, err := exec.CommandContext(ctx, "magick", "identify", "-format", "%w %h", path).Output()
	if err != nil {
		return Result{}, fmt.Errorf("imaging: identify %s: %w", path, err)
	}
	var w, h int
	if _, err := fmt.Sscanf(string(out), "%d %d", &w, &h); err != nil {
		return Result{}, fmt.Errorf("imaging: parsing identify output %q: %w", out, err)
	}
	return Result{Width: w, Height: h}, nil
}
