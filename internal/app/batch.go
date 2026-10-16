package app

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/m4rii0/rdtui/pkg/models"
)

type BatchAPI interface {
	DeleteTorrent(context.Context, string) error
	UnrestrictLink(context.Context, string) (models.UnrestrictedLink, error)
	SelectFiles(context.Context, string, []int) error
	AddTorrentPath(context.Context, string) (models.AddTorrentResult, error)
}

type Desktop interface {
	CopyURL(string) (bool, error)
	OpenFile(string) error
	RevealInDirectory(string) error
}

type WaitFunc func(context.Context, time.Duration) error

type OutcomeKind string

const (
	OutcomeSuccess  OutcomeKind = "success"
	OutcomeFailed   OutcomeKind = "failed"
	OutcomeSkipped  OutcomeKind = "skipped"
	OutcomeCanceled OutcomeKind = "canceled"
)

type ItemOutcome struct {
	TorrentID            string
	TargetIndex          int
	Source               string
	URL                  string
	TorrentCreatedID     string
	Kind                 OutcomeKind
	Err                  error
	RemoteOutcomeUnknown bool
}

type BatchResult struct {
	Items []ItemOutcome
	// Err reports interruption of the batch; individual failures remain in Items.
	Err error
}

type LinkBatchResult struct {
	BatchResult
	ResolvedURLs []string
	CopiedURLs   int
	ClipboardErr error
}

type LinkRequest struct {
	TorrentID string
	Target    models.DownloadTarget
	Eligible  bool
}

type FileSelection struct {
	TorrentID string
	FileIDs   []int
}

type BatchService struct {
	api     BatchAPI
	desktop Desktop
	wait    WaitFunc
}

func NewBatchService(api BatchAPI, desktop Desktop, wait WaitFunc) *BatchService {
	if wait == nil {
		wait = waitBatch
	}
	return &BatchService{api: api, desktop: desktop, wait: wait}
}

func waitBatch(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// run owns terminal outcomes. Only attempted eligible requests participate in
// throttling; cancellation leaves completed work intact and accounts for all rest.
func (s *BatchService) run(ctx context.Context, items []ItemOutcome, throttled bool, execute func(int, *ItemOutcome) error) BatchResult {
	result := BatchResult{Items: items}
	attempted := false
	var stopped error
	for i := range result.Items {
		item := &result.Items[i]
		if stopped == nil {
			stopped = ctx.Err()
		}
		if stopped != nil {
			item.Kind, item.Err = OutcomeCanceled, stopped
			continue
		}
		if item.Kind == OutcomeSkipped {
			continue
		}
		if throttled && attempted {
			stopped = s.wait(ctx, 250*time.Millisecond)
			if stopped == nil {
				stopped = ctx.Err()
			}
			if stopped != nil {
				item.Kind, item.Err = OutcomeCanceled, stopped
				continue
			}
		}
		attempted = true
		err := execute(i, item)
		item.Err = err
		switch {
		case err == nil:
			item.Kind = OutcomeSuccess
		case ctx.Err() != nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded):
			item.Kind, item.RemoteOutcomeUnknown = OutcomeCanceled, true
			stopped = ctx.Err()
			if stopped == nil {
				stopped = err
			}
		default:
			item.Kind = OutcomeFailed
		}
	}
	result.Err = stopped
	return result
}

func (s *BatchService) Delete(ctx context.Context, ids []string) BatchResult {
	ids = slices.Clone(ids)
	items := make([]ItemOutcome, len(ids))
	for i, id := range ids {
		items[i] = ItemOutcome{TorrentID: id}
	}
	return s.run(ctx, items, true, func(i int, _ *ItemOutcome) error { return s.api.DeleteTorrent(ctx, ids[i]) })
}

func (s *BatchService) Select(ctx context.Context, selections []FileSelection) BatchResult {
	selections = slices.Clone(selections)
	items := make([]ItemOutcome, len(selections))
	for i := range selections {
		selections[i].FileIDs = slices.Clone(selections[i].FileIDs)
		items[i] = ItemOutcome{TorrentID: selections[i].TorrentID}
	}
	return s.run(ctx, items, true, func(i int, _ *ItemOutcome) error {
		return s.api.SelectFiles(ctx, selections[i].TorrentID, selections[i].FileIDs)
	})
}

