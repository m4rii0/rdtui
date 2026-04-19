package main

import (
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"
	"github.com/m4rii0/rdtui/internal/app"
	"github.com/m4rii0/rdtui/internal/debug"
	"github.com/m4rii0/rdtui/internal/demo"
	"github.com/m4rii0/rdtui/internal/tui"
	"github.com/m4rii0/rdtui/internal/version"
)

func main() {
	debug.Init()

	if len(os.Args) > 1 && os.Args[1] == "--version" {
		fmt.Printf("rdtui %s\n", version.Version)
		return
	}

	var service app.AppService
	if os.Getenv("RDTUI_DEMO") == "1" {
		service = demo.NewService()
	} else {
		svc, err := app.New()
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to initialize app: %v\n", err)
			os.Exit(1)
		}
		defer func() {
			_ = svc.Close()
		}()
		service = svc
	}

	program := tea.NewProgram(tui.NewModel(service))
	if _, err := program.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "rdtui error: %v\n", err)
		os.Exit(1)
	}
}
