package app

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/m4rii0/rdtui/pkg/models"
)

type batchAPIStub struct {
	delete      func(context.Context, string) error
	link        func(context.Context, string) (models.UnrestrictedLink, error)
	selectFiles func(context.Context, string, []int) error
	add         func(context.Context, string) (models.AddTorrentResult, error)
}

func (f batchAPIStub) DeleteTorrent(ctx context.Context, id string) error { return f.delete(ctx, id) }
func (f batchAPIStub) UnrestrictLink(ctx context.Context, link string) (models.UnrestrictedLink, error) {
	return f.link(ctx, link)
}
func (f batchAPIStub) SelectFiles(ctx context.Context, id string, ids []int) error {
	return f.selectFiles(ctx, id, ids)
}
func (f batchAPIStub) AddTorrentPath(ctx context.Context, path string) (models.AddTorrentResult, error) {
	return f.add(ctx, path)
}

type batchDesktopStub struct{ copy func(string) (bool, error) }

func (f batchDesktopStub) CopyURL(url string) (bool, error) { return f.copy(url) }
func (batchDesktopStub) OpenFile(string) error              { return nil }
func (batchDesktopStub) RevealInDirectory(string) error     { return nil }

func noBatchWait(context.Context, time.Duration) error { return nil }

// Dropping duplicates, sorting inputs, or aborting on an item error loses planned outcomes.
func TestBatchPreservesOrderAndContinues(t *testing.T) {
	bad := errors.New("rejected")
	for _, operation := range []string{"delete", "select", "import"} {
		t.Run(operation, func(t *testing.T) {
			var calls []string
			var waits []time.Duration
			visit := func(id string) error {
				calls = append(calls, id)
				if id == "bad" {
					return bad
				}
				return nil
			}
			api := batchAPIStub{
				delete:      func(_ context.Context, id string) error { return visit(id) },
				selectFiles: func(_ context.Context, id string, _ []int) error { return visit(id) },
				add: func(_ context.Context, path string) (models.AddTorrentResult, error) {
					err := visit(path)
					return models.AddTorrentResult{ID: "new-" + path}, err
				},
			}
			service := NewBatchService(api, nil, func(_ context.Context, d time.Duration) error { waits = append(waits, d); return nil })
			var result BatchResult
			switch operation {
			case "delete":
				result = service.Delete(t.Context(), []string{"z", "bad", "z"})
			case "select":
				result = service.Select(t.Context(), []FileSelection{{TorrentID: "z"}, {TorrentID: "bad"}, {TorrentID: "z"}})
			case "import":
				result = service.Import(t.Context(), []string{"z", "bad", "z"})
			}
			if !reflect.DeepEqual(calls, []string{"z", "bad", "z"}) || len(result.Items) != 3 {
				t.Fatalf("calls=%v result=%+v", calls, result)
			}
			if result.Items[0].Kind != OutcomeSuccess || result.Items[1].Kind != OutcomeFailed || !errors.Is(result.Items[1].Err, bad) || result.Items[2].Kind != OutcomeSuccess {
				t.Fatal(result)
			}
			if operation == "import" {
				if len(waits) != 0 || result.Items[2].Source != "z" || result.Items[2].TorrentCreatedID != "new-z" {
					t.Fatal(result, waits)
				}
			} else if !reflect.DeepEqual(waits, []time.Duration{250 * time.Millisecond, 250 * time.Millisecond}) {
				t.Fatal(waits)
			}
		})
	}
}

func TestCopyCountsTargetsNotTorrents(t *testing.T) {
	var calls []string
	var waits []time.Duration
	var copied string
	api := batchAPIStub{link: func(_ context.Context, link string) (models.UnrestrictedLink, error) {
		calls = append(calls, link)
		if link == "bad" {
			return models.UnrestrictedLink{}, errors.New("bad link")
		}
		return models.UnrestrictedLink{Download: "same-url"}, nil
	}}
	svc := NewBatchService(api, batchDesktopStub{copy: func(s string) (bool, error) { copied = s; return true, nil }}, func(_ context.Context, d time.Duration) error { waits = append(waits, d); return nil })
	result := svc.CopyLinks(t.Context(), []LinkRequest{{TorrentID: "a", Target: models.DownloadTarget{Index: 4, Link: "one"}, Eligible: true}, {TorrentID: "skip"}, {TorrentID: "a", Target: models.DownloadTarget{Index: 7, Link: "bad"}, Eligible: true}, {TorrentID: "a", Target: models.DownloadTarget{Index: 2, Link: "two"}, Eligible: true}})
	if !reflect.DeepEqual(calls, []string{"one", "bad", "two"}) || copied != "same-url\nsame-url" || result.CopiedURLs != 2 || !reflect.DeepEqual(result.ResolvedURLs, []string{"same-url", "same-url"}) {
		t.Fatalf("%+v copied=%q calls=%v", result, copied, calls)
	}
	if len(result.Items) != 4 || result.Items[0].TargetIndex != 4 || result.Items[2].TargetIndex != 7 || result.Items[3].TargetIndex != 2 || result.Items[0].URL != "same-url" || result.Items[2].Source != "bad" {
		t.Fatal(result)
	}
	if got := SummarizeTorrents(result.Items); got != (TorrentSummary{Partial: 1, Skipped: 1}) {
		t.Fatal(got)
	}
	if !reflect.DeepEqual(waits, []time.Duration{250 * time.Millisecond, 250 * time.Millisecond}) {
		t.Fatal(waits)
	}
}

