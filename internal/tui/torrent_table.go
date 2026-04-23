package tui

import (
	"sort"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/m4rii0/rdtui/pkg/models"
	"github.com/sahilm/fuzzy"
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
	case "waiting_files_selection", "magnet_conversion":
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

func renderTableRow(torrent models.Torrent, columns []columnSpec, matchIndices []int, selected bool) string {
	var fm map[int][]int
	if len(matchIndices) > 0 {
		fm = fieldMatchIndices(torrent, matchIndices)
	}
	cells := make([]string, len(columns))
	for i, col := range columns {
		var value string
		switch col.Index {
		case colStatus:
			plain := statusLabelPlain(torrent.Status)
			if fm != nil {
				plain = highlightChars(plain, fm[colStatus], matchHighlightStyle)
			}
			value = styledStatusLabel(torrent.Status, plain)
		case colProgress:
			value = formatProgress(torrent.Progress)
			if fm != nil {
				value = highlightChars(value, fm[colProgress], matchHighlightStyle)
			}
		case colSize:
			value = humanBytes(torrent.Bytes)
			if fm != nil {
				value = highlightChars(value, fm[colSize], matchHighlightStyle)
			}
		case colAdded:
			value = formatAddedTime(torrent.Added)
			if fm != nil {
				value = highlightChars(value, fm[colAdded], matchHighlightStyle)
			}
		case colName:
			value = torrent.Filename
			if selected {
				value = selectedStyle.Render(value)
			}
			if fm != nil {
				value = highlightChars(value, fm[colName], matchHighlightStyle)
			}
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

func statusLabelPlain(status string) string {
	switch status {
	case "downloaded":
		return "✓ DONE"
	case "downloading":
		return "● DL"
	case "queued":
		return "◌ QD"
	case "compressing":
		return "◌ CMP"
	case "uploading":
		return "◌ UL"
	case "waiting_files_selection":
		return "◐ WAIT"
	case "magnet_conversion":
		return "◐ MAG"
	case "error":
		return "✗ ERR"
	case "dead":
		return "✗ DEAD"
	case "virus":
		return "✗ VIRUS"
	case "magnet_error":
		return "✗ MAGERR"
	default:
		return status
	}
}

// statusLabelForSearch returns ASCII-only labels used in torrentMatchString and
// fieldMatchIndices so that byte indices from fuzzy.Find align with rune counts.
func statusLabelForSearch(status string) string {
	switch status {
	case "downloaded":
		return "DONE"
	case "downloading":
		return "DL"
	case "queued":
		return "QD"
	case "compressing":
		return "CMP"
	case "uploading":
		return "UL"
	case "waiting_files_selection":
		return "WAIT"
	case "magnet_conversion":
		return "MAG"
	case "error":
		return "ERR"
	case "dead":
		return "DEAD"
	case "virus":
		return "VIRUS"
	case "magnet_error":
		return "MAGERR"
	default:
		return status
	}
}

func torrentMatchString(t models.Torrent) string {
	return strings.Join([]string{
		statusLabelForSearch(t.Status),
		formatProgress(t.Progress),
		humanBytes(t.Bytes),
		t.Filename,
		formatAddedTime(t.Added),
	}, " ")
}

func filterTorrents(torrents []models.Torrent, query string) []models.Torrent {
	source := make([]string, len(torrents))
	for i, t := range torrents {
		source[i] = torrentMatchString(t)
	}
	matches := fuzzy.Find(query, source)
	result := make([]models.Torrent, 0, len(matches))
	for _, match := range matches {
		result = append(result, torrents[match.Index])
	}
	return result
}

type filterResult struct {
	torrent models.Torrent
	indices []int
}

func filterTorrentsWithMatches(torrents []models.Torrent, query string) []filterResult {
	source := make([]string, len(torrents))
	for i, t := range torrents {
		source[i] = torrentMatchString(t)
	}
	matches := fuzzy.Find(query, source)
	result := make([]filterResult, len(matches))
	for i, match := range matches {
		result[i] = filterResult{torrent: torrents[match.Index], indices: match.MatchedIndexes}
	}
	return result
}

func (m Model) visibleTorrents() []models.Torrent {
	if m.filterApplied || m.mode == modeSearch {
		return m.filteredTorrents
	}
	return m.torrents
}

func (m *Model) applyFilter() {
	query := m.searchInput.Value()
	if query == "" {
		m.filteredTorrents = append([]models.Torrent(nil), m.torrents...)
		m.filterMatches = nil
	} else {
		results := filterTorrentsWithMatches(m.torrents, query)
		m.filteredTorrents = make([]models.Torrent, len(results))
		m.filterMatches = make(map[string][]int, len(results))
		for i, r := range results {
			m.filteredTorrents[i] = r.torrent
			m.filterMatches[r.torrent.ID] = r.indices
		}
	}
	sortTorrents(m.filteredTorrents, m.sortColumn, m.sortAscending)
	if m.selectedIdx >= len(m.filteredTorrents) {
		m.selectedIdx = max(0, len(m.filteredTorrents)-1)
	}
}

func highlightChars(s string, indices []int, style lipgloss.Style) string {
	if len(indices) == 0 {
		return s
	}
	runes := []rune(s)
	set := make(map[int]bool, len(indices))
	for _, i := range indices {
		if i >= 0 && i < len(runes) {
			set[i] = true
		}
	}
	var b strings.Builder
	i := 0
	for i < len(runes) {
		if set[i] {
			start := i
			for i < len(runes) && set[i] {
				i++
			}
			b.WriteString(style.Render(string(runes[start:i])))
		} else {
			b.WriteRune(runes[i])
			i++
		}
	}
	return b.String()
}

func fieldMatchIndices(t models.Torrent, matchIndices []int) map[int][]int {
	fields := []struct {
		col   int
		value string
	}{
		{colStatus, statusLabelForSearch(t.Status)},
		{colProgress, formatProgress(t.Progress)},
		{colSize, humanBytes(t.Bytes)},
		{colName, t.Filename},
		{colAdded, formatAddedTime(t.Added)},
	}
	offsets := make([]int, len(fields))
	offset := 0
	for i, f := range fields {
		offsets[i] = offset
		offset += len([]rune(f.value)) + 1
	}
	result := make(map[int][]int)
	for _, idx := range matchIndices {
		for i, start := range offsets {
			end := start + len([]rune(fields[i].value))
			if idx >= start && idx < end {
				result[fields[i].col] = append(result[fields[i].col], idx-start)
				break
			}
		}
	}
	return result
}
