package update

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestInstallUnixSuccess(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix executable script test")
	}
	dir := t.TempDir()
	target := filepath.Join(dir, "rdtui")
	writeScript(t, target, "rdtui v1.0.0")

	res, err := Install(context.Background(), scriptBytes("rdtui v1.2.0"), InstallOptions{
		GOOS:    "linux",
		ExePath: target,
		Version: "v1.2.0",
	})
	if err != nil {
		t.Fatalf("Install returned error: %v", err)
	}
	if !res.Installed {
		t.Fatal("expected install result to report Installed")
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	if !strings.Contains(string(data), "rdtui v1.2.0") {
		t.Fatalf("target was not replaced: %s", string(data))
	}
}

func TestInstallUnixValidationFailureRollsBack(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix executable script test")
	}
	dir := t.TempDir()
	target := filepath.Join(dir, "rdtui")
	writeScript(t, target, "rdtui v1.0.0")

	_, err := Install(context.Background(), scriptBytes("rdtui wrong"), InstallOptions{
		GOOS:    "linux",
		ExePath: target,
		Version: "v1.2.0",
	})
	if err == nil {
		t.Fatal("expected validation error")
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	if !strings.Contains(string(data), "rdtui v1.0.0") {
		t.Fatalf("target was not rolled back: %s", string(data))
	}
}

func TestInstallInvalidTarget(t *testing.T) {
	_, err := Install(context.Background(), []byte("asset"), InstallOptions{GOOS: "linux", ExePath: filepath.Join(t.TempDir(), "missing")})
	if err == nil {
		t.Fatal("expected invalid target error")
	}
}

func TestInstallWindowsWritesManualReplacement(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "rdtui.exe")
	if err := os.WriteFile(target, []byte("old"), 0o755); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	res, err := Install(context.Background(), []byte("new"), InstallOptions{
		GOOS:    "windows",
		ExePath: target,
		TempDir: filepath.Join(dir, "replacement"),
		Version: "v1.2.0",
	})
	if err != nil {
		t.Fatalf("Install returned error: %v", err)
	}
	if !res.WindowsManual {
		t.Fatal("expected Windows manual result")
	}
	replacement, err := os.ReadFile(res.ReplacementPath)
	if err != nil {
		t.Fatalf("ReadFile replacement returned error: %v", err)
	}
	if string(replacement) != "new" {
		t.Fatalf("replacement = %q, want new", string(replacement))
	}
	current, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("ReadFile target returned error: %v", err)
	}
	if string(current) != "old" {
		t.Fatalf("target changed on Windows fallback: %q", string(current))
	}
}

func writeScript(t *testing.T, path, output string) {
	t.Helper()
	if err := os.WriteFile(path, scriptBytes(output), 0o755); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}
}

func scriptBytes(output string) []byte {
	return []byte("#!/bin/sh\nprintf '%s\\n' '" + output + "'\n")
}
