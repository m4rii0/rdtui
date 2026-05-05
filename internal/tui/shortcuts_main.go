package tui

func surfaceShortcutDefs(m Model) []shortcutDef {
	switch m.mode {
	case modeMain:
		return mainShortcutDefs(m)
	case modeDetail:
		return detailShortcutDefs(m)
	case modeDownload:
		return downloadShortcutDefs(m)
	case modeBulkDownload:
		return bulkDownloadShortcutDefs(m)
	case modeFileBrowser:
		return browserShortcutDefs(m)
	default:
		return nil
	}
}

func subcontextShortcutDefs(m Model) []shortcutDef {
	defs := []shortcutDef{}
	if m.mode == modeSearch {
		defs = append(defs, searchShortcutDefs()...)
	}
	if m.mode == modeMain && m.batchMode {
		defs = append(defs, batchShortcutDefs(m)...)
	}
	if m.mode == modeFileBrowser && m.browser.EditingPath {
		defs = append(defs, browserEditShortcutDefs()...)
	}
	return defs
}

func mainShortcutDefs(m Model) []shortcutDef {
	defs := []shortcutDef{
		shortcut(actionMoveUp, shortcutGroupNavigation, 10, []string{"up", "k"}, "↑/k", "up").hiddenFromHelp().withFooter(false),
		shortcut(actionMoveDown, shortcutGroupNavigation, 11, []string{"down", "j"}, "↓/j", "down").hiddenFromHelp().withFooter(false),
		helpOnlyShortcut(shortcutGroupNavigation, 12, "↑↓ j/k", "move").withFooterWhen(func(m Model) bool { return m.mode == modeMain && !m.batchMode && !m.filterApplied }),
		shortcut(actionOpenDetail, shortcutGroupNavigation, 20, []string{"enter"}, "enter", "details").whenVisible(func(m Model) bool { return len(m.visibleTorrents()) > 0 }).withFooterWhen(func(m Model) bool { return m.mode == modeMain && !m.batchMode }),
		shortcut(actionOpenSearch, shortcutGroupNavigation, 30, []string{"/"}, "/", "search").withFooterWhen(func(m Model) bool { return m.mode == modeMain && !m.batchMode }),
		shortcut(actionRefresh, shortcutGroupActions, 40, []string{"r"}, "r", "refresh").withFooterWhen(func(m Model) bool { return m.mode == modeMain && !m.batchMode }),
		shortcut(actionOpenMagnetInput, shortcutGroupActions, 50, []string{"m"}, "m", "magnet").withFooterWhen(func(m Model) bool { return m.mode == modeMain && !m.batchMode }),
		shortcut(actionOpenURLInput, shortcutGroupActions, 60, []string{"u"}, "u", "url").withFooterWhen(func(m Model) bool { return m.mode == modeMain && !m.batchMode }),
		shortcut(actionOpenImport, shortcutGroupActions, 70, []string{"i"}, "i", "import").withFooterWhen(func(m Model) bool { return m.mode == modeMain && !m.batchMode }),
		shortcut(actionToggleBatch, shortcutGroupSelection, 80, []string{"b"}, "b", "batch").withFooterWhen(func(m Model) bool { return m.mode == modeMain && !m.batchMode }),
		shortcut(actionOpenSelectFiles, shortcutGroupActions, 90, []string{"s"}, "s", "select").whenVisible(func(m Model) bool { return len(m.visibleTorrents()) > 0 }).whenEnabled(func(m Model) bool { return m.canSelectFilesFromSelection() }).withFooterWhen(func(m Model) bool { return m.mode == modeMain && !m.batchMode }),
		shortcut(actionCopyURL, shortcutGroupActions, 100, []string{"y"}, "y", "copy").whenVisible(func(m Model) bool { return len(m.visibleTorrents()) > 0 }).whenEnabled(func(m Model) bool { return m.canReadyActionsFromSelection() }).withFooterWhen(func(m Model) bool { return m.mode == modeMain && !m.batchMode }),
		shortcut(actionStartDownload, shortcutGroupActions, 110, []string{"d"}, "d", "download").whenVisible(func(m Model) bool { return len(m.visibleTorrents()) > 0 && !m.batchMode }).whenEnabled(func(m Model) bool { return m.canReadyActionsFromSelection() && !m.batchMode }).withFooterWhen(func(m Model) bool { return m.mode == modeMain && !m.batchMode }),
		shortcut(actionDelete, shortcutGroupActions, 120, []string{"x"}, "x", "delete").whenVisible(func(m Model) bool { return len(m.visibleTorrents()) > 0 }).whenEnabled(func(m Model) bool { return m.canDeleteSelectedTorrent() }).withFooterWhen(func(m Model) bool { return m.mode == modeMain && !m.batchMode }),
		shortcut(actionQuit, shortcutGroupGlobal, 130, []string{"q"}, "q", "quit").withFooterWhen(func(m Model) bool { return m.mode == modeMain && !m.batchMode }),

		shortcut(actionSortStatus, shortcutGroupSort, 140, []string{"S"}, "S", "status").hiddenFromHelp().withFooter(false),
		shortcut(actionSortProgress, shortcutGroupSort, 141, []string{"P"}, "P", "progress").hiddenFromHelp().withFooter(false),
		shortcut(actionSortSize, shortcutGroupSort, 142, []string{"Z"}, "Z", "size").hiddenFromHelp().withFooter(false),
		shortcut(actionSortAdded, shortcutGroupSort, 143, []string{"D"}, "D", "date").hiddenFromHelp().withFooter(false),
		shortcut(actionSortName, shortcutGroupSort, 144, []string{"N"}, "N", "name").hiddenFromHelp().withFooter(false),
		helpOnlyShortcut(shortcutGroupSort, 145, "S/P/Z/D/N", "sort").withFooterWhen(func(m Model) bool { return m.mode == modeMain && !m.batchMode }),
	}
	if m.filterApplied {
		for i := range defs {
			if defs[i].action == actionQuit {
				defs[i].binding.SetHelp("q", "clear filter")
			}
		}
	}
	return defs
}

