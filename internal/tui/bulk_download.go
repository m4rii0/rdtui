package tui

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/m4rii0/rdtui/internal/app"
	"github.com/m4rii0/rdtui/pkg/models"
)

// BulkQueueService is the bulk feature's application boundary. Observation
// cannot advance, resolve, start, or cancel a queue item.
type BulkQueueService interface {
	Start(context.Context, app.BulkPlan) (app.QueueID, error)
	Snapshot(app.QueueID) (app.QueueSnapshot, error)
	Decide(context.Context, app.QueueDecision) error
	Cancel(app.QueueID) error
}

type bulkStartedMsg struct {
	generation uint64
	queueID    app.QueueID
	active     *app.DownloadSnapshot
	err        error
}
type bulkSnapshotMsg struct {
	queueID  app.QueueID
	request  requestID
	snapshot app.QueueSnapshot
	err      error
}
type bulkTickMsg struct {
	queueID app.QueueID
	request requestID
}
type bulkDecisionMsg struct {
	decision app.QueueDecision
	err      error
}

func (b *bulkDownloadState) plan() app.BulkPlan {
	plan := app.BulkPlan{Items: make([]app.QueueItem, len(b.Items))}
	for i, item := range b.Items {
		plan.Items[i] = app.QueueItem{TorrentID: item.TorrentID, TorrentName: item.TorrentName, Target: item.Target}
	}
	return plan
}

func (m Model) startBulkQueue() (tea.Model, tea.Cmd) {
	if m.bulk.Starting || m.bulk.QueueID != 0 {
		return m, nil
	}
	if m.bulkQueue == nil {
		m.errText = "Bulk queue unavailable"
		return m, nil
	}
	m.bulk.Starting = true
	m.downloadPoll.invalidate()
	plan, generation := m.bulk.plan(), m.queueGeneration
	return m, operationCmd(m.runtime.Context(), m.runtime, func(ctx context.Context) tea.Msg {
		id, err := m.bulkQueue.Start(ctx, plan)
		msg := bulkStartedMsg{generation: generation, queueID: id, err: err}
		if errors.Is(err, app.ErrActiveDownload) {
			if current, exists := m.downloads.Current(); exists {
				msg.active = new(current)
			}
		}
		return msg
	})
}

func (m Model) handleBulkStarted(msg bulkStartedMsg) (tea.Model, tea.Cmd) {
	if m.bulk == nil || msg.generation != m.queueGeneration || !m.bulk.Starting {
		return m, nil
	}
	m.bulk.Starting = false
	m.loading = false
	if msg.err != nil {
		m.errText = msg.err.Error()
		if msg.active != nil {
			m.downloadRef, m.download = msg.active.Ref, new(msg.active.Download)
			m.downloadTorrentID = "" // Never attribute an unrelated transfer to this plan.
			if m.mode == modeBulkDownload {
				m.returnMode, m.mode = modeBulkConfirm, modeDownload
			}
			cmd := m.pollDownload()
			return m, cmd
		}
		if m.mode == modeBulkDownload {
			m.mode = modeBulkConfirm
		}
		return m, nil
	}
	m.bulk.QueueID = msg.queueID
	m.bulk.Snapshot = app.QueueSnapshot{ID: msg.queueID, CurrentIndex: -1}
	m.errText = ""
	cmd := m.pollBulkQueue()
	return m, cmd
}

func (m *Model) pollBulkQueue() tea.Cmd {
	if m.bulk == nil || m.bulk.QueueID == 0 || m.bulk.isFinished() || m.queuePoll.InFlight {
		return nil
	}
	ctx, request := m.queuePoll.begin(m.runtime.Context())
	id := m.bulk.QueueID
	return operationCmd(ctx, m.runtime, func(context.Context) tea.Msg {
		snapshot, err := m.bulkQueue.Snapshot(id)
		return bulkSnapshotMsg{queueID: id, request: request, snapshot: snapshot, err: err}
	})
}

