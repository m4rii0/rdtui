package demo

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/m4rii0/rdtui/internal/app"
	"github.com/m4rii0/rdtui/internal/config"
	"github.com/m4rii0/rdtui/pkg/models"
)

const defaultDownloadDuration = 10 * time.Second

// Service implements app.AppService with fully offline mock data.
// All state lives in memory; nothing touches the network or disk.
type Service struct {
	mu       sync.Mutex
	torrents []torrentEntry
	nextID   int

	activeDownload     *models.ManagedDownload
	downloadGeneration uint64
	downloadStart      time.Time
	downloadDuration   time.Duration
}

type torrentEntry struct {
	torrent models.Torrent
	info    models.TorrentInfo
}

func NewService() *Service {
	s := &Service{nextID: 100, downloadDuration: demoDownloadDuration()}
	s.torrents = s.seedData()
	return s
}

// ---------------------------------------------------------------------------
// Auth / config — all no-ops for demo
// ---------------------------------------------------------------------------

func (s *Service) Config() config.Config {
	return config.Config{
		DefaultDownloadDir: "/tmp/rdtui-demo",
		DownloadBackend:    "direct",
	}
}

func (s *Service) Bootstrap(_ context.Context) (*models.AuthSession, error) {
	return &models.AuthSession{
		Method: models.AuthMethodToken,
		Token:  "demo-token",
		User: models.User{
			ID:         1,
			Username:   "linux_iso_collector",
			Email:      "demo@example.com",
			Premium:    1,
			Expiration: time.Now().Add(30 * 24 * time.Hour),
		},
	}, nil
}

func (s *Service) AuthenticateWithToken(_ context.Context, _ string, _ bool) (*models.AuthSession, error) {
	return s.Bootstrap(context.Background())
}

func (s *Service) StartDeviceFlow(_ context.Context) (models.DeviceCode, error) {
	return models.DeviceCode{
		UserCode:        "DEMO1234",
		VerificationURL: "https://real-debrid.com/device",
		Interval:        5,
		ExpiresIn:       600,
		RequestedAt:     time.Now(),
	}, nil
}

func (s *Service) CompleteDeviceFlow(_ context.Context, _ models.DeviceCode) (*models.AuthSession, error) {
	return s.Bootstrap(context.Background())
}

func (s *Service) SavePrivateToken(_ string) error  { return nil }
func (s *Service) OpenFile(_ string) error          { return nil }
func (s *Service) RevealInDirectory(_ string) error { return nil }
func (s *Service) Close() error                     { return nil }

// ---------------------------------------------------------------------------
// Torrent CRUD
// ---------------------------------------------------------------------------

func (s *Service) ListTorrents(_ context.Context) ([]models.Torrent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]models.Torrent, len(s.torrents))
	for i, e := range s.torrents {
		out[i] = e.torrent
	}
	return out, nil
}

func (s *Service) TorrentInfo(_ context.Context, id string) (models.TorrentInfo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, e := range s.torrents {
		if e.torrent.ID == id {
			return e.info, nil
		}
	}
	return models.TorrentInfo{}, fmt.Errorf("torrent %s not found", id)
}

func (s *Service) AddMagnet(_ context.Context, magnet string) (models.AddTorrentResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	id := fmt.Sprintf("demo-%d", s.nextID)
	s.nextID++

	name := "New Magnet Torrent"
	if _, rest, found := strings.Cut(magnet, "dn="); found {
		rest, _, _ = strings.Cut(rest, "&")
		name = strings.ReplaceAll(rest, "+", " ")
	}

	entry := s.makeTorrent(id, name, "waiting_files_selection", 0, randSize())
	s.torrents = append([]torrentEntry{entry}, s.torrents...)
	return models.AddTorrentResult{ID: id, URI: "https://api.real-debrid.com/rest/1.0/torrents/info/" + id}, nil
}

func (s *Service) AddTorrentURL(_ context.Context, remoteURL string) (models.AddTorrentResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	id := fmt.Sprintf("demo-%d", s.nextID)
	s.nextID++

	name := remoteURL
	if _, base, found := strings.CutLast(remoteURL, "/"); found && base != "" {
		name = base
	}

	entry := s.makeTorrent(id, name, "waiting_files_selection", 0, randSize())
	s.torrents = append([]torrentEntry{entry}, s.torrents...)
	return models.AddTorrentResult{ID: id}, nil
}

func (s *Service) ImportTorrentFiles(_ context.Context, paths []string) []models.ImportResult {
	s.mu.Lock()
	defer s.mu.Unlock()

	results := make([]models.ImportResult, 0, len(paths))
	for _, path := range paths {
		id := fmt.Sprintf("demo-%d", s.nextID)
		s.nextID++

		name := path
		if _, base, found := strings.CutLast(path, "/"); found {
			name = base
		}
		name = strings.TrimSuffix(name, ".torrent")

		entry := s.makeTorrent(id, name, "waiting_files_selection", 0, randSize())
		s.torrents = append([]torrentEntry{entry}, s.torrents...)
		results = append(results, models.ImportResult{Source: path, TorrentID: id})
	}
	return results
}

