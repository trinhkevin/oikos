package config

import (
	"strings"
	"testing"
)

func TestLoadValid(t *testing.T) {
	cfg, err := Load("testdata/valid.yaml")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Site.Title != "Brivin Household" {
		t.Errorf("Site.Title = %q, want %q", cfg.Site.Title, "Brivin Household")
	}
	if cfg.Site.Listen != ":8080" {
		t.Errorf("Site.Listen = %q, want %q", cfg.Site.Listen, ":8080")
	}
	if cfg.Photos.MaxFileBytes != 26214400 {
		t.Errorf("Photos.MaxFileBytes = %d, want 26214400", cfg.Photos.MaxFileBytes)
	}
	if cfg.Limits.WindowMinutes != 15 {
		t.Errorf("Limits.WindowMinutes = %d, want 15", cfg.Limits.WindowMinutes)
	}
}

func TestLoadMissingFile(t *testing.T) {
	if _, err := Load("testdata/does-not-exist.yaml"); err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

// validConfig returns a Config that passes validate() outright, so each
// table-driven case below can zero out exactly one field and prove that
// field (and only that field) is what validate() is reacting to.
func validConfig() Config {
	return Config{
		Site:       SiteConfig{Listen: ":8080", URL: "http://localhost:8080"},
		ContentDir: "./content",
		DataDir:    "./data",
		UploadsDir: "./uploads",
		Photos:     PhotosConfig{MaxFileBytes: 26214400, MaxTotalBytes: 85899345920},
		Limits: LimitsConfig{
			GuestbookPerWindow: 2, PhotoUploadsPerWindow: 30, SongRequestsPerWindow: 3, WindowMinutes: 15,
		},
	}
}

func TestValidateAcceptsFullyValidConfig(t *testing.T) {
	cfg := validConfig()
	if err := cfg.validate(); err != nil {
		t.Fatalf("validate() = %v, want nil for a fully valid config", err)
	}
}

func TestValidateRejectsMissingOrZeroFields(t *testing.T) {
	tests := []struct {
		name        string
		mutate      func(*Config)
		wantInError string // substring the error must name
	}{
		{"empty site.listen", func(c *Config) { c.Site.Listen = "" }, "site.listen"},
		{"empty site.url", func(c *Config) { c.Site.URL = "" }, "site.url"},
		{"empty content_dir", func(c *Config) { c.ContentDir = "" }, "content_dir"},
		{"empty data_dir", func(c *Config) { c.DataDir = "" }, "data_dir"},
		{"empty uploads_dir", func(c *Config) { c.UploadsDir = "" }, "uploads_dir"},
		{"zero photos.max_file_bytes", func(c *Config) { c.Photos.MaxFileBytes = 0 }, "photos.max_file_bytes"},
		{"negative photos.max_file_bytes", func(c *Config) { c.Photos.MaxFileBytes = -1 }, "photos.max_file_bytes"},
		{"zero photos.max_total_bytes", func(c *Config) { c.Photos.MaxTotalBytes = 0 }, "photos.max_total_bytes"},
		{"zero limits.window_minutes", func(c *Config) { c.Limits.WindowMinutes = 0 }, "limits.window_minutes"},
		{"zero limits.guestbook_per_window", func(c *Config) { c.Limits.GuestbookPerWindow = 0 }, "limits.guestbook_per_window"},
		{"zero limits.photo_uploads_per_window", func(c *Config) { c.Limits.PhotoUploadsPerWindow = 0 }, "limits.photo_uploads_per_window"},
		{"zero limits.song_requests_per_window", func(c *Config) { c.Limits.SongRequestsPerWindow = 0 }, "limits.song_requests_per_window"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := validConfig()
			tc.mutate(&cfg)
			err := cfg.validate()
			if err == nil {
				t.Fatalf("validate() = nil, want an error naming %q", tc.wantInError)
			}
			if !strings.Contains(err.Error(), tc.wantInError) {
				t.Errorf("validate() = %q, want it to name the field %q", err, tc.wantInError)
			}
		})
	}
}

func TestLoadRejectsConfigMissingWindowMinutes(t *testing.T) {
	// End-to-end proof that Load itself (not just validate() in
	// isolation) rejects a hand-edited config with a missing numeric
	// field, rather than silently zero-valuing it into a permanently
	// blocked (or wide-open) rate limiter.
	if _, err := Load("testdata/missing-window-minutes.yaml"); err == nil {
		t.Fatal("expected Load to reject a config missing limits.window_minutes")
	}
}
