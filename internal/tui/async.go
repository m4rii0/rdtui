package tui

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/m4rii0/rdtui/internal/app"
	"github.com/m4rii0/rdtui/pkg/models"
)

type requestID struct{ Generation uint64 }
type downloadRequestID struct {
	Request requestID
	Ref     app.DownloadRef
}

type asyncRequest struct {
	Generation uint64
	InFlight   bool
	ctx        context.Context
	cancel     context.CancelFunc
}

func (r *asyncRequest) invalidate() {
	if r.cancel != nil {
		r.cancel()
	}
	r.ctx = nil
	r.cancel = nil
	r.Generation++
	r.InFlight = false
}

func (r *asyncRequest) begin(parent context.Context) (context.Context, requestID) {
	r.invalidate()
	r.ctx, r.cancel = context.WithCancel(parent)
	r.InFlight = true
	return r.ctx, requestID{r.Generation}
}

func (r *asyncRequest) current(id requestID) bool {
	return id.Generation == r.Generation && (r.ctx == nil || r.ctx.Err() == nil)
}

type asyncFeature uint8

const (
	authFeature asyncFeature = iota
	listFeature
	setupFeature
)

type queueIdentity struct {
	ID   uint64
	Item int
}

type featureResultMsg struct {
	feature asyncFeature
	request requestID
	queue   queueIdentity
	msg     tea.Msg
}

func featureCmd(cmd tea.Cmd, feature asyncFeature, request requestID, queue queueIdentity) tea.Cmd {
	if cmd == nil {
		return nil
	}
	return func() tea.Msg { return featureResultMsg{feature, request, queue, cmd()} }
}

func (m *Model) feature(feature asyncFeature) *asyncRequest {
	switch feature {
	case authFeature:
		return &m.auth
	case listFeature:
		return &m.list
	default:
		return &m.setup
	}
}

func (m *Model) queueIdentity() queueIdentity {
	if m.bulk != nil {
		return queueIdentity{m.queueGeneration, m.bulk.Current}
	}
	return queueIdentity{}
}

func (m *Model) beginFeature(feature asyncFeature, makeCmd func(context.Context) tea.Cmd) tea.Cmd {
	ctx, request := m.feature(feature).begin(m.runtime.Context())
	return featureCmd(makeCmd(ctx), feature, request, m.queueIdentity())
}

func (m *Model) refresh() tea.Cmd {
	if m.list.InFlight || m.runtime.Context().Err() != nil {
		return nil
	}
	return m.beginFeature(listFeature, func(ctx context.Context) tea.Cmd { return refreshCmd(ctx, m.runtime, m.service) })
}

func (m *Model) resolveDownload(target models.DownloadTarget) tea.Cmd {
	return m.beginFeature(setupFeature, func(ctx context.Context) tea.Cmd { return resolveDownloadCmd(ctx, m.runtime, m.service, target) })
}

func (m *Model) pollDevice() tea.Cmd {
	if m.auth.InFlight || m.deviceCode == nil {
		return nil
	}
	return m.beginFeature(authFeature, func(ctx context.Context) tea.Cmd {
		return pollDeviceCmd(ctx, m.runtime, m.service, *m.deviceCode)
	})
}

func (m *Model) requestDetail(id string) tea.Cmd {
	ctx, request := m.detailRequest.begin(m.runtime.Context())
	return detailCmd(ctx, runtimeReader{m.runtime, m.service}, id, request)
}

func (m *Model) pollDownload() tea.Cmd {
	if m.downloadPoll.InFlight || m.download == nil || m.download.IsTerminal() || m.runtime.Context().Err() != nil {
		return nil
	}
	ctx, request := m.downloadPoll.begin(m.runtime.Context())
	cmd := downloadStatusCmd(ctx, runtimeReader{m.runtime, m.service}, downloadRequestID{request, m.downloadRef})
	queue := m.queueIdentity()
	return func() tea.Msg {
		msg := cmd().(managedDownloadStatusMsg)
		msg.queue = queue
		return msg
	}
}

// Injecting the scheduler lets tests deliver ticks without waiting on wall time.
func (m Model) after(delay time.Duration, msg tea.Msg) tea.Cmd {
	if m.runtime.Context().Err() != nil {
		return nil
	}
	if m.schedule != nil {
		return m.schedule(delay, msg)
	}
	return tea.Tick(delay, func(time.Time) tea.Msg { return msg })
}

func (m Model) nextListPoll() tea.Cmd {
	return m.after(refreshInterval, tickMsg{requestID{m.list.Generation}})
}
func (m Model) nextDownloadPoll() tea.Cmd {
	return m.after(downloadRefreshInterval, downloadTickMsg{downloadRequestID{requestID{m.downloadPoll.Generation}, m.downloadRef}})
}

type detailReader interface {
	TorrentInfo(context.Context, string) (models.TorrentInfo, error)
}
type downloadReader interface {
	Status(context.Context, app.DownloadRef) (app.DownloadSnapshot, error)
}

type runtimeReader struct {
	runtime *app.Runtime
	service app.AppService
}

func (r runtimeReader) TorrentInfo(ctx context.Context, id string) (info models.TorrentInfo, err error) {
	err = r.runtime.Do(ctx, func(ctx context.Context) error { info, err = r.service.TorrentInfo(ctx, id); return err })
	return info, err
}
func (r runtimeReader) Status(ctx context.Context, ref app.DownloadRef) (snapshot app.DownloadSnapshot, err error) {
	err = r.runtime.Do(ctx, func(ctx context.Context) error { snapshot, err = r.service.Status(ctx, ref); return err })
	return snapshot, err
}

func detailCmd(ctx context.Context, service detailReader, id string, request requestID) tea.Cmd {
	if id == "" {
		return nil
	}
	return func() tea.Msg {
		info, err := service.TorrentInfo(ctx, id)
		return detailMsg{request: request, id: id, info: info, err: err}
	}
}

func downloadStatusCmd(ctx context.Context, service downloadReader, request downloadRequestID) tea.Cmd {
	return func() tea.Msg {
		snapshot, err := service.Status(ctx, request.Ref)
		return managedDownloadStatusMsg{request: request, download: snapshot.Download, ok: snapshot.Ref == request.Ref, err: err}
	}
}
