package tui

import (
	"fmt"
	"slices"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/shranet/pcinfo/internal/collect/dockerapi"
	"github.com/shranet/pcinfo/internal/format"
	"github.com/shranet/pcinfo/internal/tui/theme"
)

var dockerColumns = []cell{
	{text: "KONTEYNER", width: 30},
	{text: "HOLAT", width: 24},
	{text: "CPU", width: 7, isRight: true},
	{text: "XOTIRA / LIMIT", width: 22, isRight: true},
	{text: "COMPOSE", width: 14},
	{text: "IMAGE"},
}

// summaries — claude tomoni va shu foydalanuvchi ko'radigan Docker'lar. TUI claude
// nomidan ishlaganda bir xil socket ikki tomondan keladi, shuning uchun takrori tashlanadi.
func (m Model) summaries() []dockerapi.Summary {
	var all []dockerapi.Summary
	if m.claude.isLoaded {
		all = append(all, m.claude.data.Docker...)
	}
	all = append(all, m.docker.data...)
	var out []dockerapi.Summary
	for _, s := range all {
		if !slices.ContainsFunc(out, func(o dockerapi.Summary) bool { return o.Endpoint.Socket == s.Endpoint.Socket }) {
			out = append(out, s)
		}
	}
	return out
}

func (m Model) dockerRows() [][]cell {
	var rows [][]cell
	for _, s := range m.summaries() {
		rows = append(rows, []cell{{text: summaryTitle(s), style: summaryStyle(s)}})
		cs := slices.Clone(s.Containers)
		slices.SortStableFunc(cs, func(a, b dockerapi.Container) int {
			return strings.Compare(a.ComposeProject+"/"+a.Name, b.ComposeProject+"/"+b.Name)
		})
		for _, c := range cs {
			rows = append(rows, dockerCells(c))
		}
	}
	return rows
}

func summaryTitle(s dockerapi.Summary) string {
	if !s.Reachable {
		return fmt.Sprintf("■ %s — %s (%s)", s.Endpoint.Name, s.Error, s.Endpoint.Socket)
	}
	running := 0
	for _, c := range s.Containers {
		if c.IsRunning() {
			running++
		}
	}
	return fmt.Sprintf("■ %s — Docker %s · %d/%d konteyner ishlayapti · %d image", s.Endpoint.Name, s.Version, running, len(s.Containers), s.Images)
}

func summaryStyle(s dockerapi.Summary) lipgloss.Style {
	if s.Reachable {
		return theme.Heading
	}
	return theme.Muted
}

func dockerCells(c dockerapi.Container) []cell {
	dot, style := "○", theme.Muted
	if c.IsRunning() {
		dot, style = "●", theme.OK
	}
	if strings.Contains(c.Status, "unhealthy") {
		style = theme.Err
	}
	mem := ""
	if c.IsRunning() {
		mem = format.Bytes(c.MemUsage) + " / "
		if c.MemLimit > 0 {
			mem += format.Bytes(uint64(c.MemLimit))
		} else {
			mem += "∞"
		}
	}
	memStyle := theme.Text
	if c.IsRunning() && c.MemLimit == 0 {
		memStyle = theme.Warn
	}
	return []cell{
		{text: " " + dot + " " + c.Name, style: style, width: dockerColumns[0].width},
		{text: c.Status, style: theme.Muted, width: dockerColumns[1].width},
		{text: cpuText(c.IsRunning(), c.CPUPercent), style: theme.Text, width: dockerColumns[2].width, isRight: true},
		{text: mem, style: memStyle, width: dockerColumns[3].width, isRight: true},
		{text: c.ComposeProject, style: theme.Accent, width: dockerColumns[4].width},
		{text: c.Image, style: theme.Muted},
	}
}

func (m Model) viewDocker() []string {
	head := make([]cell, len(dockerColumns))
	for i, c := range dockerColumns {
		c.style = theme.Header
		head[i] = c
	}
	lines := []string{renderRow(head, m.width, false)}
	rows := m.dockerRows()
	switch {
	case len(rows) == 0 && m.claude.err == nil && (!m.claude.isLoaded || !m.docker.isLoaded):
		return append(lines, theme.Muted.Render("Docker ma'lumoti yuklanmoqda…"))
	case len(rows) == 0:
		msg := "Ochiq Docker topilmadi"
		if m.claude.err != nil {
			msg += " (claude tomoni: " + errText(m.claude.err) + ")"
		}
		return append(lines, theme.Muted.Render(msg))
	}
	h := m.listHeight()
	list := m.dock
	list.clamp(len(rows), h)
	from, to := list.visible(len(rows), h)
	for i := from; i < to; i++ {
		lines = append(lines, renderRow(rows[i], m.width, i == list.cursor))
	}
	return lines
}
