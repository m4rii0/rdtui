package tui

import (
	"time"

	"github.com/charmbracelet/lipgloss"
	tea "github.com/charmbracelet/bubbletea"
)

type flashLevel int

const (
	flashInfo flashLevel = iota
	flashSuccess
	flashWarn
	flashError
)

type flashState struct {
	message string
	level   flashLevel
	setAt   time.Time
}

type flashTimeoutMsg time.Time

const flashDuration = 3 * time.Second

func (m *Model) setFlash(level flashLevel, msg string) tea.Cmd {
	m.flash = flashState{
		message: msg,
		level:   level,
		setAt:   time.Now(),
	}
	return tea.Tick(flashDuration, func(t time.Time) tea.Msg {
		return flashTimeoutMsg(t)
	})
}

func (m *Model) clearFlash() {
	m.flash = flashState{}
}

func renderFlash(m Model) string {
	if m.flash.message == "" {
		return ""
	}
	var style lipgloss.Style
	switch m.flash.level {
	case flashSuccess:
		style = okStyle
	case flashError:
		style = errorStyle
	case flashWarn:
		style = lipgloss.NewStyle().Foreground(lipgloss.Color("220")).Bold(true)
	default:
		style = lipgloss.NewStyle().Foreground(lipgloss.Color("111")).Bold(true)
	}
	return style.Render(m.flash.message)
}
