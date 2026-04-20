package tui

func detailShortcutDefs(m Model) []shortcutDef {
	return []shortcutDef{
		shortcut(actionBack, shortcutGroupNavigation, 0, []string{"esc"}, "esc", "back"),
		shortcut(actionOpenImport, shortcutGroupActions, 5, []string{"i"}, "i", "import"),
		shortcut(actionOpenSelectFiles, shortcutGroupActions, 10, []string{"s"}, "s", "select").whenVisible(func(m Model) bool { return m.detail != nil }).whenEnabled(func(m Model) bool { return m.canSelectFilesFromDetail() }),
		shortcut(actionCopyURL, shortcutGroupActions, 20, []string{"y"}, "y", "copy").whenVisible(func(m Model) bool { return m.detail != nil }).whenEnabled(func(m Model) bool { return m.canReadyActionsFromDetail() }),
		shortcut(actionStartDownload, shortcutGroupActions, 30, []string{"d"}, "d", "download").whenVisible(func(m Model) bool { return m.detail != nil }).whenEnabled(func(m Model) bool { return m.canReadyActionsFromDetail() }),
		shortcut(actionDelete, shortcutGroupActions, 40, []string{"x"}, "x", "delete").whenVisible(func(m Model) bool { return m.detail != nil }).whenEnabled(func(m Model) bool { return m.detail != nil }),
		shortcut(actionRefresh, shortcutGroupActions, 50, []string{"r"}, "r", "refresh"),
	}
}

func downloadShortcutDefs(m Model) []shortcutDef {
	defs := []shortcutDef{
		shortcut(actionBack, shortcutGroupNavigation, 0, []string{"esc"}, "esc", "back"),
		shortcut(actionRefresh, shortcutGroupActions, 10, []string{"r"}, "r", "refresh"),
	}
	defs = append(defs,
		shortcut(actionOpenFile, shortcutGroupActions, 20, []string{"o"}, "o", "open").whenVisible(func(m Model) bool { return m.download != nil }).whenEnabled(func(m Model) bool { return m.canOpenManagedDownloadFile() }),
		shortcut(actionRevealFile, shortcutGroupActions, 30, []string{"s"}, "s", "reveal").whenVisible(func(m Model) bool { return m.download != nil }).whenEnabled(func(m Model) bool { return m.canOpenManagedDownloadFile() }),
		shortcut(actionDelete, shortcutGroupActions, 40, []string{"x"}, "x", "delete torrent").whenVisible(func(m Model) bool { return m.download != nil }).whenEnabled(func(m Model) bool { return m.canDeleteManagedDownloadTorrent() }),
	)
	return defs
}
