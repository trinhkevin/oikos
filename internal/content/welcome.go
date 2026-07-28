package content

import (
	"bytes"
	"fmt"

	"github.com/yuin/goldmark"
)

type WelcomeLoader struct {
	cache *Cache
}

func NewWelcomeLoader(cache *Cache) *WelcomeLoader {
	return &WelcomeLoader{cache: cache}
}

func (l *WelcomeLoader) Load(path string) (string, error) {
	v, err := l.cache.Get(path, func(b []byte) (any, error) {
		var buf bytes.Buffer
		if err := goldmark.Convert(b, &buf); err != nil {
			return nil, fmt.Errorf("rendering markdown: %w", err)
		}
		return buf.String(), nil
	})
	if err != nil {
		if s, ok := v.(string); ok {
			return s, err
		}
		return "", err
	}
	return v.(string), nil
}
