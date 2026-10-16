package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/m4rii0/rdtui/internal/app"
	"github.com/m4rii0/rdtui/internal/aria2"
	"github.com/m4rii0/rdtui/internal/config"
	"github.com/m4rii0/rdtui/internal/debug"
	"github.com/m4rii0/rdtui/internal/demo"
	"github.com/m4rii0/rdtui/internal/directdl"
	"github.com/m4rii0/rdtui/internal/download"
	"github.com/m4rii0/rdtui/internal/tui"
	updater "github.com/m4rii0/rdtui/internal/update"
	"github.com/m4rii0/rdtui/internal/version"
	"github.com/m4rii0/rdtui/pkg/models"
)

func main() {
	debug.Init()
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if handled, err := download.RunDesktopHelper(os.Stdin); handled {
		if err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return 1
		}
		return 0
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	runtime := app.NewRuntime(ctx)
	defer runtime.Stop()
	root := newRootCommand(runtime.Context(), stdout, commandHandlers{
		runTUI: func() error {
			return runTUI(ctx, runtime)
		},
		checkUpdate: func(ctx context.Context, stdout io.Writer) error {
			return runtime.Do(ctx, func(ctx context.Context) error { return runCheckUpdate(ctx, stdout) })
		},
		update: func(ctx context.Context, stdout io.Writer, opts updateOptions) error {
			return runtime.Do(ctx, func(ctx context.Context) error { return runUpdate(ctx, stdout, opts) })
		},
	})
	root.SetArgs(args)
	root.SetOut(stdout)
	root.SetErr(stderr)

	err := root.Execute()
	if err != nil {
		// Reporting is best-effort: there is no other output if stderr fails.
		_, _ = fmt.Fprintf(stderr, "%v\n", err)
		if isUsageError(err) {
			_, _ = fmt.Fprintf(stderr, "\n%s", root.UsageString())
			return 2
		}
		return 1
	}
	return 0
}

func runTUI(ctx context.Context, runtime *app.Runtime) (err error) {
	deps := tui.Dependencies{Runtime: runtime}
	var closeService func(context.Context) error
	if os.Getenv("RDTUI_DEMO") == "1" {
		svc := demo.NewService(runtime.Context())
		deps.Auth, deps.Torrents, deps.Downloads, deps.Desktop = svc, svc, svc, svc
		deps.AuthState = svc
		deps.Batches, deps.Bulk, deps.PrepareDownload = svc.NewBatches(), svc.NewBulkQueue(runtime), svc.PrepareDownload
		closeService = svc.Close
	} else {
		options, err := productionOptions()
		if err != nil {
			return fmt.Errorf("failed to initialize app: %w", err)
		}
		application, err := app.NewApplication(runtime.Context(), options)
		if err != nil {
			return fmt.Errorf("failed to initialize app: %w", err)
		}
		closeService = application.Close
		deps = applicationDependencies(application)
	}
	defer func() { err = errors.Join(err, cleanupRuntime(runtime, closeService)) }()

	program := tea.NewProgram(tui.NewModel(deps), tea.WithContext(ctx))
	if _, err := program.Run(); err != nil {
		return fmt.Errorf("rdtui error: %w", err)
	}
	return nil
}

func applicationDependencies(a *app.Application) tui.Dependencies {
	return tui.Dependencies{Runtime: a.Runtime, Auth: a.Auth, AuthState: a.Auth, Torrents: a.Torrents,
		Downloads: a.Downloads, Batches: a.Batches, Bulk: a.Bulk, Desktop: a.Desktop,
		PrepareDownload: a.PrepareDownload}
}

// Process inputs and production defaults are captured only at the entry point.
func productionOptions() (app.ApplicationOptions, error) {
	store, err := config.NewStore()
	if err != nil {
		return app.ApplicationOptions{}, err
	}
	home, _ := os.UserHomeDir() // Configuration resolution reports a missing required home.
	return app.ApplicationOptions{
		Store: store, Home: home,
		Overrides: config.Overrides{PrivateToken: os.Getenv("RDTUI_PRIVATE_TOKEN"),
			APIBaseURL: os.Getenv("RDTUI_API_BASE_URL"), OAuthBaseURL: os.Getenv("RDTUI_OAUTH_BASE_URL")},
		HTTPClient: &http.Client{Timeout: 30 * time.Second}, Now: time.Now,
		Wait: func(ctx context.Context, delay time.Duration) error {
			timer := time.NewTimer(delay)
			defer timer.Stop()
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-timer.C:
				return nil
			}
		},
		Stat: downloadFileSize, BackendFactory: productionBackend,
	}, nil
}

func productionBackend(_ context.Context, cfg config.EffectiveConfig) (download.Backend, error) {
	if cfg.DownloadBackend == models.DownloadBackendAria2 {
		return aria2.NewManager(cfg.Aria2BinaryPath), nil
	}
	return directdl.NewManager(), nil
}

func downloadFileSize(path string) (int64, bool, error) {
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	if info.IsDir() {
		return 0, false, fmt.Errorf("download target is directory: %s", path)
	}
	return info.Size(), true, nil
}

func cleanupRuntime(runtime *app.Runtime, closeService func(context.Context) error) error {
	runtime.Stop()
	// Shutdown must not inherit the canceled signal/program context.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	var closeErr error
	if closeService != nil {
		closeErr = closeService(ctx)
	}
	return errors.Join(closeErr, runtime.Wait(ctx))
}

func printVersion(stdout io.Writer) error {
	_, err := fmt.Fprintf(stdout, "rdtui %s\n", version.Current())
	return err
}

func runCheckUpdate(ctx context.Context, stdout io.Writer) error {
	res, err := updater.Check(ctx, updater.CheckOptions{CurrentVersion: version.Current()})
	if err != nil {
		return fmt.Errorf("check update failed: %w", err)
	}
	if res.UpdateAvailable {
		_, err := fmt.Fprintf(stdout, "rdtui %s is available (current %s)\nasset: %s\nrun: rdtui update\n", res.LatestVersion, res.CurrentVersion, res.AssetName)
		return err
	}
	_, err = fmt.Fprintf(stdout, "rdtui is up to date (%s)\n", res.CurrentVersion)
	return err
}

func runUpdate(ctx context.Context, stdout io.Writer, opts updateOptions) error {
	res, err := updater.Update(ctx, updater.UpdateOptions{
		CurrentVersion: version.Current(),
		DryRun:         opts.DryRun,
	})
	if err != nil {
		return fmt.Errorf("update failed: %w", err)
	}
	if !res.UpdateAvailable {
		_, err := fmt.Fprintf(stdout, "rdtui is up to date (%s); no update installed\n", res.CurrentVersion)
		return err
	}
	if opts.DryRun {
		_, err := fmt.Fprintf(stdout, "would update rdtui from %s to %s using %s; no files changed\n", res.CurrentVersion, res.LatestVersion, res.AssetName)
		return err
	}
	if res.WindowsManual {
		_, err := fmt.Fprintf(stdout, "verified update downloaded: %s\nreplace current executable manually: %s\n", res.ReplacementPath, res.TargetPath)
		return err
	}
	if res.Installed {
		_, err := fmt.Fprintf(stdout, "updated rdtui from %s to %s\n", res.CurrentVersion, res.LatestVersion)
		return err
	}
	return fmt.Errorf("update did not complete")
}
