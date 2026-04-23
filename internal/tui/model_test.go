package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
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
		"magnet_conversion":       2,
	}
	for status, want := range ranks {
		got := torrentStatusRank(status)
		if got != want {
			t.Errorf("torrentStatusRank(%q) = %d, want %d", status, got, want)
		}
	}
}

func TestCanSelectFilesIncludesMagnetConversion(t *testing.T) {
	m := Model{
		mode:        modeMain,
		torrents:    []models.Torrent{{ID: "a", Status: "magnet_conversion"}},
		selectedIdx: 0,
	}
	if !m.canSelectFilesFromSelection() {
		t.Fatal("expected magnet_conversion to allow file selection from list")
	}

	m.detail = &models.TorrentInfo{Torrent: models.Torrent{ID: "a", Status: "magnet_conversion"}}
	if !m.canSelectFilesFromDetail() {
		t.Fatal("expected magnet_conversion to allow file selection from detail")
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

	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
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

	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
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

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
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
		r := []rune(tt.key)[0]
		updated, _ := m.Update(tea.KeyPressMsg{Code: r, Text: tt.key})
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

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	m = updated.(Model)
	if m.mode != modeDelete {
		t.Fatalf("mode = %q, want modeDelete after x", m.mode)
	}
	if len(m.deleteIDs) != 1 || m.deleteIDs[0] != "a" {
		t.Fatalf("deleteIDs = %v, want [a]", m.deleteIDs)
	}
}

func TestDownloadFromMainViewOpensTargetPicker(t *testing.T) {
	m := Model{
		mode:       modeMain,
		returnMode: modeMain,
		torrents: []models.Torrent{
			{ID: "a", Filename: "movie", Status: "downloaded", Added: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)},
		},
		selectedIdx: 0,
		detail: &models.TorrentInfo{
			Torrent: models.Torrent{ID: "a", Filename: "movie", Status: "downloaded", Links: []string{"https://example.com/link"}},
			Files:   []models.TorrentFile{{ID: 1, Path: "/movie.mkv", Selected: true}},
		},
	}

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'd', Text: "d"})
	m = updated.(Model)
	if m.mode != modeChooseTarget {
		t.Fatalf("mode = %q, want modeChooseTarget", m.mode)
	}
	if m.targets.Action != handoffDownload {
		t.Fatalf("action = %q, want %q", m.targets.Action, handoffDownload)
	}
}

func TestDownloadFromDetailViewOpensTargetPicker(t *testing.T) {
	m := Model{
		mode:       modeDetail,
		returnMode: modeDetail,
		detail: &models.TorrentInfo{
			Torrent: models.Torrent{ID: "a", Filename: "movie", Status: "downloaded", Links: []string{"https://example.com/link"}},
			Files:   []models.TorrentFile{{ID: 1, Path: "/movie.mkv", Selected: true}},
		},
	}

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'd', Text: "d"})
	m = updated.(Model)
	if m.mode != modeChooseTarget {
		t.Fatalf("mode = %q, want modeChooseTarget", m.mode)
	}
	if m.targets.Action != handoffDownload {
		t.Fatalf("action = %q, want %q", m.targets.Action, handoffDownload)
	}
}

func TestManagedDownloadMsgEntersDownloadMode(t *testing.T) {
	m := Model{returnMode: modeDetail}

	updated, cmd := m.Update(managedDownloadMsg{result: models.ManagedDownloadStart{Download: models.ManagedDownload{GID: "gid-1", Filename: "movie.mkv", Status: models.ManagedDownloadStatusActive}}})
	m = updated.(Model)
	if m.mode != modeDownload {
		t.Fatalf("mode = %q, want modeDownload", m.mode)
	}
	if m.download == nil || m.download.Filename != "movie.mkv" {
		t.Fatalf("download = %+v", m.download)
	}
	if cmd == nil {
		t.Fatal("expected polling command after entering download mode")
	}
}

