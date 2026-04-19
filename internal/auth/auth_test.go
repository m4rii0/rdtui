package auth

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/mario/real-debrid/internal/config"
	"github.com/mario/real-debrid/pkg/models"
)

func TestBootstrapPrefersConfiguredPrivateToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Header.Get("Authorization") {
		case "Bearer config-token":
			_, _ = io.WriteString(w, `{"id":1,"username":"token-user","email":"t@example.com","points":0,"locale":"en","avatar":"","type":"premium","premium":1,"expiration":"2030-01-01T00:00:00Z"}`)
		case "Bearer device-token":
			_, _ = io.WriteString(w, `{"id":2,"username":"device-user","email":"d@example.com","points":0,"locale":"en","avatar":"","type":"premium","premium":1,"expiration":"2030-01-01T00:00:00Z"}`)
		default:
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, `{"error":"Bad token"}`)
		}
	}))
	defer server.Close()

	store := config.NewStoreAt(filepath.Join(t.TempDir(), "rdtui"))
	cfg := config.Config{PrivateToken: "config-token", APIBaseURL: server.URL, OAuthBaseURL: server.URL}
	if err := store.SaveConfig(cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}
	if err := store.SaveDeviceCredentials(models.DeviceCredentials{AccessToken: "device-token", ClientID: "cid", ClientSecret: "secret", RefreshToken: "refresh", ExpiresAt: time.Now().Add(1 * time.Hour)}); err != nil {
		t.Fatalf("SaveDeviceCredentials() error = %v", err)
	}

	manager := NewManager(store, cfg)
	session, err := manager.Bootstrap(context.Background())
	if err != nil {
		t.Fatalf("Bootstrap() error = %v", err)
	}
	if session.Method != models.AuthMethodToken {
		t.Fatalf("method = %s, want token", session.Method)
	}
	if session.User.Username != "token-user" {
		t.Fatalf("username = %q, want token-user", session.User.Username)
	}
}
