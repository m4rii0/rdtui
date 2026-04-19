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

type DownloadRequest struct {
	URL      string
	Dir      string
	Filename string
}

type rpcClient interface {
	AddURI([]string, *arigo.Options) (arigo.GID, error)
	TellStatus(string, ...string) (arigo.Status, error)
	GetVersion() (arigo.VersionInfo, error)
	Shutdown() error
	Close() error
}

type rpcDialFunc func(string, string) (rpcClient, error)

type process interface {
	Start() error
	Wait() error
	Kill() error
}

type processFactory func(string, []string) (process, error)

type Manager struct {
	mu sync.Mutex

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

func NewManager(binaryPath string) *Manager {
	return &Manager{
		binaryPath:          binaryPath,
		dial:                dialRPC,
		makeProcess:         newExecProcess,
		choosePort:          choosePort,
		makeSecret:          makeSecret,
		startupTimeout:      defaultStartupTimeout,
		startupPollInterval: defaultStartupPollInterval,
	}
}

func (m *Manager) SetBinaryPath(path string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.binaryPath = path
}

func (m *Manager) StartDownload(ctx context.Context, req DownloadRequest) (models.ManagedDownload, error) {
	if err := m.ensureStarted(ctx); err != nil {
		return models.ManagedDownload{}, err
	}

	m.mu.Lock()
	rpc := m.rpc
	m.mu.Unlock()
	if rpc == nil {
		return models.ManagedDownload{}, ErrNotRunning
	}

	gid, err := rpc.AddURI([]string{req.URL}, buildOptions(req))
	if err != nil {
		return models.ManagedDownload{}, fmt.Errorf("add download: %w", err)
	}

	status, err := rpc.TellStatus(gid.GID, statusKeys()...)
	if err != nil {
		return models.ManagedDownload{}, fmt.Errorf("fetch download status: %w", err)
	}

	download := mapStatus(status)
	download.URL = req.URL
	if download.Filename == "" {
		download.Filename = req.Filename
	}
	if download.Directory == "" {
		download.Directory = req.Dir
	}
	return download, nil
}

func (m *Manager) DownloadStatus(_ context.Context, gid string) (models.ManagedDownload, error) {
	m.mu.Lock()
	rpc := m.rpc
	lastExit := m.lastExit
	m.mu.Unlock()

	if rpc == nil {
		if lastExit != nil {
			return models.ManagedDownload{}, fmt.Errorf("%w: %v", ErrProcessExited, lastExit)
		}
		return models.ManagedDownload{}, ErrNotRunning
	}

	status, err := rpc.TellStatus(gid, statusKeys()...)
	if err != nil {
		return models.ManagedDownload{}, fmt.Errorf("fetch download status: %w", err)
	}
	return mapStatus(status), nil
}

func (m *Manager) Shutdown(ctx context.Context) error {
	m.mu.Lock()
	rpc := m.rpc
	proc := m.process
	waitDone := m.waitDone
	m.rpc = nil
	m.process = nil
	m.waitDone = nil
	m.mu.Unlock()

	if rpc == nil && proc == nil {
		return nil
	}

	var shutdownErr error
	if rpc != nil {
		shutdownErr = rpc.Shutdown()
		_ = rpc.Close()
	}

	if waitDone == nil {
		return shutdownErr
	}

	select {
	case err := <-waitDone:
		if shutdownErr != nil {
			return shutdownErr
		}
		if err != nil && !errors.Is(err, exec.ErrNotFound) {
			return err
		}
		return nil
	case <-ctx.Done():
		if proc != nil {
			_ = proc.Kill()
		}
		<-waitDone
		if shutdownErr != nil {
			return shutdownErr
		}
		return ctx.Err()
	}
}

func (m *Manager) ensureStarted(ctx context.Context) error {
	m.mu.Lock()
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
	for attempt := 0; attempt < startAttempts; attempt++ {
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
	if err := proc.Start(); err != nil {
		return fmt.Errorf("start aria2c: %w", err)
	}

	waitDone := make(chan error, 1)
	m.mu.Lock()
	m.process = proc
	m.waitDone = waitDone
	m.lastExit = nil
	m.mu.Unlock()

	go m.watchProcess(proc, waitDone)

	readyCtx, cancel := context.WithTimeout(ctx, m.startupTimeout)
	defer cancel()
	rpc, err := m.waitForReady(readyCtx, websocketURL(port), secret, waitDone)
	if err != nil {
		_ = proc.Kill()
		<-waitDone
		return err
	}

	m.mu.Lock()
	m.rpc = rpc
	m.mu.Unlock()
	return nil
}

func (m *Manager) watchProcess(proc process, waitDone chan error) {
	err := proc.Wait()
	waitDone <- err
	close(waitDone)

	m.mu.Lock()
	defer m.mu.Unlock()
	if m.process == proc {
		if m.rpc != nil {
			_ = m.rpc.Close()
			m.rpc = nil
		}
		m.process = nil
		m.waitDone = nil
		m.lastExit = err
	}
}

func (m *Manager) waitForReady(ctx context.Context, wsURL string, secret string, waitDone <-chan error) (rpcClient, error) {
	ticker := time.NewTicker(m.startupPollInterval)
	defer ticker.Stop()

	for {
		client, err := m.dial(wsURL, secret)
		if err == nil {
			if _, versionErr := client.GetVersion(); versionErr == nil {
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

func buildOptions(req DownloadRequest) *arigo.Options {
	return &arigo.Options{
		Dir:                    req.Dir,
		Out:                    req.Filename,
		Continue:               true,
		MaxConnectionPerServer: 16,
		Split:                  16,
		MinSplitSize:           1024 * 1024,
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
	defer listener.Close()
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

func dialRPC(url string, secret string) (rpcClient, error) {
	client, err := arigo.Dial(url, secret)
	if err != nil {
		return nil, err
	}
	return &rpcAdapter{client: &client}, nil
}

type execProcess struct {
	cmd *exec.Cmd
}

func newExecProcess(binaryPath string, args []string) (process, error) {
	cmd := exec.Command(binaryPath, args...)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	return &execProcess{cmd: cmd}, nil
}

func (p *execProcess) Start() error {
	return p.cmd.Start()
}

func (p *execProcess) Wait() error {
	return p.cmd.Wait()
}

func (p *execProcess) Kill() error {
	if p.cmd.Process == nil {
		return nil
	}
	return p.cmd.Process.Kill()
}

type rpcAdapter struct {
	client *arigo.Client
}

func (r *rpcAdapter) AddURI(uris []string, options *arigo.Options) (arigo.GID, error) {
	return r.client.AddURI(uris, options)
}

func (r *rpcAdapter) TellStatus(gid string, keys ...string) (arigo.Status, error) {
	return r.client.TellStatus(gid, keys...)
}

func (r *rpcAdapter) GetVersion() (arigo.VersionInfo, error) {
	return r.client.GetVersion()
}

func (r *rpcAdapter) Shutdown() error {
	return r.client.Shutdown()
}

func (r *rpcAdapter) Close() error {
	return r.client.Close()
}
