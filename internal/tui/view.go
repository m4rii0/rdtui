package tui

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/m4rii0/rdtui/internal/debug"
)

var (
	appStyle               = lipgloss.NewStyle().Padding(1, 2)
	headStyle              = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212"))
	mutedStyle             = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	errorStyle             = lipgloss.NewStyle().Foreground(lipgloss.Color("203")).Bold(true)
	okStyle                = lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Bold(true)
	boxStyle               = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1)
	selectedStyle          = lipgloss.NewStyle().Foreground(lipgloss.Color("229")).Background(lipgloss.Color("62")).Bold(true)
	headerRowStyle         = lipgloss.NewStyle().Foreground(lipgloss.Color("252")).Bold(true)
	headerSelColStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("229")).Background(lipgloss.Color("62")).Bold(true)
	statusDownloadedStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	statusDownloadingStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("87"))
	statusWaitingStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("220"))
	statusErrorStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
	batchMarkedStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("229")).Background(lipgloss.Color("237"))
)

func renderView(m Model) string {
	debug.Log("renderView: mode=%s width=%d height=%d loading=%v", m.mode, m.width, m.height, m.loading)
	if m.mode == modeFileBrowser {
		return renderFileBrowserPopup(m)
	}
	var body string
	switch m.mode {
	case modeStarting:
		body = "Starting..."
	case modeAuthChoice:
		body = renderAuthChoice(m)
	case modeTokenInput, modeMagnetInput, modeURLInput:
		body = renderInput(m)
	case modeDeviceAuth:
		body = renderDeviceAuth(m)
	case modeDetail:
		body = renderDetailView(m)
	case modeMain:
		body = renderMain(m)
	default:
		if m.returnMode == modeDetail {
			body = renderDetailView(m)
		} else {
			body = renderMain(m)
		}
	}
	return appStyle.Render(body)
}

func renderAuthChoice(m Model) string {
	title := headStyle.Render("rdtui")
	if m.version != "" {
		title += " " + mutedStyle.Render("v"+m.version)
	}
	lines := []string{
		title,
		"",
		"Choose an authentication method:",
		"  1. Private API token",
		"  2. Device auth",
		"",
		mutedStyle.Render("Press 1/t for token, 2/d for device auth, q to quit"),
	}
	if m.errText != "" {
		lines = append(lines, "", errorStyle.Render(m.errText))
	}
	if m.status != "" {
		lines = append(lines, "", mutedStyle.Render(m.status))
	}
	return strings.Join(lines, "\n")
}

func renderInput(m Model) string {
	title := headStyle.Render("rdtui")
	if m.version != "" {
		title += " " + mutedStyle.Render("v"+m.version)
	}
	lines := []string{
		title,
		"",
		m.inputPrompt,
		m.input.View(),
		"",
		mutedStyle.Render("Enter to submit, Esc to cancel"),
	}
	if m.errText != "" {
		lines = append(lines, "", errorStyle.Render(m.errText))
	}
	return strings.Join(lines, "\n")
}

func renderDeviceAuth(m Model) string {
	title := headStyle.Render("rdtui")
	if m.version != "" {
		title += " " + mutedStyle.Render("v"+m.version)
	}
	lines := []string{
		title,
		"",
		"Complete device authentication:",
	}
	if m.deviceCode != nil {
		lines = append(lines,
			fmt.Sprintf("User code: %s", okStyle.Render(m.deviceCode.UserCode)),
			fmt.Sprintf("Verification URL: %s", m.deviceCode.VerificationURL),
		)
		if m.deviceCode.DirectVerificationURL != "" {
			lines = append(lines, fmt.Sprintf("Direct URL: %s", m.deviceCode.DirectVerificationURL))
		}
		remaining := time.Duration(m.deviceCode.ExpiresIn)*time.Second - time.Since(m.deviceCode.RequestedAt)
		if remaining > 0 {
			lines = append(lines, fmt.Sprintf("Expires in: %s", remaining.Round(time.Second)))
		}
	}
	lines = append(lines, "", mutedStyle.Render("Press Enter or r to poll now, Esc to cancel"))
	if m.errText != "" {
		lines = append(lines, "", errorStyle.Render(m.errText))
	}
	if m.loading {
		lines = append(lines, "", mutedStyle.Render("Waiting for authorization..."))
	}
	return strings.Join(lines, "\n")
}

