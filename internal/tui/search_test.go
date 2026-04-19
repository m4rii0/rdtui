package tui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/m4rii0/rdtui/pkg/models"
)

func TestTorrentMatchString(t *testing.T) {
	torrent := models.Torrent{
		Filename: "Big Buck Bunny",
		Status:   "downloaded",
		Progress: 100,
		Bytes:    1024 * 1024 * 500,
		Added:    time.Date(2026, 4, 15, 10, 30, 0, 0, time.UTC),
	}
	got := torrentMatchString(torrent)
	for _, substr := range []string{"DONE", "100", "Big Buck Bunny", "500.0 MB", "15/04/2026"} {
		if !contains(got, substr) {
			t.Errorf("torrentMatchString missing %q in %q", substr, got)
		}
	}
}

func TestTorrentMatchStringStatuses(t *testing.T) {
	tests := []struct {
		status string
		label  string
	}{
		{"downloaded", "DONE"},
		{"downloading", "DL"},
		{"queued", "QD"},
		{"error", "ERR"},
		{"dead", "DEAD"},
		{"waiting_files_selection", "WAIT"},
	}
	for _, tt := range tests {
		torrent := models.Torrent{Status: tt.status, Added: time.Now()}
		got := torrentMatchString(torrent)
		if !contains(got, tt.label) {
			t.Errorf("status %q: expected label %q in %q", tt.status, tt.label, got)
		}
	}
}

func TestFilterTorrentsFuzzy(t *testing.T) {
	torrents := []models.Torrent{
		{ID: "1", Filename: "Big Buck Bunny", Status: "downloaded", Progress: 100, Bytes: 1024, Added: time.Now()},
		{ID: "2", Filename: "Sintel Movie", Status: "downloading", Progress: 50, Bytes: 2048, Added: time.Now()},
		{ID: "3", Filename: "Elephant Dream", Status: "error", Progress: 0, Bytes: 4096, Added: time.Now()},
	}

	result := filterTorrents(torrents, "bunny")
	if len(result) != 1 || result[0].ID != "1" {
		t.Fatalf("filter 'bunny': got %d results, want 1 with ID=1", len(result))
	}

	result = filterTorrents(torrents, "DONE")
	if len(result) != 1 || result[0].ID != "1" {
		t.Fatalf("filter 'DONE': got %d results, want 1 with ID=1", len(result))
	}

	result = filterTorrents(torrents, "movie")
	if len(result) != 1 || result[0].ID != "2" {
		t.Fatalf("filter 'movie': got %d results, want 1 with ID=2", len(result))
	}

	result = filterTorrents(torrents, "xyz")
	if len(result) != 0 {
		t.Fatalf("filter 'xyz': got %d results, want 0", len(result))
	}

	result = filterTorrents(torrents, "")
	if len(result) != 0 {
		t.Fatalf("filter '': got %d results, want 0 (empty query returns nothing)", len(result))
	}
}

func TestSlashEntersSearchMode(t *testing.T) {
	m := NewModel(nil)
	m.mode = modeMain
	m.torrents = []models.Torrent{
		{ID: "a", Filename: "test", Added: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)},
	}
	m.selectedIdx = 0

	updated, _ := m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	m = updated.(Model)
	if m.mode != modeSearch {
		t.Fatalf("mode = %q, want modeSearch after /", m.mode)
	}
	vis := m.visibleTorrents()
	if len(vis) != 1 {
		t.Fatalf("visible count = %d, want 1 (all results shown on search entry)", len(vis))
	}
}

func TestSlashPreservesQueryWhenFilterApplied(t *testing.T) {
	m := NewModel(nil)
	m.mode = modeMain
	m.torrents = []models.Torrent{
		{ID: "a", Filename: "test", Added: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)},
	}
	m.selectedIdx = 0
	m.filterApplied = true
	m.searchInput.SetValue("test")

	updated, _ := m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	m = updated.(Model)
	if m.mode != modeSearch {
		t.Fatalf("mode = %q, want modeSearch", m.mode)
	}
	if m.searchInput.Value() != "test" {
		t.Fatalf("searchInput value = %q, want 'test' (preserved)", m.searchInput.Value())
	}
}

func TestEscapeClearsFilter(t *testing.T) {
	m := Model{
		mode:          modeSearch,
		sortColumn:    colAdded,
		sortAscending: false,
		torrents: []models.Torrent{
			{ID: "a", Filename: "test", Added: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)},
		},
		filteredTorrents: []models.Torrent{
			{ID: "a", Filename: "test", Added: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)},
		},
		selectedIdx:   0,
		batchSelected: map[string]bool{},
	}
	m.searchInput.SetValue("test")

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = updated.(Model)
	if m.mode != modeMain {
		t.Fatalf("mode = %q, want modeMain after Esc", m.mode)
	}
	if m.filterApplied {
		t.Fatal("filterApplied should be false after Esc")
	}
	if m.searchInput.Value() != "" {
		t.Fatalf("searchInput value = %q, want empty after Esc", m.searchInput.Value())
	}
	if m.selectedIdx != 0 {
		t.Fatalf("selectedIdx = %d, want 0 after Esc", m.selectedIdx)
	}
}

