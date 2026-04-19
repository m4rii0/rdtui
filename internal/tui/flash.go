package tui

import (
	"time"

	tea "charm.land/bubbletea/v2"
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
	var style = infoStyle
	switch m.flash.level {
	case flashSuccess:
		style = okStyle
	case flashError:
		style = errorStyle
	case flashWarn:
		style = warnStyle
	}
	return style.Render("  ▸ " + m.flash.message)
}
