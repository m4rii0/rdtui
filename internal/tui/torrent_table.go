package tui

import (
	"cmp"
	"slices"
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
	if showScrollbar {
		totalWidth -= 2
	}
	totalWidth = max(0, totalWidth)
	if totalWidth < 3 {
		return []columnSpec{{Index: colName, Label: "Name", Width: totalWidth}}
	}
	statusW := 10
	if totalWidth < 40 {
		statusW = min(6, max(1, totalWidth/3))
	}
	cols := []columnSpec{{Index: colStatus, Label: "Status", Width: statusW}}
	if totalWidth >= 40 {
		cols = append(cols, columnSpec{Index: colProgress, Label: "%", Width: 6, AlignRight: true})
	}
	if totalWidth >= 60 {
		cols = append(cols, columnSpec{Index: colSize, Label: "Size", Width: 9, AlignRight: true})
	}
	cols = append(cols, columnSpec{Index: colName, Label: "Name"})
	if totalWidth >= 80 {
		cols = append(cols, columnSpec{Index: colAdded, Label: "Added", Width: 16, AlignRight: true})
	}
	remaining := totalWidth - (len(cols) - 1)
	for _, col := range cols {
		remaining -= col.Width
	}
	for i := range cols {
		if cols[i].Index == colName {
			cols[i].Width = max(0, remaining)
		}
	}
	return cols
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
	slices.SortStableFunc(torrents, func(a, b models.Torrent) int {
		order := compareByColumn(a, b, col)
		if order != 0 {
			if ascending {
				return order
			}
			return -order
		}
		if order := b.Added.Compare(a.Added); order != 0 {
			return order
		}
		return cmp.Compare(a.Filename, b.Filename)
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
		return cmp.Compare(a.Bytes, b.Bytes)
	case colAdded:
		return a.Added.Compare(b.Added)
	case colName:
		return cmp.Compare(a.Filename, b.Filename)
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
			if col.Width <= 6 {
				plain = statusLabelForSearch(torrent.Status)
			}
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
			if fm != nil {
				value = highlightChars(value, fm[colName], matchHighlightStyle)
			}
			if selected {
				value = selectedStyle.Render(value)
			}
		}
		cells[i] = padVisual(oneLine(value, col.Width), col.Width, col.AlignRight)
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
		m.filteredTorrents = slices.Clone(m.torrents)
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