func (s *Service) SelectFiles(_ context.Context, torrentID string, ids []int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, e := range s.torrents {
		if e.torrent.ID == torrentID {
			s.torrents[i].torrent.Status = "downloading"
			s.torrents[i].torrent.Progress = 0
			s.torrents[i].info.Status = "downloading"
			s.torrents[i].info.Progress = 0
			for j, f := range s.torrents[i].info.Files {
				selected := false
				for _, sid := range ids {
					if f.ID == sid {
						selected = true
						break
					}
				}
				s.torrents[i].info.Files[j].Selected = selected
			}
			return nil
		}
	}
	return fmt.Errorf("torrent %s not found", torrentID)
}

func (s *Service) DeleteTorrent(_ context.Context, torrentID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, e := range s.torrents {
		if e.torrent.ID == torrentID {
			s.torrents = append(s.torrents[:i], s.torrents[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("torrent %s not found", torrentID)
}

// ---------------------------------------------------------------------------
// Download / URL resolution
// ---------------------------------------------------------------------------

func (s *Service) ResolveDirectURL(_ context.Context, target models.DownloadTarget) (models.UnrestrictedLink, error) {
	return models.UnrestrictedLink{
		ID:       "demo-link",
		Filename: target.Label,
		Filesize: 1_500_000_000,
		Download: "https://demo.real-debrid.com/d/" + target.Label,
		Link:     target.Link,
	}, nil
}

func (s *Service) CopyURL(_ string) (bool, error) {
	// Pretend clipboard worked
	return true, nil
}

func (s *Service) StartManagedDownload(_ context.Context, url, filename string) (models.ManagedDownloadStart, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.startDownloadLocked(url, filename)
}

func (s *Service) startDownloadLocked(url, filename string) (models.ManagedDownloadStart, error) {

	if s.activeDownload != nil && !s.activeDownload.IsTerminal() {
		return models.ManagedDownloadStart{Download: *s.activeDownload, Reused: true}, nil
	}

	dl := models.ManagedDownload{
		GID:         fmt.Sprintf("demo-gid-%d", time.Now().UnixNano()),
		URL:         url,
		Filename:    filename,
		Status:      models.ManagedDownloadStatusActive,
		TotalLength: 1_500_000_000,
		Directory:   "/tmp/rdtui-demo",
	}
	s.activeDownload = &dl
	s.downloadGeneration++
	s.downloadStart = time.Now()
	return models.ManagedDownloadStart{Download: dl}, nil
}

func (s *Service) ManagedDownloadStatus(_ context.Context) (models.ManagedDownload, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.downloadStatusLocked()
}

func (s *Service) downloadStatusLocked() (models.ManagedDownload, bool, error) {

	if s.activeDownload == nil {
		return models.ManagedDownload{}, false, nil
	}

	progress := time.Since(s.downloadStart).Seconds() / s.downloadDuration.Seconds()
	if progress >= 1.0 {
		s.activeDownload.Status = models.ManagedDownloadStatusComplete
		s.activeDownload.CompletedLength = s.activeDownload.TotalLength
		s.activeDownload.DownloadSpeed = 0
		s.activeDownload.FilePath = s.activeDownload.Directory + "/" + s.activeDownload.Filename
	} else {
		s.activeDownload.CompletedLength = int64(float64(s.activeDownload.TotalLength) * progress)
		s.activeDownload.DownloadSpeed = 150_000_000 // ~150 MB/s
	}

	return *s.activeDownload, true, nil
}

func (s *Service) Start(ctx context.Context, url, filename string) (app.DownloadStart, error) {
	// Keep admission and reference capture under the same lock.
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return app.DownloadStart{}, err
	}
	result, err := s.startDownloadLocked(url, filename)
	return app.DownloadStart{Snapshot: app.DownloadSnapshot{Ref: app.DownloadRef{Generation: s.downloadGeneration, GID: result.Download.GID}, Download: result.Download}, Reused: result.Reused}, err
}

func (s *Service) Status(ctx context.Context, ref app.DownloadRef) (app.DownloadSnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return app.DownloadSnapshot{}, err
	}
	if s.activeDownload == nil || ref != (app.DownloadRef{Generation: s.downloadGeneration, GID: s.activeDownload.GID}) {
		return app.DownloadSnapshot{}, app.ErrStaleDownload
	}
	download, _, err := s.downloadStatusLocked()
	return app.DownloadSnapshot{Ref: ref, Download: download}, err
}

func demoDownloadDuration() time.Duration {
	value := strings.TrimSpace(os.Getenv("RDTUI_DEMO_DOWNLOAD_DURATION"))
	if value == "" {
		return defaultDownloadDuration
	}
	duration, err := time.ParseDuration(value)
	if err != nil || duration <= 0 {
		return defaultDownloadDuration
	}
	return duration
}

// ---------------------------------------------------------------------------
// Seed data
// ---------------------------------------------------------------------------

func (s *Service) seedData() []torrentEntry {
	type spec struct {
		name     string
		status   string
		progress float64
		bytes    int64
	}

	seeds := []spec{
		{"Ubuntu.24.04.2.Desktop.amd64.iso", "downloaded", 100, 5_200_000_000},
		{"Arch.Linux.2025.04.01.x86_64.iso", "downloaded", 100, 1_100_000_000},
		{"Fedora.Workstation.42.x86_64.iso", "downloading", 67.3, 2_100_000_000},
		{"Debian.12.9.0.amd64.DVD-1.iso", "downloaded", 100, 3_700_000_000},
		{"Debian.12.9.0.amd64.DVD-2.iso", "downloaded", 100, 3_700_000_000},
		{"Debian.12.9.0.amd64.DVD-3.iso", "downloaded", 100, 3_700_000_000},
		{"openSUSE.Tumbleweed.DVD.x86_64.Snapshot20250410.iso", "downloading", 42.1, 4_700_000_000},
		{"Linux.Mint.22.1.Cinnamon.64bit.iso", "waiting_files_selection", 0, 2_900_000_000},
		{"Manjaro.KDE.25.0.1-250316.Linux612.iso", "downloaded", 100, 3_500_000_000},
		{"CentOS.Stream.10.x86_64.dvd1.iso", "downloaded", 100, 10_200_000_000},
		{"Alpine.Standard.3.21.3.x86_64.iso", "downloading", 89.5, 250_000_000},
		{"NixOS.25.05.x86_64-linux.GNOME.iso", "queued", 0, 2_400_000_000},
		{"Gentoo.amd64.minimal.20250407.iso", "magnet_conversion", 0, 0},
		{"Void.Linux.live.x86_64-20250313.ENLIGHTMENT.iso", "error", 0, 900_000_000},
		{"Pop_OS.22.04.amd64.nvidia.20250401.iso", "downloaded", 100, 2_700_000_000},
		{"Rocky.Linux.9.5.x86_64.dvd.iso", "waiting_files_selection", 0, 9_200_000_000},
		{"Alma.Linux.9.5.x86_64.DVD.iso", "waiting_files_selection", 0, 8_900_000_000},
		{"Kali.Linux.2025.1a.installer.amd64.iso", "waiting_files_selection", 0, 3_800_000_000},
		{"Slackware.15.0.install.dvd.iso", "waiting_files_selection", 0, 2_800_000_000},
		{"EndeavourOS.Cassini.Nova.202503.iso", "waiting_files_selection", 0, 2_200_000_000},
		{"Garuda.Linux.KDE.Raptor.202504.iso", "queued", 0, 3_100_000_000},
		{"Artix.Linux.20250315.plasma.x86_64.iso", "magnet_conversion", 0, 0},
		{"FreeBSD.14.2.RELEASE.amd64.disc1.iso", "waiting_files_selection", 0, 1_000_000_000},
		{"NetBSD.10.1.amd64.iso", "waiting_files_selection", 0, 750_000_000},
	}

	entries := make([]torrentEntry, 0, len(seeds))
	baseTime := time.Now().Add(-30 * 24 * time.Hour)
	for i, sp := range seeds {
		id := fmt.Sprintf("demo-%d", i+1)
		s.nextID = i + 2
		entry := s.makeTorrent(id, sp.name, sp.status, sp.progress, sp.bytes)
		entry.torrent.Added = baseTime.Add(time.Duration(rand.Intn(30*24*60)) * time.Minute)
		entry.info.Added = entry.torrent.Added
		entries = append(entries, entry)
	}
	return entries
}

func (s *Service) makeTorrent(id, name, status string, progress float64, bytes int64) torrentEntry {
	hash := fmt.Sprintf("%040x", rand.Int63())

	links := []string{}
	if status == "downloaded" {
		links = []string{"https://real-debrid.com/d/" + id}
	}

	t := models.Torrent{
		ID:       id,
		Filename: name,
		Hash:     hash,
		Bytes:    bytes,
		Host:     "real-debrid.com",
		Progress: progress,
		Status:   status,
		Added:    time.Now(),
		Links:    links,
	}

	files := generateFiles(name, bytes)

	info := models.TorrentInfo{
		Torrent:          t,
		OriginalFilename: name,
		OriginalBytes:    bytes,
		Files:            files,
		Seeders:          rand.Intn(500) + 1,
		Speed:            int64(rand.Intn(50_000_000)),
	}

	return torrentEntry{torrent: t, info: info}
}

func generateFiles(name string, totalBytes int64) []models.TorrentFile {
	if totalBytes == 0 {
		return []models.TorrentFile{{ID: 1, Path: "/" + name, Bytes: 0, Selected: false}}
	}

	return []models.TorrentFile{
		{ID: 1, Path: "/" + name, Bytes: totalBytes, Selected: true},
	}
}

func randSize() int64 {
	sizes := []int64{
		350_000_000,
		700_000_000,
		1_500_000_000,
		4_300_000_000,
		8_700_000_000,
	}
	return sizes[rand.Intn(len(sizes))]
}
