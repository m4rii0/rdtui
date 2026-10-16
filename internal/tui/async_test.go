package tui

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/m4rii0/rdtui/internal/app"
	"github.com/m4rii0/rdtui/internal/realdebrid"
	"github.com/m4rii0/rdtui/pkg/models"
)

type asyncService struct {
	AuthService
	TorrentService
	DownloadService
	entered      chan context.Context
	release      chan struct{}
	statusCalls  atomic.Int32
	listCalls    atomic.Int32
	resultErr    error
	requestedRef app.DownloadRef
}

func (*asyncService) Current() (app.DownloadSnapshot, bool) { return app.DownloadSnapshot{}, false }

func currentDownloadResult(m Model, download models.ManagedDownload, err error) managedDownloadStatusMsg {
	return managedDownloadStatusMsg{request: downloadRequestID{requestID{m.downloadPoll.Generation}, m.downloadRef}, queue: m.queueIdentity(), ok: true, download: download, err: err}
}

func currentSetupResult(m Model, msg tea.Msg) featureResultMsg {
	return featureResultMsg{feature: setupFeature, request: requestID{m.setup.Generation}, queue: m.queueIdentity(), msg: msg}
}

func (s *asyncService) Status(ctx context.Context, ref app.DownloadRef) (app.DownloadSnapshot, error) {
	s.statusCalls.Add(1)
	s.requestedRef = ref
	s.entered <- ctx
	select {
	case <-s.release:
	case <-ctx.Done():
	}
	return app.DownloadSnapshot{Ref: ref, Download: models.ManagedDownload{GID: "current", Status: models.ManagedDownloadStatusActive}}, s.resultErr
}
func (s *asyncService) ListTorrents(ctx context.Context) ([]models.Torrent, error) {
	s.listCalls.Add(1)
	s.entered <- ctx
	select {
	case <-s.release:
	case <-ctx.Done():
	}
	return nil, s.resultErr
}
func (s *asyncService) AuthenticateWithToken(ctx context.Context, _ string, _ bool) (*models.AuthSession, error) {
	s.entered <- ctx
	select {
	case <-s.release:
	case <-ctx.Done():
	}
	return &models.AuthSession{User: models.User{Username: "old"}}, s.resultErr
}

func (s *asyncService) TorrentInfo(ctx context.Context, id string) (models.TorrentInfo, error) {
	s.entered <- ctx
	select {
	case <-s.release:
	case <-ctx.Done():
	}
	return models.TorrentInfo{ID: id}, s.resultErr
}

func (s *asyncService) ResolveDirectURL(ctx context.Context, _ models.DownloadTarget) (models.UnrestrictedLink, error) {
	s.entered <- ctx
	select {
	case <-s.release:
	case <-ctx.Done():
	}
	return models.UnrestrictedLink{Download: "https://example.com/old", Filename: "old.mkv"}, s.resultErr
}

func TestLateDownloadResultIgnored(t *testing.T) {
	for _, identity := range []string{"request", "same-gid-old-admission", "other-download", "queue", "item"} {
		for _, response := range []string{"success", "error"} {
			t.Run(identity+"/"+response, func(t *testing.T) {
				m := asyncQueue(t)
				m.mode, m.status, m.errText = modeMain, "current download", "current error"
				m.downloadRef = app.DownloadRef{Generation: 2, GID: "a"}
				m.downloadPoll.Generation, m.queueGeneration = 5, 3
				currentRef, currentDownload := m.downloadRef, *m.download
				late := currentDownloadResult(m, models.ManagedDownload{GID: "a", Status: models.ManagedDownloadStatusComplete}, nil)
				if response == "error" {
					late.err = errors.New("obsolete error")
				}
				switch identity {
				case "request":
					late.request.Request.Generation = 4
				case "same-gid-old-admission":
					late.request.Ref.Generation = 1
				case "other-download":
					late.request.Ref.GID = "old"
				case "queue":
					late.queue.ID = 2
				case "item":
					late.queue.Item = 1
				}
				updated, cmd := m.Update(late)
				m = updated.(Model)
				if m.downloadRef != currentRef || *m.download != currentDownload || m.bulk.Current != 0 || m.status != "current download" || m.errText != "current error" || m.mode != modeMain || cmd != nil {
					t.Fatal("accepted stale download")
				}
			})
		}
	}
}

