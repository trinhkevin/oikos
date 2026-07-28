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