func TestClipboardFailureRetainsResolvedURLs(t *testing.T) {
	for _, unavailable := range []bool{false, true} {
		t.Run(map[bool]string{false: "error", true: "unavailable"}[unavailable], func(t *testing.T) {
			clipboardErr := errors.New("clipboard failed")
			svc := NewBatchService(batchAPIStub{link: func(context.Context, string) (models.UnrestrictedLink, error) {
				return models.UnrestrictedLink{Link: "url"}, nil
			}}, batchDesktopStub{copy: func(string) (bool, error) {
				if unavailable {
					return false, nil
				}
				return false, clipboardErr
			}}, noBatchWait)
			result := svc.CopyLinks(t.Context(), []LinkRequest{{TorrentID: "a", Eligible: true}, {TorrentID: "a", Eligible: true}})
			if len(result.ResolvedURLs) != 2 || result.CopiedURLs != 0 || result.ClipboardErr == nil || result.Items[0].Kind != OutcomeSuccess {
				t.Fatal(result)
			}
			if !unavailable && !errors.Is(result.ClipboardErr, clipboardErr) {
				t.Fatal(result)
			}
		})
	}
	t.Run("empty URL fails without clipboard", func(t *testing.T) {
		svc := NewBatchService(batchAPIStub{link: func(context.Context, string) (models.UnrestrictedLink, error) { return models.UnrestrictedLink{}, nil }}, nil, noBatchWait)
		result := svc.CopyLinks(t.Context(), []LinkRequest{{TorrentID: "a", Eligible: true}})
		if len(result.ResolvedURLs) != 0 || result.CopiedURLs != 0 || result.Items[0].Kind != OutcomeFailed || result.Items[0].Err == nil || SummarizeTorrents(result.Items).Failed != 1 {
			t.Fatal(result)
		}
	})
}

func TestBatchCancelStopsRequests(t *testing.T) {
	for _, operation := range []string{"delete", "select", "copy", "import"} {
		t.Run(operation, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			calls := 0
			clipboardCalls := 0
			visit := func() { calls++; cancel() }
			svc := NewBatchService(batchAPIStub{
				delete:      func(context.Context, string) error { visit(); return nil },
				selectFiles: func(context.Context, string, []int) error { visit(); return nil },
				link: func(context.Context, string) (models.UnrestrictedLink, error) {
					visit()
					return models.UnrestrictedLink{Download: "url"}, nil
				},
				add: func(context.Context, string) (models.AddTorrentResult, error) {
					visit()
					return models.AddTorrentResult{ID: "created"}, nil
				},
			}, batchDesktopStub{copy: func(string) (bool, error) { clipboardCalls++; return true, nil }}, noBatchWait)
			var result BatchResult
			switch operation {
			case "delete":
				result = svc.Delete(ctx, []string{"a", "b", "c"})
			case "select":
				result = svc.Select(ctx, []FileSelection{{TorrentID: "a"}, {TorrentID: "b"}, {TorrentID: "c"}})
			case "copy":
				result = svc.CopyLinks(ctx, []LinkRequest{{TorrentID: "a", Eligible: true}, {TorrentID: "b", Eligible: true}, {TorrentID: "c", Eligible: true}}).BatchResult
			case "import":
				result = svc.Import(ctx, []string{"a", "b", "c"})
			}
			if calls != 1 || len(result.Items) != 3 || !errors.Is(result.Err, context.Canceled) {
				t.Fatal(calls, result)
			}
			if clipboardCalls != 0 {
				t.Fatal("clipboard work started after cancellation")
			}
			if result.Items[0].Kind != OutcomeSuccess || result.Items[1].Kind != OutcomeCanceled || result.Items[2].Kind != OutcomeCanceled || result.Items[1].RemoteOutcomeUnknown {
				t.Fatal(result)
			}
		})
	}
	t.Run("during wait preserves earlier failure", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		calls := 0
		svc := NewBatchService(batchAPIStub{delete: func(context.Context, string) error { calls++; return errors.New("failure") }}, nil, func(ctx context.Context, _ time.Duration) error { cancel(); return ctx.Err() })
		result := svc.Delete(ctx, []string{"a", "b", "c"})
		if calls != 1 || len(result.Items) != 3 || result.Items[0].Kind != OutcomeFailed || result.Items[1].Kind != OutcomeCanceled || result.Items[2].Kind != OutcomeCanceled {
			t.Fatal(calls, result)
		}
	})
}

