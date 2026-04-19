package tui

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/m4rii0/rdtui/pkg/models"
)

func TestSortTorrentsAddedNewestFirst(t *testing.T) {
	torrents := []models.Torrent{
		{ID: "old", Added: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)},
		{ID: "new", Added: time.Date(2026, 4, 3, 0, 0, 0, 0, time.UTC)},
		{ID: "mid", Added: time.Date(2026, 4, 2, 0, 0, 0, 0, time.UTC)},
	}

	sortTorrents(torrents, colAdded, false)

	got := []string{torrents[0].ID, torrents[1].ID, torrents[2].ID}
	want := []string{"new", "mid", "old"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("sorted ids = %v, want %v", got, want)
		}
	}
}

func TestSortByColumnPreservesSelection(t *testing.T) {
	m := Model{
		sortColumn:    colAdded,
		sortAscending: false,
		torrents: []models.Torrent{
			{ID: "a", Added: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC), Bytes: 10},
			{ID: "b", Added: time.Date(2026, 4, 2, 0, 0, 0, 0, time.UTC), Bytes: 30},
			{ID: "c", Added: time.Date(2026, 4, 3, 0, 0, 0, 0, time.UTC), Bytes: 20},
		},
		selectedIdx: 1,
	}

	m.sortByColumn(colSize)

	if got := m.selectedTorrentID(); got != "b" {
		t.Fatalf("selected torrent = %q, want b", got)
	}
	if m.sortColumn != colSize {
		t.Fatalf("sortColumn = %d, want %d", m.sortColumn, colSize)
	}
	if m.selectedIdx != 0 {
		t.Fatalf("selectedIdx = %d, want 0 after sorting by size desc", m.selectedIdx)
	}
}

func TestSortTogglesDirection(t *testing.T) {
	m := Model{
		sortColumn:    colAdded,
		sortAscending: false,
		torrents: []models.Torrent{
			{ID: "old", Added: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)},
			{ID: "new", Added: time.Date(2026, 4, 3, 0, 0, 0, 0, time.UTC)},
		},
		selectedIdx: 0,
	}

	m.sortByColumn(colAdded)
	if !m.sortAscending {
		t.Fatal("expected ascending after first toggle on same column")
	}
	if m.torrents[0].ID != "old" {
		t.Fatalf("expected old first in ascending, got %s", m.torrents[0].ID)
	}

	m.sortByColumn(colAdded)
	if m.sortAscending {
		t.Fatal("expected descending after second toggle")
	}
	if m.torrents[0].ID != "new" {
		t.Fatalf("expected new first in descending, got %s", m.torrents[0].ID)
	}
}

func TestSortDifferentColumnResetsDirection(t *testing.T) {
	m := Model{
		sortColumn:    colAdded,
		sortAscending: true,
		torrents: []models.Torrent{
			{ID: "a", Bytes: 10},
			{ID: "b", Bytes: 30},
		},
		selectedIdx: 0,
	}

	m.sortByColumn(colSize)

	if m.sortColumn != colSize {
		t.Fatalf("sortColumn = %d, want colSize", m.sortColumn)
	}
	if m.sortAscending {
		t.Fatal("expected descending (default) when switching columns")
	}
}

func TestDefaultSortState(t *testing.T) {
	m := NewModel(nil)
	if m.sortColumn != colAdded {
		t.Fatalf("default sortColumn = %d, want colAdded", m.sortColumn)
	}
	if m.sortAscending {
		t.Fatal("default sortAscending should be false")
	}
}

func TestSelectedTorrentIDSurvivesRefresh(t *testing.T) {
	m := Model{
		sortColumn:    colAdded,
		sortAscending: false,
		torrents: []models.Torrent{
			{ID: "a", Added: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)},
			{ID: "b", Added: time.Date(2026, 4, 2, 0, 0, 0, 0, time.UTC)},
		},
		selectedIdx: 1,
	}

	originalID := m.selectedTorrentID()
	if originalID != "b" {
		t.Fatalf("initial selection = %q, want b", originalID)
	}

	newTorrents := []models.Torrent{
		{ID: "c", Added: time.Date(2026, 4, 3, 0, 0, 0, 0, time.UTC)},
		{ID: "a", Added: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)},
		{ID: "b", Added: time.Date(2026, 4, 2, 0, 0, 0, 0, time.UTC)},
	}
	m.torrents = append([]models.Torrent(nil), newTorrents...)
	sortTorrents(m.torrents, m.sortColumn, m.sortAscending)
	m.selectedIdx = selectedIndexForID(m.torrents, originalID)

	if m.selectedTorrentID() != "b" {
		t.Fatalf("after refresh: selected = %q, want b", m.selectedTorrentID())
	}
}

func TestStatusRankOrdering(t *testing.T) {
	ranks := map[string]int{
		"downloading":             0,
		"queued":                  1,
		"downloaded":              3,
		"error":                   5,
		"unknown_status":          4,
		"waiting_files_selection": 2,
	}
	for status, want := range ranks {
		got := torrentStatusRank(status)
		if got != want {
			t.Errorf("torrentStatusRank(%q) = %d, want %d", status, got, want)
		}
	}
}

