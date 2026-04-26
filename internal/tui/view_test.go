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
	main := Model{mode: modeMain, torrents: []models.Torrent{{ID: "a", Status: "downloaded"}}, selectedIdx: 0}
	if got := ansi.Strip(listFooter(main)); !strings.Contains(got, "d download") {
		t.Fatalf("listFooter() = %q, want managed download hint", got)
	}
	if got := ansi.Strip(detailFooter()); !strings.Contains(got, "d download") {
		t.Fatalf("detailFooter() = %q, want managed download hint", got)
	}
}

func TestVersionRenderersDoNotPrefixEmbeddedTag(t *testing.T) {
	for name, got := range map[string]string{
		"banner":        ansi.Strip(renderBanner("v1.2.3")),
		"compactHeader": ansi.Strip(renderCompactHeader("v1.2.3", "")),
	} {
		if strings.Contains(got, "vv1.2.3") {
			t.Fatalf("%s rendered duplicate version prefix: %q", name, got)
		}
		if !strings.Contains(got, "v1.2.3") {
			t.Fatalf("%s = %q, want embedded version", name, got)
		}
	}
}

func TestListFooterDimsUnavailableActions(t *testing.T) {
	m := Model{mode: modeMain, torrents: []models.Torrent{{ID: "a", Status: "queued"}}, selectedIdx: 0}
	got := ansi.Strip(listFooter(m))
	if !strings.Contains(got, "d download") {
		t.Fatalf("listFooter() = %q, should keep download visible for unfinished torrent", got)
	}
	if !strings.Contains(got, "y copy") {
		t.Fatalf("listFooter() = %q, should keep copy visible for unfinished torrent", got)
	}
}

func TestSearchFooterHidesUnrelatedActions(t *testing.T) {
	m := Model{mode: modeSearch}
	got := ansi.Strip(listFooter(m))
	if !strings.Contains(got, "esc clear") || !strings.Contains(got, "enter keep") {
		t.Fatalf("listFooter() = %q, want search actions", got)
	}
	if strings.Contains(got, "x delete") || strings.Contains(got, "d download") {
		t.Fatalf("listFooter() = %q, should hide unrelated list actions", got)
	}
}

func TestBrowserEditingFooterShowsOnlyEditingShortcuts(t *testing.T) {
	b := newFileBrowser(".")
	b.startEditing()
	m := Model{mode: modeFileBrowser, browser: b}
	footer := ansi.Strip(renderShortcutFooter(m.renderShortcutDefs(), m))
	if strings.Contains(footer, "hidden") || strings.Contains(footer, "visual") {
		t.Fatalf("footer = %q, should hide normal browser actions while editing", footer)
	}
	if !strings.Contains(footer, "tab complete") || !strings.Contains(footer, "enter navigate/select") {
		t.Fatalf("footer = %q, want editing shortcuts", footer)
	}
}

func TestBrowserFooterKeepsImportVisibleWhenUnavailable(t *testing.T) {
	b := newFileBrowser(".")
	m := Model{mode: modeFileBrowser, browser: b}
	footer := ansi.Strip(renderShortcutFooter(m.renderShortcutDefs(), m))
	if !strings.Contains(footer, "ctrl+s import") {
		t.Fatalf("footer = %q, want import shortcut visible", footer)
	}
}

func TestConstrainedInputViewShowsTailOfLongMagnet(t *testing.T) {
	m := NewModel(nil)
	m.mode = modeMagnetInput
	m.input.SetValue("magnet:?xt=urn:btih:" + strings.Repeat("a", 80) + "restofthestring")
	m.input.CursorEnd()

	got := ansi.Strip(constrainedInputView(m, 30))
	if !strings.Contains(got, "restofthestring") {
		t.Fatalf("input view = %q, want pasted URL tail visible", got)
	}
	if w := lipgloss.Width(got); w > 30 {
		t.Fatalf("input view width = %d, want <= 30: %q", w, got)
	}
}

func TestInputPopupKeepsLongMagnetOnPromptLine(t *testing.T) {
	m := NewModel(nil)
	m.mode = modeMagnetInput
	m.width = 100
	m.height = 24
	m.inputPrompt = "Paste a magnet link"
	m.input.SetValue("magnet:?xt=urn:btih:" + strings.Repeat("a", 80) + "restofthestring")
	m.input.CursorEnd()

	for _, line := range strings.Split(ansi.Strip(renderInputPopup(m, "Paste a magnet link")), "\n") {
		if strings.Contains(line, "restofthestring") && !strings.Contains(line, ">") {
			t.Fatalf("long input wrapped away from prompt line: %q", line)
		}
	}
}

func TestBatchFooterShowsBulkDownloadActionWhenUnavailable(t *testing.T) {
	m := Model{
		mode:          modeMain,
		batchMode:     true,
		batchSelected: map[string]bool{"a": true},
		torrents:      []models.Torrent{{ID: "a", Status: "downloaded"}},
	}
	footer := ansi.Strip(listFooter(m))
	if !strings.Contains(footer, "d download") {
		t.Fatalf("footer = %q, want dimmed bulk download action visible", footer)
	}
	if m.canBulkDownloadSelection() {
		t.Fatal("single marked torrent should not be eligible for bulk download")
	}
}

func TestRenderBulkOrderPopup(t *testing.T) {
	m := Model{mode: modeBulkOrder, width: 100, height: 24, bulk: newBulkDownloadState([]models.Torrent{{ID: "a", Filename: "Movie A"}, {ID: "b", Filename: "Movie B"}})}
	view := ansi.Strip(renderBulkOrderPopup(m))
	if !strings.Contains(view, "Bulk Download Order") || !strings.Contains(view, "Movie A") {
		t.Fatalf("bulk order popup = %q", view)
	}
}