func (m Model) handleBulkSnapshot(msg bulkSnapshotMsg) (tea.Model, tea.Cmd) {
	if m.bulk == nil || msg.queueID != m.bulk.QueueID || !m.queuePoll.current(msg.request) {
		return m, nil
	}
	previous := m.bulk.Snapshot
	if msg.err == nil {
		s := msg.snapshot
		if s.ID != msg.queueID || s.CurrentIndex < previous.CurrentIndex || previous.Finished && !s.Finished {
			return m, nil
		}
		if s.CurrentIndex == previous.CurrentIndex && s.Download != nil && previous.Download != nil &&
			(s.Download.Ref.Generation < previous.Download.Ref.Generation || s.Download.Ref.Generation == previous.Download.Ref.Generation && s.Download.Ref.GID != previous.Download.Ref.GID) {
			return m, nil
		}
	}
	m.queuePoll.invalidate()
	if msg.err != nil {
		// A transient observation error retains the last snapshot and retries.
		return m, m.nextBulkPoll()
	}
	m.bulk.Snapshot = msg.snapshot
	m.bulk.renderSnapshot()
	m.loading = false
	m.download = nil
	if msg.snapshot.Download != nil {
		m.downloadRef = msg.snapshot.Download.Ref
		m.download = new(msg.snapshot.Download.Download)
	}
	if pending := m.pendingDownload; pending != nil && pending.Queue != nil && pending.Queue.QueueID == msg.queueID &&
		(pending.Queue.ItemIndex != msg.snapshot.CurrentIndex || !msg.snapshot.AwaitingOverwrite) {
		m.mode = pending.ResumeMode
		m.pendingDownload = nil
	}
	if msg.snapshot.Finished {
		if m.pendingDownload != nil && m.pendingDownload.Queue != nil && m.pendingDownload.Queue.QueueID == msg.queueID {
			m.mode = m.pendingDownload.ResumeMode
			m.pendingDownload = nil
		}
		success, failed, skipped := m.bulk.resultCounts()
		m.status = "Bulk download complete: " + bulkSummaryLine(success, failed, skipped)
		if canceled := m.bulk.canceledCount(); canceled > 0 {
			m.status += fmt.Sprintf(", %d canceled", canceled)
		}
		return m, nil
	}
	if item := m.bulk.currentItem(); item != nil {
		m.status = fmt.Sprintf("Bulk downloading %d/%d: %s", m.bulk.Current+1, len(m.bulk.Items), bulkItemLabel(*item))
	}
	if msg.snapshot.AwaitingOverwrite {
		decision := app.QueueDecision{QueueID: msg.queueID, ItemIndex: msg.snapshot.CurrentIndex}
		if m.bulk.DecidedItem == nil || m.bulk.DecidedItem.QueueID != decision.QueueID || m.bulk.DecidedItem.ItemIndex != decision.ItemIndex {
			if m.pendingDownload == nil || m.pendingDownload.Queue == nil || *m.pendingDownload.Queue != decision {
				m.pendingDownload = &pendingDownloadState{Queue: new(decision), ResumeMode: m.mode, Path: msg.snapshot.ExistingPath,
					Filename: filepath.Base(msg.snapshot.ExistingPath), RemoteBytes: msg.snapshot.RemoteBytes, ExistingBytes: msg.snapshot.ExistingBytes}
				m.mode = modeOverwrite
				m.status = "Existing file found"
			}
		}
	}
	return m, m.nextBulkPoll()
}

func (m Model) nextBulkPoll() tea.Cmd {
	return m.after(downloadRefreshInterval, bulkTickMsg{m.bulk.QueueID, requestID{m.queuePoll.Generation}})
}

func (m Model) decideBulkOverwrite(decision app.QueueDecision, overwrite bool) (tea.Model, tea.Cmd) {
	if m.bulk == nil || decision.QueueID != m.bulk.QueueID || decision.ItemIndex != m.bulk.Current || m.bulk.isFinished() {
		return m, nil
	}
	decision.Overwrite = overwrite
	m.bulk.DecidedItem = new(decision)
	m.errText = ""
	return m, operationCmd(m.runtime.Context(), m.runtime, func(ctx context.Context) tea.Msg {
		return bulkDecisionMsg{decision: decision, err: m.bulkQueue.Decide(ctx, decision)}
	})
}