func TestResolvedDownloadPromptsWhenFileExists(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "movie.mkv")
	if err := os.WriteFile(path, []byte("old-data"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	m := Model{mode: modeChooseTarget, returnMode: modeDetail, downloadDir: dir}

	updated, cmd := m.Update(resolvedDownloadMsg{url: "https://example.com/file", filename: "movie.mkv", filesize: 100})
	m = updated.(Model)
	if cmd != nil {
		t.Fatal("expected no start command before overwrite confirmation")
	}
	if m.mode != modeOverwrite {
		t.Fatalf("mode = %q, want modeOverwrite", m.mode)
	}
	if m.pendingDownload == nil {
		t.Fatal("expected pending download state")
	}
	if m.pendingDownload.ExistingBytes != 8 {
		t.Fatalf("ExistingBytes = %d, want 8", m.pendingDownload.ExistingBytes)
	}
	if m.pendingDownload.RemoteBytes != 100 {
		t.Fatalf("RemoteBytes = %d, want 100", m.pendingDownload.RemoteBytes)
	}
}

func TestResolvedDownloadStartsImmediatelyWhenFileMissing(t *testing.T) {
	dir := t.TempDir()
	m := Model{mode: modeChooseTarget, returnMode: modeDetail, downloadDir: dir}

	updated, cmd := m.Update(resolvedDownloadMsg{url: "https://example.com/file", filename: "movie.mkv", filesize: 100})
	m = updated.(Model)
	if cmd == nil {
		t.Fatal("expected start command when file missing")
	}
	if m.pendingDownload != nil {
		t.Fatal("pending download should be cleared")
	}
	if !m.loading {
		t.Fatal("expected loading while starting download")
	}
}

func TestOverwriteCancelReturnsToPreviousMode(t *testing.T) {
	m := Model{
		mode:       modeOverwrite,
		returnMode: modeDetail,
		pendingDownload: &pendingDownloadState{
			Filename: "movie.mkv",
		},
	}

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = updated.(Model)
	if m.mode != modeDetail {
		t.Fatalf("mode = %q, want modeDetail", m.mode)
	}
	if m.pendingDownload != nil {
		t.Fatal("pending download should be cleared")
	}
}

func TestManagedDownloadStatusMsgMarksCompletion(t *testing.T) {
	m := Model{mode: modeDownload, download: &models.ManagedDownload{GID: "gid-1", Filename: "movie.mkv", Status: models.ManagedDownloadStatusActive}}

	updated, _ := m.Update(managedDownloadStatusMsg{ok: true, download: models.ManagedDownload{GID: "gid-1", Filename: "movie.mkv", Status: models.ManagedDownloadStatusComplete}})
	m = updated.(Model)
	if m.download == nil || !m.download.IsComplete() {
		t.Fatalf("download = %+v", m.download)
	}
	if m.status != "Download complete" {
		t.Fatalf("status = %q", m.status)
	}
}

func TestDownloadPathMsgUpdatesStatus(t *testing.T) {
	m := Model{mode: modeDownload, download: &models.ManagedDownload{Status: models.ManagedDownloadStatusComplete}}

	updated, _ := m.Update(downloadPathMsg{action: "reveal"})
	m = updated.(Model)
	if m.status != "Opened download directory" {
		t.Fatalf("status = %q", m.status)
	}
}

func TestCompletedDownloadCanDeleteTorrent(t *testing.T) {
	m := Model{
		mode:              modeDownload,
		returnMode:        modeDetail,
		download:          &models.ManagedDownload{Status: models.ManagedDownloadStatusComplete},
		downloadTorrentID: "torrent-1",
	}

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	m = updated.(Model)
	if m.mode != modeDelete {
		t.Fatalf("mode = %q, want modeDelete", m.mode)
	}
	if m.returnMode != modeDownload {
		t.Fatalf("returnMode = %q, want modeDownload", m.returnMode)
	}
	if len(m.deleteIDs) != 1 || m.deleteIDs[0] != "torrent-1" {
		t.Fatalf("deleteIDs = %v", m.deleteIDs)
	}
}

func TestDeleteCancelReturnsToReturnMode(t *testing.T) {
	m := Model{
		mode:       modeDelete,
		returnMode: modeDetail,
		deleteIDs:  []string{"a"},
	}

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
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

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
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

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'b', Text: "b"})
	m = updated.(Model)
	if !m.batchMode {
		t.Fatal("expected batch mode active after pressing b")
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'b', Text: "b"})
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

	updated, _ := m.Update(tea.KeyPressMsg{Code: ' ', Text: " "})
	m = updated.(Model)
	if !m.batchSelected["a"] {
		t.Fatal("expected torrent 'a' marked after space")
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: ' ', Text: " "})
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

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
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

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
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

