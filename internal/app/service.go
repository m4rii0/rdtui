package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/m4rii0/rdtui/internal/aria2"
	"github.com/m4rii0/rdtui/internal/auth"
	"github.com/m4rii0/rdtui/internal/config"
	"github.com/m4rii0/rdtui/internal/directdl"
	"github.com/m4rii0/rdtui/internal/download"
	"github.com/m4rii0/rdtui/internal/realdebrid"
	"github.com/m4rii0/rdtui/pkg/models"
)

type Service struct {
	prefs     *config.Repository
	auth      *auth.Manager
	client    *realdebrid.Client
	session   *models.AuthSession
	downloads *DownloadCoordinator
	runtime   *Runtime
}

func New(parent context.Context) (*Service, error) {
	store, err := config.NewStore()
	if err != nil {
		return nil, err
	}
	// Capture process inputs once; preference saves never reread the environment.
	overrides := config.Overrides{
		PrivateToken: os.Getenv("RDTUI_PRIVATE_TOKEN"),
		APIBaseURL:   os.Getenv("RDTUI_API_BASE_URL"),
		OAuthBaseURL: os.Getenv("RDTUI_OAUTH_BASE_URL"),
	}
	home, _ := os.UserHomeDir() // Resolve reports a missing home only when needed.
	prefs, err := config.NewRepository(store, overrides, home)
	if err != nil {
		return nil, err
	}
	cfg := prefs.Effective()
	runtime := NewRuntime(parent)
	manager := auth.NewManager(auth.SessionDependencies{
		Root: runtime.Context(), Spawn: runtime.Spawn,
		OAuth: realdebrid.NewClient("", cfg.APIBaseURL, cfg.OAuthBaseURL),
		Validate: func(ctx context.Context, token string) (models.User, error) {
			return realdebrid.NewClient(token, cfg.APIBaseURL, cfg.OAuthBaseURL).User(ctx)
		},
		Credentials: store, Preferences: prefs, Now: time.Now,
	})
	return &Service{
		prefs:     prefs,
		auth:      manager,
		runtime:   runtime,
		downloads: NewDownloadCoordinator(parent, newDownloadManager(cfg), cfg.DefaultDownloadDir),
	}, nil
}

func newDownloadManager(cfg config.EffectiveConfig) download.Backend {
	switch cfg.DownloadBackend {
	case models.DownloadBackendAria2:
		return aria2.NewManager(cfg.Aria2BinaryPath)
	default:
		return directdl.NewManager()
	}
}

func (s *Service) Config() config.Config {
	// Temporary facade view for the TUI; it is never used for persistence.
	cfg := s.prefs.Effective()
	return config.Config{
		PrivateToken: cfg.PrivateToken, DefaultDownloadDir: cfg.DefaultDownloadDir,
		DownloadBackend: string(cfg.DownloadBackend), Aria2BinaryPath: cfg.Aria2BinaryPath,
		APIBaseURL: cfg.APIBaseURL, OAuthBaseURL: cfg.OAuthBaseURL,
	}
}

func (s *Service) Bootstrap(ctx context.Context) (*models.AuthSession, error) {
	session, err := s.auth.Bootstrap(ctx)
	if err != nil {
		return nil, err
	}
	s.setSession(session)
	return session, nil
}

func (s *Service) AuthenticateWithToken(ctx context.Context, token string, persist bool) (*models.AuthSession, error) {
	session, err := s.auth.AuthenticateWithToken(ctx, token, persist)
	if err != nil {
		return nil, err
	}
	s.setSession(session)
	return session, nil
}

func (s *Service) StartDeviceFlow(ctx context.Context) (models.DeviceCode, error) {
	return s.auth.StartDeviceFlow(ctx)
}

func (s *Service) CompleteDeviceFlow(ctx context.Context, code models.DeviceCode) (*models.AuthSession, error) {
	session, err := s.auth.CompleteDeviceFlow(ctx, code)
	if err != nil {
		return nil, err
	}
	s.setSession(session)
	return session, nil
}

