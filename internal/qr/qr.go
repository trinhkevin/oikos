// internal/qr/qr.go
package qr

import (
	"bytes"
	"fmt"

	qrcode "github.com/yeqown/go-qrcode/v2"
	"github.com/yeqown/go-qrcode/writer/standard"
)

// bufWriteCloser adapts a bytes.Buffer to io.WriteCloser, which the
// standard writer requires, so PNG rendering can happen fully in memory
// with no temp file on disk.
type bufWriteCloser struct {
	*bytes.Buffer
}

func (bufWriteCloser) Close() error { return nil }

// PNG renders payload as a QR code PNG: white background, black modules
// (locked regardless of viewer theme — an inverted QR fails to scan on
// many camera apps), error correction level Q (25% recovery, per the
// spec's "reads at an angle, off a glossy screen, in dim light"
// requirement), and a generous quiet zone.
func PNG(payload string) ([]byte, error) {
	if payload == "" {
		return nil, fmt.Errorf("qr: payload must not be empty")
	}

	qrc, err := qrcode.NewWith(payload, qrcode.WithErrorCorrectionLevel(qrcode.ErrorCorrectionQuart))
	if err != nil {
		return nil, fmt.Errorf("qr: building code: %w", err)
	}

	buf := &bufWriteCloser{Buffer: &bytes.Buffer{}}
	w := standard.NewWithWriter(buf,
		standard.WithBgColorRGBHex("#FFFFFF"),
		standard.WithFgColorRGBHex("#000000"),
		standard.WithQRWidth(12),
		standard.WithBorderWidth(20),
		standard.WithBuiltinImageEncoder(standard.PNG_FORMAT),
	)

	if err := qrc.Save(w); err != nil {
		return nil, fmt.Errorf("qr: rendering: %w", err)
	}
	return buf.Bytes(), nil
}
