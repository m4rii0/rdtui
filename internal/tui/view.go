package tui

import (
	"fmt"
	"math"
	"slices"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/m4rii0/rdtui/internal/debug"
	"github.com/m4rii0/rdtui/pkg/models"
)

func renderView(m Model) string {
	debug.Log("renderView: mode=%s width=%d height=%d loading=%v", m.mode, m.width, m.height, m.loading)
	if m.helpVisible {
		bg := appStyle.Render(renderBackground(m))
		return boundView(renderOverlayOnBackground(bg, renderHelpOverlay(m), m.width, m.height), m.width, m.height)
	}
	if isPopupMode(m.mode) {
		bg := appStyle.Render(renderBackground(m))
		return boundView(renderOverlayOnBackground(bg, renderPopupContent(m), m.width, m.height), m.width, m.height)
	}
	if activityBase(m.mode) && ((m.width > 0 && m.width < 40) || (m.height > 0 && m.height < 12)) {
		return renderCompactMain(m)
	}
	return boundView(appStyle.Render(renderBackground(m)), m.width, m.height)
}

func username(m Model) string {
	if m.session != nil {
		return m.session.User.Username
	}
	return ""
}

func renderAuthChoice(m Model) string {
	lines := []string{
		renderBanner(m.version),
		"",
		subtleStyle.Render("── Authentication ──────────────────────────────"),
		"",
		textStyle.Render("  Choose an authentication method:"),
		"",
		"  " + footerKeyStyle.Render("1") + " " + footerDescStyle.Render("/ ") + footerKeyStyle.Render("t") + "  " + textStyle.Render("Private API token"),
		"  " + footerKeyStyle.Render("2") + " " + footerDescStyle.Render("/ ") + footerKeyStyle.Render("d") + "  " + textStyle.Render("Device auth (browser)"),
		"",
		mutedStyle.Render("  q  quit"),
	}
	if m.retryBootstrap {
		lines = append(lines, "", "  "+footerKeyStyle.Render("r / enter")+"  "+textStyle.Render("Retry saving credentials"))
	}
	if m.errText != "" {
		lines = append(lines, "", errorStyle.Render("  ✗ "+m.errText))
	}
	if m.status != "" {
		lines = append(lines, "", infoStyle.Render("  ▸ "+m.status))
	}
	return authLayout(m, lines)
}

func renderInput(m Model) string {
	lines := []string{
		renderBanner(m.version),
		"",
		subtleStyle.Render("── Input ───────────────────────────────────────"),
		"",
		headStyle.Render("  ▸ " + m.inputPrompt),
		"  " + m.input.View(),
		"",
		mutedStyle.Render("  enter  submit   esc  cancel"),
	}
	if m.errText != "" {
		lines = append(lines, "", errorStyle.Render("  ✗ "+m.errText))
	}
	return authLayout(m, lines)
}

func renderDeviceAuth(m Model) string {
	lines := []string{
		renderBanner(m.version),
		"",
		subtleStyle.Render("── Device Authentication ───────────────────────"),
		"",
	}
	if m.deviceCode != nil {
		lines = append(lines,
			textStyle.Render("  Open this URL in your browser and enter the code:"),
			"",
			fmt.Sprintf("  %s  %s", mutedStyle.Render("Code:"), okStyle.Render("  "+m.deviceCode.UserCode+"  ")),
			fmt.Sprintf("  %s  %s", mutedStyle.Render("URL: "), infoStyle.Render(m.deviceCode.VerificationURL)),
		)
		if m.deviceCode.DirectVerificationURL != "" {
			lines = append(lines, fmt.Sprintf("  %s  %s", mutedStyle.Render("Direct:"), infoStyle.Render(m.deviceCode.DirectVerificationURL)))
		}
		remaining := time.Duration(m.deviceCode.ExpiresIn)*time.Second - time.Since(m.deviceCode.RequestedAt)
		if remaining > 0 {
			lines = append(lines, "", mutedStyle.Render(fmt.Sprintf("  Expires in: %s", remaining.Round(time.Second))))
		}
	}
	lines = append(lines, "", mutedStyle.Render("  enter / r  poll now   esc  cancel"))
	if m.errText != "" {
		lines = append(lines, "", errorStyle.Render("  ✗ "+m.errText))
	}
	if m.loading {
		lines = append(lines, "", infoStyle.Render("  ⠋ Waiting for authorization..."))
	}
	return authLayout(m, lines)
}

