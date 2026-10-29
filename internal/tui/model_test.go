package tui

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/shranet/pcinfo/internal/collect/dockerapi"
	"github.com/shranet/pcinfo/internal/collect/modem"
	"github.com/shranet/pcinfo/internal/collect/network"
	"github.com/shranet/pcinfo/internal/collect/process"
	"github.com/shranet/pcinfo/internal/collect/projects"
	"github.com/shranet/pcinfo/internal/collect/system"
	"github.com/shranet/pcinfo/internal/snapshot"
)

const (
	testWidth  = 80
	testHeight = 24
)

type fakeFetcher struct{}

func (fakeFetcher) Fetch(context.Context) (snapshot.ClaudeSide, error) { return testClaude(), nil }

func testClaude() snapshot.ClaudeSide {
	web := dockerapi.Container{Name: "shop-web-1", State: "running", Status: "Up 2 hours", ComposeProject: "shop", MemUsage: 100 << 20, MemLimit: 512 << 20}
	return snapshot.ClaudeSide{
		Schema: snapshot.Schema,
		Docker: []dockerapi.Summary{{Endpoint: dockerapi.Endpoint{Name: "claude (rootless)"}, Reachable: true, Version: "29", Containers: []dockerapi.Container{web}}},
		Projects: projects.Result{Projects: []projects.Project{
			{
				Workspace: projects.Workspace{Name: "ruvex-shop", Repos: []projects.Repo{{Path: "shop-api", Branch: "main", HasUpstream: true, Ahead: 2}}},
				Compose:   []projects.Compose{{Name: "shop", Containers: []projects.Container{{Name: "shop-web-1", State: "running", MemUsage: 100 << 20}}}},
				Sites:     []projects.Site{{Host: "shop.long.uz", Kind: projects.SitePreview, Upstreams: []string{"shop-web-1"}, PublicPaths: []string{"/api/health"}}},
				Warnings:  []string{"shop-web-1: mem_limit yo'q"},
				Running:   1, Total: 1, MemUsage: 100 << 20,
			},
			{Workspace: projects.Workspace{Name: "idle"}},
		}},
	}
}

func newTestModel(t *testing.T) Model {
	t.Helper()
	m := New(Deps{Claude: fakeFetcher{}, Modem: modem.New("http://127.0.0.1:1")}, Options{})
	m = update(t, m, tea.WindowSizeMsg{Width: testWidth, Height: testHeight})
	m = update(t, m, claudeMsg{snap: testClaude()})
	m = update(t, m, systemMsg{snap: system.Snapshot{
		CPU:    system.CPU{Total: 42, PerCore: []float64{10, 90, 50, 0}},
		Memory: system.Memory{Total: 32 << 30, Available: 16 << 30},
		Disks:  []system.Disk{{Mount: "/", Total: 100 << 30, Used: 50 << 30, Percent: 50, FSType: "ext4"}},
	}})
	m = update(t, m, processMsg{procs: []process.Process{
		{PID: 1, User: "root", Name: "systemd", CPUPercent: 0.1, RSS: 10 << 20},
		{PID: 200, User: "shranet", Name: "ghostty", Cmdline: "ghostty", CPUPercent: 5, RSS: 300 << 20},
		{PID: 300, User: "claude", Name: "node", Cmdline: "node server.js", CPUPercent: 50, RSS: 100 << 20},
	}})
	m = update(t, m, linksMsg{at: time.Now(), links: []network.Link{
		{Iface: "enp6s0", Kind: network.KindFiber, HasDefault: true, IsActive: true, IsUp: true, Gateway: "192.168.100.1", Metric: 100, IPv4: []string{"192.168.100.19/24"}},
		{Iface: "enx0", Kind: network.KindUSBModem, HasDefault: true, IsUp: true, Gateway: "192.168.8.1", Metric: 700},
	}})
	m = update(t, m, modemMsg{info: modem.Info{Signal: modem.Signal{RSRP: "-106dBm", SINR: "-2dB"}, Status: modem.Status{ConnectionStatus: "901", CurrentNetworkType: "19"}}})
	return m
}

func update(t *testing.T, m Model, msg tea.Msg) Model {
	t.Helper()
	next, _ := m.Update(msg)
	return next.(Model)
}

func key(s string) tea.KeyPressMsg {
	switch s {
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "tab":
		return tea.KeyPressMsg{Code: tea.KeyTab}
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	}
	r := []rune(s)[0]
	return tea.KeyPressMsg{Code: r, Text: s}
}

func click(x, y int) tea.MouseClickMsg {
	return tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft}
}

func TestEveryTabFitsScreen(t *testing.T) {
	m := newTestModel(t)
	for tb := range tabCount {
		m.tab = tb
		m.proj.expanded["ruvex-shop"] = true
		lines := strings.Split(m.render(), "\n")
		if len(lines) != testHeight {
			t.Errorf("%s: %d qator, want %d", tb.title(), len(lines), testHeight)
		}
		for i, l := range lines {
			if w := ansi.StringWidth(l); w > testWidth {
				t.Errorf("%s: %d-qator kengligi %d > %d: %q", tb.title(), i, w, testWidth, ansi.Strip(l))
			}
		}
	}
}

