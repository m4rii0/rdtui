package aria2

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/cenkalti/rpc2"
	"github.com/cenkalti/rpc2/jsonrpc"
	"github.com/gorilla/websocket"
	"github.com/m4rii0/rdtui/internal/download"
	"github.com/m4rii0/rdtui/pkg/models"
	"github.com/siku2/arigo"
)

var (
	ErrBinaryNotFound = errors.New("aria2c binary not found")
	ErrNotRunning     = errors.New("managed aria2 is not running")
	ErrProcessExited  = errors.New("managed aria2 process exited")
)

const (
	defaultStartupTimeout      = 5 * time.Second
	defaultStartupPollInterval = 100 * time.Millisecond
	startAttempts              = 3
)

type rpcClient interface {
	AddURI([]string, *arigo.Options) (arigo.GID, error)
	TellStatus(string, ...string) (arigo.Status, error)
	ForceRemove(string) error
	GetVersion() (arigo.VersionInfo, error)
	Shutdown() error
	Close() error
}

type rpcDialFunc func(context.Context, string, string) (rpcClient, error)

type process interface {
	Start() error
	Wait() error
	Kill() error
}

type processFactory func(string, []string) (process, error)

type Manager struct {
	mu        sync.Mutex
	startMu   sync.Mutex // Serializes process startup, never metadata access.
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	closing   bool
	done      chan struct{}
	readyRPC  rpcClient
	lost      bool
	transfers map[string]*transfer

	binaryPath string
	rpc        rpcClient
	process    process
	waitDone   chan error
	lastExit   error

	dial                rpcDialFunc
	makeProcess         processFactory
	choosePort          func() (int, error)
	makeSecret          func() (string, error)
	startupTimeout      time.Duration
	startupPollInterval time.Duration
}

type transfer struct {
	// Protected by Manager.mu, including publication of an AddURI result.
	gid      string
	adding   bool
	finished chan struct{}
	terminal bool
}

func NewManager(binaryPath string) *Manager {
	ctx, cancel := context.WithCancel(context.Background())
	return &Manager{
		ctx: ctx, cancel: cancel, done: make(chan struct{}),
		transfers:           make(map[string]*transfer),
		binaryPath:          binaryPath,
		dial:                dialRPC,
		makeProcess:         newExecProcess,
		choosePort:          choosePort,
		makeSecret:          makeSecret,
		startupTimeout:      defaultStartupTimeout,
		startupPollInterval: defaultStartupPollInterval,
	}
}

func (m *Manager) StartDownload(ctx context.Context, req models.ManagedDownloadRequest) (models.ManagedDownload, error) {
	if err := ctx.Err(); err != nil {
		return models.ManagedDownload{}, err
	}
	type result struct {
		download models.ManagedDownload
		err      error
	}
	resultCh := make(chan result, 1)
	startDone := make(chan struct{})
	startCtx, cancelStart := context.WithCancel(ctx)
	t := &transfer{finished: make(chan struct{})}
	m.mu.Lock()
	if m.closing {
		m.mu.Unlock()
		cancelStart()
		return models.ManagedDownload{}, download.ErrClosed
	}
	if err := ctx.Err(); err != nil {
		m.mu.Unlock()
		cancelStart()
		return models.ManagedDownload{}, err
	}
	// Both start and cancellation ownership are registered before closure can
	// observe the final task set. Neither worker performs I/O under mu.
	m.wg.Go(func() {
		defer cancelStart()
		d, err := m.startTransfer(startCtx, req, t)
		resultCh <- result{d, err}
		close(startDone)
	})
	m.wg.Go(func() {
		// In particular, manager cancellation must interrupt a startup dial,
		// which has no RPC connection available for Close yet.
		m.watchTransfer(ctx, t, startDone)
		if m.ctx.Err() != nil {
			cancelStart()
		}
	})
	m.mu.Unlock()
	res := <-resultCh
	return res.download, res.err
}

