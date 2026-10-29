// Package tui — pcinfo terminal interfeysi (Bubble Tea). Update va View hech
// qachon bloklamaydi: barcha o'qishlar tea.Cmd ichida, natija xabar bo'lib qaytadi.
package tui

import (
	"context"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/shranet/pcinfo/internal/collect/dockerapi"
	"github.com/shranet/pcinfo/internal/collect/modem"
	"github.com/shranet/pcinfo/internal/collect/network"
	"github.com/shranet/pcinfo/internal/collect/process"
	"github.com/shranet/pcinfo/internal/collect/system"
	"github.com/shranet/pcinfo/internal/safetext"
	"github.com/shranet/pcinfo/internal/snapshot"
	"github.com/shranet/pcinfo/internal/tui/theme"
)

const (
	appName = "pcinfo"

	// Ekran: 0-qator tablar, 1-qator status, oxirgi qator tugmalar.
	tabBarRow   = 0
	statusRow   = 1
	bodyTop     = 2
	chromeLines = 3

	historyLimit = 240
	wheelStep    = 3

	// ussdHint — Tarmoq tabida footer boshidagi bosiladigan tugma.
	ussdHint = "[ u — balansni qayta so'rash ]"
)

type tab int

const (
	tabSystem tab = iota
	tabProcess
	tabProjects
	tabDocker
	tabNetwork
	tabCount
)

func (t tab) title() string {
	switch t {
	case tabSystem:
		return "Tizim"
	case tabProcess:
		return "Jarayonlar"
	case tabProjects:
		return "Proyektlar"
	case tabDocker:
		return "Docker"
	case tabNetwork:
		return "Tarmoq"
	}
	return ""
}

// claudeFetcher — claude tomonini beruvchi (privileged.Fetcher yoki testda soxta).
type claudeFetcher interface {
	Fetch(ctx context.Context) (snapshot.ClaudeSide, error)
}

// Deps — TUI bog'liqliklari.
type Deps struct {
	Claude       claudeFetcher
	Modem        *modem.Client
	System       *system.Sampler
	Process      *process.Sampler
	SelfWarnings []string
}

// Options — buyruq satri sozlamalari.
type Options struct {
	PublicIP bool
	USSD     bool
	Version  string
}

// loaded — bitta manba holati: yuklanmoqda / xato / ma'lumot.
type loaded[T any] struct {
	data     T
	err      error
	isLoaded bool
	at       time.Time
}

func (l *loaded[T]) set(data T, err error, at time.Time) {
	l.err = err
	l.at = at
	// Xatoli javob oldingi yaxshi ma'lumotni o'chirmaydi; birinchi marta esa
	// qisman ma'lumot ham saqlanadi.
	if err == nil || !l.isLoaded {
		l.data = data
	}
	l.isLoaded = l.isLoaded || err == nil
}

// isFailed — hali birorta ham muvaffaqiyatli o'qish bo'lmagan va xato bor.
func (l loaded[T]) isFailed() bool { return !l.isLoaded && l.err != nil }

type rate struct{ rx, tx float64 }

type ussdState struct {
	isRunning bool
	progress  modem.Progress
	balance   *modem.Balance
	err       error
	startedAt time.Time
	doneAt    time.Time
	notice    string
	events    <-chan tea.Msg
}

// Model — butun ilova holati.
type Model struct {
	deps Deps
	opts Options

	width, height int
	tab           tab
	now           time.Time

	sys     loaded[system.Snapshot]
	cpuHist []float64
	procs   loaded[[]process.Process]
	claude  loaded[snapshot.ClaudeSide]
	docker  loaded[[]dockerapi.Summary]

	links      loaded[[]network.Link]
	prevLinks  map[string]network.Counter
	prevLinkAt time.Time
	rates      map[string]rate
	rxHist     map[string][]float64
	probes     map[string]network.ProbeResult
	publicIPs  map[string]string
	modemInfo  loaded[modem.Info]
	ussd       ussdState

	proc procView
	proj projView
	dock listState
}

// New modelni yaratadi.
func New(deps Deps, opts Options) Model {
	m := Model{
		deps:      deps,
		opts:      opts,
		now:       time.Now(),
		prevLinks: map[string]network.Counter{},
		rates:     map[string]rate{},
		rxHist:    map[string][]float64{},
		probes:    map[string]network.ProbeResult{},
		publicIPs: map[string]string{},
		proc:      newProcView(),
		proj:      projView{expanded: map[string]bool{}},
	}
	// Ishga tushishdagi USSD holati shu yerda yoziladi: Init qiymat bo'yicha ishlaydi
	// va o'zgarishlari saqlanmaydi — aks holda 5 daqiqalik cheklov aylanib o'tilardi.
	if opts.USSD {
		m.ussd = ussdState{isRunning: true, startedAt: m.now}
	}
	return m
}