// Keep authentication actionable when the banner and spacing exceed the screen.
func authLayout(m Model, lines []string) string {
	available := m.height - appStyle.GetVerticalFrameSize()
	if m.height > 0 && lipgloss.Height(strings.Join(lines, "\n")) > available {
		compact := make([]string, 0, len(lines)-1)
		for _, line := range lines[1:] {
			if strings.TrimSpace(line) != "" {
				compact = append(compact, strings.TrimPrefix(line, "  "))
			}
		}
		lines = compact
	}
	return strings.Join(lines, "\n")
}

func renderMain(m Model) string {
	width := m.width
	if width <= 0 {
		width = 120
	}
	innerWidth := max(1, width-4)
	tableHeight := mainTableHeight(width, m.height)

	header := renderHeaderLine(renderCompactHeader(m.version, username(m)), m, innerWidth)

	table := renderTorrentList(m, innerWidth, tableHeight)

	lines := []string{header, table, oneLine(renderSearchBar(m), innerWidth), renderActivity(m, innerWidth), renderFeedback(m, innerWidth)}
	footer := activityFooter(m, innerWidth)
	lines = append(lines, dividerLine(innerWidth), truncateLine(footer, innerWidth))
	for i := range lines {
		if i != 1 {
			lines[i] = oneLine(lines[i], innerWidth)
		}
	}
	return strings.Join(lines, "\n")
}

func renderDetailView(m Model) string {
	width := m.width
	if width <= 0 {
		width = 120
	}
	innerWidth := max(20, width-4)

	header := renderHeaderLine(renderCompactHeader(m.version, username(m)), m, innerWidth)

	lines := []string{header, ""}

	if m.detail == nil {
		lines = append(lines, m.spinner.View()+" "+mutedStyle.Render("loading torrent details..."))
	} else {
		info := m.detail
		detailLines := []string{
			sectionTitle("Torrent Details"),
			"",
			fmt.Sprintf("  %s  %s", mutedStyle.Render("Name:    "), textStyle.Render(info.Filename)),
			fmt.Sprintf("  %s  %s", mutedStyle.Render("Status:  "), styledStatus(info.Status)),
			fmt.Sprintf("  %s  %s%%", mutedStyle.Render("Progress:"), infoStyle.Render(formatProgress(info.Progress))),
			fmt.Sprintf("  %s  %s", mutedStyle.Render("Size:    "), infoStyle.Render(humanBytes(max64(info.Bytes, info.OriginalBytes)))),
		}
		if !info.Added.IsZero() {
			detailLines = append(detailLines, fmt.Sprintf("  %s  %s", mutedStyle.Render("Added:   "), mutedStyle.Render(info.Added.Format(time.RFC822))))
		}
		if len(info.Files) > 0 {
			detailLines = append(detailLines, "", subtleStyle.Render("  ── Files ──"))
			for _, file := range info.Files {
				marker := subtleStyle.Render("[ ]")
				if file.Selected {
					marker = okStyle.Render("[✓]")
				}
				detailLines = append(detailLines, fmt.Sprintf("    %s %s %s", marker, textStyle.Render(file.Path), mutedStyle.Render("("+humanBytes(file.Bytes)+")")))
			}
		}
		if len(info.Links) > 0 {
			detailLines = append(detailLines, "", fmt.Sprintf("  %s  %s", mutedStyle.Render("Links:   "), infoStyle.Render(fmt.Sprintf("%d generated", len(info.Links)))))
		}
		lines = append(lines, panelStyle.Width(innerWidth).Render(strings.Join(detailLines, "\n")))
	}

	return finishWorkbench(m, lines, innerWidth)
}

