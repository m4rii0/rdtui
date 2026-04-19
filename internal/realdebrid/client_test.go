package realdebrid

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClientUserSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer token-123" {
			t.Fatalf("unexpected auth header: %q", got)
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

func TestClientListTorrentsParsesResponse(t *testing.T) {
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
