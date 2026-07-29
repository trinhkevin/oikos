package photos

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"path/filepath"
)

type Photo struct {
	ID        int64
	FileID    string
	Path      string
	ThumbPath string
	ByteSize  int64
	Width     int
	Height    int
	Source    string // "gallery" | "guestbook"
	Caption   string
	CreatedAt string
	Hidden    bool
	ClientIP  string
}

type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

func (s *Store) Insert(ctx context.Context, p Photo) (int64, error) {
	res, err := s.db.ExecContext(ctx, `
		INSERT INTO photos (file_id, path, thumb_path, byte_size, width, height, source, caption, created_at, hidden, client_ip)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.FileID, p.Path, p.ThumbPath, p.ByteSize, p.Width, p.Height, p.Source, p.Caption, p.CreatedAt, boolToInt(p.Hidden), p.ClientIP,
	)
	if err != nil {
		return 0, fmt.Errorf("photos: inserting: %w", err)
	}
	return res.LastInsertId()
}

func (s *Store) List(ctx context.Context, limit, offset int) ([]Photo, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, file_id, path, thumb_path, byte_size, width, height, source, caption, created_at, hidden, client_ip
		FROM photos WHERE hidden = 0 ORDER BY created_at DESC LIMIT ? OFFSET ?`,
		limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("photos: listing: %w", err)
	}
	defer rows.Close()

	var out []Photo
	for rows.Next() {
		var p Photo
		var hidden int
		var caption sql.NullString
		if err := rows.Scan(&p.ID, &p.FileID, &p.Path, &p.ThumbPath, &p.ByteSize, &p.Width, &p.Height, &p.Source, &caption, &p.CreatedAt, &hidden, &p.ClientIP); err != nil {
			return nil, fmt.Errorf("photos: scanning row: %w", err)
		}
		p.Caption = caption.String
		p.Hidden = hidden != 0
		out = append(out, p)
	}
	return out, rows.Err()
}

// DiskUsageBytes sums the size of every regular file under dir. It backs
// the 80GB upload cap — checked before every ingest — and the /healthz
// disk-usage figure (Task 18).
func (s *Store) DiskUsageBytes(dir string) (int64, error) {
	var total int64
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		total += info.Size()
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("photos: computing disk usage of %s: %w", dir, err)
	}
	return total, nil
}

// Get fetches a single photo by id — used to enrich a guest book entry
// with its linked photo's path/thumb_path for rendering, since
// guestbook.Store.List only returns the bare PhotoID.
func (s *Store) Get(ctx context.Context, id int64) (Photo, error) {
	var p Photo
	var hidden int
	var caption sql.NullString
	err := s.db.QueryRowContext(ctx, `
		SELECT id, file_id, path, thumb_path, byte_size, width, height, source, caption, created_at, hidden, client_ip
		FROM photos WHERE id = ?`, id,
	).Scan(&p.ID, &p.FileID, &p.Path, &p.ThumbPath, &p.ByteSize, &p.Width, &p.Height, &p.Source, &caption, &p.CreatedAt, &hidden, &p.ClientIP)
	if err != nil {
		return Photo{}, fmt.Errorf("photos: getting %d: %w", id, err)
	}
	p.Caption = caption.String
	p.Hidden = hidden != 0
	return p, nil
}

// ListForModeration returns every photo (hidden or not), newest first,
// for the admin panel — unlike List, which only ever shows guests what's
// currently visible.
func (s *Store) ListForModeration(ctx context.Context) ([]Photo, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, file_id, path, thumb_path, byte_size, width, height, source, caption, created_at, hidden, client_ip
		FROM photos ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("photos: listing for moderation: %w", err)
	}
	defer rows.Close()

	var out []Photo
	for rows.Next() {
		var p Photo
		var hidden int
		var caption sql.NullString
		if err := rows.Scan(&p.ID, &p.FileID, &p.Path, &p.ThumbPath, &p.ByteSize, &p.Width, &p.Height, &p.Source, &caption, &p.CreatedAt, &hidden, &p.ClientIP); err != nil {
			return nil, fmt.Errorf("photos: scanning row: %w", err)
		}
		p.Caption = caption.String
		p.Hidden = hidden != 0
		out = append(out, p)
	}
	return out, rows.Err()
}

// SetHidden archives (hidden=true) or restores (hidden=false) a photo.
// An archived photo is excluded from List and blocked from direct
// /uploads/ access (see static_files.go's IsHidden check) without
// deleting anything — the moderation equivalent of a soft delete.
func (s *Store) SetHidden(ctx context.Context, id int64, hidden bool) error {
	res, err := s.db.ExecContext(ctx, `UPDATE photos SET hidden = ? WHERE id = ?`, boolToInt(hidden), id)
	if err != nil {
		return fmt.Errorf("photos: setting hidden=%v for %d: %w", hidden, id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("photos: checking rows affected for %d: %w", id, err)
	}
	if n == 0 {
		return fmt.Errorf("photos: no photo with id %d", id)
	}
	return nil
}

// Delete permanently removes a photo's row and returns it so the caller
// can also remove its files from disk — the store package doesn't know
// the uploads directory root, so file cleanup is the caller's job.
func (s *Store) Delete(ctx context.Context, id int64) (Photo, error) {
	p, err := s.Get(ctx, id)
	if err != nil {
		return Photo{}, err
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM photos WHERE id = ?`, id); err != nil {
		return Photo{}, fmt.Errorf("photos: deleting %d: %w", id, err)
	}
	return p, nil
}

// IsHidden reports whether relPath (or its thumbnail counterpart)
// belongs to a photo explicitly marked hidden via moderation. A path
// with no matching row is NOT considered hidden — fail open for
// untracked files, fail closed only for explicitly moderated ones.
func (s *Store) IsHidden(ctx context.Context, relPath string) (bool, error) {
	var hidden int
	err := s.db.QueryRowContext(ctx,
		`SELECT hidden FROM photos WHERE path = ? OR thumb_path = ?`,
		relPath, relPath,
	).Scan(&hidden)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("photos: checking hidden status for %s: %w", relPath, err)
	}
	return hidden != 0, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
