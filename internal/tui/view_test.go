package tui

import (
	"fmt"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/mario/real-debrid/pkg/models"
)

func TestTorrentListWindowKeepsSelectionVisible(t *testing.T) {
	start, end := torrentListWindow(100, 50, 10)
	if start > 50 || end <= 50 {
		t.Fatalf("selection not visible: start=%d end=%d", start, end)
	}
	if end-start != 10 {
		t.Fatalf("window size = %d, want 10", end-start)
	}
}

func TestTorrentListWindowClampsToStart(t *testing.T) {
	start, end := torrentListWindow(20, 1, 8)
	if start != 0 || end != 8 {
		t.Fatalf("window = (%d,%d), want (0,8)", start, end)
	}
}

func TestTorrentListWindowClampsToEnd(t *testing.T) {
	start, end := torrentListWindow(20, 19, 8)
	if start != 12 || end != 20 {
		t.Fatalf("window = (%d,%d), want (12,20)", start, end)
	}
}

func TestScrollbarThumbFitsVisibleTrack(t *testing.T) {
	top, size := scrollbarThumb(100, 10, 45)
	if size < 1 || size > 10 {
		t.Fatalf("thumb size = %d, want 1..10", size)
	}
	if top < 0 || top > 10-size {
		t.Fatalf("thumb top = %d, out of bounds for size %d", top, size)
	}
}

func TestRenderViewHeightWithManyTorrents(t *testing.T) {
	m := Model{
		mode:        modeMain,
		width:       120,
		height:      24,
		selectedIdx: 35,
		session:     &models.AuthSession{User: models.User{Username: "debug"}},
		detail:      &models.TorrentInfo{Torrent: models.Torrent{Filename: "selected torrent", Status: "downloading", Progress: 99.67}, Files: []models.TorrentFile{{ID: 1, Path: "/file.mkv", Bytes: 1024}}, OriginalBytes: 1024},
	}
	for i := 0; i < 80; i++ {
		m.torrents = append(m.torrents, models.Torrent{
			ID:       fmt.Sprintf("id-%d", i),
			Filename: fmt.Sprintf("torrent-%02d-with-a-rather-long-name-to-force-layout-behavior", i),
			Status:   "downloading",
			Progress: float64(i) + 0.67,
			Added:    time.Unix(0, 0),
		})
	}

	view := renderView(m)
	height := lipgloss.Height(view)
	t.Logf("rendered height=%d", height)
	if height > m.height {
		t.Fatalf("rendered height = %d, want <= %d\n%s", height, m.height, view)
	}
}
