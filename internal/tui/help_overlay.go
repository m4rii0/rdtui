package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
)

var helpGroupOrder = []shortcutGroup{
	shortcutGroupNavigation,
	shortcutGroupSelection,
	shortcutGroupSort,
	shortcutGroupActions,
	shortcutGroupGlobal,
}

func renderHelpOverlay(m Model) string {
	popupW, _ := popupSize(m.width, m.height)
	innerW := popupW - 4
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
	lines = append(lines, "", popupFooter(shortcutHint{Key: "?/esc", Desc: "close"}))
	return popupBox(helpOverlayTitle(m), strings.Join(lines, "\n"), innerW, false)
}

func helpOverlayTitle(m Model) string {
	switch m.mode {
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
