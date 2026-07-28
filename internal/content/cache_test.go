package content

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCacheReparsesOnChange(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sample.txt")
	if err := os.WriteFile(path, []byte("v1"), 0o644); err != nil {
		t.Fatal(err)
	}

	c := NewCache()
	parseCount := 0
	parse := func(b []byte) (any, error) {
		parseCount++
		return string(b), nil
	}

	got, err := c.Get(path, parse)
	if err != nil {
		t.Fatal(err)
	}
	if got.(string) != "v1" || parseCount != 1 {
		t.Fatalf("got %v, parseCount %d, want v1/1", got, parseCount)
	}

	// Re-fetch without modifying the file: must NOT reparse.
	if _, err := c.Get(path, parse); err != nil {
		t.Fatal(err)
	}
	if parseCount != 1 {
		t.Fatalf("parseCount = %d after unchanged re-fetch, want 1", parseCount)
	}

	// mtime granularity on some filesystems is 1s; force it forward.
	future := time.Now().Add(2 * time.Second)
	if err := os.WriteFile(path, []byte("v2"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, future, future); err != nil {
		t.Fatal(err)
	}

	got, err = c.Get(path, parse)
	if err != nil {
		t.Fatal(err)
	}
	if got.(string) != "v2" || parseCount != 2 {
		t.Fatalf("got %v, parseCount %d, want v2/2 after modification", got, parseCount)
	}
}

func TestCacheMissingFileReturnsError(t *testing.T) {
	c := NewCache()
	_, err := c.Get("/no/such/file", func(b []byte) (any, error) { return nil, nil })
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}