func TestRenderBulkSummaryView(t *testing.T) {
	m := Model{mode: modeBulkDownload, width: 100, height: 24, bulk: newBulkDownloadState([]models.Torrent{{ID: "a", Filename: "Movie A"}, {ID: "b", Filename: "Movie B"}})}
	m.bulk.Items = []bulkQueueItem{
		{TorrentID: "a", TorrentName: "Movie A", Status: bulkFileSuccess},
		{TorrentID: "b", TorrentName: "Movie B", Status: bulkFileFailed, Error: "network"},
	}
	view := ansi.Strip(renderBulkDownloadView(m))
	if !strings.Contains(view, "Bulk Download Summary") || !strings.Contains(view, "Complete:") || !strings.Contains(view, "Failed:") {
		t.Fatalf("bulk summary view = %q", view)
	}
}

func TestRenderBulkProgressShowsMovingQueue(t *testing.T) {
	m := Model{mode: modeBulkDownload, width: 110, height: 24, bulk: newBulkDownloadState([]models.Torrent{{ID: "a", Filename: "Movie A"}, {ID: "b", Filename: "Movie B"}, {ID: "c", Filename: "Movie C"}, {ID: "d", Filename: "Movie D"}})}
	m.bulk.Items = []bulkQueueItem{
		{TorrentID: "a", TorrentName: "Movie A", Target: models.DownloadTarget{Label: "a.mkv"}, Status: bulkFileSuccess},
		{TorrentID: "b", TorrentName: "Movie B", Target: models.DownloadTarget{Label: "b.mkv"}, Status: bulkFileActive},
		{TorrentID: "c", TorrentName: "Movie C", Target: models.DownloadTarget{Label: "c.mkv"}, Status: bulkFilePending},
		{TorrentID: "d", TorrentName: "Movie D", Target: models.DownloadTarget{Label: "d.mkv"}, Status: bulkFilePending},
	}
	m.bulk.Current = 1
	m.download = &models.ManagedDownload{Filename: "b.mkv", Status: models.ManagedDownloadStatusActive, TotalLength: 100, CompletedLength: 25}

	view := ansi.Strip(renderBulkDownloadView(m))
	for _, want := range []string{"Queue:", "2 / 4", "01/04", "complete", "02/04", "active", "03/04", "pending"} {
		if !strings.Contains(view, want) {
			t.Fatalf("bulk progress view missing %q:\n%s", want, view)
		}
	}
}

func TestRenderBulkQueueKeepsCurrentVisibleAndBounded(t *testing.T) {
	bulk := newBulkDownloadState([]models.Torrent{{ID: "a", Filename: "Movie A"}})
	for idx := 0; idx < 12; idx++ {
		status := bulkFilePending
		if idx < 8 {
			status = bulkFileSuccess
		}
		if idx == 8 {
			status = bulkFileActive
		}
		bulk.Items = append(bulk.Items, bulkQueueItem{TorrentID: "a", TorrentName: "Movie A", Target: models.DownloadTarget{Label: fmt.Sprintf("file-%02d.mkv", idx+1)}, Status: status})
	}
	bulk.Current = 8

	queue := ansi.Strip(renderBulkQueue(bulk, 90, 5))
	if !strings.Contains(queue, "09/12") || !strings.Contains(queue, "active") {
		t.Fatalf("queue should keep active item visible:\n%s", queue)
	}
	if strings.Contains(queue, "01/12") || strings.Contains(queue, "12/12") {
		t.Fatalf("queue should be bounded around current item:\n%s", queue)
	}
	if strings.Count(queue, "...") != 2 {
		t.Fatalf("queue should show hidden rows above and below:\n%s", queue)
	}
}

func TestRenderBulkCleanupWarning(t *testing.T) {
	m := Model{mode: modeBulkCleanup, width: 100, height: 24, bulk: newBulkDownloadState([]models.Torrent{{ID: "a", Filename: "Movie A"}})}
	m.bulk.Items = []bulkQueueItem{{TorrentID: "a", TorrentName: "Movie A", Status: bulkFileFailed, Error: "network"}}
	m.bulk.CleanupSelected = map[string]bool{"a": true}
	view := ansi.Strip(renderBulkCleanupPopup(m))
	if !strings.Contains(view, "Warning: selected torrents include incomplete downloads") {
		t.Fatalf("cleanup popup = %q, want risky selection warning", view)
	}
}

func TestHelpOverlayKeepsUnavailableMainActionsVisible(t *testing.T) {
	m := Model{mode: modeMain, torrents: []models.Torrent{{ID: "a", Status: "queued"}}, selectedIdx: 0}
	got := ansi.Strip(renderHelpOverlay(m))
	if !strings.Contains(got, "y  copy") || !strings.Contains(got, "d  download") {
		t.Fatalf("help overlay = %q, want unavailable actions still listed", got)
	}
}

func TestDownloadFooterMentionsTorrentDeleteWhenAvailable(t *testing.T) {
	got := ansi.Strip(downloadFooter(&models.ManagedDownload{Status: models.ManagedDownloadStatusComplete}, true))
	if !strings.Contains(got, "x delete torrent") {
		t.Fatalf("downloadFooter() = %q, want delete torrent hint", got)
	}
}

func TestDownloadFooterKeepsCompletionActionsVisibleWhileActive(t *testing.T) {
	got := ansi.Strip(downloadFooter(&models.ManagedDownload{Status: models.ManagedDownloadStatusActive}, false))
	if !strings.Contains(got, "o open") || !strings.Contains(got, "s reveal") || !strings.Contains(got, "x delete torrent") {
		t.Fatalf("downloadFooter() = %q, want completion actions still visible", got)
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