func TestBatchCopyCancellationAfterFinalResolution(t *testing.T) {
	for _, tc := range []struct {
		name  string
		links []string
	}{
		{name: "single item", links: []string{"one"}},
		{name: "final item", links: []string{"one", "two"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			calls, clipboardCalls := 0, 0
			service := NewBatchService(batchAPIStub{link: func(_ context.Context, link string) (models.UnrestrictedLink, error) {
				calls++
				if calls == len(tc.links) {
					cancel()
				}
				return models.UnrestrictedLink{Download: "url:" + link}, nil
			}}, batchDesktopStub{copy: func(string) (bool, error) {
				clipboardCalls++
				return true, nil
			}}, noBatchWait)
			requests := make([]LinkRequest, len(tc.links))
			for i, link := range tc.links {
				requests[i] = LinkRequest{TorrentID: "torrent", Target: models.DownloadTarget{Index: i, Link: link}, Eligible: true}
			}
			result := service.CopyLinks(ctx, requests)
			if clipboardCalls != 0 || result.CopiedURLs != 0 || !errors.Is(result.Err, context.Canceled) {
				t.Fatalf("clipboard calls=%d result=%+v", clipboardCalls, result)
			}
			if calls != len(tc.links) || len(result.Items) != len(tc.links) || len(result.ResolvedURLs) != len(tc.links) {
				t.Fatalf("calls=%d result=%+v", calls, result)
			}
			for i, item := range result.Items {
				if item.Kind != OutcomeSuccess || item.Err != nil || item.RemoteOutcomeUnknown || item.URL != "url:"+tc.links[i] || result.ResolvedURLs[i] != "url:"+tc.links[i] {
					t.Fatalf("lost successful resolution: %+v", result)
				}
			}
		})
	}
}

func TestCanceledMutationOutcomeUnknown(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	svc := NewBatchService(batchAPIStub{delete: func(context.Context, string) error { cancel(); return context.Canceled }}, nil, noBatchWait)
	result := svc.Delete(ctx, []string{"a", "b"})
	if len(result.Items) != 2 || result.Items[0].Kind != OutcomeCanceled || !result.Items[0].RemoteOutcomeUnknown || result.Items[1].RemoteOutcomeUnknown || !errors.Is(result.Err, context.Canceled) {
		t.Fatal(result)
	}
}

func TestSelectionCopiesNestedIDs(t *testing.T) {
	selections := []FileSelection{{TorrentID: "a", FileIDs: []int{7}}, {TorrentID: "b", FileIDs: []int{9}}}
	var received []int
	svc := NewBatchService(batchAPIStub{selectFiles: func(_ context.Context, id string, ids []int) error {
		if id == "a" {
			selections[1].FileIDs[0] = 99
		}
		received = append(received, ids[0])
		ids[0] = 100
		return nil
	}}, nil, noBatchWait)
	result := svc.Select(t.Context(), selections)
	if !reflect.DeepEqual(received, []int{7, 9}) || selections[0].FileIDs[0] != 7 || len(result.Items) != 2 {
		t.Fatal(received, selections, result)
	}
}

func TestBatchTorrentOutcomes(t *testing.T) {
	items := []ItemOutcome{
		{TorrentID: "success", Kind: OutcomeSuccess}, {TorrentID: "success", Kind: OutcomeSuccess},
		{TorrentID: "partial", Kind: OutcomeSuccess}, {TorrentID: "partial", Kind: OutcomeCanceled},
		{TorrentID: "failed", Kind: OutcomeSkipped}, {TorrentID: "failed", Kind: OutcomeFailed}, {TorrentID: "failed", Kind: OutcomeCanceled},
		{TorrentID: "skipped", Kind: OutcomeSkipped}, {TorrentID: "canceled", Kind: OutcomeSkipped}, {TorrentID: "canceled", Kind: OutcomeCanceled},
	}
	want := map[string]TorrentOutcomeKind{"success": "success", "partial": "partial", "failed": "failed", "skipped": "skipped", "canceled": "canceled"}
	if got := TorrentOutcomes(items); !reflect.DeepEqual(got, want) {
		t.Fatal(got)
	}
	if got := SummarizeTorrents(items); got != (TorrentSummary{Success: 1, Partial: 1, Failed: 1, Skipped: 1, Canceled: 1}) {
		t.Fatal(got)
	}
}
