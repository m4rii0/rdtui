package realdebrid

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/m4rii0/rdtui/pkg/models"
)

const (
	DefaultAPIBaseURL   = "https://api.real-debrid.com/rest/1.0"
	DefaultOAuthBaseURL = "https://api.real-debrid.com/oauth/v2"
	OpenSourceClientID  = "X245A4XAIBGVM"
	deviceGrantType     = "http://oauth.net/grant_type/device/1.0"
)

var ErrAuthorizationPending = errors.New("authorization pending")

type Client struct {
	baseURL      string
	oauthBaseURL string
	token        string
	httpClient   *http.Client
}

type ErrorResponse struct {
	Error     string `json:"error"`
	ErrorCode int    `json:"error_code,omitempty"`
}

type apiError struct {
	StatusCode int
	Message    string
	Code       int
}

func (e *apiError) Error() string {
	if e.Code != 0 {
		return fmt.Sprintf("real-debrid error %d (%d): %s", e.StatusCode, e.Code, e.Message)
	}
	return fmt.Sprintf("real-debrid error %d: %s", e.StatusCode, e.Message)
}

func NewClient(token, baseURL, oauthBaseURL string) *Client {
	if baseURL == "" {
		baseURL = DefaultAPIBaseURL
	}
	if oauthBaseURL == "" {
		oauthBaseURL = DefaultOAuthBaseURL
	}
	return &Client{
		baseURL:      strings.TrimRight(baseURL, "/"),
		oauthBaseURL: strings.TrimRight(oauthBaseURL, "/"),
		token:        token,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (c *Client) SetToken(token string) {
	c.token = token
}

func (c *Client) User(ctx context.Context) (models.User, error) {
	var out models.User
	err := c.doJSON(ctx, http.MethodGet, c.baseURL+"/user", nil, "", &out)
	return out, err
}

func (c *Client) ListTorrents(ctx context.Context, limit int) ([]models.Torrent, error) {
	if limit <= 0 {
		limit = 100
	}
	endpoint := fmt.Sprintf("%s/torrents?limit=%d", c.baseURL, limit)
	var raw []struct {
		ID       string   `json:"id"`
		Filename string   `json:"filename"`
		Hash     string   `json:"hash"`
		Bytes    int64    `json:"bytes"`
		Host     string   `json:"host"`
		Split    int      `json:"split"`
		Progress float64  `json:"progress"`
		Status   string   `json:"status"`
		Added    string   `json:"added"`
		Ended    string   `json:"ended"`
		Links    []string `json:"links"`
	}
	if err := c.doJSON(ctx, http.MethodGet, endpoint, nil, "", &raw); err != nil {
		return nil, err
	}
	out := make([]models.Torrent, 0, len(raw))
	for _, item := range raw {
		out = append(out, models.Torrent{
			ID:       item.ID,
			Filename: item.Filename,
			Hash:     item.Hash,
			Bytes:    item.Bytes,
			Host:     item.Host,
			Split:    item.Split,
			Progress: item.Progress,
			Status:   item.Status,
			Added:    parseTime(item.Added),
			Ended:    parseTime(item.Ended),
			Links:    item.Links,
		})
	}
	return out, nil
}

func (c *Client) TorrentInfo(ctx context.Context, id string) (models.TorrentInfo, error) {
	var raw struct {
		ID               string  `json:"id"`
		Filename         string  `json:"filename"`
		OriginalFilename string  `json:"original_filename"`
		Hash             string  `json:"hash"`
		Bytes            int64   `json:"bytes"`
		OriginalBytes    int64   `json:"original_bytes"`
		Host             string  `json:"host"`
		Split            int     `json:"split"`
		Progress         float64 `json:"progress"`
		Status           string  `json:"status"`
		Added            string  `json:"added"`
		Ended            string  `json:"ended"`
		Speed            int64   `json:"speed"`
		Seeders          int     `json:"seeders"`
		Links            []string
		Files            []struct {
			ID       int    `json:"id"`
			Path     string `json:"path"`
			Bytes    int64  `json:"bytes"`
			Selected int    `json:"selected"`
		} `json:"files"`
	}
	endpoint := fmt.Sprintf("%s/torrents/info/%s", c.baseURL, url.PathEscape(id))
	if err := c.doJSON(ctx, http.MethodGet, endpoint, nil, "", &raw); err != nil {
		return models.TorrentInfo{}, err
	}
	out := models.TorrentInfo{
		Torrent: models.Torrent{
			ID:       raw.ID,
			Filename: raw.Filename,
			Hash:     raw.Hash,
			Bytes:    raw.Bytes,
			Host:     raw.Host,
			Split:    raw.Split,
			Progress: raw.Progress,
			Status:   raw.Status,
			Added:    parseTime(raw.Added),
			Ended:    parseTime(raw.Ended),
			Links:    raw.Links,
		},
		OriginalFilename: raw.OriginalFilename,
		OriginalBytes:    raw.OriginalBytes,
		Seeders:          raw.Seeders,
		Speed:            raw.Speed,
	}
	for _, file := range raw.Files {
		out.Files = append(out.Files, models.TorrentFile{
			ID:       file.ID,
			Path:     file.Path,
			Bytes:    file.Bytes,
			Selected: file.Selected == 1,
		})
	}
	return out, nil
}

func (c *Client) AddMagnet(ctx context.Context, magnet string) (models.AddTorrentResult, error) {
	values := url.Values{}
	values.Set("magnet", magnet)
	var out models.AddTorrentResult
	err := c.doJSON(ctx, http.MethodPost, c.baseURL+"/torrents/addMagnet", strings.NewReader(values.Encode()), "application/x-www-form-urlencoded", &out)
	return out, err
}

func (c *Client) AddTorrentFile(ctx context.Context, filename string, data []byte) (models.AddTorrentResult, error) {
	var out models.AddTorrentResult
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, c.baseURL+"/torrents/addTorrent", bytes.NewReader(data))
	if err != nil {
		return out, err
	}
	c.authorize(req)
	contentType := "application/x-bittorrent"
	if extType := mime.TypeByExtension(filepath.Ext(filename)); extType != "" {
		contentType = extType
	}
	req.Header.Set("Content-Type", contentType)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return out, err
	}
	defer resp.Body.Close()
	if err := c.decodeResponse(resp, &out, http.StatusCreated); err != nil {
		return out, err
	}
	return out, nil
}

