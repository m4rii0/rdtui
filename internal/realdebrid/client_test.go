package realdebrid

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/m4rii0/rdtui/internal/version"
	"github.com/m4rii0/rdtui/pkg/models"
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
	user, err := client.User(t.Context())
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
	torrents, err := client.ListTorrents(t.Context(), 10)
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
	torrents, err := client.ListTorrents(t.Context(), 10)
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
		_, _ = io.WriteString(w, `{"id":"abc","filename":"Example","original_filename":"Original","hash":"h","bytes":123,"original_bytes":456,"host":"rd","split":50,"progress":99.97,"status":"downloading","added":"2030-01-01T00:00:00Z","ended":"2030-01-02T00:00:00Z","seeders":3,"speed":1024,"files":[{"id":1,"path":"movie.mkv","bytes":123,"selected":1}],"links":["https://example.com/link"]}`)
	}))
	defer server.Close()

	client := NewClient("token-123", server.URL, server.URL)
	info, err := client.TorrentInfo(t.Context(), "abc")
	if err != nil {
		t.Fatalf("TorrentInfo() error = %v", err)
	}
	if math.Abs(info.Progress-99.97) > 0.0001 {
		t.Fatalf("progress = %v, want 99.97", info.Progress)
	}
	want := models.TorrentInfo{
		ID:               "abc",
		Filename:         "Example",
		Hash:             "h",
		Bytes:            123,
		Host:             "rd",
		Split:            50,
		Progress:         99.97,
		Status:           "downloading",
		Added:            time.Date(2030, 1, 1, 0, 0, 0, 0, time.Local),
		Ended:            time.Date(2030, 1, 2, 0, 0, 0, 0, time.Local),
		Links:            []string{"https://example.com/link"},
		OriginalFilename: "Original",
		OriginalBytes:    456,
		Seeders:          3,
		Speed:            1024,
		Files:            []models.TorrentFile{{ID: 1, Path: "movie.mkv", Bytes: 123, Selected: true}},
	}
	if !reflect.DeepEqual(info, want) {
		t.Errorf("TorrentInfo() = %+v, want %+v", info, want)
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
	result, err := client.AddMagnet(t.Context(), "magnet:?xt=urn:btih:abc&tr=udp://tracker.example/announce")
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
	result, err := client.AddTorrentFile(t.Context(), "example.torrent", []byte("torrent data"))
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
	link, err := client.UnrestrictLink(t.Context(), "https://example.com/file")
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
	_, err := client.User(t.Context())
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
	_, err := client.UnrestrictLink(t.Context(), "https://example.com/file")
	if err == nil {
		t.Fatal("UnrestrictLink() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "Too many requests") {
		t.Fatalf("error = %v, want API message", err)
	}
}

func TestErrorResponseJSONOmitsZeroCode(t *testing.T) {
	for _, tt := range []struct {
		code int
		want string
	}{
		{0, `{"error":"failure"}`},
		{42, `{"error":"failure","error_code":42}`},
	} {
		response := ErrorResponse{Error: "failure", ErrorCode: tt.code}
		data, err := json.Marshal(response)
		if err != nil {
			t.Fatalf("Marshal() error = %v", err)
		}
		if string(data) != tt.want {
			t.Errorf("Marshal(%+v) = %s, want %s", response, data, tt.want)
		}
	}
}

func TestTokenSourceUsedForEveryAuthenticatedRequest(t *testing.T) {
	authorizations := make(chan string, 3)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case authorizations <- r.Header.Get("Authorization"):
		default:
			t.Error("unexpected extra request")
		}
		if r.Method == http.MethodPut {
			w.WriteHeader(http.StatusCreated)
		}
		_, _ = io.WriteString(w, `{}`)
	}))
	t.Cleanup(server.Close)
	sourceCalls := 0
	client := NewClient("stale-token", server.URL, server.URL,
		WithHTTPClient(server.Client()),
		WithTokenSource(func(ctx context.Context) (string, error) {
			if ctx != t.Context() {
				t.Error("token source did not receive request context")
			}
			sourceCalls++
			return "refreshed-token-" + strconv.Itoa(sourceCalls), nil
		}))
	for range 2 {
		if _, err := client.User(t.Context()); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := client.AddTorrentFile(t.Context(), "example.torrent", []byte("torrent data")); err != nil {
		t.Fatal(err)
	}
	if len(authorizations) != 3 {
		t.Fatalf("requests = %d, want 3", len(authorizations))
	}
	for _, want := range []string{"Bearer refreshed-token-1", "Bearer refreshed-token-2", "Bearer refreshed-token-3"} {
		if got := <-authorizations; got != want {
			t.Errorf("Authorization = %q, want %q", got, want)
		}
	}
	if sourceCalls != 3 {
		t.Fatalf("token source calls = %d, want 3", sourceCalls)
	}
}

func TestTokenSourceCancellationStopsRequest(t *testing.T) {
	for _, sourceErr := range []error{context.Canceled, errors.New("credentials unavailable")} {
		t.Run(sourceErr.Error(), func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			t.Cleanup(cancel)
			requests := 0
			client := NewClient("fallback-must-not-be-used", "", "",
				WithHTTPClient(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
					requests++
					return nil, errors.New("unexpected request")
				})}),
				WithTokenSource(func(sourceCtx context.Context) (string, error) {
					if sourceCtx != ctx {
						t.Error("token source did not receive request context")
					}
					if errors.Is(sourceErr, context.Canceled) {
						cancel()
						<-sourceCtx.Done()
						return "", sourceCtx.Err()
					}
					return "", sourceErr
				}))
			_, err := client.User(ctx)
			if !errors.Is(err, sourceErr) || requests != 0 {
				t.Fatalf("User() error = %v, requests = %d", err, requests)
			}
		})
	}
}

