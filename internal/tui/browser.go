package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/m4rii0/rdtui/internal/debug"
)

type browserEntry struct {
	Name     string
	Path     string
	IsDir    bool
	FileSize int64
}

type browserReadDirMsg struct {
	dir     string
	entries []browserEntry
	err     error
}

type navFrame struct {
	Cursor int
}

type fileBrowserState struct {
	CurrentDir   string
	Entries      []browserEntry
	Cursor       int
	Selected     map[string]struct{}
	Err          string
	ShowHidden   bool
	VisualMode   bool
	VisualAnchor int
	navStack     []navFrame

	pathInput      textinput.Model
	EditingPath    bool
	editCompletions []browserEntry
	editCursor     int
}

func newFileBrowser(startDir string) fileBrowserState {
	if startDir == "" {
		startDir = "."
	}
	abs, err := filepath.Abs(startDir)
	if err != nil {
		abs = startDir
	}
	debug.Log("newFileBrowser: startDir=%s abs=%s", startDir, abs)
	pi := textinput.New()
	pi.Prompt = ""
	pi.Placeholder = "type path..."
	pi.SetWidth(60)
	return fileBrowserState{CurrentDir: abs, Selected: map[string]struct{}{}, pathInput: pi}
}

func (b *fileBrowserState) readDirCmd() tea.Cmd {
	dir := b.CurrentDir
	showHidden := b.ShowHidden
	return func() tea.Msg {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return browserReadDirMsg{dir: dir, err: err}
		}
		var result []browserEntry
		parent := filepath.Dir(dir)
		if parent != dir {
			result = append(result, browserEntry{Name: "..", Path: parent, IsDir: true})
		}
		for _, entry := range entries {
			name := entry.Name()
			if !showHidden && strings.HasPrefix(name, ".") {
				continue
			}
			info, err := entry.Info()
			if err != nil {
				continue
			}
			isDir := entry.IsDir()
			isTorrent := strings.EqualFold(filepath.Ext(name), ".torrent")
			if info.Mode()&os.ModeSymlink != 0 {
				symPath := filepath.Join(dir, name)
				resolved, err := filepath.EvalSymlinks(symPath)
				if err == nil {
					resolvedInfo, err := os.Stat(resolved)
					if err == nil && resolvedInfo.IsDir() {
						isDir = true
					}
				}
			}
			if isDir || isTorrent {
				var size int64
				if isTorrent {
					size = info.Size()
				}
				result = append(result, browserEntry{
					Name:     name,
					Path:     filepath.Join(dir, name),
					IsDir:    isDir,
					FileSize: size,
				})
			}
		}
		sort.SliceStable(result, func(i, j int) bool {
			if result[i].Name == ".." {
				return true
			}
			if result[j].Name == ".." {
				return false
			}
			if result[i].IsDir != result[j].IsDir {
				return result[i].IsDir
			}
			return strings.ToLower(result[i].Name) < strings.ToLower(result[j].Name)
		})
		return browserReadDirMsg{dir: dir, entries: result}
	}
}

func (b *fileBrowserState) handleReadDir(msg browserReadDirMsg) {
	if msg.dir != b.CurrentDir {
		return
	}
	if msg.err != nil {
		b.Err = msg.err.Error()
		debug.Log("handleReadDir: error reading dir %s: %s", b.CurrentDir, b.Err)
		return
	}
	b.Err = ""
	b.Entries = msg.entries
	if b.Cursor >= len(b.Entries) {
		b.Cursor = max(0, len(b.Entries)-1)
	}
	if b.Cursor < 0 {
		b.Cursor = 0
	}
	debug.Log("handleReadDir: dir=%s entries=%d showHidden=%v", b.CurrentDir, len(b.Entries), b.ShowHidden)
}

func (b *fileBrowserState) move(delta int) {
	if len(b.Entries) == 0 {
		return
	}
	b.Cursor += delta
	if b.Cursor < 0 {
		b.Cursor = 0
	}
	if b.Cursor >= len(b.Entries) {
		b.Cursor = len(b.Entries) - 1
	}
	if b.VisualMode {
		b.updateVisualSelection()
	}
}

