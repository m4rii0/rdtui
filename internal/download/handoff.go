package download

import (
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/mario/real-debrid/pkg/models"
)

var ErrNoCommandConfigured = errors.New("no external downloader configured")

type TemplateData struct {
	URL      string
	Dir      string
	Filename string
}

func RenderCommand(template []string, data TemplateData) ([]string, error) {
	if len(template) == 0 {
		return nil, ErrNoCommandConfigured
	}
	out := make([]string, 0, len(template))
	replacer := strings.NewReplacer(
		"{{url}}", data.URL,
		"{{dir}}", data.Dir,
		"{{filename}}", data.Filename,
	)
	for _, part := range template {
		rendered := replacer.Replace(part)
		if strings.TrimSpace(rendered) == "" {
			continue
		}
		out = append(out, rendered)
	}
	if len(out) == 0 {
		return nil, ErrNoCommandConfigured
	}
	return out, nil
}

func Launch(template []string, data TemplateData) (models.HandoffResult, error) {
	cmdArgs, err := RenderCommand(template, data)
	if err != nil {
		return models.HandoffResult{}, err
	}
	cmd := exec.Command(cmdArgs[0], cmdArgs[1:]...)
	if err := cmd.Start(); err != nil {
		return models.HandoffResult{}, fmt.Errorf("launch downloader: %w", err)
	}
	return models.HandoffResult{URL: data.URL, Launched: true, Command: cmdArgs}, nil
}

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
