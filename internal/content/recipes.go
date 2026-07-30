// internal/content/recipes.go
package content

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/adrg/frontmatter"
	"github.com/yuin/goldmark"
)

type Recipe struct {
	Title      string   `yaml:"title"`
	Slug       string   `yaml:"slug"`
	SourceURL  string   `yaml:"source_url"`
	SourceName string   `yaml:"source_name"`
	Tags       []string `yaml:"tags"`
	Servings   string   `yaml:"servings"`
	Time       string   `yaml:"time"`
	BodyHTML   string   `yaml:"-"`
	BodyText   string   `yaml:"-"`
}

type RecipeLoader struct {
	cache *Cache
	dir   string
}

func NewRecipeLoader(cache *Cache, dir string) *RecipeLoader {
	return &RecipeLoader{cache: cache, dir: dir}
}

func (l *RecipeLoader) LoadOne(slug string) (Recipe, error) {
	path := filepath.Join(l.dir, slug+".md")
	v, err := l.cache.Get(path, parseRecipe)
	if err != nil {
		if r, ok := v.(Recipe); ok {
			return r, err
		}
		return Recipe{}, err
	}
	return v.(Recipe), nil
}

// LoadAll reads every *.md file in dir, skipping (and logging) any that
// fail to parse rather than failing the whole page.
func (l *RecipeLoader) LoadAll() ([]Recipe, error) {
	entries, err := os.ReadDir(l.dir)
	if err != nil {
		return nil, fmt.Errorf("reading recipes dir %s: %w", l.dir, err)
	}
	var recipes []Recipe
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		slug := strings.TrimSuffix(e.Name(), ".md")
		recipe, err := l.LoadOne(slug)
		if err != nil {
			log.Printf("content: skipping recipe %s: %v", e.Name(), err)
			continue
		}
		recipes = append(recipes, recipe)
	}
	return recipes, nil
}

func parseRecipe(b []byte) (any, error) {
	var recipe Recipe
	rest, err := frontmatter.Parse(bytes.NewReader(b), &recipe)
	if err != nil {
		return nil, fmt.Errorf("parsing front matter: %w", err)
	}
	recipe.BodyText = string(rest)
	var buf bytes.Buffer
	if err := goldmark.Convert(rest, &buf); err != nil {
		return nil, fmt.Errorf("rendering markdown: %w", err)
	}
	recipe.BodyHTML = buf.String()
	return recipe, nil
}
