package tui

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"charm.land/bubbles/v2/progress"
	tea "charm.land/bubbletea/v2"
	"github.com/m4rii0/rdtui/internal/app"
	"github.com/m4rii0/rdtui/pkg/models"
)

type DownloadService interface {
	Start(context.Context, string, string) (app.DownloadStart, error)
	Status(context.Context, app.DownloadRef) (app.DownloadSnapshot, error)
	Current() (app.DownloadSnapshot, bool)
}

type DesktopService interface {
	CopyURL(string) (bool, error)
	OpenFile(string) error
	RevealInDirectory(string) error
}

type downloadsModel struct {
	downloads         DownloadService
	desktop           DesktopService
	prepareDownload   func(context.Context, models.DownloadTarget) (app.PreparedDownload, error)
	bulkQueue         BulkQueueService
	queuePoll         asyncRequest
	downloadPoll      asyncRequest
	downloadRef       app.DownloadRef
	queueGeneration   uint64
	progress          progress.Model
	targets           targetPickerState
	pendingDownload   *pendingDownloadState
	showURL           string
	download          *models.ManagedDownload
	downloadTorrentID string
	observationFailed bool
	bulk              *bulkDownloadState
}

func (d *downloadsModel) update(m *Model, msg tea.Msg) (tea.Model, tea.Cmd) {
	// Preparation and start results update the screen beneath quit, never its
	// input owner. Canceling quit reveals any pending overwrite separately.
	if m.mode == modeQuitConfirm {
		switch msg.(type) {
		case resolvedDownloadMsg, managedDownloadMsg:
			m.mode = m.quitReturnMode
			updated, cmd := d.update(m, msg)
			result := updated.(Model)
			result.quitReturnMode, result.mode = result.mode, modeQuitConfirm
			return result, cmd
		}
	}
	switch msg := msg.(type) {
	case bulkDetailsMsg:
		m.loading = false
		if msg.err != nil {
			m.errText = msg.err.Error()
			return *m, nil
		}
		if d.bulk == nil {
			return *m, nil
		}
		d.bulk.applyDetails(msg.details)
		if d.bulk.prepareFirstFilePrompt() {
			m.mode = modeBulkFiles
			m.errText = ""
			return *m, nil
		}
		d.bulk.buildQueueItems()
		m.mode = modeBulkConfirm
		m.errText = ""
	case handoffMsg:
		m.loading = false
		if msg.err != nil && msg.result.URL == "" {
			m.errText = msg.err.Error()
			return *m, nil
		}
		d.showURL = msg.result.URL
		m.mode = modeShowURL
		m.errText = ""
		if msg.err != nil {
			m.errText = msg.err.Error()
		}
		flashMsg := "Direct URL ready"
		if msg.result.Copied {
			flashMsg = "Direct URL copied to clipboard"
		}
		return *m, m.setFlash(flashSuccess, flashMsg)
	case bulkStartedMsg:
		return m.handleBulkStarted(msg)
	case bulkSnapshotMsg:
		return m.handleBulkSnapshot(msg)
	case bulkTickMsg:
		if d.bulk == nil || msg.queueID != d.bulk.QueueID || !d.queuePoll.current(msg.request) {
			return *m, nil
		}
		cmd := m.pollBulkQueue()
		return *m, cmd
	case bulkDecisionMsg:
		if d.bulk == nil || msg.decision.QueueID != d.bulk.QueueID || msg.decision.ItemIndex != d.bulk.Current {
			return *m, nil
		}
		if msg.err != nil && !errors.Is(msg.err, app.ErrStaleQueue) {
			m.errText = msg.err.Error()
		}
	case downloadTickMsg:
		if d.bulk != nil && d.bulk.QueueID != 0 && !d.bulk.isFinished() {
			return *m, nil
		}
		if !d.downloadPoll.current(msg.request.Request) || msg.request.Ref != d.downloadRef {
			return *m, nil
		}
		if d.download == nil || d.download.IsTerminal() {
			return *m, nil
		}
		cmd := m.pollDownload()
		return *m, cmd
	case managedDownloadMsg:
		d.observationFailed = false
		d.downloadPoll.invalidate()
		d.downloadRef = msg.ref
		m.loading = false
		if msg.err != nil {
			if errors.Is(msg.err, app.ErrQueueActive) && d.bulk != nil {
				m.mode = modeBulkDownload
				return *m, nil
			}
			m.errText = msg.err.Error()
			return *m, nil
		}
		d.download = new(msg.result.Download)
		m.mode = modeDownload
		m.errText = ""
		if msg.result.Reused {
			m.status = "Reopened active download: " + d.download.Filename
		} else {
			m.status = "Started download: " + d.download.Filename
		}
		if d.download.IsTerminal() {
			return *m, nil
		}
		cmd := m.pollDownload()
		return *m, cmd
	case resolvedDownloadMsg:
		m.loading = false
		if msg.err != nil {
			m.errText = msg.err.Error()
			return *m, nil
		}
		p := msg.prepared
		if p.Exists {
			d.pendingDownload = &pendingDownloadState{ResumeMode: m.mode, URL: p.URL, Filename: p.Filename, Path: p.Path, RemoteBytes: p.RemoteBytes, ExistingBytes: p.ExistingBytes}
			m.mode = modeOverwrite
			m.errText = ""
			m.status = "Existing file found"
			return *m, nil
		}
		return m.startPendingDownload(p.URL, p.Filename)
	case managedDownloadStatusMsg:
		if d.bulk != nil && d.bulk.QueueID != 0 && !d.bulk.isFinished() {
			return *m, nil
		}
		if !d.downloadPoll.current(msg.request.Request) || msg.request.Ref != d.downloadRef || msg.queue != m.queueIdentity() {
			return *m, nil
		}
		d.downloadPoll.invalidate()
		m.loading = false
		if !msg.ok || (msg.err != nil && !msg.download.IsTerminal()) {
			d.observationFailed = true
			if d.download != nil && !d.download.IsTerminal() {
				return *m, m.nextDownloadPoll()
			}
			return *m, nil
		}
		d.download = new(msg.download)
		d.observationFailed = false
		if msg.err != nil || d.download.IsError() {
			m.status = "Managed download failed"
			m.errText = ""
			return *m, nil
		}
		if d.download.IsComplete() {
			m.status = "Download complete"
			m.errText = ""
			return *m, nil
		}
		return *m, m.nextDownloadPoll()
	case downloadPathMsg:
		m.loading = false
		if msg.err != nil {
			m.errText = msg.err.Error()
			return *m, nil
		}
		m.errText = ""
		if msg.action == "open" {
			m.status = "Opened downloaded file"
		} else {
			m.status = "Opened download directory"
		}
	case progress.FrameMsg:
		var cmd tea.Cmd
		d.progress, cmd = d.progress.Update(msg)
		return *m, cmd
	}
	return *m, nil
}

