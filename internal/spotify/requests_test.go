package spotify

import (
	"context"
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
