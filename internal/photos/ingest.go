package photos

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"homesite/internal/config"
	"homesite/internal/imaging"
)

var (
	ErrNotAnImage = errors.New("not a recognized image type")
	ErrTooLarge   = errors.New("file exceeds maximum size")
	ErrDiskFull   = errors.New("upload storage is full")
	ErrProcessing = errors.New("image processing failed")
)

// Converter is the seam between Ingester and ImageMagick, so tests can
// substitute a fake and exercise the pipeline's decision logic without
// requiring `magick` to be installed. internal/imaging's own tests cover
// the real binary.
type Converter interface {
	ConvertAndStrip(ctx context.Context, srcPath, dstPath string) (imaging.Result, error)
	Thumbnail(ctx context.Context, srcPath, dstPath string, longEdge int) error
}

type realConverter struct{}

func (realConverter) ConvertAndStrip(ctx context.Context, src, dst string) (imaging.Result, error) {
	return imaging.ConvertAndStrip(ctx, src, dst)
}
func (realConverter) Thumbnail(ctx context.Context, src, dst string, longEdge int) error {
	return imaging.Thumbnail(ctx, src, dst, longEdge)
}

type Ingester struct {
	store      *Store
	cfg        config.PhotosConfig
	uploadsDir string
	conv       Converter
}

func NewIngester(store *Store, cfg config.PhotosConfig, uploadsDir string) *Ingester {
	return &Ingester{store: store, cfg: cfg, uploadsDir: uploadsDir, conv: realConverter{}}
}

// Ingest reads r fully (capped at cfg.MaxFileBytes+1 so an oversized
// upload can't exhaust memory), sniffs its real type, checks the disk
// cap, converts it to a metadata-stripped JPEG plus thumbnail, and
// records it. source is "gallery" or "guestbook".
func (ing *Ingester) Ingest(ctx context.Context, r io.Reader, source, clientIP string) (Photo, error) {
	buf, err := io.ReadAll(io.LimitReader(r, ing.cfg.MaxFileBytes+1))
	if err != nil {
		return Photo{}, fmt.Errorf("photos: reading upload: %w", err)
	}

	// Sniff before the size check: a huge non-image upload should report
	// "not an image," not "too large" — the size limit is a check on
	// otherwise-legitimate images, not a substitute for sniffing.
	sniffLen := min(len(buf), 512)
	mimeType, ok := Sniff(buf[:sniffLen])
	if !ok {
		return Photo{}, fmt.Errorf("photos: unrecognized file type: %w", ErrNotAnImage)
	}
	if int64(len(buf)) > ing.cfg.MaxFileBytes {
		return Photo{}, fmt.Errorf("photos: exceeds %d bytes: %w", ing.cfg.MaxFileBytes, ErrTooLarge)
	}

	// Size before the disk-cap walk: no reason to walk the whole uploads
	// tree for a file that's already rejected.
	used, err := ing.store.DiskUsageBytes(ing.uploadsDir)
	if err != nil {
		return Photo{}, fmt.Errorf("photos: checking disk usage: %w", err)
	}
	if used+int64(len(buf)) > ing.cfg.MaxTotalBytes {
		return Photo{}, fmt.Errorf("photos: at capacity: %w", ErrDiskFull)
	}

	fileID, err := newFileID()
	if err != nil {
		return Photo{}, fmt.Errorf("photos: generating file id: %w", err)
	}

	monthDir := time.Now().UTC().Format("2006-01")
	destDir := filepath.Join(ing.uploadsDir, monthDir)
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return Photo{}, fmt.Errorf("photos: creating upload dir: %w", err)
	}

	// The raw guest upload (full EXIF, full GPS) is written to a temp
	// directory OUTSIDE the served uploads tree, never inside destDir —
	// destDir is reachable via /uploads/ and must only ever contain the
	// already-stripped final output and thumbnail. defer os.RemoveAll
	// covers both the normal-completion cleanup and the crash/SIGKILL
	// case: a process death just leaves this under the OS temp dir
	// (isolated by systemd's PrivateTmp=true in production), never inside
	// the public tree.
	tmpDir, err := os.MkdirTemp("", "homesite-ingest-")
	if err != nil {
		return Photo{}, fmt.Errorf("photos: creating temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	tmpSrc := filepath.Join(tmpDir, fileID+"_src"+extensionFor(mimeType))
	if err := os.WriteFile(tmpSrc, buf, 0o644); err != nil {
		return Photo{}, fmt.Errorf("photos: writing temp source: %w", err)
	}

	relPath := filepath.Join(monthDir, fileID+".jpg")
	relThumb := filepath.Join(monthDir, fileID+"_thumb.jpg")
	finalPath := filepath.Join(ing.uploadsDir, relPath)
	thumbPath := filepath.Join(ing.uploadsDir, relThumb)

	result, err := ing.conv.ConvertAndStrip(ctx, tmpSrc, finalPath)
	if err != nil {
		return Photo{}, fmt.Errorf("photos: converting: %w: %w", err, ErrProcessing)
	}
	if err := ing.conv.Thumbnail(ctx, tmpSrc, thumbPath, ing.cfg.ThumbLongEdge); err != nil {
		os.Remove(finalPath)
		return Photo{}, fmt.Errorf("photos: thumbnailing: %w: %w", err, ErrProcessing)
	}

	finalInfo, err := os.Stat(finalPath)
	if err != nil {
		os.Remove(finalPath)
		os.Remove(thumbPath)
		return Photo{}, fmt.Errorf("photos: stat final image: %w", err)
	}

	photo := Photo{
		FileID: fileID, Path: relPath, ThumbPath: relThumb,
		ByteSize: finalInfo.Size(), Width: result.Width, Height: result.Height,
		Source: source, CreatedAt: time.Now().UTC().Format(time.RFC3339), ClientIP: clientIP,
	}
	id, err := ing.store.Insert(ctx, photo)
	if err != nil {
		os.Remove(finalPath)
		os.Remove(thumbPath)
		return Photo{}, fmt.Errorf("photos: recording metadata: %w", err)
	}
	photo.ID = id
	return photo, nil
}

func newFileID() (string, error) {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return time.Now().UTC().Format("20060102T150405") + "-" + hex.EncodeToString(b[:]), nil
}

func extensionFor(mimeType string) string {
	switch mimeType {
	case "image/jpeg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/webp":
		return ".webp"
	case "image/heic":
		return ".heic"
	default:
		return ".bin"
	}
}
