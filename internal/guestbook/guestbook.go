// internal/guestbook/guestbook.go
package guestbook

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

var (
	ErrNameRequired    = errors.New("name is required")
	ErrNameTooLong     = errors.New("name is too long")
	ErrMessageRequired = errors.New("message is required")
	ErrMessageTooLong  = errors.New("message is too long")
)

const (
	MaxNameLength    = 40
	MaxMessageLength = 500
)

type Entry struct {
	ID        int64
	Name      string
	Message   string
	PhotoID   *int64
	CreatedAt string
	Hidden    bool
	ClientIP  string
}

type Store struct {
	sqlDB *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{sqlDB: db}
}

func (s *Store) db() *sql.DB { return s.sqlDB }

func (s *Store) Create(ctx context.Context, e Entry) (int64, error) {
	if e.Name == "" {
		return 0, ErrNameRequired
	}
	if len(e.Name) > MaxNameLength {
		return 0, ErrNameTooLong
	}
	if e.Message == "" {
		return 0, ErrMessageRequired
	}
	if len(e.Message) > MaxMessageLength {
		return 0, ErrMessageTooLong
	}

	createdAt := time.Now().UTC().Format(time.RFC3339)
	res, err := s.sqlDB.ExecContext(ctx, `
		INSERT INTO guestbook_entries (name, message, photo_id, created_at, hidden, client_ip)
		VALUES (?, ?, ?, ?, 0, ?)`,
		e.Name, e.Message, e.PhotoID, createdAt, e.ClientIP,
	)
	if err != nil {
		return 0, fmt.Errorf("guestbook: inserting: %w", err)
	}
	return res.LastInsertId()
}

func (s *Store) List(ctx context.Context) ([]Entry, error) {
	rows, err := s.sqlDB.QueryContext(ctx, `
		SELECT id, name, message, photo_id, created_at, hidden, client_ip
		FROM guestbook_entries WHERE hidden = 0 ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("guestbook: listing: %w", err)
	}
	defer rows.Close()

	var out []Entry
	for rows.Next() {
		var e Entry
		var hidden int
		var photoID sql.NullInt64
		if err := rows.Scan(&e.ID, &e.Name, &e.Message, &photoID, &e.CreatedAt, &hidden, &e.ClientIP); err != nil {
			return nil, fmt.Errorf("guestbook: scanning row: %w", err)
		}
		if photoID.Valid {
			e.PhotoID = &photoID.Int64
		}
		e.Hidden = hidden != 0
		out = append(out, e)
	}
	return out, rows.Err()
}
