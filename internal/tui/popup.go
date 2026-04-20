package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)



func renderOverlay(termW, termH int, content string) string {
	if termW <= 0 {
		termW = 80
	}
	if termH <= 0 {
		termH = 24
	}
	if termW < 40 || termH < 12 {
		return content
	}
	return lipgloss.Place(termW, termH, lipgloss.Center, lipgloss.Center, content)
}

func renderOverlayOnBackground(bg, popup string, termW, termH int) string {
	if termW <= 0 {
		termW = 80
	}
	if termH <= 0 {
		termH = 24
	}
	if termW < 40 || termH < 12 {
		return popup
	}

	popLines := strings.Split(popup, "\n")
	popH := len(popLines)
	popW := 0
	for _, l := range popLines {
		if w := lipgloss.Width(l); w > popW {
			popW = w
		}
	}

	startRow := max(0, (termH-popH)/2)
	startCol := max(0, (termW-popW)/2)

	bgLines := strings.Split(bg, "\n")
	for len(bgLines) < termH {
		bgLines = append(bgLines, "")
	}

	result := make([]string, len(bgLines))
	copy(result, bgLines)

	for i, popLine := range popLines {
		row := startRow + i
		if row >= len(result) {
			break
		}
		if lipgloss.Width(popLine) == 0 {
			continue
		}
		left := ansi.Truncate(result[row], startCol, "")
		leftW := lipgloss.Width(left)
		if leftW < startCol {
			left += strings.Repeat(" ", startCol-leftW)
		}
		result[row] = left + popLine
	}

	return strings.Join(result, "\n")
}

func popupBox(title, content string, width int, danger bool) string {
	style := popupBoxStyle
	if danger {
		style = dangerStyle
	}
	return style.Width(width).Render(title + "\n" + content)
}

func popupFooter(shortcuts ...shortcutHint) string {
	return mutedStyle.Render(renderFooterShortcuts(shortcuts...))
}

func popupShortcutFooter(m Model) string {
	return mutedStyle.Render(renderShortcutFooter(m.renderShortcutDefs(), m))
}

func popupSize(termW, termH int) (int, int) {
	w := max(40, termW*7/10)
	h := max(10, termH/2)
	return w, h
}

func isPopupMode(m mode) bool {
	switch m {
	case modeSelectFiles, modeDelete, modeChooseTarget,
		modeOverwrite, modeShowURL, modeFileBrowser,
		modeMagnetInput, modeURLInput:
		return true
	}
	return false
}

func renderPopupContent(m Model) string {
	switch m.mode {
	case modeSelectFiles:
		return renderSelectFilesPopup(m)
	case modeDelete:
		return renderDeletePopup(m)
	case modeChooseTarget:
		return renderTargetPickerPopup(m)
	case modeOverwrite:
		return renderOverwritePopup(m)
	case modeShowURL:
		return renderShowURLPopup(m)
	case modeFileBrowser:
		return renderFileBrowserContent(m)
	case modeMagnetInput:
		return renderInputPopup(m, "Paste a magnet link")
	case modeURLInput:
		return renderInputPopup(m, "Paste a .torrent URL")
	}
	return ""
}

func renderBackground(m Model) string {
	switch {
	case m.mode == modeStarting:
		return "Starting..."
	case m.mode == modeAuthChoice:
		return renderAuthChoice(m)
	case m.mode == modeTokenInput:
		return renderInput(m)
	case m.mode == modeDeviceAuth:
		return renderDeviceAuth(m)
	case m.mode == modeDetail:
		return renderDetailView(m)
	case m.mode == modeDownload:
		return renderDownloadView(m)
	case m.mode == modeMain, m.mode == modeSearch,
		m.mode == modeMagnetInput, m.mode == modeURLInput:
		return renderMain(m)
	default:
		if m.returnMode == modeDetail {
			return renderDetailView(m)
		}
		if m.returnMode == modeDownload {
			return renderDownloadView(m)
		}
		return renderMain(m)
	}
}

func renderFileBrowserContent(m Model) string {
	popupW, popupH := popupSize(m.width, m.height)
	innerW := popupW - 4
	innerH := popupH - 2

	title := headStyle.Render("◆ Import: ") + mutedStyle.Render(m.browser.CurrentDir)
	if m.browser.EditingPath {
		title = headStyle.Render("◆ Import: ") + infoStyle.Render("edit path")
	}
	content := m.browser.view(innerW, innerH)
	if m.errText != "" {
		content += "\n" + errorStyle.Render(m.errText)
	}
	return popupBox(title, content, innerW, false)
}

func renderSelectFilesPopup(m Model) string {
	popupW, _ := popupSize(m.width, m.height)
	innerW := popupW - 4

	files := m.selector.Files
	total := len(files)
	scrollInd := ""
	if total > 0 {
		scrollInd = headStyle.Render(fmt.Sprintf(" %d/%d ", m.selector.Cursor+1, total))
	}

	title := headStyle.Render("◆ Select Files") + "  " + mutedStyle.Render(scrollInd)

	var lines []string
	visibleH := max(1, popupW/3-6)
	start, end := fileListWindow(total, m.selector.Cursor, visibleH)
	for idx := start; idx < end; idx++ {
		file := files[idx]
		cursor := "  "
		if idx == m.selector.Cursor {
			cursor = warnStyle.Render("▸ ")
		}
		marker := subtleStyle.Render("[ ]")
		if m.selector.Selected[file.ID] {
			marker = okStyle.Render("[✓]")
		}
		lines = append(lines, fmt.Sprintf("%s%s %s (%s)", cursor, marker, file.Path, humanBytes(file.Bytes)))
	}

	content := strings.Join(lines, "\n")
	content += "\n\n" + popupShortcutFooter(m)
	return popupBox(title, content, innerW, false)
}

