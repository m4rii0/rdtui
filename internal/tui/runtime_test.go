package tui

import (
	"context"
	"errors"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/m4rii0/rdtui/internal/app"
	"github.com/m4rii0/rdtui/pkg/models"
)

func newTestRuntime(t *testing.T) *app.Runtime {
	t.Helper()
	runtime := app.NewRuntime(t.Context())
	t.Cleanup(func() {
		runtime.Stop()
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := runtime.Wait(ctx); err != nil {
			t.Error(err)
		}
	})
	return runtime
}

func TestLeavingDownloadDoesNotStopRuntime(t *testing.T) {
	runtime := newTestRuntime(t)
	m := NewModel(Dependencies{Runtime: runtime})
	m.mode = modeDownload
	m.download = &models.ManagedDownload{Status: models.ManagedDownloadStatusActive}
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if updated.(Model).mode != modeMain {
		t.Fatal("download screen did not return to list")
	}
	if err := runtime.Do(t.Context(), func(context.Context) error { return nil }); err != nil {
		t.Fatal(err)
	}
}

type batchCancelService struct {
	testBatchAPI
	DesktopService
	cancel context.CancelFunc
	ids    []string
}

func (s *batchCancelService) DeleteTorrent(_ context.Context, id string) error {
	s.ids = append(s.ids, id)
	s.cancel()
	return nil
}

func (s *batchCancelService) SelectFiles(_ context.Context, id string, _ []int) error {
	s.ids = append(s.ids, id)
	s.cancel()
	return nil
}

func TestCanceledBatchSendsNoFurtherRequests(t *testing.T) {
	for _, kind := range []string{"delete", "file selection"} {
		t.Run(kind, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			runtime := newTestRuntime(t)
			service := &batchCancelService{cancel: cancel}
			var resultErr error
			if kind == "delete" {
				result := batchOpCmd(ctx, runtime, testBatches(service), batchOpDelete, []string{"first", "second"}, nil)().(batchResultMsg)
				resultErr = result.err
				if result.success != 1 {
					t.Fatal(result)
				}
			} else {
				result := bulkFileSelectionSubmitCmd(ctx, runtime, testBatches(service), []bulkFileSelectionSubmission{{ID: "first", FileIDs: []int{1}}, {ID: "second", FileIDs: []int{2}}}, nil)().(bulkFileSelectionResultMsg)
				resultErr = result.err
				if result.success != 1 {
					t.Fatal(result)
				}
			}
			if !errors.Is(resultErr, context.Canceled) {
				t.Fatal(resultErr)
			}
			if len(service.ids) != 1 || service.ids[0] != "first" {
				t.Fatal(service.ids)
			}
		})
	}
}
