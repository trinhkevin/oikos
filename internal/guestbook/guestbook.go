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

// ListForModeration returns every entry (hidden or not), newest first,
// for the admin panel.
func (s *Store) ListForModeration(ctx context.Context) ([]Entry, error) {
	rows, err := s.sqlDB.QueryContext(ctx, `
		SELECT id, name, message, photo_id, created_at, hidden, client_ip
		FROM guestbook_entries ORDER BY created_at DESC, id DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("guestbook: listing for moderation: %w", err)
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

// SetHidden archives (hidden=true) or restores (hidden=false) an entry.
func (s *Store) SetHidden(ctx context.Context, id int64, hidden bool) error {
	res, err := s.sqlDB.ExecContext(ctx, `UPDATE guestbook_entries SET hidden = ? WHERE id = ?`, boolToInt(hidden), id)
	if err != nil {
		return fmt.Errorf("guestbook: setting hidden=%v for %d: %w", hidden, id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("guestbook: checking rows affected for %d: %w", id, err)
	}
	if n == 0 {
		return fmt.Errorf("guestbook: no entry with id %d", id)
	}
	return nil
}

// Delete permanently removes an entry.
func (s *Store) Delete(ctx context.Context, id int64) error {
	if _, err := s.sqlDB.ExecContext(ctx, `DELETE FROM guestbook_entries WHERE id = ?`, id); err != nil {
		return fmt.Errorf("guestbook: deleting %d: %w", id, err)
	}
	return nil
}

// ClearPhotoID unlinks any entry pointing at photoID, e.g. right before
// that photo is permanently deleted — photo_id has an enforced foreign
// key (see internal/store's foreign_keys(1) pragma), so deleting a
// still-referenced photo would otherwise fail outright. The guest's
// name/message stand on their own either way.
func (s *Store) ClearPhotoID(ctx context.Context, photoID int64) error {
	if _, err := s.sqlDB.ExecContext(ctx, `UPDATE guestbook_entries SET photo_id = NULL WHERE photo_id = ?`, photoID); err != nil {
		return fmt.Errorf("guestbook: clearing photo_id %d: %w", photoID, err)
	}
	return nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// Count returns the number of non-hidden entries -- used by the Welcome
// page's live "party pulse" line, where a full List would fetch every
// column just to discard it for a number.
func (s *Store) Count(ctx context.Context) (int, error) {
	var count int
	err := s.sqlDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM guestbook_entries WHERE hidden = 0`).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("guestbook: counting: %w", err)
	}
	return count, nil
}

func (s *Store) List(ctx context.Context) ([]Entry, error) {
	rows, err := s.sqlDB.QueryContext(ctx, `
		SELECT id, name, message, photo_id, created_at, hidden, client_ip
		FROM guestbook_entries WHERE hidden = 0 ORDER BY created_at DESC, id DESC`,
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
