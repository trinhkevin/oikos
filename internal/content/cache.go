package content

import (
	"fmt"
	"os"
	"sync"
	"time"
)

type entry struct {
	modTime time.Time
	value   any
}

// Cache holds parsed content keyed by file path, invalidated when the
// file's mtime changes. A file that fails to parse leaves the previous
// good entry in place — callers should log the error and keep serving
// stale-but-valid content rather than surface a 500.
type Cache struct {
	mu      sync.Mutex
	entries map[string]entry
}

func NewCache() *Cache {
	return &Cache{entries: make(map[string]entry)}
}

// Get returns the cached, parsed value for path, reparsing via parse
// only if the file's mtime has advanced since the last successful parse.
// If parse fails and a previous good value exists, that value is returned
// alongside the error so callers can choose to log-and-serve-stale.
func (c *Cache) Get(path string, parse func([]byte) (any, error)) (any, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("stat %s: %w", path, err)
	}

	c.mu.Lock()
	e, ok := c.entries[path]
	c.mu.Unlock()
	if ok && !info.ModTime().After(e.modTime) {
		return e.value, nil
	}

	b, err := os.ReadFile(path)
	if err != nil {
		if ok {
			return e.value, fmt.Errorf("reading %s (serving stale): %w", path, err)
		}
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}

	value, err := parse(b)
	if err != nil {
		if ok {
			return e.value, fmt.Errorf("parsing %s (serving stale): %w", path, err)
		}
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}

	c.mu.Lock()
	c.entries[path] = entry{modTime: info.ModTime(), value: value}
	c.mu.Unlock()
	return value, nil
}
