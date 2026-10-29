package tui

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/shranet/pcinfo/internal/collect/projects"
	"github.com/shranet/pcinfo/internal/format"
	"github.com/shranet/pcinfo/internal/tui/theme"
)

// orphanKey — hech qaysi workspace'ga bog'lanmagan narsalar guruhi.
const orphanKey = "\x00orphans"

const (
	indentChild = "    "
	indentLeaf  = "      "
)

type projView struct {
	list     listState
	expanded map[string]bool
}

// projRow — daraxtning bitta qatori. key bo'sh bo'lmasa, qator ochib-yopiladi.
type projRow struct {
	key   string
	cells []cell
}

func (v *projView) key(key string, rows []projRow, height int) {
	if v.list.navKey(key, len(rows), height) {
		return
	}
	switch key {
	case "enter", "space", "right", "left", "l", "h":
		v.toggleAt(v.list.cursor, rows, height, key)
	case "a":
		isAnyOpen := false
		for _, isOpen := range v.expanded {
			isAnyOpen = isAnyOpen || isOpen
		}
		for _, r := range rows {
			if r.key != "" {
				v.expanded[r.key] = !isAnyOpen
			}
		}
		v.list.clamp(len(rows), height)
	}
}

func (v *projView) toggleAt(i int, rows []projRow, height int, key string) {
	if i >= len(rows) {
		return
	}
	k := rows[i].key
	if k == "" {
		return
	}
	switch key {
	case "right", "l":
		v.expanded[k] = true
	case "left", "h":
		v.expanded[k] = false
	default:
		v.expanded[k] = !v.expanded[k]
	}
	v.list.clamp(len(rows), height)
}

func (v *projView) click(i int, rows []projRow, height int) {
	if i >= len(rows) {
		return
	}
	if v.list.cursor == i || rows[i].key != "" {
		v.toggleAt(i, rows, height, "enter")
	}
	v.list.cursor = i
	v.list.clamp(len(rows), height)
}

// sortedProjects: ishlayotganlari tepada, keyin preview'i borlari, keyin alifbo.
func sortedProjects(ps []projects.Project) []projects.Project {
	out := slices.Clone(ps)
	rank := func(p projects.Project) int {
		switch {
		case p.Running > 0:
			return 0
		case len(p.Sites) > 0 || p.Total > 0:
			return 1
		}
		return 2
	}
	slices.SortStableFunc(out, func(a, b projects.Project) int {
		return cmp.Or(cmp.Compare(rank(a), rank(b)), strings.Compare(a.Workspace.Name, b.Workspace.Name))
	})
	return out
}

// projectRows — joriy ochiq/yopiq holat bo'yicha daraxt qatorlari (View va sichqoncha uchun bir xil).
func (m Model) projectRows() []projRow {
	if !m.claude.isLoaded {
		return nil
	}
	var rows []projRow
	for _, p := range sortedProjects(m.claude.data.Projects.Projects) {
		key := p.Workspace.Name
		isOpen := m.proj.expanded[key]
		rows = append(rows, projRow{key: key, cells: projectHeader(p, isOpen)})
		if isOpen {
			rows = append(rows, projectChildren(p)...)
		}
	}
	res := m.claude.data.Projects
	if len(res.OrphanSites) > 0 || len(res.OrphanCompose) > 0 {
		isOpen := m.proj.expanded[orphanKey]
		total := 0
		for _, c := range res.OrphanCompose {
			total += len(c.Containers)
		}
		rows = append(rows, projRow{key: orphanKey, cells: []cell{
			{text: expander(isOpen) + " Bog'lanmagan", style: theme.Muted, width: nameWidth},
			{text: fmt.Sprintf("%d konteyner · %d sayt", total, len(res.OrphanSites)), style: theme.Muted},
		}})
		if isOpen {
			for _, s := range res.OrphanSites {
				rows = append(rows, siteRow(s))
			}
			for _, c := range res.OrphanCompose {
				rows = append(rows, composeRows(c)...)
			}
		}
	}
	return rows
}

const (
	nameWidth   = 24
	statusWidth = 9
	memWidth    = 10
	cpuWidth    = 7
)

func expander(isOpen bool) string {
	if isOpen {
		return "▼"
	}
	return "▶"
}

func projectHeader(p projects.Project, isOpen bool) []cell {
	statusStyle := theme.Muted
	status := "○ —"
	if p.Total > 0 {
		status = fmt.Sprintf("● %d/%d", p.Running, p.Total)
		statusStyle = theme.OK
		if p.Running < p.Total {
			statusStyle = theme.Warn
		}
		if p.Running == 0 {
			statusStyle = theme.Muted
		}
	}
	var extras []string
	for _, s := range p.Sites {
		extras = append(extras, s.Host)
	}
	if p.Workspace.IsLocalOnly {
		extras = append([]string{"[local]"}, extras...)
	}
	if n := unpushed(p.Workspace.Repos); n > 0 {
		extras = append(extras, fmt.Sprintf("↑%d", n))
	}
	warn := ""
	if len(p.Warnings) > 0 {
		warn = fmt.Sprintf("⚠%d ", len(p.Warnings))
	}
	mem, cpu := "", ""
	if p.Running > 0 {
		mem, cpu = format.Bytes(p.MemUsage), format.Percent(p.CPUPercent)
	}
	return []cell{
		{text: expander(isOpen) + " " + p.Workspace.Name, style: theme.Bold, width: nameWidth},
		{text: status, style: statusStyle, width: statusWidth},
		{text: mem, style: theme.Text, width: memWidth, isRight: true},
		{text: cpu, style: theme.Text, width: cpuWidth, isRight: true},
		{text: warn, style: theme.Warn, width: 4},
		{text: strings.Join(extras, "  "), style: theme.Accent},
	}
}

