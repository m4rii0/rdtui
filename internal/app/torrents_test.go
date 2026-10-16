package app

import (
	"context"
	"testing"

	"github.com/m4rii0/rdtui/pkg/models"
)

type torrentListAPI struct {
	TorrentAPI
	observedListLimit int
}

func (a *torrentListAPI) ListTorrents(_ context.Context, limit int) ([]models.Torrent, error) {
	a.observedListLimit = limit
	return []models.Torrent{{ID: "existing"}}, nil
}

func TestTorrentServiceUsesExistingLimit(t *testing.T) {
	api := &torrentListAPI{}
	items, err := NewTorrentService(api).ListTorrents(t.Context())
	if err != nil || len(items) != 1 || items[0].ID != "existing" {
		t.Fatalf("items=%v err=%v", items, err)
	}
	if api.observedListLimit != 100 {
		t.Fatal(api.observedListLimit)
	}
}
