// internal/spotify/requests.go
package spotify

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

var (
	ErrTrackNameTooLong  = errors.New("track name is too long")
	ErrArtistNameTooLong = errors.New("artist name is too long")
)

// MaxTrackNameLength and MaxArtistNameLength cap the guest-supplied
// track/artist text stored in song_requests and rendered back to every
// guest on the Music page's "Recently added" list. These come from
// hidden form fields the search-results UI populates from Spotify's own
// (always reasonably short) metadata, but a request can be crafted
// directly, so the domain layer enforces the cap regardless of caller —
// same convention as guestbook.Store.Create's MaxNameLength/
// MaxMessageLength.
const (
	MaxTrackNameLength  = 200
	MaxArtistNameLength = 200
)

type SongRequest struct {
	ID          int64
	TrackURI    string
	TrackName   string
	ArtistName  string
	RequestedBy string
	CreatedAt   string
	ClientIP    string
	Status      string // "queued" | "failed"
}

// RequestStore is not a moderation queue — nothing here is approved
// before reaching Spotify. It exists so a failed queue-add has a
// debugging trail and "who requested that" is answerable.
type RequestStore struct {
	db *sql.DB
}

func NewRequestStore(db *sql.DB) *RequestStore {
	return &RequestStore{db: db}
}

func (s *RequestStore) Insert(ctx context.Context, r SongRequest) (int64, error) {
	if len(r.TrackName) > MaxTrackNameLength {
		return 0, ErrTrackNameTooLong
	}
	if len(r.ArtistName) > MaxArtistNameLength {
		return 0, ErrArtistNameTooLong
	}

	res, err := s.db.ExecContext(ctx, `
		INSERT INTO song_requests (track_uri, track_name, artist_name, requested_by, created_at, client_ip, status)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		r.TrackURI, r.TrackName, r.ArtistName, r.RequestedBy, r.CreatedAt, r.ClientIP, r.Status,
	)
	if err != nil {
		return 0, fmt.Errorf("spotify: recording song request: %w", err)
	}
	return res.LastInsertId()
}

// Recent returns the most recently successfully-queued requests, newest
// first — shown on the Music page so guests can see what's already been
// added and avoid duplicates.
func (s *RequestStore) Recent(ctx context.Context, limit int) ([]SongRequest, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, track_uri, track_name, artist_name, requested_by, created_at, client_ip, status
		FROM song_requests WHERE status = 'queued' ORDER BY created_at DESC, id DESC LIMIT ?`, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("spotify: listing recent requests: %w", err)
	}
	defer rows.Close()

	var out []SongRequest
	for rows.Next() {
		var r SongRequest
		var requestedBy sql.NullString
		if err := rows.Scan(&r.ID, &r.TrackURI, &r.TrackName, &r.ArtistName, &requestedBy, &r.CreatedAt, &r.ClientIP, &r.Status); err != nil {
			return nil, fmt.Errorf("spotify: scanning song request: %w", err)
		}
		r.RequestedBy = requestedBy.String
		out = append(out, r)
	}
	return out, rows.Err()
}