// Init barcha manbalarni birinchi marta so'raydi.
func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{
		fetchSystem(m.deps.System),
		fetchProcesses(m.deps.Process),
		fetchLinks,
		fetchModem(m.deps.Modem),
		fetchClaude(m.deps.Claude),
		fetchDocker,
	}
	if m.ussd.isRunning {
		cmds = append(cmds, startUSSD(m.deps.Modem))
	}
	return tea.Batch(cmds...)
}

func (m Model) bodyHeight() int { return max(0, m.height-chromeLines) }

// Update xabarlarni qayta ishlaydi.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	m.now = time.Now()
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case tea.KeyPressMsg:
		return m.handleKey(msg)
	case tea.MouseClickMsg:
		return m.handleClick(msg.Mouse())
	case tea.MouseWheelMsg:
		return m.handleWheel(msg.Mouse())
	case tickMsg:
		if msg.src == srcClaude && !m.needsClaude() {
			return m, tick(srcClaude, claudeInterval)
		}
		return m, m.fetch(msg.src)
	}
	return m.handleData(msg)
}

func (m Model) fetch(src source) tea.Cmd {
	switch src {
	case srcSystem:
		return fetchSystem(m.deps.System)
	case srcProcess:
		return fetchProcesses(m.deps.Process)
	case srcLinks:
		return fetchLinks
	case srcModem:
		return fetchModem(m.deps.Modem)
	case srcProbe:
		return fetchProbes(m.probeIfaces())
	case srcClaude:
		return fetchClaude(m.deps.Claude)
	case srcDocker:
		return fetchDocker
	}
	return nil
}

func (m Model) handleData(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case systemMsg:
		m.sys.set(msg.snap, msg.err, m.now)
		m.cpuHist = pushHistory(m.cpuHist, msg.snap.CPU.Total, historyLimit)
		return m, tick(srcSystem, systemInterval)
	case processMsg:
		m.procs.set(msg.procs, msg.err, m.now)
		m.proc.refresh(m.procs.data, m.bodyHeight())
		return m, tick(srcProcess, processInterval)
	case linksMsg:
		return m.applyLinks(msg)
	case probeMsg:
		for _, r := range msg.results {
			m.probes[r.Iface] = r.Result
		}
		return m, tick(srcProbe, probeInterval)
	case publicIPMsg:
		if msg.err != nil {
			m.publicIPs[msg.iface] = "✗ " + errText(msg.err)
		} else {
			m.publicIPs[msg.iface] = msg.ip
		}
		return m, nil
	case modemMsg:
		m.modemInfo.set(msg.info, msg.err, m.now)
		return m, tick(srcModem, modemInterval)
	case claudeMsg:
		m.claude.set(msg.snap, msg.err, m.now)
		m.clampLists()
		return m, tick(srcClaude, claudeInterval)
	case dockerMsg:
		m.docker.set(msg.summaries, nil, m.now)
		m.clampLists()
		return m, tick(srcDocker, dockerInterval)
	case ussdStartedMsg:
		m.ussd.events = msg.events
		return m, waitUSSD(msg.events)
	case ussdProgressMsg:
		m.ussd.progress = msg.progress
		return m, waitUSSD(m.ussd.events)
	case ussdDoneMsg:
		m.ussd.isRunning, m.ussd.doneAt, m.ussd.err = false, m.now, msg.err
		if msg.err == nil {
			bal := msg.balance
			m.ussd.balance = &bal
		}
		return m, waitUSSD(m.ussd.events)
	}
	return m, nil
}

func (m Model) applyLinks(msg linksMsg) (tea.Model, tea.Cmd) {
	isFirst := !m.links.isLoaded
	m.links.set(msg.links, msg.err, msg.at)
	elapsed := msg.at.Sub(m.prevLinkAt)
	for _, l := range msg.links {
		if prev, ok := m.prevLinks[l.Iface]; ok {
			rx, tx := network.Rate(prev, l.Counter, elapsed)
			m.rates[l.Iface] = rate{rx: rx, tx: tx}
			m.rxHist[l.Iface] = pushHistory(m.rxHist[l.Iface], rx+tx, historyLimit)
		}
		m.prevLinks[l.Iface] = l.Counter
	}
	m.prevLinkAt = msg.at
	cmds := []tea.Cmd{tick(srcLinks, linksInterval)}
	if isFirst && msg.err == nil {
		cmds = append(cmds, fetchProbes(m.probeIfaces()))
		if m.opts.PublicIP {
			for _, iface := range m.probeIfaces() {
				cmds = append(cmds, fetchPublicIP(iface))
			}
		}
	}
	return m, tea.Batch(cmds...)
}

