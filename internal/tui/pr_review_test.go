package tui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/m4rii0/rdtui/internal/app"
	"github.com/m4rii0/rdtui/pkg/models"
)

func TestReviewSortOwnsPendingOverwrite(t *testing.T) {
	for _, key := range []rune{tea.KeyEnter, tea.KeyEscape} {
		m, queue := observedQueue(t)
		m.mode = modeMain
		m, _ = uxRune(m, 'o')
		queue.snapshot.AwaitingOverwrite = true
		queue.snapshot.ExistingPath = "/downloads/existing.mkv"
		m, _ = updateQueue(m, queue.snapshot)
		if m.mode != modeSort {
			t.Fatalf("queue stole sort picker: %s", m.mode)
		}
		m, _ = uxPress(m, tea.KeyPressMsg{Code: key})
		if m.mode != modeOverwrite || m.pendingDownload == nil || m.pendingDownload.ResumeMode != modeMain || m.bulk.DecidedItem != nil {
			t.Fatal("sort answer did not reveal an undecided overwrite")
		}
	}
}

func TestReviewQuitOwnsStandaloneResults(t *testing.T) {
	for _, result := range []tea.Msg{
		resolvedDownloadMsg{prepared: app.PreparedDownload{Exists: true, URL: "url", Filename: "existing"}},
		managedDownloadMsg{ref: app.DownloadRef{Generation: 2}, result: models.ManagedDownloadStart{Download: models.ManagedDownload{Status: models.ManagedDownloadStatusComplete}}},
		managedDownloadMsg{ref: app.DownloadRef{Generation: 2}, result: models.ManagedDownloadStart{Download: models.ManagedDownload{Status: models.ManagedDownloadStatusActive}}},
	} {
		for _, answer := range []rune{tea.KeyEnter, 'n', tea.KeyEscape, 'y'} {
			m := NewModel(Dependencies{Runtime: newTestRuntime(t)})
			m.mode = modeMain
			m.download = &models.ManagedDownload{Status: models.ManagedDownloadStatusActive}
			m, _ = uxRune(m, 'q')
			next, _ := m.Update(result)
			m = next.(Model)
			if m.mode != modeQuitConfirm {
				t.Fatalf("%T stole quit dialog: %s", result, m.mode)
			}
			key := tea.KeyPressMsg{Code: answer}
			if answer == 'n' || answer == 'y' {
				key.Text = string(answer)
			}
			m, cmd := uxPress(m, key)
			if answer == 'y' {
				if cmd == nil || m.runtime.Context().Err() == nil {
					t.Fatal("quit not confirmed")
				}
			} else if _, exists := result.(resolvedDownloadMsg); exists {
				if m.mode != modeOverwrite || m.pendingDownload == nil || m.setup.InFlight || cmd != nil {
					t.Fatal("cancel quit authorized overwrite or lost pending decision")
				}
			} else if m.mode != modeDownload {
				t.Fatal("cancel quit lost started download")
			}
		}
	}
}