func (b *fileBrowserState) pageUp(pageSize int) {
	if len(b.Entries) == 0 {
		return
	}
	b.Cursor -= pageSize
	if b.Cursor < 0 {
		b.Cursor = 0
	}
	if b.VisualMode {
		b.updateVisualSelection()
	}
}

func (b *fileBrowserState) pageDown(pageSize int) {
	if len(b.Entries) == 0 {
		return
	}
	b.Cursor += pageSize
	if b.Cursor >= len(b.Entries) {
		b.Cursor = len(b.Entries) - 1
	}
	if b.VisualMode {
		b.updateVisualSelection()
	}
}

func (b *fileBrowserState) jumpTop() {
	b.Cursor = 0
	if b.VisualMode {
		b.updateVisualSelection()
	}
}

func (b *fileBrowserState) jumpBottom() {
	if len(b.Entries) == 0 {
		return
	}
	b.Cursor = len(b.Entries) - 1
	if b.VisualMode {
		b.updateVisualSelection()
	}
}

func (b *fileBrowserState) pushNav() {
	b.navStack = append(b.navStack, navFrame{Cursor: b.Cursor})
}

func (b *fileBrowserState) popNav() {
	if len(b.navStack) == 0 {
		b.Cursor = 0
		return
	}
	frame := b.navStack[len(b.navStack)-1]
	b.navStack = b.navStack[:len(b.navStack)-1]
	b.Cursor = frame.Cursor
}

func (b *fileBrowserState) current() *browserEntry {
	if len(b.Entries) == 0 || b.Cursor < 0 || b.Cursor >= len(b.Entries) {
		return nil
	}
	return &b.Entries[b.Cursor]
}

func (b *fileBrowserState) openCurrent() tea.Cmd {
	entry := b.current()
	if entry == nil {
		return nil
	}
	if entry.IsDir {
		b.pushNav()
		b.CurrentDir = entry.Path
		b.VisualMode = false
		return b.readDirCmd()
	}
	b.toggleCurrent()
	return nil
}

func (b *fileBrowserState) goUp() tea.Cmd {
	parent := filepath.Dir(b.CurrentDir)
	if parent == b.CurrentDir {
		return nil
	}
	b.CurrentDir = parent
	b.VisualMode = false
	b.popNav()
	return b.readDirCmd()
}

func (b *fileBrowserState) toggleCurrent() {
	entry := b.current()
	if entry == nil || entry.IsDir {
		return
	}
	if _, ok := b.Selected[entry.Path]; ok {
		delete(b.Selected, entry.Path)
		return
	}
	b.Selected[entry.Path] = struct{}{}
}

func (b *fileBrowserState) toggleAll() {
	torrentCount := 0
	for _, entry := range b.Entries {
		if !entry.IsDir {
			torrentCount++
		}
	}
	allSelected := torrentCount > 0 && len(b.Selected) >= torrentCount
	for _, entry := range b.Entries {
		if entry.IsDir {
			continue
		}
		if allSelected {
			delete(b.Selected, entry.Path)
		} else {
			b.Selected[entry.Path] = struct{}{}
		}
	}
}

func (b *fileBrowserState) clearSelection() {
	for k := range b.Selected {
		delete(b.Selected, k)
	}
}

func (b *fileBrowserState) toggleVisual() {
	if b.VisualMode {
		b.VisualMode = false
		return
	}
	b.VisualMode = true
	b.VisualAnchor = b.Cursor
	b.updateVisualSelection()
}

func (b *fileBrowserState) updateVisualSelection() {
	if !b.VisualMode {
		return
	}
	lo, hi := b.VisualAnchor, b.Cursor
	if lo > hi {
		lo, hi = hi, lo
	}
	for i := lo; i <= hi; i++ {
		if i < 0 || i >= len(b.Entries) {
			continue
		}
		if b.Entries[i].IsDir {
			continue
		}
		b.Selected[b.Entries[i].Path] = struct{}{}
	}
}

