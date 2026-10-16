package main

import (
	"bufio"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/m4rii0/rdtui/internal/app"
	"github.com/m4rii0/rdtui/internal/config"
	"github.com/m4rii0/rdtui/internal/download"
)

// Only isolated PATH fixtures are run: never the user's clipboard or desktop.
func gatedDesktopHelper(t *testing.T, name string) (entered func() int, release func()) {
	t.Helper()
	dir := t.TempDir()
	enteredPath, releasePath := filepath.Join(dir, "entered"), filepath.Join(dir, "release")
	for _, path := range []string{enteredPath, releasePath} {
		if err := syscall.Mkfifo(path, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	openFIFO := func(path string) *os.File {
		t.Helper()
		file, err := os.OpenFile(path, os.O_RDWR, 0)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = file.Close() })
		return file
	}
	entry, gate := openFIFO(enteredPath), openFIFO(releasePath)
	script := "#!/bin/sh\nprintf '%s\\n' \"$$\" > \"$DESKTOP_ENTERED\"\nread ignored < \"$DESKTOP_RELEASE\"\n"
	if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	t.Setenv("DESKTOP_ENTERED", enteredPath)
	t.Setenv("DESKTOP_RELEASE", releasePath)
	return func() int {
		t.Helper()
		if err := entry.SetReadDeadline(time.Now().Add(3 * time.Second)); err != nil {
			t.Fatal(err)
		}
		line, err := bufio.NewReader(entry).ReadString('\n')
		if err != nil {
			t.Fatal(err)
		}
		pid, err := strconv.Atoi(strings.TrimSpace(line))
		if err != nil {
			t.Fatal(err)
		}
		return pid
	}, func() { _, _ = gate.WriteString("release\n") }
}

func TestProductionDesktopStopsAndReapsWithApplication(t *testing.T) {
	for _, action := range []string{"copy", "open", "reveal"} {
		t.Run(action, func(t *testing.T) {
			name := "xdg-open"
			if action == "copy" {
				name = "wl-copy"
			}
			entered, release := gatedDesktopHelper(t, name)
			options, err := productionOptions()
			if err != nil {
				t.Fatal(err)
			}
			options.Store, options.Home = config.NewStoreAt(t.TempDir()), t.TempDir()
			application, err := app.NewApplication(t.Context(), options)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				release() // also drains the deliberately broken implementation on RED
				cleanup, cancel := context.WithTimeout(context.Background(), time.Second)
				defer cancel()
				if err := application.Close(cleanup); err != nil {
					t.Error(err)
				}
			})
			result := make(chan error, 1)
			path := filepath.Join(t.TempDir(), "fixture")
			if err := os.WriteFile(path, nil, 0o600); err != nil {
				t.Fatal(err)
			}
			if err := application.Runtime.Spawn(t.Context(), func(context.Context) {
				var err error
				switch action {
				case "copy":
					_, err = application.Desktop.CopyURL("https://example.invalid/fixture")
				case "open":
					err = application.Desktop.OpenFile(path)
				case "reveal":
					err = application.Desktop.RevealInDirectory(path)
				}
				result <- err
			}); err != nil {
				t.Fatal(err)
			}
			pid := entered()
			cleanup, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			if err := application.Close(cleanup); err != nil {
				t.Fatalf("owned helper ignored root cancellation: %v", err)
			}
			if err := <-result; !errors.Is(err, context.Canceled) {
				t.Fatalf("lost cancellation: %v", err)
			}
			if err := syscall.Kill(pid, 0); !errors.Is(err, syscall.ESRCH) {
				t.Fatalf("owned helper not reaped: %v", err)
			}
		})
	}
}

func TestLegacyDesktopHelperHasFiniteBudget(t *testing.T) {
	entered, release := gatedDesktopHelper(t, "wl-copy")
	result := make(chan error, 1)
	t.Cleanup(func() { release(); <-result })
	go func() { _, err := download.CopyURL("https://example.invalid/fixture"); result <- err }()
	pid := entered()
	guard, cancel := context.WithTimeout(t.Context(), 7*time.Second)
	defer cancel()
	select {
	case err := <-result:
		result <- err // cleanup joins both the RED and GREEN path
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("lost helper timeout: %v", err)
		}
		if err := syscall.Kill(pid, 0); !errors.Is(err, syscall.ESRCH) {
			t.Fatalf("timed-out helper not reaped: %v", err)
		}
	case <-guard.Done():
		t.Fatal("legacy clipboard helper has no finite budget")
	}
}
