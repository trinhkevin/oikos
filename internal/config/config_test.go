package config

import "testing"

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