func (c *Client) AddTorrentPath(ctx context.Context, path string) (models.AddTorrentResult, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return models.AddTorrentResult{}, fmt.Errorf("read torrent %s: %w", path, err)
	}
	return c.AddTorrentFile(ctx, filepath.Base(path), data)
}

func (c *Client) AddTorrentURL(ctx context.Context, torrentURL string) (models.AddTorrentResult, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, torrentURL, nil)
	if err != nil {
		return models.AddTorrentResult{}, err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return models.AddTorrentResult{}, fmt.Errorf("fetch torrent url: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return models.AddTorrentResult{}, &apiError{StatusCode: resp.StatusCode, Message: "failed to fetch torrent url"}
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return models.AddTorrentResult{}, fmt.Errorf("read torrent url: %w", err)
	}
	return c.AddTorrentFile(ctx, torrentURL, data)
}

func (c *Client) SelectFiles(ctx context.Context, torrentID string, fileIDs []int) error {
	values := url.Values{}
	if len(fileIDs) == 0 {
		values.Set("files", "all")
	} else {
		parts := make([]string, 0, len(fileIDs))
		for _, id := range fileIDs {
			parts = append(parts, strconv.Itoa(id))
		}
		values.Set("files", strings.Join(parts, ","))
	}
	endpoint := fmt.Sprintf("%s/torrents/selectFiles/%s", c.baseURL, url.PathEscape(torrentID))
	resp, err := c.do(ctx, http.MethodPost, endpoint, strings.NewReader(values.Encode()), "application/x-www-form-urlencoded")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return c.expectStatus(resp, http.StatusNoContent, http.StatusAccepted)
}

func (c *Client) DeleteTorrent(ctx context.Context, torrentID string) error {
	endpoint := fmt.Sprintf("%s/torrents/delete/%s", c.baseURL, url.PathEscape(torrentID))
	resp, err := c.do(ctx, http.MethodDelete, endpoint, nil, "")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return c.expectStatus(resp, http.StatusNoContent)
}

func (c *Client) UnrestrictLink(ctx context.Context, link string) (models.UnrestrictedLink, error) {
	values := url.Values{}
	values.Set("link", link)
	var out models.UnrestrictedLink
	err := c.doJSON(ctx, http.MethodPost, c.baseURL+"/unrestrict/link", strings.NewReader(values.Encode()), "application/x-www-form-urlencoded", &out)
	return out, err
}

