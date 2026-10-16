package app

import (
	"context"

	"github.com/m4rii0/rdtui/pkg/models"
)

type TorrentAPI interface {
	ListTorrents(context.Context, int) ([]models.Torrent, error)
	TorrentInfo(context.Context, string) (models.TorrentInfo, error)
	AddMagnet(context.Context, string) (models.AddTorrentResult, error)
	AddTorrentURL(context.Context, string) (models.AddTorrentResult, error)
	SelectFiles(context.Context, string, []int) error
	DeleteTorrent(context.Context, string) error
}

type TorrentService struct{ api TorrentAPI }

func NewTorrentService(api TorrentAPI) *TorrentService { return &TorrentService{api: api} }

func (s *TorrentService) ListTorrents(ctx context.Context) ([]models.Torrent, error) {
	return s.api.ListTorrents(ctx, 100)
}
func (s *TorrentService) TorrentInfo(ctx context.Context, id string) (models.TorrentInfo, error) {
	return s.api.TorrentInfo(ctx, id)
}
func (s *TorrentService) AddMagnet(ctx context.Context, magnet string) (models.AddTorrentResult, error) {
	return s.api.AddMagnet(ctx, magnet)
}
func (s *TorrentService) AddTorrentURL(ctx context.Context, url string) (models.AddTorrentResult, error) {
	return s.api.AddTorrentURL(ctx, url)
}
func (s *TorrentService) SelectFiles(ctx context.Context, id string, ids []int) error {
	return s.api.SelectFiles(ctx, id, ids)
}
func (s *TorrentService) DeleteTorrent(ctx context.Context, id string) error {
	return s.api.DeleteTorrent(ctx, id)
}
