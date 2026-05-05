package tui

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/m4rii0/rdtui/internal/config"
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

func TestAddTorrentFocusesNewTorrentAfterRefresh(t *testing.T) {
	m := Model{
		mode:          modeURLInput,
		sortColumn:    colAdded,
		sortAscending: false,
		torrents: []models.Torrent{
			{ID: "old", Added: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)},
		},
		selectedIdx: 0,
	}

	updated, cmd := m.Update(addTorrentMsg{result: models.AddTorrentResult{ID: "new"}, label: "remote torrent URL"})
	m = updated.(Model)
	if cmd == nil {
		t.Fatal("expected refresh command after adding torrent")
	}
	if m.pendingFocusID != "new" {
		t.Fatalf("pendingFocusID = %q, want new", m.pendingFocusID)
	}

	updated, cmd = m.Update(torrentsMsg{torrents: []models.Torrent{
		{ID: "old", Added: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)},
		{ID: "new", Added: time.Date(2026, 4, 2, 0, 0, 0, 0, time.UTC)},
	}})
	m = updated.(Model)
	if got := m.selectedTorrentID(); got != "new" {
		t.Fatalf("selected torrent = %q, want new", got)
	}
	if m.pendingFocusID != "" {
		t.Fatalf("pendingFocusID = %q, want cleared", m.pendingFocusID)
	}
	if cmd == nil {
		t.Fatal("expected detail command for focused torrent")
	}
}

func TestImportFocusesNewTorrentAfterRefresh(t *testing.T) {
	m := Model{
		mode:          modeFileBrowser,
		sortColumn:    colAdded,
		sortAscending: false,
		torrents: []models.Torrent{
			{ID: "old", Added: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)},
		},
		selectedIdx: 0,
	}

	updated, cmd := m.Update(importMsg{results: []models.ImportResult{{Source: "movie.torrent", TorrentID: "new"}}})
	m = updated.(Model)
	if cmd == nil {
		t.Fatal("expected refresh command after importing torrent")
	}
	if m.pendingFocusID != "new" {
		t.Fatalf("pendingFocusID = %q, want new", m.pendingFocusID)
	}

	updated, _ = m.Update(torrentsMsg{torrents: []models.Torrent{
		{ID: "old", Added: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)},
		{ID: "new", Added: time.Date(2026, 4, 2, 0, 0, 0, 0, time.UTC)},
	}})
	m = updated.(Model)
	if got := m.selectedTorrentID(); got != "new" {
		t.Fatalf("selected torrent = %q, want new", got)
	}
	if m.pendingFocusID != "" {
		t.Fatalf("pendingFocusID = %q, want cleared", m.pendingFocusID)
	}
}

func TestPendingFocusWaitsUntilTorrentAppears(t *testing.T) {
	m := Model{
		sortColumn:    colAdded,
		sortAscending: false,
		torrents: []models.Torrent{
			{ID: "old", Added: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)},
		},
		selectedIdx:    0,
		pendingFocusID: "new",
	}

	updated, _ := m.Update(torrentsMsg{torrents: []models.Torrent{
		{ID: "old", Added: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)},
	}})
	m = updated.(Model)
	if got := m.selectedTorrentID(); got != "old" {
		t.Fatalf("selected torrent = %q, want old", got)
	}
	if m.pendingFocusID != "new" {
		t.Fatalf("pendingFocusID = %q, want new", m.pendingFocusID)
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

func TestDownloadFromMainViewStartsDirectlyForSingleTarget(t *testing.T) {
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

	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'd', Text: "d"})
	m = updated.(Model)
	if m.mode != modeMain {
		t.Fatalf("mode = %q, want modeMain (single target skips picker)", m.mode)
	}
	if cmd == nil {
		t.Fatal("expected resolve command when single target is available")
	}
	if !m.loading {
		t.Fatal("expected loading to be true while resolving direct download")
	}
}

