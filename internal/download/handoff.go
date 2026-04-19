package download

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

func CopyURL(value string) (bool, error) {
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
		cmd := exec.Command(candidate[0], candidate[1:]...)
		cmd.Stdin = strings.NewReader(value)
		if err := cmd.Run(); err == nil {
			return true, nil
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
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("open file: %w", err)
	}
	cmd, err := openCommand(path, false)
	if err != nil {
		return err
	}
	return cmd.Run()
}

func RevealInDirectory(path string) error {
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("reveal file: %w", err)
	}
	cmd, err := openCommand(path, true)
	if err != nil {
		return err
	}
	return cmd.Run()
}

func openCommand(path string, reveal bool) (*exec.Cmd, error) {
	switch runtime.GOOS {
	case "darwin":
		if reveal {
			return exec.Command("open", "-R", path), nil
		}
		return exec.Command("open", path), nil
	case "linux":
		if reveal {
			return exec.Command("xdg-open", filepath.Dir(path)), nil
		}
		return exec.Command("xdg-open", path), nil
	case "windows":
		if reveal {
			return exec.Command("explorer", "/select,", path), nil
		}
		return exec.Command("cmd", "/c", "start", "", path), nil
	default:
		return nil, fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
	}
}
