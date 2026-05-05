package realdebrid

import (
	"context"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/m4rii0/rdtui/internal/version"
)

func TestClientUserSuccess(t *testing.T) {
	setTestVersion(t, "1.2.3")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer token-123" {
			t.Fatalf("unexpected auth header: %q", got)
		}
		if got := r.Header.Get("User-Agent"); got != "rdtui/1.2.3" {
			t.Fatalf("unexpected user-agent header: %q", got)
		}
		if r.URL.Path != "/user" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_, _ = io.WriteString(w, `{"id":42,"username":"alice","email":"a@example.com","points":5,"locale":"en","avatar":"","type":"premium","premium":10,"expiration":"2030-01-01T00:00:00Z"}`)
	}))
	defer server.Close()

	client := NewClient("token-123", server.URL, server.URL)
	user, err := client.User(context.Background())
	if err != nil {
		t.Fatalf("User() error = %v", err)
	}
	if user.Username != "alice" {
		t.Fatalf("username = %q, want alice", user.Username)
	}
}

func TestParseTimeUsesLocalTimezoneWhenMissingOffset(t *testing.T) {
	loc := time.FixedZone("TEST", 2*60*60)
	original := time.Local
	time.Local = loc
	t.Cleanup(func() { time.Local = original })

	got := parseTime("2026-05-05T23:23:00")
	want := time.Date(2026, 5, 5, 23, 23, 0, 0, loc)
	if !got.Equal(want) || got.Location() != loc {
		t.Fatalf("parseTime() = %v (%v), want %v (%v)", got, got.Location(), want, loc)
	}
}

func TestParseTimeUsesLocalWallTimeWhenUTCMarked(t *testing.T) {
	loc := time.FixedZone("TEST", 2*60*60)
	original := time.Local
	time.Local = loc
	t.Cleanup(func() { time.Local = original })

	got := parseTime("2026-05-05T21:23:00Z")
	want := time.Date(2026, 5, 5, 21, 23, 0, 0, loc)
	if !got.Equal(want) || got.Location() != loc {
		t.Fatalf("parseTime() = %v (%v), want %v (%v)", got, got.Location(), want, loc)
	}
}

func TestClientListTorrentsParsesIntegerProgressResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/torrents" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_, _ = io.WriteString(w, `[{"id":"abc","filename":"Example","hash":"h","bytes":123,"host":"rd","split":50,"progress":100,"status":"downloaded","added":"2030-01-01T00:00:00Z","links":["https://example/link"]}]`)
	}))
	defer server.Close()

	client := NewClient("token-123", server.URL, server.URL)
	torrents, err := client.ListTorrents(context.Background(), 10)
	if err != nil {
		t.Fatalf("ListTorrents() error = %v", err)
	}
	if len(torrents) != 1 {
		t.Fatalf("len(torrents) = %d, want 1", len(torrents))
	}
	if torrents[0].Filename != "Example" || torrents[0].Progress != 100 {
		t.Fatalf("unexpected torrent: %+v", torrents[0])
	}
}

func TestClientListTorrentsParsesFloatProgressResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/torrents" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_, _ = io.WriteString(w, `[{"id":"abc","filename":"Example","hash":"h","bytes":123,"host":"rd","split":50,"progress":99.67,"status":"downloading","added":"2030-01-01T00:00:00Z","links":[]}]`)
	}))
	defer server.Close()

	client := NewClient("token-123", server.URL, server.URL)
	torrents, err := client.ListTorrents(context.Background(), 10)
	if err != nil {
		t.Fatalf("ListTorrents() error = %v", err)
	}
	if len(torrents) != 1 {
		t.Fatalf("len(torrents) = %d, want 1", len(torrents))
	}
	if math.Abs(torrents[0].Progress-99.67) > 0.0001 {
		t.Fatalf("progress = %v, want 99.67", torrents[0].Progress)
	}
}

func TestClientTorrentInfoParsesFloatProgressResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/torrents/info/abc" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_, _ = io.WriteString(w, `{"id":"abc","filename":"Example","original_filename":"Example","hash":"h","bytes":123,"original_bytes":123,"host":"rd","split":50,"progress":99.97,"status":"downloading","added":"2030-01-01T00:00:00Z","files":[],"links":[]}`)
	}))
	defer server.Close()

	client := NewClient("token-123", server.URL, server.URL)
	info, err := client.TorrentInfo(context.Background(), "abc")
	if err != nil {
		t.Fatalf("TorrentInfo() error = %v", err)
	}
	if math.Abs(info.Progress-99.97) > 0.0001 {
		t.Fatalf("progress = %v, want 99.97", info.Progress)
	}
}

func TestClientAddMagnetAcceptsCreatedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/torrents/addMagnet" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer token-123" {
			t.Fatalf("unexpected auth header: %q", got)
		}
		if got := r.Header.Get("Content-Type"); !strings.Contains(got, "application/x-www-form-urlencoded") {
			t.Fatalf("content-type = %q", got)
		}
		body, _ := io.ReadAll(r.Body)
		if got := string(body); got != "magnet=magnet%3A%3Fxt%3Durn%3Abtih%3Aabc%26tr%3Dudp%3A%2F%2Ftracker.example%2Fannounce" {
			t.Fatalf("body = %q", got)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"id":"D6YQSTIYHWEJC","uri":"https://api.real-debrid.com/rest/1.0/torrents/info/D6YQSTIYHWEJC"}`)
	}))
	defer server.Close()

	client := NewClient("token-123", server.URL, server.URL)
	result, err := client.AddMagnet(context.Background(), "magnet:?xt=urn:btih:abc&tr=udp://tracker.example/announce")
	if err != nil {
		t.Fatalf("AddMagnet() error = %v", err)
	}
	if result.ID != "D6YQSTIYHWEJC" {
		t.Fatalf("id = %q, want D6YQSTIYHWEJC", result.ID)
	}
}

func TestClientAddTorrentFileSetsUserAgent(t *testing.T) {
	setTestVersion(t, "1.2.3")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/torrents/addTorrent" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != http.MethodPut {
			t.Fatalf("method = %s, want PUT", r.Method)
		}
		if got := r.Header.Get("User-Agent"); got != "rdtui/1.2.3" {
			t.Fatalf("unexpected user-agent header: %q", got)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"id":"D6YQSTIYHWEJC","uri":"https://api.real-debrid.com/rest/1.0/torrents/info/D6YQSTIYHWEJC"}`)
	}))
	defer server.Close()

	client := NewClient("token-123", server.URL, server.URL)
	result, err := client.AddTorrentFile(context.Background(), "example.torrent", []byte("torrent data"))
	if err != nil {
		t.Fatalf("AddTorrentFile() error = %v", err)
	}
	if result.ID != "D6YQSTIYHWEJC" {
		t.Fatalf("id = %q, want D6YQSTIYHWEJC", result.ID)
	}
}

func TestClientUnrestrictLinkUsesFormData(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/unrestrict/link" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Content-Type"); !strings.Contains(got, "application/x-www-form-urlencoded") {
			t.Fatalf("content-type = %q", got)
		}
		body, _ := io.ReadAll(r.Body)
		if got := string(body); got != "link=https%3A%2F%2Fexample.com%2Ffile" {
			t.Fatalf("body = %q", got)
		}
		_, _ = io.WriteString(w, `{"id":"1","filename":"video.mkv","link":"https://example.com/file","download":"https://cdn.example.com/video.mkv"}`)
	}))
	defer server.Close()

	client := NewClient("token-123", server.URL, server.URL)
	link, err := client.UnrestrictLink(context.Background(), "https://example.com/file")
	if err != nil {
		t.Fatalf("UnrestrictLink() error = %v", err)
	}
	if link.Download != "https://cdn.example.com/video.mkv" {
		t.Fatalf("download url = %q", link.Download)
	}
}

func TestClientReturnsAPIErrorOnUnauthorized(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"error":"Bad token"}`)
	}))
	defer server.Close()

	client := NewClient("bad-token", server.URL, server.URL)
	_, err := client.User(context.Background())
	if err == nil {
		t.Fatal("User() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "Bad token") {
		t.Fatalf("error = %v, want API message", err)
	}
}

func TestClientReturnsAPIErrorOnRateLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = io.WriteString(w, `{"error":"Too many requests"}`)
	}))
	defer server.Close()

	client := NewClient("token-123", server.URL, server.URL)
	_, err := client.UnrestrictLink(context.Background(), "https://example.com/file")
	if err == nil {
		t.Fatal("UnrestrictLink() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "Too many requests") {
		t.Fatalf("error = %v, want API message", err)
	}
}

func setTestVersion(t *testing.T, value string) {
	t.Helper()
	previous := version.Version
	version.Version = value
	t.Cleanup(func() {
		version.Version = previous
	})
}