func renderMain(m Model) string {
	width := m.width
	if width <= 0 {
		width = 120
	}
	innerWidth := width - 4

	bodyHeight := m.height - 2
	if bodyHeight <= 0 {
		bodyHeight = 24
	}
	reserved := 3
	if m.status != "" {
		reserved++
	}
	if m.errText != "" {
		reserved++
	}
	if m.loading {
		reserved++
	}
	tableHeight := max(4, bodyHeight-reserved)

	header := headStyle.Render("rdtui")
	if m.version != "" {
		header += " " + mutedStyle.Render("v"+m.version)
	}
	if m.session != nil {
		header += "  " + mutedStyle.Render("user: "+m.session.User.Username)
	}

	table := renderTorrentList(m, innerWidth, tableHeight)

	lines := []string{header, "", table}
	if m.status != "" {
		lines = append(lines, mutedStyle.Render(m.status))
	}
	if m.errText != "" {
		lines = append(lines, errorStyle.Render(m.errText))
	}
	if modal := renderModal(m); modal != "" {
		lines = append(lines, "", boxStyle.Render(modal))
	}
	lines = append(lines, listFooter(m))
	if m.loading {
		lines = append(lines, mutedStyle.Render("Working..."))
	}
	return strings.Join(lines, "\n")
}

func renderDetailView(m Model) string {
	width := m.width
	if width <= 0 {
		width = 120
	}
	innerWidth := max(20, width-4)

	header := headStyle.Render("rdtui")
	if m.version != "" {
		header += " " + mutedStyle.Render("v"+m.version)
	}
	if m.session != nil {
		header += "  " + mutedStyle.Render("user: "+m.session.User.Username)
	}

	lines := []string{header, ""}

	if m.detail == nil {
		lines = append(lines, mutedStyle.Render("Loading torrent details..."))
	} else {
		info := m.detail
		detailLines := []string{
			"Details",
			"",
			fmt.Sprintf("  Name:     %s", info.Filename),
			fmt.Sprintf("  Status:   %s", styledStatus(info.Status)),
			fmt.Sprintf("  Progress: %s%%", formatProgress(info.Progress)),
			fmt.Sprintf("  Size:     %s", humanBytes(max64(info.Bytes, info.OriginalBytes))),
		}
		if !info.Added.IsZero() {
			detailLines = append(detailLines, fmt.Sprintf("  Added:    %s", info.Added.Format(time.RFC822)))
		}
		if len(info.Files) > 0 {
			detailLines = append(detailLines, "", "  Files:")
			for _, file := range info.Files {
				marker := "[ ]"
				if file.Selected {
					marker = "[x]"
				}
				detailLines = append(detailLines, fmt.Sprintf("    %s %s (%s)", marker, file.Path, humanBytes(file.Bytes)))
			}
		}
		if len(info.Links) > 0 {
			detailLines = append(detailLines, "", fmt.Sprintf("  Generated links: %d", len(info.Links)))
		}
		lines = append(lines, boxStyle.Width(innerWidth).Render(strings.Join(detailLines, "\n")))
	}

	if m.status != "" {
		lines = append(lines, mutedStyle.Render(m.status))
	}
	if m.errText != "" {
		lines = append(lines, errorStyle.Render(m.errText))
	}
	if modal := renderModal(m); modal != "" {
		lines = append(lines, "", boxStyle.Render(modal))
	}
	lines = append(lines, detailFooter())
	if m.loading {
		lines = append(lines, mutedStyle.Render("Working..."))
	}
	return strings.Join(lines, "\n")
}