// Items is a rendering projection only; all transitions come from the snapshot.
func (b *bulkDownloadState) renderSnapshot() {
	b.Current = b.Snapshot.CurrentIndex
	b.Items = make([]bulkQueueItem, len(b.Snapshot.Items))
	for i, item := range b.Snapshot.Items {
		row := bulkQueueItem{TorrentID: item.Item.TorrentID, TorrentName: item.Item.TorrentName, Target: item.Item.Target, Status: bulkFilePending}
		if item.Status != app.QueueItemPending {
			row.Status = bulkFileActive
		}
		if item.Outcome != nil {
			row.Status = bulkFileStatus(item.Outcome.Kind)
			if item.Outcome.Err != nil {
				row.Error = item.Outcome.Err.Error()
			}
			if item.Outcome.RemoteOutcomeUnknown {
				row.Error += " (remote outcome unknown; not replayed)"
			}
		}
		if i == b.Current && b.Snapshot.Download != nil {
			row.Filename = b.Snapshot.Download.Download.Filename
		}
		b.Items[i] = row
	}
}

type bulkFileStatus string

const (
	bulkFilePending  bulkFileStatus = "pending"
	bulkFileActive   bulkFileStatus = "active"
	bulkFileSuccess  bulkFileStatus = "success"
	bulkFileFailed   bulkFileStatus = "failed"
	bulkFileSkipped  bulkFileStatus = "skipped"
	bulkFileCanceled bulkFileStatus = "canceled"
)

type bulkTorrentPlan struct {
	ID       string
	Name     string
	Targets  []models.DownloadTarget
	Selected map[int]bool
}

type bulkQueueItem struct {
	TorrentID   string
	TorrentName string
	Target      models.DownloadTarget
	Status      bulkFileStatus
	Error       string
	Filename    string
}

type bulkDownloadState struct {
	QueueID            app.QueueID
	Snapshot           app.QueueSnapshot
	Starting           bool
	CleanupInitialized bool
	DecidedItem        *app.QueueDecision
	Plans              []bulkTorrentPlan
	Cursor             int
	FilePrompt         int
	FileCursor         int
	Items              []bulkQueueItem
	Current            int
	CleanupCursor      int
	CleanupSelected    map[string]bool
}

func newBulkDownloadState(torrents []models.Torrent) *bulkDownloadState {
	plans := make([]bulkTorrentPlan, 0, len(torrents))
	for _, torrent := range torrents {
		plans = append(plans, bulkTorrentPlan{ID: torrent.ID, Name: torrent.Filename, Selected: map[int]bool{}})
	}
	return &bulkDownloadState{Plans: plans, FilePrompt: -1, Current: -1, CleanupSelected: map[string]bool{}}
}

func (b *bulkDownloadState) moveOrderCursor(delta int) {
	if b == nil || len(b.Plans) == 0 {
		return
	}
	b.Cursor = clampIndex(b.Cursor+delta, len(b.Plans))
}

func (b *bulkDownloadState) moveOrderItem(delta int) {
	if b == nil || len(b.Plans) == 0 {
		return
	}
	next := b.Cursor + delta
	if next < 0 || next >= len(b.Plans) {
		return
	}
	b.Plans[b.Cursor], b.Plans[next] = b.Plans[next], b.Plans[b.Cursor]
	b.Cursor = next
}

func (b *bulkDownloadState) moveFileCursor(delta int) {
	plan := b.currentFilePlan()
	if plan == nil || len(plan.Targets) == 0 {
		return
	}
	b.FileCursor = clampIndex(b.FileCursor+delta, len(plan.Targets))
}

func (b *bulkDownloadState) moveCleanupCursor(delta int) {
	if b == nil || len(b.Plans) == 0 {
		return
	}
	b.CleanupCursor = clampIndex(b.CleanupCursor+delta, len(b.Plans))
}

func (b *bulkDownloadState) currentFilePlan() *bulkTorrentPlan {
	if b == nil || b.FilePrompt < 0 || b.FilePrompt >= len(b.Plans) {
		return nil
	}
	return &b.Plans[b.FilePrompt]
}

func (b *bulkDownloadState) toggleCurrentFile() {
	plan := b.currentFilePlan()
	if plan == nil || b.FileCursor < 0 || b.FileCursor >= len(plan.Targets) {
		return
	}
	if plan.Selected[b.FileCursor] {
		delete(plan.Selected, b.FileCursor)
		return
	}
	plan.Selected[b.FileCursor] = true
}

func (b *bulkDownloadState) selectAllCurrentFiles() {
	plan := b.currentFilePlan()
	if plan == nil {
		return
	}
	for idx := range plan.Targets {
		plan.Selected[idx] = true
	}
}

