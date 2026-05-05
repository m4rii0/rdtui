package tui

import (
	tea "charm.land/bubbletea/v2"
	"github.com/m4rii0/rdtui/pkg/models"
)

type pendingShortcutAction struct {
	torrentID string
	action    shortcutAction
}

func (m Model) selectedTorrent() *models.Torrent {
	vis := m.visibleTorrents()
	if len(vis) == 0 || m.selectedIdx < 0 || m.selectedIdx >= len(vis) {
		return nil
	}
	return &vis[m.selectedIdx]
}

func (m Model) selectedDetail() *models.TorrentInfo {
	if m.detail == nil {
		return nil
	}
	if m.selectedTorrentID() != "" && m.detail.ID == m.selectedTorrentID() {
		return m.detail
	}
	if m.mode == modeDetail && m.detail != nil {
		return m.detail
	}
	return nil
}

func (m Model) selectedTorrentStatus() string {
	if selected := m.selectedTorrent(); selected != nil && selected.Status != "" {
		return selected.Status
	}
	if detail := m.selectedDetail(); detail != nil {
		return detail.Status
	}
	return ""
}

func (m Model) canSelectFilesFromSelection() bool {
	return isFileSelectionStatus(m.selectedTorrentStatus())
}

func (m Model) canReadyActionsFromSelection() bool {
	return m.selectedTorrentStatus() == "downloaded"
}

func (m Model) canDeleteSelectedTorrent() bool {
	return m.selectedTorrentID() != ""
}

func (m Model) canSelectFilesFromDetail() bool {
	return m.detail != nil && isFileSelectionStatus(m.detail.Status)
}

func isFileSelectionStatus(status string) bool {
	switch status {
	case "waiting_files_selection", "magnet_conversion":
		return true
	default:
		return false
	}
}

func (m Model) canReadyActionsFromDetail() bool {
	return m.detail != nil && m.detail.Status == "downloaded"
}

func (m Model) canOpenManagedDownloadFile() bool {
	return m.download != nil && m.download.IsComplete() && m.download.FilePath != ""
}

func (m Model) canDeleteManagedDownloadTorrent() bool {
	return m.download != nil && m.download.IsComplete() && m.downloadTorrentID != ""
}

func (m Model) canBulkDownloadSelection() bool {
	if !m.batchMode || len(m.batchSelected) < 2 {
		return false
	}
	selected := 0
	for _, torrent := range m.visibleTorrents() {
		if !m.batchSelected[torrent.ID] {
			continue
		}
		selected++
		if torrent.Status != "downloaded" {
			return false
		}
	}
	return selected >= 2
}

func (m Model) canBulkSelectFilesSelection() bool {
	if !m.batchMode || len(m.batchSelected) == 0 {
		return false
	}
	for _, torrent := range m.visibleTorrents() {
		if m.batchSelected[torrent.ID] && isFileSelectionStatus(torrent.Status) {
			return true
		}
	}
	return false
}

func (m Model) canShowHelp() bool {
	switch m.mode {
	case modeMain, modeDetail, modeDownload, modeBulkDownload,
		modeSelectFiles, modeChooseTarget, modeBulkSelectFiles,
		modeBulkOrder, modeBulkFiles, modeBulkConfirm, modeBulkCleanup:
		return true
	case modeFileBrowser:
		return !m.browser.EditingPath
	default:
		return false
	}
}

func (m *Model) queueSelectedDetailAction(action shortcutAction) (tea.Model, tea.Cmd) {
	id := m.selectedTorrentID()
	if id == "" {
		return *m, nil
	}
	m.pendingAction = &pendingShortcutAction{torrentID: id, action: action}
	return *m, detailCmd(m.service, id)
}

func (m *Model) consumePendingDetailAction(id string) (shortcutAction, bool) {
	if m.pendingAction == nil {
		return "", false
	}
	if m.pendingAction.torrentID != id {
		return "", false
	}
	action := m.pendingAction.action
	m.pendingAction = nil
	if id != m.selectedTorrentID() {
		return "", false
	}
	return action, true
}

func (m *Model) clearPendingDetailAction(id string) {
	if m.pendingAction != nil && m.pendingAction.torrentID == id {
		m.pendingAction = nil
	}
}
