package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/m4rii0/rdtui/pkg/models"
)

func TestTorrentListWindowKeepsSelectionVisible(t *testing.T) {
	start, end := torrentListWindow(100, 50, 10)
	if start > 50 || end <= 50 {
		t.Fatalf("selection not visible: start=%d end=%d", start, end)
	}
	if end-start != 10 {
		t.Fatalf("window size = %d, want 10", end-start)
	}
}

func TestTorrentListWindowClampsToStart(t *testing.T) {
	start, end := torrentListWindow(20, 1, 8)
	if start != 0 || end != 8 {
		t.Fatalf("window = (%d,%d), want (0,8)", start, end)
	}
}

func TestTorrentListWindowClampsToEnd(t *testing.T) {
	start, end := torrentListWindow(20, 19, 8)
	if start != 12 || end != 20 {
		t.Fatalf("window = (%d,%d), want (12,20)", start, end)
	}
}

func TestScrollbarThumbFitsVisibleTrack(t *testing.T) {
	top, size := scrollbarThumb(100, 10, 45)
	if size < 1 || size > 10 {
		t.Fatalf("thumb size = %d, want 1..10", size)
	}
	if top < 0 || top > 10-size {
		t.Fatalf("thumb top = %d, out of bounds for size %d", top, size)
	}
}

func TestRenderViewHeightWithManyTorrents(t *testing.T) {
	m := Model{
		mode:          modeMain,
		width:         120,
		height:        24,
		selectedIdx:   35,
		sortColumn:    colAdded,
		sortAscending: false,
		session:       &models.AuthSession{User: models.User{Username: "debug"}},
		detail:        &models.TorrentInfo{Torrent: models.Torrent{Filename: "selected torrent", Status: "downloading", Progress: 99.67}, Files: []models.TorrentFile{{ID: 1, Path: "/file.mkv", Bytes: 1024}}, OriginalBytes: 1024},
	}
	for i := 0; i < 80; i++ {
		m.torrents = append(m.torrents, models.Torrent{
			ID:       fmt.Sprintf("id-%d", i),
			Filename: fmt.Sprintf("torrent-%02d-with-a-rather-long-name-to-force-layout-behavior", i),
			Status:   "downloading",
			Progress: float64(i) + 0.67,
			Added:    time.Unix(0, 0),
		})
	}

	view := renderView(m)
	height := lipgloss.Height(view)
	t.Logf("rendered height=%d", height)
	if height > m.height {
		t.Fatalf("rendered height = %d, want <= %d\n%s", height, m.height, view)
	}
}

func TestLongNamesDoNotWrap(t *testing.T) {
	torrent := models.Torrent{
		ID:       "1",
		Filename: strings.Repeat("A", 200),
		Status:   "downloaded",
		Progress: 100,
		Added:    time.Now(),
		Bytes:    1024,
	}
	m := Model{
		mode:          modeMain,
		width:         120,
		height:        24,
		selectedIdx:   0,
		sortColumn:    colAdded,
		sortAscending: false,
		session:       &models.AuthSession{User: models.User{Username: "test"}},
		torrents:      []models.Torrent{torrent},
		detail:        &models.TorrentInfo{Torrent: torrent, OriginalBytes: 1024},
	}

	view := renderView(m)
	padW := appStyle.GetHorizontalFrameSize()
	for _, line := range strings.Split(view, "\n") {
		w := lipgloss.Width(line)
		if w > m.width+padW {
			t.Fatalf("line width %d exceeds terminal width %d (incl padding): %q", w, m.width+padW, line)
		}
	}
}

func TestTableColumnsWidthsSumValid(t *testing.T) {
	for _, totalWidth := range []int{40, 60, 80, 100, 120} {
		for _, scrollbar := range []bool{true, false} {
			cols := tableColumns(totalWidth, scrollbar)
			sum := 0
			for _, c := range cols {
				sum += c.Width
			}
			gaps := len(cols) - 1
			if scrollbar {
				gaps += 2
			}
			totalUsed := sum + gaps
			if totalUsed > totalWidth {
				t.Errorf("totalWidth=%d scrollbar=%v: used %d (cols sum=%d gaps=%d)", totalWidth, scrollbar, totalUsed, sum, gaps)
			}
		}
	}
}

func TestRenderTableHeaderShowsSortArrow(t *testing.T) {
	cols := tableColumns(80, false)

	h := renderTableHeader(cols, colAdded, false, 80)
	if !strings.Contains(h, "↓") {
		t.Fatal("descending sort should show ↓")
	}

	h = renderTableHeader(cols, colAdded, true, 80)
	if !strings.Contains(h, "↑") {
		t.Fatal("ascending sort should show ↑")
	}

	h = renderTableHeader(cols, colSize, false, 80)
	if strings.Contains(h, "↓") {
		glyphs := strings.Count(h, "↓") + strings.Count(h, "↑")
		if glyphs > 1 {
			t.Fatal("only the sorted column should have an arrow")
		}
	}
}

func TestPadVisual(t *testing.T) {
	got := padVisual("hi", 6, false)
	if got != "hi    " {
		t.Fatalf("left-aligned: %q", got)
	}

	got = padVisual("hi", 6, true)
	if got != "    hi" {
		t.Fatalf("right-aligned: %q", got)
	}

	got = padVisual("toolong", 4, false)
	if lipgloss.Width(got) != 4 {
		t.Fatalf("truncated width = %d, want 4: %q", lipgloss.Width(got), got)
	}
}

func TestFootersMentionManagedDownload(t *testing.T) {
	if got := ansi.Strip(listFooter(Model{})); !strings.Contains(got, "d download") {
		t.Fatalf("listFooter() = %q, want managed download hint", got)
	}
	if got := ansi.Strip(detailFooter()); !strings.Contains(got, "d download") {
		t.Fatalf("detailFooter() = %q, want managed download hint", got)
	}
}

func TestDownloadFooterMentionsTorrentDeleteWhenAvailable(t *testing.T) {
	got := ansi.Strip(downloadFooter(&models.ManagedDownload{Status: models.ManagedDownloadStatusComplete}, true))
	if !strings.Contains(got, "x delete torrent") {
		t.Fatalf("downloadFooter() = %q, want delete torrent hint", got)
	}
}

func TestOverwriteModalShowsByteDiff(t *testing.T) {
	m := Model{
		mode:  modeOverwrite,
		width: 120,
		pendingDownload: &pendingDownloadState{
			Filename:      "movie.mkv",
			Path:          "/tmp/movie.mkv",
			ExistingBytes: 1024,
			RemoteBytes:   2048,
		},
	}

	got := ansi.Strip(renderOverwritePopup(m))
	if !strings.Contains(got, "Diff:") {
		t.Fatalf("renderOverwritePopup() = %q, want diff line", got)
	}
	if !strings.Contains(got, "smaller than remote") {
		t.Fatalf("renderOverwritePopup() = %q, want size comparison", got)
	}
}