func (b *bulkDownloadState) clearCurrentFiles() {
	plan := b.currentFilePlan()
	if plan == nil {
		return
	}
	plan.Selected = map[int]bool{}
}

func (b *bulkDownloadState) selectedCurrentFileCount() int {
	plan := b.currentFilePlan()
	if plan == nil {
		return 0
	}
	return len(plan.Selected)
}

func (b *bulkDownloadState) applyDetails(details []models.TorrentInfo) {
	if b == nil {
		return
	}
	detailsByID := make(map[string]models.TorrentInfo, len(details))
	for _, detail := range details {
		detailsByID[detail.ID] = detail
	}
	for idx := range b.Plans {
		detail, ok := detailsByID[b.Plans[idx].ID]
		if !ok {
			continue
		}
		if detail.Filename != "" {
			b.Plans[idx].Name = detail.Filename
		}
		b.Plans[idx].Targets = appDownloadTargets(detail)
		b.Plans[idx].Selected = map[int]bool{}
		for targetIdx := range b.Plans[idx].Targets {
			b.Plans[idx].Selected[targetIdx] = true
		}
	}
}

func (b *bulkDownloadState) prepareFirstFilePrompt() bool {
	return b.prepareNextFilePrompt(0)
}

func (b *bulkDownloadState) prepareNextFilePrompt(start int) bool {
	if b == nil {
		return false
	}
	for idx := start; idx < len(b.Plans); idx++ {
		if len(b.Plans[idx].Targets) > 1 {
			b.FilePrompt = idx
			b.FileCursor = 0
			return true
		}
	}
	b.FilePrompt = -1
	b.FileCursor = 0
	return false
}

func (b *bulkDownloadState) buildQueueItems() {
	if b == nil {
		return
	}
	b.Items = nil
	for _, plan := range b.Plans {
		for targetIdx, target := range plan.Targets {
			if !plan.Selected[targetIdx] {
				continue
			}
			b.Items = append(b.Items, bulkQueueItem{
				TorrentID:   plan.ID,
				TorrentName: plan.Name,
				Target:      target,
				Status:      bulkFilePending,
			})
		}
	}
	b.Current = -1
}

func (b *bulkDownloadState) nextPendingIndex() int {
	if b == nil {
		return -1
	}
	for idx, item := range b.Items {
		if item.Status == bulkFilePending {
			return idx
		}
	}
	return -1
}

func (b *bulkDownloadState) currentItem() *bulkQueueItem {
	if b == nil || b.Current < 0 || b.Current >= len(b.Items) {
		return nil
	}
	return &b.Items[b.Current]
}

func (b *bulkDownloadState) isFinished() bool {
	return b != nil && b.Snapshot.Finished
}

func (b *bulkDownloadState) resultCounts() (success int, failed int, skipped int) {
	if b == nil {
		return 0, 0, 0
	}
	for _, item := range b.Items {
		switch item.Status {
		case bulkFileSuccess:
			success++
		case bulkFileFailed:
			failed++
		case bulkFileSkipped:
			skipped++
		}
	}
	return success, failed, skipped
}

func (b *bulkDownloadState) completedCount() int {
	success, failed, skipped := b.resultCounts()
	return success + failed + skipped + b.canceledCount()
}

func (b *bulkDownloadState) canceledCount() int {
	count := 0
	for _, item := range b.Items {
		if item.Status == bulkFileCanceled {
			count++
		}
	}
	return count
}

func (b *bulkDownloadState) filesForTorrent(id string) []bulkQueueItem {
	if b == nil {
		return nil
	}
	items := make([]bulkQueueItem, 0)
	for _, item := range b.Items {
		if item.TorrentID == id {
			items = append(items, item)
		}
	}
	return items
}

func (b *bulkDownloadState) torrentOutcome(id string) string {
	items := b.filesForTorrent(id)
	if len(items) == 0 {
		return "skipped"
	}
	success := 0
	failed := 0
	skipped := 0
	pending := 0
	canceled := 0
	for _, item := range items {
		switch item.Status {
		case bulkFileSuccess:
			success++
		case bulkFileFailed:
			failed++
		case bulkFileSkipped:
			skipped++
		case bulkFileCanceled:
			canceled++
		default:
			pending++
		}
	}
	if pending > 0 {
		return "pending"
	}
	if success == len(items) {
		return "complete"
	}
	if success > 0 {
		return fmt.Sprintf("partial: %d/%d files failed", failed+skipped+canceled, len(items))
	}
	if failed > 0 {
		return "failed"
	}
	if canceled > 0 {
		return "canceled"
	}
	return "skipped"
}

