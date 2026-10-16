package download_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/m4rii0/rdtui/internal/aria2"
	"github.com/m4rii0/rdtui/internal/directdl"
	"github.com/m4rii0/rdtui/internal/download"
	"github.com/m4rii0/rdtui/pkg/models"
)

// Closing an unused backend must close admission, not merely stop current work.
func TestStartAfterShutdown(t *testing.T) {
	for _, tc := range []struct {
		name       string
		newBackend func() download.Backend
	}{
		{"direct", func() download.Backend { return directdl.NewManager() }},
		{"aria2", func() download.Backend { return aria2.NewManager("definitely-missing-aria2c-binary") }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			backend := tc.newBackend()
			if err := backend.Shutdown(t.Context()); err != nil {
				t.Fatal(err)
			}
			_, err := backend.StartDownload(t.Context(), models.ManagedDownloadRequest{Dir: t.TempDir(), Filename: "file"})
			if !errors.Is(err, download.ErrClosed) {
				t.Fatalf("late start: %v", err)
			}
		})
	}
}

func TestCanceledStartNotAdmitted(t *testing.T) {
	for _, tc := range []struct {
		name    string
		backend download.Backend
	}{
		{"direct", directdl.NewManager()},
		{"aria2", aria2.NewManager("definitely-missing-aria2c-binary")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			cancel()
			dir := filepath.Join(t.TempDir(), "not-created")
			_, err := tc.backend.StartDownload(ctx, models.ManagedDownloadRequest{Dir: dir, Filename: "file"})
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("canceled start: %v", err)
			}
			if _, err := os.Stat(dir); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("canceled start created directory: %v", err)
			}
			if err := tc.backend.Shutdown(t.Context()); err != nil {
				t.Fatal(err)
			}
		})
	}
}