func TestEnterKeepsFilter(t *testing.T) {
	m := Model{
		mode:          modeSearch,
		sortColumn:    colAdded,
		sortAscending: false,
		torrents: []models.Torrent{
			{ID: "a", Filename: "alpha test", Added: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)},
			{ID: "b", Filename: "beta file", Added: time.Date(2026, 4, 2, 0, 0, 0, 0, time.UTC)},
		},
		filteredTorrents: []models.Torrent{
			{ID: "a", Filename: "alpha test", Added: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)},
		},
		selectedIdx:   0,
		batchSelected: map[string]bool{},
	}
	m.searchInput.SetValue("alpha")

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	if m.mode != modeMain {
		t.Fatalf("mode = %q, want modeMain after Enter", m.mode)
	}
	if !m.filterApplied {
		t.Fatal("filterApplied should be true after Enter with query")
	}
	if m.searchInput.Value() != "alpha" {
		t.Fatalf("searchInput value = %q, want 'alpha'", m.searchInput.Value())
	}
	vis := m.visibleTorrents()
	if len(vis) != 1 || vis[0].ID != "a" {
		t.Fatalf("visible = %v, want [{a}]", vis)
	}
}

func TestEnterWithEmptyQueryClearsFilter(t *testing.T) {
	m := Model{
		mode:          modeSearch,
		sortColumn:    colAdded,
		sortAscending: false,
		torrents: []models.Torrent{
			{ID: "a", Filename: "test", Added: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)},
		},
		selectedIdx:   0,
		batchSelected: map[string]bool{},
	}
	m.searchInput.Reset()

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	if m.mode != modeMain {
		t.Fatalf("mode = %q, want modeMain", m.mode)
	}
	if m.filterApplied {
		t.Fatal("filterApplied should be false with empty query")
	}
}

func TestSelectedIdxClampedAfterFilter(t *testing.T) {
	m := Model{
		mode:          modeSearch,
		sortColumn:    colAdded,
		sortAscending: false,
		torrents: []models.Torrent{
			{ID: "a", Filename: "alpha", Added: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)},
			{ID: "b", Filename: "beta", Added: time.Date(2026, 4, 2, 0, 0, 0, 0, time.UTC)},
			{ID: "c", Filename: "gamma", Added: time.Date(2026, 4, 3, 0, 0, 0, 0, time.UTC)},
		},
		selectedIdx:   2,
		batchSelected: map[string]bool{},
	}
	m.searchInput.Reset()
	m.applyFilter()

	m.searchInput.SetValue("alpha")
	m.applyFilter()

	vis := m.visibleTorrents()
	if len(vis) != 1 {
		t.Fatalf("visible count = %d, want 1", len(vis))
	}
	if m.selectedIdx != 0 {
		t.Fatalf("selectedIdx = %d, want 0 (clamped)", m.selectedIdx)
	}
}

func TestFilteredResultsRespectSortOrder(t *testing.T) {
	m := Model{
		mode:          modeSearch,
		sortColumn:    colName,
		sortAscending: true,
		torrents: []models.Torrent{
			{ID: "c", Filename: "charlie", Bytes: 30, Added: time.Date(2026, 4, 3, 0, 0, 0, 0, time.UTC)},
			{ID: "a", Filename: "alpha", Bytes: 10, Added: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)},
			{ID: "b", Filename: "bravo", Bytes: 20, Added: time.Date(2026, 4, 2, 0, 0, 0, 0, time.UTC)},
		},
		selectedIdx:   0,
		batchSelected: map[string]bool{},
	}
	m.searchInput.Reset()
	m.applyFilter()

	vis := m.visibleTorrents()
	if len(vis) != 3 {
		t.Fatalf("visible count = %d, want 3", len(vis))
	}
	if vis[0].ID != "a" || vis[1].ID != "b" || vis[2].ID != "c" {
		t.Fatalf("order = %v, want [a b c] (name ascending)", idsOf(vis))
	}
}

func TestVisibleTorrentsReturnsFullListWhenNoFilter(t *testing.T) {
	m := Model{
		mode:       modeMain,
		torrents:   []models.Torrent{{ID: "a"}, {ID: "b"}},
	}
	vis := m.visibleTorrents()
	if len(vis) != 2 {
		t.Fatalf("visible count = %d, want 2", len(vis))
	}
}