func (c *Client) StartDeviceAuth(ctx context.Context, clientID string) (models.DeviceCode, error) {
	if clientID == "" {
		clientID = OpenSourceClientID
	}
	endpoint := fmt.Sprintf("%s/device/code?client_id=%s&new_credentials=yes", c.oauthBaseURL, url.QueryEscape(clientID))
	var out models.DeviceCode
	if err := c.doJSON(ctx, http.MethodGet, endpoint, nil, "", &out); err != nil {
		return models.DeviceCode{}, err
	}
	out.RequestedAt = time.Now()
	return out, nil
}

func (c *Client) PollDeviceCredentials(ctx context.Context, clientID, deviceCode string) (string, string, error) {
	endpoint := fmt.Sprintf("%s/device/credentials?client_id=%s&code=%s", c.oauthBaseURL, url.QueryEscape(clientID), url.QueryEscape(deviceCode))
	resp, err := c.do(ctx, http.MethodGet, endpoint, nil, "")
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		var apiErr ErrorResponse
		if json.Unmarshal(body, &apiErr) == nil {
			if strings.Contains(strings.ToLower(apiErr.Error), "pending") || strings.Contains(strings.ToLower(apiErr.Error), "denied") || resp.StatusCode == http.StatusNotFound {
				return "", "", ErrAuthorizationPending
			}
			return "", "", &apiError{StatusCode: resp.StatusCode, Message: apiErr.Error, Code: apiErr.ErrorCode}
		}
		return "", "", &apiError{StatusCode: resp.StatusCode, Message: strings.TrimSpace(string(body))}
	}
	var raw struct {
		ClientID     string `json:"client_id"`
		ClientSecret string `json:"client_secret"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return "", "", err
	}
	return raw.ClientID, raw.ClientSecret, nil
}

func (c *Client) ExchangeDeviceToken(ctx context.Context, clientID, clientSecret, code string) (models.DeviceCredentials, error) {
	return c.tokenRequest(ctx, clientID, clientSecret, code)
}

func (c *Client) RefreshToken(ctx context.Context, clientID, clientSecret, refreshToken string) (models.DeviceCredentials, error) {
	return c.tokenRequest(ctx, clientID, clientSecret, refreshToken)
}

func (c *Client) tokenRequest(ctx context.Context, clientID, clientSecret, code string) (models.DeviceCredentials, error) {
	values := url.Values{}
	values.Set("client_id", clientID)
	values.Set("client_secret", clientSecret)
	values.Set("code", code)
	values.Set("grant_type", deviceGrantType)
	var raw struct {
		AccessToken  string `json:"access_token"`
		ExpiresIn    int    `json:"expires_in"`
		TokenType    string `json:"token_type"`
		RefreshToken string `json:"refresh_token"`
	}
	if err := c.doJSON(ctx, http.MethodPost, c.oauthBaseURL+"/token", strings.NewReader(values.Encode()), "application/x-www-form-urlencoded", &raw); err != nil {
		return models.DeviceCredentials{}, err
	}
	now := time.Now()
	return models.DeviceCredentials{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		AccessToken:  raw.AccessToken,
		RefreshToken: raw.RefreshToken,
		TokenType:    raw.TokenType,
		ExpiresAt:    now.Add(time.Duration(raw.ExpiresIn) * time.Second),
		CreatedAt:    now,
	}, nil
}

func (c *Client) doJSON(ctx context.Context, method, endpoint string, body io.Reader, contentType string, target any) error {
	resp, err := c.do(ctx, method, endpoint, body, contentType)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return c.decodeResponse(resp, target, http.StatusOK)
}

func (c *Client) do(ctx context.Context, method, endpoint string, body io.Reader, contentType string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return nil, err
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	c.authorize(req)
	return c.httpClient.Do(req)
}

func (c *Client) authorize(req *http.Request) {
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
}

func (c *Client) decodeResponse(resp *http.Response, target any, okStatuses ...int) error {
	if err := c.expectStatus(resp, okStatuses...); err != nil {
		return err
	}
	if target == nil || resp.StatusCode == http.StatusNoContent {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

func (c *Client) expectStatus(resp *http.Response, okStatuses ...int) error {
	for _, status := range okStatuses {
		if resp.StatusCode == status {
			return nil
		}
	}
	body, _ := io.ReadAll(resp.Body)
	var apiErr ErrorResponse
	if json.Unmarshal(body, &apiErr) == nil && apiErr.Error != "" {
		return &apiError{StatusCode: resp.StatusCode, Message: apiErr.Error, Code: apiErr.ErrorCode}
	}
	return &apiError{StatusCode: resp.StatusCode, Message: strings.TrimSpace(string(body))}
}

func parseTime(value string) time.Time {
	if value == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339, value)
	if err == nil {
		return t
	}
	return time.Time{}
}
