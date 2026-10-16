package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/m4rii0/rdtui/internal/app"
	"github.com/m4rii0/rdtui/pkg/models"
)

func TestUXResizeAndNavigationPreservesWork(t *testing.T) {
	m := NewModel(Dependencies{Runtime: newTestRuntime(t)})
	m.mode = modeMain
	m.torrents = []models.Torrent{{ID: "a", Filename: "other"}, {ID: "b", Filename: "keep"}}
	m.searchInput.SetValue("keep")
	m.filterApplied = true
	m.applyFilter()
	m.batchMode = true
	m.batchSelected["b"] = true
	m.downloadRef = app.DownloadRef{Generation: 3, GID: "transfer"}
	m.download = &models.ManagedDownload{GID: "transfer", Filename: "download.mkv", Status: models.ManagedDownloadStatusActive}
	m.bulk = &bulkDownloadState{QueueID: 7, Snapshot: app.QueueSnapshot{ID: 7, CurrentIndex: 0, Download: &app.DownloadSnapshot{Ref: m.downloadRef, Download: *m.download}, Items: []app.QueueItemSnapshot{{Item: app.QueueItem{TorrentID: "b", Target: models.DownloadTarget{Label: "download.mkv"}}, Status: app.QueueItemActive}}}}
	m.bulk.renderSnapshot()
	ref := m.downloadRef
	for _, size := range [][2]int{{120, 30}, {40, 12}, {80, 24}} {
		next, _ := m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		m = next.(Model)
		view := renderView(m)
		if lipgloss.Width(view) > size[0] || lipgloss.Height(view) > size[1] {
			t.Fatalf("resize overflow %v: %dx%d", size, lipgloss.Width(view), lipgloss.Height(view))
		}
		m, _ = uxPress(m, tea.KeyPressMsg{Code: tea.KeyF6})
		m, _ = uxPress(m, tea.KeyPressMsg{Code: tea.KeyEscape})
		m, _ = uxRune(m, 'o')
		m, _ = uxPress(m, tea.KeyPressMsg{Code: tea.KeyEscape})
		m, _ = uxRune(m, 'q')
		m, _ = uxRune(m, 'n')
		if m.mode != modeMain || m.selectedTorrentID() != "b" || !m.filterApplied || !m.batchSelected["b"] || m.downloadRef != ref || m.bulk.QueueID != 7 || m.runtime.Context().Err() != nil {
			t.Fatalf("navigation damaged state after resize %v", size)
		}
	}
}
