package update

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLatestReleaseParsesResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/m4rii0/rdtui/releases/latest" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"tag_name":"v1.2.0",
			"html_url":"https://github.com/m4rii0/rdtui/releases/tag/v1.2.0",
			"assets":[{"name":"rdtui-linux-amd64","browser_download_url":"https://example.test/asset","size":12,"state":"uploaded"}]
		}`))
	}))
	defer server.Close()

	client := &Client{HTTPClient: server.Client(), BaseURL: server.URL}
	release, err := client.LatestRelease(context.Background())
	if err != nil {
		t.Fatalf("LatestRelease returned error: %v", err)
	}
	if release.TagName != "v1.2.0" {
		t.Fatalf("TagName = %q, want v1.2.0", release.TagName)
	}
	if len(release.Assets) != 1 || release.Assets[0].Name != "rdtui-linux-amd64" {
		t.Fatalf("unexpected assets: %+v", release.Assets)
	}
}

func TestDownloadReturnsBytes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("asset"))
	}))
	defer server.Close()

	client := &Client{HTTPClient: server.Client()}
	data, err := client.Download(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("Download returned error: %v", err)
	}
	if string(data) != "asset" {
		t.Fatalf("Download = %q, want asset", string(data))
	}
}
