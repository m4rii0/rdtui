package tui

import (
	"sort"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
)

type shortcutAction string

const (
	actionToggleHelp shortcutAction = "toggle-help"
	actionCloseHelp  shortcutAction = "close-help"

	actionQuit shortcutAction = "quit"

	actionMoveUp   shortcutAction = "move-up"
	actionMoveDown shortcutAction = "move-down"

	actionOpenDetail shortcutAction = "open-detail"
	actionOpenSearch shortcutAction = "open-search"

	actionSortStatus   shortcutAction = "sort-status"
	actionSortProgress shortcutAction = "sort-progress"
	actionSortSize     shortcutAction = "sort-size"
	actionSortAdded    shortcutAction = "sort-added"
	actionSortName     shortcutAction = "sort-name"

	actionRefresh shortcutAction = "refresh"

	actionOpenMagnetInput shortcutAction = "open-magnet-input"
	actionOpenURLInput    shortcutAction = "open-url-input"
	actionOpenImport      shortcutAction = "open-import"

	actionOpenSelectFiles shortcutAction = "open-select-files"
	actionCopyURL         shortcutAction = "copy-url"
	actionStartDownload   shortcutAction = "start-download"
	actionDelete          shortcutAction = "delete"

	actionToggleBatch shortcutAction = "toggle-batch"
	actionExitBatch   shortcutAction = "exit-batch"
	actionBatchMark   shortcutAction = "batch-mark"
	actionBatchAll    shortcutAction = "batch-all"
	actionBatchClear  shortcutAction = "batch-clear"

	actionSearchConfirm shortcutAction = "search-confirm"
	actionSearchCancel  shortcutAction = "search-cancel"

	actionBack shortcutAction = "back"

	actionBrowserOpenOrToggle shortcutAction = "browser-open-or-toggle"
	actionBrowserToggle       shortcutAction = "browser-toggle"
	actionBrowserEditPath     shortcutAction = "browser-edit-path"
	actionBrowserToggleVisual shortcutAction = "browser-toggle-visual"
	actionBrowserToggleAll    shortcutAction = "browser-toggle-all"
	actionBrowserClear        shortcutAction = "browser-clear"
	actionBrowserToggleHidden shortcutAction = "browser-toggle-hidden"
	actionBrowserUpDir        shortcutAction = "browser-up-dir"
	actionBrowserPageUp       shortcutAction = "browser-page-up"
	actionBrowserPageDown     shortcutAction = "browser-page-down"
	actionBrowserTop          shortcutAction = "browser-top"
	actionBrowserBottom       shortcutAction = "browser-bottom"
	actionBrowserImport       shortcutAction = "browser-import"
	actionBrowserEditConfirm  shortcutAction = "browser-edit-confirm"
	actionBrowserEditComplete shortcutAction = "browser-edit-complete"
	actionBrowserEditCancel   shortcutAction = "browser-edit-cancel"

	actionPopupCancel shortcutAction = "popup-cancel"
	actionPopupConfirm shortcutAction = "popup-confirm"
	actionToggleSelection shortcutAction = "toggle-selection"
	actionSelectAll shortcutAction = "select-all"
	actionClearSelection shortcutAction = "clear-selection"

	actionOpenFile   shortcutAction = "open-file"
	actionRevealFile shortcutAction = "reveal-file"
)

type shortcutGroup string

const (
	shortcutGroupNavigation shortcutGroup = "Navigation"
	shortcutGroupSelection  shortcutGroup = "Selection"
	shortcutGroupSort       shortcutGroup = "Sort"
	shortcutGroupActions    shortcutGroup = "Actions"
	shortcutGroupGlobal     shortcutGroup = "Global"
)

type shortcutDef struct {
	action     shortcutAction
	binding    key.Binding
	group      shortcutGroup
	order      int
	help       bool
	footer     bool
	when       func(Model) bool
	enabled    func(Model) bool
	footerWhen func(Model) bool
}

func shortcut(action shortcutAction, group shortcutGroup, order int, keys []string, helpKey, helpDesc string) shortcutDef {
	return shortcutDef{
		action:  action,
		binding: key.NewBinding(key.WithKeys(keys...), key.WithHelp(helpKey, helpDesc)),
		group:   group,
		order:   order,
		help:    helpDesc != "",
		footer:  true,
	}
}

