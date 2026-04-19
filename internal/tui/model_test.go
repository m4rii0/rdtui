package tui

import (
	"testing"
	"time"

	"github.com/mario/real-debrid/pkg/models"
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

func TestSortSelectedColumnPreservesSelection(t *testing.T) {
	m := Model{
		sortColumn:     colAdded,
		selectedColumn: colSize,
		sortAscending:  false,
		torrents: []models.Torrent{
			{ID: "a", Added: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC), Bytes: 10},
			{ID: "b", Added: time.Date(2026, 4, 2, 0, 0, 0, 0, time.UTC), Bytes: 30},
			{ID: "c", Added: time.Date(2026, 4, 3, 0, 0, 0, 0, time.UTC), Bytes: 20},
		},
		selectedIdx: 1,
	}

	m.sortSelectedColumn()

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
		sortColumn:     colAdded,
		selectedColumn: colAdded,
		sortAscending:  false,
		torrents: []models.Torrent{
			{ID: "old", Added: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)},
			{ID: "new", Added: time.Date(2026, 4, 3, 0, 0, 0, 0, time.UTC)},
		},
		selectedIdx: 0,
	}

	m.sortSelectedColumn()
	if !m.sortAscending {
		t.Fatal("expected ascending after first toggle on same column")
	}
	if m.torrents[0].ID != "old" {
		t.Fatalf("expected old first in ascending, got %s", m.torrents[0].ID)
	}

	m.sortSelectedColumn()
	if m.sortAscending {
		t.Fatal("expected descending after second toggle")
	}
	if m.torrents[0].ID != "new" {
		t.Fatalf("expected new first in descending, got %s", m.torrents[0].ID)
	}
}

func TestSortDifferentColumnResetsDirection(t *testing.T) {
	m := Model{
		sortColumn:     colAdded,
		selectedColumn: colSize,
		sortAscending:  true,
		torrents: []models.Torrent{
			{ID: "a", Bytes: 10},
			{ID: "b", Bytes: 30},
		},
		selectedIdx: 0,
	}

	m.sortSelectedColumn()

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
	if m.selectedColumn != colAdded {
		t.Fatalf("default selectedColumn = %d, want colAdded", m.selectedColumn)
	}
}

func TestMoveColumnSelection(t *testing.T) {
	m := Model{selectedColumn: colAdded}

	m.moveColumnSelection(1)
	if m.selectedColumn != colName {
		t.Fatalf("after right: selectedColumn = %d, want colName(%d)", m.selectedColumn, colName)
	}

	m.moveColumnSelection(-1)
	if m.selectedColumn != colAdded {
		t.Fatalf("after left: selectedColumn = %d, want colAdded(%d)", m.selectedColumn, colAdded)
	}

	m.selectedColumn = 0
	m.moveColumnSelection(-1)
	if m.selectedColumn != columnCount-1 {
		t.Fatalf("wrap left: selectedColumn = %d, want %d", m.selectedColumn, columnCount-1)
	}

	m.selectedColumn = columnCount - 1
	m.moveColumnSelection(1)
	if m.selectedColumn != 0 {
		t.Fatalf("wrap right: selectedColumn = %d, want 0", m.selectedColumn)
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
		"downloading":            0,
		"queued":                 1,
		"downloaded":             3,
		"error":                  5,
		"unknown_status":         4,
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
