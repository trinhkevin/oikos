package photos

import (
	"context"
	"testing"

	"homesite/internal/store"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	db, err := store.OpenMemory()
	if err != nil {
		t.Fatalf("OpenMemory: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return NewStore(db)
}

func TestStoreInsertAndList(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	p := Photo{
		FileID: "abc123", Path: "2026-07/abc123.jpg", ThumbPath: "2026-07/abc123_thumb.jpg",
		ByteSize: 1024, Width: 800, Height: 600, Source: "gallery",
		CreatedAt: "2026-07-27T12:00:00Z", ClientIP: "192.168.1.10",
	}
	if _, err := s.Insert(ctx, p); err != nil {
		t.Fatalf("Insert: %v", err)
	}

	got, err := s.List(ctx, 10, 0)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 1 || got[0].FileID != "abc123" {
		t.Fatalf("List = %+v, want one entry with FileID abc123", got)
	}
}

func TestStoreCountExcludesHidden(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	visible := Photo{
		FileID: "visible1", Path: "2026-07/visible1.jpg", ThumbPath: "2026-07/visible1_thumb.jpg",
		ByteSize: 1024, Width: 800, Height: 600, Source: "gallery",
		CreatedAt: "2026-07-27T12:00:00Z", ClientIP: "192.168.1.10",
	}
	id, err := s.Insert(ctx, visible)
	if err != nil {
		t.Fatalf("Insert: %v", err)
	}

	hidden := visible
	hidden.FileID = "hidden1"
	hidden.Path = "2026-07/hidden1.jpg"
	hidden.ThumbPath = "2026-07/hidden1_thumb.jpg"
	hiddenID, err := s.Insert(ctx, hidden)
	if err != nil {
		t.Fatalf("Insert: %v", err)
	}
	if err := s.SetHidden(ctx, hiddenID, true); err != nil {
		t.Fatalf("SetHidden: %v", err)
	}

	count, err := s.Count(ctx)
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if count != 1 {
		t.Fatalf("Count = %d, want 1 (only the non-hidden photo, id %d)", count, id)
	}
}

func TestStoreListExcludesHidden(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	visible := Photo{FileID: "visible", Path: "p1.jpg", ThumbPath: "t1.jpg", Source: "gallery", CreatedAt: "2026-07-27T12:00:00Z", ClientIP: "1.1.1.1"}
	hiddenID, err := s.Insert(ctx, visible)
	if err != nil {
		t.Fatal(err)
	}
	_ = hiddenID

	hidden := Photo{FileID: "hidden", Path: "p2.jpg", ThumbPath: "t2.jpg", Source: "gallery", CreatedAt: "2026-07-27T12:00:01Z", ClientIP: "1.1.1.1", Hidden: true}
	if _, err := s.Insert(ctx, hidden); err != nil {
		t.Fatal(err)
	}

	got, err := s.List(ctx, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].FileID != "visible" {
		t.Fatalf("List = %+v, want only the visible entry", got)
	}
}

func TestIsHiddenReturnsTrueForHiddenPhoto(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	hidden := Photo{
		FileID: "hidden1", Path: "2026-07/hidden1.jpg", ThumbPath: "2026-07/hidden1_thumb.jpg",
		Source: "gallery", CreatedAt: "2026-07-27T12:00:00Z", ClientIP: "1.1.1.1", Hidden: true,
	}
	if _, err := s.Insert(ctx, hidden); err != nil {
		t.Fatal(err)
	}

	got, err := s.IsHidden(ctx, "2026-07/hidden1.jpg")
	if err != nil {
		t.Fatalf("IsHidden: %v", err)
	}
	if !got {
		t.Error("IsHidden = false, want true for a hidden photo's path")
	}

	// The thumbnail counterpart must report hidden too.
	got, err = s.IsHidden(ctx, "2026-07/hidden1_thumb.jpg")
	if err != nil {
		t.Fatalf("IsHidden: %v", err)
	}
	if !got {
		t.Error("IsHidden = false, want true for a hidden photo's thumb_path")
	}
}

func TestIsHiddenReturnsFalseForVisiblePhoto(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	visible := Photo{
		FileID: "visible1", Path: "2026-07/visible1.jpg", ThumbPath: "2026-07/visible1_thumb.jpg",
		Source: "gallery", CreatedAt: "2026-07-27T12:00:00Z", ClientIP: "1.1.1.1",
	}
	if _, err := s.Insert(ctx, visible); err != nil {
		t.Fatal(err)
	}

	got, err := s.IsHidden(ctx, "2026-07/visible1.jpg")
	if err != nil {
		t.Fatalf("IsHidden: %v", err)
	}
	if got {
		t.Error("IsHidden = true, want false for a visible photo's path")
	}
}

func TestIsHiddenReturnsFalseForUnknownPath(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	got, err := s.IsHidden(ctx, "2026-07/no-such-file.jpg")
	if err != nil {
		t.Fatalf("IsHidden: %v", err)
	}
	if got {
		t.Error("IsHidden = true, want false (fail open) for a path with no matching row")
	}
}

func TestStoreListAfterExcludesOlderAndHidden(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	mk := func(fileID string) int64 {
		p := Photo{
			FileID: fileID, Path: "2026-07/" + fileID + ".jpg", ThumbPath: "2026-07/" + fileID + "_thumb.jpg",
			ByteSize: 1024, Width: 800, Height: 600, Source: "gallery",
			CreatedAt: "2026-07-27T12:00:00Z", ClientIP: "192.168.1.10",
		}
		id, err := s.Insert(ctx, p)
		if err != nil {
			t.Fatalf("Insert: %v", err)
		}
		return id
	}

	oldID := mk("old")
	newID := mk("new")
	hiddenID := mk("hidden")
	if err := s.SetHidden(ctx, hiddenID, true); err != nil {
		t.Fatalf("SetHidden: %v", err)
	}

	got, err := s.ListAfter(ctx, oldID, 10)
	if err != nil {
		t.Fatalf("ListAfter: %v", err)
	}
	if len(got) != 1 || got[0].FileID != "new" {
		t.Fatalf("ListAfter(after=%d) = %+v, want just the \"new\" photo (id %d); \"hidden\" (id %d) must be excluded", oldID, got, newID, hiddenID)
	}
}

func TestDiskUsageBytesSumsFiles(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir+"/a.jpg", make([]byte, 100))
	writeFile(t, dir+"/b.jpg", make([]byte, 250))

	s := newTestStore(t)
	total, err := s.DiskUsageBytes(dir)
	if err != nil {
		t.Fatalf("DiskUsageBytes: %v", err)
	}
	if total != 350 {
		t.Errorf("DiskUsageBytes = %d, want 350", total)
	}
}
