package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Site       SiteConfig    `yaml:"site"`
	WiFi       WiFiConfig    `yaml:"wifi"`
	Spotify    SpotifyConfig `yaml:"spotify"`
	Photos     PhotosConfig  `yaml:"photos"`
	Limits     LimitsConfig  `yaml:"limits"`
	Admin      AdminConfig   `yaml:"admin"`
	ContentDir string        `yaml:"content_dir"`
	DataDir    string        `yaml:"data_dir"`
	UploadsDir string        `yaml:"uploads_dir"`
}

type SiteConfig struct {
	Title       string `yaml:"title"`
	Listen      string `yaml:"listen"`
	URL         string `yaml:"url"`
	FallbackURL string `yaml:"fallback_url"`
}

type WiFiConfig struct {
	SSID     string `yaml:"ssid"`
	Password string `yaml:"password"`
	Auth     string `yaml:"auth"`
	Hidden   bool   `yaml:"hidden"`
}

type SpotifyConfig struct {
	ClientID               string `yaml:"client_id"`
	ClientSecret           string `yaml:"client_secret"`
	RedirectURI            string `yaml:"redirect_uri"`
	NowPlayingCacheSeconds int    `yaml:"now_playing_cache_seconds"`
}

type PhotosConfig struct {
	MaxFileBytes  int64 `yaml:"max_file_bytes"`
	MaxTotalBytes int64 `yaml:"max_total_bytes"`
	WarnAtPercent int   `yaml:"warn_at_percent"`
	ThumbLongEdge int   `yaml:"thumb_long_edge"`
}

type LimitsConfig struct {
	GuestbookPerWindow    int `yaml:"guestbook_per_window"`
	PhotoUploadsPerWindow int `yaml:"photo_uploads_per_window"`
	SongRequestsPerWindow int `yaml:"song_requests_per_window"`
	WindowMinutes         int `yaml:"window_minutes"`
}

// AdminConfig gates /admin (moderation: archive/delete photos and guest
// book entries) behind HTTP Basic Auth. Empty Username/Password means
// admin is disabled — handled explicitly in the auth middleware as
// fail-closed, not an accidental empty-string bypass.
type AdminConfig struct {
	Username string `yaml:"username"`
	Password string `yaml:"password"`
}

func Load(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config %s: %w", path, err)
	}
	var cfg Config
	if err := yaml.Unmarshal(b, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config %s: %w", path, err)
	}
	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("validating config %s: %w", path, err)
	}
	return &cfg, nil
}

// validate rejects a config that would otherwise degrade silently
// instead of failing loudly at startup. This matters most on the
// disaster-recovery path: a hand-recreated config.yaml (exactly what
// docs/RUNBOOK.md asks someone to do after a dead SD card) with any
// numeric field accidentally omitted would otherwise zero-value its way
// into a silent, total feature outage — a missing window_minutes or
// *_per_window makes every rate-limited endpoint either wide open or
// permanently blocked (see internal/web/ratelimit.go's allow method: a
// zero max makes len(kept) >= rl.max trivially true), a missing
// max_total_bytes makes every upload look like it exceeds a zero cap,
// and a missing max_file_bytes breaks upload sniffing entirely.
func (cfg *Config) validate() error {
	switch {
	case cfg.Site.Listen == "":
		return fmt.Errorf("site.listen must not be empty")
	case cfg.Site.URL == "":
		return fmt.Errorf("site.url must not be empty")
	case cfg.ContentDir == "":
		return fmt.Errorf("content_dir must not be empty")
	case cfg.DataDir == "":
		return fmt.Errorf("data_dir must not be empty")
	case cfg.UploadsDir == "":
		return fmt.Errorf("uploads_dir must not be empty")
	case cfg.Photos.MaxFileBytes <= 0:
		return fmt.Errorf("photos.max_file_bytes must be greater than 0")
	case cfg.Photos.MaxTotalBytes <= 0:
		return fmt.Errorf("photos.max_total_bytes must be greater than 0")
	case cfg.Limits.WindowMinutes <= 0:
		return fmt.Errorf("limits.window_minutes must be greater than 0")
	case cfg.Limits.GuestbookPerWindow <= 0:
		return fmt.Errorf("limits.guestbook_per_window must be greater than 0")
	case cfg.Limits.PhotoUploadsPerWindow <= 0:
		return fmt.Errorf("limits.photo_uploads_per_window must be greater than 0")
	case cfg.Limits.SongRequestsPerWindow <= 0:
		return fmt.Errorf("limits.song_requests_per_window must be greater than 0")
	}
	return nil
}
