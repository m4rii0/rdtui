package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mario/real-debrid/internal/app"
	"github.com/mario/real-debrid/internal/tui"
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