func TestCompareByColumn(t *testing.T) {
	a := models.Torrent{ID: "a", Filename: "alpha", Bytes: 100, Progress: 50, Added: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)}
	b := models.Torrent{ID: "b", Filename: "beta", Bytes: 200, Progress: 75, Added: time.Date(2026, 4, 2, 0, 0, 0, 0, time.UTC)}

	if cmp := compareByColumn(a, b, colAdded); cmp >= 0 {
		t.Fatal("a before b by added, expected cmp < 0")
	}
	if cmp := compareByColumn(a, b, colSize); cmp >= 0 {
		t.Fatal("a smaller than b, expected cmp < 0")
	}
	if cmp := compareByColumn(a, b, colProgress); cmp >= 0 {
		t.Fatal("a less progress than b, expected cmp < 0")
	}
	if cmp := compareByColumn(a, b, colName); cmp >= 0 {
		t.Fatal("alpha < beta, expected cmp < 0")
	}
}

func TestEnterOpensDetailView(t *testing.T) {
	m := Model{
		mode:          modeMain,
		sortColumn:    colAdded,
		sortAscending: false,
		torrents: []models.Torrent{
			{ID: "a", Filename: "test", Added: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)},
		},
		selectedIdx: 0,
	}

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if m.mode != modeDetail {
		t.Fatalf("mode = %q, want modeDetail after Enter", m.mode)
	}
	if cmd == nil {
		t.Fatal("expected a detail fetch command when entering detail view without cached detail")
	}
}

func TestEnterWithCachedDetail(t *testing.T) {
	m := Model{
		mode:          modeMain,
		sortColumn:    colAdded,
		sortAscending: false,
		torrents: []models.Torrent{
			{ID: "a", Filename: "test", Added: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)},
		},
		selectedIdx: 0,
		detail:      &models.TorrentInfo{Torrent: models.Torrent{ID: "a"}},
	}

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if m.mode != modeDetail {
		t.Fatalf("mode = %q, want modeDetail after Enter", m.mode)
	}
	if cmd != nil {
		t.Fatal("expected no command when detail is already cached")
	}
}

func TestEscapeReturnsToList(t *testing.T) {
	m := Model{
		mode:       modeDetail,
		returnMode: modeMain,
		sortColumn: colAdded,
		torrents: []models.Torrent{
			{ID: "a", Filename: "test", Added: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)},
		},
		selectedIdx: 0,
	}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEscape})
	m = updated.(Model)
	if m.mode != modeMain {
		t.Fatalf("mode = %q, want modeMain after ESC", m.mode)
	}
}

func TestShiftLetterSortShortcuts(t *testing.T) {
	m := Model{
		mode:          modeMain,
		sortColumn:    colAdded,
		sortAscending: false,
		torrents: []models.Torrent{
			{ID: "a", Bytes: 10, Status: "downloaded", Progress: 50, Added: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC), Filename: "a"},
			{ID: "b", Bytes: 30, Status: "error", Progress: 75, Added: time.Date(2026, 4, 2, 0, 0, 0, 0, time.UTC), Filename: "b"},
		},
		selectedIdx: 0,
	}

	tests := []struct {
		key  string
		want int
	}{
		{"S", colStatus},
		{"P", colProgress},
		{"Z", colSize},
		{"D", colAdded},
		{"N", colName},
	}
	for _, tt := range tests {
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(tt.key)})
		m = updated.(Model)
		if m.sortColumn != tt.want {
			t.Fatalf("after %s: sortColumn = %d, want %d", tt.key, m.sortColumn, tt.want)
		}
	}
}

func TestDeleteFromMainView(t *testing.T) {
	m := Model{
		mode:          modeMain,
		returnMode:    modeMain,
		sortColumn:    colAdded,
		sortAscending: false,
		torrents: []models.Torrent{
			{ID: "a", Filename: "test", Added: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)},
		},
		selectedIdx: 0,
		detail:      &models.TorrentInfo{Torrent: models.Torrent{ID: "a", Filename: "test"}},
	}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	m = updated.(Model)
	if m.mode != modeDelete {
		t.Fatalf("mode = %q, want modeDelete after x", m.mode)
	}
	if len(m.deleteIDs) != 1 || m.deleteIDs[0] != "a" {
		t.Fatalf("deleteIDs = %v, want [a]", m.deleteIDs)
	}
}

func TestDeleteCancelReturnsToReturnMode(t *testing.T) {
	m := Model{
		mode:       modeDelete,
		returnMode: modeDetail,
		deleteIDs:  []string{"a"},
	}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEscape})
	m = updated.(Model)
	if m.mode != modeDetail {
		t.Fatalf("mode = %q, want modeDetail after cancel", m.mode)
	}
}

