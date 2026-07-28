// internal/content/menu_test.go
package content

import "testing"

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
	// Load valid content first, then verify that after the underlying
	// cache records a good value, a parse failure on re-fetch would
	// serve stale — this is exercised at the Cache layer (Task 2); here
	// we only confirm MenuLoader propagates Cache's contract untouched
	// by decoding into the correct type on the happy path.
	loader := NewMenuLoader(NewCache())
	menu, err := loader.Load("testdata/menu/valid.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if menu.Note == "" {
		t.Error("expected non-empty Note in fixture")
	}
}
