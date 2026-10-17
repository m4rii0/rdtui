package tui

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/m4rii0/rdtui/internal/app"
	"github.com/m4rii0/rdtui/pkg/models"
)

func TestActivityStrip(t *testing.T) {
	m := NewModel(Dependencies{Runtime: newTestRuntime(t)})
	m.mode, m.width, m.height = modeMain, 120, 30
	if !strings.Contains(ansi.Strip(renderView(m)), "No active download") {
		t.Fatal("missing idle strip")
	}
	d := models.ManagedDownload{GID: "one", Filename: "film.mkv", URL: "https://SECRET", Status: models.ManagedDownloadStatusActive, TotalLength: 100, CompletedLength: 63, DownloadSpeed: 1024}
	m.download = new(d)
	m.downloadRef = app.DownloadRef{Generation: 1, GID: "one"}
	for _, mode := range []mode{modeMain, modeSearch, modeDetail, modeDownload, modeBulkDownload} {
		m.mode = mode
		v := ansi.Strip(renderView(m))
		if !strings.Contains(v, "63.0%") || strings.Contains(v, "SECRET") {
			t.Fatalf("activity missing/leaked in %s: %s", mode, v)
		}
		if mode == modeSearch {
			if strings.Contains(v, "a activity") {
				t.Fatal("search advertises an inactive activity shortcut")
			}
		} else if !strings.Contains(v, "a activity") {
			t.Fatalf("missing letter shortcut in %s", mode)
		}
	}
	m.mode = modeMain
	m.bulk = &bulkDownloadState{QueueID: 1, Snapshot: app.QueueSnapshot{ID: 1, CurrentIndex: 0, Items: []app.QueueItemSnapshot{{Item: app.QueueItem{Target: models.DownloadTarget{Label: "new.mkv"}}, Status: app.QueueItemResolving}}}}
	v := ansi.Strip(renderView(m))
	if !strings.Contains(v, "resolving") || strings.Contains(v, "film.mkv") {
		t.Fatalf("queue shows stale transfer: %s", v)
	}
	m.bulk.Snapshot.Items[0].Status = app.QueueItemAwaitingOverwrite
	if !strings.Contains(ansi.Strip(renderView(m)), "overwrite") {
		t.Fatal("missing overwrite state")
	}
	m.bulk.Snapshot.Finished = true
	m.bulk.Snapshot.Download = &app.DownloadSnapshot{Ref: m.downloadRef, Download: d}
	if !strings.Contains(ansi.Strip(renderView(m)), "Queue finished") {
		t.Fatal("terminal queue not shown")
	}
	m.downloadRef.Generation = 2
	m.download.Filename = "newer.mkv"
	if !strings.Contains(ansi.Strip(renderView(m)), "newer.mkv") {
		t.Fatal("new standalone hidden by old queue")
	}
	m.bulk = nil
	for _, status := range []models.ManagedDownloadStatus{models.ManagedDownloadStatusWaiting, models.ManagedDownloadStatusComplete, models.ManagedDownloadStatusError, models.ManagedDownloadStatusRemoved} {
		m.download.Status = status
		if !strings.Contains(ansi.Strip(renderActivity(m, 120)), string(status)) {
			t.Fatalf("missing standalone %s", status)
		}
	}
	m.bulk = &bulkDownloadState{Starting: true}
	if !strings.Contains(ansi.Strip(renderActivity(m, 120)), "Queue starting") {
		t.Fatal("missing starting state")
	}
}

func TestActivityNavigation(t *testing.T) {
	for _, base := range []mode{modeMain, modeDetail} {
		m := NewModel(Dependencies{Runtime: newTestRuntime(t)})
		m.mode, m.returnMode = base, modeMain
		m.download = &models.ManagedDownload{Filename: "file", Status: models.ManagedDownloadStatusActive}
		m.searchInput.SetValue("query")
		next, cmd := m.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
		m = next.(Model)
		if m.mode != modeDownload || cmd != nil {
			t.Fatal("a must navigate without starting/resolving")
		}
		next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
		m = next.(Model)
		if m.mode != base || m.returnMode != modeMain || m.searchInput.Value() != "query" {
			t.Fatalf("return context lost: %s", m.mode)
		}
	}
	for _, base := range []mode{modeDelete, modeSort, modeQuitConfirm} {
		m := NewModel(Dependencies{Runtime: newTestRuntime(t)})
		m.mode = base
		m.download = &models.ManagedDownload{}
		next, cmd := m.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
		if next.(Model).mode != base || cmd != nil {
			t.Fatal("a escaped a modal")
		}
	}
}

