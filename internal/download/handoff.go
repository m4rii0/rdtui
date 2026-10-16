package download

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const helperTimeout = 5 * time.Second

// SystemDesktop keeps the consumer port context-free while binding production
// helpers to the application root. Each call also has a finite independent cap.
type SystemDesktop struct{ root context.Context }

func NewSystemDesktop(root context.Context) *SystemDesktop { return &SystemDesktop{root: root} }

func (d *SystemDesktop) CopyURL(value string) (bool, error) { return CopyURLContext(d.root, value) }
func (d *SystemDesktop) OpenFile(path string) error         { return OpenFileContext(d.root, path) }
func (d *SystemDesktop) RevealInDirectory(path string) error {
	return RevealInDirectoryContext(d.root, path)
}

func CopyURL(value string) (bool, error) {
	return CopyURLContext(context.Background(), value)
}

func CopyURLContext(parent context.Context, value string) (bool, error) {
	ctx, cancel := context.WithTimeout(parent, helperTimeout)
	defer cancel()
	commands := [][]string{}
	switch runtime.GOOS {
	case "darwin":
		commands = append(commands, []string{"pbcopy"})
	case "windows":
		commands = append(commands, []string{"clip"})
	default:
		commands = append(commands,
			[]string{"wl-copy"},
			[]string{"xclip", "-selection", "clipboard"},
			[]string{"xsel", "--clipboard", "--input"},
		)
	}
	for _, candidate := range commands {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		cmd := exec.CommandContext(ctx, candidate[0], candidate[1:]...)
		cmd.Stdin = strings.NewReader(value)
		if err := runHelper(ctx, cmd); err == nil {
			return true, nil
		} else if ctx.Err() != nil {
			// Cancellation/timeout must not start another clipboard candidate.
			return false, err
		}
	}
	return false, nil
}

func FilenameForURL(path string, fallback string) string {
	if fallback != "" {
		return fallback
	}
	base := filepath.Base(path)
	if base == "." || base == "/" || base == "" {
		return "download"
	}
	return base
}

func OpenFile(path string) error {
	return OpenFileContext(context.Background(), path)
}

func OpenFileContext(parent context.Context, path string) error {
	ctx, cancel := context.WithTimeout(parent, helperTimeout)
	defer cancel()
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("open file: %w", err)
	}
	cmd, err := openCommand(ctx, path, false)
	if err != nil {
		return err
	}
	return runHelper(ctx, cmd)
}

func RevealInDirectory(path string) error {
	return RevealInDirectoryContext(context.Background(), path)
}

func RevealInDirectoryContext(parent context.Context, path string) error {
	ctx, cancel := context.WithTimeout(parent, helperTimeout)
	defer cancel()
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("reveal file: %w", err)
	}
	cmd, err := openCommand(ctx, path, true)
	if err != nil {
		return err
	}
	return runHelper(ctx, cmd)
}

func runHelper(ctx context.Context, cmd *exec.Cmd) error {
	// Bound pipe-copy waits even if a launched descendant inherits an fd. Kill
	// and wait only for the owned helper PID, never a user's application group.
	cmd.WaitDelay = 100 * time.Millisecond
	err := cmd.Run()
	if ctx.Err() != nil {
		return errors.Join(ctx.Err(), err)
	}
	return err
}

func openCommand(ctx context.Context, path string, reveal bool) (*exec.Cmd, error) {
	switch runtime.GOOS {
	case "darwin":
		if reveal {
			return exec.CommandContext(ctx, "open", "-R", path), nil
		}
		return exec.CommandContext(ctx, "open", path), nil
	case "linux":
		if reveal {
			return exec.CommandContext(ctx, "xdg-open", filepath.Dir(path)), nil
		}
		return exec.CommandContext(ctx, "xdg-open", path), nil
	case "windows":
		if reveal {
			// Wait for the launcher shell, not Explorer itself.
			return exec.CommandContext(ctx, "cmd", "/c", "start", "", "explorer", "/select,", path), nil
		}
		return exec.CommandContext(ctx, "cmd", "/c", "start", "", path), nil
	default:
		return nil, fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
	}
}
