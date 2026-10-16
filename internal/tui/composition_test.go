package tui

import (
	"context"
	"errors"
	"slices"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/m4rii0/rdtui/internal/app"
	"github.com/m4rii0/rdtui/internal/auth"
	"github.com/m4rii0/rdtui/internal/realdebrid"
	"github.com/m4rii0/rdtui/pkg/models"
)

func TestCompositionRoutesAuthenticationRequired(t *testing.T) {
	err := errors.Join(auth.ErrNoValidCredentials, &realdebrid.APIError{StatusCode: 401})
	for name, msg := range map[string]tea.Msg{
		"list":         torrentsMsg{err: err},
		"detail":       detailMsg{id: "id", err: err},
		"add":          addTorrentMsg{err: err},
		"select":       selectFilesMsg{err: err},
		"delete":       deleteMsg{err: err},
		"resolve":      resolvedDownloadMsg{err: err},
		"handoff":      handoffMsg{err: err},
		"batch":        batchResultMsg{items: []app.ItemOutcome{{Err: err}}},
		"import":       importMsg{result: app.BatchResult{Items: []app.ItemOutcome{{Err: err}}}},
		"bulk select":  bulkFileSelectionResultMsg{items: []app.ItemOutcome{{Err: err}}},
		"bulk details": bulkFileSelectionDetailsMsg{err: err},
		"operation":    operationErrorMsg{err: err},
	} {
		t.Run(name, func(t *testing.T) {
			m := NewModel(Dependencies{Runtime: newTestRuntime(t)})
			m.mode, m.loading = modeMain, true
			m.session = &models.AuthSession{User: models.User{Username: "old"}}
			m.torrents = []models.Torrent{{ID: "id"}}
			updated, _ := m.Update(msg)
			got := updated.(Model)
			if got.mode != modeAuthChoice || got.session != nil || got.loading {
				t.Fatalf("not routed to authentication: %s, %+v", got.mode, got.session)
			}
		})
	}
}

func TestCompositionBulkDetailsPreservesAuthenticationCause(t *testing.T) {
	release := make(chan struct{})
	close(release)
	service := &asyncService{entered: make(chan context.Context, 2), release: release, resultErr: auth.ErrNoValidCredentials}
	runtime := newTestRuntime(t)
	msg := bulkFileSelectionDetailsCmd(t.Context(), runtime, service, []bulkFileSelectionPlan{{ID: "id"}})().(bulkFileSelectionDetailsMsg)
	if !errors.Is(msg.err, auth.ErrNoValidCredentials) {
		t.Fatalf("lost authentication cause: %+v", msg)
	}
}

func TestCompositionIgnoresStaleAuthenticationRejection(t *testing.T) {
	m := NewModel(Dependencies{Runtime: newTestRuntime(t)})
	m.mode = modeMain
	m.list.Generation = 2
	updated, _ := m.Update(featureResultMsg{feature: listFeature, request: requestID{1}, msg: torrentsMsg{err: auth.ErrNoValidCredentials}})
	if updated.(Model).mode != modeMain {
		t.Fatal("stale result replaced current screen")
	}
	updated, _ = m.Update(torrentsMsg{err: &realdebrid.APIError{StatusCode: 401}})
	if updated.(Model).mode != modeMain {
		t.Fatal("old-token API rejection invalidated new session")
	}
}

