package photos

import "testing"

func TestSniffDetectsJPEG(t *testing.T) {
	jpeg := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 'J', 'F', 'I', 'F'}
	mt, ok := Sniff(jpeg)
	if !ok || mt != "image/jpeg" {
		t.Errorf("Sniff(jpeg) = %q, %v, want image/jpeg, true", mt, ok)
	}
}

func TestSniffDetectsPNG(t *testing.T) {
	png := []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a}
	mt, ok := Sniff(png)
	if !ok || mt != "image/png" {
		t.Errorf("Sniff(png) = %q, %v, want image/png, true", mt, ok)
	}
}

func TestSniffDetectsHEIC(t *testing.T) {
	// ISO-BMFF ftyp box: 4-byte size, "ftyp", 4-byte major brand "heic".
	// http.DetectContentType alone returns application/octet-stream for
	// this — verified directly — which is exactly the gap Sniff closes.
	heic := []byte{0x00, 0x00, 0x00, 0x18, 'f', 't', 'y', 'p', 'h', 'e', 'i', 'c', 0x00, 0x00, 0x00, 0x00}
	mt, ok := Sniff(heic)
	if !ok || mt != "image/heic" {
		t.Errorf("Sniff(heic) = %q, %v, want image/heic, true", mt, ok)
	}
}

func TestSniffRejectsTextFileNamedJPG(t *testing.T) {
	// The point: sniffing must be byte-based, never trust a filename or
	// client-supplied MIME type.
	text := []byte("this is not an image, just named photo.jpg\n")
	_, ok := Sniff(text)
	if ok {
		t.Error("expected Sniff to reject plain text regardless of extension")
	}
}
