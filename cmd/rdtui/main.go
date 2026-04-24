package main

import (
	"context"
	"fmt"
	"io"
	"os"

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
	root := newRootCommand(context.Background(), stdout, commandHandlers{
		runTUI: func() error {
			return runTUI()
		},
		checkUpdate: runCheckUpdate,
		update:      runUpdate,
	})
	root.SetArgs(args)
	root.SetOut(stdout)
	root.SetErr(stderr)

	err := root.Execute()
	if err != nil {
		fmt.Fprintf(stderr, "%v\n", err)
		if isUsageError(err) {
			fmt.Fprintf(stderr, "\n%s", root.UsageString())
			return 2
		}
		return 1
	}
	return 0
}

func runTUI() error {
	var service app.AppService
	if os.Getenv("RDTUI_DEMO") == "1" {
		service = demo.NewService()
	} else {
		svc, err := app.New()
		if err != nil {
			return fmt.Errorf("failed to initialize app: %w", err)
		}
		defer func() {
			_ = svc.Close()
		}()
		service = svc
	}

	program := tea.NewProgram(tui.NewModel(service))
	if _, err := program.Run(); err != nil {
		return fmt.Errorf("rdtui error: %w", err)
	}
	return nil
}

func printVersion(stdout io.Writer) error {
	fmt.Fprintf(stdout, "rdtui %s\n", version.Current())
	return nil
}

func runCheckUpdate(ctx context.Context, stdout io.Writer) error {
	res, err := updater.Check(ctx, updater.CheckOptions{CurrentVersion: version.Current()})
	if err != nil {
		return fmt.Errorf("check update failed: %w", err)
	}
	if res.UpdateAvailable {
		fmt.Fprintf(stdout, "rdtui %s is available (current %s)\n", res.LatestVersion, res.CurrentVersion)
		fmt.Fprintf(stdout, "asset: %s\n", res.AssetName)
		fmt.Fprintln(stdout, "run: rdtui update")
		return nil
	}
	fmt.Fprintf(stdout, "rdtui is up to date (%s)\n", res.CurrentVersion)
	return nil
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
		fmt.Fprintf(stdout, "rdtui is up to date (%s); no update installed\n", res.CurrentVersion)
		return nil
	}
	if opts.DryRun {
		fmt.Fprintf(stdout, "would update rdtui from %s to %s using %s; no files changed\n", res.CurrentVersion, res.LatestVersion, res.AssetName)
		return nil
	}
	if res.WindowsManual {
		fmt.Fprintf(stdout, "verified update downloaded: %s\n", res.ReplacementPath)
		fmt.Fprintf(stdout, "replace current executable manually: %s\n", res.TargetPath)
		return nil
	}
	if res.Installed {
		fmt.Fprintf(stdout, "updated rdtui from %s to %s\n", res.CurrentVersion, res.LatestVersion)
		return nil
	}
	return fmt.Errorf("update did not complete")
}
