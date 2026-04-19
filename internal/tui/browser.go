package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type browserEntry struct {
	Name  string
	Path  string
	IsDir bool
}

type fileBrowserState struct {
	CurrentDir string
	Entries    []browserEntry
	Cursor     int
	Selected   map[string]struct{}
	Err        string
}

func newFileBrowser(startDir string) fileBrowserState {
	if startDir == "" {
		startDir = "."
	}
	abs, err := filepath.Abs(startDir)
	if err != nil {
		abs = startDir
	}
	state := fileBrowserState{CurrentDir: abs, Selected: map[string]struct{}{}}
	state.reload()
	return state
}

func (b *fileBrowserState) reload() {
	entries, err := os.ReadDir(b.CurrentDir)
	if err != nil {
		b.Err = err.Error()
		return
	}
	b.Err = ""
	b.Entries = b.Entries[:0]
	parent := filepath.Dir(b.CurrentDir)
	if parent != b.CurrentDir {
		b.Entries = append(b.Entries, browserEntry{Name: "..", Path: parent, IsDir: true})
	}
	for _, entry := range entries {
		if entry.IsDir() || strings.EqualFold(filepath.Ext(entry.Name()), ".torrent") {
			b.Entries = append(b.Entries, browserEntry{
				Name:  entry.Name(),
				Path:  filepath.Join(b.CurrentDir, entry.Name()),
				IsDir: entry.IsDir(),
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

func (b *fileBrowserState) selectedPaths() []string {
	out := make([]string, 0, len(b.Selected))
	for path := range b.Selected {
		out = append(out, path)
	}
	sort.Strings(out)
	return out
}

func (b fileBrowserState) view(height int) string {
	var lines []string
	lines = append(lines, fmt.Sprintf("Directory: %s", b.CurrentDir))
	lines = append(lines, "Enter=open/toggle  space=toggle  backspace=parent  ctrl+s=import  esc=cancel")
	if b.Err != "" {
		lines = append(lines, "", "Error: "+b.Err)
	}
	lines = append(lines, "")
	for idx, entry := range b.Entries {
		cursor := "  "
		if idx == b.Cursor {
			cursor = "> "
		}
		marker := "  "
		name := entry.Name
		if entry.IsDir {
			name += "/"
		} else if _, ok := b.Selected[entry.Path]; ok {
			marker = "* "
		}
		lines = append(lines, cursor+marker+name)
	}
	lines = append(lines, "", fmt.Sprintf("Selected: %d", len(b.Selected)))
	if height > 0 && len(lines) > height {
		lines = lines[:height]
	}
	return strings.Join(lines, "\n")
}
