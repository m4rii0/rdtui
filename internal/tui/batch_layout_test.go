package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/m4rii0/rdtui/pkg/models"
)

func TestBatchToggleKeepsColumnsFixed(t *testing.T) {
	for _, width := range []int{36, 40, 42, 56, 60, 62, 76, 80, 82, 116} {
		for _, scrollbar := range []bool{false, true} {
			t.Run(fmt.Sprintf("width=%d/scroll=%v", width, scrollbar), func(t *testing.T) {
				m := NewModel(Dependencies{Runtime: newTestRuntime(t)})
				m.mode = modeMain
				m.torrents = []models.Torrent{{ID: "a", Filename: "映像Alpha.mkv", Status: "downloaded"}}
				if scrollbar {
					for i := range 8 {
						m.torrents = append(m.torrents, models.Torrent{ID: fmt.Sprint(i), Filename: "other", Status: "queued"})
					}
				}
				render := func() []string { return strings.Split(ansi.Strip(renderTorrentList(m, width, 6)), "\n") }
				baseline := render()
				if !strings.HasPrefix(baseline[1], "  ") || !strings.HasPrefix(baseline[3], "  ") {
					t.Fatal("normal view does not reserve the selection gutter")
				}
				check := func(marker string) {
					t.Helper()
					lines := render()
					if lines[1] != baseline[1] {
						t.Fatalf("toggle changed columns:\n%q\n%q", baseline[1], lines[1])
					}
					if !strings.HasPrefix(lines[3], marker) {
						t.Fatalf("marker=%q want=%q", lines[3], marker)
					}
					for i := 3; i < min(len(lines), 3+len(m.torrents)); i++ {
						if string([]rune(lines[i])[2:]) != string([]rune(baseline[i])[2:]) {
							t.Fatalf("row %d moved or resized after toggle:\n%q\n%q", i, baseline[i], lines[i])
						}
					}
				}
				for range 2 {
					m, _ = uxRune(m, 'b')
					check("· ")
					m, _ = uxPress(m, tea.KeyPressMsg{Code: ' ', Text: " "})
					check("* ")
					m, _ = uxRune(m, 'b')
					check("  ")
				}
			})
		}
	}
}

func TestBatchModeHasPersistentLabelAndActions(t *testing.T) {
	for _, width := range []int{40, 80, 120} {
		for _, empty := range []bool{false, true} {
			m := NewModel(Dependencies{Runtime: newTestRuntime(t)})
			m.mode, m.width, m.height = modeMain, width, 24
			wantSelected := 0
			if !empty {
				m.torrents = []models.Torrent{{ID: "a", Filename: "file", Status: "downloaded"}, {ID: "b", Filename: "other", Status: "queued"}}
				wantSelected = 1
			}
			m, _ = uxRune(m, 'b')
			if !empty {
				m, _ = uxPress(m, tea.KeyPressMsg{Code: ' ', Text: " "})
			}
			view := ansi.Strip(renderView(m))
			if !strings.Contains(view, fmt.Sprintf("BATCH · %d selected", wantSelected)) {
				t.Fatalf("batch label/count missing at width %d:\n%s", width, view)
			}
			footer := ansi.Strip(activityFooter(m, width-4))
			if !strings.Contains(footer, "space mark") || !strings.Contains(footer, "esc exit") {
				t.Fatalf("batch actions hidden: %s", footer)
			}
			allKey := "ctrl+a all"
			if width == 40 {
				allKey = "^A all"
			}
			if !strings.Contains(footer, allKey) {
				t.Fatalf("select-all hint hidden: %s", footer)
			}
			if lipgloss.Width(view) > width || lipgloss.Height(view) != 24 {
				t.Fatalf("batch indicator changes frame: %dx%d", lipgloss.Width(view), lipgloss.Height(view))
			}
			if !empty {
				m, _ = uxPress(m, tea.KeyPressMsg{Code: 'a', Mod: tea.ModCtrl})
				if !strings.Contains(ansi.Strip(renderView(m)), "BATCH · 2 selected") {
					t.Fatal("select-all did not update the visible count")
				}
			}
			m, _ = uxPress(m, tea.KeyPressMsg{Code: tea.KeyEscape})
			if m.batchMode || strings.Contains(ansi.Strip(renderView(m)), "BATCH") {
				t.Fatal("Esc did not remove batch indicator")
			}
		}
	}
}

func TestBatchColumnHeadersAlignWithRows(t *testing.T) {
	for _, width := range []int{36, 40, 42, 56, 60, 62, 76, 80, 82, 116} {
		for _, scrollbar := range []bool{false, true} {
			for _, selected := range []int{0, 1} {
				t.Run(fmt.Sprintf("width=%d/scroll=%v/selected=%d", width, scrollbar, selected), func(t *testing.T) {
					m := NewModel(Dependencies{Runtime: newTestRuntime(t)})
					m.mode = modeMain
					m.selectedIdx = selected
					m.torrents = []models.Torrent{
						{ID: "a", Filename: "映像Alpha.mkv", Status: "downloaded", Progress: 42.5, Bytes: 1024, Added: time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)},
						{ID: "b", Filename: "other", Status: "queued"},
					}
					if scrollbar {
						for i := range 8 {
							m.torrents = append(m.torrents, models.Torrent{ID: fmt.Sprint(i), Filename: "other", Status: "queued"})
						}
					}
					check := func(state string) {
						t.Helper()
						lines := strings.Split(ansi.Strip(renderTorrentList(m, width, 6)), "\n")
						header, row := lines[1], lines[3]
						cell := func(line, text string) int {
							t.Helper()
							index := strings.Index(line, text)
							if index < 0 {
								t.Fatalf("%s: missing %q in %q", state, text, line)
							}
							return lipgloss.Width(line[:index])
						}
						if cell(header, "Name") != cell(row, "映像Alpha.mkv") {
							t.Fatalf("%s: Name header and row drift:\n%q\n%q", state, header, row)
						}
						if strings.Contains(header, "Size") && cell(header, "Size")+4 != cell(row, "1.0 KB")+6 {
							t.Fatalf("%s: right-aligned Size column drifts:\n%q\n%q", state, header, row)
						}
						for _, line := range lines {
							if lipgloss.Width(line) > width {
								t.Fatalf("%s: line overflows width %d: %q", state, width, line)
							}
						}
					}
					check("normal")
					for range 2 {
						m, _ = uxRune(m, 'b')
						check("batch unmarked")
						m.batchSelected["a"] = true
						check("batch marked")
						m, _ = uxRune(m, 'b')
						check("normal after batch")
					}
				})
			}
		}
	}
}
