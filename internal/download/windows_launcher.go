package download

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

const desktopActionEnv = "RDTUI_INTERNAL_DESKTOP_ACTION"
const maxDesktopPathBytes = 128 * 1024 // More than a Windows 32,767 UTF-16-unit path.

// Keep this constructor portable so its process/data boundary is testable even
// without Windows. No filename is ever serialized as command-line text.
func windowsOpenCommand(ctx context.Context, path string, reveal bool) (*exec.Cmd, error) {
	executable, err := os.Executable()
	if err != nil {
		return nil, err
	}
	action := "open"
	if reveal {
		action = "reveal"
	}
	cmd := exec.CommandContext(ctx, executable)
	cmd.Env = append(os.Environ(), desktopActionEnv+"="+action)
	cmd.Stdin = strings.NewReader(path)
	return cmd, nil
}

// RunDesktopHelper handles the private Windows child-process protocol before
// CLI/runtime startup. Only the helper is owned and killable by the parent; the
// native Shell APIs hand applications off without returning an owned process.
func RunDesktopHelper(input io.Reader) (handled bool, err error) {
	action := os.Getenv(desktopActionEnv)
	if runtime.GOOS != "windows" || action == "" {
		return false, nil
	}
	// Do not let intentionally launched applications inherit the helper mode.
	if err := os.Unsetenv(desktopActionEnv); err != nil {
		return true, err
	}
	return true, consumeDesktopPath(input, action, nativeDesktop)
}

func consumeDesktopPath(input io.Reader, action string, launch func(string, bool) error) error {
	if action != "open" && action != "reveal" {
		return fmt.Errorf("invalid internal desktop action")
	}
	data, err := io.ReadAll(io.LimitReader(input, maxDesktopPathBytes+1))
	if err != nil {
		return err
	}
	path := string(data)
	// Quotes and NUL are not valid Windows filename characters. Reject them as
	// data, rather than attempting any shell escaping or quote interpretation.
	if path == "" || len(data) > maxDesktopPathBytes || strings.ContainsAny(path, "\x00\"") {
		return fmt.Errorf("invalid Windows desktop path")
	}
	return launch(path, action == "reveal")
}
