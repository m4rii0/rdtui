package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/m4rii0/rdtui/pkg/models"
)

func TestMainStableGeometry(t *testing.T) {
	for _, size := range [][2]int{{120, 30}, {80, 24}, {40, 12}, {24, 8}, {1, 1}} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			m := NewModel(Dependencies{Runtime: newTestRuntime(t)})
			m.mode, m.width, m.height = modeMain, size[0], size[1]
			m.torrents = []models.Torrent{{ID: "one", Filename: strings.Repeat("映像", 80), Status: "downloaded"}}
			for _, variant := range []string{"idle", "status", "error", "flash", "search"} {
				m.status, m.errText = "", ""
				m.flash = flashState{}
				m.mode = modeMain
				switch variant {
				case "status":
					m.status = "Copied"
				case "error":
					m.errText = "first\n" + strings.Repeat("error", 80)
				case "flash":
					m.flash.message = "Copied\nURL"
				case "search":
					m.mode = modeSearch
					m.applyFilter()
				}
				view := renderView(m)
				lines := strings.Split(view, "\n")
				if len(lines) > size[1] {
					t.Fatalf("%s height %d > %d", variant, len(lines), size[1])
				}
				for _, line := range lines {
					if lipgloss.Width(line) > size[0] {
						t.Fatalf("%s width %d > %d: %q", variant, lipgloss.Width(line), size[0], line)
					}
				}
				if size[0] >= 40 && size[1] >= 12 {
					if len(lines) != size[1] {
						t.Fatalf("%s height=%d want=%d", variant, len(lines), size[1])
					}
					if !strings.Contains(ansi.Strip(lines[size[1]-2]), "?") {
						t.Fatalf("footer moved: %s", view)
					}
					if !strings.Contains(ansi.Strip(lines[3]), "Status") {
						t.Fatalf("table moved: %s", view)
					}
				}
			}
		})
	}
}

func TestHelpErrorScroll(t *testing.T) {
	m := NewModel(Dependencies{Runtime: newTestRuntime(t)})
	m.mode, m.width, m.height = modeMain, 40, 12
	m.errText = strings.Repeat("wrapped error detail\n", 40) + "LAST ERROR DETAIL"
	m.helpVisible = true
	found := false
	for range 80 {
		view := renderView(m)
		if strings.Contains(ansi.Strip(renderHelpOverlay(m)), "LAST ERROR DETAIL") {
			found = true
			break
		}
		if lipgloss.Height(view) > 12 || lipgloss.Width(view) > 40 {
			t.Fatalf("help overflow: %dx%d", lipgloss.Width(view), lipgloss.Height(view))
		}
		next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyPgDown})
		m = next.(Model)
	}
	if !found {
		t.Fatal("full error not reachable through help")
	}
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = next.(Model)
	if m.helpVisible || m.mode != modeMain {
		t.Fatal("help did not restore base screen")
	}
}
