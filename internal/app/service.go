package app

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/m4rii0/rdtui/internal/auth"
	"github.com/m4rii0/rdtui/internal/config"
	"github.com/m4rii0/rdtui/internal/download"
	"github.com/m4rii0/rdtui/internal/realdebrid"
	"github.com/m4rii0/rdtui/pkg/models"
)

type Service struct {
	store   *config.Store
	config  config.Config
	auth    *auth.Manager
	client  *realdebrid.Client
	session *models.AuthSession
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
		store:  store,
		config: cfg,
		auth:   auth.NewManager(store, cfg),
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

func (s *Service) LaunchDownload(url, filename string) (models.HandoffResult, error) {
	return download.Launch(s.config.ExternalCommand, download.TemplateData{
		URL:      url,
		Dir:      s.config.DefaultDownloadDir,
		Filename: download.FilenameForURL(url, filename),
	})
}

func (s *Service) SaveExternalCommand(command []string) error {
	s.config.ExternalCommand = append([]string(nil), command...)
	return s.store.SaveConfig(s.config)
}

func (s *Service) SavePrivateToken(token string) error {
	s.config.PrivateToken = token
	return s.store.SaveConfig(s.config)
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