// needsClaude — claude tomonini hozir yangilash kerakmi: u ko'rinib turgan tabda
// yoki oxirgi o'qishdan beri claudeIdleAge o'tgan bo'lsa.
func (m Model) needsClaude() bool {
	return m.tab == tabProjects || m.tab == tabDocker || m.now.Sub(m.claude.at) >= claudeIdleAge
}

// probeIfaces — internetga chiqadigan (default route'li) liniyalar.
func (m Model) probeIfaces() []string {
	var out []string
	for _, l := range m.links.data {
		if l.HasDefault {
			out = append(out, l.Iface)
		}
	}
	return out
}

// beginUSSD USSD'ni boshlaydi; bu jarayonda oxirgisidan MinInterval o'tmagan bo'lsa — yo'q.
func (m *Model) beginUSSD() tea.Cmd {
	switch {
	case m.ussd.isRunning:
		m.ussd.notice = "USSD hali ishlayapti"
		return nil
	case !m.ussd.startedAt.IsZero() && m.now.Sub(m.ussd.startedAt) < modem.MinInterval:
		left := modem.MinInterval - m.now.Sub(m.ussd.startedAt)
		m.ussd.notice = "qayta so'rash " + left.Round(time.Second).String() + " dan keyin (operator cheklovi)"
		return nil
	}
	m.ussd = ussdState{isRunning: true, startedAt: m.now, balance: m.ussd.balance}
	return startUSSD(m.deps.Modem)
}

func (m Model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	if m.tab == tabProcess && m.proc.isFiltering {
		m.proc.filterKey(msg, m.procs.data, m.bodyHeight())
		return m, nil
	}
	switch key {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "tab":
		m.tab = (m.tab + 1) % tabCount
		return m, nil
	case "shift+tab":
		m.tab = (m.tab + tabCount - 1) % tabCount
		return m, nil
	}
	if len(key) == 1 && key[0] >= '1' && key[0] < '1'+byte(tabCount) {
		m.tab = tab(key[0] - '1')
		return m, nil
	}
	h := m.listHeight()
	switch m.tab {
	case tabProcess:
		m.proc.key(key, m.procs.data, h)
	case tabProjects:
		m.proj.key(key, m.projectRows(), h)
	case tabDocker:
		m.dock.navKey(key, len(m.dockerRows()), h)
	case tabNetwork:
		if key == "u" {
			cmd := m.beginUSSD()
			return m, cmd
		}
	}
	return m, nil
}

// listHeight — ustun sarlavhasidan keyingi ro'yxat balandligi.
func (m Model) listHeight() int { return max(0, m.bodyHeight()-1) }

// listRowAt ekrandagi y ni ro'yxat indeksiga aylantiradi (sarlavha qatori bodyTop da).
func listRowAt(y int, l listState) (int, bool) {
	row := y - bodyTop - 1
	if row < 0 {
		return 0, false
	}
	return l.offset + row, true
}

func (m Model) handleClick(mouse tea.Mouse) (tea.Model, tea.Cmd) {
	if mouse.Button != tea.MouseLeft {
		return m, nil
	}
	if mouse.Y == tabBarRow {
		for i, sp := range tabSpans() {
			if mouse.X >= sp.from && mouse.X < sp.to {
				m.tab = tab(i)
			}
		}
		return m, nil
	}
	h := m.listHeight()
	switch m.tab {
	case tabProcess:
		if mouse.Y == bodyTop {
			m.proc.clickHeader(mouse.X, m.width, m.procs.data, h)
		} else if i, ok := listRowAt(mouse.Y, m.proc.list); ok {
			m.proc.selectIndex(i, h)
		}
	case tabProjects:
		if i, ok := listRowAt(mouse.Y, m.proj.list); ok {
			m.proj.click(i, m.projectRows(), h)
		}
	case tabDocker:
		if i, ok := listRowAt(mouse.Y, m.dock); ok && i < len(m.dockerRows()) {
			m.dock.cursor = i
			m.dock.clamp(len(m.dockerRows()), h)
		}
	case tabNetwork:
		if mouse.Y == m.height-1 && mouse.X < ansi.StringWidth(ussdHint) {
			cmd := m.beginUSSD()
			return m, cmd
		}
	}
	return m, nil
}