func asyncQueue(t *testing.T) Model {
	t.Helper()
	m := NewModel(Dependencies{Runtime: newTestRuntime(t)})
	m.mode, m.returnMode = modeBulkDownload, modeBulkDownload
	m.bulk = newBulkDownloadState([]models.Torrent{{ID: "a"}, {ID: "b"}, {ID: "c"}})
	m.bulk.Items = []bulkQueueItem{{TorrentID: "a", Status: bulkFileActive}, {TorrentID: "b", Status: bulkFilePending}, {TorrentID: "c", Status: bulkFilePending}}
	m.bulk.Current = 0
	m.download = &models.ManagedDownload{GID: "a", Status: models.ManagedDownloadStatusActive}
	return m
}

func TestDuplicateTerminalResultIgnored(t *testing.T) {
	m, s := observedQueue(t)
	s.snapshot.CurrentIndex = 1
	s.snapshot.Items[0].Status = app.QueueItemTerminal
	s.snapshot.Items[0].Outcome = &app.ItemOutcome{TorrentID: "a", Kind: app.OutcomeSuccess}
	s.snapshot.Items[1].Status = app.QueueItemActive
	terminal := bulkSnapshotMsg{queueID: 7, request: requestID{m.queuePoll.Generation}, snapshot: s.snapshot}
	updated, _ := m.Update(terminal)
	m = updated.(Model)
	updated, cmd := m.Update(terminal)
	m = updated.(Model)
	if m.bulk.Current != 1 || m.bulk.Items[1].Status != bulkFileActive || cmd != nil {
		t.Fatal("duplicate terminal result advanced queue twice")
	}
}

func TestDownloadPollingSingleFlight(t *testing.T) {
	s := &asyncService{entered: make(chan context.Context, 2), release: make(chan struct{})}
	m := NewModel(asyncDependencies(t, s))
	m.mode = modeDownload
	m.download = &models.ManagedDownload{GID: "current", Status: models.ManagedDownloadStatusActive}
	m.downloadRef = app.DownloadRef{Generation: 7, GID: "current"}
	updated, cmd := m.Update(downloadTickMsg{downloadRequestID{Ref: m.downloadRef}})
	m = updated.(Model)
	done := make(chan tea.Msg, 1)
	go func() { done <- cmd() }()
	<-s.entered
	_, overlap := m.Update(downloadTickMsg{downloadRequestID{requestID{m.downloadPoll.Generation}, m.downloadRef}})
	_, manual := m.handleShortcutAction(actionRefresh)
	close(s.release)
	<-done
	if overlap != nil || manual != nil || s.statusCalls.Load() != 1 || s.requestedRef != m.downloadRef {
		t.Fatal("overlapping poll")
	}
}

func TestListPollingSingleFlight(t *testing.T) {
	s := &asyncService{entered: make(chan context.Context, 2), release: make(chan struct{})}
	m := NewModel(asyncDependencies(t, s))
	m.mode = modeMain
	updated, cmd := m.Update(tickMsg{})
	m = updated.(Model)
	done := make(chan tea.Msg, 1)
	go func() { done <- cmd() }()
	<-s.entered
	_, overlap := m.Update(tickMsg{requestID{m.list.Generation}})
	_, manual := m.handleShortcutAction(actionRefresh)
	close(s.release)
	<-done
	if overlap != nil || manual != nil || s.listCalls.Load() != 1 {
		t.Fatal("overlapping list poll")
	}
}

func TestCanceledAuthResultIgnored(t *testing.T) {
	s := &asyncService{entered: make(chan context.Context, 1), release: make(chan struct{})}
	m := NewModel(asyncDependencies(t, s))
	m.mode, m.inputAction = modeTokenInput, inputToken
	m.input.SetValue("token")
	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	done := make(chan tea.Msg, 1)
	go func() { done <- cmd() }()
	ctx := <-s.entered
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = updated.(Model)
	canceled := ctx.Err() != nil
	close(s.release)
	updated, _ = m.Update(<-done)
	m = updated.(Model)
	if !canceled || m.mode != modeAuthChoice || m.session != nil {
		t.Fatal("canceled auth response won or request was not canceled")
	}
}

func TestOldDetailErrorDoesNotReplaceCurrent(t *testing.T) {
	for _, identity := range []string{"other-torrent", "same-torrent-old-generation"} {
		t.Run(identity, func(t *testing.T) {
			m := NewModel(Dependencies{Runtime: newTestRuntime(t)})
			m.mode = modeMain
			m.torrents = []models.Torrent{{ID: "current"}}
			m.detail = &models.TorrentInfo{ID: "current"}
			m.errText = "current error"
			late := detailMsg{id: "old", err: errors.New("old error")}
			if identity == "same-torrent-old-generation" {
				m.detailRequest.Generation = 2
				late.request.Generation = 1
				late.id = "current"
			}
			updated, cmd := m.Update(late)
			m = updated.(Model)
			if m.errText != "current error" || m.detail.ID != "current" || cmd != nil {
				t.Fatal("old detail error replaced current state")
			}
		})
	}
}

