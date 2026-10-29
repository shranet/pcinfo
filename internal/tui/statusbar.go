package tui

import (
	"fmt"
	"strings"

	"github.com/shranet/pcinfo/internal/collect/modem"
	"github.com/shranet/pcinfo/internal/collect/network"
	"github.com/shranet/pcinfo/internal/format"
	"github.com/shranet/pcinfo/internal/tui/theme"
)

const statusSep = " │ "

// viewStatus — har bir tabda ko'rinadigan qisqa holat qatori.
func (m Model) viewStatus() string {
	parts := []string{m.internetStatus()}
	if m.sys.isLoaded {
		s := m.sys.data
		parts = append(parts, theme.Muted.Render("CPU ")+theme.ForPercent(s.CPU.Total).Render(format.Percent(s.CPU.Total)))
		if s.Memory.Total > 0 {
			used := s.Memory.Total - s.Memory.Available
			pct := float64(used) / float64(s.Memory.Total) * percentScale
			parts = append(parts, theme.Muted.Render("RAM ")+theme.ForPercent(pct).Render(format.Bytes(used)+"/"+format.Bytes(s.Memory.Total)))
		}
	}
	if running, total := m.containerCounts(); total > 0 {
		parts = append(parts, theme.Muted.Render("Konteyner ")+theme.Text.Render(fmt.Sprintf("%d/%d", running, total)))
	}
	if b := m.ussd.balance; b != nil && b.MB != nil {
		parts = append(parts, theme.Muted.Render("USB ")+theme.Text.Render(fmt.Sprintf("%.1f GB", float64(*b.MB)/mbPerGB)))
	}
	if m.modemInfo.isLoaded && m.modemInfo.err == nil {
		style, _ := qualityStyle(modem.RSRPQuality(m.modemInfo.data.Signal.RSRP))
		parts = append(parts, theme.Muted.Render("RSRP ")+style.Render(orDash(m.modemInfo.data.Signal.RSRP)))
	}
	if n := m.warningCount(); n > 0 {
		parts = append(parts, theme.Warn.Render(fmt.Sprintf("⚠ %d", n)))
	}
	return truncateLine(" "+strings.Join(parts, theme.Track.Render(statusSep)), m.width)
}

func (m Model) internetStatus() string {
	if !m.links.isLoaded {
		return theme.Muted.Render("Internet …")
	}
	l, ok := m.activeLink()
	if !ok {
		return theme.Err.Render("Internet yo'q")
	}
	text, style := m.probeText(l.Iface)
	name := theme.OK.Render(kindLabel(l.Kind))
	if l.Kind == network.KindUSBModem {
		name = theme.Warn.Render(kindLabel(l.Kind) + " (zaxira!)")
	}
	return theme.Muted.Render("Internet ") + name + " " + style.Render(text)
}

func (m Model) containerCounts() (running, total int) {
	for _, s := range m.summaries() {
		for _, c := range s.Containers {
			total++
			if c.IsRunning() {
				running++
			}
		}
	}
	return running, total
}

func (m Model) warningCount() int {
	n := len(m.deps.SelfWarnings)
	if m.claude.isLoaded {
		for _, p := range m.claude.data.Projects.Projects {
			n += len(p.Warnings)
		}
	}
	return n
}
