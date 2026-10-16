package app

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/m4rii0/rdtui/pkg/models"
)

func TestPrepareDownloadUsesBackendFilename(t *testing.T) {
	for _, tc := range []struct{ name, remote, label, url, want string }{
		{"remote path", "../../folder/movie.mkv", "ignored", "https://example.com/other", "movie.mkv"},
		{"windows path", `folder\movie.mkv`, "ignored", "https://example.com/other", "movie.mkv"},
		{"label fallback", "", "folder/label.mkv", "https://example.com/other", "label.mkv"},
		{"URL fallback", "", "", "https://example.com/folder/url.mkv?token=secret", "url.mkv"},
		{"empty URL path", "", "", "https://example.com/", "download"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := newQueueRig(t, false)
			dir := t.TempDir()
			r.c.dir = dir
			target := models.DownloadTarget{Label: tc.label, Link: "restricted"}
			p, err := PrepareDownload(t.Context(), target, dir, func(ctx context.Context, got models.DownloadTarget) (models.UnrestrictedLink, error) {
				if got != target || ctx != t.Context() {
					t.Fatal(got)
				}
				return models.UnrestrictedLink{Filename: tc.remote, Download: tc.url, Filesize: 20}, nil
			}, func(path string) (int64, bool, error) {
				if path != filepath.Join(dir, tc.want) {
					t.Fatalf("stat path=%q", path)
				}
				return 10, true, nil
			})
			if err != nil || p.Filename != tc.want || p.Path != filepath.Join(dir, tc.want) || !p.Exists || p.ExistingBytes != 10 || p.RemoteBytes != 20 {
				t.Fatal(p, err)
			}
			results := make(chan startResult, 1)
			go func() { s, e := r.c.Start(t.Context(), p.URL, p.Filename); results <- startResult{s, e} }()
			call := receive(t, r.b.starts)
			if call.req.Filename != tc.want || filepath.Join(call.req.Dir, call.req.Filename) != p.Path {
				t.Fatal(call.req, p)
			}
			call.reply <- backendResult{download: models.ManagedDownload{GID: "terminal", Status: models.ManagedDownloadStatusComplete}}
			if result := receive(t, results); result.err != nil {
				t.Fatal(result)
			}
		})
	}
}

func TestPrepareDownloadErrors(t *testing.T) {
	want := errors.New("preparation failed")
	_, err := PrepareDownload(t.Context(), models.DownloadTarget{}, "", func(context.Context, models.DownloadTarget) (models.UnrestrictedLink, error) {
		return models.UnrestrictedLink{}, want
	}, func(string) (int64, bool, error) { t.Fatal("stat after failed resolution"); return 0, false, nil })
	if !errors.Is(err, want) {
		t.Fatal(err)
	}
	_, err = PrepareDownload(t.Context(), models.DownloadTarget{}, "", func(context.Context, models.DownloadTarget) (models.UnrestrictedLink, error) {
		return models.UnrestrictedLink{Link: "https://example.com/file"}, nil
	}, func(string) (int64, bool, error) { return 0, false, want })
	if !errors.Is(err, want) {
		t.Fatal(err)
	}
}
