package tui

import (
	"context"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/m4rii0/rdtui/internal/app"
	"github.com/m4rii0/rdtui/pkg/models"
)

type bulkFileSelectionOutcomeStatus string

const (
	bulkFileSelectionSuccess bulkFileSelectionOutcomeStatus = "success"
	bulkFileSelectionFailed  bulkFileSelectionOutcomeStatus = "failed"
	bulkFileSelectionSkipped bulkFileSelectionOutcomeStatus = "skipped"
)

type bulkFileSelectionPlan struct {
	ID       string
	Name     string
	Files    []models.TorrentFile
	Selected map[int]bool
}

type bulkFileSelectionOutcome struct {
	ID     string
	Name   string
	Status bulkFileSelectionOutcomeStatus
	Error  string
}

type bulkFileSelectionState struct {
	Plans      []bulkFileSelectionPlan
	Prompt     int
	FileCursor int
	Outcomes   []bulkFileSelectionOutcome
}

type bulkFileSelectionDetailsMsg struct {
	details  []models.TorrentInfo
	outcomes []bulkFileSelectionOutcome
	err      error
}

type bulkFileSelectionResultMsg struct {
	total   int
	success int
	failed  int
	skipped int
	detail  string
	err     error
}

type bulkFileSelectionSubmission struct {
	ID      string
	Name    string
	FileIDs []int
}

func newBulkFileSelectionState(eligible []models.Torrent, skipped []models.Torrent) *bulkFileSelectionState {
	plans := make([]bulkFileSelectionPlan, 0, len(eligible))
	for _, torrent := range eligible {
		plans = append(plans, bulkFileSelectionPlan{ID: torrent.ID, Name: torrent.Filename, Selected: map[int]bool{}})
	}
	outcomes := make([]bulkFileSelectionOutcome, 0, len(skipped))
	for _, torrent := range skipped {
		outcomes = append(outcomes, bulkFileSelectionOutcome{ID: torrent.ID, Name: torrent.Filename, Status: bulkFileSelectionSkipped, Error: "not waiting for file selection"})
	}
	return &bulkFileSelectionState{Plans: plans, Prompt: -1, Outcomes: outcomes}
}

func (b *bulkFileSelectionState) applyDetails(details []models.TorrentInfo, outcomes []bulkFileSelectionOutcome) {
	if b == nil {
		return
	}
	b.Outcomes = append(b.Outcomes, outcomes...)
	detailsByID := make(map[string]models.TorrentInfo, len(details))
	for _, detail := range details {
		detailsByID[detail.ID] = detail
	}
	plans := make([]bulkFileSelectionPlan, 0, len(b.Plans))
	for _, plan := range b.Plans {
		detail, ok := detailsByID[plan.ID]
		if !ok {
			continue
		}
		if len(detail.Files) == 0 {
			b.Outcomes = append(b.Outcomes, bulkFileSelectionOutcome{ID: plan.ID, Name: plan.Name, Status: bulkFileSelectionFailed, Error: "no files available"})
			continue
		}
		if detail.Filename != "" {
			plan.Name = detail.Filename
		}
		plan.Files = append([]models.TorrentFile(nil), detail.Files...)
		plan.Selected = defaultSelectedFiles(detail)
		plans = append(plans, plan)
	}
	b.Plans = plans
}

func defaultSelectedFiles(info models.TorrentInfo) map[int]bool {
	selected := map[int]bool{}
	for _, id := range app.DefaultFileSelection(info) {
		selected[id] = true
	}
	return selected
}

func (b *bulkFileSelectionState) prepareFirstPrompt() bool {
	return b.prepareNextPrompt(0)
}

func (b *bulkFileSelectionState) prepareNextPrompt(start int) bool {
	if b == nil {
		return false
	}
	for idx := start; idx < len(b.Plans); idx++ {
		b.Prompt = idx
		b.FileCursor = 0
		return true
	}
	b.Prompt = -1
	b.FileCursor = 0
	return false
}

func (b *bulkFileSelectionState) currentPlan() *bulkFileSelectionPlan {
	if b == nil || b.Prompt < 0 || b.Prompt >= len(b.Plans) {
		return nil
	}
	return &b.Plans[b.Prompt]
}

func (b *bulkFileSelectionState) moveFileCursor(delta int) {
	plan := b.currentPlan()
	if plan == nil || len(plan.Files) == 0 {
		return
	}
	b.FileCursor = clampIndex(b.FileCursor+delta, len(plan.Files))
}

func (b *bulkFileSelectionState) toggleCurrentFile() {
	plan := b.currentPlan()
	if plan == nil || b.FileCursor < 0 || b.FileCursor >= len(plan.Files) {
		return
	}
	id := plan.Files[b.FileCursor].ID
	if plan.Selected[id] {
		delete(plan.Selected, id)
		return
	}
	plan.Selected[id] = true
}

func (b *bulkFileSelectionState) selectAllCurrentFiles() {
	plan := b.currentPlan()
	if plan == nil {
		return
	}
	for _, file := range plan.Files {
		plan.Selected[file.ID] = true
	}
}

func (b *bulkFileSelectionState) clearCurrentFiles() {
	plan := b.currentPlan()
	if plan == nil {
		return
	}
	plan.Selected = map[int]bool{}
}

func (b *bulkFileSelectionState) selectedCurrentFileCount() int {
	plan := b.currentPlan()
	if plan == nil {
		return 0
	}
	return len(plan.Selected)
}

func (b *bulkFileSelectionState) submissions() []bulkFileSelectionSubmission {
	if b == nil {
		return nil
	}
	submissions := make([]bulkFileSelectionSubmission, 0, len(b.Plans))
	for _, plan := range b.Plans {
		ids := plan.selectedIDs()
		if len(ids) == 0 {
			continue
		}
		submissions = append(submissions, bulkFileSelectionSubmission{ID: plan.ID, Name: plan.Name, FileIDs: ids})
	}
	return submissions
}

