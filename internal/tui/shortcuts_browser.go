package tui

import "github.com/m4rii0/rdtui/internal/app"

func browserShortcutDefs(m Model) []shortcutDef {
	if m.browser.EditingPath {
		return nil
	}
	defs := []shortcutDef{
		shortcut(actionMoveUp, shortcutGroupNavigation, 0, []string{"up", "k"}, "↑/k", "up").hiddenFromHelp().withFooter(false),
		shortcut(actionMoveDown, shortcutGroupNavigation, 1, []string{"down", "j"}, "↓/j", "down").hiddenFromHelp().withFooter(false),
		helpOnlyShortcut(shortcutGroupNavigation, 2, "↑↓ j/k", "move"),
		shortcut(actionBrowserOpenOrToggle, shortcutGroupNavigation, 10, []string{"enter", "right", "l"}, "enter/l", "open"),
		shortcut(actionBrowserToggle, shortcutGroupSelection, 11, []string{"space"}, "space", "toggle"),
		shortcut(actionBrowserEditPath, shortcutGroupNavigation, 12, []string{"/"}, "/", "edit path"),
		shortcut(actionBrowserToggleVisual, shortcutGroupSelection, 13, []string{"V"}, "V", "visual"),
		shortcut(actionBrowserTop, shortcutGroupNavigation, 14, []string{"g"}, "g", "top"),
		shortcut(actionBrowserBottom, shortcutGroupNavigation, 15, []string{"G"}, "G", "bottom"),
		shortcut(actionBrowserPageUp, shortcutGroupNavigation, 16, []string{"pgup", "K"}, "pgup/K", "page up"),
		shortcut(actionBrowserPageDown, shortcutGroupNavigation, 17, []string{"pgdown", "J"}, "pgdn/J", "page down"),
		shortcut(actionBrowserToggleAll, shortcutGroupSelection, 18, []string{"ctrl+a"}, "ctrl+a", "all"),
		shortcut(actionBrowserClear, shortcutGroupSelection, 19, []string{"ctrl+d"}, "ctrl+d", "clear"),
		shortcut(actionBrowserToggleHidden, shortcutGroupActions, 20, []string{"H"}, "H", "hidden"),
		shortcut(actionBrowserImport, shortcutGroupActions, 21, []string{"ctrl+s"}, "ctrl+s", "import").whenEnabled(func(m Model) bool { return len(browserImportablePaths(m.browser)) > 0 }),
		shortcut(actionBack, shortcutGroupNavigation, 22, []string{"esc"}, "esc", "cancel"),
		shortcut(actionBrowserUpDir, shortcutGroupNavigation, 23, []string{"backspace", "left", "h"}, "backspace/h", "up dir").withFooter(false),
	}
	if m.browser.VisualMode {
		defs = append(defs, helpOnlyShortcut(shortcutGroupSelection, 24, "[VISUAL]", "active").withFooter(false).hiddenFromHelp())
	}
	return defs
}

func browserImportablePaths(b fileBrowserState) []string {
	selected := b.selectedPaths()
	valid, _ := app.ValidTorrentFiles(selected)
	return valid
}

func browserEditShortcutDefs() []shortcutDef {
	return []shortcutDef{
		shortcut(actionBrowserEditComplete, shortcutGroupNavigation, 0, []string{"tab"}, "tab", "complete"),
		shortcut(actionMoveUp, shortcutGroupNavigation, 1, []string{"up"}, "↑", "pick"),
		shortcut(actionMoveDown, shortcutGroupNavigation, 2, []string{"down"}, "↓", "pick"),
		shortcut(actionBrowserEditConfirm, shortcutGroupActions, 3, []string{"enter"}, "enter", "navigate/select"),
		shortcut(actionBrowserEditCancel, shortcutGroupNavigation, 4, []string{"esc"}, "esc", "cancel edit"),
	}
}
