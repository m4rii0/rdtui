package tui

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/m4rii0/rdtui/internal/app"
	"github.com/m4rii0/rdtui/pkg/models"
)

type TorrentService interface {
	ListTorrents(context.Context) ([]models.Torrent, error)
	TorrentInfo(context.Context, string) (models.TorrentInfo, error)
	AddMagnet(context.Context, string) (models.AddTorrentResult, error)
	AddTorrentURL(context.Context, string) (models.AddTorrentResult, error)
	SelectFiles(context.Context, string, []int) error
	DeleteTorrent(context.Context, string) error
}

type BatchService interface {
	Delete(context.Context, []string) app.BatchResult
	Select(context.Context, []app.FileSelection) app.BatchResult
	Import(context.Context, []string) app.BatchResult
	CopyLinks(context.Context, []app.LinkRequest) app.LinkBatchResult
}

type torrentsModel struct {
	torrentService   TorrentService
	batches          BatchService
	list             asyncRequest
	detailRequest    asyncRequest
	torrents         []models.Torrent
	filteredTorrents []models.Torrent
	selectedIdx      int
	detail           *models.TorrentInfo
	pendingFocusID   string
	sortColumn       int
	sortAscending    bool
	filterApplied    bool
	filterMatches    map[string][]int
	batchMode        bool
	batchSelected    map[string]bool
	searchInput      textinput.Model
	browser          fileBrowserState
	selector         selectFilesState
	bulkSelect       *bulkFileSelectionState
	deleteIDs        []string
	pendingAction    *pendingShortcutAction
}

func (t *torrentsModel) update(m *Model, msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tickMsg:
		if !t.list.current(msg.request) || t.list.InFlight {
			return *m, nil
		}
		if m.mode == modeMain || m.mode == modeDetail || m.mode == modeSearch {
			cmd := m.refresh()
			return *m, cmd
		}
		return *m, m.nextListPoll()
	case torrentsMsg:
		m.loading = false
		if msg.err != nil {
			m.errText = msg.err.Error()
			return *m, m.nextListPoll()
		}
		selectedID := m.selectedTorrentID()
		t.torrents = slices.Clone(msg.torrents)
		sortTorrents(t.torrents, t.sortColumn, t.sortAscending)
		if t.filterApplied || m.mode == modeSearch {
			m.applyFilter()
		}
		vis := m.visibleTorrents()
		if t.pendingFocusID != "" {
			if idx, ok := torrentIndexForID(vis, t.pendingFocusID); ok {
				t.selectedIdx = idx
				t.pendingFocusID = ""
			} else {
				t.selectedIdx = selectedIndexForID(vis, selectedID)
			}
		} else {
			t.selectedIdx = selectedIndexForID(vis, selectedID)
		}
		m.errText = ""
		if len(t.torrents) == 0 {
			t.detailRequest.invalidate()
			t.detail = nil
			m.status = "No torrents found"
			return *m, m.nextListPoll()
		}
		cmd := m.requestDetail(m.selectedTorrentID())
		return *m, tea.Batch(cmd, m.nextListPoll())
	case detailMsg:
		if !t.detailRequest.current(msg.request) || msg.id != m.selectedTorrentID() {
			return *m, nil
		}
		t.detailRequest.invalidate()
		if msg.err != nil {
			m.clearPendingDetailAction(msg.id)
			m.errText = msg.err.Error()
			return *m, nil
		}
		t.detail = new(msg.info)
		m.errText = ""
		if action, ok := m.consumePendingDetailAction(msg.id); ok {
			return m.handleShortcutAction(action)
		}
	case browserReadDirMsg:
		t.browser.handleReadDir(msg)
	case addTorrentMsg:
		m.loading = false
		if msg.err != nil {
			m.errText = msg.err.Error()
			return *m, nil
		}
		m.mode = modeMain
		m.status = fmt.Sprintf("Added %s (%s)", msg.label, msg.result.ID)
		t.pendingFocusID = msg.result.ID
		m.errText = ""
		cmd := m.refresh()
		return *m, cmd
	case importMsg:
		m.loading = false
		m.mode = modeMain
		success, failed, canceled := 0, 0, 0
		var failedNames []string
		focusID := ""
		for _, result := range msg.result.Items {
			if result.Kind == app.OutcomeSuccess {
				success++
				if focusID == "" {
					focusID = result.TorrentCreatedID
				}
			} else if result.Kind == app.OutcomeCanceled {
				canceled++
			} else {
				failed++
				failedNames = append(failedNames, filepathBase(result.Source))
			}
		}
		if focusID != "" {
			t.pendingFocusID = focusID
		}
		if failed == 0 {
			m.status = fmt.Sprintf("Imported %d torrent file(s)", success)
			m.errText = ""
		} else {
			m.status = fmt.Sprintf("Imported %d torrent file(s), %d failed", success, failed)
			m.errText = "Failed: " + strings.Join(failedNames, ", ")
		}
		if canceled > 0 {
			m.status += fmt.Sprintf(", %d canceled", canceled)
			m.errText = strings.TrimSpace(m.errText + "\n" + fmt.Sprint(msg.result.Err))
			for _, item := range msg.result.Items {
				if item.RemoteOutcomeUnknown {
					m.errText += "\nRemote outcome unknown; not replayed"
					break
				}
			}
		}
		cmd := m.refresh()
		return *m, cmd
	case selectFilesMsg:
		m.loading = false
		if msg.err != nil {
			m.errText = msg.err.Error()
			return *m, nil
		}
		m.mode = m.returnMode
		m.errText = ""
		cmd := m.refresh()
		return *m, tea.Batch(cmd, m.setFlash(flashSuccess, "Updated torrent file selection"))
	case bulkFileSelectionDetailsMsg:
		m.loading = false
		if msg.err != nil {
			m.errText = msg.err.Error()
			return *m, nil
		}
		if t.bulkSelect == nil {
			return *m, nil
		}
		t.bulkSelect.applyDetails(msg.details, msg.outcomes)
		if t.bulkSelect.prepareFirstPrompt() {
			m.mode = modeBulkSelectFiles
			m.errText = ""
			return *m, nil
		}
		return m.startBulkFileSelectionSubmit()
	case bulkFileSelectionResultMsg:
		m.loading = false
		m.mode = modeMain
		t.bulkSelect = nil
		m.clearBatchSelection()
		t.batchMode = false
		flashMsg := "Bulk file selection complete: " + bulkFileSelectionSummaryLine(msg.success, msg.failed, msg.skipped)
		if msg.canceled > 0 {
			flashMsg += fmt.Sprintf(", %d canceled", msg.canceled)
		}
		m.errText = strings.TrimSpace(msg.detail)
		level := flashSuccess
		if msg.failed > 0 || msg.canceled > 0 {
			level = flashWarn
		}
		cmd := m.refresh()
		return *m, tea.Batch(cmd, m.setFlash(level, flashMsg))
	case deleteMsg:
		m.loading = false
		if msg.err != nil {
			m.errText = msg.err.Error()
			return *m, nil
		}
		if m.returnMode == modeDownload {
			m.downloadTorrentID = ""
		}
		m.mode = modeMain
		m.errText = ""
		t.deleteIDs = nil
		cmd := m.refresh()
		return *m, tea.Batch(cmd, m.setFlash(flashSuccess, "Deleted torrent"))
	case batchResultMsg:
		return t.updateBatch(m, msg)
	}
	return *m, nil
}

