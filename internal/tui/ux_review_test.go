package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/m4rii0/rdtui/internal/app"
	"github.com/m4rii0/rdtui/pkg/models"
)

func TestQuitOwnsModalDuringQueueOverwrite(t *testing.T) {
	for _, answer := range []rune{tea.KeyEnter, 'n', tea.KeyEscape, 'y'} {
		m, queue := observedQueue(t)
		m.mode = modeMain
		m, _ = uxRune(m, 'q')
		if m.mode != modeQuitConfirm {
			t.Fatal("quit dialog missing")
		}
		queue.snapshot.AwaitingOverwrite = true
		queue.snapshot.Items[0].Status = app.QueueItemAwaitingOverwrite
		queue.snapshot.ExistingPath = "/downloads/existing.mkv"
		m, _ = updateQueue(m, queue.snapshot)
		if m.mode != modeQuitConfirm {
			t.Fatalf("queue stole quit modal before answer %d: %s", answer, m.mode)
		}
		if m.pendingDownload == nil || m.pendingDownload.ResumeMode != modeMain {
			t.Fatal("overwrite return context corrupted")
		}
		key := tea.KeyPressMsg{Code: answer}
		if answer == 'y' || answer == 'n' {
			key.Text = string(answer)
		}
		m, cmd := uxPress(m, key)
		if m.bulk.DecidedItem != nil {
			t.Fatal("quit answer authorized overwrite")
		}
		if answer == 'y' {
			if cmd == nil || m.runtime.Context().Err() == nil {
				t.Fatal("confirmed quit failed")
			}
		} else {
			if cmd != nil || m.mode != modeOverwrite || m.runtime.Context().Err() != nil {
				t.Fatal("cancel did not safely restore pending decision")
			}
		}
	}
}

func TestMultilineTorrentNamesKeepGeometry(t *testing.T) {
	for _, selected := range []int{0, 1} {
		for _, marked := range []bool{false, true} {
			m := NewModel(Dependencies{Runtime: newTestRuntime(t)})
			m.mode, m.width, m.height = modeMain, 40, 12
			m.torrents = []models.Torrent{{ID: "a", Filename: "first\n\x1b[31m映像\x1b[0m\r\nsecond", Status: "downloaded"}, {ID: "b", Filename: "other"}}
			m.selectedIdx = selected
			m.batchMode = marked
			m.batchSelected["a"] = marked
			view := renderView(m)
			if lipgloss.Height(view) != 12 || lipgloss.Width(view) > 40 {
				t.Fatalf("cell content broke frame: %dx%d", lipgloss.Width(view), lipgloss.Height(view))
			}
		}
	}
}

func TestCompactWorkbenchBounds(t *testing.T) {
	for _, base := range []mode{modeDetail, modeDownload, modeBulkDownload} {
		for _, size := range [][2]int{{12, 4}, {1, 1}, {2, 2}, {39, 11}} {
			m := NewModel(Dependencies{Runtime: newTestRuntime(t)})
			m.mode, m.width, m.height = base, size[0], size[1]
			m.detail = &models.TorrentInfo{Filename: "file", Status: "downloaded"}
			m.download = &models.ManagedDownload{Filename: "file", Status: models.ManagedDownloadStatusActive}
			view := renderView(m)
			if lipgloss.Width(view) > size[0] || lipgloss.Height(view) > size[1] {
				t.Fatalf("%s %v overflow: %dx%d", base, size, lipgloss.Width(view), lipgloss.Height(view))
			}
			if size[0] >= 12 && !strings.Contains(ansi.Strip(view), "file") && base != modeBulkDownload {
				t.Fatal("compact view lost filename")
			}
		}
	}
}

func TestQueueActivityReservesStatistics(t *testing.T) {
	m := NewModel(Dependencies{Runtime: newTestRuntime(t)})
	d := models.ManagedDownload{Filename: strings.Repeat("longname", 30), Status: models.ManagedDownloadStatusActive, TotalLength: 100, CompletedLength: 63, DownloadSpeed: 1024}
	m.bulk = &bulkDownloadState{QueueID: 1, Snapshot: app.QueueSnapshot{ID: 1, CurrentIndex: 0, Download: &app.DownloadSnapshot{Download: d}, Items: []app.QueueItemSnapshot{{Status: app.QueueItemActive, Item: app.QueueItem{Target: models.DownloadTarget{Label: d.Filename}}}}}}
	strip := ansi.Strip(renderActivity(m, 76))
	if !strings.Contains(strip, "63.0%") || !strings.Contains(strip, "KB/s") || !strings.Contains(strip, "longname") {
		t.Fatalf("filename hid statistics: %s", strip)
	}
}

func TestFilteredAndMarkedFooterKeepsMovementHint(t *testing.T) {
	for _, marked := range []bool{false, true} {
		m := NewModel(Dependencies{Runtime: newTestRuntime(t)})
		m.mode = modeMain
		m.filterApplied = true
		m.batchMode = marked
		footer := ansi.Strip(activityFooter(m, 36))
		if !strings.Contains(footer, "move") || !strings.Contains(footer, "? help") {
			t.Fatalf("navigation hint lost: %s", footer)
		}
	}
}

func TestDownloadWaitsForSelectedTorrentDetail(t *testing.T) {
	m := NewModel(Dependencies{Runtime: newTestRuntime(t)})
	m.mode = modeMain
	m.torrents = []models.Torrent{{ID: "selected", Filename: "selected", Status: "downloaded"}}
	m.detail = &models.TorrentInfo{ID: "stale", Filename: "wrong", Status: "downloaded", Links: []string{"wrong-source"}}
	m, cmd := uxRune(m, 'd')
	if cmd == nil || m.pendingAction == nil || m.pendingAction.torrentID != "selected" || m.setup.InFlight {
		t.Fatal("download resolved a previously focused torrent instead of waiting for the selection")
	}
}
