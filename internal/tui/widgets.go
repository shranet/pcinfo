package tui

import (
	"math"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/shranet/pcinfo/internal/tui/theme"
)

const (
	ellipsis   = "…"
	barFull    = "█"
	barEmpty   = "░"
	sparkRunes = "▁▂▃▄▅▆▇█"
	cellGap    = " "
)

// cell — jadval katakchasi. width 0 — qolgan hamma joy.
type cell struct {
	text    string
	style   lipgloss.Style
	width   int
	isRight bool
}

// fit matnni aniq width ustunga sig'diradi: kesadi yoki bo'sh joy bilan to'ldiradi.
func fit(s string, width int, isRight bool) string {
	if width <= 0 {
		return ""
	}
	s = ansi.Truncate(s, width, ellipsis)
	pad := strings.Repeat(" ", max(0, width-ansi.StringWidth(s)))
	if isRight {
		return pad + s
	}
	return s + pad
}

// renderRow katakchalarni bitta qatorga yig'adi. Tanlangan qator butunlay
// bitta stil bilan chiziladi: ichki ranglar fonni buzmasin.
func renderRow(cells []cell, width int, isSelected bool) string {
	fixed := 0
	for _, c := range cells {
		fixed += c.width + len(cellGap)
	}
	var parts []string
	for _, c := range cells {
		w := c.width
		if w == 0 {
			w = max(0, width-fixed)
		}
		text := fit(c.text, w, c.isRight)
		if !isSelected {
			text = c.style.Render(text)
		}
		parts = append(parts, text)
	}
	line := strings.Join(parts, cellGap)
	if isSelected {
		return theme.Selected.Render(fit(line, width, false))
	}
	return line
}

// truncateLine stillangan qatorni ekran kengligida kesadi.
func truncateLine(s string, width int) string {
	return ansi.Truncate(s, width, ellipsis)
}

// bar — [█████░░░░] ko'rinishidagi indikator.
func bar(frac float64, width int, style lipgloss.Style) string {
	if width <= 0 {
		return ""
	}
	frac = math.Max(0, math.Min(1, frac))
	full := int(math.Round(frac * float64(width)))
	return style.Render(strings.Repeat(barFull, full)) + theme.Track.Render(strings.Repeat(barEmpty, width-full))
}

// sparkline oxirgi width qiymatni chizadi; top — shkala tepasi (0 bo'lsa eng katta qiymat).
func sparkline(values []float64, width int, top float64) string {
	if width <= 0 || len(values) == 0 {
		return ""
	}
	if len(values) > width {
		values = values[len(values)-width:]
	}
	if top <= 0 {
		for _, v := range values {
			top = math.Max(top, v)
		}
	}
	runes := []rune(sparkRunes)
	var b strings.Builder
	for _, v := range values {
		i := 0
		if top > 0 {
			i = int(math.Round(math.Max(0, math.Min(1, v/top)) * float64(len(runes)-1)))
		}
		b.WriteRune(runes[i])
	}
	return b.String()
}

// pushHistory tarixga qiymat qo'shadi va uni limit bilan cheklaydi.
func pushHistory(h []float64, v float64, limit int) []float64 {
	h = append(h, v)
	if len(h) > limit {
		h = h[len(h)-limit:]
	}
	return h
}

// listState — tanlangan qator va aylantirish holati.
type listState struct {
	cursor int
	offset int
}

func (l *listState) clamp(n, height int) {
	l.cursor = max(0, min(l.cursor, n-1))
	if height <= 0 {
		return
	}
	if l.cursor < l.offset {
		l.offset = l.cursor
	}
	if l.cursor >= l.offset+height {
		l.offset = l.cursor - height + 1
	}
	l.offset = max(0, min(l.offset, n-height))
}

func (l *listState) move(delta, n, height int) {
	l.cursor += delta
	l.clamp(n, height)
}

// scroll ko'rinadigan oynani suradi (g'ildirak), kursor oyna ichida qoladi.
func (l *listState) scroll(delta, n, height int) {
	l.offset = max(0, min(l.offset+delta, n-height))
	l.cursor = max(l.offset, min(l.cursor, l.offset+height-1))
	l.clamp(n, height)
}

// navKey umumiy ro'yxat tugmalarini qo'llaydi; tugma tanilsa true.
func (l *listState) navKey(key string, n, height int) bool {
	switch key {
	case "up", "k":
		l.move(-1, n, height)
	case "down", "j":
		l.move(1, n, height)
	case "pgup":
		l.move(-height, n, height)
	case "pgdown":
		l.move(height, n, height)
	case "home", "g":
		l.move(-n, n, height)
	case "end", "G":
		l.move(n, n, height)
	default:
		return false
	}
	return true
}

// visible — ko'rinadigan qatorlar indekslari [from, to).
func (l listState) visible(n, height int) (int, int) {
	from := min(l.offset, n)
	return from, min(n, from+height)
}

// padLines ro'yxatni aniq height qatorga keltiradi.
func padLines(lines []string, height int) []string {
	if len(lines) > height {
		return lines[:height]
	}
	for len(lines) < height {
		lines = append(lines, "")
	}
	return lines
}