func TestSupersededRequestsCanceled(t *testing.T) {
	for _, feature := range []string{"auth", "detail", "setup", "file-selection-setup", "download-setup"} {
		for _, response := range []string{"success", "error"} {
			t.Run(feature+"/"+response, func(t *testing.T) {
				s := &asyncService{entered: make(chan context.Context, 1), release: make(chan struct{})}
				if response == "error" {
					s.resultErr = errors.New("obsolete error")
				}
				m := NewModel(asyncDependencies(t, s))
				m.mode = modeMain
				m.torrents = []models.Torrent{{ID: "old"}, {ID: "current"}}
				var cmd tea.Cmd
				switch feature {
				case "auth":
					m.mode, m.inputAction = modeTokenInput, inputToken
					m.input.SetValue("old-token")
					updated, command := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
					m, cmd = updated.(Model), command
				case "detail":
					cmd = m.requestDetail("old")
				case "setup":
					m.mode = modeChooseTarget
					m.targets = targetPickerState{Action: handoffDownload, Items: []models.DownloadTarget{{Link: "old"}}}
					updated, command := m.handlePopupConfirm()
					m, cmd = updated.(Model), command
				case "file-selection-setup":
					m.batchSelected = map[string]bool{"old": true}
					m.torrents[0].Status = "waiting_files_selection"
					updated, command := m.beginBulkFileSelectionSetup()
					m, cmd = updated.(Model), command
				case "download-setup":
					m.bulk = newBulkDownloadState(m.torrents)
					m.mode = modeBulkOrder
					updated, command := m.handlePopupConfirm()
					m, cmd = updated.(Model), command
				}
				done := make(chan tea.Msg, 1)
				go func() { done <- cmd() }()
				ctx := <-s.entered
				switch feature {
				case "auth":
					m.input.SetValue("current-token")
					updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
					m = updated.(Model)
				case "detail":
					m.selectedIdx = 1
					_ = m.requestDetail("current")
				default:
					updated, _ := m.handlePopupCancel()
					m = updated.(Model)
				}
				if ctx.Err() == nil {
					t.Error("superseded request context is still live")
				}
				m.errText, m.status = "current error", "current status"
				activeGeneration, activeMode := m.auth.Generation, m.mode
				close(s.release)
				updated, next := m.Update(<-done)
				m = updated.(Model)
				if m.errText != "current error" || m.status != "current status" || m.mode != activeMode || m.auth.Generation != activeGeneration || next != nil {
					t.Fatal("obsolete response changed current state")
				}
			})
		}
	}
}

func TestPollingSchedulesAfterCompletion(t *testing.T) {
	for _, feature := range []string{"download", "list"} {
		t.Run(feature, func(t *testing.T) {
			s := &asyncService{entered: make(chan context.Context, 1), release: make(chan struct{}), resultErr: errors.New("temporary observation failure")}
			m := NewModel(asyncDependencies(t, s))
			m.mode = modeMain
			m.download = &models.ManagedDownload{GID: "current", Filename: "keep.mkv", Status: models.ManagedDownloadStatusActive}
			var delays []time.Duration
			m.schedule = func(delay time.Duration, msg tea.Msg) tea.Cmd {
				delays = append(delays, delay)
				return func() tea.Msg { return msg }
			}
			var cmd tea.Cmd
			wantDelay := 5 * time.Second
			if feature == "download" {
				m.mode = modeDownload
				cmd = m.pollDownload()
				wantDelay = time.Second
			} else {
				cmd = m.refresh()
			}
			done := make(chan tea.Msg, 1)
			go func() { done <- cmd() }()
			<-s.entered
			if len(delays) != 0 {
				t.Fatal("scheduled poll before request completed")
			}
			close(s.release)
			result := <-done
			updated, tick := m.Update(result)
			m = updated.(Model)
			if len(delays) != 1 || delays[0] != wantDelay || tick == nil {
				t.Fatalf("retry scheduling = %v", delays)
			}
			if m.download.Filename != "keep.mkv" {
				t.Fatal("query error replaced latest snapshot")
			}
			_, duplicate := m.Update(result)
			if duplicate != nil || len(delays) != 1 {
				t.Fatal("duplicate response scheduled another poll")
			}
			updated, next := m.Update(tick())
			m = updated.(Model)
			if next == nil {
				t.Fatal("retry tick did not start request")
			}
			if feature == "download" {
				terminal := currentDownloadResult(m, models.ManagedDownload{GID: "current", Status: models.ManagedDownloadStatusComplete}, nil)
				_, next = m.Update(terminal)
				if next != nil || len(delays) != 1 {
					t.Fatal("terminal result continued polling")
				}
			}
		})
	}
}

