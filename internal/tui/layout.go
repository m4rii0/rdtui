package tui

import "strings"

func mainTableHeight(_, height int) int {
	if height <= 0 {
		height = 24
	}
	return max(4, height-8)
}

func mainDataRows(width, height int) int { return max(1, mainTableHeight(width, height)-3) }

func oneLine(text string, width int) string {
	return truncateLine(strings.NewReplacer("\r", " ", "\n", " ", "\t", " ").Replace(text), width)
}

func renderFeedback(m Model, width int) string {
	if m.errText != "" {
		return oneLine(errorStyle.Render("✗ "+m.errText), width)
	}
	if m.flash.message != "" {
		return oneLine(renderFlash(m), width)
	}
	if m.status != "" {
		return oneLine(mutedStyle.Render("▸ "+m.status), width)
	}
	return ""
}

func boundView(view string, width, height int) string {
	lines := strings.Split(view, "\n")
	if height > 0 && len(lines) > height {
		lines = lines[:height]
	}
	if width > 0 {
		for i := range lines {
			lines[i] = truncateLine(lines[i], width)
		}
	}
	return strings.Join(lines, "\n")
}

func renderCompactMain(m Model) string {
	lines := []string{}
	if id := m.selectedTorrentID(); id != "" {
		t := m.visibleTorrents()[m.selectedIdx]
		lines = append(lines, statusLabelForSearch(t.Status)+" "+t.Filename)
	} else {
		lines = append(lines, "No torrents")
	}
	lines = append(lines, "F6 activity · ? help · ↑↓ move", renderActivity(m, m.width), renderFeedback(m, m.width))
	return boundView(strings.Join(lines, "\n"), m.width, m.height)
}