func unpushed(repos []projects.Repo) int {
	n := 0
	for _, r := range repos {
		n += r.Ahead
	}
	return n
}

func textRow(style lipgloss.Style, text string) projRow {
	return projRow{cells: []cell{{text: text, style: style}}}
}

func projectChildren(p projects.Project) []projRow {
	var rows []projRow
	for _, w := range p.Warnings {
		rows = append(rows, textRow(theme.Warn, indentChild+"⚠ "+w))
	}
	for _, s := range p.Sites {
		rows = append(rows, siteRow(s))
	}
	for _, c := range p.Compose {
		rows = append(rows, composeRows(c)...)
	}
	for _, r := range p.Workspace.Repos {
		rows = append(rows, repoRow(r))
	}
	if len(p.Workspace.DeployTags) > 0 {
		rows = append(rows, textRow(theme.Muted, indentChild+"⬆ prod (deploy.json): "+strings.Join(p.Workspace.DeployTags, ", ")))
	}
	if len(rows) == 0 {
		rows = append(rows, textRow(theme.Muted, indentChild+"konteyner, preview va repo yo'q"))
	}
	return rows
}

func siteRow(s projects.Site) projRow {
	icon, style := "⇢", theme.Accent
	if s.Kind == projects.SiteLocal {
		icon = "⌂"
	}
	text := fmt.Sprintf("%s%s %s → %s", indentChild, icon, s.Host, strings.Join(s.Upstreams, ", "))
	if n := len(s.PublicPaths); n > 0 {
		text += fmt.Sprintf("   [%d ochiq yo'l: %s]", n, strings.Join(s.PublicPaths, " "))
	}
	return textRow(style, text)
}

func composeRows(c projects.Compose) []projRow {
	rows := []projRow{textRow(theme.Muted, indentChild+"◆ compose: "+c.Name)}
	for _, ct := range c.Containers {
		rows = append(rows, projRow{cells: containerCells(ct)})
	}
	return rows
}

func containerCells(c projects.Container) []cell {
	dot, style := "○", theme.Muted
	if c.IsRunning() {
		dot, style = "●", theme.OK
	}
	mem := ""
	if c.IsRunning() {
		mem = format.Bytes(c.MemUsage)
		if c.MemLimit > 0 {
			mem += " / " + format.Bytes(uint64(c.MemLimit))
		}
	}
	return []cell{
		{text: indentLeaf + dot + " " + c.Name, style: style, width: nameWidth + 6},
		{text: c.Status, style: theme.Muted, width: 22},
		{text: mem, style: theme.Text, width: 22, isRight: true},
		{text: cpuText(c.IsRunning(), c.CPUPercent), style: theme.Text, width: cpuWidth, isRight: true},
		{text: strings.Join(c.Networks, ","), style: theme.Muted},
	}
}

func cpuText(isRunning bool, p float64) string {
	if !isRunning {
		return ""
	}
	return format.Percent(p)
}

func repoRow(r projects.Repo) projRow {
	name := r.Path
	if name == "." {
		name = "(workspace)"
	}
	var parts []string
	switch {
	case r.Err != "":
		return textRow(theme.Err, fmt.Sprintf("%s⎇ %s: %s", indentChild, name, r.Err))
	case !r.HasUpstream:
		parts = append(parts, "upstream yo'q")
	case r.Ahead > 0 || r.Behind > 0:
		parts = append(parts, fmt.Sprintf("↑%d ↓%d", r.Ahead, r.Behind))
	default:
		parts = append(parts, "✓ push qilingan")
	}
	if r.Dirty > 0 {
		parts = append(parts, fmt.Sprintf("●%d o'zgarish", r.Dirty))
	}
	style := theme.Muted
	if r.Ahead > 0 || r.Dirty > 0 || !r.HasUpstream {
		style = theme.Warn
	}
	return textRow(style, fmt.Sprintf("%s⎇ %s  %s  %s", indentChild, name, r.Branch, strings.Join(parts, "  ")))
}

func (m Model) viewProjects() []string {
	switch {
	case m.claude.isFailed():
		return []string{theme.Err.Render("Proyektlar olinmadi: " + errText(m.claude.err))}
	case !m.claude.isLoaded:
		return []string{theme.Muted.Render("Proyektlar yuklanmoqda (claude tomoni)…")}
	}
	head := []cell{
		{text: "PROYEKT", style: theme.Header, width: nameWidth},
		{text: "HOLAT", style: theme.Header, width: statusWidth},
		{text: "XOTIRA", style: theme.Header, width: memWidth, isRight: true},
		{text: "CPU", style: theme.Header, width: cpuWidth, isRight: true},
		{text: "", style: theme.Header, width: 4},
		{text: "PREVIEW / GIT", style: theme.Header},
	}
	if m.claude.err != nil {
		// Qator qo'shilmaydi: sichqoncha klikida qatorlar siljimasin.
		head[len(head)-1] = cell{text: "⚠ oxirgi yangilash xatosi: " + errText(m.claude.err), style: theme.Err}
	}
	lines := []string{renderRow(head, m.width, false)}
	rows := m.projectRows()
	if len(rows) == 0 {
		return append(lines, theme.Muted.Render("Proyekt topilmadi"))
	}
	h := m.listHeight()
	list := m.proj.list
	list.clamp(len(rows), h)
	from, to := list.visible(len(rows), h)
	for i := from; i < to; i++ {
		lines = append(lines, renderRow(rows[i].cells, m.width, i == list.cursor))
	}
	return lines
}
