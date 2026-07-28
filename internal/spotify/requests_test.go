package spotify

import (
	"context"
	"errors"
	"strings"
	"testing"

	"homesite/internal/store"
)

func TestRequestStoreInsertAndRecent(t *testing.T) {
	db, err := store.OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	rs := NewRequestStore(db)
	ctx := context.Background()

	if _, err := rs.Insert(ctx, SongRequest{
		TrackURI: "spotify:track:abc", TrackName: "Song A", ArtistName: "Artist One",
		CreatedAt: "2026-07-27T20:00:00Z", ClientIP: "192.168.1.10", Status: "queued",
	}); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	if _, err := rs.Insert(ctx, SongRequest{
		TrackURI: "spotify:track:def", TrackName: "Song B", ArtistName: "Artist Two",
		CreatedAt: "2026-07-27T20:01:00Z", ClientIP: "192.168.1.11", Status: "failed",
	}); err != nil {
		t.Fatalf("Insert (failed status): %v", err)
	}

	recent, err := rs.Recent(ctx, 10)
	if err != nil {
		t.Fatalf("Recent: %v", err)
	}
	if len(recent) != 1 || recent[0].TrackName != "Song A" {
		t.Fatalf("Recent = %+v, want only the queued entry", recent)
	}
}

// TestRequestStoreInsertRejectsOverlongFields proves the store enforces a
// length cap on guest-supplied track/artist text — following the same
// domain-layer-validates convention as guestbook.Store.Create — since
// these hidden-form-field values are rendered unescaped-length to every
// other guest on the Music page's "Recently added" list.
func TestRequestStoreInsertRejectsOverlongFields(t *testing.T) {
	db, err := store.OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	rs := NewRequestStore(db)
	ctx := context.Background()

	longName := strings.Repeat("x", MaxTrackNameLength+1)
	if _, err := rs.Insert(ctx, SongRequest{
		TrackURI: "spotify:track:abc", TrackName: longName, ArtistName: "Artist",
		CreatedAt: "2026-07-27T20:00:00Z", ClientIP: "192.168.1.10", Status: "queued",
	}); !errors.Is(err, ErrTrackNameTooLong) {
		t.Fatalf("err = %v, want ErrTrackNameTooLong", err)
	}

	longArtist := strings.Repeat("y", MaxArtistNameLength+1)
	if _, err := rs.Insert(ctx, SongRequest{
		TrackURI: "spotify:track:abc", TrackName: "Song", ArtistName: longArtist,
		CreatedAt: "2026-07-27T20:00:00Z", ClientIP: "192.168.1.10", Status: "queued",
	}); !errors.Is(err, ErrArtistNameTooLong) {
		t.Fatalf("err = %v, want ErrArtistNameTooLong", err)
	}

	recent, err := rs.Recent(ctx, 10)
	if err != nil {
		t.Fatalf("Recent: %v", err)
	}
	if len(recent) != 0 {
		t.Errorf("Recent = %+v, want no rows — both inserts should have been rejected", recent)
	}
}
