package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/m4rii0/rdtui/pkg/models"
)

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
