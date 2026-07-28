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

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
