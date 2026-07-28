package spotify

import (
	"context"
	"testing"

	"homesite/internal/store"
)

func TestSQLTokenStoreRoundTrips(t *testing.T) {
	db, err := store.OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ts := NewSQLTokenStore(db)
	ctx := context.Background()

	got, err := ts.LoadRefreshToken(ctx)
	if err != nil {
		t.Fatalf("LoadRefreshToken (empty): %v", err)
	}
	if got != "" {
		t.Errorf("LoadRefreshToken on empty table = %q, want empty string", got)
	}

	if err := ts.SaveTokens(ctx, "refresh-abc", timeNow()); err != nil {
		t.Fatalf("SaveTokens: %v", err)
	}
	got, err = ts.LoadRefreshToken(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got != "refresh-abc" {
		t.Errorf("LoadRefreshToken = %q, want refresh-abc", got)
	}

	// Saving again must update, not duplicate — the table holds exactly one row.
	if err := ts.SaveTokens(ctx, "refresh-def", timeNow()); err != nil {
		t.Fatalf("second SaveTokens: %v", err)
	}
	got, err = ts.LoadRefreshToken(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got != "refresh-def" {
		t.Errorf("LoadRefreshToken after update = %q, want refresh-def", got)
	}
}
