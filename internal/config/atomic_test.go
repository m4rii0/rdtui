package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/m4rii0/rdtui/pkg/models"
)

func TestAtomicSavePreservesOldFileOnFailure(t *testing.T) {
	t.Parallel()
	for _, stage := range []string{"create", "write", "short write", "sync", "close", "replace"} {
		t.Run(stage, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "config.json")
			before := []byte("{\n  \"private_token\": \"old-token\"\n}\n")
			if err := os.WriteFile(path, before, 0o600); err != nil {
				t.Fatal(err)
			}
			failure := errors.New("injected " + stage + " failure")
			ops := failingAtomicOps(stage, failure)
			wantErr := failure
			if stage == "short write" {
				wantErr = io.ErrShortWrite
			}
			if err := atomicWriteWithOps(path, []byte(`{"private_token":"new-token"}`), 0o600, ops); !errors.Is(err, wantErr) {
				t.Fatalf("atomic write error = %v, want %v", err, wantErr)
			}
			after, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(after, before) {
				t.Fatalf("destination corrupted: got %q, want %q", after, before)
			}
			assertOnlyDestination(t, path)
		})
	}
}

func TestAtomicSaveCleansTemporaryFiles(t *testing.T) {
	t.Parallel()
	for _, stage := range []string{"create", "write", "short write", "sync", "close", "replace"} {
		t.Run(stage, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "auth.json")
			failure := errors.New("injected failure")
			if err := atomicWriteWithOps(path, []byte(`{"access_token":"new-token"}`), 0o600, failingAtomicOps(stage, failure)); err == nil {
				t.Fatal("expected save failure")
			}
			entries, err := os.ReadDir(filepath.Dir(path))
			if err != nil {
				t.Fatal(err)
			}
			if len(entries) != 0 {
				t.Fatalf("temporary file leak or failed save created destination: %v", entries)
			}
		})
	}
	t.Run("cleanup errors retain original cause", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join(t.TempDir(), "auth.json")
		writeErr, closeErr, removeErr := errors.New("write"), errors.New("close"), errors.New("remove")
		ops := failingAtomicOps("write", writeErr)
		ops.close = func(f *os.File) error {
			return errors.Join(f.Close(), closeErr)
		}
		var temporary string
		ops.remove = func(path string) error {
			temporary = path
			return removeErr
		}
		err := atomicWriteWithOps(path, []byte("new state"), 0o600, ops)
		for _, cause := range []error{writeErr, closeErr, removeErr} {
			if !errors.Is(err, cause) {
				t.Fatalf("save error = %v, missing cause %v", err, cause)
			}
		}
		if temporary == "" || temporary == path || filepath.Dir(temporary) != filepath.Dir(path) {
			t.Fatalf("cleanup targeted %q, want sibling temporary file", temporary)
		}
		if err := os.Remove(temporary); err != nil {
			t.Fatal(err)
		}
	})
}

func failingAtomicOps(stage string, failure error) atomicOps {
	ops := defaultAtomicOps
	switch stage {
	case "create":
		ops.createTemp = func(string, string) (*os.File, error) { return nil, failure }
	case "write":
		ops.write = func(f *os.File, data []byte) (int, error) {
			n, err := f.Write(data[:len(data)/2])
			return n, errors.Join(err, failure)
		}
	case "short write":
		ops.write = func(f *os.File, data []byte) (int, error) { return f.Write(data[:len(data)/2]) }
	case "sync":
		ops.sync = func(*os.File) error { return failure }
	case "close":
		ops.close = func(f *os.File) error { return errors.Join(f.Close(), failure) }
	case "replace":
		ops.replace = func(string, string) error { return failure }
	}
	return ops
}

func TestAtomicSaveInitialFile(t *testing.T) {
	t.Parallel()
	testStoreSaves(t, false)
}

func TestAtomicSaveReplacesExistingFile(t *testing.T) {
	t.Parallel()
	testStoreSaves(t, true)
}

func testStoreSaves(t *testing.T, existing bool) {
	t.Helper()
	for _, name := range []string{"config", "auth"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			store := NewStoreAt(filepath.Join(t.TempDir(), "state"))
			path, key := store.ConfigPath(), "private_token"
			save := func() error { return store.SaveConfig(Config{PrivateToken: "new-token"}) }
			if name == "auth" {
				path, key = store.AuthPath(), "access_token"
				save = func() error {
					return store.SaveDeviceCredentials(models.DeviceCredentials{AccessToken: "new-token"})
				}
			}
			if existing {
				if err := store.EnsureDir(); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(`{"old":"state"}`), 0o644); err != nil {
					t.Fatal(err)
				}
				if err := os.Chmod(path, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if err := save(); err != nil {
				t.Fatal(err)
			}
			assertSavedFile(t, path, key, "new-token")
			assertOnlyDestination(t, path)
		})
	}
}

func assertSavedFile(t *testing.T, path, key, token string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded[key] != token {
		t.Fatalf("stored %s = %v, want %q", key, decoded[key], token)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("permissions = %o, want 600", info.Mode().Perm())
	}
}

func assertOnlyDestination(t *testing.T, path string) {
	t.Helper()
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != filepath.Base(path) {
		t.Fatalf("temporary file leak: directory contains %v", entries)
	}
}