func TestVisibleTorrentsReturnsFilteredList(t *testing.T) {
	m := Model{
		mode:             modeMain,
		filterApplied:    true,
		torrents:         []models.Torrent{{ID: "a"}, {ID: "b"}, {ID: "c"}},
		filteredTorrents: []models.Torrent{{ID: "a"}},
	}
	vis := m.visibleTorrents()
	if len(vis) != 1 || vis[0].ID != "a" {
		t.Fatalf("visible = %v, want [{a}]", vis)
	}
}

func contains(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func TestHighlightCharsNoIndices(t *testing.T) {
	got := highlightChars("hello", nil, matchHighlightStyle)
	if got != "hello" {
		t.Fatalf("got %q, want %q", got, "hello")
	}
}

func TestHighlightCharsWithIndices(t *testing.T) {
	style := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("226"))
	got := highlightChars("hello", []int{0, 1}, style)
	plain := stripAnsi(got)
	if plain != "hello" {
		t.Fatalf("stripped = %q, want %q", plain, "hello")
	}
	gotNoStyle := highlightChars("hello", []int{0, 1}, lipgloss.NewStyle())
	if gotNoStyle != "hello" {
		t.Fatalf("empty style should not change string, got %q", gotNoStyle)
	}
	gotNoIndices := highlightChars("hello", nil, style)
	if gotNoIndices != "hello" {
		t.Fatalf("no indices should not change string, got %q", gotNoIndices)
	}
}

func TestFieldMatchIndices(t *testing.T) {
	torrent := models.Torrent{
		Filename: "Big Buck Bunny",
		Status:   "downloaded",
		Progress: 100,
		Bytes:    1024,
		Added:    time.Date(2026, 4, 15, 10, 30, 0, 0, time.UTC),
	}
	matchStr := torrentMatchString(torrent)
	filenameOffset := strings.Index(matchStr, "Big Buck Bunny")

	indices := []int{filenameOffset + 0, filenameOffset + 1, filenameOffset + 4}
	fm := fieldMatchIndices(torrent, indices)

	if len(fm[colName]) != 3 {
		t.Fatalf("name matches = %v, want 3 indices", fm[colName])
	}
	if fm[colName][0] != 0 || fm[colName][1] != 1 || fm[colName][2] != 4 {
		t.Fatalf("name local indices = %v, want [0 1 4]", fm[colName])
	}
	if len(fm[colStatus]) != 0 {
		t.Fatalf("status matches = %v, want none", fm[colStatus])
	}
}

func TestFilterTorrentsWithMatches(t *testing.T) {
	torrents := []models.Torrent{
		{ID: "1", Filename: "Big Buck Bunny", Status: "downloaded", Added: time.Now()},
		{ID: "2", Filename: "Sintel Movie", Status: "downloading", Added: time.Now()},
	}
	results := filterTorrentsWithMatches(torrents, "bunny")
	if len(results) != 1 {
		t.Fatalf("results = %d, want 1", len(results))
	}
	if results[0].torrent.ID != "1" {
		t.Fatalf("torrent ID = %q, want 1", results[0].torrent.ID)
	}
	if len(results[0].indices) == 0 {
		t.Fatal("expected non-empty match indices")
	}
}

func TestApplyFilterPopulatesMatchMap(t *testing.T) {
	m := Model{
		mode:          modeSearch,
		sortColumn:    colAdded,
		sortAscending: false,
		torrents: []models.Torrent{
			{ID: "a", Filename: "alpha test", Added: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)},
			{ID: "b", Filename: "beta file", Added: time.Date(2026, 4, 2, 0, 0, 0, 0, time.UTC)},
		},
		selectedIdx: 0,
	}
	m.searchInput = NewModel(nil).searchInput
	m.searchInput.SetValue("alpha")
	m.applyFilter()

	if m.filterMatches == nil {
		t.Fatal("filterMatches should not be nil with active query")
	}
	if len(m.filterMatches["a"]) == 0 {
		t.Fatal("torrent 'a' should have match indices")
	}
	if _, ok := m.filterMatches["b"]; ok {
		t.Fatal("torrent 'b' should not be in filterMatches (not matched)")
	}
}

func TestApplyFilterClearsMatchMapOnEmptyQuery(t *testing.T) {
	m := Model{
		mode:          modeSearch,
		sortColumn:    colAdded,
		sortAscending: false,
		torrents: []models.Torrent{
			{ID: "a", Filename: "alpha test", Added: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)},
		},
		selectedIdx: 0,
	}
	m.searchInput = NewModel(nil).searchInput
	m.searchInput.Reset()
	m.applyFilter()

	if m.filterMatches != nil {
		t.Fatal("filterMatches should be nil with empty query")
	}
}

func stripAnsi(s string) string {
	var b strings.Builder
	esc := false
	for _, r := range s {
		if r == '\x1b' {
			esc = true
			continue
		}
		if esc {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				esc = false
			}
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func idsOf(torrents []models.Torrent) []string {
	ids := make([]string, len(torrents))
	for i, t := range torrents {
		ids[i] = t.ID
	}
	return ids
}
