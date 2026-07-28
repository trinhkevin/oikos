// internal/web/healthz_test.go
package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestHealthzReportsDiskUsageAndBackupStatus(t *testing.T) {
	cfg, uploadsDir := testConfigWithPhotos(t)
	if err := os.WriteFile(filepath.Join(uploadsDir, "a.jpg"), make([]byte, 500), 0o644); err != nil {
		t.Fatal(err)
	}
	dataDir := t.TempDir()
	cfg.DataDir = dataDir
	backupStamp := "2026-07-27T04:00:00Z"
	if err := os.WriteFile(filepath.Join(dataDir, ".last_backup"), []byte(backupStamp+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	s := New(cfg, mustOpenMemoryDB(t))
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body struct {
		Status        string `json:"status"`
		DiskUsedBytes int64  `json:"disk_used_bytes"`
		DiskCapBytes  int64  `json:"disk_cap_bytes"`
		LastBackup    string `json:"last_backup"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decoding healthz JSON: %v", err)
	}
	if body.Status != "ok" {
		t.Errorf("Status = %q, want ok", body.Status)
	}
	if body.DiskUsedBytes != 500 {
		t.Errorf("DiskUsedBytes = %d, want 500", body.DiskUsedBytes)
	}
	if body.LastBackup != backupStamp {
		t.Errorf("LastBackup = %q, want %q", body.LastBackup, backupStamp)
	}
}

func TestHealthzReportsNeverBackedUp(t *testing.T) {
	cfg, _ := testConfigWithPhotos(t)
	cfg.DataDir = t.TempDir() // no .last_backup file present

	s := New(cfg, mustOpenMemoryDB(t))
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)

	var body struct {
		LastBackup string `json:"last_backup"`
	}
	json.NewDecoder(rec.Body).Decode(&body)
	if body.LastBackup != "never" {
		t.Errorf("LastBackup = %q, want %q when no backup has run", body.LastBackup, "never")
	}
}