func (m *Manager) startTransfer(ctx context.Context, req models.ManagedDownloadRequest, t *transfer) (models.ManagedDownload, error) {
	if err := m.ensureStarted(ctx); err != nil {
		return models.ManagedDownload{}, err
	}

	m.mu.Lock()
	rpc := m.rpc
	if m.closing {
		m.mu.Unlock()
		return models.ManagedDownload{}, download.ErrClosed
	}
	if err := ctx.Err(); err != nil {
		m.mu.Unlock()
		return models.ManagedDownload{}, err
	}
	t.adding = rpc != nil
	m.mu.Unlock()
	if rpc == nil {
		return models.ManagedDownload{}, ErrNotRunning
	}

	gid, err := rpc.AddURI([]string{req.URL}, buildOptions(req))
	m.mu.Lock()
	t.adding = false
	t.gid = gid.GID
	if gid.GID != "" {
		m.transfers[gid.GID] = t
	}
	m.mu.Unlock()
	if gid.GID == "" {
		if _, rejected := errors.AsType[rpc2.ServerError](err); rejected {
			return models.ManagedDownload{}, fmt.Errorf("add download: %w", err)
		}
		// Once AddURI was attempted, a transport error cannot prove rejection.
		m.closeAndInterrupt()
		return models.ManagedDownload{}, fmt.Errorf("%w: %v", download.ErrUncertainStart, err)
	}

	// Admission already succeeded. Observation failure must never lose its GID.
	snapshot := models.ManagedDownload{GID: gid.GID, URL: req.URL, Filename: req.Filename,
		Directory: req.Dir, FilePath: filepath.Join(req.Dir, req.Filename), Status: models.ManagedDownloadStatusWaiting}
	if observed, err := m.DownloadStatus(ctx, gid.GID); err == nil {
		observed.GID = gid.GID
		observed.URL = req.URL
		if observed.Filename == "" {
			observed.Filename = req.Filename
		}
		if observed.Directory == "" {
			observed.Directory = req.Dir
		}
		if observed.FilePath == "" {
			observed.FilePath = snapshot.FilePath
		}
		snapshot = observed
	}
	return snapshot, nil
}

