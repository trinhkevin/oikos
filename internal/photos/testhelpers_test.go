package photos

import (
	"os"
	"testing"
)

func writeFile(t *testing.T, path string, b []byte) {
	t.Helper()
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
}
