package tui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/shranet/pcinfo/internal/collect/modem"
	"github.com/shranet/pcinfo/internal/collect/network"
	"github.com/shranet/pcinfo/internal/format"
	"github.com/shranet/pcinfo/internal/tui/theme"
)

const (
	mbPerGB       = 1024
	rawWrapIndent = "  "
)

func kindLabel(k network.Kind) string {
	switch k {
	case network.KindFiber:
		return "Optika"
	case network.KindUSBModem:
		return "USB modem"
	case network.KindVPN:
		return "VPN"
	}
	return "Boshqa"
}

var linkColumns = []cell{
	{text: "LINIYA", width: 13},
	{text: "INTERFEYS", width: 16},
	{text: "IPv4", width: 18},
	{text: "GATEWAY", width: 15},
	{text: "METRIC", width: 6, isRight: true},
	{text: "TEKSHIRUV", width: 13},
	{text: "↓ TEZLIK", width: 11, isRight: true},
	{text: "↑ TEZLIK", width: 11, isRight: true},
	{text: "TARIX"},
}

// activeLink — trafik chiqayotgan liniya.
func (m Model) activeLink() (network.Link, bool) {
	for _, l := range m.links.data {
		if l.IsActive {
			return l, true
		}
	}
	return network.Link{}, false
}

func (m Model) probeText(iface string) (string, lipgloss.Style) {
	p, ok := m.probes[iface]
	switch {
	case !ok:
		return "…", theme.Muted
	case p.OK:
		return fmt.Sprintf("✓ %d ms", p.Latency.Milliseconds()), theme.OK
	}
	return "✗ " + p.Err, theme.Err
}

func (m Model) viewNetwork() []string {
	var lines []string
	lines = append(lines, theme.Heading.Render("Internet liniyalari"))
	switch {
	case m.links.isFailed():
		lines = append(lines, theme.Err.Render("o'qilmadi: "+errText(m.links.err)))
	case !m.links.isLoaded:
		lines = append(lines, theme.Muted.Render("yuklanmoqda…"))
	case len(m.links.data) == 0:
		lines = append(lines, theme.Err.Render("default route yo'q — internet yo'q"))
	default:
		lines = append(lines, m.linkTable()...)
	}
	if active, ok := m.activeLink(); ok && active.Kind == network.KindUSBModem {
		lines = append(lines, theme.Warn.Render("⚠ Trafik USB modem orqali ketyapti — limitli trafik!"))
	}
	lines = append(lines, "")
	lines = append(lines, m.modemLines()...)
	lines = append(lines, "")
	lines = append(lines, m.ussdLines()...)
	return lines
}

func (m Model) linkTable() []string {
	head := make([]cell, len(linkColumns))
	for i, c := range linkColumns {
		c.style = theme.Header
		head[i] = c
	}
	lines := []string{renderRow(head, m.width, false)}
	for _, l := range m.links.data {
		marker, kindStyle := "  ", theme.Text
		if l.IsActive {
			marker, kindStyle = "▶ ", theme.OK
			if l.Kind == network.KindUSBModem {
				kindStyle = theme.Warn
			}
		}
		if !l.IsUp {
			kindStyle = theme.Err
		}
		probe, probeStyle := "—", theme.Muted
		if l.HasDefault {
			probe, probeStyle = m.probeText(l.Iface)
		}
		r := m.rates[l.Iface]
		gw, metric := l.Gateway, ""
		if l.HasDefault {
			metric = fmt.Sprint(l.Metric)
		}
		lines = append(lines, renderRow([]cell{
			{text: marker + kindLabel(l.Kind), style: kindStyle.Bold(l.IsActive), width: linkColumns[0].width},
			{text: l.Iface, style: theme.Text, width: linkColumns[1].width},
			{text: strings.Join(l.IPv4, " "), style: theme.Text, width: linkColumns[2].width},
			{text: gw, style: theme.Muted, width: linkColumns[3].width},
			{text: metric, style: theme.Muted, width: linkColumns[4].width, isRight: true},
			{text: probe, style: probeStyle, width: linkColumns[5].width},
			{text: format.Rate(r.rx), style: theme.Text, width: linkColumns[6].width, isRight: true},
			{text: format.Rate(r.tx), style: theme.Text, width: linkColumns[7].width, isRight: true},
			{text: sparkline(m.rxHist[l.Iface], m.width, 0), style: theme.Accent},
		}, m.width, false))
		if ip, ok := m.publicIPs[l.Iface]; ok {
			lines = append(lines, theme.Muted.Render("    public IP: "+ip))
		}
		if len(l.IPv6Global) > 0 {
			lines = append(lines, theme.Muted.Render("    IPv6: "+strings.Join(l.IPv6Global, " ")))
		}
	}
	return lines
}

func qualityStyle(q modem.Quality) (lipgloss.Style, string) {
	switch q {
	case modem.QualityGood:
		return theme.OK, "yaxshi"
	case modem.QualityFair:
		return theme.Warn, "o'rtacha"
	case modem.QualityPoor:
		return theme.Err, "yomon"
	}
	return theme.Muted, "—"
}