func TestDeleteSuccessReturnsToMain(t *testing.T) {
	m := Model{
		mode:       modeDelete,
		returnMode: modeDetail,
		deleteIDs:  []string{"a"},
	}

	updated, _ := m.Update(deleteMsg{err: nil})
	m = updated.(Model)
	if m.mode != modeMain {
		t.Fatalf("mode = %q, want modeMain after successful delete", m.mode)
	}
}

func TestModalReturnsToReturnMode(t *testing.T) {
	m := Model{
		mode:       modeSelectFiles,
		returnMode: modeDetail,
		selector:   selectFilesState{Files: []models.TorrentFile{}, Selected: map[int]bool{}},
	}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEscape})
	m = updated.(Model)
	if m.mode != modeDetail {
		t.Fatalf("mode = %q, want modeDetail after modal cancel", m.mode)
	}
}

func TestBatchModeToggle(t *testing.T) {
	m := Model{
		mode:          modeMain,
		sortColumn:    colAdded,
		sortAscending: false,
		torrents: []models.Torrent{
			{ID: "a", Filename: "test", Added: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)},
		},
		selectedIdx:   0,
		batchSelected: map[string]bool{},
	}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})
	m = updated.(Model)
	if !m.batchMode {
		t.Fatal("expected batch mode active after pressing b")
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})
	m = updated.(Model)
	if m.batchMode {
		t.Fatal("expected batch mode inactive after pressing b again")
	}
	if len(m.batchSelected) != 0 {
		t.Fatal("expected batch selection cleared when exiting batch mode")
	}
}

func TestBatchSpaceTogglesMark(t *testing.T) {
	m := Model{
		mode:          modeMain,
		batchMode:     true,
		sortColumn:    colAdded,
		sortAscending: false,
		torrents: []models.Torrent{
			{ID: "a", Filename: "test", Added: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)},
		},
		selectedIdx:   0,
		batchSelected: map[string]bool{},
	}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	m = updated.(Model)
	if !m.batchSelected["a"] {
		t.Fatal("expected torrent 'a' marked after space")
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	m = updated.(Model)
	if m.batchSelected["a"] {
		t.Fatal("expected torrent 'a' unmarked after second space")
	}
}

func TestBatchDeleteEntersConfirmation(t *testing.T) {
	m := Model{
		mode:       modeMain,
		batchMode:  true,
		sortColumn: colAdded,
		torrents: []models.Torrent{
			{ID: "a", Filename: "test", Added: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)},
			{ID: "b", Filename: "test2", Added: time.Date(2026, 4, 2, 0, 0, 0, 0, time.UTC)},
		},
		selectedIdx:   0,
		batchSelected: map[string]bool{"a": true, "b": true},
	}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	m = updated.(Model)
	if m.mode != modeDelete {
		t.Fatalf("mode = %q, want modeDelete after x with batch selection", m.mode)
	}
	if len(m.deleteIDs) != 2 {
		t.Fatalf("deleteIDs len = %d, want 2", len(m.deleteIDs))
	}
}

func TestBatchEscapeClearsSelection(t *testing.T) {
	m := Model{
		mode:          modeMain,
		batchMode:     true,
		batchSelected: map[string]bool{"a": true},
	}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEscape})
	m = updated.(Model)
	if m.batchMode {
		t.Fatal("expected batch mode off after esc")
	}
	if len(m.batchSelected) != 0 {
		t.Fatal("expected batch selection cleared after esc")
	}
}

func TestSelectAllTorrents(t *testing.T) {
	m := Model{
		mode:       modeMain,
		batchMode:  true,
		sortColumn: colAdded,
		torrents: []models.Torrent{
			{ID: "a", Added: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)},
			{ID: "b", Added: time.Date(2026, 4, 2, 0, 0, 0, 0, time.UTC)},
		},
		batchSelected: map[string]bool{},
	}
	m.selectAllTorrents()
	if len(m.batchSelected) != 2 {
		t.Fatalf("batchSelected len = %d, want 2", len(m.batchSelected))
	}
	if !m.batchSelected["a"] || !m.batchSelected["b"] {
		t.Fatal("expected both torrents selected")
	}
}

func TestBatchSelectedIDsMaintainsOrder(t *testing.T) {
	m := Model{
		sortColumn: colAdded,
		torrents: []models.Torrent{
			{ID: "c", Added: time.Date(2026, 4, 3, 0, 0, 0, 0, time.UTC)},
			{ID: "a", Added: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)},
			{ID: "b", Added: time.Date(2026, 4, 2, 0, 0, 0, 0, time.UTC)},
		},
		batchSelected: map[string]bool{"b": true, "a": true},
	}
	ids := m.batchSelectedIDs()
	if len(ids) != 2 {
		t.Fatalf("ids len = %d, want 2", len(ids))
	}
	if ids[0] != "a" || ids[1] != "b" {
		t.Fatalf("ids = %v, want [a b] (torrent list order)", ids)
	}
}
