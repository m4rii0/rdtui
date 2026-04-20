package tui

func overlayShortcutDefs(m Model) []shortcutDef {
	switch m.mode {
	case modeSelectFiles:
		return selectFilesShortcutDefs()
	case modeDelete:
		return deletePopupShortcutDefs()
	case modeChooseTarget:
		return targetPickerShortcutDefs()
	case modeOverwrite:
		return overwriteShortcutDefs()
	case modeShowURL:
		return showURLShortcutDefs()
	default:
		return nil
	}
}

func selectFilesShortcutDefs() []shortcutDef {
	return []shortcutDef{
		shortcut(actionMoveUp, shortcutGroupNavigation, 0, []string{"up", "k"}, "↑/k", "up").hiddenFromHelp().withFooter(false),
		shortcut(actionMoveDown, shortcutGroupNavigation, 1, []string{"down", "j"}, "↓/j", "down").hiddenFromHelp().withFooter(false),
		helpOnlyShortcut(shortcutGroupNavigation, 2, "↑↓ j/k", "move"),
		shortcut(actionToggleSelection, shortcutGroupSelection, 10, []string{"space"}, "space", "toggle"),
		shortcut(actionSelectAll, shortcutGroupSelection, 11, []string{"ctrl+a"}, "ctrl+a", "all"),
		shortcut(actionClearSelection, shortcutGroupSelection, 12, []string{"ctrl+d"}, "ctrl+d", "clear"),
		shortcut(actionPopupConfirm, shortcutGroupActions, 20, []string{"enter"}, "enter", "confirm"),
		shortcut(actionPopupCancel, shortcutGroupNavigation, 21, []string{"esc"}, "esc", "cancel"),
	}
}

func targetPickerShortcutDefs() []shortcutDef {
	return []shortcutDef{
		shortcut(actionMoveUp, shortcutGroupNavigation, 0, []string{"up", "k"}, "↑/k", "up").hiddenFromHelp().withFooter(false),
		shortcut(actionMoveDown, shortcutGroupNavigation, 1, []string{"down", "j"}, "↓/j", "down").hiddenFromHelp().withFooter(false),
		helpOnlyShortcut(shortcutGroupNavigation, 2, "↑↓ j/k", "navigate"),
		shortcut(actionPopupConfirm, shortcutGroupActions, 10, []string{"enter"}, "enter", "confirm"),
		shortcut(actionPopupCancel, shortcutGroupNavigation, 11, []string{"esc"}, "esc", "cancel"),
	}
}

func deletePopupShortcutDefs() []shortcutDef {
	return []shortcutDef{
		shortcut(actionPopupConfirm, shortcutGroupActions, 0, []string{"y", "enter"}, "y/enter", "delete"),
		shortcut(actionPopupCancel, shortcutGroupNavigation, 1, []string{"n", "esc"}, "n/esc", "cancel"),
	}
}

func overwriteShortcutDefs() []shortcutDef {
	return []shortcutDef{
		shortcut(actionPopupConfirm, shortcutGroupActions, 0, []string{"y", "enter"}, "y/enter", "download again"),
		shortcut(actionPopupCancel, shortcutGroupNavigation, 1, []string{"n", "esc"}, "n/esc", "cancel"),
	}
}

func showURLShortcutDefs() []shortcutDef {
	return []shortcutDef{
		shortcut(actionPopupCancel, shortcutGroupNavigation, 0, []string{"enter", "esc"}, "enter/esc", "close"),
	}
}
