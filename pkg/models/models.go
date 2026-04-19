package models

import "time"

type AuthMethod string

const (
	AuthMethodNone   AuthMethod = "none"
	AuthMethodToken  AuthMethod = "token"
	AuthMethodDevice AuthMethod = "device"
)

type User struct {
	ID         int       `json:"id"`
	Username   string    `json:"username"`
	Email      string    `json:"email"`
	Points     int       `json:"points"`
	Locale     string    `json:"locale"`
	Avatar     string    `json:"avatar"`
	Type       string    `json:"type"`
	Premium    int       `json:"premium"`
	Expiration time.Time `json:"expiration"`
}

type Torrent struct {
	ID       string    `json:"id"`
	Filename string    `json:"filename"`
	Hash     string    `json:"hash"`
	Bytes    int64     `json:"bytes"`
	Host     string    `json:"host"`
	Split    int       `json:"split"`
	Progress float64   `json:"progress"`
	Status   string    `json:"status"`
	Added    time.Time `json:"added"`
	Ended    time.Time `json:"ended,omitempty"`
	Links    []string  `json:"links"`
}

type TorrentFile struct {
	ID       int    `json:"id"`
	Path     string `json:"path"`
	Bytes    int64  `json:"bytes"`
	Selected bool   `json:"selected"`
}

type TorrentInfo struct {
	Torrent
	OriginalFilename string        `json:"original_filename"`
	OriginalBytes    int64         `json:"original_bytes"`
	Files            []TorrentFile `json:"files"`
	Seeders          int           `json:"seeders,omitempty"`
	Speed            int64         `json:"speed,omitempty"`
}

type AddTorrentResult struct {
	ID  string `json:"id"`
	URI string `json:"uri"`
}

type ImportResult struct {
	Source    string
	TorrentID string
	Err       error
}

func (r ImportResult) OK() bool {
	return r.Err == nil
}

type UnrestrictedLink struct {
	ID       string `json:"id"`
	Filename string `json:"filename"`
	MimeType string `json:"mimeType"`
	Filesize int64  `json:"filesize"`
	Link     string `json:"link"`
	Host     string `json:"host"`
	Chunks   int    `json:"chunks"`
	Download string `json:"download"`
}

type DownloadTarget struct {
	Index    int
	Label    string
	Link     string
	FilePath string
}

type DownloadBackend string

const (
	DownloadBackendAria2  DownloadBackend = "aria2"
	DownloadBackendDirect DownloadBackend = "direct"
)

type ManagedDownloadRequest struct {
	URL      string
	Dir      string
	Filename string
}

type ManagedDownloadStatus string

const (
	ManagedDownloadStatusActive   ManagedDownloadStatus = "active"
	ManagedDownloadStatusWaiting  ManagedDownloadStatus = "waiting"
	ManagedDownloadStatusPaused   ManagedDownloadStatus = "paused"
	ManagedDownloadStatusComplete ManagedDownloadStatus = "complete"
	ManagedDownloadStatusError    ManagedDownloadStatus = "error"
	ManagedDownloadStatusRemoved  ManagedDownloadStatus = "removed"
)

type ManagedDownload struct {
	GID             string
	URL             string
	Filename        string
	Status          ManagedDownloadStatus
	TotalLength     int64
	CompletedLength int64
	DownloadSpeed   int64
	Connections     int
	ErrorMessage    string
	Directory       string
	FilePath        string
}

func (d ManagedDownload) Progress() float64 {
	if d.TotalLength == 0 {
		return 0
	}
	return float64(d.CompletedLength) / float64(d.TotalLength) * 100
}

func (d ManagedDownload) ETA() time.Duration {
	if d.DownloadSpeed <= 0 || d.TotalLength <= 0 {
		return -1
	}
	remaining := d.TotalLength - d.CompletedLength
	if remaining <= 0 {
		return 0
	}
	return time.Duration(remaining/d.DownloadSpeed) * time.Second
}

func (d ManagedDownload) IsComplete() bool {
	return d.Status == ManagedDownloadStatusComplete
}

func (d ManagedDownload) IsError() bool {
	return d.Status == ManagedDownloadStatusError
}

func (d ManagedDownload) IsTerminal() bool {
	switch d.Status {
	case ManagedDownloadStatusComplete, ManagedDownloadStatusError, ManagedDownloadStatusRemoved:
		return true
	default:
		return false
	}
}

type ManagedDownloadStart struct {
	Download ManagedDownload
	Reused   bool
}

type HandoffResult struct {
	URL      string
	Copied   bool
	Launched bool
	Command  []string
}

type DeviceCode struct {
	DeviceCode            string `json:"device_code"`
	UserCode              string `json:"user_code"`
	Interval              int    `json:"interval"`
	ExpiresIn             int    `json:"expires_in"`
	VerificationURL       string `json:"verification_url"`
	DirectVerificationURL string `json:"direct_verification_url"`
	RequestedAt           time.Time
}

type DeviceCredentials struct {
	ClientID     string    `json:"client_id"`
	ClientSecret string    `json:"client_secret"`
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	TokenType    string    `json:"token_type"`
	ExpiresAt    time.Time `json:"expires_at"`
	CreatedAt    time.Time `json:"created_at"`
}

type AuthSession struct {
	Method AuthMethod
	Token  string
	User   User
}
