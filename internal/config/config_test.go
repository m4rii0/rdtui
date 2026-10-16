package config

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/m4rii0/rdtui/pkg/models"
)

func TestStoredJSONCompatibility(t *testing.T) {
	for _, key := range []string{"RDTUI_PRIVATE_TOKEN", "RDTUI_API_BASE_URL", "RDTUI_OAUTH_BASE_URL"} {
		t.Setenv(key, "")
	}
	store := NewStoreAt(t.TempDir())
	cfg := Config{
		PrivateToken: "<private>", DefaultDownloadDir: "/downloads", DownloadBackend: "aria2",
		Aria2BinaryPath: "/bin/aria2c", APIBaseURL: "https://api.example", OAuthBaseURL: "https://oauth.example",
	}
	const configJSON = `{
  "private_token": "\u003cprivate\u003e",
  "default_download_dir": "/downloads",
  "download_backend": "aria2",
  "aria2c_path": "/bin/aria2c",
  "api_base_url": "https://api.example",
  "oauth_base_url": "https://oauth.example"
}
`
	if err := os.WriteFile(store.ConfigPath(), []byte(configJSON), 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.LoadConfig()
	if err != nil || loaded != cfg {
		t.Fatalf("LoadConfig() = %+v, %v; want %+v", loaded, err, cfg)
	}
	if err := store.SaveConfig(loaded); err != nil {
		t.Fatal(err)
	}
	assertJSONBytes(t, store.ConfigPath(), configJSON)

	creds := models.DeviceCredentials{
		ClientID: "client", ClientSecret: "secret", AccessToken: "access", RefreshToken: "refresh", TokenType: "Bearer",
	}
	const authJSON = `{
  "client_id": "client",
  "client_secret": "secret",
  "access_token": "access",
  "refresh_token": "refresh",
  "token_type": "Bearer",
  "expires_at": "0001-01-01T00:00:00Z",
  "created_at": "0001-01-01T00:00:00Z"
}
`
	if err := os.WriteFile(store.AuthPath(), []byte(authJSON), 0o600); err != nil {
		t.Fatal(err)
	}
	loadedCreds, err := store.LoadDeviceCredentials()
	if err != nil || *loadedCreds != creds {
		t.Fatalf("LoadDeviceCredentials() = %+v, %v; want %+v", loadedCreds, err, creds)
	}
	if err := store.SaveDeviceCredentials(*loadedCreds); err != nil {
		t.Fatal(err)
	}
	assertJSONBytes(t, store.AuthPath(), authJSON)
	if err := store.SaveConfig(Config{}); err != nil {
		t.Fatal(err)
	}
	assertJSONBytes(t, store.ConfigPath(), "{}\n")
}

func assertJSONBytes(t *testing.T, path, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid(data) || !bytes.Equal(data, []byte(want)) {
		t.Fatalf("stored JSON = %s, want %s", data, want)
	}
}

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
	if cfg.DownloadBackend != "" {
		t.Fatalf("persisted DownloadBackend = %q, want empty", cfg.DownloadBackend)
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