func (b *fileBrowserState) selectedPaths() []string {
	out := make([]string, 0, len(b.Selected))
	for path := range b.Selected {
		out = append(out, path)
	}
	sort.Strings(out)
	return out
}

func (b *fileBrowserState) startEditing() {
	b.pathInput.SetValue(b.CurrentDir + string(os.PathSeparator))
	b.EditingPath = true
	b.editCursor = 0
	b.updateCompletions()
	b.pathInput.Focus()
}

func (b *fileBrowserState) stopEditing() {
	b.EditingPath = false
	b.pathInput.Blur()
	b.editCompletions = nil
	b.editCursor = 0
}

func (b *fileBrowserState) updateCompletions() {
	raw := b.pathInput.Value()
	dirPart, prefix := splitPathInput(raw)
	if dirPart == "" {
		dirPart = "."
	}
	abs, err := filepath.Abs(dirPart)
	if err != nil {
		b.editCompletions = nil
		return
	}
	entries, err := os.ReadDir(abs)
	if err != nil {
		b.editCompletions = nil
		return
	}
	b.editCompletions = nil
	for _, entry := range entries {
		name := entry.Name()
		if !b.ShowHidden && strings.HasPrefix(name, ".") {
			continue
		}
		if prefix != "" && !strings.HasPrefix(strings.ToLower(name), strings.ToLower(prefix)) {
			continue
		}
		isDir := entry.IsDir()
		isTorrent := strings.EqualFold(filepath.Ext(name), ".torrent")
		if !isDir && !isTorrent {
			continue
		}
		var size int64
		if isTorrent {
			if info, err := entry.Info(); err == nil {
				size = info.Size()
			}
		}
		b.editCompletions = append(b.editCompletions, browserEntry{
			Name:     name,
			Path:     filepath.Join(abs, name),
			IsDir:    isDir,
			FileSize: size,
		})
	}
	sort.SliceStable(b.editCompletions, func(i, j int) bool {
		if b.editCompletions[i].IsDir != b.editCompletions[j].IsDir {
			return b.editCompletions[i].IsDir
		}
		return strings.ToLower(b.editCompletions[i].Name) < strings.ToLower(b.editCompletions[j].Name)
	})
	if b.editCursor >= len(b.editCompletions) {
		b.editCursor = max(0, len(b.editCompletions)-1)
	}
}

func (b *fileBrowserState) tabComplete() {
	if len(b.editCompletions) == 0 {
		return
	}
	raw := b.pathInput.Value()
	dirPart, _ := splitPathInput(raw)
	if dirPart == "" {
		dirPart = "."
	}

	highlighted := b.editCompletions[b.editCursor]
	suffix := highlighted.Name
	if highlighted.IsDir {
		suffix += string(os.PathSeparator)
	}
	newVal := dirPart + string(os.PathSeparator) + suffix
	b.pathInput.SetValue(newVal)
	b.pathInput.CursorEnd()

	b.editCursor++
	if b.editCursor >= len(b.editCompletions) {
		b.editCursor = 0
	}
	b.updateCompletions()
}

