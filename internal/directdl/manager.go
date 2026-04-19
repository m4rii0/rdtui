package directdl

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/m4rii0/rdtui/pkg/models"
)

type Manager struct {
	mu        sync.Mutex
	client    *http.Client
	downloads map[string]*downloadState
	nextID    uint64
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
}

type downloadState struct {
	mu       sync.RWMutex
	download models.ManagedDownload
	cancel   context.CancelFunc
}

func NewManager() *Manager {
	ctx, cancel := context.WithCancel(context.Background())
	return &Manager{
		client:    &http.Client{Timeout: 0},
		downloads: map[string]*downloadState{},
		ctx:       ctx,
		cancel:    cancel,
	}
}

func (m *Manager) SetBinaryPath(string) {}

func (m *Manager) StartDownload(_ context.Context, req models.ManagedDownloadRequest) (models.ManagedDownload, error) {
	if err := os.MkdirAll(req.Dir, 0o755); err != nil {
		return models.ManagedDownload{}, fmt.Errorf("ensure download dir: %w", err)
	}

	gid := fmt.Sprintf("direct-%d", atomic.AddUint64(&m.nextID, 1))
	ctx, cancel := context.WithCancel(m.ctx)
	state := &downloadState{
		download: models.ManagedDownload{
			GID:       gid,
			URL:       req.URL,
			Filename:  req.Filename,
			Status:    models.ManagedDownloadStatusWaiting,
			Directory: req.Dir,
			FilePath:  filepath.Join(req.Dir, req.Filename),
		},
		cancel: cancel,
	}

	m.mu.Lock()
	m.downloads[gid] = state
	m.mu.Unlock()

	m.wg.Add(1)
	go m.runDownload(ctx, state)

	return state.snapshot(), nil
}

func (m *Manager) DownloadStatus(_ context.Context, gid string) (models.ManagedDownload, error) {
	m.mu.Lock()
	state := m.downloads[gid]
	m.mu.Unlock()
	if state == nil {
		return models.ManagedDownload{}, fmt.Errorf("download not found: %s", gid)
	}
	return state.snapshot(), nil
}

func (m *Manager) Shutdown(ctx context.Context) error {
	m.cancel()

	done := make(chan struct{})
	go func() {
		m.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (m *Manager) runDownload(ctx context.Context, state *downloadState) {
	defer m.wg.Done()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, state.snapshot().URL, nil)
	if err != nil {
		state.fail(err)
		return
	}

	resp, err := m.client.Do(req)
	if err != nil {
		state.fail(err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		state.fail(fmt.Errorf("download request failed: %s", resp.Status))
		return
	}

	filePath := state.snapshot().FilePath
	file, err := os.Create(filePath)
	if err != nil {
		state.fail(fmt.Errorf("create download file: %w", err))
		return
	}
	defer file.Close()

	state.update(func(download *models.ManagedDownload) {
		download.Status = models.ManagedDownloadStatusActive
		if resp.ContentLength > 0 {
			download.TotalLength = resp.ContentLength
		}
	})

	buf := make([]byte, 32*1024)
	windowStart := time.Now()
	windowBytes := int64(0)
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, err := file.Write(buf[:n]); err != nil {
				state.fail(fmt.Errorf("write download file: %w", err))
				return
			}
			windowBytes += int64(n)
			now := time.Now()
			elapsed := now.Sub(windowStart)
			state.update(func(download *models.ManagedDownload) {
				download.CompletedLength += int64(n)
				if elapsed > 0 {
					download.DownloadSpeed = int64(float64(windowBytes) / elapsed.Seconds())
				}
			})
			if elapsed >= time.Second {
				windowStart = now
				windowBytes = 0
			}
		}
		if readErr == nil {
			continue
		}
		if readErr == io.EOF {
			state.update(func(download *models.ManagedDownload) {
				if download.TotalLength == 0 {
					download.TotalLength = download.CompletedLength
				}
				download.DownloadSpeed = 0
				download.Status = models.ManagedDownloadStatusComplete
			})
			return
		}
		state.fail(fmt.Errorf("read download body: %w", readErr))
		return
	}
}

func (s *downloadState) snapshot() models.ManagedDownload {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.download
}

func (s *downloadState) update(fn func(*models.ManagedDownload)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	fn(&s.download)
}

func (s *downloadState) fail(err error) {
	s.update(func(download *models.ManagedDownload) {
		download.Status = models.ManagedDownloadStatusError
		download.ErrorMessage = err.Error()
	})
}
