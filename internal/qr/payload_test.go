// internal/qr/payload_test.go
package qr

import "testing"

func TestWiFiPayloadBasic(t *testing.T) {
	got := WiFiPayload("MyNet", "hunter2", "WPA", false)
	want := "WIFI:T:WPA;S:MyNet;P:hunter2;;"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestWiFiPayloadHidden(t *testing.T) {
	got := WiFiPayload("MyNet", "hunter2", "WPA", true)
	want := "WIFI:T:WPA;S:MyNet;P:hunter2;H:true;;"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestWiFiPayloadEscapesSpecialChars(t *testing.T) {
	// SSID and password containing `;` `,` `:` `\` must be backslash-escaped
	// per the WIFI: QR payload spec — otherwise a camera app misparses the
	// field boundaries.
	got := WiFiPayload(`Kev;in,Net:work\`, `p:a;s,s\word`, "WPA", false)
	want := `WIFI:T:WPA;S:Kev\;in\,Net\:work\\;P:p\:a\;s\,s\\word;;`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestURLPayload(t *testing.T) {
	got := URLPayload("http://home.arpa")
	want := "http://home.arpa"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
