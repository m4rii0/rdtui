package tui

import (
	"context"

	"github.com/m4rii0/rdtui/internal/app"
	"github.com/m4rii0/rdtui/pkg/models"
)

// Dependencies exposes workflows, not persistence or application shutdown.
type Dependencies struct {
	Auth            AuthService
	Torrents        TorrentService
	Downloads       DownloadService
	Batches         BatchService
	Bulk            BulkQueueService
	Desktop         DesktopService
	Runtime         *app.Runtime
	PrepareDownload func(context.Context, models.DownloadTarget) (app.PreparedDownload, error)
}
