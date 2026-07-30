// internal/guestbook/guestbook_test.go
package guestbook

import (
	"context"
	"errors"
	"testing"

	"homesite/internal/store"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	db, err := store.OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return NewStore(db)
}

func TestCreateAndList(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	id, err := s.Create(ctx, Entry{Name: "Alex", Message: "Great party!", ClientIP: "1.2.3.4"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if id == 0 {
		t.Fatal("expected non-zero id")
	}

	entries, err := s.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) != 1 || entries[0].Name != "Alex" {
		t.Fatalf("List = %+v", entries)
	}
}

func TestStoreCountExcludesHidden(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if _, err := s.Create(ctx, Entry{Name: "Visible", Message: "hi", ClientIP: "1.2.3.4"}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	hiddenID, err := s.Create(ctx, Entry{Name: "Hidden", Message: "hi", ClientIP: "1.2.3.4"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := s.SetHidden(ctx, hiddenID, true); err != nil {
		t.Fatalf("SetHidden: %v", err)
	}

	count, err := s.Count(ctx)
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if count != 1 {
		t.Fatalf("Count = %d, want 1 (only the non-hidden entry)", count)
	}
}

func TestCreateRejectsEmptyName(t *testing.T) {
	s := newTestStore(t)
	_, err := s.Create(context.Background(), Entry{Name: "", Message: "hi", ClientIP: "1.2.3.4"})
	if !errors.Is(err, ErrNameRequired) {
		t.Fatalf("err = %v, want ErrNameRequired", err)
	}
}

func TestCreateRejectsOverlongName(t *testing.T) {
	s := newTestStore(t)
	longName := make([]byte, 41)
	for i := range longName {
		longName[i] = 'a'
	}
	_, err := s.Create(context.Background(), Entry{Name: string(longName), Message: "hi", ClientIP: "1.2.3.4"})
	if !errors.Is(err, ErrNameTooLong) {
		t.Fatalf("err = %v, want ErrNameTooLong", err)
	}
}

func TestCreateRejectsEmptyMessage(t *testing.T) {
	s := newTestStore(t)
	_, err := s.Create(context.Background(), Entry{Name: "Alex", Message: "", ClientIP: "1.2.3.4"})
	if !errors.Is(err, ErrMessageRequired) {
		t.Fatalf("err = %v, want ErrMessageRequired", err)
	}
}

func TestCreateRejectsOverlongMessage(t *testing.T) {
	s := newTestStore(t)
	longMsg := make([]byte, 501)
	for i := range longMsg {
		longMsg[i] = 'a'
	}
	_, err := s.Create(context.Background(), Entry{Name: "Alex", Message: string(longMsg), ClientIP: "1.2.3.4"})
	if !errors.Is(err, ErrMessageTooLong) {
		t.Fatalf("err = %v, want ErrMessageTooLong", err)
	}
}

func TestListExcludesHidden(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	if _, err := s.Create(ctx, Entry{Name: "Visible", Message: "shown", ClientIP: "1.1.1.1"}); err != nil {
		t.Fatal(err)
	}
	id, err := s.Create(ctx, Entry{Name: "Hidden", Message: "moderated away", ClientIP: "1.1.1.1"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.db().ExecContext(ctx, "UPDATE guestbook_entries SET hidden = 1 WHERE id = ?", id); err != nil {
		t.Fatal(err)
	}

	entries, err := s.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name != "Visible" {
		t.Fatalf("List = %+v, want only Visible", entries)
	}
}