func renderTorrentList(m Model, width, height int) string {
	if height <= 0 {
		height = 20
	}
	if width <= 0 {
		width = 20
	}
	if len(m.torrents) == 0 {
		return strings.Join(fitLines([]string{
			headStyle.Render(fmt.Sprintf("Torrents [%d]", len(m.torrents))),
			"",
			mutedStyle.Render("No torrents loaded"),
		}, height), "\n")
	}
	title := headStyle.Render(fmt.Sprintf("Torrents [%d]", len(m.torrents)))
	bodyHeight := max(1, height-2)
	showScrollbar := len(m.torrents) > bodyHeight
	colWidth := width
	if m.batchMode {
		colWidth -= 2
	}
	columns := tableColumns(colWidth, showScrollbar)
	header := renderTableHeader(columns, m.sortColumn, m.sortAscending, colWidth)
	start, end := torrentListWindow(len(m.torrents), m.selectedIdx, bodyHeight)
	thumbTop, thumbSize := scrollbarThumb(len(m.torrents), bodyHeight, start)

	var bodyLines []string
	for row, idx := 0, start; idx < end; row, idx = row+1, idx+1 {
		t := m.torrents[idx]
		mark := ""
		if m.batchMode {
			if m.batchSelected[t.ID] {
				mark = "* "
			} else {
				mark = "  "
			}
		}
		rowStr := mark + renderTableRow(t, columns)
		if showScrollbar {
			rowStr = truncateLine(rowStr, max(1, width-2)) + " " + mutedStyle.Render(scrollbarGlyph(row, thumbTop, thumbSize))
		}
		if idx == m.selectedIdx {
			rowStr = selectedStyle.Width(width).Render(truncateLine(rowStr, width))
		} else if m.batchMode && m.batchSelected[t.ID] {
			rowStr = batchMarkedStyle.Width(width).Render(truncateLine(rowStr, width))
		} else {
			rowStr = truncateLine(rowStr, width)
		}
		bodyLines = append(bodyLines, rowStr)
	}

	lines := []string{title, header}
	lines = append(lines, bodyLines...)
	return strings.Join(fitLines(lines, height), "\n")
}

func renderModal(m Model) string {
	switch m.mode {
	case modeSelectFiles:
		var lines []string
		lines = append(lines, "Select files for torrent", "", "Space=toggle  Enter=confirm  Esc=cancel", "")
		for idx, file := range m.selector.Files {
			cursor := "  "
			if idx == m.selector.Cursor {
				cursor = "> "
			}
			marker := "[ ]"
			if m.selector.Selected[file.ID] {
				marker = "[x]"
			}
			lines = append(lines, fmt.Sprintf("%s%s %s (%s)", cursor, marker, file.Path, humanBytes(file.Bytes)))
		}
		return strings.Join(lines, "\n")
	case modeChooseTarget:
		var lines []string
		verb := "Copy URL"
		if m.targets.Action == handoffLaunch {
			verb = "Launch downloader"
		}
		lines = append(lines, verb+" for:", "")
		for idx, item := range m.targets.Items {
			prefix := "  "
			if idx == m.targets.Cursor {
				prefix = "> "
			}
			label := item.Label
			if item.FilePath != "" {
				label = fmt.Sprintf("%s (%s)", label, item.FilePath)
			}
			lines = append(lines, prefix+label)
		}
		lines = append(lines, "", mutedStyle.Render("Enter=confirm  Esc=cancel"))
		return strings.Join(lines, "\n")
	case modeShowURL:
		return strings.Join([]string{"Direct URL", "", m.showURL, "", mutedStyle.Render("Enter/Esc to close")}, "\n")
	case modeDelete:
		if len(m.deleteIDs) > 1 {
			return strings.Join([]string{
				fmt.Sprintf("Delete %d torrent(s)?", len(m.deleteIDs)),
				"",
				mutedStyle.Render("y/Enter=delete  n/Esc=cancel"),
			}, "\n")
		}
		name := ""
		if len(m.deleteIDs) > 0 {
			name = m.deleteIDs[0]
		}
		if m.detail != nil && m.detail.ID == name {
			name = m.detail.Filename
		}
		return strings.Join([]string{fmt.Sprintf("Delete torrent '%s'?", name), "", mutedStyle.Render("y/Enter=delete  n/Esc=cancel")}, "\n")
	}
	return ""
}