func TestDownloadFromDetailViewStartsDirectlyForSingleTarget(t *testing.T) {
	m := Model{
		mode:       modeDetail,
		returnMode: modeDetail,
		detail: &models.TorrentInfo{
			Torrent: models.Torrent{ID: "a", Filename: "movie", Status: "downloaded", Links: []string{"https://example.com/link"}},
			Files:   []models.TorrentFile{{ID: 1, Path: "/movie.mkv", Selected: true}},
		},
	}

	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'd', Text: "d"})
	m = updated.(Model)
	if m.mode != modeDetail {
		t.Fatalf("mode = %q, want modeDetail (single target skips picker)", m.mode)
	}
	if cmd == nil {
		t.Fatal("expected resolve command when single target is available")
	}
	if !m.loading {
		t.Fatal("expected loading to be true while resolving direct download")
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

func TestBulkDownloadEligibilityRequiresTwoDownloadedSelections(t *testing.T) {
	m := Model{
		mode:          modeMain,
		batchMode:     true,
		sortColumn:    colAdded,
		sortAscending: false,
		torrents: []models.Torrent{
			{ID: "a", Filename: "ready", Status: "downloaded", Added: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)},
			{ID: "b", Filename: "queued", Status: "queued", Added: time.Date(2026, 4, 2, 0, 0, 0, 0, time.UTC)},
		},
		batchSelected: map[string]bool{"a": true, "b": true},
	}

	if m.canBulkDownloadSelection() {
		t.Fatal("expected mixed-status selection to be ineligible")
	}
	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'd', Text: "d"})
	m = updated.(Model)
	if cmd != nil {
		t.Fatal("expected no command for ineligible bulk download")
	}
	if m.mode != modeMain {
		t.Fatalf("mode = %q, want modeMain", m.mode)
	}
	if !strings.Contains(m.errText, "Mark at least two downloaded") {
		t.Fatalf("errText = %q, want eligibility error", m.errText)
	}
}

func TestBulkDownloadStartsInVisibleOrder(t *testing.T) {
	m := Model{
		mode:          modeMain,
		batchMode:     true,
		sortColumn:    colAdded,
		sortAscending: false,
		torrents: []models.Torrent{
			{ID: "b", Filename: "second", Status: "downloaded", Added: time.Date(2026, 4, 2, 0, 0, 0, 0, time.UTC)},
			{ID: "a", Filename: "first", Status: "downloaded", Added: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)},
			{ID: "c", Filename: "third", Status: "downloaded", Added: time.Date(2026, 4, 3, 0, 0, 0, 0, time.UTC)},
		},
		batchSelected: map[string]bool{"a": true, "b": true},
	}

	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'd', Text: "d"})
	m = updated.(Model)
	if cmd != nil {
		t.Fatal("expected setup to open without async command")
	}
	if m.mode != modeBulkOrder {
		t.Fatalf("mode = %q, want modeBulkOrder", m.mode)
	}
	if len(m.bulk.Plans) != 2 || m.bulk.Plans[0].ID != "b" || m.bulk.Plans[1].ID != "a" {
		t.Fatalf("bulk order = %+v, want visible order [b a]", m.bulk.Plans)
	}
}

func TestBulkFileChoiceRejectsEmptyAndBuildsConfirmation(t *testing.T) {
	m := Model{
		mode: modeBulkOrder,
		bulk: newBulkDownloadState([]models.Torrent{
			{ID: "a", Filename: "single"},
			{ID: "b", Filename: "multi"},
		}),
	}

	updated, _ := m.Update(bulkDetailsMsg{details: []models.TorrentInfo{
		{Torrent: models.Torrent{ID: "a", Filename: "single", Links: []string{"https://example.com/a"}}, Files: []models.TorrentFile{{ID: 1, Path: "a.mkv", Selected: true}}},
		{Torrent: models.Torrent{ID: "b", Filename: "multi", Links: []string{"https://example.com/b1", "https://example.com/b2"}}, Files: []models.TorrentFile{{ID: 1, Path: "b1.mkv", Selected: true}, {ID: 2, Path: "b2.mkv", Selected: true}}},
	}})
	m = updated.(Model)
	if m.mode != modeBulkFiles {
		t.Fatalf("mode = %q, want modeBulkFiles", m.mode)
	}

	m.bulk.clearCurrentFiles()
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	if m.mode != modeBulkFiles {
		t.Fatalf("mode = %q, want to stay in modeBulkFiles", m.mode)
	}
	if m.errText != "Select at least one file" {
		t.Fatalf("errText = %q, want empty selection error", m.errText)
	}

	m.bulk.toggleCurrentFile()
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	if m.mode != modeBulkConfirm {
		t.Fatalf("mode = %q, want modeBulkConfirm", m.mode)
	}
	if len(m.bulk.Items) != 2 {
		t.Fatalf("items len = %d, want single target plus selected multi target", len(m.bulk.Items))
	}
}