func TestActivityFooterPriority(t *testing.T) {
	m := NewModel(Dependencies{Runtime: newTestRuntime(t)})
	m.mode = modeMain
	footer := ansi.Strip(activityFooter(m, 36))
	if strings.Count(footer, "a activity") != 1 || !strings.Contains(footer, "? help") || !strings.Contains(footer, "move") {
		t.Fatalf("essential hints crowded out: %q", footer)
	}
}

func TestActivityShortcutPreservesTextInput(t *testing.T) {
	for _, base := range []mode{modeSearch, modeTokenInput, modeMagnetInput, modeURLInput} {
		m := NewModel(Dependencies{Runtime: newTestRuntime(t)})
		m.mode = base
		m.download = &models.ManagedDownload{Filename: "file", Status: models.ManagedDownloadStatusActive}
		if base == modeSearch {
			m.searchInput.Focus()
		} else {
			m.input.Focus()
		}
		m, _ = uxRune(m, 'a')
		if m.mode != base {
			t.Fatalf("activity shortcut intercepted input in %s", base)
		}
		text := m.input.Value()
		if base == modeSearch {
			text = m.searchInput.Value()
		}
		if text != "a" {
			t.Fatalf("%s input=%q, want a", base, text)
		}
	}
}

func TestActivityShortcutRespectsHelp(t *testing.T) {
	m := NewModel(Dependencies{Runtime: newTestRuntime(t)})
	m.mode = modeMain
	m.download = &models.ManagedDownload{Status: models.ManagedDownloadStatusActive}
	m.helpVisible = true
	m, cmd := uxRune(m, 'a')
	if m.mode != modeMain || !m.helpVisible || cmd != nil {
		t.Fatal("activity escaped help overlay")
	}
}

func TestActivityBackgroundObservation(t *testing.T) {
	m := NewModel(Dependencies{Runtime: newTestRuntime(t)})
	m.mode = modeMain
	m.download = &models.ManagedDownload{GID: "one", Filename: "file", Status: models.ManagedDownloadStatusActive}
	m.downloadRef = app.DownloadRef{Generation: 1, GID: "one"}
	m.schedule = func(_ time.Duration, msg tea.Msg) tea.Cmd { return func() tea.Msg { return msg } }
	next, cmd := m.Update(downloadTickMsg{downloadRequestID{requestID{m.downloadPoll.Generation}, m.downloadRef}})
	m = next.(Model)
	if cmd == nil || !m.downloadPoll.InFlight {
		t.Fatal("background polling stopped outside download screen")
	}
	_, duplicate := m.Update(downloadTickMsg{downloadRequestID{requestID{m.downloadPoll.Generation}, m.downloadRef}})
	if duplicate != nil {
		t.Fatal("overlapping background poll")
	}
	// Do not execute external I/O: deliver the boundary result deterministically.
	next, _ = m.Update(currentDownloadResult(m, *m.download, errors.New("temporary")))
	m = next.(Model)
	if m.download.Filename != "file" {
		t.Fatal("transient error discarded snapshot")
	}
	if !strings.Contains(ansi.Strip(renderActivity(m, 120)), "retrying") {
		t.Fatal("stale snapshot indistinguishable from current progress")
	}
	d := *m.download
	d.TotalLength = 100
	d.CompletedLength = 42
	next, _ = m.Update(currentDownloadResult(m, d, nil))
	m = next.(Model)
	if m.download.CompletedLength != 42 {
		t.Fatal("background progress not updated")
	}
}
