package demo

import (
	"context"
	"errors"
	"runtime"
	"testing"
	"time"

	"github.com/m4rii0/rdtui/internal/app"
	"github.com/m4rii0/rdtui/internal/tui"
	"github.com/m4rii0/rdtui/pkg/models"
)

func TestDemoSatisfiesFeatureContracts(t *testing.T) {
	s := NewService(t.Context())
	r := app.NewRuntime(t.Context())
	deps := tui.Dependencies{Auth: s, Torrents: s, Downloads: s, Desktop: s, Runtime: r,
		Batches: s.NewBatches(), Bulk: s.NewBulkQueue(r), PrepareDownload: s.PrepareDownload}
	t.Cleanup(func() {
		r.Stop()
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := s.Close(ctx); err != nil {
			t.Error(err)
		}
		if err := r.Wait(ctx); err != nil {
			t.Error(err)
		}
	})
	session, err := deps.Auth.Bootstrap(t.Context())
	if err != nil || session.Token != "" {
		t.Fatalf("display session: %+v %v", session, err)
	}
	torrents, err := deps.Torrents.ListTorrents(t.Context())
	if err != nil || len(torrents) == 0 {
		t.Fatalf("list: %v %v", torrents, err)
	}
	prepared, err := deps.PrepareDownload(t.Context(), models.DownloadTarget{Link: "demo", Label: "file.iso"})
	if err != nil || prepared.Filename != "file.iso" {
		t.Fatalf("prepare: %+v %v", prepared, err)
	}
	started, err := deps.Downloads.Start(t.Context(), prepared.URL, prepared.Filename)
	if err != nil {
		t.Fatal(err)
	}
	current, ok := deps.Downloads.Current()
	if !ok || current.Ref != started.Snapshot.Ref {
		t.Fatal("download identity not shared")
	}
	if _, err := deps.Bulk.Start(t.Context(), app.BulkPlan{}); !errors.Is(err, app.ErrActiveDownload) {
		t.Fatal(err)
	}
}

func TestDemoBulkQueueUsesSharedAdmissionAndRunsWithoutUI(t *testing.T) {
	s := NewService(t.Context())
	r := app.NewRuntime(t.Context())
	t.Cleanup(func() {
		r.Stop()
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := s.Close(ctx); err != nil {
			t.Error(err)
		}
		if err := r.Wait(ctx); err != nil {
			t.Error(err)
		}
	})
	s.backend.downloadDuration = time.Hour
	started, err := s.Start(t.Context(), "single", "single")
	if err != nil {
		t.Fatal(err)
	}
	q := s.NewBulkQueue(r)
	if _, err := q.Start(t.Context(), app.BulkPlan{}); !errors.Is(err, app.ErrActiveDownload) {
		t.Fatalf("queue reused unrelated single: %v", err)
	}
	current, ok := s.Current()
	if !ok || current.Ref != started.Snapshot.Ref {
		t.Fatal("missing active transfer")
	}
	// Complete the offline backend before retrying admission.
	s.backend.mu.Lock()
	s.backend.downloadDuration = time.Nanosecond
	s.backend.mu.Unlock()
	if _, err := s.Status(t.Context(), started.Snapshot.Ref); err != nil {
		t.Fatal(err)
	}
	id, err := q.Start(t.Context(), app.BulkPlan{Items: []app.QueueItem{
		{TorrentID: "a", Target: models.DownloadTarget{Link: "a", Label: "a.iso"}},
		{TorrentID: "b", Target: models.DownloadTarget{Link: "b", Label: "b.iso"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	for {
		snapshot, err := q.Snapshot(id)
		if err != nil {
			t.Fatal(err)
		}
		if snapshot.Finished {
			for _, item := range snapshot.Items {
				if item.Outcome == nil || item.Outcome.Kind != app.OutcomeSuccess {
					t.Fatalf("outcome: %+v", item)
				}
			}
			break
		}
		if ctx.Err() != nil {
			t.Fatal("demo queue did not progress without UI")
		}
		runtime.Gosched()
	}
}

func TestAddedTorrentFilenames(t *testing.T) {
	for _, tt := range []struct{ method, input, want string }{
		{"magnet", "magnet:?xt=hash", "New Magnet Torrent"},
		{"magnet", "magnet:?dn=My+Movie&tr=tracker", "My Movie"},
		{"magnet", "magnet:?dn=My+Movie", "My Movie"},
		{"magnet", "magnet:?dn=&tr=tracker", ""},
		{"url", "https://example.com/file.torrent", "file.torrent"},
		{"url", "https://example.com/dir/", "https://example.com/dir/"},
		{"url", "file.torrent", "file.torrent"},
		{"file", "/dir/file.torrent", "file"},
		{"file", "file.torrent", "file"},
		{"file", "/dir/", ""},
	} {
		t.Run(tt.method+"/"+tt.input, func(t *testing.T) {
			s := &Service{}
			var added models.AddTorrentResult
			var err error
			switch tt.method {
			case "magnet":
				added, err = s.AddMagnet(t.Context(), tt.input)
			case "url":
				added, err = s.AddTorrentURL(t.Context(), tt.input)
			case "file":
				added, err = s.AddTorrentPath(t.Context(), tt.input)
			}
			if err != nil {
				t.Fatal(err)
			}
			info, err := s.TorrentInfo(t.Context(), added.ID)
			if err != nil {
				t.Fatal(err)
			}
			if info.Filename != tt.want {
				t.Errorf("filename = %q, want %q", info.Filename, tt.want)
			}
		})
	}
}