func TestDevicePollsUseNewIdentity(t *testing.T) {
	s := &asyncService{entered: make(chan context.Context, 1), release: make(chan struct{})}
	m := NewModel(asyncDependencies(t, s))
	m.mode = modeDeviceAuth
	m.deviceCode = &models.DeviceCode{RequestedAt: time.Now(), ExpiresIn: 100, Interval: 1}
	m.schedule = func(_ time.Duration, msg tea.Msg) tea.Cmd { return func() tea.Msg { return msg } }
	// Drive the real command envelope, substituting the external provider below.
	first := m.pollDevice()
	if first == nil {
		t.Fatal("missing device poll")
	}
	old := featureResultMsg{feature: authFeature, request: requestID{m.auth.Generation}, msg: devicePollMsg{err: realdebrid.ErrAuthorizationPending}}
	updated, tick := m.Update(old)
	m = updated.(Model)
	updated, _ = m.Update(tick())
	m = updated.(Model)
	m.errText = "current error"
	updated, cmd := m.Update(old)
	m = updated.(Model)
	if cmd != nil || m.errText != "current error" || !m.auth.InFlight {
		t.Fatal("old device poll was accepted during new request")
	}
}

func TestQueueIdentitySurvivesNavigation(t *testing.T) {
	m, s := observedQueue(t)
	s.snapshot.CurrentIndex = 1
	s.snapshot.Items[0].Status = app.QueueItemTerminal
	s.snapshot.Items[0].Outcome = &app.ItemOutcome{TorrentID: "a", Kind: app.OutcomeSuccess}
	s.snapshot.Items[1].Status = app.QueueItemActive
	// Opening another screen can change returnMode; it is not queue identity.
	m.mode, m.returnMode = modeFileBrowser, modeMain
	m, cmd := updateQueue(m, s.snapshot)
	if cmd == nil || m.bulk.Current != 1 || m.mode != modeFileBrowser {
		t.Fatal("navigation orphaned the active queue result")
	}
}

func TestNewTargetPickerCancelsPreviousSetup(t *testing.T) {
	s := &asyncService{entered: make(chan context.Context, 1), release: make(chan struct{})}
	m := NewModel(asyncDependencies(t, s))
	m.mode = modeMain
	cmd := m.resolveDownload(models.DownloadTarget{Link: "old"})
	done := make(chan tea.Msg, 1)
	go func() { done <- cmd() }()
	ctx := <-s.entered
	m.detail = &models.TorrentInfo{ID: "new", Status: "downloaded", Links: []string{"one", "two"}}
	updated, _ := m.beginHandoff(handoffDownload)
	m = updated.(Model)
	if ctx.Err() == nil {
		t.Error("new setup did not cancel superseded resolution")
	}
	close(s.release)
	updated, next := m.Update(<-done)
	m = updated.(Model)
	if next != nil || m.mode != modeChooseTarget || m.pendingDownload != nil {
		t.Fatal("old setup displaced new target picker")
	}
}

