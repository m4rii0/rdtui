package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/m4rii0/rdtui/internal/app"
	"github.com/m4rii0/rdtui/pkg/models"
)

func (m Model) activityIsQueue() bool {
	b := m.bulk
	if b == nil || (!b.Starting && b.QueueID == 0) {
		return false
	}
	if !b.isFinished() {
		return true
	}
	return m.download == nil || (b.Snapshot.Download != nil && b.Snapshot.Download.Ref == m.downloadRef)
}

func (m Model) hasActiveWork() bool {
	return (m.bulk != nil && (m.bulk.Starting || (m.bulk.QueueID != 0 && !m.bulk.isFinished()))) || (m.download != nil && !m.download.IsTerminal())
}

func activityBase(md mode) bool {
	switch md {
	case modeMain, modeSearch, modeDetail, modeDownload, modeBulkDownload:
		return true
	}
	return false
}

func activityShortcutContext(md mode) bool {
	return activityBase(md) && md != modeSearch
}

func renderActivity(m Model, width int) string {
	prefix := "Activity · "
	if activityShortcutContext(m.mode) {
		prefix = "a activity · "
	}
	text := "No active download"
	if m.activityIsQueue() {
		b := m.bulk
		s := b.Snapshot
		switch {
		case b.Starting:
			text = "Queue starting"
		case s.Finished:
			ok, failed, skipped := b.resultCounts()
			text = "Queue finished · " + bulkSummaryLine(ok, failed, skipped)
			if canceled := b.canceledCount(); canceled > 0 {
				text += fmt.Sprintf(" · %d canceled", canceled)
			}
		case s.CurrentIndex >= 0 && s.CurrentIndex < len(s.Items):
			item := s.Items[s.CurrentIndex]
			label := bulkItemLabel(bulkQueueItem{Target: item.Item.Target, TorrentName: item.Item.TorrentName})
			status := string(item.Status)
			if item.Status == app.QueueItemActive && s.Download != nil {
				status = activityDownloadStats(s.Download.Download.Status, s.Download.Download.Progress(), s.Download.Download.DownloadSpeed, s.Download.Download.TotalLength)
			}
			text = fmt.Sprintf("Queue %d/%d · %s · %s", s.CurrentIndex+1, len(s.Items), status, label)
		default:
			text = "Queue waiting"
		}
	} else if d := m.download; d != nil {
		text = activityDownloadStats(d.Status, d.Progress(), d.DownloadSpeed, d.TotalLength) + " · " + d.Filename
		if m.observationFailed && !d.IsTerminal() {
			text = "Status unavailable; retrying · " + text
		}
	}
	return oneLine(infoStyle.Render(prefix+text), width)
}

func activityDownloadStats(status models.ManagedDownloadStatus, pct float64, speed, total int64) string {
	text := string(status)
	if total > 0 {
		text += fmt.Sprintf(" · %.1f%%", pct)
	}
	if speed > 0 {
		text += " · " + humanBytes(speed) + "/s"
	}
	return text
}

func (m Model) openActivity() (tea.Model, tea.Cmd) {
	target := modeDownload
	if m.activityIsQueue() {
		target = modeBulkDownload
	} else if m.download == nil {
		return m, nil
	}
	if m.mode == target {
		return m, nil
	}
	m.activityReturnMode = m.mode
	m.mode = target
	return m, nil
}

func activityFooter(m Model, width int) string {
	defs := m.renderShortcutDefs()
	filtered := make([]shortcutDef, 0, len(defs))
	prefix := "? help"
	if activityShortcutContext(m.mode) {
		prefix = "a activity · " + prefix
	}
	list := m.mode == modeMain || m.mode == modeSearch
	if list {
		prefix += " · ↑↓ move"
	}
	for _, def := range defs {
		movement := list && (def.action == actionMoveUp || def.action == actionMoveDown || (def.action == "" && def.binding.Help().Desc == "move"))
		if def.action != actionOpenActivity && def.action != actionToggleHelp && !movement {
			filtered = append(filtered, def)
		}
	}
	return oneLine(prefix+" · "+renderShortcutFooter(filtered, m), width)
}

func finishWorkbench(m Model, lines []string, width int) string {
	content := strings.Split(strings.Join(lines, "\n"), "\n")
	if m.height > 0 {
		content = fitLines(content, max(1, m.height-6))
	}
	content = append(content, renderActivity(m, width), renderFeedback(m, width), dividerLine(width), activityFooter(m, width))
	for i := range content {
		content[i] = oneLine(content[i], width)
	}
	return strings.Join(content, "\n")
}
