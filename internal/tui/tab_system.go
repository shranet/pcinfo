package tui

import (
	"fmt"
	"strings"

	"github.com/shranet/pcinfo/internal/collect/system"
	"github.com/shranet/pcinfo/internal/format"
	"github.com/shranet/pcinfo/internal/tui/theme"
)

const (
	labelWidth   = 8
	diskLabelW   = 14
	coreCellW    = 24
	coreLabelW   = 4
	corePctW     = 6
	minBarWidth  = 10
	sideTextW    = 28
	percentScale = 100
	tempWarnC    = 70
	tempErrC     = 85
)

func (m Model) viewSystem() []string {
	switch {
	case m.sys.isFailed():
		return []string{theme.Err.Render("Tizim ma'lumoti o'qilmadi: " + m.sys.err.Error())}
	case !m.sys.isLoaded:
		return []string{theme.Muted.Render("Tizim ma'lumoti yuklanmoqda…")}
	case m.sys.err != nil && m.sys.data.Memory.Total == 0:
		return []string{theme.Err.Render("Tizim ma'lumoti o'qilmadi: " + m.sys.err.Error())}
	}
	s := m.sys.data
	h := s.Host
	lines := []string{
		theme.Bold.Render(h.Hostname) + theme.Muted.Render(fmt.Sprintf("  %s · kernel %s · ishlayapti %s · load %.2f %.2f %.2f · %d jarayon",
			h.Platform, h.Kernel, format.Duration(h.Uptime), h.Load1, h.Load5, h.Load15, h.Procs)),
		"",
	}
	barW := max(minBarWidth, m.width-labelWidth-sideTextW)
	lines = append(lines, gaugeLine("CPU", s.CPU.Total/percentScale, barW, format.Percent(s.CPU.Total)))
	lines = append(lines, strings.Repeat(" ", labelWidth)+theme.Accent.Render(sparkline(m.cpuHist, barW, percentScale)))
	lines = append(lines, coreGrid(s.CPU.PerCore, m.width)...)
	lines = append(lines, "")

	mem := s.Memory
	if mem.Total > 0 {
		used := mem.Total - mem.Available
		lines = append(lines, gaugeLine("RAM", float64(used)/float64(mem.Total), barW,
			fmt.Sprintf("%s / %s", format.Bytes(used), format.Bytes(mem.Total))))
	}
	if mem.SwapTotal > 0 {
		lines = append(lines, gaugeLine("Swap", float64(mem.SwapUsed)/float64(mem.SwapTotal), barW,
			fmt.Sprintf("%s / %s", format.Bytes(mem.SwapUsed), format.Bytes(mem.SwapTotal))))
	}
	lines = append(lines, "", theme.Heading.Render("Disklar"))
	for _, d := range s.Disks {
		lines = append(lines, diskLine(d, barW))
	}
	if len(s.Temps) > 0 {
		lines = append(lines, "", theme.Heading.Render("Temperatura"))
		var parts []string
		for _, t := range s.Temps {
			style := theme.OK
			switch {
			case t.Celsius >= tempErrC:
				style = theme.Err
			case t.Celsius >= tempWarnC:
				style = theme.Warn
			}
			parts = append(parts, theme.Muted.Render(t.Name+" ")+style.Render(fmt.Sprintf("%.0f°C", t.Celsius)))
		}
		lines = append(lines, "  "+strings.Join(parts, "   "))
	}
	if len(m.deps.SelfWarnings) > 0 {
		lines = append(lines, "", theme.Err.Render("Xavfsizlik"))
		for _, w := range m.deps.SelfWarnings {
			lines = append(lines, theme.Err.Render("  ⚠ "+w))
		}
	}
	return lines
}

func gaugeLine(label string, frac float64, width int, text string) string {
	return fit(theme.Bold.Render(label), labelWidth, false) +
		bar(frac, width, theme.ForPercent(frac*percentScale)) + " " + theme.Text.Render(text)
}

func diskLine(d system.Disk, width int) string {
	label := fit(d.Mount, diskLabelW, false)
	return theme.Text.Render(label) + bar(d.Percent/percentScale, width-(diskLabelW-labelWidth), theme.ForPercent(d.Percent)) +
		theme.Text.Render(fmt.Sprintf(" %s / %s ", format.Bytes(d.Used), format.Bytes(d.Total))) +
		theme.Muted.Render(d.FSType+" "+d.Device)
}

// coreGrid yadrolarni ekran kengligiga sig'adigan ustunlarda ko'rsatadi.
func coreGrid(cores []float64, width int) []string {
	perRow := max(1, (width-labelWidth)/coreCellW)
	barW := coreCellW - coreLabelW - corePctW - 1
	var lines []string
	for start := 0; start < len(cores); start += perRow {
		var b strings.Builder
		b.WriteString(strings.Repeat(" ", labelWidth))
		for i := start; i < min(start+perRow, len(cores)); i++ {
			p := cores[i]
			b.WriteString(theme.Muted.Render(fit(fmt.Sprintf("%d", i), coreLabelW-1, true) + " "))
			b.WriteString(bar(p/percentScale, barW, theme.ForPercent(p)))
			b.WriteString(theme.Text.Render(fit(fmt.Sprintf("%.0f%%", p), corePctW, true)) + " ")
		}
		lines = append(lines, b.String())
	}
	return lines
}