func renderDownloadView(m Model) string {
	width := m.width
	if width <= 0 {
		width = 120
	}
	innerWidth := max(20, width-4)

	header := renderHeaderLine(renderCompactHeader(m.version, username(m)), m, innerWidth)

	lines := []string{header, ""}
	if m.download == nil {
		lines = append(lines, m.spinner.View()+" "+mutedStyle.Render("preparing download..."))
	} else {
		d := m.download
		pct := d.Progress() / 100.0
		progBar := m.progress.ViewAs(pct)
		stats := []string{
			sectionTitle("Managed Download"),
			"",
			fmt.Sprintf("  %s  %s", mutedStyle.Render("File:       "), textStyle.Render(d.Filename)),
			fmt.Sprintf("  %s  %s", mutedStyle.Render("Status:     "), managedDownloadStatusLabel(d.Status)),
			fmt.Sprintf("  %s  %s  %.1f%%", mutedStyle.Render("Progress:   "), progBar, d.Progress()),
			fmt.Sprintf("  %s  %s / %s", mutedStyle.Render("Transferred:"), infoStyle.Render(humanBytes(d.CompletedLength)), mutedStyle.Render(humanBytes(d.TotalLength))),
			fmt.Sprintf("  %s  %s/s", mutedStyle.Render("Speed:      "), infoStyle.Render(humanBytes(d.DownloadSpeed))),
			fmt.Sprintf("  %s  %s", mutedStyle.Render("ETA:        "), textStyle.Render(formatETA(d.ETA()))),
		}
		if d.Connections > 0 {
			stats = append(stats, fmt.Sprintf("  %s  %d", mutedStyle.Render("Connections:"), d.Connections))
		}
		if d.FilePath != "" {
			stats = append(stats, fmt.Sprintf("  %s  %s", mutedStyle.Render("Path:       "), mutedStyle.Render(d.FilePath)))
		} else if d.Directory != "" {
			stats = append(stats, fmt.Sprintf("  %s  %s", mutedStyle.Render("Directory:  "), mutedStyle.Render(d.Directory)))
		}
		if d.ErrorMessage != "" {
			stats = append(stats, "", errorStyle.Render("  ✗ "+d.ErrorMessage))
		}
		lines = append(lines, boxStyle.Width(innerWidth).Render(strings.Join(stats, "\n")))
	}
	return finishWorkbench(m, lines, innerWidth)
}

func renderTorrentList(m Model, width, height int) string {
	if height <= 0 {
		height = 20
	}
	if width <= 0 {
		width = 20
	}
	vis := m.visibleTorrents()
	count := fmt.Sprintf("[%d]", len(vis))
	if len(vis) == 0 && (m.filterApplied || m.mode == modeSearch) {
		count = fmt.Sprintf("[0/%d]", len(m.torrents))
	}
	title := headStyle.Render("▸ Torrents") + "  " + subtleStyle.Render(count)
	if m.batchMode {
		badge := warnStyle.Render(fmt.Sprintf("BATCH · %d selected", len(m.batchSelected)))
		title = oneLine(headStyle.Render("Torrents")+" "+subtleStyle.Render(count), max(0, width-lipgloss.Width(badge)-3)) + " · " + badge
	}
	title = oneLine(title, width)
	if len(vis) == 0 {
		if m.filterApplied || m.mode == modeSearch {
			return strings.Join(fitLines([]string{
				title,
				"",
				mutedStyle.Render("  No matches"),
			}, height), "\n")
		}
		return strings.Join(fitLines([]string{
			title,
			"",
			mutedStyle.Render("  No torrents loaded"),
		}, height), "\n")
	}
	bodyHeight := max(1, height-3) // +1 for divider line
	showScrollbar := len(vis) > bodyHeight
	const markPadding = "  "
	colWidth := width - len(markPadding)
	columns := tableColumns(colWidth, showScrollbar)
	header := markPadding + renderTableHeader(columns, m.sortColumn, m.sortAscending, colWidth)
	headerDiv := dividerLine(width)
	start, end := torrentListWindow(len(vis), m.selectedIdx, bodyHeight)
	thumbTop, thumbSize := scrollbarThumb(len(vis), bodyHeight, start)

	var bodyLines []string
	for row, idx := 0, start; idx < end; row, idx = row+1, idx+1 {
		t := vis[idx]
		mark := markPadding
		if m.batchMode {
			if m.batchSelected[t.ID] {
				mark = "* "
			} else {
				mark = "· "
			}
		}
		rowStr := mark + renderTableRow(t, columns, m.filterMatches[t.ID], idx == m.selectedIdx)
		if showScrollbar {
			rowStr = truncateLine(rowStr, max(1, width-2)) + " " + mutedStyle.Render(scrollbarGlyph(row, thumbTop, thumbSize))
		}
		if idx == m.selectedIdx || (m.batchMode && m.batchSelected[t.ID]) {
			if m.batchMode && m.batchSelected[t.ID] {
				rowStr = batchMarkedStyle.Width(width).Render(truncateLine(rowStr, width))
			} else {
				rowStr = truncateLine(rowStr, width)
			}
		} else {
			rowStr = truncateLine(rowStr, width)
		}
		bodyLines = append(bodyLines, rowStr)
	}

	lines := []string{title, header, headerDiv}
	lines = append(lines, bodyLines...)
	return strings.Join(fitLines(lines, height), "\n")
}