func TestLoadingStates(t *testing.T) {
	m := New(Deps{Claude: fakeFetcher{}}, Options{})
	m = update(t, m, tea.WindowSizeMsg{Width: testWidth, Height: testHeight})
	for tb, want := range map[tab]string{
		tabSystem:   "yuklanmoqda",
		tabProcess:  "yuklanmoqda",
		tabProjects: "yuklanmoqda",
		tabDocker:   "yuklanmoqda",
		tabNetwork:  "yuklanmoqda",
	} {
		m.tab = tb
		if !strings.Contains(ansi.Strip(m.render()), want) {
			t.Errorf("%s: %q ko'rinmadi", tb.title(), want)
		}
	}
	m = update(t, m, claudeMsg{err: errors.New("sudo ruxsati yo'q")})
	m.tab = tabProjects
	if !strings.Contains(ansi.Strip(m.render()), "sudo ruxsati yo'q") {
		t.Error("claude xatosi ko'rinmadi")
	}
}

func TestTabSwitching(t *testing.T) {
	m := newTestModel(t)
	m = update(t, m, key("3"))
	if m.tab != tabProjects {
		t.Errorf("3 → %v", m.tab)
	}
	m = update(t, m, key("tab"))
	if m.tab != tabDocker {
		t.Errorf("tab → %v", m.tab)
	}
	sp := tabSpans()[tabNetwork]
	m = update(t, m, click(sp.from+1, tabBarRow))
	if m.tab != tabNetwork {
		t.Errorf("klik → %v", m.tab)
	}
}

func TestProjectsExpandByClick(t *testing.T) {
	m := newTestModel(t)
	m.tab = tabProjects
	before := len(m.projectRows())
	m = update(t, m, click(2, bodyTop+1))
	rows := m.projectRows()
	if len(rows) <= before {
		t.Fatalf("klikdan keyin ochilmadi: %d → %d", before, len(rows))
	}
	text := ansi.Strip(m.render())
	for _, want := range []string{"shop.long.uz → shop-web-1", "1 ochiq yo'l", "⚠ shop-web-1: mem_limit yo'q", "↑2", "compose: shop"} {
		if !strings.Contains(text, want) {
			t.Errorf("%q ko'rinmadi", want)
		}
	}
	m = update(t, m, key("enter"))
	if len(m.projectRows()) != before {
		t.Error("Enter yopishi kerak edi")
	}
}

func TestProcessSortByHeaderClick(t *testing.T) {
	m := newTestModel(t)
	m.tab = tabProcess
	if m.proc.rows[0].PID != 300 {
		t.Fatalf("standart saralash CPU bo'yicha: %d", m.proc.rows[0].PID)
	}
	m = update(t, m, click(1, bodyTop)) // PID ustuni
	if m.proc.sortBy != sortPID || m.proc.rows[0].PID != 1 {
		t.Errorf("PID bo'yicha o'sish tartibi kutilgan: %v %d", m.proc.sortBy, m.proc.rows[0].PID)
	}
	m = update(t, m, key("/"))
	for _, r := range "ghost" {
		m = update(t, m, key(string(r)))
	}
	m = update(t, m, key("enter"))
	if len(m.proc.rows) != 1 || m.proc.rows[0].PID != 200 {
		t.Errorf("qidiruv: %+v", m.proc.rows)
	}
	if m.proc.isFiltering {
		t.Error("Enter qidiruvni yakunlashi kerak")
	}
}

func TestSelectionFollowsPID(t *testing.T) {
	m := newTestModel(t)
	m.tab = tabProcess
	m = update(t, m, key("down"))
	pid := m.proc.selectedPID
	m = update(t, m, processMsg{procs: append([]process.Process{{PID: 999, CPUPercent: 99}}, m.procs.data...)})
	if m.proc.rows[m.proc.list.cursor].PID != pid {
		t.Errorf("yangilanishdan keyin tanlov %d dan ko'chib ketdi", pid)
	}
}

func TestUSSDRateLimit(t *testing.T) {
	m := newTestModel(t)
	m.tab = tabNetwork
	m.ussd.startedAt = time.Now().Add(-time.Minute)
	next, cmd := m.Update(key("u"))
	if cmd != nil {
		t.Error("5 daqiqa o'tmasdan USSD yuborilmasligi kerak")
	}
	if !strings.Contains(next.(Model).ussd.notice, "operator cheklovi") {
		t.Errorf("notice = %q", next.(Model).ussd.notice)
	}
	m.ussd = ussdState{isRunning: true}
	if _, cmd := m.Update(key("u")); cmd != nil {
		t.Error("ishlayotgan USSD ustiga yangisi yuborilmasligi kerak")
	}
}

func TestUSSDResultShown(t *testing.T) {
	m := newTestModel(t)
	m.tab = tabNetwork
	mb := int64(304519)
	m = update(t, m, ussdDoneMsg{balance: modem.Balance{Raw: "Balans: 5000; 304519 MB", Balance: "5000", MB: &mb}})
	text := ansi.Strip(m.render())
	for _, want := range []string{"Balans 5000", "297.4 GB", "✓ olindi"} {
		if !strings.Contains(text, want) {
			t.Errorf("%q ko'rinmadi", want)
		}
	}
}

func TestUSBActiveWarning(t *testing.T) {
	m := newTestModel(t)
	m = update(t, m, linksMsg{at: time.Now(), links: []network.Link{
		{Iface: "enx0", Kind: network.KindUSBModem, HasDefault: true, IsActive: true, IsUp: true},
	}})
	if !strings.Contains(ansi.Strip(m.viewStatus()), "USB modem (zaxira!)") {
		t.Errorf("status = %q", ansi.Strip(m.viewStatus()))
	}
}
