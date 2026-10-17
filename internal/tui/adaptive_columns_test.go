package tui

import (
	"slices"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/m4rii0/rdtui/pkg/models"
)

func TestAdaptiveColumns(t *testing.T) {
	for _, tt := range []struct {
		width   int
		indices []int
	}{
		{1, []int{colName}}, {12, []int{colStatus, colName}}, {24, []int{colStatus, colName}},
		{39, []int{colStatus, colName}}, {40, []int{colStatus, colProgress, colName}},
		{59, []int{colStatus, colProgress, colName}}, {60, []int{colStatus, colProgress, colSize, colName}},
		{79, []int{colStatus, colProgress, colSize, colName}}, {80, []int{colStatus, colProgress, colSize, colName, colAdded}},
	} {
		for _, scroll := range []bool{false, true} {
			width := tt.width
			if scroll {
				width += 2
			}
			cols := tableColumns(width, scroll)
			var got []int
			for _, c := range cols {
				got = append(got, c.Index)
				if c.Width < 0 {
					t.Fatal("negative column width")
				}
			}
			if !slices.Equal(got, tt.indices) {
				t.Errorf("width=%d scroll=%v columns=%v want=%v", width, scroll, got, tt.indices)
			}
			row := renderTableRow(models.Torrent{Filename: strings.Repeat("映像", 40), Status: "waiting_files_selection"}, cols, nil, false)
			if lipgloss.Width(row) > tt.width {
				t.Errorf("row overflows usable width %d: %d", tt.width, lipgloss.Width(row))
			}
		}
	}
}

func TestAdaptiveColumnHighlights(t *testing.T) {
	torrent := models.Torrent{ID: "a", Filename: "映像 Movie", Status: "downloaded", Bytes: 2048}
	results := filterTorrentsWithMatches([]models.Torrent{torrent}, "Movie")
	if len(results) != 1 {
		t.Fatal("missing fuzzy match")
	}
	row := renderTableRow(torrent, tableColumns(39, false), results[0].indices, false)
	if !strings.Contains(ansi.Strip(row), "映像 Movie") || !strings.Contains(row, "\x1b[") {
		t.Fatalf("name match lost: %q", row)
	}
	rows := []models.Torrent{{ID: "a", Bytes: 2}, {ID: "b", Bytes: 1}}
	sortTorrents(rows, colSize, true)
	if rows[0].ID != "b" {
		t.Fatal("hidden size column not sortable")
	}
}
