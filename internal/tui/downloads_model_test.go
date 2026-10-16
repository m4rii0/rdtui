package tui

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/m4rii0/rdtui/internal/app"
	"github.com/m4rii0/rdtui/pkg/models"
)

// This fake deliberately has no authentication, configuration, or desktop API.
type downloadOnlyFake struct {
	url, filename string
	ref           app.DownloadRef
}

var _ DownloadService = (*downloadOnlyFake)(nil)

func (f *downloadOnlyFake) Start(_ context.Context, url, filename string) (app.DownloadStart, error) {
	f.url, f.filename = url, filename
	return app.DownloadStart{Snapshot: app.DownloadSnapshot{Ref: f.ref, Download: models.ManagedDownload{GID: f.ref.GID, Filename: filename, Status: models.ManagedDownloadStatusActive}}}, nil
}
func (f *downloadOnlyFake) Status(_ context.Context, ref app.DownloadRef) (app.DownloadSnapshot, error) {
	if ref != f.ref {
		return app.DownloadSnapshot{}, errors.New("wrong reference")
	}
	return app.DownloadSnapshot{Ref: ref, Download: models.ManagedDownload{GID: ref.GID, Filename: f.filename, Status: models.ManagedDownloadStatusComplete}}, nil
}
func (f *downloadOnlyFake) Current() (app.DownloadSnapshot, bool) {
	return app.DownloadSnapshot{}, false
}

// Missing download routing, bypassing preparation, or requiring a broad service
// breaks this complete prepare -> confirm -> start -> observe UI interaction.
func TestDownloadFeatureNeedsOnlyDownloadDependencies(t *testing.T) {
	f := &downloadOnlyFake{ref: app.DownloadRef{Generation: 1, GID: "download"}}
	m := NewModel(Dependencies{Downloads: f, Runtime: newTestRuntime(t), PrepareDownload: func(ctx context.Context, target models.DownloadTarget) (app.PreparedDownload, error) {
		if ctx.Err() != nil || target.Link != "link" {
			t.Fatal("wrong preparation input")
		}
		return app.PreparedDownload{URL: "resolved", Filename: "safe.mkv", Path: "/chosen/safe.mkv", Exists: true, ExistingBytes: 8, RemoteBytes: 100}, nil
	}})
	m.mode, m.returnMode = modeDetail, modeDetail
	cmd := m.resolveDownload(models.DownloadTarget{Link: "link"})
	updated, cmd := m.Update(cmd())
	m = updated.(Model)
	if cmd != nil || m.mode != modeOverwrite || m.pendingDownload.Path != "/chosen/safe.mkv" || m.pendingDownload.ExistingBytes != 8 || m.pendingDownload.RemoteBytes != 100 {
		t.Fatalf("preparation not presented: %+v", m.pendingDownload)
	}
	updated, cmd = m.handlePopupConfirm()
	m = updated.(Model)
	updated, cmd = m.Update(cmd())
	m = updated.(Model)
	if f.url != "resolved" || f.filename != "safe.mkv" || m.mode != modeDownload || !strings.Contains(renderView(m), "safe.mkv") || cmd == nil {
		t.Fatal("download was not started/presented")
	}
	updated, cmd = m.Update(cmd())
	m = updated.(Model)
	if !m.download.IsComplete() || cmd != nil || m.status != "Download complete" {
		t.Fatal("terminal observation was not routed")
	}
}

func TestManagedDownloadMsgEntersDownloadMode(t *testing.T) {
	m := Model{runtime: newTestRuntime(t), returnMode: modeDetail}

	updated, cmd := m.Update(managedDownloadMsg{result: models.ManagedDownloadStart{Download: models.ManagedDownload{GID: "gid-1", Filename: "movie.mkv", Status: models.ManagedDownloadStatusActive}}})
	m = updated.(Model)
	if m.mode != modeDownload {
		t.Fatalf("mode = %q, want modeDownload", m.mode)
	}
	if m.download == nil || m.download.Filename != "movie.mkv" {
		t.Fatalf("download = %+v", m.download)
	}
	if cmd == nil {
		t.Fatal("expected polling command after entering download mode")
	}
}

func TestManagedDownloadStatusMsgMarksCompletion(t *testing.T) {
	m := Model{runtime: newTestRuntime(t), mode: modeDownload, download: &models.ManagedDownload{GID: "gid-1", Filename: "movie.mkv", Status: models.ManagedDownloadStatusActive}}

	updated, _ := m.Update(currentDownloadResult(m, models.ManagedDownload{GID: "gid-1", Filename: "movie.mkv", Status: models.ManagedDownloadStatusComplete}, nil))
	m = updated.(Model)
	if m.download == nil || !m.download.IsComplete() {
		t.Fatalf("download = %+v", m.download)
	}
	if m.status != "Download complete" {
		t.Fatalf("status = %q", m.status)
	}
}

func TestHandoffKeepsResolvedURLWhenClipboardUnavailable(t *testing.T) {
	m := NewModel(Dependencies{Runtime: newTestRuntime(t)})
	msg := handoffCmd(t.Context(), m.runtime, testBatches(&batchCopyTestService{}), models.DownloadTarget{Link: "good"})().(handoffMsg)
	updated, _ := m.Update(msg)
	m = updated.(Model)
	if m.mode != modeShowURL || m.showURL != "url" || !strings.Contains(m.errText, "clipboard not available") {
		t.Fatalf("mode=%s url=%q err=%q", m.mode, m.showURL, m.errText)
	}
}