func TestBulkFinalConfirmationStartsFirstQueuedFile(t *testing.T) {
	m := Model{
		mode:          modeBulkConfirm,
		batchMode:     true,
		batchSelected: map[string]bool{"a": true, "b": true},
		bulk: newBulkDownloadState([]models.Torrent{
			{ID: "a", Filename: "a"},
			{ID: "b", Filename: "b"},
		}),
	}
	m.bulk.Items = []bulkQueueItem{
		{TorrentID: "a", TorrentName: "a", Target: models.DownloadTarget{Link: "https://example.com/a", Label: "a.mkv"}, Status: bulkFilePending},
		{TorrentID: "b", TorrentName: "b", Target: models.DownloadTarget{Link: "https://example.com/b", Label: "b.mkv"}, Status: bulkFilePending},
	}

	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	if cmd == nil {
		t.Fatal("expected resolve command for first queued file")
	}
	if m.mode != modeBulkDownload || m.bulk.Current != 0 || m.bulk.Items[0].Status != bulkFileActive {
		t.Fatalf("bulk state = mode %q current %d status %q", m.mode, m.bulk.Current, m.bulk.Items[0].Status)
	}
	if m.batchMode || len(m.batchSelected) != 0 {
		t.Fatal("expected batch selection cleared once bulk queue starts")
	}
}

func TestBulkQueueContinuesAfterFailure(t *testing.T) {
	m := Model{
		mode:       modeBulkDownload,
		returnMode: modeBulkDownload,
		bulk:       newBulkDownloadState([]models.Torrent{{ID: "a", Filename: "a"}, {ID: "b", Filename: "b"}}),
	}
	m.bulk.Items = []bulkQueueItem{
		{TorrentID: "a", TorrentName: "a", Target: models.DownloadTarget{Link: "https://example.com/a", Label: "a.mkv"}, Status: bulkFileActive},
		{TorrentID: "b", TorrentName: "b", Target: models.DownloadTarget{Link: "https://example.com/b", Label: "b.mkv"}, Status: bulkFilePending},
	}
	m.bulk.Current = 0

	updated, cmd := m.Update(resolvedDownloadMsg{err: errors.New("resolve failed")})
	m = updated.(Model)
	if cmd == nil {
		t.Fatal("expected command to start next queued file after failure")
	}
	if m.bulk.Items[0].Status != bulkFileFailed || m.bulk.Current != 1 || m.bulk.Items[1].Status != bulkFileActive {
		t.Fatalf("bulk items after failure = %+v current=%d", m.bulk.Items, m.bulk.Current)
	}

	updated, cmd = m.Update(managedDownloadMsg{result: models.ManagedDownloadStart{Download: models.ManagedDownload{GID: "gid", Filename: "b.mkv", Status: models.ManagedDownloadStatusActive}}})
	m = updated.(Model)
	if cmd == nil {
		t.Fatal("expected status polling after managed download starts")
	}
	updated, cmd = m.Update(managedDownloadStatusMsg{ok: true, download: models.ManagedDownload{GID: "gid", Filename: "b.mkv", Status: models.ManagedDownloadStatusComplete}})
	m = updated.(Model)
	if cmd != nil {
		t.Fatal("expected no command after queue finishes")
	}
	if !m.bulk.isFinished() || m.bulk.Items[1].Status != bulkFileSuccess {
		t.Fatalf("bulk state after completion = %+v", m.bulk.Items)
	}
}