func (b *bulkDownloadState) torrentCleanupSafe(id string) bool {
	return b.torrentOutcome(id) == "complete"
}

func (b *bulkDownloadState) initCleanupSelection() {
	if b == nil || b.CleanupInitialized {
		return
	}
	b.CleanupInitialized = true
	b.CleanupSelected = map[string]bool{}
	for _, plan := range b.Plans {
		if b.torrentCleanupSafe(plan.ID) {
			b.CleanupSelected[plan.ID] = true
		}
	}
	b.CleanupCursor = 0
}

func (b *bulkDownloadState) toggleCleanupSelection() {
	if b == nil || b.CleanupCursor < 0 || b.CleanupCursor >= len(b.Plans) {
		return
	}
	id := b.Plans[b.CleanupCursor].ID
	if b.CleanupSelected[id] {
		delete(b.CleanupSelected, id)
		return
	}
	b.CleanupSelected[id] = true
}

func (b *bulkDownloadState) selectAllCleanup() {
	if b == nil {
		return
	}
	for _, plan := range b.Plans {
		b.CleanupSelected[plan.ID] = true
	}
}

func (b *bulkDownloadState) clearCleanup() {
	if b == nil {
		return
	}
	b.CleanupSelected = map[string]bool{}
}

func (b *bulkDownloadState) cleanupSelectedIDs() []string {
	if b == nil {
		return nil
	}
	ids := make([]string, 0, len(b.CleanupSelected))
	for _, plan := range b.Plans {
		if b.CleanupSelected[plan.ID] {
			ids = append(ids, plan.ID)
		}
	}
	return ids
}

func (b *bulkDownloadState) hasRiskyCleanupSelection() bool {
	if b == nil {
		return false
	}
	for _, plan := range b.Plans {
		if b.CleanupSelected[plan.ID] && !b.torrentCleanupSafe(plan.ID) {
			return true
		}
	}
	return false
}

func (b *bulkDownloadState) selectedTorrentCount() int {
	if b == nil {
		return 0
	}
	seen := map[string]bool{}
	for _, item := range b.Items {
		seen[item.TorrentID] = true
	}
	return len(seen)
}

func (b *bulkDownloadState) planFileCount(plan bulkTorrentPlan) int {
	return len(plan.Selected)
}

func bulkItemLabel(item bulkQueueItem) string {
	label := item.Target.Label
	if label == "" {
		label = item.Filename
	}
	if label == "" {
		label = item.Target.FilePath
	}
	if label == "" {
		label = item.TorrentName
	}
	return label
}

func bulkStatusStyle(status bulkFileStatus) lipgloss.Style {
	switch status {
	case bulkFileSuccess:
		return okStyle
	case bulkFileFailed:
		return errorStyle
	case bulkFileSkipped, bulkFileCanceled:
		return warnStyle
	case bulkFileActive:
		return infoStyle
	default:
		return mutedStyle
	}
}

func clampIndex(value int, length int) int {
	if length <= 0 {
		return 0
	}
	if value < 0 {
		return 0
	}
	if value >= length {
		return length - 1
	}
	return value
}

func appDownloadTargets(info models.TorrentInfo) []models.DownloadTarget {
	return app.DownloadTargets(info)
}

func bulkSummaryLine(success, failed, skipped int) string {
	parts := []string{}
	if success > 0 {
		parts = append(parts, fmt.Sprintf("%d complete", success))
	}
	if failed > 0 {
		parts = append(parts, fmt.Sprintf("%d failed", failed))
	}
	if skipped > 0 {
		parts = append(parts, fmt.Sprintf("%d skipped", skipped))
	}
	if len(parts) == 0 {
		return "No files processed"
	}
	return strings.Join(parts, ", ")
}

