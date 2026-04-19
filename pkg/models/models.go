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
