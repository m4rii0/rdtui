package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/m4rii0/rdtui/internal/debug"
)

type browserEntry struct {
	Name     string
	Path     string
	IsDir    bool
	FileSize int64
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
	state := fileBrowserState{CurrentDir: abs, Selected: map[string]struct{}{}}
	state.reload()
	return state
}

func (b *fileBrowserState) reload() {
	entries, err := os.ReadDir(b.CurrentDir)
	if err != nil {
		b.Err = err.Error()
		debug.Log("reload: error reading dir %s: %s", b.CurrentDir, b.Err)
		return
	}
	b.Err = ""
	b.Entries = b.Entries[:0]
	parent := filepath.Dir(b.CurrentDir)
	if parent != b.CurrentDir {
		b.Entries = append(b.Entries, browserEntry{Name: "..", Path: parent, IsDir: true})
	}
	for _, entry := range entries {
		name := entry.Name()
		if !b.ShowHidden && strings.HasPrefix(name, ".") {
			continue
		}
		isTorrent := strings.EqualFold(filepath.Ext(name), ".torrent")
		if entry.IsDir() || isTorrent {
			var size int64
			if isTorrent {
				if info, err := entry.Info(); err == nil {
					size = info.Size()
				}
			}
			b.Entries = append(b.Entries, browserEntry{
				Name:     name,
				Path:     filepath.Join(b.CurrentDir, name),
				IsDir:    entry.IsDir(),
				FileSize: size,
			})
		}
	}
	sort.SliceStable(b.Entries, func(i, j int) bool {
		if b.Entries[i].Name == ".." {
			return true
		}
		if b.Entries[j].Name == ".." {
			return false
		}
		if b.Entries[i].IsDir != b.Entries[j].IsDir {
			return b.Entries[i].IsDir
		}
		return strings.ToLower(b.Entries[i].Name) < strings.ToLower(b.Entries[j].Name)
	})
	if b.Cursor >= len(b.Entries) {
		b.Cursor = max(0, len(b.Entries)-1)
	}
	if b.Cursor < 0 {
		b.Cursor = 0
	}
	debug.Log("reload: dir=%s entries=%d showHidden=%v", b.CurrentDir, len(b.Entries), b.ShowHidden)
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

func (b *fileBrowserState) current() *browserEntry {
	if len(b.Entries) == 0 || b.Cursor < 0 || b.Cursor >= len(b.Entries) {
		return nil
	}
	return &b.Entries[b.Cursor]
}

func (b *fileBrowserState) openCurrent() {
	entry := b.current()
	if entry == nil {
		return
	}
	if entry.IsDir {
		b.CurrentDir = entry.Path
		b.VisualMode = false
		b.reload()
		return
	}
	b.toggleCurrent()
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

func (b fileBrowserState) view(width, height int) string {
	debug.Log("browser.view: width=%d height=%d entries=%d cursor=%d selected=%d err=%q",
		width, height, len(b.Entries), b.Cursor, len(b.Selected), b.Err)
	maxContentHeight := height - 4
	if maxContentHeight < 1 {
		maxContentHeight = 1
	}

	var lines []string

	if b.Err != "" {
		lines = append(lines, "", "Error: "+b.Err, "")
	} else {
		lines = append(lines, "")
		start, end := 0, len(b.Entries)
		if len(b.Entries) > maxContentHeight {
			start = b.Cursor - maxContentHeight/2
			if start < 0 {
				start = 0
			}
			end = start + maxContentHeight
			if end > len(b.Entries) {
				end = len(b.Entries)
				start = end - maxContentHeight
				if start < 0 {
					start = 0
				}
			}
		}
		for idx := start; idx < end; idx++ {
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
			lines = append(lines, cursor+marker+name)
		}
	}

	footer := "enter=open/toggle  space=toggle  V=visual  ctrl+a=all  ctrl+d=clear  H=hidden  ctrl+s=import  esc=cancel"
	if b.VisualMode {
		footer = "[VISUAL] " + footer
	}
	footer += fmt.Sprintf("  │  Selected: %d", len(b.Selected))

	lines = append(lines, "", footer)

	return strings.Join(lines, "\n")
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
