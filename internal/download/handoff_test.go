package download

import (
	"testing"
)

func TestFilenameForURLUsesFallback(t *testing.T) {
	if got := FilenameForURL("https://example.com/file.zip", "movie.mkv"); got != "movie.mkv" {
		t.Fatalf("FilenameForURL() = %q, want movie.mkv", got)
	}
}

func TestFilenameForURLUsesPathBase(t *testing.T) {
	if got := FilenameForURL("/tmp/downloads/file.zip", ""); got != "file.zip" {
		t.Fatalf("FilenameForURL() = %q, want file.zip", got)
	}
	if got := FilenameForURL("/", ""); got != "download" {
		t.Fatalf("FilenameForURL() = %q, want download", got)
	}
}
