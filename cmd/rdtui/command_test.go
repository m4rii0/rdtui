package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestCobraCommandRunTUI(t *testing.T) {
	var called bool
	cmd := testRootCommand(commandHandlers{
		runTUI: func() error {
			called = true
			return nil
		},
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !called {
		t.Fatal("expected root command to run TUI")
	}
}

func TestCobraCommandVersion(t *testing.T) {
	var stdout bytes.Buffer
	cmd := testRootCommand(commandHandlers{
		runTUI: func() error {
			t.Fatal("version command should not run TUI")
			return nil
		},
	})
	cmd.SetOut(&stdout)
	cmd.SetArgs([]string{"--version"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !strings.HasPrefix(stdout.String(), "rdtui ") {
		t.Fatalf("version output = %q", stdout.String())
	}
}

func TestCobraCommandCheckUpdate(t *testing.T) {
	var called bool
	cmd := testRootCommand(commandHandlers{
		checkUpdate: func(ctx context.Context, out io.Writer) error {
			called = true
			return nil
		},
	})
	cmd.SetArgs([]string{"check-update"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !called {
		t.Fatal("expected check-update handler to run")
	}
}

func TestCobraCommandUpdateOptions(t *testing.T) {
	var called bool
	var got updateOptions
	cmd := testRootCommand(commandHandlers{
		update: func(ctx context.Context, out io.Writer, opts updateOptions) error {
			called = true
			got = opts
			return nil
		},
	})
	cmd.SetArgs([]string{"update", "--dry-run", "--dry-run"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !called {
		t.Fatal("expected update handler to run")
	}
	if !got.DryRun {
		t.Fatal("expected dry-run option to be true")
	}
}

func TestCobraCommandInvalidArguments(t *testing.T) {
	tests := [][]string{
		{"--version", "extra"},
		{"check-update", "--dry-run"},
		{"update", "--unknown"},
		{"update", "extra"},
		{"unknown"},
	}

	for _, args := range tests {
		cmd := testRootCommand(commandHandlers{})
		cmd.SetArgs(args)
		err := cmd.Execute()
		if err == nil {
			t.Fatalf("Execute(%v) expected error", args)
		}
		if !isUsageError(err) && !errors.Is(err, errUsage) {
			t.Fatalf("Execute(%v) error = %v, want usage error", args, err)
		}
	}
}

func TestRunReturnsUsageExitCode(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"update", "--unknown"}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("run exit code = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "Usage:") {
		t.Fatalf("stderr should include usage, got %q", stderr.String())
	}
}

func testRootCommand(handlers commandHandlers) *cobra.Command {
	var stdout bytes.Buffer
	if handlers.runTUI == nil {
		handlers.runTUI = func() error { return nil }
	}
	if handlers.checkUpdate == nil {
		handlers.checkUpdate = func(context.Context, io.Writer) error { return nil }
	}
	if handlers.update == nil {
		handlers.update = func(context.Context, io.Writer, updateOptions) error { return nil }
	}
	cmd := newRootCommand(context.Background(), &stdout, handlers)
	cmd.SetOut(&stdout)
	cmd.SetErr(&bytes.Buffer{})
	return cmd
}
