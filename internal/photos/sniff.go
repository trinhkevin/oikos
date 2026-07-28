package photos

import "net/http"

var heicBrands = map[string]bool{
	"heic": true, "heix": true, "heim": true, "heis": true,
	"hevc": true, "hevx": true, "mif1": true, "msf1": true,
}

// Sniff identifies an image's real type from its bytes, never from a
// filename or client-supplied Content-Type header. It special-cases the
// ISO-BMFF `ftyp` box (HEIC/HEIF) before falling back to
// http.DetectContentType, which does not recognize that family at all.
func Sniff(b []byte) (mimeType string, ok bool) {
	if len(b) >= 12 && string(b[4:8]) == "ftyp" && heicBrands[string(b[8:12])] {
		return "image/heic", true
	}
	switch ct := http.DetectContentType(b); ct {
	case "image/jpeg", "image/png", "image/webp":
		return ct, true
	}
	return "", false
}
