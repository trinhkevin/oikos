// internal/content/recipes_test.go
package content

import (
	"strings"
	"testing"
)

func TestRecipeLoaderLoadOne(t *testing.T) {
	loader := NewRecipeLoader(NewCache(), "testdata/recipes")
	recipe, err := loader.LoadOne("test-skillet")
	if err != nil {
		t.Fatalf("LoadOne: %v", err)
	}
	if recipe.Title != "Test Skillet" {
		t.Errorf("Title = %q, want %q", recipe.Title, "Test Skillet")
	}
	if recipe.SourceURL != "https://example.com/test-skillet" {
		t.Errorf("SourceURL = %q, want the fixture's source_url", recipe.SourceURL)
	}
	if len(recipe.Tags) != 2 {
		t.Errorf("Tags = %v, want 2 entries", recipe.Tags)
	}
	if recipe.Servings != "2" {
		t.Errorf("Servings = %q, want %q", recipe.Servings, "2")
	}
	if !strings.Contains(recipe.BodyHTML, "<h2>Ingredients</h2>") {
		t.Errorf("BodyHTML = %q, want a rendered Ingredients heading", recipe.BodyHTML)
	}
	if !strings.Contains(recipe.BodyText, "test ingredient") {
		t.Errorf("BodyText = %q, want the raw markdown body (for search)", recipe.BodyText)
	}
}

func TestRecipeLoaderLoadAll(t *testing.T) {
	loader := NewRecipeLoader(NewCache(), "testdata/recipes")
	recipes, err := loader.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	// testdata/recipes has test-skillet.md (valid) and malformed.md
	// (invalid front matter) -- LoadAll must skip and log the bad one.
	if len(recipes) != 1 {
		t.Fatalf("LoadAll returned %d recipes, want 1 (malformed.md skipped)", len(recipes))
	}
}

func TestRecipeLoaderUnknownSlug(t *testing.T) {
	loader := NewRecipeLoader(NewCache(), "testdata/recipes")
	if _, err := loader.LoadOne("no-such-recipe"); err == nil {
		t.Fatal("expected error for unknown slug")
	}
}

// TestRecipeLoaderRejectsPathTraversal locks in that LoadOne refuses any
// slug that could escape the recipes directory via filepath.Join, since
// http.ServeMux unescapes {slug} after routing (e.g. "..%2f..%2fCLAUDE"
// arrives here as "../../CLAUDE").
func TestRecipeLoaderRejectsPathTraversal(t *testing.T) {
	loader := NewRecipeLoader(NewCache(), "testdata/recipes")
	for _, slug := range []string{
		"../../CLAUDE",
		"../recipes_test",
		"foo/bar",
		`foo\bar`,
		"..",
		"",
	} {
		if _, err := loader.LoadOne(slug); err == nil {
			t.Errorf("LoadOne(%q) = nil error, want error rejecting traversal/invalid slug", slug)
		}
	}
}