func TestMainDownloadFetchesDetailWhenSelectionIsReady(t *testing.T) {
	m := Model{
		mode:          modeMain,
		returnMode:    modeMain,
		sortColumn:    colAdded,
		sortAscending: false,
		torrents: []models.Torrent{{
			ID:       "a",
			Filename: "movie",
			Status:   "downloaded",
			Added:    time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC),
		}},
		selectedIdx: 0,
	}

	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'd', Text: "d"})
	m = updated.(Model)
	if cmd == nil {
		t.Fatal("expected detail fetch command for ready selection without cached detail")
	}
	if m.pendingAction == nil || m.pendingAction.action != actionStartDownload {
		t.Fatalf("pendingAction = %+v, want start-download", m.pendingAction)
	}
	if m.mode != modeMain {
		t.Fatalf("mode = %q, want modeMain while waiting for detail", m.mode)
	}
}

func TestHelpOverlayTogglesWithoutChangingContext(t *testing.T) {
	m := Model{mode: modeMain, torrents: []models.Torrent{{ID: "a", Status: "downloaded"}}, selectedIdx: 0}

	updated, _ := m.Update(tea.KeyPressMsg{Code: '?', Text: "?"})
	m = updated.(Model)
	if !m.helpVisible {
		t.Fatal("expected help overlay open after ?")
	}
	if m.mode != modeMain {
		t.Fatalf("mode = %q, want modeMain", m.mode)
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = updated.(Model)
	if m.helpVisible {
		t.Fatal("expected help overlay closed after esc")
	}
	if m.mode != modeMain {
		t.Fatalf("mode = %q, want modeMain after closing help", m.mode)
	}
}

func TestBrowserEditingQuestionMarkStaysInInput(t *testing.T) {
	b := newFileBrowser(".")
	b.startEditing()
	m := Model{mode: modeFileBrowser, browser: b}

	updated, _ := m.Update(tea.KeyPressMsg{Code: '?', Text: "?"})
	m = updated.(Model)
	if m.helpVisible {
		t.Fatal("help overlay should not open while browser path input is focused")
	}
	if !strings.Contains(m.browser.pathInput.Value(), "?") {
		t.Fatalf("path input value = %q, want to contain ?", m.browser.pathInput.Value())
	}
}

func TestDimmedMainDownloadShortcutDoesNotLaunch(t *testing.T) {
	m := Model{
		mode:          modeMain,
		sortColumn:    colAdded,
		sortAscending: false,
		torrents: []models.Torrent{{
			ID:       "a",
			Filename: "movie",
			Status:   "queued",
			Added:    time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC),
		}},
		selectedIdx: 0,
	}

	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'd', Text: "d"})
	m = updated.(Model)
	if cmd != nil {
		t.Fatal("expected no command when download action is unavailable")
	}
	if m.mode != modeMain {
		t.Fatalf("mode = %q, want modeMain", m.mode)
	}
	if m.errText != "Selected torrent is not ready to download" {
		t.Fatalf("errText = %q, want inline unavailable-action error", m.errText)
	}
}
