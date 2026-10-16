package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
)

const (
	modeSort          mode           = "sort"
	modeQuitConfirm   mode           = "quit-confirm"
	actionPageUp      shortcutAction = "page-up"
	actionPageDown    shortcutAction = "page-down"
	actionFirst       shortcutAction = "first"
	actionLast        shortcutAction = "last"
	actionMainEscape  shortcutAction = "main-escape"
	actionOpenSort    shortcutAction = "open-sort"
	actionSortUp      shortcutAction = "sort-up"
	actionSortDown    shortcutAction = "sort-down"
	actionSortApply   shortcutAction = "sort-apply"
	actionSortCancel  shortcutAction = "sort-cancel"
	actionQuitConfirm shortcutAction = "quit-confirm"
	actionQuitCancel  shortcutAction = "quit-cancel"
)

var sortChoices = []struct {
	column int
	label  string
}{{colStatus, "Status"}, {colProgress, "Progress"}, {colSize, "Size"}, {colAdded, "Date"}, {colName, "Name"}}

func (m Model) moveTorrentCursor(index int) (tea.Model, tea.Cmd) {
	visible := m.visibleTorrents()
	if len(visible) == 0 {
		return m, nil
	}
	index = max(0, min(index, len(visible)-1))
	if index == m.selectedIdx {
		return m, nil
	}
	m.selectedIdx = index
	return m, m.requestDetail(m.selectedTorrentID())
}

func (m *Model) clearTorrentFilter() {
	id := m.selectedTorrentID()
	m.searchInput.Blur()
	m.searchInput.Reset()
	m.filterApplied = false
	m.filteredTorrents = nil
	m.filterMatches = nil
	m.selectedIdx = selectedIndexForID(m.torrents, id)
}

func (m Model) navigationAction(action shortcutAction) (tea.Model, tea.Cmd) {
	switch action {
	case actionPageUp:
		return m.moveTorrentCursor(m.selectedIdx - mainDataRows(m.width, m.height))
	case actionPageDown:
		return m.moveTorrentCursor(m.selectedIdx + mainDataRows(m.width, m.height))
	case actionFirst:
		return m.moveTorrentCursor(0)
	case actionLast:
		return m.moveTorrentCursor(len(m.visibleTorrents()) - 1)
	case actionMainEscape:
		if m.batchMode {
			m.batchMode = false
			m.clearBatchSelection()
		} else if m.filterApplied {
			m.clearTorrentFilter()
		}
	case actionOpenSort:
		m.sortCursor = 0
		for i, choice := range sortChoices {
			if choice.column == m.sortColumn {
				m.sortCursor = i
			}
		}
		m.mode = modeSort
	case actionSortUp:
		m.sortCursor = max(0, m.sortCursor-1)
	case actionSortDown:
		m.sortCursor = min(len(sortChoices)-1, m.sortCursor+1)
	case actionSortApply:
		m.sortByColumn(sortChoices[m.sortCursor].column)
		m.mode = modeMain
	case actionSortCancel:
		m.mode = modeMain
	case actionQuitCancel:
		m.mode = m.quitReturnMode
		m.quitReturnMode = ""
	case actionQuitConfirm:
		m.runtime.Stop()
		return m, tea.Quit
	}
	return m, nil
}

func renderNavigationPopup(m Model) string {
	width := max(1, min(50, m.width-4))
	if m.width <= 0 {
		width = 50
	}
	if m.mode == modeQuitConfirm {
		return popupBox("Quit rdtui?", "Exiting cancels active work.\n\n"+popupShortcutFooter(m), width, true)
	}
	lines := []string{}
	for i, choice := range sortChoices {
		label := "  " + choice.label
		if choice.column == m.sortColumn {
			if m.sortAscending {
				label += " ↑"
			} else {
				label += " ↓"
			}
		}
		if i == m.sortCursor {
			label = selectedStyle.Render("▸" + label[1:])
		}
		lines = append(lines, label)
	}
	lines = append(lines, "", popupShortcutFooter(m))
	return popupBox("Sort torrents", strings.Join(lines, "\n"), width, false)
}
