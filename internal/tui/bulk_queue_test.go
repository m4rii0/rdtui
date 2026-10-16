package tui

import (
	"context"
	"errors"
	"runtime"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/m4rii0/rdtui/internal/app"
	"github.com/m4rii0/rdtui/pkg/models"
)

// These spies cover the presentation boundary: navigation must not cancel work,
// observations must not start work, and decisions must retain the prompt identity.
type queueServiceSpy struct {
	startCalls, cancelCalls, snapshotCalls int
	plan                                   app.BulkPlan
	decision                               app.QueueDecision
	snapshot                               app.QueueSnapshot
	err                                    error
}

func (s *queueServiceSpy) Start(_ context.Context, plan app.BulkPlan) (app.QueueID, error) {
	s.startCalls++
	s.plan = plan
	return 7, s.err
}
func (s *queueServiceSpy) Snapshot(id app.QueueID) (app.QueueSnapshot, error) {
	s.snapshotCalls++
	return s.snapshot, s.err
}
func (s *queueServiceSpy) Decide(_ context.Context, d app.QueueDecision) error {
	s.decision = d
	return s.err
}
func (s *queueServiceSpy) Cancel(app.QueueID) error { s.cancelCalls++; return nil }

func observedQueue(t *testing.T) (Model, *queueServiceSpy) {
	t.Helper()
	s := &queueServiceSpy{}
	m := NewModel(nil, newTestRuntime(t), s)
	m.mode = modeBulkDownload
	m.bulk = newBulkDownloadState([]models.Torrent{{ID: "a", Filename: "complete"}, {ID: "b", Filename: "partial"}})
	m.bulk.QueueID = 7
	m.bulk.Snapshot = app.QueueSnapshot{ID: 7, CurrentIndex: 0, Items: []app.QueueItemSnapshot{
		{Item: app.QueueItem{TorrentID: "a", TorrentName: "complete"}, Status: app.QueueItemActive},
		{Item: app.QueueItem{TorrentID: "b", TorrentName: "partial"}, Status: app.QueueItemPending},
	}}
	s.snapshot = m.bulk.Snapshot
	m, _ = updateQueue(m, s.snapshot)
	return m, s
}

func updateQueue(m Model, snapshot app.QueueSnapshot) (Model, tea.Cmd) {
	updated, cmd := m.Update(bulkSnapshotMsg{queueID: snapshot.ID, request: requestID{m.queuePoll.Generation}, snapshot: snapshot})
	return updated.(Model), cmd
}

func TestBulkEscapeKeepsQueueRunning(t *testing.T) {
	m, s := observedQueue(t)
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = updated.(Model)
	if s.cancelCalls != 0 || m.mode != modeMain || m.bulk.QueueID != 7 {
		t.Fatal("navigation canceled or lost background queue")
	}
	s.snapshot.CurrentIndex = 1
	s.snapshot.Items[0].Status = app.QueueItemTerminal
	s.snapshot.Items[0].Outcome = &app.ItemOutcome{TorrentID: "a", Kind: app.OutcomeSuccess}
	s.snapshot.Items[1].Status = app.QueueItemActive
	m, _ = updateQueue(m, s.snapshot)
	if m.mode != modeMain || m.bulk.Current != 1 {
		t.Fatal("background observation displaced navigation")
	}
}