func TestBulkCleanupDefaultsAndManualToggle(t *testing.T) {
	m := Model{
		mode: modeBulkDownload,
		bulk: newBulkDownloadState([]models.Torrent{
			{ID: "a", Filename: "complete"},
			{ID: "b", Filename: "partial"},
		}),
	}
	m.bulk.Items = []bulkQueueItem{
		{TorrentID: "a", TorrentName: "complete", Status: bulkFileSuccess},
		{TorrentID: "b", TorrentName: "partial", Status: bulkFileSuccess},
		{TorrentID: "b", TorrentName: "partial", Status: bulkFileFailed, Error: "network"},
	}

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	m = updated.(Model)
	if m.mode != modeBulkCleanup {
		t.Fatalf("mode = %q, want modeBulkCleanup", m.mode)
	}
	if !m.bulk.CleanupSelected["a"] {
		t.Fatal("complete torrent should be preselected")
	}
	if m.bulk.CleanupSelected["b"] {
		t.Fatal("partial torrent should not be preselected")
	}
	m.bulk.CleanupCursor = 1
	updated, _ = m.Update(tea.KeyPressMsg{Code: ' ', Text: " "})
	m = updated.(Model)
	if !m.bulk.CleanupSelected["b"] || !m.bulk.hasRiskyCleanupSelection() {
		t.Fatal("partial torrent should be manually toggleable and risky")
	}
}

func TestBulkDownloadEscapeReturnsToMainWhileQueueRuns(t *testing.T) {
	m := Model{
		mode:        modeBulkDownload,
		downloadDir: t.TempDir(),
		download:    &models.ManagedDownload{GID: "gid-1", Status: models.ManagedDownloadStatusActive},
		bulk:        newBulkDownloadState([]models.Torrent{{ID: "a", Filename: "a"}, {ID: "b", Filename: "b"}}),
	}
	m.bulk.Items = []bulkQueueItem{
		{TorrentID: "a", TorrentName: "a", Target: models.DownloadTarget{Link: "https://example.com/a", Label: "a.mkv"}, Status: bulkFileActive},
		{TorrentID: "b", TorrentName: "b", Target: models.DownloadTarget{Link: "https://example.com/b", Label: "b.mkv"}, Status: bulkFilePending},
	}
	m.bulk.Current = 0

	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = updated.(Model)
	if cmd != nil {
		t.Fatal("expected no command when leaving active bulk view")
	}
	if m.mode != modeMain {
		t.Fatalf("mode = %q, want modeMain", m.mode)
	}
	if m.status != "Bulk download continues in background" {
		t.Fatalf("status = %q, want background status", m.status)
	}

	updated, cmd = m.Update(downloadTickMsg(time.Now()))
	m = updated.(Model)
	if cmd == nil {
		t.Fatal("expected background bulk download polling after leaving view")
	}
	if m.mode != modeMain {
		t.Fatalf("mode = %q, want to stay in modeMain while polling", m.mode)
	}

	updated, cmd = m.Update(managedDownloadStatusMsg{ok: true, download: models.ManagedDownload{GID: "gid-1", Filename: "a.mkv", Status: models.ManagedDownloadStatusComplete}})
	m = updated.(Model)
	if cmd == nil {
		t.Fatal("expected command to start next queued file in background")
	}
	if m.mode != modeMain {
		t.Fatalf("mode = %q, want background queue to preserve modeMain", m.mode)
	}
	if m.bulk.Items[0].Status != bulkFileSuccess || m.bulk.Current != 1 || m.bulk.Items[1].Status != bulkFileActive {
		t.Fatalf("bulk state = current %d items %+v", m.bulk.Current, m.bulk.Items)
	}

	updated, cmd = m.Update(resolvedDownloadMsg{url: "https://example.com/b", filename: "b.mkv", filesize: 100})
	m = updated.(Model)
	if cmd == nil {
		t.Fatal("expected command to start resolved background bulk item")
	}
	if m.mode != modeMain {
		t.Fatalf("mode = %q, want resolved background item to preserve modeMain", m.mode)
	}

	updated, cmd = m.Update(managedDownloadMsg{result: models.ManagedDownloadStart{Download: models.ManagedDownload{GID: "gid-2", Filename: "b.mkv", Status: models.ManagedDownloadStatusActive}}})
	m = updated.(Model)
	if cmd == nil {
		t.Fatal("expected polling command after background managed download starts")
	}
	if m.mode != modeMain {
		t.Fatalf("mode = %q, want background managed download start to preserve modeMain", m.mode)
	}

	updated, cmd = m.Update(managedDownloadStatusMsg{ok: true, download: models.ManagedDownload{GID: "gid-2", Filename: "b.mkv", Status: models.ManagedDownloadStatusComplete}})
	m = updated.(Model)
	if cmd != nil {
		t.Fatal("expected no command after background queue finishes")
	}
	if m.mode != modeMain {
		t.Fatalf("mode = %q, want finished background queue to preserve modeMain", m.mode)
	}
	if !m.bulk.isFinished() || m.bulk.Items[1].Status != bulkFileSuccess {
		t.Fatalf("bulk state after background completion = current %d items %+v", m.bulk.Current, m.bulk.Items)
	}
}