func (t *torrentsModel) updateBatch(m *Model, msg batchResultMsg) (tea.Model, tea.Cmd) {
	m.loading = false
	m.errText = strings.TrimSpace(msg.detail)
	switch msg.op {
	case batchOpDelete, batchOpBulkCleanup:
		unit := "torrent(s)"
		if msg.op == batchOpBulkCleanup {
			m.mode = modeBulkDownload
			unit = "source torrent(s)"
		} else {
			m.mode = modeMain
			t.deleteIDs = nil
			m.clearBatchSelection()
			t.batchMode = false
		}
		flashMsg := fmt.Sprintf("Deleted %d %s", msg.success, unit)
		if msg.failed > 0 || msg.canceled > 0 {
			flashMsg = fmt.Sprintf("Deleted %d/%d %s", msg.success, msg.total, unit)
		}
		if msg.canceled > 0 {
			flashMsg += fmt.Sprintf(", %d canceled", msg.canceled)
		}
		cmd := m.refresh()
		return *m, tea.Batch(cmd, m.setFlash(flashSuccess, flashMsg))
	case batchOpCopy:
		m.mode = modeMain
		var parts []string
		if msg.copied == msg.resolved && msg.copied > 0 {
			parts = append(parts, fmt.Sprintf("Copied %d URL(s)", msg.copied))
		} else {
			parts = append(parts, fmt.Sprintf("Resolved %d URL(s), copied %d", msg.resolved, msg.copied))
		}
		if msg.success > 0 {
			parts = append(parts, fmt.Sprintf("%d successful torrent(s)", msg.success))
		}
		if msg.partial > 0 {
			parts = append(parts, fmt.Sprintf("%d partial torrent(s)", msg.partial))
		}
		if msg.skipped > 0 {
			parts = append(parts, fmt.Sprintf("%d skipped torrent(s)", msg.skipped))
		}
		if msg.failed > 0 {
			parts = append(parts, fmt.Sprintf("%d failed torrent(s)", msg.failed))
		}
		if msg.canceled > 0 {
			parts = append(parts, fmt.Sprintf("%d canceled torrent(s)", msg.canceled))
		}
		m.clearBatchSelection()
		t.batchMode = false
		level := flashSuccess
		if msg.partial > 0 || msg.failed > 0 || msg.canceled > 0 || msg.copied < msg.resolved {
			level = flashWarn
		}
		return *m, m.setFlash(level, strings.Join(parts, ", "))
	}
	return *m, nil
}

