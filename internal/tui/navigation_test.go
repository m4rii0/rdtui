package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/m4rii0/rdtui/internal/app"
	"github.com/m4rii0/rdtui/pkg/models"
)

func uxPress(m Model, key tea.KeyPressMsg) (Model, tea.Cmd) {
	next, cmd := m.Update(key)
	return next.(Model), cmd
}
func uxRune(m Model, r rune) (Model, tea.Cmd) {
	return uxPress(m, tea.KeyPressMsg{Code: r, Text: string(r)})
}

func TestListPageNavigation(t *testing.T) {
	m := NewModel(Dependencies{Runtime: newTestRuntime(t)})
	m.mode, m.width, m.height = modeMain, 80, 24
	for i := range 60 {
		m.torrents = append(m.torrents, models.Torrent{ID: fmt.Sprint(i), Filename: fmt.Sprintf("file-%d", i)})
	}
	m.batchSelected = map[string]bool{"2": true}
	for _, step := range []struct {
		key     rune
		want    int
		command bool
	}{{tea.KeyPgDown, 13, true}, {tea.KeyEnd, 59, true}, {tea.KeyPgDown, 59, false}, {tea.KeyPgUp, 46, true}, {tea.KeyHome, 0, true}, {tea.KeyHome, 0, false}} {
		var cmd tea.Cmd
		m, cmd = uxPress(m, tea.KeyPressMsg{Code: step.key})
		if m.selectedIdx != step.want || (cmd != nil) != step.command {
			t.Fatalf("key=%d selection=%d cmd=%v want=%d/%v", step.key, m.selectedIdx, cmd != nil, step.want, step.command)
		}
	}
	if !m.batchSelected["2"] {
		t.Fatal("navigation cleared marks")
	}
	m.torrents = nil
	m, cmd := uxPress(m, tea.KeyPressMsg{Code: tea.KeyEnd})
	if cmd != nil || m.selectedIdx != 0 {
		t.Fatal("empty navigation is not a no-op")
	}
}

