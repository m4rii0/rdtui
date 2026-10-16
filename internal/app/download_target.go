package app

import (
	"cmp"
	"context"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/m4rii0/rdtui/internal/download"
	"github.com/m4rii0/rdtui/pkg/models"
)

type PreparedDownload struct {
	URL, Filename, Path        string
	RemoteBytes, ExistingBytes int64
	Exists                     bool
}

// PrepareDownload inspects exactly the destination that Start will submit to
// either backend. Both single and queued flows can share this preparation.
func PrepareDownload(ctx context.Context, target models.DownloadTarget, dir string, resolve func(context.Context, models.DownloadTarget) (models.UnrestrictedLink, error), stat func(string) (int64, bool, error)) (PreparedDownload, error) {
	if err := ctx.Err(); err != nil {
		return PreparedDownload{}, err
	}
	link, err := resolve(ctx, target)
	if err != nil {
		return PreparedDownload{}, err
	}
	if err := ctx.Err(); err != nil {
		return PreparedDownload{}, err
	}
	p := PreparedDownload{URL: cmp.Or(link.Download, link.Link), RemoteBytes: link.Filesize}
	p.Filename = downloadFilename(p.URL, cmp.Or(link.Filename, target.Label))
	p.Path = filepath.Join(dir, p.Filename)
	p.ExistingBytes, p.Exists, err = stat(p.Path)
	return p, err
}

func downloadFilename(rawURL, filename string) string {
	if filename == "" {
		if parsed, err := url.Parse(rawURL); err == nil {
			rawURL = parsed.Path
		}
		filename = download.FilenameForURL(rawURL, "")
	}
	filename = filepath.Base(strings.ReplaceAll(filename, `\`, "/"))
	if filename == "." || filename == ".." || filename == "/" || filename == "" {
		return "download"
	}
	return filename
}