func renderSearchBar(m Model) string {
	if m.mode != modeSearch && !m.filterApplied {
		return ""
	}
	vis := m.visibleTorrents()
	count := infoStyle.Render(fmt.Sprintf("%d", len(vis))) + mutedStyle.Render("/"+fmt.Sprintf("%d", len(m.torrents)))
	prefix := searchStyle.Render("▸ ") + mutedStyle.Render("search: ")
	if m.filterApplied && m.mode != modeSearch {
		prefix = warnStyle.Render("▸ ") + mutedStyle.Render("filter: ")
	}
	suffix := "  " + mutedStyle.Render("[") + count + mutedStyle.Render("]")
	if (m.width > 0 && m.width < 40) || (m.height > 0 && m.height < 12) {
		// Resize the viewport before rendering so it scrolls with the cursor.
		m.searchInput.SetWidth(max(1, m.width-lipgloss.Width(prefix+suffix+m.searchInput.Prompt)-1))
		m.searchInput.SetCursor(m.searchInput.Position())
	}
	return prefix + m.searchInput.View() + suffix
}

type shortcutHint struct {
	Key     string
	Desc    string
	Enabled bool
}

func renderFooterShortcuts(items ...shortcutHint) string {
	parts := make([]string, 0, len(items))
	for _, item := range items {
		keyStyle := footerKeyStyle
		descStyle := footerDescStyle
		if !item.Enabled {
			keyStyle = footerKeyDimStyle
			descStyle = footerDescDimStyle
		}
		part := keyStyle.Render(item.Key)
		if item.Desc != "" {
			part += " " + descStyle.Render(item.Desc)
		}
		parts = append(parts, part)
	}
	return strings.Join(parts, footerSepStyle.Render("  ▪  "))
}

func renderHeaderLine(header string, m Model, width int) string {
	if !m.loading {
		return truncateLine(header, width)
	}

	working := m.spinner.View() + " " + mutedStyle.Render("working...")
	workingWidth := lipgloss.Width(working)
	if width <= workingWidth+2 {
		return truncateLine(working, width)
	}

	headerWidth := width - workingWidth - 2
	left := truncateLine(header, headerWidth)
	gap := width - lipgloss.Width(left) - workingWidth
	if gap < 2 {
		gap = 2
	}
	return left + strings.Repeat(" ", gap) + working
}

func listFooter(m Model) string {
	footer := renderShortcutFooter(m.renderShortcutDefs(), m)
	if m.mode == modeMain && m.batchMode {
		footer += footerSepStyle.Render("  ──  ") + warnStyle.Render(fmt.Sprintf("marked: %d", len(m.batchSelected)))
	}
	return footer
}

func detailFooter() string {
	m := Model{mode: modeDetail, detail: &models.TorrentInfo{Status: "downloaded"}}
	return renderShortcutFooter(m.renderShortcutDefs(), m)
}

func downloadFooter(download *models.ManagedDownload, canDeleteTorrent bool) string {
	m := Model{mode: modeDownload, download: download}
	if canDeleteTorrent {
		m.downloadTorrentID = "torrent"
	}
	return renderShortcutFooter(m.renderShortcutDefs(), m)
}

func managedDownloadStatusLabel(status models.ManagedDownloadStatus) string {
	text := string(status)
	if text == "" {
		text = "unknown"
	}
	switch status {
	case models.ManagedDownloadStatusComplete:
		return statusDownloadedStyle.Render(text)
	case models.ManagedDownloadStatusActive:
		return statusDownloadingStyle.Render(text)
	case models.ManagedDownloadStatusWaiting, models.ManagedDownloadStatusPaused:
		return statusWaitingStyle.Render(text)
	case models.ManagedDownloadStatusError, models.ManagedDownloadStatusRemoved:
		return statusErrorStyle.Render(text)
	default:
		return mutedStyle.Render(text)
	}
}

func styledStatusLabel(status, text string) string {
	switch status {
	case "downloaded":
		return statusDownloadedStyle.Render(text)
	case "downloading":
		return statusDownloadingStyle.Render(text)
	case "queued":
		return statusWaitingStyle.Render(text)
	case "compressing":
		return statusWaitingStyle.Render(text)
	case "uploading":
		return statusWaitingStyle.Render(text)
	case "waiting_files_selection", "magnet_conversion":
		return statusWaitingStyle.Render(text)
	case "error":
		return statusErrorStyle.Render(text)
	case "dead":
		return statusErrorStyle.Render(text)
	case "virus":
		return statusErrorStyle.Render(text)
	case "magnet_error":
		return statusErrorStyle.Render(text)
	default:
		return mutedStyle.Render(text)
	}
}