func TestBulkSelectFilesStartsWithEligibleAndSkippedSelections(t *testing.T) {
	m := Model{
		mode:          modeMain,
		batchMode:     true,
		sortColumn:    colAdded,
		sortAscending: false,
		torrents: []models.Torrent{
			{ID: "a", Filename: "waiting", Status: "waiting_files_selection", Added: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)},
			{ID: "b", Filename: "ready", Status: "downloaded", Added: time.Date(2026, 4, 2, 0, 0, 0, 0, time.UTC)},
			{ID: "c", Filename: "queued", Status: "queued", Added: time.Date(2026, 4, 3, 0, 0, 0, 0, time.UTC)},
		},
		batchSelected: map[string]bool{"a": true, "b": true, "c": true},
	}

	if !m.canBulkSelectFilesSelection() {
		t.Fatal("expected at least one eligible marked torrent")
	}
	updated, cmd := m.Update(tea.KeyPressMsg{Code: 's', Text: "s"})
	m = updated.(Model)
	if cmd == nil {
		t.Fatal("expected command to load eligible torrent details")
	}
	if m.mode != modeBulkSelectFiles || m.bulkSelect == nil {
		t.Fatalf("mode = %q bulkSelect nil=%v, want bulk file-selection setup", m.mode, m.bulkSelect == nil)
	}
	if len(m.bulkSelect.Plans) != 1 || m.bulkSelect.Plans[0].ID != "a" {
		t.Fatalf("plans = %+v, want only eligible torrent a", m.bulkSelect.Plans)
	}
	if len(m.bulkSelect.Outcomes) != 2 || m.bulkSelect.Outcomes[0].Status != bulkFileSelectionSkipped || m.bulkSelect.Outcomes[1].Status != bulkFileSelectionSkipped {
		t.Fatalf("outcomes = %+v, want two skipped torrents", m.bulkSelect.Outcomes)
	}
}

func TestBulkSelectFilesRejectsSelectionWithNoEligibleTorrents(t *testing.T) {
	m := Model{
		mode:          modeMain,
		batchMode:     true,
		torrents:      []models.Torrent{{ID: "a", Filename: "ready", Status: "downloaded"}},
		batchSelected: map[string]bool{"a": true},
	}

	updated, cmd := m.Update(tea.KeyPressMsg{Code: 's', Text: "s"})
	m = updated.(Model)
	if cmd != nil {
		t.Fatal("expected no command when no marked torrent needs file selection")
	}
	if m.mode != modeMain || m.errText != "Mark at least one torrent that needs file selection" {
		t.Fatalf("mode=%q errText=%q", m.mode, m.errText)
	}
}

