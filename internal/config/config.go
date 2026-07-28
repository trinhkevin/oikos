package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Site        SiteConfig    `yaml:"site"`
	WiFi        WiFiConfig    `yaml:"wifi"`
	Spotify     SpotifyConfig `yaml:"spotify"`
	Photos      PhotosConfig  `yaml:"photos"`
	Limits      LimitsConfig  `yaml:"limits"`
	ContentDir  string        `yaml:"content_dir"`
	DataDir     string        `yaml:"data_dir"`
	UploadsDir  string        `yaml:"uploads_dir"`
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
	PlaylistURL  string `yaml:"playlist_url"`
	ClientID     string `yaml:"client_id"`
	ClientSecret string `yaml:"client_secret"`
	CacheSeconds int    `yaml:"cache_seconds"`
}

type PhotosConfig struct {
	MaxFileBytes   int64 `yaml:"max_file_bytes"`
	MaxTotalBytes  int64 `yaml:"max_total_bytes"`
	WarnAtPercent  int   `yaml:"warn_at_percent"`
	ThumbLongEdge  int   `yaml:"thumb_long_edge"`
}

type LimitsConfig struct {
	GuestbookPerWindow    int `yaml:"guestbook_per_window"`
	PhotoUploadsPerWindow int `yaml:"photo_uploads_per_window"`
	WindowMinutes         int `yaml:"window_minutes"`
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
	return &cfg, nil
}
