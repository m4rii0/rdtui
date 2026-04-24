package update

import "testing"

func TestPlatformAssetName(t *testing.T) {
	tests := []struct {
		platform Platform
		want     string
	}{
		{Platform{OS: "darwin", Arch: "amd64"}, "rdtui-darwin-amd64"},
		{Platform{OS: "darwin", Arch: "arm64"}, "rdtui-darwin-arm64"},
		{Platform{OS: "linux", Arch: "amd64"}, "rdtui-linux-amd64"},
		{Platform{OS: "linux", Arch: "arm64"}, "rdtui-linux-arm64"},
		{Platform{OS: "windows", Arch: "amd64"}, "rdtui-windows-amd64.exe"},
		{Platform{OS: "windows", Arch: "arm64"}, "rdtui-windows-arm64.exe"},
	}

	for _, tt := range tests {
		got, err := tt.platform.AssetName()
		if err != nil {
			t.Fatalf("AssetName(%+v) returned error: %v", tt.platform, err)
		}
		if got != tt.want {
			t.Fatalf("AssetName(%+v) = %q, want %q", tt.platform, got, tt.want)
		}
	}
}

func TestPlatformAssetNameUnsupported(t *testing.T) {
	if _, err := (Platform{OS: "freebsd", Arch: "amd64"}).AssetName(); err == nil {
		t.Fatal("expected unsupported platform error")
	}
}

func TestSelectAssetMissing(t *testing.T) {
	release := Release{TagName: "v1.2.0", Assets: []Asset{{Name: "checksums.txt", BrowserDownloadURL: "https://example.test/checksums.txt"}}}
	_, expected, err := SelectAsset(release, Platform{OS: "linux", Arch: "amd64"})
	if err == nil {
		t.Fatal("expected missing asset error")
	}
	if expected != "rdtui-linux-amd64" {
		t.Fatalf("expected asset name = %q, want rdtui-linux-amd64", expected)
	}
}
