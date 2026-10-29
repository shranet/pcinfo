package tui

import (
	"cmp"
	"fmt"
	"slices"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/shranet/pcinfo/internal/collect/process"
	"github.com/shranet/pcinfo/internal/format"
	"github.com/shranet/pcinfo/internal/privileged"
	"github.com/shranet/pcinfo/internal/tui/theme"
)

const maxFilterLen = 64

type procSort int

const (
	sortCPU procSort = iota
	sortMem
	sortPID
	sortUser
	sortName
)

// procColumn — jarayonlar jadvali ustuni; canSort false bo'lsa saralanmaydi.
type procColumn struct {
	title   string
	width   int
	isRight bool
	sortBy  procSort
	canSort bool
}

var procColumns = []procColumn{
	{title: "PID", width: 7, isRight: true, sortBy: sortPID, canSort: true},
	{title: "USER", width: 10, sortBy: sortUser, canSort: true},
	{title: "CPU%", width: 6, isRight: true, sortBy: sortCPU, canSort: true},
	{title: "MEM%", width: 5, isRight: true, sortBy: sortMem, canSort: true},
	{title: "RSS", width: 9, isRight: true, sortBy: sortMem, canSort: true},
	{title: "S", width: 1},
	{title: "THR", width: 4, isRight: true},
	{title: "BUYRUQ", width: 0, sortBy: sortName, canSort: true},
}

type procView struct {
	list        listState
	sortBy      procSort
	isDesc      bool
	filter      string
	isFiltering bool
	selectedPID int32
	rows        []process.Process
}

func newProcView() procView { return procView{sortBy: sortCPU, isDesc: true} }

// refresh saralangan va filtrlangan ro'yxatni qayta quradi, tanlangan PID saqlanadi.
func (v *procView) refresh(all []process.Process, height int) {
	needle := strings.ToLower(v.filter)
	rows := make([]process.Process, 0, len(all))
	for _, p := range all {
		if needle == "" || strings.Contains(strings.ToLower(p.Name+" "+p.User+" "+p.Cmdline), needle) {
			rows = append(rows, p)
		}
	}
	slices.SortStableFunc(rows, func(a, b process.Process) int {
		c := compareProc(a, b, v.sortBy)
		if v.isDesc {
			c = -c
		}
		return cmp.Or(c, cmp.Compare(a.PID, b.PID))
	})
	v.rows = rows
	if i := slices.IndexFunc(rows, func(p process.Process) bool { return p.PID == v.selectedPID }); i >= 0 {
		v.list.cursor = i
	}
	v.list.clamp(len(rows), height)
	v.syncSelected()
}

func compareProc(a, b process.Process, by procSort) int {
	switch by {
	case sortCPU:
		return cmp.Compare(a.CPUPercent, b.CPUPercent)
	case sortMem:
		return cmp.Compare(a.RSS, b.RSS)
	case sortPID:
		return cmp.Compare(a.PID, b.PID)
	case sortUser:
		return strings.Compare(a.User, b.User)
	case sortName:
		return strings.Compare(a.Name, b.Name)
	}
	return 0
}

func (v *procView) syncSelected() {
	if v.list.cursor < len(v.rows) {
		v.selectedPID = v.rows[v.list.cursor].PID
	}
}

func (v *procView) setSort(by procSort, all []process.Process, height int) {
	if v.sortBy == by {
		v.isDesc = !v.isDesc
	} else {
		v.sortBy = by
		// Raqamli ustunlar kattadan, matnlilar alifbo bo'yicha boshlanadi.
		v.isDesc = by == sortCPU || by == sortMem
	}
	v.refresh(all, height)
}

func (v *procView) key(key string, all []process.Process, height int) {
	if v.list.navKey(key, len(v.rows), height) {
		v.syncSelected()
		return
	}
	switch key {
	case "/":
		v.isFiltering = true
	case "esc":
		v.filter = ""
		v.refresh(all, height)
	case "c":
		v.setSort(sortCPU, all, height)
	case "m":
		v.setSort(sortMem, all, height)
	case "p":
		v.setSort(sortPID, all, height)
	case "n":
		v.setSort(sortName, all, height)
	case "r":
		v.isDesc = !v.isDesc
		v.refresh(all, height)
	}
}

