package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/m4rii0/rdtui/pkg/models"
)

func TestUnrelatedSaveDoesNotPersistEnvironment(t *testing.T) {
	t.Setenv("RDTUI_PRIVATE_TOKEN", "environment-token")
	t.Setenv("RDTUI_API_BASE_URL", "https://environment-api.example")
	t.Setenv("RDTUI_OAUTH_BASE_URL", "https://environment-oauth.example")
	store := NewStoreAt(t.TempDir())
	original := Config{PrivateToken: "persisted-token"}
	if err := store.SaveConfig(original); err != nil {
		t.Fatal(err)
	}
	repo, err := NewRepository(store, Overrides{
		PrivateToken: os.Getenv("RDTUI_PRIVATE_TOKEN"),
		APIBaseURL:   os.Getenv("RDTUI_API_BASE_URL"),
		OAuthBaseURL: os.Getenv("RDTUI_OAUTH_BASE_URL"),
	}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	// Mutating the process environment after composition must not affect saves.
	t.Setenv("RDTUI_PRIVATE_TOKEN", "later-token")
	t.Setenv("RDTUI_API_BASE_URL", "https://later-api.example")
	t.Setenv("RDTUI_OAUTH_BASE_URL", "https://later-oauth.example")
	if err := repo.Update(func(prefs *Config) error {
		prefs.Aria2BinaryPath = "/custom/aria2c"
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	persisted, err := store.LoadPreferences()
	if err != nil {
		t.Fatal(err)
	}
	if persisted.PrivateToken != original.PrivateToken || persisted.APIBaseURL != "" || persisted.OAuthBaseURL != "" || persisted.DefaultDownloadDir != "" || persisted.DownloadBackend != "" {
		t.Fatal("environment values or computed defaults leaked into preferences")
	}
	if persisted.Aria2BinaryPath != "/custom/aria2c" {
		t.Fatal("explicit preference was not saved")
	}
	effective := repo.Effective()
	if effective.PrivateToken != "environment-token" || effective.APIBaseURL != "https://environment-api.example" || effective.OAuthBaseURL != "https://environment-oauth.example" {
		t.Fatalf("composition snapshot changed: %+v", effective)
	}
}

func TestExplicitTokenSaveUsesEnteredToken(t *testing.T) {
	store := NewStoreAt(t.TempDir())
	repo, err := NewRepository(store, Overrides{PrivateToken: "environment-token"}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.SavePrivateToken("entered-token"); err != nil {
		t.Fatal(err)
	}
	persisted, err := store.LoadPreferences()
	if err != nil || persisted.PrivateToken != "entered-token" {
		t.Fatalf("entered token not saved: %+v, %v", persisted, err)
	}
	if repo.EffectivePrivateToken() != "environment-token" {
		t.Fatal("explicit save replaced the environment override")
	}
}

type gatedPreferenceStore struct {
	mu      sync.Mutex
	prefs   Config
	saves   int
	entered chan struct{}
	release chan struct{}
}

func (s *gatedPreferenceStore) LoadPreferences() (Config, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.prefs, nil
}

func (s *gatedPreferenceStore) SaveConfig(prefs Config) error {
	s.mu.Lock()
	s.saves++
	first := s.saves == 1
	s.mu.Unlock()
	if first {
		close(s.entered)
		<-s.release
	}
	s.mu.Lock()
	s.prefs = prefs
	s.mu.Unlock()
	return nil
}

func TestConcurrentPreferenceUpdatesPreserveBothChanges(t *testing.T) {
	store := &gatedPreferenceStore{entered: make(chan struct{}), release: make(chan struct{})}
	repo, err := NewRepository(store, Overrides{}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	firstDone, secondDone := make(chan error, 1), make(chan error, 1)
	go func() {
		firstDone <- repo.Update(func(prefs *Config) error {
			prefs.Aria2BinaryPath = "/changed/aria2c"
			return nil
		})
	}()
	<-store.entered
	// Mutex waits are not durably blocked in testing/synctest. Probe the actual
	// admission lock while the first save is gated, rather than merely observing
	// a goroutine before Update. If serialization is removed, force the second
	// commit to finish first, making the lost update deterministic too.
	admission := make(chan bool)
	go func() {
		unserialized := repo.mu.TryLock()
		if unserialized {
			repo.mu.Unlock()
		}
		admission <- unserialized
		secondDone <- repo.Update(func(prefs *Config) error {
			prefs.DefaultDownloadDir = "/changed/downloads"
			return nil
		})
	}()
	unserialized := <-admission
	var secondErr error
	if unserialized {
		secondErr = <-secondDone
	}
	close(store.release)
	firstErr := <-firstDone
	if !unserialized {
		secondErr = <-secondDone
	}
	if firstErr != nil || secondErr != nil {
		t.Fatalf("updates failed: %v, %v", firstErr, secondErr)
	}
	persisted, err := store.LoadPreferences()
	if err != nil {
		t.Fatal(err)
	}
	if persisted.Aria2BinaryPath != "/changed/aria2c" || persisted.DefaultDownloadDir != "/changed/downloads" {
		t.Fatalf("lost update: %+v", persisted)
	}
	if effective := repo.Effective(); effective.Aria2BinaryPath != "/changed/aria2c" || effective.DefaultDownloadDir != "/changed/downloads" {
		t.Fatalf("effective config lost an update: %+v", effective)
	}
}

type failingPreferenceStore struct {
	*Store
	err error
}

func (s *failingPreferenceStore) SaveConfig(prefs Config) error {
	if s.err != nil {
		return s.err
	}
	return s.Store.SaveConfig(prefs)
}

func TestFailedSaveDoesNotPublishPreferences(t *testing.T) {
	store := &failingPreferenceStore{Store: NewStoreAt(t.TempDir())}
	if err := store.SaveConfig(Config{PrivateToken: "original"}); err != nil {
		t.Fatal(err)
	}
	repo, err := NewRepository(store, Overrides{}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	before := repo.Effective()
	failure := errors.New("replacement failed")
	store.err = failure
	if err := repo.SavePrivateToken("unsaved"); !errors.Is(err, failure) {
		t.Fatalf("save error = %v", err)
	}
	if repo.Effective() != before {
		t.Fatal("failed save published effective config")
	}
	store.err = nil
	if err := repo.Update(func(prefs *Config) error {
		prefs.Aria2BinaryPath = "/changed/aria2c"
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	persisted, err := store.LoadPreferences()
	if err != nil || persisted.PrivateToken != "original" || persisted.Aria2BinaryPath != "/changed/aria2c" {
		t.Fatalf("failed preference leaked into later save: %+v, %v", persisted, err)
	}
}

func TestPreferenceUpdateRejectsInvalidOrAbortedChanges(t *testing.T) {
	store := NewStoreAt(t.TempDir())
	original := Config{DefaultDownloadDir: "/explicit/downloads"}
	if err := store.SaveConfig(original); err != nil {
		t.Fatal(err)
	}
	repo, err := NewRepository(store, Overrides{}, "")
	if err != nil {
		t.Fatal(err)
	}
	before := repo.Effective()
	abort := errors.New("change aborted")
	if err := repo.Update(func(prefs *Config) error {
		prefs.PrivateToken = "aborted"
		return abort
	}); !errors.Is(err, abort) {
		t.Fatalf("callback error = %v", err)
	}
	if err := repo.Update(func(prefs *Config) error {
		prefs.DefaultDownloadDir = ""
		return nil
	}); err == nil {
		t.Fatal("invalid default directory accepted")
	}
	persisted, err := store.LoadPreferences()
	if err != nil || persisted != original || repo.Effective() != before {
		t.Fatalf("rejected change was published: %+v, %v", persisted, err)
	}
}

func TestMissingHomeRequiresExplicitDownloadDir(t *testing.T) {
	if _, err := Resolve(Config{}, Overrides{}, ""); err == nil || !strings.Contains(err.Error(), "default_download_dir") {
		t.Fatalf("missing home needs actionable error: %v", err)
	}
	if _, err := NewRepository(NewStoreAt(t.TempDir()), Overrides{}, ""); err == nil {
		t.Fatal("repository accepted unavailable default home")
	}
	const explicit = "relative/../downloads"
	got, err := Resolve(Config{DefaultDownloadDir: explicit}, Overrides{}, "")
	if err != nil || got.DefaultDownloadDir != explicit {
		t.Fatalf("explicit directory changed: %+v, %v", got, err)
	}
}

func TestResolvePreferences(t *testing.T) {
	home := t.TempDir()
	for _, tc := range []struct {
		name              string
		prefs             Config
		overrides         Overrides
		backend           models.DownloadBackend
		token, api, oauth string
	}{
		{name: "empty", backend: models.DownloadBackendDirect, api: "https://api.real-debrid.com/rest/1.0", oauth: "https://api.real-debrid.com/oauth/v2"},
		{name: "invalid", prefs: Config{DownloadBackend: "invalid"}, backend: models.DownloadBackendDirect, api: "https://api.real-debrid.com/rest/1.0", oauth: "https://api.real-debrid.com/oauth/v2"},
		{name: "direct", prefs: Config{DownloadBackend: "direct", PrivateToken: "saved", APIBaseURL: "https://saved-api.example", OAuthBaseURL: "https://saved-oauth.example"}, backend: models.DownloadBackendDirect, token: "saved", api: "https://saved-api.example", oauth: "https://saved-oauth.example"},
		{name: "aria2 with overrides", prefs: Config{DownloadBackend: "aria2", PrivateToken: "saved", APIBaseURL: "https://saved-api.example", OAuthBaseURL: "https://saved-oauth.example"}, overrides: Overrides{PrivateToken: "env", APIBaseURL: "https://env-api.example", OAuthBaseURL: "https://env-oauth.example"}, backend: models.DownloadBackendAria2, token: "env", api: "https://env-api.example", oauth: "https://env-oauth.example"},
		{name: "partial overrides", prefs: Config{PrivateToken: "saved", APIBaseURL: "https://saved-api.example"}, overrides: Overrides{OAuthBaseURL: "https://env-oauth.example"}, backend: models.DownloadBackendDirect, token: "saved", api: "https://saved-api.example", oauth: "https://env-oauth.example"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Resolve(tc.prefs, tc.overrides, home)
			if err != nil {
				t.Fatal(err)
			}
			if got.DownloadBackend != tc.backend || got.PrivateToken != tc.token || got.APIBaseURL != tc.api || got.OAuthBaseURL != tc.oauth || got.DefaultDownloadDir != filepath.Join(home, "Downloads") {
				t.Fatalf("resolved config = %+v", got)
			}
		})
	}
}

func TestRepositoryRejectsUnreadablePreferences(t *testing.T) {
	store := NewStoreAt(t.TempDir())
	if err := os.WriteFile(store.ConfigPath(), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewRepository(store, Overrides{}, t.TempDir()); err == nil {
		t.Fatal("malformed preferences accepted")
	}
}
