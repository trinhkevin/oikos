// views/recipes.go
package views

import (
	"sort"
	"strings"

	"homesite/internal/content"
)

// recipeSearchBlob is what static/js/recipes.js substring-matches
// against: title, tags, and the raw (unrendered) markdown body so
// ingredient/instruction text is searchable too, without needing to
// parse ingredients out as structured data.
func recipeSearchBlob(r content.Recipe) string {
	parts := []string{r.Title, strings.Join(r.Tags, " "), r.BodyText}
	return strings.ToLower(strings.Join(parts, " "))
}

// recipeTagData joins tags with "|" (not ", ") for the data-tags
// attribute, so the JS filter can split on a delimiter that never
// collides with whitespace inside a tag.
func recipeTagData(tags []string) string {
	return strings.Join(tags, "|")
}

// distinctRecipeTags collects every tag across all recipes, in first-
// seen-then-alphabetical order, for the index page's filter chips.
func distinctRecipeTags(recipes []content.Recipe) []string {
	seen := make(map[string]bool)
	var tags []string
	for _, r := range recipes {
		for _, tag := range r.Tags {
			if !seen[tag] {
				seen[tag] = true
				tags = append(tags, tag)
			}
		}
	}
	sort.Strings(tags)
	return tags
}

// recipeMeta renders the compact "20 min · serves 4" line, omitting
// either half when its field is blank, and returning "" if both are.
func recipeMeta(r content.Recipe) string {
	var parts []string
	if r.Time != "" {
		parts = append(parts, r.Time)
	}
	if r.Servings != "" {
		parts = append(parts, "serves "+r.Servings)
	}
	return strings.Join(parts, " · ")
}
