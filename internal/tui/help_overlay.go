package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

var helpGroupOrder = []shortcutGroup{
	shortcutGroupNavigation,
	shortcutGroupSelection,
	shortcutGroupSort,
	shortcutGroupActions,
	shortcutGroupGlobal,
}

func helpContent(m Model) []string {
	innerW := max(1, helpWidth(m)-4-popupBoxStyle.GetHorizontalFrameSize())
	defs := sortShortcutDefs(m.helpContextShortcutDefs())
	grouped := make(map[shortcutGroup][]shortcutDef)
	for _, def := range defs {
		if !def.isHelpVisible() {
			continue
		}
		grouped[def.group] = append(grouped[def.group], def)
	}

	var lines []string
	for _, group := range helpGroupOrder {
		items := grouped[group]
		if len(items) == 0 {
			continue
		}
		if len(lines) > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, headStyle.Render(string(group)))
		maxKeyW := 0
		for _, item := range items {
			help := item.binding.Help()
			if w := lipgloss.Width(help.Key); w > maxKeyW {
				maxKeyW = w
			}
		}
		for _, item := range items {
			help := item.binding.Help()
			keyText := padVisual(help.Key, maxKeyW, false)
			keyStyle := footerKeyStyle
			descStyle := footerDescStyle
			if !item.isEnabled(m) {
				keyStyle = footerKeyDimStyle
				descStyle = footerDescDimStyle
			}
			line := fmt.Sprintf("  %s  %s", keyStyle.Render(keyText), descStyle.Render(help.Desc))
			lines = append(lines, truncateLine(line, innerW))
		}
	}
	if len(lines) == 0 {
		lines = append(lines, mutedStyle.Render("  No shortcuts available"))
	}
	if m.errText != "" {
		lines = append(lines, "", headStyle.Render("Current error"))
		lines = append(lines, strings.Split(ansi.Hardwrap(m.errText, max(1, innerW), false), "\n")...)
	}
	if m.mode == modeDownload && m.download != nil && m.download.ErrorMessage != "" && m.download.ErrorMessage != m.errText {
		lines = append(lines, "", headStyle.Render("Download error"))
		lines = append(lines, strings.Split(ansi.Hardwrap(m.download.ErrorMessage, innerW, false), "\n")...)
	}
	return lines
}

func helpWidth(m Model) int {
	if m.width <= 0 {
		return 76
	}
	return max(5, min(m.width, m.width*7/10+4))
}
func helpRows(m Model) int {
	if m.height <= 0 {
		return max(1, len(helpContent(m)))
	}
	return max(1, m.height-6)
}
func helpMaxOffset(m Model) int { return max(0, len(helpContent(m))-helpRows(m)) }
func renderHelpOverlay(m Model) string {
	lines := helpContent(m)
	start := min(max(0, m.helpOffset), max(0, len(lines)-helpRows(m)))
	lines = lines[start:min(len(lines), start+helpRows(m))]
	content := strings.Join(lines, "\n") + "\n" + mutedStyle.Render("↑↓ pgup/pgdown scroll · esc close")
	return boundView(popupBox(helpOverlayTitle(m), content, helpWidth(m)-4, false), m.width, m.height)
}

func helpOverlayTitle(m Model) string {
	switch m.mode {
	case modeSort:
		return headStyle.Render("◆ Sort Shortcuts")
	case modeQuitConfirm:
		return headStyle.Render("◆ Quit Shortcuts")
	case modeDetail:
		return headStyle.Render("◆ Detail Shortcuts")
	case modeDownload:
		return headStyle.Render("◆ Download Shortcuts")
	case modeFileBrowser:
		return headStyle.Render("◆ Import Shortcuts")
	case modeSelectFiles:
		return headStyle.Render("◆ Select Files Shortcuts")
	case modeChooseTarget:
		return headStyle.Render("◆ Target Shortcuts")
	default:
		return headStyle.Render("◆ Shortcuts")
	}
}