func TestBulkSelectFilesUsesFilteredVisibleOrder(t *testing.T) {
	m := Model{
		mode:          modeMain,
		batchMode:     true,
		filterApplied: true,
		torrents: []models.Torrent{
			{ID: "a", Filename: "alpha", Status: "waiting_files_selection"},
			{ID: "b", Filename: "hidden", Status: "waiting_files_selection"},
			{ID: "c", Filename: "charlie", Status: "magnet_conversion"},
		},
		filteredTorrents: []models.Torrent{
			{ID: "c", Filename: "charlie", Status: "magnet_conversion"},
			{ID: "a", Filename: "alpha", Status: "waiting_files_selection"},
		},
		batchSelected: map[string]bool{"a": true, "b": true, "c": true},
	}

	updated, _ := m.Update(tea.KeyPressMsg{Code: 's', Text: "s"})
	m = updated.(Model)
	if len(m.bulkSelect.Plans) != 2 || m.bulkSelect.Plans[0].ID != "c" || m.bulkSelect.Plans[1].ID != "a" {
		t.Fatalf("plans = %+v, want filtered visible order [c a]", m.bulkSelect.Plans)
	}
}

func TestBulkSelectFilesDetailsDefaultAndEmptySelectionValidation(t *testing.T) {
	m := Model{
		mode:       modeBulkSelectFiles,
		batchMode:  true,
		bulkSelect: newBulkFileSelectionState([]models.Torrent{{ID: "a", Filename: "alpha"}}, nil),
	}

	updated, _ := m.Update(bulkFileSelectionDetailsMsg{details: []models.TorrentInfo{{
		Torrent: models.Torrent{ID: "a", Filename: "alpha"},
		Files: []models.TorrentFile{
			{ID: 1, Path: "small.mkv", Bytes: 100},
			{ID: 2, Path: "large.mkv", Bytes: 200},
		},
	}}})
	m = updated.(Model)
	if m.mode != modeBulkSelectFiles {
		t.Fatalf("mode = %q, want modeBulkSelectFiles", m.mode)
	}
	if !m.bulkSelect.Plans[0].Selected[2] || m.bulkSelect.Plans[0].Selected[1] {
		t.Fatalf("selected = %+v, want only largest file preselected", m.bulkSelect.Plans[0].Selected)
	}

	m.bulkSelect.clearCurrentFiles()
	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	if cmd != nil {
		t.Fatal("expected no submit command for empty file selection")
	}
	if m.mode != modeBulkSelectFiles || m.errText != "Select at least one file" {
		t.Fatalf("mode=%q errText=%q", m.mode, m.errText)
	}
}

func TestBulkSelectFilesCancelKeepsBatchSelection(t *testing.T) {
	m := Model{
		mode:          modeBulkSelectFiles,
		batchMode:     true,
		batchSelected: map[string]bool{"a": true},
		bulkSelect:    newBulkFileSelectionState([]models.Torrent{{ID: "a", Filename: "alpha"}}, nil),
	}

	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = updated.(Model)
	if cmd != nil {
		t.Fatal("expected no command when cancelling bulk file selection setup")
	}
	if m.mode != modeMain || !m.batchMode || !m.batchSelected["a"] || m.bulkSelect != nil {
		t.Fatalf("mode=%q batchMode=%v selected=%+v bulkSelect=%+v", m.mode, m.batchMode, m.batchSelected, m.bulkSelect)
	}
}

func TestBulkSelectFilesSubmitsSequentiallyAndReportsPartialFailure(t *testing.T) {
	service := &bulkFileSelectionTestService{
		selectErr: map[string]error{"b": errors.New("denied")},
		selected:  map[string][]int{},
	}
	m := Model{
		mode:          modeBulkSelectFiles,
		service:       service,
		batchMode:     true,
		batchSelected: map[string]bool{"a": true, "b": true, "c": true},
		bulkSelect: &bulkFileSelectionState{
			Prompt: 1,
			Plans: []bulkFileSelectionPlan{
				{ID: "a", Name: "alpha", Files: []models.TorrentFile{{ID: 1, Path: "a.mkv"}}, Selected: map[int]bool{1: true}},
				{ID: "b", Name: "beta", Files: []models.TorrentFile{{ID: 2, Path: "b.mkv"}}, Selected: map[int]bool{2: true}},
			},
			Outcomes: []bulkFileSelectionOutcome{{ID: "c", Name: "complete", Status: bulkFileSelectionSkipped}},
		},
	}

	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	if cmd == nil {
		t.Fatal("expected bulk SelectFiles command")
	}
	updated, cmd = m.Update(cmd())
	m = updated.(Model)
	if cmd == nil {
		t.Fatal("expected refresh/flash batch command after bulk selection result")
	}
	if len(service.selected["a"]) != 1 || service.selected["a"][0] != 1 || len(service.selected["b"]) != 1 || service.selected["b"][0] != 2 {
		t.Fatalf("selected submissions = %+v", service.selected)
	}
	if m.mode != modeMain || m.batchMode || len(m.batchSelected) != 0 || m.bulkSelect != nil {
		t.Fatalf("mode=%q batchMode=%v selected=%+v bulkSelect=%+v", m.mode, m.batchMode, m.batchSelected, m.bulkSelect)
	}
	if !strings.Contains(m.flash.message, "1 updated, 1 failed, 1 skipped") {
		t.Fatalf("flash = %q, want result counts", m.flash.message)
	}
	if !strings.Contains(m.errText, "beta: denied") {
		t.Fatalf("errText = %q, want failed torrent detail", m.errText)
	}
}