func (s *BatchService) Import(ctx context.Context, paths []string) BatchResult {
	paths = slices.Clone(paths)
	items := make([]ItemOutcome, len(paths))
	for i, path := range paths {
		items[i] = ItemOutcome{Source: path}
	}
	return s.run(ctx, items, false, func(i int, item *ItemOutcome) error {
		added, err := s.api.AddTorrentPath(ctx, paths[i])
		if err == nil {
			item.TorrentCreatedID = added.ID
		}
		return err
	})
}

func (s *BatchService) CopyLinks(ctx context.Context, requests []LinkRequest) LinkBatchResult {
	requests = slices.Clone(requests)
	items := make([]ItemOutcome, len(requests))
	for i, request := range requests {
		items[i] = ItemOutcome{TorrentID: request.TorrentID, TargetIndex: request.Target.Index, Source: request.Target.Link}
		if !request.Eligible {
			items[i].Kind = OutcomeSkipped
		}
	}
	result := LinkBatchResult{BatchResult: s.run(ctx, items, true, func(i int, item *ItemOutcome) error {
		resolved, err := s.api.UnrestrictLink(ctx, requests[i].Target.Link)
		if err != nil {
			return err
		}
		item.URL = resolved.Download
		if item.URL == "" {
			item.URL = resolved.Link
		}
		if item.URL == "" {
			return errors.New("resolved link is empty")
		}
		return nil
	})}
	for _, item := range result.Items {
		if item.Kind == OutcomeSuccess {
			result.ResolvedURLs = append(result.ResolvedURLs, item.URL)
		}
	}
	if len(result.ResolvedURLs) == 0 || result.Err != nil {
		return result
	}
	if s.desktop == nil {
		result.ClipboardErr = errors.New("clipboard not available")
		return result
	}
	copied, err := s.desktop.CopyURL(strings.Join(result.ResolvedURLs, "\n"))
	result.ClipboardErr = err
	if err == nil && copied {
		result.CopiedURLs = len(result.ResolvedURLs)
	} else if err == nil {
		result.ClipboardErr = errors.New("clipboard not available")
	}
	return result
}

type TorrentOutcomeKind string

const (
	TorrentOutcomeSuccess  TorrentOutcomeKind = "success"
	TorrentOutcomePartial  TorrentOutcomeKind = "partial"
	TorrentOutcomeFailed   TorrentOutcomeKind = "failed"
	TorrentOutcomeSkipped  TorrentOutcomeKind = "skipped"
	TorrentOutcomeCanceled TorrentOutcomeKind = "canceled"
)

type TorrentSummary struct{ Success, Partial, Failed, Skipped, Canceled int }

func TorrentOutcomes(items []ItemOutcome) map[string]TorrentOutcomeKind {
	type counts struct{ total, success, failed, skipped int }
	groups := make(map[string]counts)
	for _, item := range items {
		if item.TorrentID == "" {
			continue
		}
		count := groups[item.TorrentID]
		count.total++
		switch item.Kind {
		case OutcomeSuccess:
			count.success++
		case OutcomeFailed:
			count.failed++
		case OutcomeSkipped:
			count.skipped++
		}
		groups[item.TorrentID] = count
	}
	out := make(map[string]TorrentOutcomeKind, len(groups))
	for id, count := range groups {
		switch {
		case count.success == count.total:
			out[id] = TorrentOutcomeSuccess
		case count.success > 0:
			out[id] = TorrentOutcomePartial
		case count.failed > 0:
			out[id] = TorrentOutcomeFailed
		case count.skipped == count.total:
			out[id] = TorrentOutcomeSkipped
		default:
			out[id] = TorrentOutcomeCanceled
		}
	}
	return out
}

func SummarizeTorrents(items []ItemOutcome) TorrentSummary {
	var summary TorrentSummary
	for _, kind := range TorrentOutcomes(items) {
		switch kind {
		case TorrentOutcomeSuccess:
			summary.Success++
		case TorrentOutcomePartial:
			summary.Partial++
		case TorrentOutcomeFailed:
			summary.Failed++
		case TorrentOutcomeSkipped:
			summary.Skipped++
		case TorrentOutcomeCanceled:
			summary.Canceled++
		}
	}
	return summary
}