func TestNavigationEscapeAndInput(t *testing.T) {
	m := NewModel(Dependencies{Runtime: newTestRuntime(t)})
	m.mode = modeMain
	m.torrents = []models.Torrent{{ID: "a", Filename: "other"}, {ID: "b", Filename: "keep"}}
	m.searchInput.SetValue("keep")
	m.filterApplied = true
	m.applyFilter()
	m.batchMode = true
	m.batchSelected["b"] = true
	m, _ = uxPress(m, tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.batchMode || !m.filterApplied {
		t.Fatal("Esc must exit batch before clearing filter")
	}
	m, _ = uxPress(m, tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.filterApplied || m.selectedTorrentID() != "b" {
		t.Fatal("filter clear lost selected torrent")
	}
	m, _ = uxRune(m, '/')
	m, _ = uxRune(m, 'q')
	m, _ = uxRune(m, 'o')
	if m.mode != modeSearch || m.searchInput.Value() != "qo" {
		t.Fatalf("search input intercepted: %s %q", m.mode, m.searchInput.Value())
	}
	m, _ = uxPress(m, tea.KeyPressMsg{Code: tea.KeyEscape})
	m.selectedIdx = 1
	m, _ = uxRune(m, '/')
	m.searchInput.SetValue("keep")
	m.applyFilter()
	m, _ = uxPress(m, tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.selectedTorrentID() != "b" {
		t.Fatal("canceling search reset selection")
	}
}

func TestSortPicker(t *testing.T) {
	m := NewModel(Dependencies{Runtime: newTestRuntime(t)})
	m.mode, m.width, m.height = modeMain, 40, 12
	m.torrents = []models.Torrent{{ID: "a", Filename: "z", Bytes: 1}, {ID: "b", Filename: "a", Bytes: 2}}
	m.selectedIdx = 1
	m, _ = uxRune(m, 'o')
	if m.mode != "sort" {
		t.Fatalf("sort picker not opened: %s", m.mode)
	}
	view := ansi.Strip(renderView(m))
	if !strings.Contains(view, "Sort") {
		t.Fatal("sort picker not rendered")
	}
	for range 5 {
		m, _ = uxRune(m, 'k')
	}
	m, _ = uxPress(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.sortColumn != colStatus || m.selectedTorrentID() != "b" {
		t.Fatal("sort application lost identity")
	}
	ascending := m.sortAscending
	m, _ = uxRune(m, 'o')
	m, _ = uxPress(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.sortAscending == ascending {
		t.Fatal("same-column sort did not toggle")
	}
	for _, column := range []int{colProgress, colSize, colAdded, colName} {
		m, _ = uxRune(m, 'o')
		m, _ = uxRune(m, 'j')
		m, _ = uxPress(m, tea.KeyPressMsg{Code: tea.KeyEnter})
		if m.sortColumn != column || m.selectedTorrentID() != "b" {
			t.Fatalf("sort column=%d want=%d", m.sortColumn, column)
		}
	}
	m, _ = uxRune(m, 'o')
	m, _ = uxRune(m, '?')
	m, _ = uxPress(m, tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.mode != "sort" || m.helpVisible {
		t.Fatal("help escaped sort context")
	}
	m, _ = uxPress(m, tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.mode != modeMain {
		t.Fatal("sort cancel failed")
	}
	m, _ = uxRune(m, 'Z')
	if m.sortColumn != colSize {
		t.Fatal("legacy sort shortcut lost")
	}
}

func TestQuitConfirmation(t *testing.T) {
	for _, base := range []mode{modeMain, modeDetail, modeDownload, modeBulkDownload} {
		for _, cancel := range []rune{'n', tea.KeyEnter, tea.KeyEscape} {
			m := NewModel(Dependencies{Runtime: newTestRuntime(t)})
			m.mode, m.returnMode = base, modeDetail
			m.download = &models.ManagedDownload{Status: models.ManagedDownloadStatusActive}
			m.filterApplied = true
			m.batchSelected["marked"] = true
			pending := &pendingDownloadState{Queue: &app.QueueDecision{QueueID: 2, ItemIndex: 1}, ResumeMode: base}
			m.pendingDownload = pending
			m, cmd := uxRune(m, 'q')
			if cmd != nil || m.mode != "quit-confirm" || m.runtime.Context().Err() != nil {
				t.Fatalf("quit not guarded in %s", base)
			}
			key := tea.KeyPressMsg{Code: cancel}
			if cancel == 'n' {
				key.Text = "n"
			}
			m, _ = uxPress(m, key)
			if m.mode != base || m.returnMode != modeDetail || !m.filterApplied || !m.batchSelected["marked"] || m.pendingDownload != pending || m.runtime.Context().Err() != nil {
				t.Fatalf("quit cancellation damaged context: base=%s cancel=%d mode=%s return=%s filter=%v mark=%v pending=%v runtime=%v", base, cancel, m.mode, m.returnMode, m.filterApplied, m.batchSelected["marked"], m.pendingDownload == pending, m.runtime.Context().Err())
			}
			m, _ = uxRune(m, 'q')
			m, cmd = uxRune(m, 'y')
			if cmd == nil || m.runtime.Context().Err() == nil {
				t.Fatal("confirmed quit did not stop runtime")
			}
			if _, ok := cmd().(tea.QuitMsg); !ok {
				t.Fatal("missing quit command")
			}
		}
	}
	for _, terminal := range []bool{false, true} {
		m := NewModel(Dependencies{Runtime: newTestRuntime(t)})
		m.mode = modeMain
		if terminal {
			m.download = &models.ManagedDownload{Status: models.ManagedDownloadStatusComplete}
		}
		_, cmd := uxRune(m, 'q')
		if cmd == nil {
			t.Fatal("idle or terminal work should quit directly")
		}
	}
	m := NewModel(Dependencies{Runtime: newTestRuntime(t)})
	m.mode = modeMain
	m.bulk = &bulkDownloadState{Starting: true}
	m, cmd := uxRune(m, 'q')
	if cmd != nil || m.mode != "quit-confirm" {
		t.Fatal("starting queue not protected")
	}
	_, cmd = uxPress(m, tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if cmd == nil {
		t.Fatal("Ctrl+C no longer immediate")
	}
}
