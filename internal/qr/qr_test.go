// internal/qr/qr_test.go
package qr

import "testing"

func TestPNGProducesValidImage(t *testing.T) {
	b, err := PNG("http://home.arpa")
	if err != nil {
		t.Fatalf("PNG: %v", err)
	}
	if len(b) == 0 {
		t.Fatal("PNG returned empty bytes")
	}
	// PNG magic bytes.
	sig := []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a}
	if len(b) < len(sig) {
		t.Fatalf("output too short to be a PNG: %d bytes", len(b))
	}
	for i, want := range sig {
		if b[i] != want {
			t.Fatalf("byte %d = %#x, want %#x — not a PNG", i, b[i], want)
		}
	}
}

func TestPNGRejectsEmptyPayload(t *testing.T) {
	if _, err := PNG(""); err == nil {
		t.Fatal("expected error for empty payload")
	}
}
