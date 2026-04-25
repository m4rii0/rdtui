package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
)

type updateOptions struct {
	DryRun bool
}

type commandHandlers struct {
	runTUI      func() error
	checkUpdate func(context.Context, io.Writer) error
	update      func(context.Context, io.Writer, updateOptions) error
}

var errUsage = errors.New("usage error")

func newRootCommand(ctx context.Context, stdout io.Writer, handlers commandHandlers) *cobra.Command {
	// Cobra's Windows mousetrap exits with a CLI-only prompt when launched from Explorer.
	// rdtui supports direct launch into the TUI, so keep that guard disabled.
	cobra.MousetrapHelpText = ""

	var showVersion bool
	var updateOpts updateOptions

	root := &cobra.Command{
		Use:           "rdtui",
		Short:         "Manage Real-Debrid torrents from your terminal",
		Args:          noArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if showVersion {
				return printVersion(cmd.OutOrStdout())
			}
			return handlers.runTUI()
		},
	}
	root.SetOut(stdout)
	root.Flags().BoolVar(&showVersion, "version", false, "print version information")
	root.SetFlagErrorFunc(flagError)

	checkUpdate := &cobra.Command{
		Use:   "check-update",
		Short: "Check for a newer stable release",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return handlers.checkUpdate(ctx, cmd.OutOrStdout())
		},
	}
	checkUpdate.SetFlagErrorFunc(flagError)

	update := &cobra.Command{
		Use:   "update",
		Short: "Download and install a newer stable release",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return handlers.update(ctx, cmd.OutOrStdout(), updateOpts)
		},
	}
	update.Flags().BoolVar(&updateOpts.DryRun, "dry-run", false, "preview the update without replacing files")
	update.SetFlagErrorFunc(flagError)

	root.AddCommand(checkUpdate, update)
	return root
}

func noArgs(cmd *cobra.Command, args []string) error {
	if len(args) > 0 {
		return usageError("%s does not accept arguments: %s", cmd.CommandPath(), strings.Join(args, " "))
	}
	return nil
}

func flagError(cmd *cobra.Command, err error) error {
	return usageError("%v", err)
}

func usageError(format string, args ...any) error {
	return fmt.Errorf("%w: "+format, append([]any{errUsage}, args...)...)
}

func isUsageError(err error) bool {
	if errors.Is(err, errUsage) {
		return true
	}
	message := err.Error()
	return strings.HasPrefix(message, "unknown command ")
}