func (m *Manager) watchTransfer(ctx context.Context, t *transfer, startDone <-chan struct{}) {
	select {
	case <-ctx.Done():
	case <-m.ctx.Done():
		return
	case <-t.finished:
		return
	case <-startDone:
		m.mu.Lock()
		admitted := t.gid != ""
		m.mu.Unlock()
		if !admitted {
			return
		}
		select {
		case <-ctx.Done():
		case <-m.ctx.Done():
			return
		case <-t.finished:
			return
		}
	}
	m.mu.Lock()
	gid, adding, rpc := t.gid, t.adding, m.rpc
	closing := m.closing
	m.mu.Unlock()
	if closing {
		return
	}
	if adding {
		// Cancellation during an unacknowledged add is itself uncertain.
		m.closeAndInterrupt()
		return
	}
	if gid == "" {
		// Startup honors ctx; the canceled admission check prevents a later add.
		select {
		case <-startDone:
		case <-m.ctx.Done():
		}
		return
	}
	if rpc == nil {
		return
	} // The process watcher confirms backend loss.
	removeErr := rpc.ForceRemove(gid)
	for {
		status, err := rpc.TellStatus(gid, statusKeys()...)
		if err == nil && terminalStatus(mapStatus(status)) {
			m.finishTransfer(gid)
			return
		}
		if err != nil || removeErr != nil {
			// A failed remove/confirmation cannot authorize an orphaned transfer.
			m.closeAndInterrupt()
			return
		}
		timer := time.NewTimer(time.Second)
		select {
		case <-m.ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func terminalStatus(d models.ManagedDownload) bool {
	return d.IsComplete() || d.IsError() || d.Status == models.ManagedDownloadStatus("removed")
}

func (m *Manager) finishTransfer(gid string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if t := m.transfers[gid]; t != nil && !t.terminal {
		t.terminal = true
		close(t.finished)
	}
}

func (m *Manager) DownloadStatus(ctx context.Context, gid string) (models.ManagedDownload, error) {
	if err := ctx.Err(); err != nil {
		return models.ManagedDownload{}, err
	}
	type result struct {
		download models.ManagedDownload
		err      error
	}
	results := make(chan result, 1)
	m.mu.Lock()
	rpc := m.rpc
	lastExit := m.lastExit
	lost := m.lost
	if rpc == nil {
		m.mu.Unlock()
		if lost {
			return models.ManagedDownload{}, fmt.Errorf("%w: %w: %v", download.ErrBackendLost, ErrProcessExited, lastExit)
		}
		return models.ManagedDownload{}, ErrNotRunning
	}
	if m.closing {
		m.mu.Unlock()
		return models.ManagedDownload{}, download.ErrClosed
	}
	m.wg.Go(func() {
		status, err := rpc.TellStatus(gid, statusKeys()...)
		d := mapStatus(status)
		if err != nil {
			err = fmt.Errorf("fetch download status: %w", err)
		} else if terminalStatus(d) {
			m.finishTransfer(gid)
		}
		results <- result{d, err}
	})
	m.mu.Unlock()
	select {
	case res := <-results:
		return res.download, res.err
	case <-ctx.Done():
		return models.ManagedDownload{}, ctx.Err()
	case <-m.ctx.Done():
		return models.ManagedDownload{}, download.ErrClosed
	}
}

func (m *Manager) Shutdown(ctx context.Context) error {
	m.mu.Lock()
	m.beginShutdownLocked()
	m.mu.Unlock()
	select {
	case <-m.done:
		return nil
	default:
	}
	select {
	case <-m.done:
		return nil
	case <-ctx.Done():
		m.interrupt()
		return ctx.Err()
	}
}

// Called only under mu; every worker is registered before the one shared waiter.
func (m *Manager) beginShutdownLocked() {
	if m.closing {
		return
	}
	m.closing = true
	m.cancel()
	rpc, ready, proc, waitDone := m.rpc, m.readyRPC, m.process, m.waitDone
	m.wg.Go(func() {
		// This process is exclusively owned by the application. A graceful RPC
		// races transport interruption and can itself block, so stop the process
		// directly rather than waiting out the unrelated startup timeout.
		if proc != nil {
			_ = proc.Kill()
		}
		// Closing the transport interrupts add/status/readiness/removal RPCs.
		if rpc != nil {
			_ = rpc.Close()
		}
		if ready != nil {
			_ = ready.Close()
		}
		if waitDone != nil {
			<-waitDone
		}
	})
	go func() { m.wg.Wait(); close(m.done) }()
}

func (m *Manager) closeAndInterrupt() {
	m.mu.Lock()
	m.beginShutdownLocked()
	m.mu.Unlock()
	m.interrupt()
}

func (m *Manager) interrupt() {
	m.mu.Lock()
	rpc, ready, proc := m.rpc, m.readyRPC, m.process
	m.mu.Unlock()
	// Backend termination must not depend on any transport cleanup succeeding.
	if proc != nil {
		_ = proc.Kill()
	}
	if rpc != nil {
		_ = rpc.Close()
	}
	if ready != nil {
		_ = ready.Close()
	}
}

func (m *Manager) ensureStarted(ctx context.Context) error {
	m.startMu.Lock()
	defer m.startMu.Unlock()
	m.mu.Lock()
	if m.closing {
		m.mu.Unlock()
		return download.ErrClosed
	}
	if err := ctx.Err(); err != nil {
		m.mu.Unlock()
		return err
	}
	if m.lost {
		m.mu.Unlock()
		return download.ErrBackendLost
	}
	if m.rpc != nil && m.process != nil {
		m.mu.Unlock()
		return nil
	}
	binaryPath := m.binaryPath
	m.mu.Unlock()

	resolved, err := resolveBinaryPath(binaryPath)
	if err != nil {
		return err
	}

	var lastErr error
	for range startAttempts {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := m.ctx.Err(); err != nil {
			return download.ErrClosed
		}
		if err := m.startOnce(ctx, resolved); err == nil {
			return nil
		} else {
			lastErr = err
		}
	}
	return lastErr
}

func (m *Manager) startOnce(ctx context.Context, binaryPath string) error {
	port, err := m.choosePort()
	if err != nil {
		return fmt.Errorf("choose rpc port: %w", err)
	}
	secret, err := m.makeSecret()
	if err != nil {
		return fmt.Errorf("generate rpc secret: %w", err)
	}

	proc, err := m.makeProcess(binaryPath, buildCommandArgs(port, secret))
	if err != nil {
		return fmt.Errorf("build aria2 process: %w", err)
	}
	waitDone := make(chan error, 1)
	started := make(chan error, 1)
	m.mu.Lock()
	if m.closing {
		m.mu.Unlock()
		return download.ErrClosed
	}
	m.process = proc
	m.waitDone = waitDone
	m.lastExit = nil
	m.wg.Go(func() {
		if err := <-started; err != nil {
			waitDone <- err
			close(waitDone)
			return
		}
		m.watchProcess(proc, waitDone)
	})
	m.mu.Unlock()
	startErr := proc.Start()
	started <- startErr
	if startErr != nil {
		return fmt.Errorf("start aria2c: %w", startErr)
	}
	if ctx.Err() != nil || m.ctx.Err() != nil {
		_ = proc.Kill()
		<-waitDone
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return download.ErrClosed
	}

	readyCtx, cancel := context.WithTimeout(ctx, m.startupTimeout)
	defer cancel()
	rpc, err := m.waitForReady(readyCtx, websocketURL(port), secret, waitDone)
	if err != nil {
		_ = proc.Kill()
		<-waitDone
		return err
	}

	m.mu.Lock()
	if m.closing || m.process != proc {
		m.mu.Unlock()
		_ = rpc.Close()
		_ = proc.Kill()
		<-waitDone
		return download.ErrClosed
	}
	m.rpc = rpc
	m.mu.Unlock()
	return nil
}

func (m *Manager) watchProcess(proc process, waitDone chan error) {
	err := proc.Wait()
	var rpc rpcClient
	m.mu.Lock()
	if m.process == proc {
		rpc = m.rpc
		if rpc != nil {
			m.lost = true
		}
		m.rpc = nil
		m.process = nil
		m.waitDone = nil
		m.lastExit = err
	}
	m.mu.Unlock()
	if rpc != nil {
		_ = rpc.Close()
	}
	waitDone <- err
	close(waitDone)
}

func (m *Manager) waitForReady(ctx context.Context, wsURL string, secret string, waitDone <-chan error) (rpcClient, error) {
	ticker := time.NewTicker(m.startupPollInterval)
	defer ticker.Stop()

	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		client, err := m.dial(ctx, wsURL, secret)
		if err == nil {
			checked, watcherDone := make(chan struct{}), make(chan struct{})
			m.mu.Lock()
			if m.closing {
				m.mu.Unlock()
				_ = client.Close()
				return nil, download.ErrClosed
			}
			m.readyRPC = client
			m.wg.Go(func() {
				defer close(watcherDone)
				select {
				case <-ctx.Done():
					_ = client.Close()
				case <-m.ctx.Done():
					_ = client.Close()
				case <-checked:
				}
			})
			m.mu.Unlock()
			_, versionErr := client.GetVersion()
			close(checked)
			<-watcherDone
			m.mu.Lock()
			m.readyRPC = nil
			m.mu.Unlock()
			if versionErr == nil && ctx.Err() == nil && m.ctx.Err() == nil {
				return client, nil
			}
			_ = client.Close()
		}

		select {
		case err := <-waitDone:
			if err == nil {
				err = ErrProcessExited
			}
			return nil, fmt.Errorf("wait for aria2 readiness: %w", err)
		case <-ctx.Done():
			return nil, fmt.Errorf("wait for aria2 readiness: %w", ctx.Err())
		case <-m.ctx.Done():
			return nil, download.ErrClosed
		case <-ticker.C:
		}
	}
}

func buildCommandArgs(port int, secret string) []string {
	return []string{
		"--enable-rpc=true",
		"--rpc-listen-all=false",
		fmt.Sprintf("--rpc-listen-port=%d", port),
		"--rpc-secret=" + secret,
		"--no-conf=true",
		"--quiet=true",
		"--download-result=hide",
		"--console-log-level=error",
	}
}

func buildOptions(req models.ManagedDownloadRequest) *arigo.Options {
	return &arigo.Options{
		Dir:                    req.Dir,
		Out:                    req.Filename,
		Continue:               true,
		MaxConnectionPerServer: 16,
		Split:                  16,
		MinSplitSize:           "1M",
	}
}

func statusKeys() []string {
	return []string{
		"gid",
		"status",
		"totalLength",
		"completedLength",
		"downloadSpeed",
		"connections",
		"errorMessage",
		"dir",
		"files",
	}
}

func mapStatus(status arigo.Status) models.ManagedDownload {
	download := models.ManagedDownload{
		GID:             status.GID,
		Status:          models.ManagedDownloadStatus(string(status.Status)),
		TotalLength:     int64(status.TotalLength),
		CompletedLength: int64(status.CompletedLength),
		DownloadSpeed:   int64(status.DownloadSpeed),
		Connections:     int(status.Connections),
		ErrorMessage:    status.ErrorMessage,
		Directory:       status.Dir,
	}
	if len(status.Files) == 0 {
		return download
	}

	file := status.Files[0]
	download.FilePath = file.Path
	if download.Directory == "" && file.Path != "" {
		download.Directory = filepath.Dir(file.Path)
	}
	if file.Path != "" {
		download.Filename = filepath.Base(file.Path)
	}
	return download
}

func websocketURL(port int) string {
	return fmt.Sprintf("ws://127.0.0.1:%d/jsonrpc", port)
}

func resolveBinaryPath(binaryPath string) (string, error) {
	if binaryPath == "" {
		binaryPath = "aria2c"
	}
	resolved, err := exec.LookPath(binaryPath)
	if err != nil {
		return "", fmt.Errorf("%w: %s", ErrBinaryNotFound, binaryPath)
	}
	return resolved, nil
}

func choosePort() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer func() { _ = listener.Close() }()
	addr, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		return 0, errors.New("unexpected tcp address")
	}
	return addr.Port, nil
}

