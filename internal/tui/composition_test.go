package tui

import (
	"context"
	"errors"
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
