// internal/content/menu_test.go
package content

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestMenuLoaderLoadsValidFile(t *testing.T) {
	loader := NewMenuLoader(NewCache())
	menu, err := loader.Load("testdata/menu/valid.yaml")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if menu.Title != "Cocktail Menu" {
		t.Errorf("Title = %q, want %q", menu.Title, "Cocktail Menu")
	}
	if len(menu.Sections) != 1 || menu.Sections[0].Name != "Classics" {
		t.Fatalf("Sections = %+v", menu.Sections)
	}
	item := menu.Sections[0].Items[0]
	if item.Name != "Negroni" || len(item.Ingredients) != 3 {
		t.Fatalf("Items[0] = %+v", item)
	}
}

func TestMenuLoaderMalformedFileReturnsError(t *testing.T) {
	loader := NewMenuLoader(NewCache())
	if _, err := loader.Load("testdata/menu/malformed.yaml"); err == nil {
		t.Fatal("expected error for malformed YAML")
	}
}

func TestMenuLoaderServesStaleOnSubsequentFailure(t *testing.T) {
	// Genuinely exercise the stale-serving contract at the MenuLoader
	// level (mirroring internal/content/cache_test.go's own
	// TestCacheServesStaleValueOnParseFailure): load a valid file, then
	// overwrite it with malformed content — advancing mtime past the
	// filesystem's resolution, since some filesystems only have 1s mtime
	// granularity — reload, and assert the STALE (previous good) value
	// is returned alongside a non-nil error.
	dir := t.TempDir()
	path := filepath.Join(dir, "menu.yaml")
	valid, err := os.ReadFile("testdata/menu/valid.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, valid, 0o644); err != nil {
		t.Fatal(err)
	}

	loader := NewMenuLoader(NewCache())
	menu, err := loader.Load(path)
	if err != nil {
		t.Fatalf("initial Load: %v", err)
	}
	if menu.Note == "" {
		t.Fatal("expected non-empty Note in fixture")
	}

	future := time.Now().Add(2 * time.Second)
	if err := os.WriteFile(path, []byte("not: [valid: yaml: at all"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, future, future); err != nil {
		t.Fatal(err)
	}

	stale, err := loader.Load(path)
	if err == nil {
		t.Fatal("expected an error after the file became malformed")
	}
	if stale.Note != menu.Note || stale.Title != menu.Title {
		t.Errorf("Load after malformed rewrite = %+v, want the stale valid value %+v", stale, menu)
	}
}