func makeSecret() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func dialRPC(ctx context.Context, url string, secret string) (rpcClient, error) {
	conn, _, err := (&websocket.Dialer{}).DialContext(ctx, url, nil)
	if err != nil {
		return nil, err
	}
	return newRPCAdapter(conn, secret), nil
}

func newRPCAdapter(conn *websocket.Conn, secret string) *rpcAdapter {
	// Keep arigo's RPC methods and the existing JSON codec, but not its wsrpc
	// wrapper: that wrapper flushes a mutable message writer from Close, racing
	// active writes and potentially returning without ever closing the socket.
	codec := jsonrpc.NewJSONCodec(&websocketStream{conn: conn})
	client := arigo.NewClient(rpc2.NewClientWithCodec(codec), secret)
	adapter := &rpcAdapter{client: client}
	adapter.wg.Go(client.Run)
	return adapter
}

// jsonrpc serializes writes; rpc2 runs one reader. Close touches neither's
// state, only the immutable connection, whose Close supports concurrent I/O.
type websocketStream struct {
	conn   *websocket.Conn
	reader io.Reader
}

func (s *websocketStream) Read(p []byte) (int, error) {
	for {
		if s.reader == nil {
			_, reader, err := s.conn.NextReader()
			if err != nil {
				return 0, err
			}
			s.reader = reader
		}
		n, err := s.reader.Read(p)
		if errors.Is(err, io.EOF) {
			s.reader = nil
			if n == 0 {
				continue
			}
			return n, nil
		}
		return n, err
	}
}

