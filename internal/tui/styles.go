package tui

import "charm.land/lipgloss/v2"

// ── Cyberpunk palette ────────────────────────────────────────────────────────

const (
	colorNeon    = lipgloss.ANSIColor(213) // #ff87ff  — primary accent / headers
	colorViolet  = lipgloss.ANSIColor(141) // #af87ff  — active, info, search
	colorAmber   = lipgloss.ANSIColor(220) // #ffdf00  — warnings, highlights
	colorGreen   = lipgloss.ANSIColor(42)  // #00d75f  — success, downloaded
	colorRed     = lipgloss.ANSIColor(203) // #ff5f5f  — errors, danger
	colorMuted   = lipgloss.ANSIColor(244) // #808080  — secondary text
	colorSubtle  = lipgloss.ANSIColor(240) // #585858  — separators, dim chrome
	colorDarkBg  = lipgloss.ANSIColor(236) // #303030  — panel bg, row alt
	colorPurplBg = lipgloss.ANSIColor(62)  // #5f5faf  — selected row bg
	colorText    = lipgloss.ANSIColor(252) // #d0d0d0  — normal foreground
)

// ── Base styles ──────────────────────────────────────────────────────────────

var (
	appStyle = lipgloss.NewStyle().Padding(1, 2)

	// Header / title
	headStyle   = lipgloss.NewStyle().Bold(true).Foreground(colorNeon)
	versionStyle = lipgloss.NewStyle().Foreground(colorMuted)
	userStyle   = lipgloss.NewStyle().Foreground(colorViolet)

	// Body text
	textStyle  = lipgloss.NewStyle().Foreground(colorText)
	mutedStyle = lipgloss.NewStyle().Foreground(colorMuted)
	subtleStyle = lipgloss.NewStyle().Foreground(colorSubtle)

	// Status
	errorStyle = lipgloss.NewStyle().Foreground(colorRed).Bold(true)
	okStyle    = lipgloss.NewStyle().Foreground(colorGreen).Bold(true)
	warnStyle  = lipgloss.NewStyle().Foreground(colorAmber).Bold(true)
	infoStyle  = lipgloss.NewStyle().Foreground(colorViolet).Bold(true)

	// Table rows
	selectedStyle     = lipgloss.NewStyle().Foreground(colorAmber).Background(colorPurplBg).Bold(true)
	batchMarkedStyle  = lipgloss.NewStyle().Foreground(colorAmber).Background(colorDarkBg)
	altRowStyle       = lipgloss.NewStyle().Background(lipgloss.ANSIColor(235))
	headerRowStyle    = lipgloss.NewStyle().Foreground(colorText).Bold(true)
	headerSelColStyle = lipgloss.NewStyle().Foreground(colorNeon).Bold(true).Underline(true)

	// Status labels
	statusDownloadedStyle  = lipgloss.NewStyle().Foreground(colorGreen).Bold(true)
	statusDownloadingStyle = lipgloss.NewStyle().Foreground(colorViolet).Bold(true)
	statusWaitingStyle     = lipgloss.NewStyle().Foreground(colorAmber)
	statusErrorStyle       = lipgloss.NewStyle().Foreground(colorRed).Bold(true)

	// Search
	searchStyle         = lipgloss.NewStyle().Foreground(colorViolet)
	matchHighlightStyle = lipgloss.NewStyle().Bold(true).Foreground(colorAmber).Underline(true)

	// Footer
	footerKeyStyle  = lipgloss.NewStyle().Bold(true).Foreground(colorNeon)
	footerDescStyle = lipgloss.NewStyle().Foreground(colorMuted)
	footerSepStyle  = lipgloss.NewStyle().Foreground(colorSubtle)
	selectedCountStyle = lipgloss.NewStyle().Bold(true).Foreground(colorAmber)

	// Panels / boxes (rounded cyberpunk style)
	boxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorNeon).
			Padding(0, 1)

	panelStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorViolet).
			Padding(0, 1)

	// Popup styles
	popupBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorViolet).
			Padding(0, 1)

	dangerStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorRed).
			Padding(0, 1)
)

// ── ASCII banner ─────────────────────────────────────────────────────────────

// appBanner is the figlet-style "rdtui" header.
const appBannerRaw = "" +
	"  ██████  ██████  ████████ ██    ██ ██\n" +
	"  ██   ██ ██   ██    ██    ██    ██ ██\n" +
	"  ██████  ██   ██    ██    ██    ██ ██\n" +
	"  ██   ██ ██   ██    ██    ██    ██ ██\n" +
	"  ██   ██ ██████     ██     ██████  ██"

// renderBanner returns the styled ASCII banner with optional version string.
func renderBanner(version string) string {
	banner := headStyle.Render(appBannerRaw)
	if version != "" {
		banner += "\n" + versionStyle.Render("  v"+version)
	}
	return banner
}

// renderCompactHeader returns a single-line styled header for main/detail views.
func renderCompactHeader(version, username string) string {
	h := headStyle.Render("◆ rdtui")
	if version != "" {
		h += " " + versionStyle.Render("v"+version)
	}
	if username != "" {
		h += "  " + subtleStyle.Render("▸") + " " + userStyle.Render(username)
	}
	return h
}

// ── Decorator helpers ────────────────────────────────────────────────────────

// sectionTitle renders a decorated section title: ◆ Title
func sectionTitle(title string) string {
	return headStyle.Render("◆ "+title)
}

// dividerLine renders a full-width ── divider in subtle color.
func dividerLine(width int) string {
	if width <= 0 {
		width = 40
	}
	line := ""
	for i := 0; i < width; i++ {
		line += "─"
	}
	return subtleStyle.Render(line)
}

// sectionDivider renders:  ─── Label ───
func sectionDivider(label string, width int) string {
	if label == "" {
		return dividerLine(width)
	}
	decorated := "── " + label + " ──"
	padLen := width - len([]rune(decorated))
	if padLen > 0 {
		for i := 0; i < padLen; i++ {
			decorated += "─"
		}
	}
	return subtleStyle.Render(decorated)
}