func renderBulkOrderPopup(m Model) string {
	popupW, _ := popupSize(m.width, m.height)
	innerW := popupW - 4
	if m.bulk == nil {
		return ""
	}
	title := headStyle.Render("◆ Bulk Download Order")
	visibleH := max(1, popupW/3-6)
	start, end := fileListWindow(len(m.bulk.Plans), m.bulk.Cursor, visibleH)
	lines := []string{mutedStyle.Render("  Default order follows the current torrent view."), ""}
	for idx := start; idx < end; idx++ {
		plan := m.bulk.Plans[idx]
		cursor := "  "
		if idx == m.bulk.Cursor {
			cursor = warnStyle.Render("▸ ")
		}
		lines = append(lines, fmt.Sprintf("%s%s", cursor, truncateLine(plan.Name, innerW-4)))
	}
	if m.errText != "" {
		lines = append(lines, "", errorStyle.Render("  ✗ "+m.errText))
	}
	lines = append(lines, "", popupShortcutFooter(m))
	return popupBox(title, strings.Join(lines, "\n"), innerW, false)
}

func renderBulkFilesPopup(m Model) string {
	popupW, _ := popupSize(m.width, m.height)
	innerW := popupW - 4
	if m.bulk == nil {
		return ""
	}
	plan := m.bulk.currentFilePlan()
	if plan == nil {
		return ""
	}
	scrollInd := headStyle.Render(fmt.Sprintf(" %d/%d ", m.bulk.FilePrompt+1, len(m.bulk.Plans)))
	title := headStyle.Render("◆ Select Download Files") + "  " + mutedStyle.Render(scrollInd)
	lines := []string{mutedStyle.Render("  " + truncateLine(plan.Name, innerW-4)), ""}
	visibleH := max(1, popupW/3-8)
	start, end := fileListWindow(len(plan.Targets), m.bulk.FileCursor, visibleH)
	for idx := start; idx < end; idx++ {
		target := plan.Targets[idx]
		cursor := "  "
		if idx == m.bulk.FileCursor {
			cursor = warnStyle.Render("▸ ")
		}
		marker := subtleStyle.Render("[ ]")
		if plan.Selected[idx] {
			marker = okStyle.Render("[✓]")
		}
		label := target.Label
		if target.FilePath != "" {
			label = fmt.Sprintf("%s (%s)", label, target.FilePath)
		}
		lines = append(lines, fmt.Sprintf("%s%s %s", cursor, marker, truncateLine(label, innerW-8)))
	}
	if m.errText != "" {
		lines = append(lines, "", errorStyle.Render("  ✗ "+m.errText))
	}
	lines = append(lines, "", popupShortcutFooter(m))
	return popupBox(title, strings.Join(lines, "\n"), innerW, false)
}

func renderBulkConfirmPopup(m Model) string {
	popupW, _ := popupSize(m.width, m.height)
	innerW := popupW - 4
	if m.bulk == nil {
		return ""
	}
	if len(m.bulk.Items) == 0 {
		m.bulk.buildQueueItems()
	}
	title := headStyle.Render("◆ Start Bulk Download?")
	lines := []string{
		warnStyle.Render(fmt.Sprintf("  Download %d file(s) from %d torrent(s)?", len(m.bulk.Items), m.bulk.selectedTorrentCount())),
		"",
	}
	count := 0
	for _, plan := range m.bulk.Plans {
		fileCount := m.bulk.planFileCount(plan)
		if fileCount == 0 {
			continue
		}
		if count >= 5 {
			lines = append(lines, mutedStyle.Render(fmt.Sprintf("  ... and %d more", len(m.bulk.Plans)-5)))
			break
		}
		lines = append(lines, fmt.Sprintf("  %s %s", mutedStyle.Render("▸"), truncateLine(fmt.Sprintf("%s (%d file(s))", plan.Name, fileCount), innerW-6)))
		count++
	}
	if m.errText != "" {
		lines = append(lines, "", errorStyle.Render("  ✗ "+m.errText))
	}
	lines = append(lines, "", popupShortcutFooter(m))
	return popupBox(title, strings.Join(lines, "\n"), innerW, false)
}

