package app

import (
	"context"
	"errors"
	"testing"

	"github.com/m4rii0/rdtui/pkg/models"
)

func TestAcquireQueueRefreshesStandaloneStatus(t *testing.T) {
	for _, status := range []models.ManagedDownloadStatus{
		models.ManagedDownloadStatusComplete,
		models.ManagedDownloadStatusError,
		models.ManagedDownloadStatusActive,
	} {
		t.Run(string(status), func(t *testing.T) {
			observations := 0
			backend := &fakeDownloadManager{
				startDownload: func(context.Context, models.ManagedDownloadRequest) (models.ManagedDownload, error) {
					return models.ManagedDownload{GID: "single", Status: models.ManagedDownloadStatusActive}, nil
				},
				downloadStatus: func(context.Context, string) (models.ManagedDownload, error) {
					observations++
					return models.ManagedDownload{GID: "single", Status: status}, nil
				},
			}
			c := serviceWithBackend(t, backend)
			started, err := c.Start(t.Context(), "https://example.com/single", "single")
			if err != nil {
				t.Fatal(err)
			}
			lease, err := c.AcquireQueue(t.Context())
			if status == models.ManagedDownloadStatusActive {
				if !errors.Is(err, ErrActiveDownload) {
					t.Fatalf("active transfer admitted queue: %v", err)
				}
			} else {
				if err != nil {
					t.Fatalf("unobserved terminal transfer blocked queue: %v", err)
				}
				if err := c.ReleaseQueue(lease); err != nil {
					t.Fatal(err)
				}
			}
			current, _ := c.Current()
			if observations != 1 || current.Ref != started.Snapshot.Ref || backend.startCalls.Load() != 1 {
				t.Fatalf("incorrect observation/ownership: calls=%d current=%+v", observations, current)
			}
		})
	}
}

func TestAcquireQueueRetainsTransferAfterObservationFailure(t *testing.T) {
	failure := true
	backend := &fakeDownloadManager{
		startDownload: func(context.Context, models.ManagedDownloadRequest) (models.ManagedDownload, error) {
			return models.ManagedDownload{GID: "single", Status: models.ManagedDownloadStatusActive}, nil
		},
		downloadStatus: func(context.Context, string) (models.ManagedDownload, error) {
			if failure {
				return models.ManagedDownload{}, errors.New("temporary status failure")
			}
			return models.ManagedDownload{GID: "single", Status: models.ManagedDownloadStatusComplete}, nil
		},
	}
	c := serviceWithBackend(t, backend)
	started, err := c.Start(t.Context(), "https://example.com/single", "single")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.AcquireQueue(t.Context()); !errors.Is(err, ErrActiveDownload) {
		t.Fatalf("status error freed admission: %v", err)
	}
	current, _ := c.Current()
	if current != started.Snapshot {
		t.Fatalf("observation failure corrupted transfer: %+v", current)
	}
	failure = false
	lease, err := c.AcquireQueue(t.Context())
	if err != nil {
		t.Fatalf("recovered observation still blocks queue: %v", err)
	}
	if err := c.ReleaseQueue(lease); err != nil {
		t.Fatal(err)
	}
}

func TestAcquireQueueCancellationDoesNotCancelTransfer(t *testing.T) {
	b := newCoordinatorBackend()
	c := testCoordinator(t, b)
	current := admit(t, c, b, "single")
	ctx, cancel := context.WithCancel(t.Context())
	result := make(chan error, 1)
	go func() { _, err := c.AcquireQueue(ctx); result <- err }()
	query := receive(t, b.statuses)
	cancel()
	if err := receive(t, result); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled queue admission: %v", err)
	}
	if query.ctx.Err() != nil {
		t.Fatal("queue waiter canceled application-owned observation")
	}
	// An in-flight observation must not block reopening the same transfer.
	reopened := receive(t, asyncStart(c, t.Context()))
	if reopened.err != nil || !reopened.start.Reused || reopened.start.Snapshot.Ref != current.Ref {
		t.Fatalf("canceled queue admission lost active transfer: %+v", reopened)
	}
	query.reply <- backendResult{download: models.ManagedDownload{Status: models.ManagedDownloadStatusActive}}
}

func TestConcurrentQueueAdmissionSharesStandaloneObservation(t *testing.T) {
	b := newCoordinatorBackend()
	c := testCoordinator(t, b)
	admit(t, c, b, "single")
	type result struct {
		lease QueueLease
		err   error
	}
	results := make(chan result, 2)
	acquire := func(ctx context.Context) {
		lease, err := c.AcquireQueue(ctx)
		results <- result{lease, err}
	}
	go acquire(t.Context())
	query := receive(t, b.statuses)
	waiter := observeWait(t.Context())
	go acquire(waiter)
	receive(t, waiter.waiting)
	query.reply <- backendResult{download: models.ManagedDownload{Status: models.ManagedDownloadStatusComplete}}
	first, second := receive(t, results), receive(t, results)
	if first.err != nil {
		first, second = second, first
	}
	if first.err != nil || first.lease.ID == 0 || !errors.Is(second.err, ErrQueueActive) || b.statusCalls.Load() != 1 {
		t.Fatalf("queue admission did not revalidate/shared observation: %+v %+v calls=%d", first, second, b.statusCalls.Load())
	}
	if err := c.ReleaseQueue(first.lease); err != nil {
		t.Fatal(err)
	}
}