func TestCompositionQueueRejectionIsConsumedBeforeReauthentication(t *testing.T) {
	m, queue := observedQueue(t)
	m.session = &models.AuthSession{User: models.User{Username: "old"}}
	snapshot := queue.snapshot
	snapshot.Items = slices.Clone(snapshot.Items)
	snapshot.CurrentIndex = 1
	snapshot.Items[0].Status = app.QueueItemTerminal
	snapshot.Items[0].Outcome = &app.ItemOutcome{TorrentID: "a", Kind: app.OutcomeFailed,
		Err: errors.Join(auth.ErrNoValidCredentials, &realdebrid.APIError{StatusCode: 401})}
	snapshot.Items[1].Status = app.QueueItemActive
	m, _ = updateQueue(m, snapshot)
	if m.mode != modeAuthChoice || m.session != nil {
		t.Fatalf("fresh queue rejection did not request authentication: %s", m.mode)
	}
	if m.bulk.Snapshot.Items[0].Outcome == nil {
		t.Error("rejection snapshot was not retained for display and acknowledgement")
	}

	updated, _ := m.Update(authMsg{session: &models.AuthSession{User: models.User{Username: "replacement"}}})
	m = updated.(Model)
	updated, _ = m.handleDownloadAction()
	m = updated.(Model)
	if m.mode != modeBulkDownload {
		t.Fatal("could not reopen background queue after login")
	}
	queue.snapshot = snapshot
	updated, poll := m.handleShortcutAction(actionRefresh)
	m = updated.(Model)
	if poll == nil {
		t.Fatal("reopened queue did not poll")
	}
	updated, _ = m.Update(poll())
	m = updated.(Model)
	if m.mode != modeBulkDownload || m.session == nil || m.session.User.Username != "replacement" {
		t.Fatalf("historical queue rejection invalidated replacement session: mode=%s session=%+v", m.mode, m.session)
	}
	if outcome := m.bulk.Snapshot.Items[0].Outcome; outcome == nil || outcome.Kind != app.OutcomeFailed || !errors.Is(outcome.Err, auth.ErrNoValidCredentials) {
		t.Fatalf("historical failure was erased: %+v", outcome)
	}

	// Unseen outcomes need the authoritative owner's state; the composed
	// regression covers both pre-login failures and replacement rejection.
}

func TestCompositionStaleQueueRejectionIsNotConsumed(t *testing.T) {
	for _, stale := range []string{"queue", "request", "snapshot queue", "item", "download", "finished"} {
		t.Run(stale, func(t *testing.T) {
			m, queue := observedQueue(t)
			m.session = &models.AuthSession{User: models.User{Username: "replacement"}}
			snapshot := queue.snapshot
			snapshot.Items = slices.Clone(snapshot.Items)
			snapshot.Items[0].Outcome = &app.ItemOutcome{TorrentID: "a", Kind: app.OutcomeFailed, Err: auth.ErrNoValidCredentials}
			msg := bulkSnapshotMsg{queueID: 7, request: requestID{m.queuePoll.Generation}, snapshot: snapshot}
			switch stale {
			case "queue":
				msg.queueID = 6
			case "request":
				msg.request.Generation++
			case "snapshot queue":
				msg.snapshot.ID = 6
			case "item":
				m.bulk.Snapshot.CurrentIndex = 1
			case "download":
				m.bulk.Snapshot.Download = &app.DownloadSnapshot{Ref: app.DownloadRef{Generation: 2, GID: "current"}}
				msg.snapshot.Download = &app.DownloadSnapshot{Ref: app.DownloadRef{Generation: 1, GID: "old"}}
			case "finished":
				m.bulk.Snapshot.Finished = true
			}
			updated, cmd := m.Update(msg)
			m = updated.(Model)
			if cmd != nil || m.mode != modeBulkDownload || m.session == nil {
				t.Fatal("stale rejection changed current authentication/navigation")
			}
			if m.bulk.Snapshot.Items[0].Outcome != nil {
				t.Fatal("stale rejection snapshot was accepted")
			}
			// The same outcome must still be actionable when a valid current
			// observation arrives: ignoring a stale result must not acknowledge it.
			snapshot.CurrentIndex = m.bulk.Snapshot.CurrentIndex
			snapshot.Download = m.bulk.Snapshot.Download
			snapshot.Finished = m.bulk.Snapshot.Finished
			m, _ = updateQueue(m, snapshot)
			if m.mode != modeAuthChoice || m.session != nil {
				t.Fatal("stale result consumed a current rejection")
			}
		})
	}
}
