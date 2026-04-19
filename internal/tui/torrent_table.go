package tui

import (
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/m4rii0/rdtui/pkg/models"
)

const (
	colStatus   = 0
	colProgress = 1
	colSize     = 2
	colName     = 3
	colAdded    = 4
	columnCount = 5
)

type columnSpec struct {
	Index      int
	Label      string
	Width      int
	AlignRight bool
}

func tableColumns(totalWidth int, showScrollbar bool) []columnSpec {
	statusW := 10
	progressW := 6
	sizeW := 9
	addedW := 16
	scrollW := 0
	if showScrollbar {
		scrollW = 2
	}
	used := statusW + progressW + sizeW + addedW + 4 + scrollW
	nameW := totalWidth - used
	if nameW < 8 {
		statusW = 8
		progressW = 5
		sizeW = 8
		addedW = 8
		used = statusW + progressW + sizeW + addedW + 4 + scrollW
		nameW = totalWidth - used
	}
	if nameW < 4 {
		nameW = 4
	}
	return []columnSpec{
		{Index: colStatus, Label: "Status", Width: statusW},
		{Index: colProgress, Label: "%", Width: progressW, AlignRight: true},
		{Index: colSize, Label: "Size", Width: sizeW, AlignRight: true},
		{Index: colName, Label: "Name", Width: nameW},
		{Index: colAdded, Label: "Added", Width: addedW, AlignRight: true},
	}
}

func columnLabel(col int) string {
	switch col {
	case colStatus:
		return "Status"
	case colProgress:
		return "Progress"
	case colSize:
		return "Size"
	case colAdded:
		return "Added"
	case colName:
		return "Name"
	default:
		return ""
	}
}

func sortTorrents(torrents []models.Torrent, col int, ascending bool) {
	sort.SliceStable(torrents, func(i, j int) bool {
		a, b := torrents[i], torrents[j]
		cmp := compareByColumn(a, b, col)
		if cmp != 0 {
			if ascending {
				return cmp < 0
			}
			return cmp > 0
		}
		if !a.Added.Equal(b.Added) {
			return a.Added.After(b.Added)
		}
		return a.Filename < b.Filename
	})
}

func compareByColumn(a, b models.Torrent, col int) int {
	switch col {
	case colStatus:
		return torrentStatusRank(a.Status) - torrentStatusRank(b.Status)
	case colProgress:
		if a.Progress < b.Progress {
			return -1
		}
		if a.Progress > b.Progress {
			return 1
		}
		return 0
	case colSize:
		if a.Bytes < b.Bytes {
			return -1
		}
		if a.Bytes > b.Bytes {
			return 1
		}
		return 0
	case colAdded:
		if a.Added.Before(b.Added) {
			return -1
		}
		if a.Added.After(b.Added) {
			return 1
		}
		return 0
	case colName:
		if a.Filename < b.Filename {
			return -1
		}
		if a.Filename > b.Filename {
			return 1
		}
		return 0
	default:
		return 0
	}
}

func torrentStatusRank(status string) int {
	switch status {
	case "downloading":
		return 0
	case "queued", "compressing", "uploading":
		return 1
	case "waiting_files_selection":
		return 2
	case "downloaded":
		return 3
	case "error", "dead", "virus", "magnet_error":
		return 5
	default:
		return 4
	}
}

func renderTableHeader(columns []columnSpec, sortCol int, sortAsc bool, width int) string {
	cells := make([]string, len(columns))
	for i, col := range columns {
		label := col.Label
		if col.Index == sortCol {
			if sortAsc {
				label += "↑"
			} else {
				label += "↓"
			}
		}
		padded := padVisual(label, col.Width, col.AlignRight)
		if col.Index == sortCol {
			cells[i] = headerSelColStyle.Render(padded)
		} else {
			cells[i] = headerRowStyle.Render(padded)
		}
	}
	return truncateLine(strings.Join(cells, " "), width)
}

func renderTableRow(torrent models.Torrent, columns []columnSpec) string {
	cells := make([]string, len(columns))
	for i, col := range columns {
		var value string
		switch col.Index {
		case colStatus:
			value = compactTorrentStatus(torrent.Status)
		case colProgress:
			value = formatProgress(torrent.Progress)
		case colSize:
			value = humanBytes(torrent.Bytes)
		case colAdded:
			value = formatAddedTime(torrent.Added)
		case colName:
			value = torrent.Filename
		}
		cells[i] = padVisual(value, col.Width, col.AlignRight)
	}
	return strings.Join(cells, " ")
}

func padVisual(content string, width int, alignRight bool) string {
	visual := lipgloss.Width(content)
	if visual > width {
		return ansi.Truncate(content, width, "…")
	}
	pad := strings.Repeat(" ", width-visual)
	if alignRight {
		return pad + content
	}
	return content + pad
}