func (s *websocketStream) Write(p []byte) (int, error) {
	if err := s.conn.WriteMessage(websocket.TextMessage, p); err != nil {
		return 0, err
	}
	return len(p), nil
}

func (s *websocketStream) Close() error { return s.conn.Close() }

type execProcess struct {
	cmd *exec.Cmd
	mu  sync.Mutex // Protects Process publication by Start against an early Kill.
}

func newExecProcess(binaryPath string, args []string) (process, error) {
	cmd := exec.Command(binaryPath, args...)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	return &execProcess{cmd: cmd}, nil
}

func (p *execProcess) Start() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.cmd.Start()
}

func (p *execProcess) Wait() error {
	return p.cmd.Wait()
}

func (p *execProcess) Kill() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cmd.Process == nil {
		return nil
	}
	return p.cmd.Process.Kill()
}

type rpcAdapter struct {
	client    *arigo.Client
	closeOnce sync.Once
	closeErr  error
	wg        sync.WaitGroup
}

func (r *rpcAdapter) AddURI(uris []string, options *arigo.Options) (arigo.GID, error) {
	return r.client.AddURI(uris, options)
}

func (r *rpcAdapter) TellStatus(gid string, keys ...string) (arigo.Status, error) {
	return r.client.TellStatus(gid, keys...)
}

func (r *rpcAdapter) ForceRemove(gid string) error { return r.client.ForceRemove(gid) }

func (r *rpcAdapter) GetVersion() (arigo.VersionInfo, error) {
	return r.client.GetVersion()
}

func (r *rpcAdapter) Shutdown() error {
	return r.client.Shutdown()
}

func (r *rpcAdapter) Close() error {
	r.closeOnce.Do(func() {
		r.closeErr = r.client.Close()
		r.wg.Wait()
	})
	return r.closeErr
}