func listFooter(m Model) string {
	if m.batchMode {
		return mutedStyle.Render(fmt.Sprintf("[BATCH] space=mark  ctrl+a=all  ctrl+d=clear  x=delete  y=copy  b/esc=exit  │  Marked: %d", len(m.batchSelected)))
	}
	return mutedStyle.Render("↑↓ j/k  enter=view  S/P/Z/D/N=sort  r=refresh  m magnet  u url  i import  b batch  s select  y copy  x delete  q quit")
}

func detailFooter() string {
	return mutedStyle.Render("esc=back  s=select  y=copy  x=delete  r=refresh")
}

func statusLabel(status string) string {
	switch status {
	case "downloaded":
		return statusDownloadedStyle.Render("DONE")
	case "downloading":
		return statusDownloadingStyle.Render("DL")
	case "queued":
		return statusWaitingStyle.Render("QD")
	case "compressing":
		return statusWaitingStyle.Render("CMP")
	case "uploading":
		return statusWaitingStyle.Render("UL")
	case "waiting_files_selection":
		return statusWaitingStyle.Render("WAIT")
	case "error":
		return statusErrorStyle.Render("ERR")
	case "dead":
		return statusErrorStyle.Render("DEAD")
	case "virus":
		return statusErrorStyle.Render("VIRUS")
	case "magnet_error":
		return statusErrorStyle.Render("MAGERR")
	default:
		return mutedStyle.Render(status)
	}
}

func styledStatus(status string) string {
	switch status {
	case "downloaded":
		return statusDownloadedStyle.Render(status)
	case "downloading":
		return statusDownloadingStyle.Render(status)
	case "waiting_files_selection":
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
		trimmed := append([]string(nil), lines[:height]...)
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

func truncateLines(lines []string, width int) []string {
	if width <= 0 {
		return lines
	}
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		out = append(out, truncateLine(line, width))
	}
	return out
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

func renderFileBrowserPopup(m Model) string {
	width := m.width
	height := m.height
	if width <= 0 {
		width = 80
	}
	if height <= 0 {
		height = 24
	}

	debug.Log("renderFileBrowserPopup: term=%dx%d mode=%s returnMode=%s", width, height, m.mode, m.returnMode)
	debug.Log("renderFileBrowserPopup: browser.CurrentDir=%s entries=%d cursor=%d selected=%d visual=%v",
		m.browser.CurrentDir, len(m.browser.Entries), m.browser.Cursor, len(m.browser.Selected), m.browser.VisualMode)

	popupW := max(40, width*7/10)
	popupH := max(10, height/2)
	innerW := popupW - 4
	innerH := popupH - 2

	debug.Log("renderFileBrowserPopup: popup=%dx%d inner=%dx%d", popupW, popupH, innerW, innerH)

	title := headStyle.Render(" Import: " + m.browser.CurrentDir + " ")
	content := m.browser.view(innerW, innerH)
	if m.errText != "" {
		content += "\n" + errorStyle.Render(m.errText)
	}
	popupBox := boxStyle.Width(innerW).Render(title + "\n" + content)

	debug.Log("renderFileBrowserPopup: popupBox lines=%d", len(strings.Split(popupBox, "\n")))

	result := lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, popupBox)

	debug.Log("renderFileBrowserPopup: result len=%d", len(result))

	return result
}