func renderBulkCleanupPopup(m Model) string {
	popupW, _ := popupSize(m.width, m.height)
	innerW := popupW - 4
	if m.bulk == nil {
		return ""
	}
	title := headStyle.Render("◆ Delete Source Torrents?")
	lines := []string{
		mutedStyle.Render("  Complete torrents are pre-selected. Failed or partial torrents are not."),
		"",
	}
	visibleH := max(1, popupW/3-9)
	start, end := fileListWindow(len(m.bulk.Plans), m.bulk.CleanupCursor, visibleH)
	for idx := start; idx < end; idx++ {
		plan := m.bulk.Plans[idx]
		cursor := "  "
		if idx == m.bulk.CleanupCursor {
			cursor = warnStyle.Render("▸ ")
		}
		marker := subtleStyle.Render("[ ]")
		if m.bulk.CleanupSelected[plan.ID] {
			marker = okStyle.Render("[✓]")
		}
		outcome := m.bulk.torrentOutcome(plan.ID)
		outcomeStyle := mutedStyle
		if outcome == "complete" {
			outcomeStyle = okStyle
		} else if strings.HasPrefix(outcome, "partial") || outcome == "skipped" {
			outcomeStyle = warnStyle
		} else if outcome == "failed" {
			outcomeStyle = errorStyle
		}
		label := fmt.Sprintf("%s  %s", truncateLine(plan.Name, max(8, innerW-24)), outcomeStyle.Render(outcome))
		lines = append(lines, fmt.Sprintf("%s%s %s", cursor, marker, truncateLine(label, innerW-8)))
	}
	if m.bulk.hasRiskyCleanupSelection() {
		lines = append(lines, "", warnStyle.Render("  Warning: selected torrents include incomplete downloads."))
	}
	if m.errText != "" {
		lines = append(lines, "", errorStyle.Render("  ✗ "+m.errText))
	}
	lines = append(lines, "", popupShortcutFooter(m))
	return popupBox(title, strings.Join(lines, "\n"), innerW, true)
}

func renderBulkDownloadView(m Model) string {
	width := m.width
	if width <= 0 {
		width = 120
	}
	innerWidth := max(20, width-4)
	header := renderHeaderLine(renderCompactHeader(m.version, username(m)), m, innerWidth)
	lines := []string{header, ""}
	if m.bulk == nil {
		lines = append(lines, mutedStyle.Render("bulk download not started"))
	} else if m.bulk.isFinished() {
		lines = append(lines, renderBulkSummaryPanel(m, innerWidth))
	} else {
		lines = append(lines, renderBulkProgressPanel(m, innerWidth))
	}
	if m.status != "" {
		lines = append(lines, mutedStyle.Render("  ▸ "+m.status))
	}
	if m.errText != "" {
		lines = append(lines, errorStyle.Render("  ✗ "+m.errText))
	}
	if flash := renderFlash(m); flash != "" {
		lines = append(lines, flash)
	}
	lines = append(lines, dividerLine(innerWidth), truncateLine(renderShortcutFooter(m.renderShortcutDefs(), m), innerWidth))
	return strings.Join(lines, "\n")
}

func renderBulkProgressPanel(m Model, width int) string {
	stats := []string{sectionTitle("Bulk Download"), ""}
	if m.bulk != nil {
		stats = append(stats, fmt.Sprintf("  %s  %d / %d", mutedStyle.Render("Files:"), m.bulk.completedCount()+1, len(m.bulk.Items)))
		if item := m.bulk.currentItem(); item != nil {
			stats = append(stats,
				fmt.Sprintf("  %s  %s", mutedStyle.Render("Torrent:"), textStyle.Render(item.TorrentName)),
				fmt.Sprintf("  %s  %s", mutedStyle.Render("File:   "), textStyle.Render(bulkItemLabel(*item))),
			)
		}
	}
	if m.download == nil {
		stats = append(stats, "", m.spinner.View()+" "+mutedStyle.Render("preparing download..."))
	} else {
		d := m.download
		pct := d.Progress() / 100.0
		stats = append(stats,
			fmt.Sprintf("  %s  %s", mutedStyle.Render("Status: "), managedDownloadStatusLabel(d.Status)),
			fmt.Sprintf("  %s  %s  %.1f%%", mutedStyle.Render("Progress:"), m.progress.ViewAs(pct), d.Progress()),
			fmt.Sprintf("  %s  %s / %s", mutedStyle.Render("Bytes:   "), infoStyle.Render(humanBytes(d.CompletedLength)), mutedStyle.Render(humanBytes(d.TotalLength))),
			fmt.Sprintf("  %s  %s/s", mutedStyle.Render("Speed:   "), infoStyle.Render(humanBytes(d.DownloadSpeed))),
		)
		if d.ErrorMessage != "" {
			stats = append(stats, "", errorStyle.Render("  ✗ "+d.ErrorMessage))
		}
	}
	if m.bulk != nil && len(m.bulk.Items) > 0 {
		stats = append(stats, "", renderBulkQueue(m.bulk, width-4, bulkQueueVisibleRows(m.height)))
	}
	return boxStyle.Width(width).Render(strings.Join(stats, "\n"))
}