func (b *fileBrowserState) confirmPath() (tea.Cmd, bool, string, string) {
	if len(b.editCompletions) > 0 && b.editCursor >= 0 && b.editCursor < len(b.editCompletions) {
		highlighted := b.editCompletions[b.editCursor]
		typedVal := strings.TrimSpace(b.pathInput.Value())
		typedAbs, _ := filepath.Abs(typedVal)
		if typedVal == "" || typedAbs == highlighted.Path || strings.HasPrefix(highlighted.Path, typedAbs+string(os.PathSeparator)) {
			if highlighted.IsDir {
				b.pushNav()
				b.CurrentDir = highlighted.Path
				b.VisualMode = false
				b.pathInput.SetValue(highlighted.Path + string(os.PathSeparator))
				b.pathInput.CursorEnd()
				b.updateCompletions()
				b.editCursor = 0
				return b.readDirCmd(), true, "", ""
			}
			if strings.EqualFold(filepath.Ext(highlighted.Path), ".torrent") {
				b.Selected[highlighted.Path] = struct{}{}
				parentDir := filepath.Dir(highlighted.Path)
				b.CurrentDir = parentDir
				b.stopEditing()
				return b.readDirCmd(), false, highlighted.Path, ""
			}
		}
	}

	raw := strings.TrimSpace(b.pathInput.Value())
	if raw == "" {
		return nil, false, "", "path cannot be empty"
	}
	abs, err := filepath.Abs(raw)
	if err != nil {
		return nil, false, "", err.Error()
	}
	info, err := os.Stat(abs)
	if err != nil {
		return nil, false, "", "path does not exist: " + abs
	}
	if info.IsDir() {
		b.pushNav()
		b.CurrentDir = abs
		b.VisualMode = false
		b.pathInput.SetValue(abs + string(os.PathSeparator))
		b.pathInput.CursorEnd()
		b.updateCompletions()
		b.editCursor = 0
		return b.readDirCmd(), true, "", ""
	}
	if strings.EqualFold(filepath.Ext(abs), ".torrent") {
		b.Selected[abs] = struct{}{}
		parentDir := filepath.Dir(abs)
		b.CurrentDir = parentDir
		b.stopEditing()
		return b.readDirCmd(), false, abs, ""
	}
	return nil, false, "", "not a .torrent file or directory"
}

func (b *fileBrowserState) moveEditCursor(delta int) {
	if len(b.editCompletions) == 0 {
		return
	}
	b.editCursor += delta
	if b.editCursor < 0 {
		b.editCursor = 0
	}
	if b.editCursor >= len(b.editCompletions) {
		b.editCursor = len(b.editCompletions) - 1
	}
}

func splitPathInput(raw string) (dir, prefix string) {
	idx := strings.LastIndex(raw, string(os.PathSeparator))
	if idx < 0 {
		return ".", raw
	}
	return raw[:idx], raw[idx+1:]
}



func (b fileBrowserState) view(width, height int) string {
	debug.Log("browser.view: width=%d height=%d entries=%d cursor=%d selected=%d err=%q editing=%v",
		width, height, len(b.Entries), b.Cursor, len(b.Selected), b.Err, b.EditingPath)

	if b.EditingPath {
		return b.viewEditing(width, height)
	}

	listH := height - 5
	if listH < 1 {
		listH = 1
	}

	var lines []string

	if b.Err != "" {
		lines = append(lines, "")
		lines = append(lines, padLines(1, "Error: "+b.Err)...)
		lines = append(lines, padLines(listH-1, "")...)
	} else {
		lines = append(lines, "")
		start, end := 0, len(b.Entries)
		if len(b.Entries) > listH {
			start = b.Cursor - listH/2
			if start < 0 {
				start = 0
			}
			end = start + listH
			if end > len(b.Entries) {
				end = len(b.Entries)
				start = end - listH
				if start < 0 {
					start = 0
				}
			}
		}
		thumbTop, thumbSize := scrollbarThumb(len(b.Entries), listH, start)
		for i := 0; i < listH; i++ {
			idx := start + i
			if idx >= 0 && idx < end {
				entry := b.Entries[idx]
				cursor := "  "
				if idx == b.Cursor {
					cursor = "> "
				}
				marker := "[ ] "
				name := entry.Name
				if entry.IsDir {
					marker = "    "
					name += "/"
				} else {
					if _, ok := b.Selected[entry.Path]; ok {
						marker = "[x] "
					}
					if entry.FileSize > 0 {
						name += "  " + humanBrowserBytes(entry.FileSize)
					}
				}
				row := cursor + marker + name
				if len(b.Entries) > listH {
					row = truncateBrowserLine(row, width-2) + " " + mutedStyle.Render(scrollbarGlyph(i, thumbTop, thumbSize))
				}
				lines = append(lines, row)
			} else {
				if len(b.Entries) > listH {
					lines = append(lines, mutedStyle.Render(scrollbarGlyph(i, thumbTop, thumbSize)))
				} else {
					lines = append(lines, "")
				}
			}
		}
	}

	browserModel := Model{mode: modeFileBrowser, browser: b}
	footer := renderShortcutFooter(browserModel.renderShortcutDefs(), browserModel)
	if b.VisualMode {
		footer = headStyle.Render("[VISUAL]") + " " + footer
	}
	footer += footerSepStyle.Render("  ──  ") + selectedCountStyle.Render(fmt.Sprintf("selected: %d", len(b.Selected)))

	lines = append(lines, "", footer)

	return strings.Join(lines, "\n")
}

