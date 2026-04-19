package app

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/m4rii0/rdtui/internal/aria2"
	"github.com/m4rii0/rdtui/internal/auth"
	"github.com/m4rii0/rdtui/internal/config"
	"github.com/m4rii0/rdtui/internal/download"
	"github.com/m4rii0/rdtui/internal/realdebrid"
	"github.com/m4rii0/rdtui/pkg/models"
)

type downloadManager interface {
	SetBinaryPath(string)
	StartDownload(context.Context, aria2.DownloadRequest) (models.ManagedDownload, error)
	DownloadStatus(context.Context, string) (models.ManagedDownload, error)
	Shutdown(context.Context) error
}

type Service struct {
	store          *config.Store
	config         config.Config
	auth           *auth.Manager
	client         *realdebrid.Client
	session        *models.AuthSession
	downloader     downloadManager
	activeDownload *models.ManagedDownload
}

func New() (*Service, error) {
	store, err := config.NewStore()
	if err != nil {
		return nil, err
	}
	cfg, err := store.LoadConfig()
	if err != nil {
		return nil, err
	}
	return &Service{
		store:      store,
		config:     cfg,
		auth:       auth.NewManager(store, cfg),
		downloader: aria2.NewManager(cfg.Aria2BinaryPath),
	}, nil
}

func (s *Service) Config() config.Config {
	return s.config
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
	session, err := s.auth.ValidateAndPersistToken(ctx, token, persist)
	if err != nil {
		return nil, err
	}
	if persist {
		s.config.PrivateToken = token
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
	if s.client == nil {
		return nil, auth.ErrNoValidCredentials
	}
	return s.client.ListTorrents(ctx, 100)
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
	results := make([]models.ImportResult, 0, len(paths))
	for _, path := range paths {
		res := models.ImportResult{Source: path}
		added, err := s.client.AddTorrentPath(ctx, path)
		if err != nil {
			res.Err = err
		} else {
			res.TorrentID = added.ID
		}
		results = append(results, res)
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
	if current, ok, err := s.ManagedDownloadStatus(ctx); err == nil && ok && !current.IsTerminal() {
		return models.ManagedDownloadStart{Download: current, Reused: true}, nil
	}

	request := aria2.DownloadRequest{
		URL:      url,
		Dir:      s.config.DefaultDownloadDir,
		Filename: download.FilenameForURL(url, filename),
	}
	started, err := s.downloader.StartDownload(ctx, request)
	if err != nil {
		return models.ManagedDownloadStart{}, err
	}
	if started.URL == "" {
		started.URL = url
	}
	if started.Filename == "" {
		started.Filename = request.Filename
	}
	if started.Directory == "" {
		started.Directory = request.Dir
	}
	s.activeDownload = &started
	return models.ManagedDownloadStart{Download: started}, nil
}

func (s *Service) ManagedDownloadStatus(ctx context.Context) (models.ManagedDownload, bool, error) {
	if s.activeDownload == nil {
		return models.ManagedDownload{}, false, nil
	}

	status, err := s.downloader.DownloadStatus(ctx, s.activeDownload.GID)
	if err != nil {
		failed := *s.activeDownload
		if !failed.IsTerminal() {
			failed.Status = models.ManagedDownloadStatusError
		}
		failed.ErrorMessage = err.Error()
		s.activeDownload = &failed
		return failed, true, err
	}

	if status.URL == "" {
		status.URL = s.activeDownload.URL
	}
	if status.Filename == "" {
		status.Filename = s.activeDownload.Filename
	}
	if status.Directory == "" {
		status.Directory = s.activeDownload.Directory
	}
	if status.FilePath == "" {
		status.FilePath = s.activeDownload.FilePath
	}
	s.activeDownload = &status
	return status, true, nil
}

func (s *Service) SavePrivateToken(token string) error {
	s.config.PrivateToken = token
	return s.store.SaveConfig(s.config)
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
	return s.downloader.Shutdown(ctx)
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
	sort.Strings(valid)
	sort.Strings(invalid)
	return valid, invalid
}

func (s *Service) setSession(session *models.AuthSession) {
	s.session = session
	s.client = realdebrid.NewClient(session.Token, s.config.APIBaseURL, s.config.OAuthBaseURL)
}
