package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/m4rii0/rdtui/pkg/models"
)

const appName = "rdtui"

type Config struct {
	PrivateToken       string `json:"private_token,omitempty"`
	DefaultDownloadDir string `json:"default_download_dir,omitempty"`
	Aria2BinaryPath    string `json:"aria2c_path,omitempty"`

	// Optional test overrides.
	APIBaseURL   string `json:"api_base_url,omitempty"`
	OAuthBaseURL string `json:"oauth_base_url,omitempty"`
}

type Store struct {
	configDir string
	config    string
	auth      string
}

func NewStore() (*Store, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return nil, fmt.Errorf("config dir: %w", err)
	}

	return NewStoreAt(filepath.Join(base, appName)), nil
}

func NewStoreAt(dir string) *Store {
	return &Store{
		configDir: dir,
		config:    filepath.Join(dir, "config.json"),
		auth:      filepath.Join(dir, "auth.json"),
	}
}

func (s *Store) EnsureDir() error {
	if err := os.MkdirAll(s.configDir, 0o755); err != nil {
		return fmt.Errorf("ensure config dir: %w", err)
	}
	return nil
}

func (s *Store) ConfigPath() string {
	return s.config
}

func (s *Store) AuthPath() string {
	return s.auth
}

func (s *Store) LoadConfig() (Config, error) {
	var cfg Config
	if err := s.loadJSON(s.config, &cfg); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return Config{}, err
		}
	}

	if token := os.Getenv("RDTUI_PRIVATE_TOKEN"); token != "" {
		cfg.PrivateToken = token
	}
	if apiBase := os.Getenv("RDTUI_API_BASE_URL"); apiBase != "" {
		cfg.APIBaseURL = apiBase
	}
	if oauthBase := os.Getenv("RDTUI_OAUTH_BASE_URL"); oauthBase != "" {
		cfg.OAuthBaseURL = oauthBase
	}

	if cfg.DefaultDownloadDir == "" {
		home, err := os.UserHomeDir()
		if err == nil {
			cfg.DefaultDownloadDir = filepath.Join(home, "Downloads")
		}
	}

	return cfg, nil
}

func (s *Store) SaveConfig(cfg Config) error {
	if err := s.EnsureDir(); err != nil {
		return err
	}
	return s.saveJSON(s.config, cfg)
}

func (s *Store) LoadDeviceCredentials() (*models.DeviceCredentials, error) {
	var creds models.DeviceCredentials
	if err := s.loadJSON(s.auth, &creds); err != nil {
		return nil, err
	}
	return &creds, nil
}

func (s *Store) SaveDeviceCredentials(creds models.DeviceCredentials) error {
	if err := s.EnsureDir(); err != nil {
		return err
	}
	return s.saveJSON(s.auth, creds)
}

func (s *Store) DeleteDeviceCredentials() error {
	err := os.Remove(s.auth)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func (s *Store) loadJSON(path string, target any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, target); err != nil {
		return fmt.Errorf("decode %s: %w", path, err)
	}
	return nil
}

func (s *Store) saveJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("encode %s: %w", path, err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}