func searchShortcutDefs() []shortcutDef {
	return []shortcutDef{
		shortcut(actionSearchCancel, shortcutGroupNavigation, 0, []string{"esc"}, "esc", "clear"),
		shortcut(actionSearchConfirm, shortcutGroupNavigation, 1, []string{"enter"}, "enter", "keep"),
		shortcut(actionMoveUp, shortcutGroupNavigation, 2, []string{"up"}, "↑", "up"),
		shortcut(actionMoveDown, shortcutGroupNavigation, 3, []string{"down"}, "↓", "down"),
	}
}

func batchShortcutDefs(m Model) []shortcutDef {
	defs := []shortcutDef{
		shortcut(actionBatchMark, shortcutGroupSelection, 0, []string{"space"}, "space", "mark"),
		shortcut(actionBatchAll, shortcutGroupSelection, 1, []string{"ctrl+a"}, "ctrl+a", "all"),
		shortcut(actionBatchClear, shortcutGroupSelection, 2, []string{"ctrl+d"}, "ctrl+d", "clear"),
		shortcut(actionExitBatch, shortcutGroupSelection, 5, []string{"esc"}, "esc", "exit").hiddenFromHelp().withFooter(false),
		shortcut(actionToggleBatch, shortcutGroupSelection, 6, []string{"b"}, "b", "batch").hiddenFromHelp().withFooter(false),
		helpOnlyShortcut(shortcutGroupSelection, 7, "b/esc", "exit"),
	}
	if m.hasBatchSelection() {
		defs = append(defs,
			shortcut(actionDelete, shortcutGroupActions, 3, []string{"x"}, "x", "delete").whenEnabled(func(m Model) bool { return m.hasBatchSelection() }),
			shortcut(actionCopyURL, shortcutGroupActions, 4, []string{"y"}, "y", "copy").whenEnabled(func(m Model) bool { return m.hasBatchSelection() }),
		)
	}
	defs = append(defs,
		shortcut(actionOpenSelectFiles, shortcutGroupActions, 8, []string{"s"}, "s", "select").whenEnabled(func(m Model) bool { return m.canBulkSelectFilesSelection() }),
		shortcut(actionStartDownload, shortcutGroupActions, 9, []string{"d"}, "d", "download").whenEnabled(func(m Model) bool { return m.canBulkDownloadSelection() }),
	)
	return defs
}

func bulkDownloadShortcutDefs(m Model) []shortcutDef {
	defs := []shortcutDef{
		shortcut(actionRefresh, shortcutGroupActions, 10, []string{"r"}, "r", "refresh"),
		shortcut(actionBack, shortcutGroupNavigation, 20, []string{"esc"}, "esc", "back"),
		shortcut(actionDelete, shortcutGroupActions, 30, []string{"x"}, "x", "cleanup").whenEnabled(func(m Model) bool { return m.bulk != nil && m.bulk.isFinished() }),
	}
	return defs
}
