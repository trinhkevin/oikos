// internal/web/healthz.go
package web

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

type healthzResponse struct {
	Status        string `json:"status"`
	DiskUsedBytes int64  `json:"disk_used_bytes"`
	DiskCapBytes  int64  `json:"disk_cap_bytes"`
	LastBackup    string `json:"last_backup"`
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	used, err := s.photosStore.DiskUsageBytes(s.cfg.UploadsDir)
	if err != nil {
		used = -1
	}

	lastBackup := "never"
	if b, err := os.ReadFile(filepath.Join(s.cfg.DataDir, ".last_backup")); err == nil {
		lastBackup = strings.TrimSpace(string(b))
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(healthzResponse{
		Status:        "ok",
		DiskUsedBytes: used,
		DiskCapBytes:  s.cfg.Photos.MaxTotalBytes,
		LastBackup:    lastBackup,
	})
}
