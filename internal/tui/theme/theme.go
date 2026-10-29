// Package theme — ranglar va stillarning yagona manbasi. TUI kodida rang
// to'g'ridan-to'g'ri yozilmaydi, faqat shu yerdagi semantik nomlar ishlatiladi.
// NO_COLOR va terminal imkoniyatlarini Bubble Tea o'zi hisobga oladi.
package theme

import "charm.land/lipgloss/v2"

var (
	colorAccent   = lipgloss.Color("#7aa2f7")
	colorOK       = lipgloss.Color("#9ece6a")
	colorWarn     = lipgloss.Color("#e0af68")
	colorErr      = lipgloss.Color("#f7768e")
	colorMuted    = lipgloss.Color("#737aa2")
	colorText     = lipgloss.Color("#c0caf5")
	colorTrack    = lipgloss.Color("#3b4261")
	colorSelectBg = lipgloss.Color("#2e3c64")
	colorBarBg    = lipgloss.Color("#1f2335")
	colorOnAccent = lipgloss.Color("#1a1b26")
)

// Semantik stillar.
var (
	Text     = lipgloss.NewStyle().Foreground(colorText)
	Muted    = lipgloss.NewStyle().Foreground(colorMuted)
	Accent   = lipgloss.NewStyle().Foreground(colorAccent)
	OK       = lipgloss.NewStyle().Foreground(colorOK)
	Warn     = lipgloss.NewStyle().Foreground(colorWarn)
	Err      = lipgloss.NewStyle().Foreground(colorErr)
	Bold     = lipgloss.NewStyle().Bold(true).Foreground(colorText)
	Heading  = lipgloss.NewStyle().Bold(true).Foreground(colorAccent)
	Header   = lipgloss.NewStyle().Bold(true).Foreground(colorMuted)
	Track    = lipgloss.NewStyle().Foreground(colorTrack)
	Selected = lipgloss.NewStyle().Bold(true).Foreground(colorText).Background(colorSelectBg)

	TabActive   = lipgloss.NewStyle().Bold(true).Foreground(colorOnAccent).Background(colorAccent)
	TabInactive = lipgloss.NewStyle().Foreground(colorText).Background(colorBarBg)
	Bar         = lipgloss.NewStyle().Foreground(colorText).Background(colorBarBg)
	Key         = lipgloss.NewStyle().Bold(true).Foreground(colorAccent)
)

// Foiz chegaralari: shundan yuqorisi sariq / qizil.
const (
	warnPercent = 60
	errPercent  = 85
)

// ForPercent yuklanish foiziga mos stil.
func ForPercent(p float64) lipgloss.Style {
	switch {
	case p >= errPercent:
		return Err
	case p >= warnPercent:
		return Warn
	}
	return OK
}