type bulkFileSelectionTestService struct {
	details   map[string]models.TorrentInfo
	detailErr map[string]error
	selectErr map[string]error
	selected  map[string][]int
}

func (s *bulkFileSelectionTestService) Config() config.Config { return config.Config{} }

func (s *bulkFileSelectionTestService) Bootstrap(context.Context) (*models.AuthSession, error) {
	return nil, nil
}

func (s *bulkFileSelectionTestService) AuthenticateWithToken(context.Context, string, bool) (*models.AuthSession, error) {
	return nil, nil
}

func (s *bulkFileSelectionTestService) StartDeviceFlow(context.Context) (models.DeviceCode, error) {
	return models.DeviceCode{}, nil
}

func (s *bulkFileSelectionTestService) CompleteDeviceFlow(context.Context, models.DeviceCode) (*models.AuthSession, error) {
	return nil, nil
}

func (s *bulkFileSelectionTestService) ListTorrents(context.Context) ([]models.Torrent, error) {
	return nil, nil
}

func (s *bulkFileSelectionTestService) TorrentInfo(_ context.Context, id string) (models.TorrentInfo, error) {
	if err := s.detailErr[id]; err != nil {
		return models.TorrentInfo{}, err
	}
	return s.details[id], nil
}

func (s *bulkFileSelectionTestService) AddMagnet(context.Context, string) (models.AddTorrentResult, error) {
	return models.AddTorrentResult{}, nil
}

func (s *bulkFileSelectionTestService) AddTorrentURL(context.Context, string) (models.AddTorrentResult, error) {
	return models.AddTorrentResult{}, nil
}

func (s *bulkFileSelectionTestService) ImportTorrentFiles(context.Context, []string) []models.ImportResult {
	return nil
}

func (s *bulkFileSelectionTestService) SelectFiles(_ context.Context, torrentID string, ids []int) error {
	if s.selected == nil {
		s.selected = map[string][]int{}
	}
	s.selected[torrentID] = append([]int(nil), ids...)
	return s.selectErr[torrentID]
}

func (s *bulkFileSelectionTestService) DeleteTorrent(context.Context, string) error { return nil }

func (s *bulkFileSelectionTestService) ResolveDirectURL(context.Context, models.DownloadTarget) (models.UnrestrictedLink, error) {
	return models.UnrestrictedLink{}, nil
}

func (s *bulkFileSelectionTestService) CopyURL(string) (bool, error) { return false, nil }

func (s *bulkFileSelectionTestService) StartManagedDownload(context.Context, string, string) (models.ManagedDownloadStart, error) {
	return models.ManagedDownloadStart{}, nil
}

func (s *bulkFileSelectionTestService) ManagedDownloadStatus(context.Context) (models.ManagedDownload, bool, error) {
	return models.ManagedDownload{}, false, nil
}

func (s *bulkFileSelectionTestService) SavePrivateToken(string) error { return nil }

func (s *bulkFileSelectionTestService) OpenFile(string) error { return nil }

func (s *bulkFileSelectionTestService) RevealInDirectory(string) error { return nil }

func (s *bulkFileSelectionTestService) Close() error { return nil }