func (b fileBrowserState) viewEditing(width, height int) string {
	listH := height - 5
	if listH < 1 {
		listH = 1
	}

	inputW := width - 4
	if inputW < 10 {
		inputW = 10
	}
	b.pathInput.SetWidth(inputW)

	var lines []string
	lines = append(lines, b.pathInput.View())

	entries := b.editCompletions
	if len(entries) == 0 {
		lines = append(lines, "  (no matches)")
		for i := 1; i < listH; i++ {
			lines = append(lines, "")
		}
	} else {
		start, end := 0, len(entries)
		if len(entries) > listH {
			start = b.editCursor - listH/2
			if start < 0 {
				start = 0
			}
			end = start + listH
			if end > len(entries) {
				end = len(entries)
				start = end - listH
				if start < 0 {
					start = 0
				}
			}
		}
		thumbTop, thumbSize := scrollbarThumb(len(entries), listH, start)
		for i := 0; i < listH; i++ {
			idx := start + i
			if idx >= 0 && idx < end {
				entry := entries[idx]
				cursor := "  "
				if idx == b.editCursor {
					cursor = "> "
				}
				name := entry.Name
				if entry.IsDir {
					name += "/"
				} else if entry.FileSize > 0 {
					name += "  " + humanBrowserBytes(entry.FileSize)
				}
				row := cursor + name
				if len(entries) > listH {
					row = truncateBrowserLine(row, width-2) + " " + mutedStyle.Render(scrollbarGlyph(i, thumbTop, thumbSize))
				}
				lines = append(lines, row)
			} else {
				if len(entries) > listH {
					lines = append(lines, mutedStyle.Render(scrollbarGlyph(i, thumbTop, thumbSize)))
				} else {
					lines = append(lines, "")
				}
			}
		}
	}

	browserModel := Model{mode: modeFileBrowser, browser: b}
	footer := headStyle.Render("[EDITING PATH]") + " " + renderShortcutFooter(browserModel.renderShortcutDefs(), browserModel)
	footer += footerSepStyle.Render("  ──  ") + selectedCountStyle.Render(fmt.Sprintf("selected: %d", len(b.Selected)))
	lines = append(lines, "", footer)

	return strings.Join(lines, "\n")
}

func padLines(n int, s string) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = s
	}
	return out
}

func truncateBrowserLine(s string, maxW int) string {
	if maxW < 1 {
		return ""
	}
	if len(s) <= maxW {
		return s
	}
	if maxW <= 3 {
		return s[:maxW]
	}
	return s[:maxW-3] + "..."
}

func humanBrowserBytes(b int64) string {
	if b <= 0 {
		return "0 B"
	}
	units := []string{"B", "KB", "MB", "GB", "TB"}
	value := float64(b)
	unit := 0
	for value >= 1024 && unit < len(units)-1 {
		value /= 1024
		unit++
	}
	if unit == 0 {
		return fmt.Sprintf("(%d %s)", b, units[unit])
	}
	return fmt.Sprintf("(%.1f %s)", value, units[unit])
}