func refreshCmd(ctx context.Context, runtime *app.Runtime, service TorrentService) tea.Cmd {
	return operationCmd(ctx, runtime, func(ctx context.Context) tea.Msg {
		torrents, err := service.ListTorrents(ctx)
		return torrentsMsg{torrents: torrents, err: err}
	})
}

func addMagnetCmd(ctx context.Context, runtime *app.Runtime, service TorrentService, magnet string) tea.Cmd {
	return operationCmd(ctx, runtime, func(ctx context.Context) tea.Msg {
		result, err := service.AddMagnet(ctx, magnet)
		return addTorrentMsg{result: result, err: err, label: "magnet"}
	})
}

func addURLCmd(ctx context.Context, runtime *app.Runtime, service TorrentService, remoteURL string) tea.Cmd {
	return operationCmd(ctx, runtime, func(ctx context.Context) tea.Msg {
		result, err := service.AddTorrentURL(ctx, remoteURL)
		return addTorrentMsg{result: result, err: err, label: "remote torrent URL"}
	})
}

func importCmd(ctx context.Context, runtime *app.Runtime, service BatchService, paths []string) tea.Cmd {
	paths = slices.Clone(paths)
	return operationCmd(ctx, runtime, func(ctx context.Context) tea.Msg {
		return importMsg{result: service.Import(ctx, paths)}
	})
}

func selectFilesCmd(ctx context.Context, runtime *app.Runtime, service TorrentService, torrentID string, ids []int) tea.Cmd {
	ids = slices.Clone(ids)
	return operationCmd(ctx, runtime, func(ctx context.Context) tea.Msg {
		return selectFilesMsg{err: service.SelectFiles(ctx, torrentID, ids)}
	})
}

func deleteCmd(ctx context.Context, runtime *app.Runtime, service TorrentService, torrentID string) tea.Cmd {
	return operationCmd(ctx, runtime, func(ctx context.Context) tea.Msg {
		return deleteMsg{err: service.DeleteTorrent(ctx, torrentID)}
	})
}

func batchOpCmd(ctx context.Context, runtime *app.Runtime, service BatchService, op batchOp, ids []string, torrents []models.Torrent) tea.Cmd {
	ids = slices.Clone(ids)
	torrents = slices.Clone(torrents)
	for idx := range torrents {
		torrents[idx].Links = slices.Clone(torrents[idx].Links)
	}
	return operationCmd(ctx, runtime, func(ctx context.Context) tea.Msg {
		switch op {
		case batchOpDelete:
			return executeBatchDelete(ctx, service, ids)
		case batchOpBulkCleanup:
			result := executeBatchDelete(ctx, service, ids)
			result.op = batchOpBulkCleanup
			return result
		case batchOpCopy:
			return executeBatchCopy(ctx, service, ids, torrents)
		}
		return batchResultMsg{op: op, err: fmt.Errorf("unknown operation: %s", op)}
	})
}

func executeBatchDelete(ctx context.Context, service BatchService, ids []string) batchResultMsg {
	return batchMessage(batchOpDelete, service.Delete(ctx, ids))
}

func executeBatchCopy(ctx context.Context, service BatchService, ids []string, torrents []models.Torrent) batchResultMsg {
	byID := make(map[string]models.Torrent, len(torrents))
	for _, torrent := range torrents {
		byID[torrent.ID] = torrent
	}
	var requests []app.LinkRequest
	for _, id := range ids {
		torrent := byID[id]
		if torrent.Status != "downloaded" || len(torrent.Links) == 0 {
			requests = append(requests, app.LinkRequest{TorrentID: id})
			continue
		}
		for index, link := range torrent.Links {
			requests = append(requests, app.LinkRequest{TorrentID: id, Target: models.DownloadTarget{Index: index, Link: link}, Eligible: true})
		}
	}
	batch := service.CopyLinks(ctx, requests)
	result := batchMessage(batchOpCopy, batch.BatchResult)
	summary := app.SummarizeTorrents(batch.Items)
	result.total = summary.Success + summary.Partial + summary.Failed + summary.Skipped + summary.Canceled
	result.success, result.partial, result.failed, result.skipped, result.canceled = summary.Success, summary.Partial, summary.Failed, summary.Skipped, summary.Canceled
	result.resolved, result.copied = len(batch.ResolvedURLs), batch.CopiedURLs
	if batch.ClipboardErr != nil {
		result.detail += batch.ClipboardErr.Error() + "\n"
	}
	return result
}

func batchMessage(op batchOp, batch app.BatchResult) batchResultMsg {
	result := batchResultMsg{op: op, total: len(batch.Items), items: batch.Items, err: batch.Err}
	for _, item := range batch.Items {
		switch item.Kind {
		case app.OutcomeSuccess:
			result.success++
		case app.OutcomeFailed:
			result.failed++
		case app.OutcomeSkipped:
			result.skipped++
		case app.OutcomeCanceled:
			result.canceled++
		}
		if item.Err != nil {
			result.detail += fmt.Sprintf("%s: %v\n", item.TorrentID, item.Err)
		}
		if item.RemoteOutcomeUnknown {
			result.detail += "Remote outcome unknown; not replayed\n"
		}
	}
	return result
}
