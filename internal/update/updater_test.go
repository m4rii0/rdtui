package update

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestNormalizeReleasedVersion(t *testing.T) {
	got, err := NormalizeReleasedVersion("v1.2")
	if err != nil {
		t.Fatalf("NormalizeReleasedVersion returned error: %v", err)
	}
	if got != "v1.2.0" {
		t.Fatalf("NormalizeReleasedVersion = %q, want v1.2.0", got)
	}

	got, err = NormalizeReleasedVersion("v1.2.3+dirty")
	if err != nil {
		t.Fatalf("NormalizeReleasedVersion returned error for dirty build: %v", err)
	}
	if got != "v1.2.3" {
		t.Fatalf("NormalizeReleasedVersion dirty build = %q, want v1.2.3", got)
	}

	invalid := []string{"", "dev", "1.2.3", "v1.2.3-dirty", "v1.2.3+local"}
	for _, version := range invalid {
		if _, err := NormalizeReleasedVersion(version); err == nil {
			t.Fatalf("NormalizeReleasedVersion(%q) expected error", version)
		}
	}
}

func TestCheckDetectsUpdateAndAsset(t *testing.T) {
	server := newReleaseServer(t, []byte("asset"), false)
	defer server.Close()

	res, err := Check(context.Background(), CheckOptions{
		CurrentVersion: "v1.0.0",
		Platform:       Platform{OS: "linux", Arch: "amd64"},
		Client:         &Client{HTTPClient: server.Client(), BaseURL: server.URL},
	})
	if err != nil {
		t.Fatalf("Check returned error: %v", err)
	}
	if !res.UpdateAvailable {
		t.Fatal("expected update to be available")
	}
	if res.AssetName != "rdtui-linux-amd64" {
		t.Fatalf("AssetName = %q, want rdtui-linux-amd64", res.AssetName)
	}
}

func TestCheckNoUpdate(t *testing.T) {
	server := newReleaseServer(t, []byte("asset"), false)
	defer server.Close()

	res, err := Check(context.Background(), CheckOptions{
		CurrentVersion: "v1.2.0",
		Platform:       Platform{OS: "linux", Arch: "amd64"},
		Client:         &Client{HTTPClient: server.Client(), BaseURL: server.URL},
	})
	if err != nil {
		t.Fatalf("Check returned error: %v", err)
	}
	if res.UpdateAvailable {
		t.Fatal("expected no update")
	}
}

func TestUpdateDownloadsVerifiesAndWritesWindowsReplacement(t *testing.T) {
	asset := []byte("verified asset")
	server := newReleaseServer(t, asset, false)
	defer server.Close()

	dir := t.TempDir()
	target := filepath.Join(dir, "rdtui.exe")
	if err := os.WriteFile(target, []byte("old"), 0o755); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	res, err := Update(context.Background(), UpdateOptions{
		CurrentVersion: "v1.0.0",
		Platform:       Platform{OS: "windows", Arch: "amd64"},
		Client:         &Client{HTTPClient: server.Client(), BaseURL: server.URL},
		ExePath:        target,
		TempDir:        filepath.Join(dir, "replacement"),
	})
	if err != nil {
		t.Fatalf("Update returned error: %v", err)
	}
	if !res.WindowsManual {
		t.Fatal("expected Windows manual update result")
	}
	data, err := os.ReadFile(res.ReplacementPath)
	if err != nil {
		t.Fatalf("ReadFile replacement returned error: %v", err)
	}
	if string(data) != string(asset) {
		t.Fatalf("replacement = %q, want %q", string(data), string(asset))
	}
}

func TestUpdateFailsOnChecksumMismatch(t *testing.T) {
	server := newReleaseServer(t, []byte("asset"), true)
	defer server.Close()

	dir := t.TempDir()
	target := filepath.Join(dir, "rdtui.exe")
	if err := os.WriteFile(target, []byte("old"), 0o755); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	_, err := Update(context.Background(), UpdateOptions{
		CurrentVersion: "v1.0.0",
		Platform:       Platform{OS: "windows", Arch: "amd64"},
		Client:         &Client{HTTPClient: server.Client(), BaseURL: server.URL},
		ExePath:        target,
		TempDir:        filepath.Join(dir, "replacement"),
	})
	if err == nil {
		t.Fatal("expected checksum mismatch error")
	}
}

func newReleaseServer(t *testing.T, asset []byte, badChecksum bool) *httptest.Server {
	t.Helper()
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/m4rii0/rdtui/releases/latest":
			fmt.Fprintf(w, `{
				"tag_name":"v1.2.0",
				"assets":[
					{"name":"rdtui-linux-amd64","browser_download_url":"%s/rdtui-linux-amd64"},
					{"name":"rdtui-windows-amd64.exe","browser_download_url":"%s/rdtui-windows-amd64.exe"},
					{"name":"checksums.txt","browser_download_url":"%s/checksums.txt"}
				]
			}`, server.URL, server.URL, server.URL)
		case "/rdtui-linux-amd64", "/rdtui-windows-amd64.exe":
			_, _ = w.Write(asset)
		case "/checksums.txt":
			digest := fmt.Sprintf("%x", sha256.Sum256(asset))
			if badChecksum {
				digest = fmt.Sprintf("%x", sha256.Sum256([]byte("different")))
			}
			fmt.Fprintf(w, "%s  rdtui-linux-amd64\n", digest)
			fmt.Fprintf(w, "%s  rdtui-windows-amd64.exe\n", digest)
		default:
			http.NotFound(w, r)
		}
	}))
	return server
}