func TestBulkOverwriteDecisionUsesItemIdentity(t *testing.T) {
	for _, overwrite := range []bool{false, true} {
		t.Run(map[bool]string{false: "skip", true: "overwrite"}[overwrite], func(t *testing.T) {
			m, s := observedQueue(t)
			m.mode, m.returnMode = modeFileBrowser, modeMain
			s.snapshot.AwaitingOverwrite = true
			s.snapshot.ExistingPath = "/downloads/a.mkv"
			s.snapshot.ExistingBytes, s.snapshot.RemoteBytes = 10, 20
			m, _ = updateQueue(m, s.snapshot)
			if m.mode != modeOverwrite || m.pendingDownload == nil || m.pendingDownload.ExistingBytes != 10 {
				t.Fatal("missing application overwrite prompt")
			}
			key := tea.KeyEscape
			if overwrite {
				key = tea.KeyEnter
			}
			updated, cmd := m.Update(tea.KeyPressMsg{Code: key})
			m = updated.(Model)
			if cmd == nil || m.mode != modeFileBrowser || m.returnMode != modeMain || m.pendingDownload != nil {
				t.Fatal("decision lost navigation")
			}
			updated, _ = m.Update(cmd())
			if s.decision != (app.QueueDecision{QueueID: 7, ItemIndex: 0, Overwrite: overwrite}) || s.startCalls != 0 {
				t.Fatal("decision lost item identity or started backend")
			}
		})
	}
}

func TestBulkSnapshotDoesNotAdvanceQueue(t *testing.T) {
	m, s := observedQueue(t)
	s.snapshot.Items[0].Status = app.QueueItemTerminal
	s.snapshot.Items[0].Outcome = &app.ItemOutcome{TorrentID: "a", Kind: app.OutcomeSuccess}
	for range 2 {
		m, _ = updateQueue(m, s.snapshot)
	}
	if s.startCalls != 0 || m.bulk.Current != 0 || m.bulk.Items[1].Status != bulkFilePending {
		t.Fatal("presentation drove execution")
	}
}

func TestBulkCleanupUsesTerminalOutcomes(t *testing.T) {
	m, s := observedQueue(t)
	s.snapshot.Finished = true
	s.snapshot.Items = []app.QueueItemSnapshot{
		{Item: app.QueueItem{TorrentID: "a"}, Status: app.QueueItemTerminal, Outcome: &app.ItemOutcome{TorrentID: "a", Kind: app.OutcomeSuccess}},
		{Item: app.QueueItem{TorrentID: "b"}, Status: app.QueueItemTerminal, Outcome: &app.ItemOutcome{TorrentID: "b", Kind: app.OutcomeSuccess}},
		{Item: app.QueueItem{TorrentID: "b"}, Status: app.QueueItemTerminal, Outcome: &app.ItemOutcome{TorrentID: "b", Kind: app.OutcomeFailed, Err: errors.New("network")}},
	}
	m, _ = updateQueue(m, s.snapshot)
	updated, _ := m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	m = updated.(Model)
	if m.mode != modeBulkCleanup || !m.bulk.CleanupSelected["a"] || m.bulk.CleanupSelected["b"] {
		t.Fatal("cleanup did not default to fully successful torrents")
	}
	if !strings.Contains(m.bulk.torrentOutcome("b"), "partial") {
		t.Fatal("missing partial label")
	}
	m.bulk.CleanupCursor = 1
	updated, _ = m.Update(tea.KeyPressMsg{Code: ' ', Text: " "})
	m = updated.(Model)
	m, _ = updateQueue(m, s.snapshot)
	if !m.bulk.CleanupSelected["b"] || !m.bulk.hasRiskyCleanupSelection() {
		t.Fatal("snapshot reset manual cleanup toggle")
	}
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	if !updated.(Model).bulk.CleanupSelected["b"] {
		t.Fatal("reopening cleanup reset manual toggle")
	}
}

func TestBulkLateSnapshotIgnored(t *testing.T) {
	for _, stale := range []string{"queue", "request", "item", "download"} {
		t.Run(stale, func(t *testing.T) {
			m, s := observedQueue(t)
			m.status = "current"
			msg := bulkSnapshotMsg{queueID: 7, request: requestID{m.queuePoll.Generation}, snapshot: s.snapshot}
			switch stale {
			case "queue":
				msg.queueID = 6
				msg.snapshot.ID = 6
			case "request":
				msg.request.Generation++
			case "item":
				m.bulk.Snapshot.CurrentIndex = 1
			case "download":
				m.bulk.Snapshot.Download = &app.DownloadSnapshot{Ref: app.DownloadRef{Generation: 2, GID: "a"}}
				msg.snapshot.Download = &app.DownloadSnapshot{Ref: app.DownloadRef{Generation: 1, GID: "a"}}
			}
			updated, cmd := m.Update(msg)
			if cmd != nil || updated.(Model).status != "current" {
				t.Fatal("accepted stale queue observation")
			}
		})
	}
}

