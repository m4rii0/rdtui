package app

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/m4rii0/rdtui/internal/auth"
	"github.com/m4rii0/rdtui/pkg/models"
)

// Authentication exposes display snapshots, never authoritative credentials.
// The session manager remains the only token owner and request-time provider.
type Authentication struct{ manager *auth.Manager }

func (a *Authentication) Snapshot() models.AuthSession { return a.manager.Snapshot() }

func (a *Authentication) Bootstrap(ctx context.Context) (*models.AuthSession, error) {
	if _, err := a.manager.Bootstrap(ctx); err != nil {
		return nil, err
	}
	return new(a.Snapshot()), nil
}

func (a *Authentication) AuthenticateWithToken(ctx context.Context, token string, persist bool) (*models.AuthSession, error) {
	if _, err := a.manager.AuthenticateWithToken(ctx, token, persist); err != nil {
		return nil, err
	}
	return new(a.Snapshot()), nil
}

func (a *Authentication) StartDeviceFlow(ctx context.Context) (models.DeviceCode, error) {
	return a.manager.StartDeviceFlow(ctx)
}

func (a *Authentication) CompleteDeviceFlow(ctx context.Context, code models.DeviceCode) (*models.AuthSession, error) {
	if _, err := a.manager.CompleteDeviceFlow(ctx, code); err != nil {
		return nil, err
	}
	return new(a.Snapshot()), nil
}

func DefaultFileSelection(info models.TorrentInfo) []int {
	if len(info.Files) == 0 {
		return nil
	}
	largest := info.Files[0]
	for _, file := range info.Files[1:] {
		if file.Bytes > largest.Bytes {
			largest = file
		}
	}
	return []int{largest.ID}
}

func DownloadTargets(info models.TorrentInfo) []models.DownloadTarget {
	selectedFiles := make([]models.TorrentFile, 0)
	for _, file := range info.Files {
		if file.Selected {
			selectedFiles = append(selectedFiles, file)
		}
	}
	if len(selectedFiles) == 0 {
		selectedFiles = append(selectedFiles, info.Files...)
	}
	out := make([]models.DownloadTarget, 0, len(info.Links))
	for idx, link := range info.Links {
		label := fmt.Sprintf("Link %d", idx+1)
		filePath := ""
		if idx < len(selectedFiles) {
			filePath = selectedFiles[idx].Path
			label = filepath.Base(filePath)
			if label == "." || label == "/" || label == "" {
				label = filePath
			}
		}
		out = append(out, models.DownloadTarget{Index: idx, Link: link, Label: label, FilePath: filePath})
	}
	return out
}

func ValidTorrentFiles(paths []string) (valid []string, invalid []string) {
	seen := map[string]struct{}{}
	for _, path := range paths {
		clean := filepath.Clean(path)
		if _, ok := seen[clean]; ok {
			continue
		}
		seen[clean] = struct{}{}
		if strings.EqualFold(filepath.Ext(clean), ".torrent") {
			valid = append(valid, clean)
		} else {
			invalid = append(invalid, clean)
		}
	}
	slices.Sort(valid)
	slices.Sort(invalid)
	return valid, invalid
}