func (s *Service) ListTorrents(ctx context.Context) ([]models.Torrent, error) {
	return s.Torrents().ListTorrents(ctx)
}

// Temporary composition bridges while the facade still owns the live client.
func (s *Service) Torrents() *TorrentService { return NewTorrentService(serviceTorrentAPI{s}) }

type serviceTorrentAPI struct{ *Service }

func (s serviceTorrentAPI) ListTorrents(ctx context.Context, limit int) ([]models.Torrent, error) {
	if s.client == nil {
		return nil, auth.ErrNoValidCredentials
	}
	return s.client.ListTorrents(ctx, limit)
}

func (s *Service) NewBatches() *BatchService { return NewBatchService(serviceBatchAPI{s}, s, nil) }

type serviceBatchAPI struct{ *Service }

func (s serviceBatchAPI) UnrestrictLink(ctx context.Context, link string) (models.UnrestrictedLink, error) {
	return s.ResolveDirectURL(ctx, models.DownloadTarget{Link: link})
}
func (s serviceBatchAPI) AddTorrentPath(ctx context.Context, path string) (models.AddTorrentResult, error) {
	return s.client.AddTorrentPath(ctx, path)
}

func (s *Service) TorrentInfo(ctx context.Context, id string) (models.TorrentInfo, error) {
	return s.client.TorrentInfo(ctx, id)
}

func (s *Service) AddMagnet(ctx context.Context, magnet string) (models.AddTorrentResult, error) {
	return s.client.AddMagnet(ctx, magnet)
}

func (s *Service) AddTorrentURL(ctx context.Context, remoteURL string) (models.AddTorrentResult, error) {
	return s.client.AddTorrentURL(ctx, remoteURL)
}

func (s *Service) ImportTorrentFiles(ctx context.Context, paths []string) []models.ImportResult {
	batch := NewBatchService(s.client, s, nil).Import(ctx, paths)
	results := make([]models.ImportResult, 0, len(batch.Items))
	for _, item := range batch.Items {
		results = append(results, models.ImportResult{Source: item.Source, TorrentID: item.TorrentCreatedID, Err: item.Err})
	}
	return results
}

func (s *Service) SelectFiles(ctx context.Context, torrentID string, ids []int) error {
	return s.client.SelectFiles(ctx, torrentID, ids)
}

func (s *Service) DeleteTorrent(ctx context.Context, torrentID string) error {
	return s.client.DeleteTorrent(ctx, torrentID)
}

func DefaultFileSelection(info models.TorrentInfo) []int {
	if len(info.Files) == 0 {
		return nil
	}
	largest := info.Files[0]
	for _, file := range info.Files[1:] {
		if file.Bytes > largest.Bytes {
			largest = file
		}
	}
	return []int{largest.ID}
}

func DownloadTargets(info models.TorrentInfo) []models.DownloadTarget {
	selectedFiles := make([]models.TorrentFile, 0)
	for _, file := range info.Files {
		if file.Selected {
			selectedFiles = append(selectedFiles, file)
		}
	}
	if len(selectedFiles) == 0 {
		selectedFiles = append(selectedFiles, info.Files...)
	}
	out := make([]models.DownloadTarget, 0, len(info.Links))
	for idx, link := range info.Links {
		label := fmt.Sprintf("Link %d", idx+1)
		filePath := ""
		if idx < len(selectedFiles) {
			filePath = selectedFiles[idx].Path
			label = filepath.Base(filePath)
			if label == "." || label == "/" || label == "" {
				label = filePath
			}
		}
		out = append(out, models.DownloadTarget{Index: idx, Link: link, Label: label, FilePath: filePath})
	}
	return out
}

func (s *Service) ResolveDirectURL(ctx context.Context, target models.DownloadTarget) (models.UnrestrictedLink, error) {
	return s.client.UnrestrictLink(ctx, target.Link)
}

