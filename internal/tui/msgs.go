package tui

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/shranet/pcinfo/internal/collect/dockerapi"
	"github.com/shranet/pcinfo/internal/collect/modem"
	"github.com/shranet/pcinfo/internal/collect/network"
	"github.com/shranet/pcinfo/internal/collect/process"
	"github.com/shranet/pcinfo/internal/collect/system"
	"github.com/shranet/pcinfo/internal/safetext"
	"github.com/shranet/pcinfo/internal/snapshot"
)

// Yangilash davrlari: tez manbalar (CPU, tarmoq) sekinlaridan (Docker, claude) alohida.
const (
	systemInterval  = 2 * time.Second
	processInterval = 2 * time.Second
	linksInterval   = 2 * time.Second
	modemInterval   = 5 * time.Second
	probeInterval   = 15 * time.Second
	claudeInterval  = 10 * time.Second
	dockerInterval  = 10 * time.Second

	collectTimeout = 15 * time.Second
	ussdTimeout    = 90 * time.Second
	ussdQueueSize  = 8
)

type source int

const (
	srcSystem source = iota
	srcProcess
	srcLinks
	srcModem
	srcProbe
	srcClaude
	srcDocker
)

type tickMsg struct{ src source }

type systemMsg struct {
	snap system.Snapshot
	err  error
}

type processMsg struct {
	procs []process.Process
	err   error
}

type linksMsg struct {
	links []network.Link
	at    time.Time
	err   error
}

type probeResult struct {
	Iface  string
	Result network.ProbeResult
}

type probeMsg struct{ results []probeResult }

type publicIPMsg struct {
	iface, ip string
	err       error
}

type modemMsg struct {
	info modem.Info
	err  error
}

type claudeMsg struct {
	snap snapshot.ClaudeSide
	err  error
}

type dockerMsg struct{ summaries []dockerapi.Summary }

type ussdStartedMsg struct{ events <-chan tea.Msg }

type ussdProgressMsg struct{ progress modem.Progress }

type ussdDoneMsg struct {
	balance modem.Balance
	err     error
}

func tick(src source, d time.Duration) tea.Cmd {
	return tea.Tick(d, func(time.Time) tea.Msg { return tickMsg{src: src} })
}

func withTimeout() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), collectTimeout)
}

func fetchSystem(s *system.Sampler) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := withTimeout()
		defer cancel()
		snap, err := s.Collect(ctx)
		safetext.CleanAll(&snap)
		return systemMsg{snap: snap, err: err}
	}
}

func fetchProcesses(s *process.Sampler) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := withTimeout()
		defer cancel()
		procs, err := s.Sample(ctx)
		safetext.CleanAll(&procs)
		return processMsg{procs: procs, err: err}
	}
}

func fetchLinks() tea.Msg {
	links, err := network.Links()
	safetext.CleanAll(&links)
	return linksMsg{links: links, at: time.Now(), err: err}
}

func fetchProbes(ifaces []string) tea.Cmd {
	return func() tea.Msg {
		results := make([]probeResult, len(ifaces))
		done := make(chan struct{}, len(ifaces))
		for i, iface := range ifaces {
			go func() {
				results[i] = probeResult{Iface: iface, Result: network.Probe(context.Background(), iface)}
				done <- struct{}{}
			}()
		}
		for range ifaces {
			<-done
		}
		return probeMsg{results: results}
	}
}

func fetchPublicIP(iface string) tea.Cmd {
	return func() tea.Msg {
		ip, err := network.PublicIP(context.Background(), iface)
		return publicIPMsg{iface: iface, ip: ip, err: err}
	}
}

func fetchModem(c *modem.Client) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := withTimeout()
		defer cancel()
		info, err := c.Info(ctx)
		safetext.CleanAll(&info)
		return modemMsg{info: info, err: err}
	}
}

func fetchClaude(f claudeFetcher) tea.Cmd {
	return func() tea.Msg {
		snap, err := f.Fetch(context.Background())
		return claudeMsg{snap: snap, err: err}
	}
}

func fetchDocker() tea.Msg {
	ctx, cancel := withTimeout()
	defer cancel()
	s := snapshot.CollectLocalDocker(ctx)
	safetext.CleanAll(&s)
	return dockerMsg{summaries: s}
}

// startUSSD USSD'ni fonda boshlaydi; bosqichlar kanal orqali xabar bo'lib keladi.
func startUSSD(c *modem.Client) tea.Cmd {
	return func() tea.Msg {
		events := make(chan tea.Msg, ussdQueueSize)
		go func() {
			defer close(events)
			ctx, cancel := context.WithTimeout(context.Background(), ussdTimeout)
			defer cancel()
			bal, err := c.USSD(ctx, modem.BalanceCode, func(p modem.Progress) {
				select {
				case events <- ussdProgressMsg{progress: p}:
				default: // UI ulgurmasa oraliq bosqichni tashlab yuboramiz
				}
			})
			safetext.CleanAll(&bal)
			events <- ussdDoneMsg{balance: bal, err: err}
		}()
		return ussdStartedMsg{events: events}
	}
}

func waitUSSD(events <-chan tea.Msg) tea.Cmd {
	return func() tea.Msg {
		msg, ok := <-events
		if !ok {
			return nil
		}
		return msg
	}
}
