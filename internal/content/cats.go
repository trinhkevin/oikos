// internal/content/cats.go
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

type Cat struct {
	Name     string   `yaml:"name"`
	Slug     string   `yaml:"slug"`
	Photo    string   `yaml:"photo"`
	Adopted  string   `yaml:"adopted"`
	Likes    []string `yaml:"likes"`
	Dislikes []string `yaml:"dislikes"`
	BioHTML  string   `yaml:"-"`
}

type CatLoader struct {
	cache *Cache
	dir   string
}

func NewCatLoader(cache *Cache, dir string) *CatLoader {
	return &CatLoader{cache: cache, dir: dir}
}

func (l *CatLoader) LoadOne(slug string) (Cat, error) {
	path := filepath.Join(l.dir, slug+".md")
	v, err := l.cache.Get(path, parseCat)
	if err != nil {
		if c, ok := v.(Cat); ok {
			return c, err
		}
		return Cat{}, err
	}
	return v.(Cat), nil
}

// LoadAll reads every *.md file in dir, skipping (and logging) any that
// fail to parse rather than failing the whole page.
func (l *CatLoader) LoadAll() ([]Cat, error) {
	entries, err := os.ReadDir(l.dir)
	if err != nil {
		return nil, fmt.Errorf("reading cats dir %s: %w", l.dir, err)
	}
	var cats []Cat
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		slug := strings.TrimSuffix(e.Name(), ".md")
		cat, err := l.LoadOne(slug)
		if err != nil {
			log.Printf("content: skipping cat %s: %v", e.Name(), err)
			continue
		}
		cats = append(cats, cat)
	}
	return cats, nil
}

func parseCat(b []byte) (any, error) {
	var cat Cat
	rest, err := frontmatter.Parse(bytes.NewReader(b), &cat)
	if err != nil {
		return nil, fmt.Errorf("parsing front matter: %w", err)
	}
	var buf bytes.Buffer
	if err := goldmark.Convert(rest, &buf); err != nil {
		return nil, fmt.Errorf("rendering markdown: %w", err)
	}
	cat.BioHTML = buf.String()
	return cat, nil
}