func TestReviewSortOwnsExpiredOverwrite(t *testing.T) {
	m, queue := observedQueue(t)
	m.mode = modeMain
	m, _ = uxRune(m, 'o')
	queue.snapshot.AwaitingOverwrite = true
	queue.snapshot.ExistingPath = "/downloads/existing.mkv"
	m, _ = updateQueue(m, queue.snapshot)
	queue.snapshot.AwaitingOverwrite = false
	queue.snapshot.Finished = true
	m, _ = updateQueue(m, queue.snapshot)
	if m.mode != modeSort || m.pendingDownload != nil {
		t.Fatal("expired overwrite closed sort picker or retained stale decision")
	}
	m, _ = uxPress(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.mode != modeMain {
		t.Fatal("sort restored expired overwrite")
	}
}

func TestReviewCancelDraftKeepsObservation(t *testing.T) {
	m := NewModel(Dependencies{Runtime: newTestRuntime(t)})
	m.mode = modeBulkOrder
	m.bulk = newBulkDownloadState([]models.Torrent{{ID: "draft"}})
	m.queueGeneration = 3
	m.downloadRef = app.DownloadRef{Generation: 1, GID: "active"}
	m.download = &models.ManagedDownload{Status: models.ManagedDownloadStatusActive}
	_, request := m.downloadPoll.begin(m.runtime.Context())
	result := managedDownloadStatusMsg{request: downloadRequestID{request, m.downloadRef}, queue: m.queueIdentity(), ok: true,
		download: models.ManagedDownload{Status: models.ManagedDownloadStatusActive, CompletedLength: 42}}
	m, _ = uxPress(m, tea.KeyPressMsg{Code: tea.KeyEscape})
	next, cmd := m.Update(result)
	m = next.(Model)
	if m.downloadPoll.InFlight || m.download.CompletedLength != 42 || cmd == nil {
		t.Fatal("canceling draft left standalone observation stuck")
	}
}

func TestReviewCompactAuthenticationControls(t *testing.T) {
	for _, version := range []string{"", "v1.0.0"} {
		m := NewModel(Dependencies{Runtime: newTestRuntime(t)})
		m.width, m.height, m.version = 40, 12, version
		m.mode = modeDeviceAuth
		m.deviceCode = &models.DeviceCode{UserCode: "ABCD1234", VerificationURL: "https://real-debrid.com/device", RequestedAt: time.Now(), ExpiresIn: 600}
		view := ansi.Strip(renderView(m))
		for _, text := range []string{"ABCD1234", "https://real-debrid.com/device", "esc", "poll"} {
			if !strings.Contains(view, text) {
				t.Errorf("missing %q: %s", text, view)
			}
		}
		m.mode, m.inputPrompt = modeTokenInput, "Private token"
		m.input.SetValue("token-value")
		view = ansi.Strip(renderView(m))
		for _, text := range []string{"token-value", "submit", "cancel"} {
			if !strings.Contains(view, text) {
				t.Errorf("missing %q: %s", text, view)
			}
		}
		m.mode = modeAuthChoice
		view = ansi.Strip(renderView(m))
		for _, text := range []string{"Private API token", "Device auth", "quit"} {
			if !strings.Contains(view, text) {
				t.Errorf("missing %q: %s", text, view)
			}
		}
	}
}

func TestReviewHelpLongErrorTailReachable(t *testing.T) {
	m := NewModel(Dependencies{Runtime: newTestRuntime(t)})
	m.mode, m.width, m.height = modeMain, 40, 12
	m.errText = strings.Repeat("abcdefghijklmnopqrstuvwxyz", 12) + " FINAL-TAIL"
	m.helpOffset = helpMaxOffset(m)
	if view := ansi.Strip(renderHelpOverlay(m)); !strings.Contains(view, "FINAL-TAIL") {
		t.Fatalf("long error tail clipped at maximum scroll: %s", view)
	}
}

func TestReviewDownloadFailureAvailableInHelp(t *testing.T) {
	m := NewModel(Dependencies{Runtime: newTestRuntime(t)})
	m.mode, m.width, m.height = modeDownload, 40, 12
	m.download = &models.ManagedDownload{Status: models.ManagedDownloadStatusError, ErrorMessage: "disk full: FINAL-FAILURE"}
	m.helpOffset = helpMaxOffset(m)
	if view := ansi.Strip(renderHelpOverlay(m)); !strings.Contains(view, "FINAL-FAILURE") {
		t.Fatalf("download error not inspectable: %s", view)
	}
}

func TestReviewCompactSearchVisible(t *testing.T) {
	for _, base := range []mode{modeSearch, modeMain} {
		for _, height := range []int{24, 2} {
			m := NewModel(Dependencies{Runtime: newTestRuntime(t)})
			m.mode, m.width, m.height = base, 39, height
			m.searchInput.SetValue("needle")
			m.filterApplied = base == modeMain
			if base == modeSearch {
				m.searchInput.Focus()
			}
			if view := ansi.Strip(renderView(m)); !strings.Contains(view, "needle") {
				t.Fatalf("%s hides search at height %d: %s", base, height, view)
			}
		}
	}
}

func TestReviewCompactSearchKeepsEditingTailVisible(t *testing.T) {
	m := NewModel(Dependencies{Runtime: newTestRuntime(t)})
	m.mode, m.width, m.height = modeSearch, 39, 24
	m.searchInput.Focus()
	m.searchInput.SetValue(strings.Repeat("a", 35) + "TAIL")
	m.searchInput.CursorEnd()
	if view := ansi.Strip(renderView(m)); !strings.Contains(strings.Split(view, "\n")[0], "TAIL") {
		t.Fatalf("editing tail hidden: %s", view)
	}
}