func (p bulkFileSelectionPlan) selectedIDs() []int {
	ids := make([]int, 0, len(p.Selected))
	for _, file := range p.Files {
		if p.Selected[file.ID] {
			ids = append(ids, file.ID)
		}
	}
	return ids
}

func (b *bulkFileSelectionState) outcomeCounts() (success int, failed int, skipped int) {
	if b == nil {
		return 0, 0, 0
	}
	for _, outcome := range b.Outcomes {
		switch outcome.Status {
		case bulkFileSelectionSuccess:
			success++
		case bulkFileSelectionFailed:
			failed++
		case bulkFileSelectionSkipped:
			skipped++
		}
	}
	return success, failed, skipped
}

func (b *bulkFileSelectionState) skippedCount() int {
	_, _, skipped := b.outcomeCounts()
	return skipped
}

func bulkFileSelectionSummaryLine(success, failed, skipped int) string {
	parts := []string{}
	if success > 0 {
		parts = append(parts, fmt.Sprintf("%d updated", success))
	}
	if failed > 0 {
		parts = append(parts, fmt.Sprintf("%d failed", failed))
	}
	if skipped > 0 {
		parts = append(parts, fmt.Sprintf("%d skipped", skipped))
	}
	if len(parts) == 0 {
		return "No torrents processed"
	}
	return strings.Join(parts, ", ")
}

func bulkFileSelectionDetailsCmd(service app.AppService, plans []bulkFileSelectionPlan) tea.Cmd {
	ordered := append([]bulkFileSelectionPlan(nil), plans...)
	return func() tea.Msg {
		details := make([]models.TorrentInfo, 0, len(ordered))
		outcomes := make([]bulkFileSelectionOutcome, 0)
		for _, plan := range ordered {
			info, err := service.TorrentInfo(context.Background(), plan.ID)
			if err != nil {
				outcomes = append(outcomes, bulkFileSelectionOutcome{ID: plan.ID, Name: plan.Name, Status: bulkFileSelectionFailed, Error: err.Error()})
				continue
			}
			details = append(details, info)
		}
		return bulkFileSelectionDetailsMsg{details: details, outcomes: outcomes}
	}
}

func bulkFileSelectionSubmitCmd(service app.AppService, submissions []bulkFileSelectionSubmission, outcomes []bulkFileSelectionOutcome) tea.Cmd {
	ordered := append([]bulkFileSelectionSubmission(nil), submissions...)
	baseOutcomes := append([]bulkFileSelectionOutcome(nil), outcomes...)
	return func() tea.Msg {
		result := bulkFileSelectionResultMsg{}
		var details []string
		for _, outcome := range baseOutcomes {
			result.total++
			switch outcome.Status {
			case bulkFileSelectionFailed:
				result.failed++
				details = append(details, fmt.Sprintf("%s: %s", outcome.Name, outcome.Error))
			case bulkFileSelectionSkipped:
				result.skipped++
			}
		}
		for idx, submission := range ordered {
			if idx > 0 {
				if err := batchTick(context.Background()); err != nil {
					result.failed += len(ordered) - idx
					result.total += len(ordered) - idx
					result.err = err
					result.detail = strings.Join(details, "\n")
					return result
				}
			}
			result.total++
			if err := service.SelectFiles(context.Background(), submission.ID, submission.FileIDs); err != nil {
				result.failed++
				details = append(details, fmt.Sprintf("%s: %s", submission.Name, err.Error()))
				continue
			}
			result.success++
		}
		result.detail = strings.Join(details, "\n")
		return result
	}
}

func renderBulkFileSelectionPopup(m Model) string {
	popupW, _ := popupSize(m.width, m.height)
	innerW := popupW - 4
	if m.bulkSelect == nil {
		return ""
	}
	plan := m.bulkSelect.currentPlan()
	if plan == nil {
		return popupBox(headStyle.Render("◆ Bulk Select Files"), m.spinner.View()+" "+mutedStyle.Render("loading file choices..."), innerW, false)
	}
	scrollInd := headStyle.Render(fmt.Sprintf(" %d/%d ", m.bulkSelect.Prompt+1, len(m.bulkSelect.Plans)))
	title := headStyle.Render("◆ Bulk Select Files") + "  " + mutedStyle.Render(scrollInd)
	lines := []string{mutedStyle.Render("  " + truncateLine(plan.Name, innerW-4)), ""}
	if skipped := m.bulkSelect.skippedCount(); skipped > 0 {
		lines = append(lines, warnStyle.Render(fmt.Sprintf("  %d ineligible marked torrent(s) will be skipped.", skipped)), "")
	}
	visibleH := max(1, popupW/3-9)
	start, end := fileListWindow(len(plan.Files), m.bulkSelect.FileCursor, visibleH)
	for idx := start; idx < end; idx++ {
		file := plan.Files[idx]
		cursor := "  "
		if idx == m.bulkSelect.FileCursor {
			cursor = warnStyle.Render("▸ ")
		}
		marker := subtleStyle.Render("[ ]")
		if plan.Selected[file.ID] {
			marker = okStyle.Render("[✓]")
		}
		label := fmt.Sprintf("%s (%s)", file.Path, humanBytes(file.Bytes))
		lines = append(lines, fmt.Sprintf("%s%s %s", cursor, marker, truncateLine(label, innerW-8)))
	}
	if m.errText != "" {
		lines = append(lines, "", errorStyle.Render("  ✗ "+m.errText))
	}
	lines = append(lines, "", popupShortcutFooter(m))
	return popupBox(title, strings.Join(lines, "\n"), innerW, false)
}
