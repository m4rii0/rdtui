package config

import (
	"cmp"
	"fmt"
	"path/filepath"
	"sync"

	"github.com/m4rii0/rdtui/internal/realdebrid"
	"github.com/m4rii0/rdtui/pkg/models"
)

// Overrides is an environment snapshot supplied by the composition root.
type Overrides struct {
	PrivateToken string
	APIBaseURL   string
	OAuthBaseURL string
}

// EffectiveConfig contains resolved runtime values, never persisted preferences.
type EffectiveConfig struct {
	PrivateToken       string
	DefaultDownloadDir string
	DownloadBackend    models.DownloadBackend
	Aria2BinaryPath    string
	APIBaseURL         string
	OAuthBaseURL       string
}

func Resolve(prefs Config, overrides Overrides, home string) (EffectiveConfig, error) {
	dir := prefs.DefaultDownloadDir
	if dir == "" {
		if home == "" {
			return EffectiveConfig{}, fmt.Errorf("resolve download directory: home directory unavailable; set default_download_dir explicitly")
		}
		dir = filepath.Join(home, "Downloads")
	}
	backend := models.DownloadBackendDirect
	if models.DownloadBackend(prefs.DownloadBackend) == models.DownloadBackendAria2 {
		backend = models.DownloadBackendAria2
	}
	return EffectiveConfig{
		PrivateToken:       cmp.Or(overrides.PrivateToken, prefs.PrivateToken),
		DefaultDownloadDir: dir,
		DownloadBackend:    backend,
		Aria2BinaryPath:    prefs.Aria2BinaryPath,
		APIBaseURL:         cmp.Or(overrides.APIBaseURL, prefs.APIBaseURL, realdebrid.DefaultAPIBaseURL),
		OAuthBaseURL:       cmp.Or(overrides.OAuthBaseURL, prefs.OAuthBaseURL, realdebrid.DefaultOAuthBaseURL),
	}, nil
}

type PreferenceStore interface {
	LoadPreferences() (Config, error)
	SaveConfig(Config) error
}

// Repository serializes narrow updates to its persisted preference snapshot.
// One repository owns updates for a store within the process.
type Repository struct {
	mu        sync.RWMutex
	store     PreferenceStore
	prefs     Config
	effective EffectiveConfig
	overrides Overrides
	home      string
}

func NewRepository(store PreferenceStore, overrides Overrides, home string) (*Repository, error) {
	prefs, err := store.LoadPreferences()
	if err != nil {
		return nil, fmt.Errorf("load preferences: %w", err)
	}
	effective, err := Resolve(prefs, overrides, home)
	if err != nil {
		return nil, err
	}
	return &Repository{store: store, prefs: prefs, effective: effective, overrides: overrides, home: home}, nil
}

func (r *Repository) Effective() EffectiveConfig {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.effective
}

// Update callbacks must not call back into the repository.
func (r *Repository) Update(change func(*Config) error) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	prefs := r.prefs
	if err := change(&prefs); err != nil {
		return err
	}
	effective, err := Resolve(prefs, r.overrides, r.home)
	if err != nil {
		return err
	}
	if err := r.store.SaveConfig(prefs); err != nil {
		return err
	}
	r.prefs = prefs
	r.effective = effective
	return nil
}

func (r *Repository) EffectivePrivateToken() string {
	return r.Effective().PrivateToken
}

func (r *Repository) SavePrivateToken(token string) error {
	return r.Update(func(prefs *Config) error {
		prefs.PrivateToken = token
		return nil
	})
}