func TestOAuthClientDoesNotInvokeTokenSource(t *testing.T) {
	sourceCalls, handlerCalls, requests := 0, 0, 0
	client := NewClient("must-not-leak", "", "",
		WithTokenSource(func(context.Context) (string, error) {
			sourceCalls++
			return "", errors.New("recursive token lookup")
		}),
		WithAuthErrorHandler(func(string, *APIError) error { handlerCalls++; return nil }),
		WithHTTPClient(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			requests++
			if got := r.Header.Get("Authorization"); got != "" {
				t.Errorf("OAuth Authorization = %q", got)
			}
			return &http.Response{StatusCode: http.StatusUnauthorized, Body: io.NopCloser(strings.NewReader(`{"error":"Bad token","error_code":8}`))}, nil
		})}))
	_, _ = client.StartDeviceAuth(t.Context(), "")
	_, _, _ = client.PollDeviceCredentials(t.Context(), "client", "device")
	_, _ = client.ExchangeDeviceToken(t.Context(), "client", "secret", "device")
	_, _ = client.RefreshToken(t.Context(), "client", "secret", "refresh")
	if sourceCalls != 0 || handlerCalls != 0 || requests != 4 {
		t.Fatalf("source calls = %d, handler calls = %d, requests = %d", sourceCalls, handlerCalls, requests)
	}
}

func TestAPIErrorPreservesStatusAndCode(t *testing.T) {
	for _, tt := range []struct {
		name, body, message, wantError string
		status, code                   int
	}{
		{"unauthorized", `{"error":"Bad token","error_code":8}`, "Bad token", "real-debrid error 401 (8): Bad token", 401, 8},
		{"rate limit", `{"error":"Too many requests","error_code":5}`, "Too many requests", "real-debrid error 429 (5): Too many requests", 429, 5},
		{"plain text", " upstream failure\n", "upstream failure", "real-debrid error 502: upstream failure", 502, 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			client := NewClient("token", "", "", WithHTTPClient(&http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: tt.status, Body: io.NopCloser(strings.NewReader(tt.body))}, nil
			})}))
			_, err := client.User(t.Context())
			apiErr, ok := errors.AsType[*APIError](err)
			if !ok || apiErr.StatusCode != tt.status || apiErr.Code != tt.code || apiErr.Message != tt.message || err.Error() != tt.wantError {
				t.Fatalf("User() error = %#v, want %s", err, tt.wantError)
			}
		})
	}
}

func TestAuthErrorHandlerReceivesSentToken(t *testing.T) {
	for _, handlerErr := range []error{nil, errors.New("authentication required")} {
		t.Run(strconv.FormatBool(handlerErr != nil), func(t *testing.T) {
			token := "sent-token"
			var gotToken string
			var gotAPIError *APIError
			handlerCalls := 0
			client := NewClient("static-token", "", "",
				WithTokenSource(func(context.Context) (string, error) { return token, nil }),
				WithHTTPClient(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
					if got := r.Header.Get("Authorization"); got != "Bearer sent-token" {
						t.Errorf("Authorization = %q", got)
					}
					token = "newer-token"
					return &http.Response{StatusCode: http.StatusUnauthorized, Body: io.NopCloser(strings.NewReader(`{"error":"Bad token","error_code":8}`))}, nil
				})}),
				WithAuthErrorHandler(func(sent string, apiErr *APIError) error {
					handlerCalls++
					gotToken, gotAPIError = sent, apiErr
					return handlerErr
				}))
			_, err := client.User(t.Context())
			apiErr, ok := errors.AsType[*APIError](err)
			if !ok || apiErr != gotAPIError || apiErr.StatusCode != 401 || apiErr.Code != 8 || gotToken != "sent-token" || handlerCalls != 1 {
				t.Fatalf("error = %v, handler token = %q, calls = %d", err, gotToken, handlerCalls)
			}
			if handlerErr != nil && !errors.Is(err, handlerErr) {
				t.Fatalf("handler error not preserved: %v", err)
			}
		})
	}
}

func TestMutationNotAutomaticallyReplayed(t *testing.T) {
	transportErr := errors.New("connection lost after sending mutation")
	for _, tt := range []struct {
		name             string
		status           int
		transportErr     error
		wantHandlerCalls int
	}{
		{"rejection", 401, nil, 1},
		{"rate limit", 429, nil, 0},
		{"permission denied", 403, nil, 0},
		{"transport failure", 0, transportErr, 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			mutationCalls, handlerCalls := 0, 0
			client := NewClient("sent-token", "", "",
				WithHTTPClient(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
					mutationCalls++
					if r.Method != http.MethodPost || r.URL.Path != "/rest/1.0/torrents/addMagnet" {
						t.Errorf("unexpected mutation: %s %s", r.Method, r.URL.Path)
					}
					if tt.transportErr != nil {
						return nil, tt.transportErr
					}
					return &http.Response{StatusCode: tt.status, Body: io.NopCloser(strings.NewReader(`{"error":"rejected"}`))}, nil
				})}),
				WithAuthErrorHandler(func(string, *APIError) error { handlerCalls++; return nil }))
			_, err := client.AddMagnet(t.Context(), "magnet:?xt=urn:btih:abc")
			if err == nil || mutationCalls != 1 || handlerCalls != tt.wantHandlerCalls {
				t.Fatalf("error = %v, mutation calls = %d, handler calls = %d", err, mutationCalls, handlerCalls)
			}
			if tt.transportErr != nil && !errors.Is(err, tt.transportErr) {
				t.Fatalf("transport error not preserved: %v", err)
			}
		})
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func setTestVersion(t *testing.T, value string) {
	t.Helper()
	previous := version.Version
	version.Version = value
	t.Cleanup(func() {
		version.Version = previous
	})
}