func renderDeletePopup(m Model) string {
	popupW, _ := popupSize(m.width, m.height)
	innerW := popupW - 4

	var title, body string
	if len(m.deleteIDs) > 1 {
		title = headStyle.Render("◆ Delete Torrents")
		lines := []string{
			warnStyle.Render(fmt.Sprintf("  Delete %d torrent(s)?", len(m.deleteIDs))),
			"",
		}
		count := 0
		for _, id := range m.deleteIDs {
			name := id
			for _, t := range m.torrents {
				if t.ID == id {
					name = t.Filename
					break
				}
			}
			if count >= 5 {
				lines = append(lines, mutedStyle.Render(fmt.Sprintf("  … and %d more", len(m.deleteIDs)-5)))
				break
			}
			lines = append(lines, "  "+mutedStyle.Render("▸ ")+truncateLine(name, innerW-4))
			count++
		}
		lines = append(lines, "", popupShortcutFooter(m))
		body = strings.Join(lines, "\n")
	} else {
		id := ""
		if len(m.deleteIDs) > 0 {
			id = m.deleteIDs[0]
		}
		name := id
		if m.detail != nil && m.detail.ID == id {
			name = m.detail.Filename
		}
		for _, t := range m.torrents {
			if t.ID == id {
				name = t.Filename
				break
			}
		}
		title = headStyle.Render("◆ Delete Torrent")
		body = strings.Join([]string{
			warnStyle.Render("  Permanently delete this torrent?"),
			"",
			"  " + mutedStyle.Render("▸ ") + truncateLine(name, innerW-4),
			"",
			popupShortcutFooter(m),
		}, "\n")
	}
	return popupBox(title, body, innerW, true)
}

func renderTargetPickerPopup(m Model) string {
	popupW, _ := popupSize(m.width, m.height)
	innerW := popupW - 4

	verb := "Copy URL"
	if m.targets.Action == handoffDownload {
		verb = "Download"
	}
	title := headStyle.Render("◆ " + verb)

	var lines []string
	for idx, item := range m.targets.Items {
		prefix := "   "
		if idx == m.targets.Cursor {
			prefix = warnStyle.Render(" ▸ ")
		}
		label := item.Label
		if item.FilePath != "" {
			label = fmt.Sprintf("%s (%s)", label, item.FilePath)
		}
		lines = append(lines, prefix+truncateLine(label, innerW-4))
	}
	lines = append(lines, "", popupShortcutFooter(m))
	return popupBox(title, strings.Join(lines, "\n"), innerW, false)
}

func renderOverwritePopup(m Model) string {
	popupW, _ := popupSize(m.width, m.height)
	innerW := popupW - 4

	if m.pendingDownload == nil {
		return ""
	}
	pending := m.pendingDownload
	title := headStyle.Render("◆ File Already Exists")
	lines := []string{
		fmt.Sprintf("  %s  %s", mutedStyle.Render("File:    "), truncateLine(pending.Filename, innerW-14)),
		fmt.Sprintf("  %s  %s", mutedStyle.Render("Path:    "), mutedStyle.Render(truncateLine(pending.Path, innerW-14))),
		fmt.Sprintf("  %s  %s", mutedStyle.Render("Current: "), infoStyle.Render(humanBytes(pending.ExistingBytes))),
	}
	if pending.RemoteBytes > 0 {
		lines = append(lines,
			fmt.Sprintf("  %s  %s", mutedStyle.Render("Remote:  "), infoStyle.Render(humanBytes(pending.RemoteBytes))),
			fmt.Sprintf("  %s  %s", mutedStyle.Render("Diff:    "), warnStyle.Render(formatByteDiff(pending.ExistingBytes, pending.RemoteBytes))),
		)
	}
	lines = append(lines, "", popupShortcutFooter(m))
	return popupBox(title, strings.Join(lines, "\n"), innerW, false)
}

func renderShowURLPopup(m Model) string {
	popupW, _ := popupSize(m.width, m.height)
	innerW := popupW - 4

	title := headStyle.Render("◆ Direct URL")
	content := strings.Join([]string{
		infoStyle.Render(truncateLine(m.showURL, innerW)),
		"",
		popupShortcutFooter(m),
	}, "\n")
	return popupBox(title, content, innerW, false)
}

func renderInputPopup(m Model, prompt string) string {
	popupW, _ := popupSize(m.width, m.height)
	innerW := popupW - 4

	title := headStyle.Render("◆ " + m.inputPrompt)
	content := strings.Join([]string{
		"  " + m.input.View(),
		"",
		popupFooter(
			shortcutHint{Key: "enter", Desc: "submit"},
			shortcutHint{Key: "esc", Desc: "cancel"},
		),
	}, "\n")
	if m.errText != "" {
		content += "\n" + errorStyle.Render("  ✗ "+m.errText)
	}
	return popupBox(title, content, innerW, false)
}

func fileListWindow(total, selected, visible int) (int, int) {
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