func (m Model) modemLines() []string {
	lines := []string{theme.Heading.Render("USB modem (Huawei HiLink)")}
	switch {
	case m.modemInfo.isFailed():
		return append(lines, theme.Err.Render("  modem javob bermadi: "+errText(m.modemInfo.err)))
	case !m.modemInfo.isLoaded:
		return append(lines, theme.Muted.Render("  yuklanmoqda…"))
	case m.modemInfo.err != nil:
		lines = append(lines, theme.Err.Render("  oxirgi so'rov xatosi: "+errText(m.modemInfo.err)))
	}
	info := m.modemInfo.data
	connStyle := theme.Warn
	if info.Status.ConnectionStatus == "901" {
		connStyle = theme.OK
	}
	lines = append(lines, "  "+connStyle.Render(modem.ConnectionText(info.Status.ConnectionStatus))+
		theme.Muted.Render(" · ")+theme.Text.Render(modem.NetworkTypeText(info.Status.CurrentNetworkType))+
		theme.Muted.Render(fmt.Sprintf(" · signal %s/%s", orDash(info.Status.SignalIcon), orDash(info.Status.MaxSignal))))

	rsrpStyle, rsrpText := qualityStyle(modem.RSRPQuality(info.Signal.RSRP))
	sinrStyle, sinrText := qualityStyle(modem.SINRQuality(info.Signal.SINR))
	lines = append(lines, "  "+theme.Muted.Render("RSSI ")+theme.Text.Render(orDash(info.Signal.RSSI))+
		theme.Muted.Render("   RSRP ")+rsrpStyle.Render(orDash(info.Signal.RSRP)+" ("+rsrpText+")")+
		theme.Muted.Render("   SINR ")+sinrStyle.Render(orDash(info.Signal.SINR)+" ("+sinrText+")")+
		theme.Muted.Render("   RSRQ ")+theme.Text.Render(orDash(info.Signal.RSRQ)))

	t := info.Traffic
	lines = append(lines, "  "+theme.Muted.Render("Tezlik ")+
		theme.Text.Render("↓ "+format.Rate(float64(t.CurrentDownloadRate))+"  ↑ "+format.Rate(float64(t.CurrentUploadRate)))+
		theme.Muted.Render("   sessiya ")+theme.Text.Render("↓ "+format.Bytes(t.CurrentDownload)+"  ↑ "+format.Bytes(t.CurrentUpload))+
		theme.Muted.Render("   ulangan ")+theme.Text.Render(format.Duration(time.Duration(t.CurrentConnectTime)*time.Second))+
		theme.Muted.Render("   jami ")+theme.Text.Render("↓ "+format.Bytes(t.TotalDownload)+"  ↑ "+format.Bytes(t.TotalUpload)))
	return lines
}

func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

func (m Model) ussdLines() []string {
	lines := []string{theme.Heading.Render("Balans (" + modem.BalanceCode + ")")}
	u := m.ussd
	switch {
	case u.isRunning:
		lines = append(lines, "  "+theme.Warn.Render("⏳ "+stageText(u.progress)))
	case u.err != nil:
		lines = append(lines, "  "+theme.Err.Render("✗ xato: "+errText(u.err)))
	case !u.doneAt.IsZero():
		lines = append(lines, "  "+theme.OK.Render("✓ olindi "+format.Ago(u.doneAt, m.now)))
	case !m.opts.USSD:
		lines = append(lines, theme.Muted.Render("  --no-ussd bilan o'chirilgan; u — qo'lda so'rash"))
	}
	if u.notice != "" {
		lines = append(lines, "  "+theme.Muted.Render(u.notice))
	}
	if b := u.balance; b != nil {
		var parts []string
		if b.Balance != "" {
			parts = append(parts, theme.Muted.Render("Balans ")+theme.Bold.Render(b.Balance))
		}
		if b.MB != nil {
			parts = append(parts, theme.Muted.Render("Internet ")+theme.Bold.Render(fmt.Sprintf("%.1f GB", float64(*b.MB)/mbPerGB))+theme.Muted.Render(fmt.Sprintf(" (%d MB)", *b.MB)))
		}
		if b.Minutes != nil {
			parts = append(parts, theme.Muted.Render("Daqiqa ")+theme.Bold.Render(fmt.Sprint(*b.Minutes)))
		}
		if b.SMS != nil {
			parts = append(parts, theme.Muted.Render("SMS ")+theme.Bold.Render(fmt.Sprint(*b.SMS)))
		}
		if len(parts) > 0 {
			lines = append(lines, "  "+strings.Join(parts, theme.Track.Render("  │  ")))
		}
		wrapped := ansi.Wordwrap(b.Raw, max(10, m.width-len(rawWrapIndent)), " ")
		for _, l := range strings.Split(wrapped, "\n") {
			lines = append(lines, theme.Muted.Render(rawWrapIndent+l))
		}
	}
	return lines
}

func stageText(p modem.Progress) string {
	switch p.Stage {
	case modem.StageSession:
		return "sessiya olinmoqda…"
	case modem.StageSent:
		return "yuborildi, javob kutilmoqda…"
	case modem.StageWaiting:
		return fmt.Sprintf("kutilmoqda (%ds / %ds)", int(p.Waited.Seconds()), int(p.MaxWait.Seconds()))
	}
	return "…"
}
