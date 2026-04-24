package update

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	DefaultAPIBaseURL = "https://api.github.com"
	DefaultOwner      = "m4rii0"
	DefaultRepo       = "rdtui"

	checksumsAssetName = "checksums.txt"
)

type Asset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
	State              string `json:"state"`
}

type Release struct {
	TagName string  `json:"tag_name"`
	HTMLURL string  `json:"html_url"`
	Assets  []Asset `json:"assets"`
}

type Client struct {
	HTTPClient *http.Client
	BaseURL    string
	Owner      string
	Repo       string
}

func NewClient() *Client {
	return &Client{
		HTTPClient: &http.Client{Timeout: 30 * time.Second},
		BaseURL:    DefaultAPIBaseURL,
		Owner:      DefaultOwner,
		Repo:       DefaultRepo,
	}
}

func (c *Client) LatestRelease(ctx context.Context) (Release, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.latestReleaseURL(), nil)
	if err != nil {
		return Release{}, err
	}
	setHeaders(req)

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return Release{}, fmt.Errorf("get latest release: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return Release{}, fmt.Errorf("get latest release: GitHub returned %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}

	var release Release
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return Release{}, fmt.Errorf("decode latest release: %w", err)
	}
	if release.TagName == "" {
		return Release{}, fmt.Errorf("decode latest release: missing tag_name")
	}
	return release, nil
}

func (c *Client) Download(ctx context.Context, rawURL string) ([]byte, error) {
	if rawURL == "" {
		return nil, fmt.Errorf("download URL is empty")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	setHeaders(req)

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("download %s: %w", rawURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("download %s: returned %s: %s", rawURL, resp.Status, strings.TrimSpace(string(body)))
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("download %s: %w", rawURL, err)
	}
	return data, nil
}

func (c *Client) latestReleaseURL() string {
	base := c.BaseURL
	if base == "" {
		base = DefaultAPIBaseURL
	}
	owner := c.Owner
	if owner == "" {
		owner = DefaultOwner
	}
	repo := c.Repo
	if repo == "" {
		repo = DefaultRepo
	}
	return fmt.Sprintf("%s/repos/%s/%s/releases/latest", strings.TrimRight(base, "/"), url.PathEscape(owner), url.PathEscape(repo))
}

func (c *Client) httpClient() *http.Client {
	if c != nil && c.HTTPClient != nil {
		return c.HTTPClient
	}
	return http.DefaultClient
}

func setHeaders(req *http.Request) {
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "rdtui-updater")
}