func renderBulkQueue(b *bulkDownloadState, width int, visible int) string {
	if b == nil || len(b.Items) == 0 {
		return ""
	}
	width = max(20, width)
	visible = max(1, visible)
	current := b.Current
	if current < 0 {
		current = b.nextPendingIndex()
	}
	if current < 0 {
		current = len(b.Items) - 1
	}
	start, end := torrentListWindow(len(b.Items), current, visible)
	lines := []string{fmt.Sprintf("  %s  %d / %d", mutedStyle.Render("Queue:"), clampIndex(current, len(b.Items))+1, len(b.Items))}
	if start > 0 {
		lines = append(lines, mutedStyle.Render("    ..."))
	}
	for idx := start; idx < end; idx++ {
		lines = append(lines, renderBulkQueueRow(b.Items[idx], idx, len(b.Items), width))
	}
	if end < len(b.Items) {
		lines = append(lines, mutedStyle.Render("    ..."))
	}
	return strings.Join(lines, "\n")
}

func renderBulkQueueRow(item bulkQueueItem, idx int, total int, width int) string {
	status := bulkQueueStatusLabel(item.Status)
	style := bulkStatusStyle(item.Status)
	label := fmt.Sprintf("%s / %s", item.TorrentName, bulkItemLabel(item))
	if item.Error != "" {
		label += ": " + item.Error
	}
	prefix := fmt.Sprintf("  %s %02d/%02d ", style.Render(bulkQueueStatusGlyph(item.Status)), idx+1, total)
	statusSuffix := " " + style.Render(status)
	available := max(8, width-lipgloss.Width(prefix)-lipgloss.Width(statusSuffix))
	return prefix + truncateLine(label, available) + statusSuffix
}

func bulkQueueStatusGlyph(status bulkFileStatus) string {
	switch status {
	case bulkFileSuccess:
		return "✓"
	case bulkFileFailed:
		return "✗"
	case bulkFileSkipped, bulkFileCanceled:
		return "-"
	case bulkFileActive:
		return "▶"
	default:
		return "◦"
	}
}

func bulkQueueStatusLabel(status bulkFileStatus) string {
	switch status {
	case bulkFileSuccess:
		return "complete"
	case bulkFileFailed:
		return "failed"
	case bulkFileSkipped:
		return "skipped"
	case bulkFileCanceled:
		return "canceled"
	case bulkFileActive:
		return "active"
	default:
		return "pending"
	}
}

func bulkQueueVisibleRows(termH int) int {
	if termH <= 0 {
		return 7
	}
	if termH < 20 {
		return 4
	}
	if termH > 32 {
		return 10
	}
	return 7
}

func renderBulkSummaryPanel(m Model, width int) string {
	success, failed, skipped := m.bulk.resultCounts()
	stats := []string{
		sectionTitle("Bulk Download Summary"),
		"",
		fmt.Sprintf("  %s  %d", mutedStyle.Render("Total files:"), len(m.bulk.Items)),
		fmt.Sprintf("  %s  %s", mutedStyle.Render("Complete:   "), okStyle.Render(fmt.Sprintf("%d", success))),
		fmt.Sprintf("  %s  %s", mutedStyle.Render("Failed:     "), errorStyle.Render(fmt.Sprintf("%d", failed))),
		fmt.Sprintf("  %s  %s", mutedStyle.Render("Skipped:    "), warnStyle.Render(fmt.Sprintf("%d", skipped))),
		"",
	}
	if canceled := m.bulk.canceledCount(); canceled > 0 {
		stats = append(stats, fmt.Sprintf("  %s  %d", mutedStyle.Render("Canceled:   "), canceled), "")
	}
	for _, plan := range m.bulk.Plans {
		outcome := m.bulk.torrentOutcome(plan.ID)
		stats = append(stats, fmt.Sprintf("  %s %s", mutedStyle.Render("▸"), truncateLine(fmt.Sprintf("%s - %s", plan.Name, outcome), width-6)))
	}
	stats = append(stats, "", mutedStyle.Render("  Press x to clean up source torrents."))
	return boxStyle.Width(width).Render(strings.Join(stats, "\n"))
}