func TestQueueSetupResultsSurviveNavigation(t *testing.T) {
	for _, tc := range []struct {
		name      string
		stage     string
		failed    bool
		statError bool
		terminal  bool
	}{
		{name: "resolution-success", stage: "resolve"},
		{name: "resolution-error", stage: "resolve", failed: true},
		{name: "path-error", stage: "resolve", statError: true, failed: true},
		{name: "start-success", stage: "start"},
		{name: "start-error", stage: "start", failed: true},
		{name: "start-terminal", stage: "start", terminal: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, s := observedQueue(t)
			updated, _ := m.handleShortcutAction(actionBack)
			m = updated.(Model)
			updated, _ = m.handleShortcutAction(actionOpenImport)
			m = updated.(Model)
			if tc.failed || tc.terminal {
				kind := app.OutcomeFailed
				if tc.terminal {
					kind = app.OutcomeSuccess
				}
				s.snapshot.Items[0].Status = app.QueueItemTerminal
				s.snapshot.Items[0].Outcome = &app.ItemOutcome{TorrentID: "a", Kind: kind}
				s.snapshot.CurrentIndex = 1
				s.snapshot.Items[1].Status = app.QueueItemActive
			} else {
				s.snapshot.Items[0].Status = app.QueueItemResolving
				if tc.stage == "start" {
					s.snapshot.Items[0].Status = app.QueueItemActive
					s.snapshot.Download = &app.DownloadSnapshot{Ref: app.DownloadRef{Generation: 8, GID: "a"}, Download: models.ManagedDownload{GID: "a", Status: models.ManagedDownloadStatusActive}}
				}
			}
			m, next := updateQueue(m, s.snapshot)
			if m.mode != modeFileBrowser || m.returnMode != modeMain {
				t.Fatalf("queue result displaced browser navigation: mode=%s return=%s", m.mode, m.returnMode)
			}
			if tc.failed || tc.terminal {
				want := bulkFileFailed
				if tc.terminal {
					want = bulkFileSuccess
				}
				if m.bulk.Current != 1 || m.bulk.Items[0].Status != want || m.bulk.Items[1].Status != bulkFileActive || next == nil {
					t.Fatalf("queue result was not routed to its item: current=%d items=%+v", m.bulk.Current, m.bulk.Items)
				}
			} else if m.bulk.Current != 0 || next == nil || tc.stage == "start" && (m.download == nil || m.downloadRef != (app.DownloadRef{Generation: 8, GID: "a"})) {
				t.Fatal("queue start did not install and poll its active download")
			}
		})
	}
}

func queueOverwriteAfterNavigation(t *testing.T) (Model, *queueServiceSpy) {
	t.Helper()
	m, s := observedQueue(t)
	updated, _ := m.handleShortcutAction(actionBack)
	m = updated.(Model)
	updated, _ = m.handleShortcutAction(actionOpenImport)
	m = updated.(Model)
	s.snapshot.CurrentIndex = 1
	s.snapshot.Items[0].Status = app.QueueItemTerminal
	s.snapshot.Items[0].Outcome = &app.ItemOutcome{TorrentID: "a", Kind: app.OutcomeSuccess}
	s.snapshot.Items[1].Status = app.QueueItemAwaitingOverwrite
	s.snapshot.AwaitingOverwrite = true
	s.snapshot.ExistingPath = "/downloads/a.mkv"
	m, _ = updateQueue(m, s.snapshot)
	if m.mode != modeOverwrite || m.pendingDownload == nil || s.startCalls != 0 {
		t.Fatal("existing file was not held for confirmation")
	}
	return m, s
}

func TestQueueOverwriteRestoresNavigation(t *testing.T) {
	for _, action := range []string{"skip", "confirm"} {
		t.Run(action, func(t *testing.T) {
			m, service := queueOverwriteAfterNavigation(t)
			key := tea.KeyEscape
			if action == "confirm" {
				key = tea.KeyEnter
			}
			updated, cmd := m.Update(tea.KeyPressMsg{Code: key})
			m = updated.(Model)
			if m.mode != modeFileBrowser || m.returnMode != modeMain || m.pendingDownload != nil || cmd == nil {
				t.Fatalf("overwrite did not restore browser and continue queue: mode=%s return=%s current=%d", m.mode, m.returnMode, m.bulk.Current)
			}
			updated, _ = m.Update(cmd())
			m = updated.(Model)
			if m.mode != modeFileBrowser || m.bulk.Current != 1 || service.startCalls != 0 || service.decision != (app.QueueDecision{QueueID: 7, ItemIndex: 1, Overwrite: action == "confirm"}) {
				t.Fatal("overwrite decision lost identity/navigation or drove execution")
			}
		})
	}
}

func TestStaleQueueOverwriteDoesNotAffectReplacement(t *testing.T) {
	for _, key := range []rune{tea.KeyEscape, tea.KeyEnter} {
		t.Run(tea.KeyPressMsg{Code: key}.String(), func(t *testing.T) {
			m, service := queueOverwriteAfterNavigation(t)
			m.bulk.QueueID++
			_ = m.resolveDownload(models.DownloadTarget{Link: "replacement"})
			replacementContext, replacementGeneration := m.setup.ctx, m.setup.Generation
			updated, cmd := m.Update(tea.KeyPressMsg{Code: key})
			m = updated.(Model)
			if cmd != nil || m.mode != modeFileBrowser || m.returnMode != modeMain || m.pendingDownload != nil || m.bulk.Current != 1 || m.bulk.Items[1].Status != bulkFileActive || service.startCalls != 0 || service.decision.QueueID != 0 || replacementContext.Err() != nil || m.setup.Generation != replacementGeneration {
				t.Fatal("obsolete overwrite prompt affected replacement queue or navigation")
			}
		})
	}
}
