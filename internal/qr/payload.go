// internal/qr/payload.go
package qr

import "strings"

var wifiEscaper = strings.NewReplacer(
	`\`, `\\`,
	`;`, `\;`,
	`,`, `\,`,
	`:`, `\:`,
)

// WiFiPayload builds the standard WIFI: QR payload
// (WIFI:T:<auth>;S:<ssid>;P:<password>;[H:true;];), escaping `;` `,` `:`
// and `\` in the SSID and password as the format requires.
func WiFiPayload(ssid, password, auth string, hidden bool) string {
	var b strings.Builder
	b.WriteString("WIFI:T:")
	b.WriteString(auth)
	b.WriteString(";S:")
	b.WriteString(wifiEscaper.Replace(ssid))
	b.WriteString(";P:")
	b.WriteString(wifiEscaper.Replace(password))
	b.WriteString(";")
	if hidden {
		b.WriteString("H:true;")
	}
	b.WriteString(";")
	return b.String()
}

// URLPayload is a passthrough that exists so call sites read
// qr.URLPayload(cfg.Site.URL) rather than passing a raw string, keeping
// every QR-producing call site symmetrical.
func URLPayload(url string) string {
	return url
}
