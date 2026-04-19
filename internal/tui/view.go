package tui

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

var (
	appStyle              = lipgloss.NewStyle().Padding(1, 2)
	headStyle             = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212"))
	mutedStyle            = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	errorStyle            = lipgloss.NewStyle().Foreground(lipgloss.Color("203")).Bold(true)
	okStyle               = lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Bold(true)
	boxStyle              = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1)
	selectedStyle         = lipgloss.NewStyle().Foreground(lipgloss.Color("229")).Background(lipgloss.Color("62")).Bold(true)
	statusDownloadedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	statusWaitingStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("220"))
	statusErrorStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
)

func renderView(m Model) string {
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
	default:
		body = renderMain(m)
	}
	return appStyle.Render(body)
}

func renderAuthChoice(m Model) string {
	lines := []string{
		headStyle.Render("rdtui"),
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
	lines := []string{
		headStyle.Render("rdtui"),
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
	lines := []string{
		headStyle.Render("rdtui"),
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
	paneHeight := mainPaneHeight(m)
	width := m.width
	if width <= 0 {
		width = 120
	}
	leftWidth := max(34, width/3)
	rightWidth := max(40, width-leftWidth-6)
	leftInnerWidth := max(10, leftWidth-boxStyle.GetHorizontalFrameSize())
	rightInnerWidth := max(10, rightWidth-boxStyle.GetHorizontalFrameSize())
	left := renderTorrentList(m, leftInnerWidth, paneHeight)
	right := renderTorrentDetail(m, rightInnerWidth, paneHeight)
	leftBox := boxStyle.Width(leftWidth).Render(left)
	rightBox := boxStyle.Width(rightWidth).Render(right)
	content := lipgloss.JoinHorizontal(lipgloss.Top, leftBox, "  ", rightBox)

	lines := []string{headStyle.Render("rdtui")}
	if m.session != nil {
		lines[0] += "  " + mutedStyle.Render("user: "+m.session.User.Username)
	}
	lines = append(lines, "", content)
	if m.status != "" {
		lines = append(lines, "", mutedStyle.Render(m.status))
	}
	if m.errText != "" {
		lines = append(lines, errorStyle.Render(m.errText))
	}
	if modal := renderModal(m); modal != "" {
		lines = append(lines, "", boxStyle.Render(modal))
	}
	lines = append(lines, "", mutedStyle.Render("j/k move  r refresh  m magnet  u url  i import  s select  y copy url  x launch  d delete  q quit"))
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
	lines := []string{"Torrents"}
	if len(m.torrents) == 0 {
		lines = append(lines, "", truncateLine(mutatedOrPlain("No torrents loaded", mutedStyle), width))
		return strings.Join(fitLines(lines, height), "\n")
	}
	footerLines := 0
	visibleRows := max(1, height-2)
	if len(m.torrents) > visibleRows {
		footerLines = 1
		visibleRows = max(1, height-3)
	}
	start, end := torrentListWindow(len(m.torrents), m.selectedIdx, visibleRows)
	thumbTop, thumbSize := scrollbarThumb(len(m.torrents), visibleRows, start)

	lines = append(lines, "")
	for row, idx := 0, start; idx < end; row, idx = row+1, idx+1 {
		torrent := m.torrents[idx]
		line := fmt.Sprintf("%s  %6s%%  %s", statusGlyph(torrent.Status), formatProgress(torrent.Progress), torrent.Filename)
		if len(m.torrents) > visibleRows {
			line = lipgloss.JoinHorizontal(lipgloss.Top,
				truncateLine(line, max(1, width-2)),
				" ",
				mutedStyle.Render(scrollbarGlyph(row, thumbTop, thumbSize)),
			)
		} else {
			line = truncateLine(line, width)
		}
		if idx == m.selectedIdx {
			line = selectedStyle.Render(line)
		}
		lines = append(lines, line)
	}

	if footerLines > 0 {
		lines = append(lines, "", truncateLine(mutatedOrPlain(fmt.Sprintf("Showing %d-%d of %d", start+1, end, len(m.torrents)), mutedStyle), width))
	}

	return strings.Join(fitLines(lines, height), "\n")
}

func renderTorrentDetail(m Model, width, height int) string {
	if height <= 0 {
		height = 20
	}
	if width <= 0 {
		width = 20
	}
	if m.detail == nil {
		return strings.Join(fitLines(truncateLines([]string{"Torrent Details", "", "Select a torrent to view details."}, width), height), "\n")
	}
	info := m.detail
	lines := []string{
		"Details",
		"",
		fmt.Sprintf("Name: %s", info.Filename),
		fmt.Sprintf("Status: %s", styledStatus(info.Status)),
		fmt.Sprintf("Progress: %s%%", formatProgress(info.Progress)),
		fmt.Sprintf("Size: %s", humanBytes(max64(info.Bytes, info.OriginalBytes))),
	}
	if !info.Added.IsZero() {
		lines = append(lines, fmt.Sprintf("Added: %s", info.Added.Format(time.RFC822)))
	}
	if len(info.Files) > 0 {
		lines = append(lines, "", "Files:")
		for _, file := range info.Files {
			marker := "[ ]"
			if file.Selected {
				marker = "[x]"
			}
			lines = append(lines, fmt.Sprintf("%s %s (%s)", marker, file.Path, humanBytes(file.Bytes)))
		}
	}
	if len(info.Links) > 0 {
		lines = append(lines, "", fmt.Sprintf("Generated links: %d", len(info.Links)))
	}
	return strings.Join(fitLines(truncateLines(lines, width), height), "\n")
}

func renderModal(m Model) string {
	switch m.mode {
	case modeFileBrowser:
		height := max(12, m.height/2)
		return m.browser.view(height)
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
		name := m.deleteID
		if m.detail != nil {
			name = m.detail.Filename
		}
		return strings.Join([]string{fmt.Sprintf("Delete torrent '%s'?", name), "", mutedStyle.Render("y/Enter=delete  n/Esc=cancel")}, "\n")
	}
	return ""
}

func statusGlyph(status string) string {
	switch status {
	case "downloaded":
		return okStyle.Render("●")
	case "waiting_files_selection":
		return statusWaitingStyle.Render("●")
	case "error", "dead", "virus", "magnet_error":
		return statusErrorStyle.Render("●")
	default:
		return mutedStyle.Render("●")
	}
}

func styledStatus(status string) string {
	switch status {
	case "downloaded":
		return statusDownloadedStyle.Render(status)
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

func formatProgress(progress float64) string {
	if progress == math.Trunc(progress) {
		return fmt.Sprintf("%.0f", progress)
	}
	return fmt.Sprintf("%.2f", progress)
}

func mainPaneHeight(m Model) int {
	if m.height <= 0 {
		return 20
	}
	reserved := 8
	if m.status != "" {
		reserved++
	}
	if m.errText != "" {
		reserved++
	}
	if m.loading {
		reserved++
	}
	return max(8, m.height-reserved)
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

func mutatedOrPlain(value string, style lipgloss.Style) string {
	return style.Render(value)
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