func (m Model) handleWheel(mouse tea.Mouse) (tea.Model, tea.Cmd) {
	delta := wheelStep
	if mouse.Button == tea.MouseWheelUp {
		delta = -wheelStep
	}
	h := m.listHeight()
	switch m.tab {
	case tabProcess:
		m.proc.list.scroll(delta, len(m.proc.rows), h)
	case tabProjects:
		m.proj.list.scroll(delta, len(m.projectRows()), h)
	case tabDocker:
		m.dock.scroll(delta, len(m.dockerRows()), h)
	}
	return m, nil
}

type span struct{ from, to int }

// tabSpans — tablar qatoridagi har tabning x oralig'i (View va sichqoncha bir xil hisoblaydi).
func tabSpans() []span {
	x := ansi.StringWidth(appTitle())
	spans := make([]span, tabCount)
	for t := range tabCount {
		w := ansi.StringWidth(tabLabel(t))
		spans[t] = span{from: x, to: x + w}
		x += w
	}
	return spans
}

func appTitle() string { return " " + appName + " " }

func tabLabel(t tab) string { return " " + string(rune('1'+t)) + " " + t.title() + " " }

// View ekranni chizadi.
func (m Model) View() tea.View {
	v := tea.NewView(m.render())
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	v.WindowTitle = appName
	return v
}

func (m Model) render() string {
	if m.width == 0 || m.height == 0 {
		return ""
	}
	var body []string
	switch m.tab {
	case tabSystem:
		body = m.viewSystem()
	case tabProcess:
		body = m.viewProcess()
	case tabProjects:
		body = m.viewProjects()
	case tabDocker:
		body = m.viewDocker()
	case tabNetwork:
		body = m.viewNetwork()
	}
	lines := []string{m.viewTabBar(), m.viewStatus()}
	for _, l := range padLines(body, m.bodyHeight()) {
		lines = append(lines, truncateLine(l, m.width))
	}
	lines = append(lines, m.viewFooter())
	return strings.Join(lines, "\n")
}

func (m Model) viewTabBar() string {
	var b strings.Builder
	b.WriteString(theme.Heading.Inherit(theme.Bar).Render(appTitle()))
	for t := range tabCount {
		style := theme.TabInactive
		if t == m.tab {
			style = theme.TabActive
		}
		b.WriteString(style.Render(tabLabel(t)))
	}
	if m.opts.Version != "" {
		b.WriteString(theme.Muted.Inherit(theme.Bar).Render(" " + m.opts.Version))
	}
	return theme.Bar.Width(m.width).Render(truncateLine(b.String(), m.width))
}

func (m Model) viewFooter() string {
	hints := []string{"1-5/Tab bo'lim", "q chiqish"}
	switch m.tab {
	case tabProcess:
		hints = append([]string{"↑↓ tanlash", "/ qidiruv", "c cpu", "m xotira", "p pid", "n nom", "r teskari", "sarlavhaga klik — saralash"}, hints...)
		if m.proc.filter != "" || m.proc.isFiltering {
			hints = append([]string{"qidiruv: " + m.proc.filter + cursorMark(m.proc.isFiltering)}, hints...)
		}
	case tabProjects:
		hints = append([]string{"↑↓ tanlash", "Enter/klik ochish", "a hammasini ochish/yopish"}, hints...)
	case tabDocker:
		hints = append([]string{"↑↓ tanlash", "g'ildirak aylantirish"}, hints...)
	case tabNetwork:
		hints = append([]string{ussdHint}, hints...)
	}
	var parts []string
	for _, h := range hints {
		parts = append(parts, theme.Muted.Render(h))
	}
	return truncateLine(strings.Join(parts, theme.Track.Render(" · ")), m.width)
}

func cursorMark(isActive bool) string {
	if isActive {
		return "▏"
	}
	return ""
}

// errText — xato matnini terminalga chiqarishga tayyorlaydi: xatolar ichida tashqi
// manbadan kelgan matn bo'lishi mumkin (modem kodi, git, sudo).
func errText(err error) string { return safetext.Clean(err.Error()) }

// clampLists yangi ma'lumotdan keyin ro'yxatlar holatini to'g'rilaydi: sichqoncha
// klikini View chizgan oyna bilan bir xil offset bo'yicha hisoblash uchun.
func (m *Model) clampLists() {
	h := m.listHeight()
	m.proj.list.clamp(len(m.projectRows()), h)
	m.dock.clamp(len(m.dockerRows()), h)
}