func (v *procView) filterKey(msg tea.KeyPressMsg, all []process.Process, height int) {
	switch msg.String() {
	case "enter":
		v.isFiltering = false
	case "esc":
		v.isFiltering, v.filter = false, ""
	case "backspace":
		if r := []rune(v.filter); len(r) > 0 {
			v.filter = string(r[:len(r)-1])
		}
	default:
		if msg.Text != "" && len(v.filter) < maxFilterLen {
			v.filter += msg.Text
		}
	}
	v.list.cursor = 0
	v.refresh(all, height)
}

func (v *procView) clickHeader(x, width int, all []process.Process, height int) {
	pos := 0
	for _, c := range procColumns {
		w := c.width
		if w == 0 {
			w = width
		}
		if x >= pos && x < pos+w && c.canSort {
			v.setSort(c.sortBy, all, height)
			return
		}
		pos += w + len(cellGap)
	}
}

func (v *procView) selectIndex(i, height int) {
	if i < len(v.rows) {
		v.list.cursor = i
		v.list.clamp(len(v.rows), height)
		v.syncSelected()
	}
}

func (m Model) viewProcess() []string {
	switch {
	case m.procs.isFailed():
		return []string{theme.Err.Render("Jarayonlar o'qilmadi: " + errText(m.procs.err))}
	case !m.procs.isLoaded:
		return []string{theme.Muted.Render("Jarayonlar yuklanmoqda…")}
	}
	header := make([]cell, len(procColumns))
	for i, c := range procColumns {
		title := c.title
		if c.canSort && c.sortBy == m.proc.sortBy && c.title != "RSS" {
			title += sortArrow(m.proc.isDesc)
		}
		header[i] = cell{text: title, style: theme.Header, width: c.width, isRight: c.isRight}
	}
	lines := []string{renderRow(header, m.width, false)}
	if len(m.proc.rows) == 0 {
		return append(lines, theme.Muted.Render("Mos jarayon yo'q"))
	}
	h := m.listHeight()
	from, to := m.proc.list.visible(len(m.proc.rows), h)
	for i := from; i < to; i++ {
		lines = append(lines, renderRow(procCells(m.proc.rows[i]), m.width, i == m.proc.list.cursor))
	}
	return lines
}

func sortArrow(isDesc bool) string {
	if isDesc {
		return "▼"
	}
	return "▲"
}

func procCells(p process.Process) []cell {
	cmd := p.Cmdline
	if cmd == "" {
		cmd = "[" + p.Name + "]"
	}
	return []cell{
		{text: strconv.Itoa(int(p.PID)), style: theme.Muted, width: procColumns[0].width, isRight: true},
		{text: p.User, style: userStyle(p.User), width: procColumns[1].width},
		{text: fmt.Sprintf("%.1f", p.CPUPercent), style: theme.ForPercent(p.CPUPercent), width: procColumns[2].width, isRight: true},
		{text: fmt.Sprintf("%.1f", p.MemPercent), style: theme.ForPercent(p.MemPercent * memPercentScale), width: procColumns[3].width, isRight: true},
		{text: format.Bytes(p.RSS), style: theme.Text, width: procColumns[4].width, isRight: true},
		{text: stateLetter(p.State), style: stateStyle(p.State), width: procColumns[5].width},
		{text: strconv.Itoa(int(p.Threads)), style: theme.Muted, width: procColumns[6].width, isRight: true},
		{text: cmd, style: theme.Text},
	}
}

// memPercentScale — bitta jarayon uchun 10% RAM ham ko'p; rang shkalasini kuchaytiradi.
const memPercentScale = 5

func userStyle(user string) lipgloss.Style {
	switch user {
	case "root":
		return theme.Err
	case privileged.ClaudeUser:
		return theme.Accent
	}
	return theme.Text
}

// stateLetter gopsutil holat nomini htop'dagi bitta harfga aylantiradi.
func stateLetter(state string) string {
	switch state {
	case "running":
		return "R"
	case "sleep":
		return "S"
	case "idle":
		return "I"
	case "wait", "disk-sleep":
		return "D"
	case "zombie":
		return "Z"
	case "stop":
		return "T"
	case "":
		return "?"
	}
	return strings.ToUpper(state[:1])
}

func stateStyle(state string) lipgloss.Style {
	switch state {
	case "running":
		return theme.OK
	case "zombie", "wait", "disk-sleep":
		return theme.Err
	}
	return theme.Muted
}
