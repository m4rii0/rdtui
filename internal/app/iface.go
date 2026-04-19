package app

import (
	"context"

	"github.com/m4rii0/rdtui/internal/config"
	"github.com/m4rii0/rdtui/pkg/models"
)

// AppService defines methods the TUI layer uses. Both the real Service and
// the demo Service implement this interface.
type AppService interface {
	Config() config.Config
	Bootstrap(ctx context.Context) (*models.AuthSession, error)
	AuthenticateWithToken(ctx context.Context, token string, persist bool) (*models.AuthSession, error)
	StartDeviceFlow(ctx context.Context) (models.DeviceCode, error)
	CompleteDeviceFlow(ctx context.Context, code models.DeviceCode) (*models.AuthSession, error)

	ListTorrents(ctx context.Context) ([]models.Torrent, error)
	TorrentInfo(ctx context.Context, id string) (models.TorrentInfo, error)
	AddMagnet(ctx context.Context, magnet string) (models.AddTorrentResult, error)
	AddTorrentURL(ctx context.Context, remoteURL string) (models.AddTorrentResult, error)
	ImportTorrentFiles(ctx context.Context, paths []string) []models.ImportResult
	SelectFiles(ctx context.Context, torrentID string, ids []int) error
	DeleteTorrent(ctx context.Context, torrentID string) error

	ResolveDirectURL(ctx context.Context, target models.DownloadTarget) (models.UnrestrictedLink, error)
	CopyURL(url string) (bool, error)
	StartManagedDownload(ctx context.Context, url, filename string) (models.ManagedDownloadStart, error)
	ManagedDownloadStatus(ctx context.Context) (models.ManagedDownload, bool, error)

	SavePrivateToken(token string) error
	OpenFile(path string) error
	RevealInDirectory(path string) error
	Close() error
}