func helpOnlyShortcut(group shortcutGroup, order int, helpKey, helpDesc string) shortcutDef {
	return shortcutDef{
		binding: key.NewBinding(key.WithHelp(helpKey, helpDesc)),
		group:   group,
		order:   order,
		help:    helpDesc != "",
		footer:  true,
	}
}

func (s shortcutDef) whenVisible(fn func(Model) bool) shortcutDef {
	s.when = fn
	return s
}

func (s shortcutDef) whenEnabled(fn func(Model) bool) shortcutDef {
	s.enabled = fn
	return s
}

func (s shortcutDef) withFooter(v bool) shortcutDef {
	s.footer = v
	return s
}

func (s shortcutDef) withFooterWhen(fn func(Model) bool) shortcutDef {
	s.footerWhen = fn
	return s
}

func (s shortcutDef) hiddenFromHelp() shortcutDef {
	s.help = false
	return s
}

func (s shortcutDef) isVisible(m Model) bool {
	return s.when == nil || s.when(m)
}

func (s shortcutDef) isEnabled(m Model) bool {
	return s.enabled == nil || s.enabled(m)
}

func (s shortcutDef) isFooterVisible(m Model) bool {
	if !s.footer {
		return false
	}
	return s.footerWhen == nil || s.footerWhen(m)
}

func (s shortcutDef) isHelpVisible() bool {
	if !s.help {
		return false
	}
	help := s.binding.Help()
	return help.Key != "" || help.Desc != ""
}

func (m Model) activeShortcutDefs() []shortcutDef {
	if m.helpVisible {
		return helpOverlayShortcuts()
	}
	return m.collectShortcutDefs(true)
}

func (m Model) renderShortcutDefs() []shortcutDef {
	return m.collectShortcutDefs(true)
}

func (m Model) helpContextShortcutDefs() []shortcutDef {
	return m.collectShortcutDefs(false)
}

func (m Model) collectShortcutDefs(includeHelpToggle bool) []shortcutDef {
	defs := make([]shortcutDef, 0, 24)
	defs = append(defs, overlayShortcutDefs(m)...)
	defs = append(defs, subcontextShortcutDefs(m)...)
	defs = append(defs, surfaceShortcutDefs(m)...)
	defs = append(defs, globalShortcutDefs(m, includeHelpToggle)...)
	return visibleShortcutDefs(m, defs)
}

func visibleShortcutDefs(m Model, defs []shortcutDef) []shortcutDef {
	out := make([]shortcutDef, 0, len(defs))
	for _, def := range defs {
		if def.isVisible(m) {
			out = append(out, def)
		}
	}
	return out
}

func sortShortcutDefs(defs []shortcutDef) []shortcutDef {
	out := append([]shortcutDef(nil), defs...)
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].order < out[j].order
	})
	return out
}

func shortcutHints(defs []shortcutDef, filter func(shortcutDef) bool, m Model) []shortcutHint {
	sorted := sortShortcutDefs(defs)
	hints := make([]shortcutHint, 0, len(sorted))
	seen := make(map[string]bool, len(sorted))
	for _, def := range sorted {
		if filter != nil && !filter(def) {
			continue
		}
		if !def.isHelpVisible() {
			continue
		}
		help := def.binding.Help()
		id := help.Key + "\x00" + help.Desc
		if seen[id] {
			continue
		}
		seen[id] = true
		hints = append(hints, shortcutHint{Key: help.Key, Desc: help.Desc, Enabled: def.isEnabled(m)})
	}
	return hints
}

func renderShortcutFooter(defs []shortcutDef, m Model) string {
	return renderFooterShortcuts(shortcutHints(defs, func(def shortcutDef) bool {
		return def.isFooterVisible(m)
	}, m)...)
}

func (m Model) matchShortcut(msg tea.KeyPressMsg) (shortcutAction, bool) {
	for _, def := range m.activeShortcutDefs() {
		if key.Matches(msg, def.binding) {
			return def.action, true
		}
	}
	return "", false
}

func globalShortcutDefs(m Model, includeHelpToggle bool) []shortcutDef {
	defs := []shortcutDef{}
	if includeHelpToggle && m.canShowHelp() {
		defs = append(defs, shortcut(actionToggleHelp, shortcutGroupGlobal, 100, []string{"?"}, "?", "help"))
	}
	return defs
}

func helpOverlayShortcuts() []shortcutDef {
	return []shortcutDef{
		shortcut(actionCloseHelp, shortcutGroupGlobal, 0, []string{"?", "esc"}, "?/esc", "close").withFooter(false),
	}
}
