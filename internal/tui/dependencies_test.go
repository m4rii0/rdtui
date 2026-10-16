package tui

import (
	"testing"

	"github.com/m4rii0/rdtui/pkg/models"
)

func TestRootRoutesFeatureMessages(t *testing.T) {
	m := NewModel(Dependencies{Runtime: newTestRuntime(t)})
	updated, _ := m.Update(authMsg{session: &models.AuthSession{User: models.User{Username: "user"}}})
	m = updated.(Model)
	if m.session == nil || m.mode != modeMain {
		t.Fatal("auth result not routed")
	}
	updated, _ = m.Update(torrentsMsg{torrents: []models.Torrent{{ID: "a", Filename: "torrent"}}})
	m = updated.(Model)
	if m.selectedTorrentID() != "a" {
		t.Fatal("list result not routed")
	}
	updated, _ = m.Update(detailMsg{request: requestID{m.detailRequest.Generation}, id: "a", info: models.TorrentInfo{ID: "a"}})
	m = updated.(Model)
	if m.detail == nil || m.detail.ID != "a" {
		t.Fatal("detail result not routed")
	}
	updated, _ = m.Update(managedDownloadMsg{result: models.ManagedDownloadStart{Download: models.ManagedDownload{Filename: "done", Status: models.ManagedDownloadStatusComplete}}})
	m = updated.(Model)
	if m.mode != modeDownload || m.download.Filename != "done" {
		t.Fatal("download result not routed")
	}
}

func TestFeatureNavigationPreservesSelection(t *testing.T) {
	m := NewModel(Dependencies{Runtime: newTestRuntime(t)})
	m.mode = modeMain
	m.torrents = []models.Torrent{{ID: "a"}, {ID: "b"}}
	m.selectedIdx = 1
	m.batchSelected["b"] = true
	m.detail = &models.TorrentInfo{ID: "b"}
	selectedBeforeNavigation := m.selectedTorrentID()
	updated, _ := m.handleShortcutAction(actionOpenDetail)
	m = updated.(Model)
	updated, _ = m.Update(managedDownloadMsg{result: models.ManagedDownloadStart{Download: models.ManagedDownload{Status: models.ManagedDownloadStatusComplete}}})
	m = updated.(Model)
	m.returnMode = modeDetail
	updated, _ = m.handleShortcutAction(actionBack)
	m = updated.(Model)
	updated, _ = m.handleShortcutAction(actionBack)
	m = updated.(Model)
	selectedAfterReturn := m.selectedTorrentID()
	if selectedAfterReturn != selectedBeforeNavigation {
		t.Fatal("selection changed")
	}
	if m.mode != modeMain || !m.batchSelected["b"] || m.detail.ID != "b" {
		t.Fatal("navigation lost torrent state")
	}
}
