package tui

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/m4rii0/rdtui/internal/app"
	"github.com/m4rii0/rdtui/pkg/models"
)

type testBatchAPI interface {
	DeleteTorrent(context.Context, string) error
	SelectFiles(context.Context, string, []int) error
	ResolveDirectURL(context.Context, models.DownloadTarget) (models.UnrestrictedLink, error)
	ImportTorrentFiles(context.Context, []string) []models.ImportResult
}

type testBatchAdapter struct{ testBatchAPI }

func (s testBatchAdapter) UnrestrictLink(ctx context.Context, link string) (models.UnrestrictedLink, error) {
	return s.ResolveDirectURL(ctx, models.DownloadTarget{Link: link})
}
func (s testBatchAdapter) AddTorrentPath(ctx context.Context, path string) (models.AddTorrentResult, error) {
	results := s.ImportTorrentFiles(ctx, []string{path})
	if len(results) != 1 {
		return models.AddTorrentResult{}, errors.New("import returned no result")
	}
	return models.AddTorrentResult{ID: results[0].TorrentID}, results[0].Err
}

func testBatches(service interface {
	testBatchAPI
	DesktopService
}) *app.BatchService {
	return app.NewBatchService(testBatchAdapter{service}, service, nil)
}

func testFileSize(path string) (int64, bool, error) {
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return info.Size(), true, nil
}

func asyncDependencies(t *testing.T, s *asyncService) Dependencies {
	t.Helper()
	return Dependencies{Auth: s, Torrents: s, Downloads: s, Runtime: newTestRuntime(t), PrepareDownload: func(ctx context.Context, target models.DownloadTarget) (app.PreparedDownload, error) {
		return app.PrepareDownload(ctx, target, "/downloads", s.ResolveDirectURL, func(string) (int64, bool, error) { return 0, false, nil })
	}}
}