func (m *Model) startPendingDownload(url string, filename string) (tea.Model, tea.Cmd) {
	m.pendingDownload = nil
	m.loading = true
	cmd := m.beginFeature(setupFeature, func(ctx context.Context) tea.Cmd {
		return startManagedDownloadCmd(ctx, m.runtime, m.downloads, url, filename)
	})
	return *m, cmd
}

func handoffCmd(ctx context.Context, runtime *app.Runtime, service BatchService, target models.DownloadTarget) tea.Cmd {
	return operationCmd(ctx, runtime, func(ctx context.Context) tea.Msg {
		result := service.CopyLinks(ctx, []app.LinkRequest{{Target: target, Eligible: true}})
		if len(result.ResolvedURLs) == 0 {
			if len(result.Items) > 0 {
				return handoffMsg{err: result.Items[0].Err}
			}
			return handoffMsg{err: result.Err}
		}
		return handoffMsg{result: models.HandoffResult{URL: result.ResolvedURLs[0], Copied: result.CopiedURLs == 1}, err: result.ClipboardErr}
	})
}

func resolveDownloadCmd(ctx context.Context, runtime *app.Runtime, prepare func(context.Context, models.DownloadTarget) (app.PreparedDownload, error), target models.DownloadTarget) tea.Cmd {
	return operationCmd(ctx, runtime, func(ctx context.Context) tea.Msg {
		prepared, err := prepare(ctx, target)
		return resolvedDownloadMsg{prepared: prepared, err: err}
	})
}

func startManagedDownloadCmd(ctx context.Context, runtime *app.Runtime, service DownloadService, url string, filename string) tea.Cmd {
	return operationCmd(ctx, runtime, func(ctx context.Context) tea.Msg {
		result, err := service.Start(ctx, url, filename)
		return managedDownloadMsg{ref: result.Snapshot.Ref, result: models.ManagedDownloadStart{Download: result.Snapshot.Download, Reused: result.Reused}, err: err}
	})
}

func openDownloadCmd(ctx context.Context, runtime *app.Runtime, service DesktopService, path string) tea.Cmd {
	return operationCmd(ctx, runtime, func(ctx context.Context) tea.Msg {
		return downloadPathMsg{action: "open", err: service.OpenFile(path)}
	})
}

func revealDownloadCmd(ctx context.Context, runtime *app.Runtime, service DesktopService, path string) tea.Cmd {
	return operationCmd(ctx, runtime, func(ctx context.Context) tea.Msg {
		return downloadPathMsg{action: "reveal", err: service.RevealInDirectory(path)}
	})
}

func bulkDetailsCmd(ctx context.Context, runtime *app.Runtime, service TorrentService, plans []bulkTorrentPlan) tea.Cmd {
	if len(plans) == 0 {
		return nil
	}
	ordered := slices.Clone(plans)
	return operationCmd(ctx, runtime, func(ctx context.Context) tea.Msg {
		details := make([]models.TorrentInfo, 0, len(ordered))
		for _, plan := range ordered {
			if err := ctx.Err(); err != nil {
				return bulkDetailsMsg{err: err}
			}
			info, err := service.TorrentInfo(ctx, plan.ID)
			if err != nil {
				return bulkDetailsMsg{err: fmt.Errorf("load %s details: %w", plan.Name, err)}
			}
			details = append(details, info)
		}
		return bulkDetailsMsg{details: details}
	})
}
