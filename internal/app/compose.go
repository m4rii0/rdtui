package app

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/m4rii0/rdtui/internal/auth"
	"github.com/m4rii0/rdtui/internal/config"
	"github.com/m4rii0/rdtui/internal/download"
	"github.com/m4rii0/rdtui/internal/realdebrid"
	"github.com/m4rii0/rdtui/pkg/models"
)

type ApplicationOptions struct {
	Store          *config.Store
	Overrides      config.Overrides
	Home           string
	HTTPClient     *http.Client
	Now            func() time.Time
	Wait           WaitFunc
	Stat           func(string) (int64, bool, error)
	Desktop        Desktop
	BackendFactory func(context.Context, config.EffectiveConfig) (download.Backend, error)
}

// Application owns the composed components; callers use feature contracts rather
// than a second facade with its own session, client or active-download state.
type Application struct {
	Runtime   *Runtime
	Auth      *Authentication
	Torrents  *TorrentService
	Downloads *DownloadCoordinator
	Batches   *BatchService
	Bulk      *BulkQueue
	Desktop   Desktop
	Config    *config.Repository
	prepare   func(context.Context, models.DownloadTarget) (PreparedDownload, error)
}

func NewApplication(ctx context.Context, options ApplicationOptions) (*Application, error) {
	if options.Store == nil || options.HTTPClient == nil || options.Stat == nil || options.BackendFactory == nil {
		return nil, fmt.Errorf("application requires store, HTTP client, stat and backend factory")
	}
	prefs, err := config.NewRepository(options.Store, options.Overrides, options.Home)
	if err != nil {
		return nil, err
	}
	cfg := prefs.Effective()
	runtime := NewRuntime(ctx)
	desktop := options.Desktop
	if desktop == nil {
		desktop = download.NewSystemDesktop(runtime.Context())
	}
	backend, err := options.BackendFactory(runtime.Context(), cfg)
	if err != nil {
		runtime.Stop()
		return nil, fmt.Errorf("initialize download backend: %w", err)
	}
	manager := auth.NewManager(auth.SessionDependencies{
		Root: runtime.Context(), Spawn: runtime.Spawn,
		OAuth: realdebrid.NewClient("", cfg.APIBaseURL, cfg.OAuthBaseURL,
			realdebrid.WithHTTPClient(options.HTTPClient), realdebrid.WithNow(options.Now)),
		Validate: func(ctx context.Context, token string) (models.User, error) {
			return realdebrid.NewClient(token, cfg.APIBaseURL, cfg.OAuthBaseURL, realdebrid.WithHTTPClient(options.HTTPClient)).User(ctx)
		},
		Credentials: options.Store, Preferences: prefs, Now: options.Now,
	})
	client := realdebrid.NewClient("", cfg.APIBaseURL, cfg.OAuthBaseURL,
		realdebrid.WithHTTPClient(options.HTTPClient), realdebrid.WithTokenSource(manager.Token),
		realdebrid.WithAuthErrorHandler(func(token string, _ *realdebrid.APIError) error {
			if manager.RejectToken(token) {
				return auth.ErrNoValidCredentials
			}
			return nil
		}))
	a := &Application{
		Runtime: runtime, Auth: &Authentication{manager: manager}, Config: prefs,
		Torrents: NewTorrentService(client), Desktop: desktop,
		Downloads: NewDownloadCoordinator(runtime.Context(), backend, cfg.DefaultDownloadDir),
		Batches:   NewBatchService(client, desktop, options.Wait),
	}
	a.prepare = func(ctx context.Context, target models.DownloadTarget) (PreparedDownload, error) {
		return PrepareDownload(ctx, target, cfg.DefaultDownloadDir,
			func(ctx context.Context, target models.DownloadTarget) (models.UnrestrictedLink, error) {
				return client.UnrestrictLink(ctx, target.Link)
			}, options.Stat)
	}
	a.Bulk = NewBulkQueue(QueueDependencies{Runtime: runtime, Downloads: a.Downloads, Prepare: a.PrepareDownload, Wait: options.Wait})
	return a, nil
}

func (a *Application) PrepareDownload(ctx context.Context, target models.DownloadTarget) (PreparedDownload, error) {
	return a.prepare(ctx, target)
}

// Close closes admission and cancels before either backend or runtime draining.
// The caller supplies one independent deadline for both waits; repeated calls
// observe the same eventual completion, even after a previous deadline expired.
func (a *Application) Close(ctx context.Context) error {
	a.Runtime.Stop()
	return errors.Join(a.Downloads.Close(ctx), a.Runtime.Wait(ctx))
}
