package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/m4rii0/rdtui/internal/app"
	"github.com/m4rii0/rdtui/internal/debug"
	"github.com/m4rii0/rdtui/internal/demo"
	"github.com/m4rii0/rdtui/internal/tui"
	updater "github.com/m4rii0/rdtui/internal/update"
	"github.com/m4rii0/rdtui/internal/version"
)

func main() {
	debug.Init()
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
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
		deps.Auth, deps.Torrents, deps.Downloads, deps.Desktop = svc, svc, downloadAdapter{svc}, svc
		deps.Batches, deps.Bulk, deps.PrepareDownload = svc.NewBatches(), svc.NewBulkQueue(runtime), svc.PrepareDownload
		closeService = svc.CloseContext
	} else {
		svc, err := app.New(runtime.Context())
		if err != nil {
			return fmt.Errorf("failed to initialize app: %w", err)
		}
		closeService = svc.CloseContext
		deps.Auth, deps.Torrents, deps.Downloads, deps.Desktop = svc, svc.Torrents(), downloadAdapter{svc}, svc
		deps.Batches, deps.Bulk, deps.PrepareDownload = svc.NewBatches(), svc.NewBulkQueue(runtime), svc.PrepareDownload
	}
	defer func() { err = errors.Join(err, cleanupRuntime(runtime, closeService)) }()

	program := tea.NewProgram(tui.NewModel(deps), tea.WithContext(ctx))
	if _, err := program.Run(); err != nil {
		return fmt.Errorf("rdtui error: %w", err)
	}
	return nil
}

// Keep the old facade's method name out of consuming feature interfaces.
type downloadAdapter struct {
	downloadFacade
}

type downloadFacade interface {
	Start(context.Context, string, string) (app.DownloadStart, error)
	Status(context.Context, app.DownloadRef) (app.DownloadSnapshot, error)
	CurrentDownload() (app.DownloadSnapshot, bool)
}

func (a downloadAdapter) Current() (app.DownloadSnapshot, bool) { return a.CurrentDownload() }

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