func TestBulkRefreshObservesQueue(t *testing.T) {
	m, s := observedQueue(t)
	backend := &bulkStatusSpy{}
	m.service = backend
	m.download = &models.ManagedDownload{GID: "a", Status: models.ManagedDownloadStatusActive}
	updated, cmd := m.handleShortcutAction(actionRefresh)
	m = updated.(Model)
	if cmd == nil {
		t.Fatal("refresh did not observe queue")
	}
	if _, ok := cmd().(bulkSnapshotMsg); !ok {
		t.Fatal("refresh polled backend instead of application queue")
	}
	if s.snapshotCalls != 1 {
		t.Fatal("missing queue observation")
	}
	if cmd := m.pollBulkQueue(); cmd != nil {
		t.Fatal("overlapping queue observations")
	}
}

type bulkStatusSpy struct {
	app.AppService
	calls int
}

func (s *bulkStatusSpy) Status(context.Context, app.DownloadRef) (app.DownloadSnapshot, error) {
	s.calls++
	return app.DownloadSnapshot{}, nil
}

func TestBulkOverwritePromptTracksCurrentItem(t *testing.T) {
	m, s := queueOverwriteAfterNavigation(t)
	s.snapshot.CurrentIndex = 0 // A stale event must not displace the pending prompt.
	m, _ = updateQueue(m, s.snapshot)
	if m.pendingDownload.Queue.ItemIndex != 1 {
		t.Fatal("stale item displaced prompt")
	}
	s.snapshot.CurrentIndex = 1
	s.snapshot.AwaitingOverwrite = false
	s.snapshot.Items[1].Status = app.QueueItemActive
	m, _ = updateQueue(m, s.snapshot)
	if m.pendingDownload != nil || m.mode != modeFileBrowser {
		t.Fatal("obsolete overwrite prompt blocked navigation")
	}
}

func TestBulkCleanupCanceledAndSkippedNotPreselected(t *testing.T) {
	m, s := observedQueue(t)
	s.snapshot.Finished = true
	s.snapshot.Items[0].Outcome = &app.ItemOutcome{TorrentID: "a", Kind: app.OutcomeCanceled}
	s.snapshot.Items[1].Outcome = &app.ItemOutcome{TorrentID: "b", Kind: app.OutcomeSkipped}
	m, _ = updateQueue(m, s.snapshot)
	m.bulk.initCleanupSelection()
	if len(m.bulk.CleanupSelected) != 0 || m.bulk.torrentOutcome("a") != "canceled" || m.bulk.torrentOutcome("b") != "skipped" {
		t.Fatal("unsafe cleanup defaults")
	}
	if label := bulkQueueStatusLabel(m.bulk.Items[0].Status); label != "canceled" {
		t.Fatalf("canceled item label=%q", label)
	}
}

type bulkBackendReply struct {
	download models.ManagedDownload
	err      error
}
type bulkBackendCall struct {
	request models.ManagedDownloadRequest
	reply   chan bulkBackendReply
}
type bulkBackend struct{ calls chan bulkBackendCall }

func (b *bulkBackend) StartDownload(ctx context.Context, request models.ManagedDownloadRequest) (models.ManagedDownload, error) {
	call := bulkBackendCall{request: request, reply: make(chan bulkBackendReply, 1)}
	select {
	case b.calls <- call:
	case <-ctx.Done():
		return models.ManagedDownload{}, ctx.Err()
	}
	select {
	case result := <-call.reply:
		return result.download, result.err
	case <-ctx.Done():
		return models.ManagedDownload{}, ctx.Err()
	}
}
func (*bulkBackend) DownloadStatus(context.Context, string) (models.ManagedDownload, error) {
	return models.ManagedDownload{}, errors.New("unexpected backend observation")
}
func (*bulkBackend) Shutdown(context.Context) error { return nil }