func styledStatus(status string) string {
	switch status {
	case "downloaded":
		return statusDownloadedStyle.Render(status)
	case "downloading":
		return statusDownloadingStyle.Render(status)
	case "waiting_files_selection", "magnet_conversion":
		return statusWaitingStyle.Render(status)
	case "error", "dead", "virus", "magnet_error":
		return statusErrorStyle.Render(status)
	default:
		return status
	}
}

func humanBytes(bytes int64) string {
	if bytes <= 0 {
		return "0 B"
	}
	units := []string{"B", "KB", "MB", "GB", "TB"}
	value := float64(bytes)
	unit := 0
	for value >= 1024 && unit < len(units)-1 {
		value /= 1024
		unit++
	}
	if unit == 0 {
		return fmt.Sprintf("%d %s", bytes, units[unit])
	}
	return fmt.Sprintf("%.1f %s", value, units[unit])
}

func formatByteDiff(current int64, remote int64) string {
	diff := remote - current
	if diff == 0 {
		return "same size"
	}
	if diff > 0 {
		return humanBytes(diff) + " smaller than remote"
	}
	return humanBytes(-diff) + " larger than remote"
}

func formatAddedTime(value time.Time) string {
	if value.IsZero() {
		return "-"
	}
	return value.Local().Format("02/01/2006 15:04")
}

func formatProgress(progress float64) string {
	if progress == math.Trunc(progress) {
		return fmt.Sprintf("%.0f", progress)
	}
	return fmt.Sprintf("%.2f", progress)
}

func formatETA(value time.Duration) string {
	if value < 0 {
		return "Calculating..."
	}
	if value == 0 {
		return "—"
	}
	return formatDuration(value)
}

func formatDuration(value time.Duration) string {
	if value < 0 {
		value = -value
	}
	hours := int(value / time.Hour)
	value -= time.Duration(hours) * time.Hour
	minutes := int(value / time.Minute)
	value -= time.Duration(minutes) * time.Minute
	seconds := int(value / time.Second)
	if hours > 0 {
		return fmt.Sprintf("%dh %dm %ds", hours, minutes, seconds)
	}
	if minutes > 0 {
		return fmt.Sprintf("%dm %ds", minutes, seconds)
	}
	return fmt.Sprintf("%ds", seconds)
}

func torrentListWindow(total, selected, visible int) (int, int) {
	if visible <= 0 || total <= visible {
		return 0, total
	}
	start := selected - visible/2
	if start < 0 {
		start = 0
	}
	maxStart := total - visible
	if start > maxStart {
		start = maxStart
	}
	return start, start + visible
}

func scrollbarThumb(total, visible, start int) (int, int) {
	if total <= visible || visible <= 0 {
		return 0, visible
	}
	thumbSize := int(math.Round(float64(visible*visible) / float64(total)))
	if thumbSize < 1 {
		thumbSize = 1
	}
	if thumbSize > visible {
		thumbSize = visible
	}
	track := visible - thumbSize
	if track <= 0 {
		return 0, thumbSize
	}
	maxStart := total - visible
	thumbTop := int(math.Round(float64(start) * float64(track) / float64(maxStart)))
	if thumbTop < 0 {
		thumbTop = 0
	}
	if thumbTop > track {
		thumbTop = track
	}
	return thumbTop, thumbSize
}

func scrollbarGlyph(row, thumbTop, thumbSize int) string {
	if row >= thumbTop && row < thumbTop+thumbSize {
		return "█"
	}
	return "│"
}

func fitLines(lines []string, height int) []string {
	if height <= 0 {
		return lines
	}
	if len(lines) > height {
		trimmed := slices.Clone(lines[:height])
		if height > 0 {
			trimmed[height-1] = mutedStyle.Render("...")
		}
		return trimmed
	}
	for len(lines) < height {
		lines = append(lines, "")
	}
	return lines
}

func truncateLine(line string, width int) string {
	if width <= 0 {
		return ""
	}
	return ansi.Truncate(line, width, "…")
}

func max64(values ...int64) int64 {
	var out int64
	for _, value := range values {
		if value > out {
			out = value
		}
	}
	return out
}
