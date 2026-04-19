package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/m4rii0/rdtui/internal/app"
	"github.com/m4rii0/rdtui/internal/tui"
)

func main() {
	service, err := app.New()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to initialize app: %v\n", err)
		os.Exit(1)
	}

	program := tea.NewProgram(tui.NewModel(service), tea.WithAltScreen())
	if _, err := program.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "rdtui error: %v\n", err)
		os.Exit(1)
	}
}