func TestBulkApplicationRunsWithoutPresentation(t *testing.T) {
	r := newTestRuntime(t)
	b := &bulkBackend{calls: make(chan bulkBackendCall, 2)}
	c := app.NewDownloadCoordinator(r.Context(), b, t.TempDir())
	t.Cleanup(func() {
		r.Stop()
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := c.Close(ctx); err != nil {
			t.Error(err)
		}
	})
	q := app.NewBulkQueue(app.QueueDependencies{Runtime: r, Downloads: c, Prepare: func(ctx context.Context, target models.DownloadTarget) (app.PreparedDownload, error) {
		return app.PreparedDownload{URL: target.Link, Filename: target.Label}, ctx.Err()
	}})
	m := NewModel(nil, r, q)
	m.mode = modeBulkConfirm
	m.bulk = newBulkDownloadState([]models.Torrent{{ID: "b"}, {ID: "a"}})
	m.bulk.Plans[0].Targets = []models.DownloadTarget{{Link: "b", Label: "b.iso"}, {Link: "unselected", Label: "skip.iso"}}
	m.bulk.Plans[0].Selected = map[int]bool{0: true}
	m.bulk.Plans[1].Targets = []models.DownloadTarget{{Link: "a", Label: "a.iso"}}
	m.bulk.Plans[1].Selected = map[int]bool{0: true}
	m.bulk.buildQueueItems()
	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	updated, poll := m.Update(cmd())
	m = updated.(Model)
	if poll == nil {
		t.Fatal("missing queue observation")
	}
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = updated.(Model)
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	for _, url := range []string{"b", "a"} {
		select {
		case call := <-b.calls:
			if call.request.URL != url {
				t.Fatalf("confirmed selection/order lost: %+v", call.request)
			}
			if url == "b" {
				call.reply <- bulkBackendReply{err: errors.New("individual failure")}
			} else {
				call.reply <- bulkBackendReply{download: models.ManagedDownload{GID: "a", Status: models.ManagedDownloadStatusComplete}}
			}
		case <-ctx.Done():
			t.Fatal("queue needed UI messages to advance")
		}
	}
	for {
		snapshot, err := q.Snapshot(m.bulk.QueueID)
		if err != nil {
			t.Fatal(err)
		}
		if snapshot.Finished {
			break
		}
		if ctx.Err() != nil {
			t.Fatal("queue did not finish independently")
		}
		runtime.Gosched()
	}
	updated, next := m.Update(poll())
	m = updated.(Model)
	if next != nil || m.mode != modeMain || !m.bulk.isFinished() || m.bulk.Items[0].Status != bulkFileFailed || m.bulk.Items[1].Status != bulkFileSuccess {
		t.Fatal("observation did not render autonomous application outcomes")
	}
}

func TestBulkAdmissionResultPreservesNavigation(t *testing.T) {
	for _, failed := range []bool{false, true} {
		t.Run(map[bool]string{false: "admitted", true: "rejected"}[failed], func(t *testing.T) {
			m, _ := observedQueue(t)
			m.bulk.QueueID = 0
			m.bulk.Starting = true
			m.mode, m.returnMode = modeFileBrowser, modeMain
			msg := bulkStartedMsg{generation: m.queueGeneration, queueID: 8}
			if failed {
				msg.err = app.ErrActiveDownload
				msg.active = &app.DownloadSnapshot{Ref: app.DownloadRef{Generation: 4, GID: "unrelated"}, Download: models.ManagedDownload{GID: "unrelated", Status: models.ManagedDownloadStatusActive}}
			}
			updated, _ := m.Update(msg)
			m = updated.(Model)
			if m.mode != modeFileBrowser || m.returnMode != modeMain {
				t.Fatal("late admission displaced navigation")
			}
		})
	}
}
