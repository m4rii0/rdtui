package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfigIgnoresLegacyExternalCommand(t *testing.T) {
	t.Parallel()

	store := NewStoreAt(t.TempDir())
	data := []byte(`{
	  "default_download_dir": "/tmp/downloads",
	  "aria2c_path": "/usr/bin/aria2c",
	  "external_command": ["aria2c", "--dir", "{{dir}}", "{{url}}"]
	}`)
	if err := os.WriteFile(filepath.Join(store.configDir, "config.json"), data, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	cfg, err := store.LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if cfg.DefaultDownloadDir != "/tmp/downloads" {
		t.Fatalf("DefaultDownloadDir = %q", cfg.DefaultDownloadDir)
	}
	if cfg.Aria2BinaryPath != "/usr/bin/aria2c" {
		t.Fatalf("Aria2BinaryPath = %q", cfg.Aria2BinaryPath)
	}
	if cfg.DownloadBackend != "direct" {
		t.Fatalf("DownloadBackend = %q, want direct", cfg.DownloadBackend)
	}
}

func TestLoadConfigSupportsDirectBackend(t *testing.T) {
	t.Parallel()

	store := NewStoreAt(t.TempDir())
	data := []byte(`{
	  "download_backend": "direct"
	}`)
	if err := os.WriteFile(filepath.Join(store.configDir, "config.json"), data, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	cfg, err := store.LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if cfg.DownloadBackend != "direct" {
		t.Fatalf("DownloadBackend = %q, want direct", cfg.DownloadBackend)
	}
}
