// internal/content/cats_test.go
package content

import (
	"strings"
	"testing"
)

func TestCatLoaderLoadOne(t *testing.T) {
	loader := NewCatLoader(NewCache(), "testdata/cats")
	cat, err := loader.LoadOne("mochi")
	if err != nil {
		t.Fatalf("LoadOne: %v", err)
	}
	if cat.Name != "Mochi" {
		t.Errorf("Name = %q, want %q", cat.Name, "Mochi")
	}
	if cat.Photo != "mochi.jpg" {
		t.Errorf("Photo = %q, want %q", cat.Photo, "mochi.jpg")
	}
	if len(cat.Likes) != 2 {
		t.Errorf("Likes = %v, want 2 entries", cat.Likes)
	}
	if !strings.Contains(cat.BioHTML, "<p>") {
		t.Errorf("BioHTML = %q, want rendered HTML paragraph", cat.BioHTML)
	}
}

func TestCatLoaderLoadAll(t *testing.T) {
	loader := NewCatLoader(NewCache(), "testdata/cats")
	cats, err := loader.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	// testdata/cats has mochi.md (valid) and malformed.md (invalid front
	// matter) — LoadAll must skip and log the bad one, not fail entirely.
	if len(cats) != 1 {
		t.Fatalf("LoadAll returned %d cats, want 1 (malformed.md skipped)", len(cats))
	}
}

func TestCatLoaderUnknownSlug(t *testing.T) {
	loader := NewCatLoader(NewCache(), "testdata/cats")
	if _, err := loader.LoadOne("no-such-cat"); err == nil {
		t.Fatal("expected error for unknown slug")
	}
}
