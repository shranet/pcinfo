package tui

import (
	"fmt"
	"os"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/shranet/pcinfo/internal/collect/modem"
	"github.com/shranet/pcinfo/internal/collect/process"
	"github.com/shranet/pcinfo/internal/collect/system"
	"github.com/shranet/pcinfo/internal/privileged"
)

// TestLiveRender haqiqiy tizimdan yig'ib, har tabni oddiy matn qilib chiqaradi.
// Faqat qo'lda: PCINFO_LIVE=1 bilan, hostda (USSD yuborilmaydi).
func TestLiveRender(t *testing.T) {
	if os.Getenv("PCINFO_LIVE") == "" {
		t.Skip("PCINFO_LIVE=1 bilan ishga tushiring")
	}
	m := New(Deps{Claude: privileged.NewFetcher(), Modem: modem.New(modem.DefaultBaseURL), System: system.NewSampler(), Process: process.NewSampler(), SelfWarnings: privileged.SelfCheck()}, Options{})
	m = update(t, m, tea.WindowSizeMsg{Width: 150, Height: 45})
	var run func(c tea.Cmd)
	run = func(c tea.Cmd) {
		if c == nil {
			return
		}
		msg := c()
		if b, ok := msg.(tea.BatchMsg); ok {
			for _, cc := range b {
				run(cc)
			}
			return
		}
		if _, ok := msg.(tickMsg); ok {
			return
		}
		next, _ := m.Update(msg)
		m = next.(Model)
	}
	run(m.Init())
	time.Sleep(2 * time.Second)
	run(fetchSystem(m.deps.System))
	run(fetchProcesses(m.deps.Process))
	run(fetchLinks)
	run(fetchProbes(m.probeIfaces()))
	m.proj.expanded["ruvex-lens"] = true
	m.proj.expanded["adbus-share"] = true
	for tb := range tabCount {
		m.tab = tb
		fmt.Println("=================", tb.title())
		fmt.Println(ansi.Strip(m.render()))
	}
}

// TestLiveUSSD haqiqiy modemga bitta *107# yuboradi. Faqat qo'lda va kamdan-kam:
// PCINFO_LIVE_USSD=1 (operator tez-tez so'rovni cheklashi mumkin).
func TestLiveUSSD(t *testing.T) {
	if os.Getenv("PCINFO_LIVE_USSD") == "" {
		t.Skip("PCINFO_LIVE_USSD=1 bilan ishga tushiring")
	}
	msg := startUSSD(modem.New(modem.DefaultBaseURL))()
	events := msg.(ussdStartedMsg).events
	for ev := range events {
		switch e := ev.(type) {
		case ussdProgressMsg:
			fmt.Println("bosqich:", stageText(e.progress))
		case ussdDoneMsg:
			if e.err != nil {
				t.Fatal(e.err)
			}
			fmt.Printf("natija: %+v\n", e.balance)
			if e.balance.MB != nil {
				fmt.Println("MB:", *e.balance.MB)
			}
		}
	}
}