func (s *Service) CopyURL(url string) (bool, error) {
	return download.CopyURL(url)
}

func (s *Service) StartManagedDownload(ctx context.Context, url, filename string) (models.ManagedDownloadStart, error) {
	started, err := s.Start(ctx, url, filename)
	if err != nil {
		return models.ManagedDownloadStart{}, err
	}
	return models.ManagedDownloadStart{Download: started.Snapshot.Download, Reused: started.Reused}, nil
}

func (s *Service) ManagedDownloadStatus(ctx context.Context) (models.ManagedDownload, bool, error) {
	current, ok := s.downloads.Current()
	if !ok {
		return models.ManagedDownload{}, false, nil
	}
	status, err := s.downloads.Status(ctx, current.Ref)
	return status.Download, true, err
}

// Start and Status preserve coordinator identity across the UI boundary.
func (s *Service) Start(ctx context.Context, url, filename string) (DownloadStart, error) {
	if current, ok := s.downloads.Current(); ok && !current.Download.IsTerminal() {
		// Refresh unobserved completion before reuse. Query errors do not release
		// the coordinator's active slot or authorize a duplicate transfer.
		_, _ = s.downloads.Status(ctx, current.Ref)
	}
	return s.downloads.Start(ctx, url, filename)
}

func (s *Service) Status(ctx context.Context, ref DownloadRef) (DownloadSnapshot, error) {
	return s.downloads.Status(ctx, ref)
}

func (s *Service) CurrentDownload() (DownloadSnapshot, bool) { return s.downloads.Current() }

// NewBulkQueue is temporary facade composition until all dependencies are wired
// at the entry point. Singles and queues must share this coordinator.
func (s *Service) NewBulkQueue(runtime *Runtime) *BulkQueue {
	return NewBulkQueue(QueueDependencies{Runtime: runtime, Downloads: s.downloads, Prepare: s.PrepareDownload})
}

func (s *Service) PrepareDownload(ctx context.Context, target models.DownloadTarget) (PreparedDownload, error) {
	return PrepareDownload(ctx, target, s.prefs.Effective().DefaultDownloadDir, s.ResolveDirectURL, func(path string) (int64, bool, error) {
		info, err := os.Stat(path)
		if errors.Is(err, os.ErrNotExist) {
			return 0, false, nil
		}
		if err != nil {
			return 0, false, err
		}
		if info.IsDir() {
			return 0, false, fmt.Errorf("download target is directory: %s", path)
		}
		return info.Size(), true, nil
	})
}

func (s *Service) SavePrivateToken(token string) error {
	return s.prefs.SavePrivateToken(token)
}

func (s *Service) OpenFile(path string) error {
	return download.OpenFile(path)
}

func (s *Service) RevealInDirectory(path string) error {
	return download.RevealInDirectory(path)
}

func (s *Service) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return s.CloseContext(ctx)
}

// CloseContext lets composition share its cleanup budget with runtime draining.
func (s *Service) CloseContext(ctx context.Context) error {
	if s.runtime == nil {
		return s.downloads.Close(ctx)
	}
	s.runtime.Stop()
	return errors.Join(s.downloads.Close(ctx), s.runtime.Wait(ctx))
}

func ValidTorrentFiles(paths []string) (valid []string, invalid []string) {
	seen := map[string]struct{}{}
	for _, path := range paths {
		clean := filepath.Clean(path)
		if _, ok := seen[clean]; ok {
			continue
		}
		seen[clean] = struct{}{}
		if strings.EqualFold(filepath.Ext(clean), ".torrent") {
			valid = append(valid, clean)
		} else {
			invalid = append(invalid, clean)
		}
	}
	slices.Sort(valid)
	slices.Sort(invalid)
	return valid, invalid
}

func (s *Service) setSession(session *models.AuthSession) {
	s.session = session
	cfg := s.prefs.Effective()
	s.client = realdebrid.NewClient(session.Token, cfg.APIBaseURL, cfg.OAuthBaseURL)
}
