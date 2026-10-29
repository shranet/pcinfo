// Package network internet liniyalarini (optika, USB modem, VPN) aniqlaydi:
// qaysi biri aktiv, har biri ishlayaptimi va qancha trafik o'tyapti.
// Faqat o'qiydi: /proc/net/route, /proc/net/dev va interfeys manzillari.
package network

import (
	"bufio"
	"context"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"os"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	procRoute = "/proc/net/route"
	procDev   = "/proc/net/dev"

	// Linux route bayroqlari (include/uapi/linux/route.h).
	routeFlagUp      = 0x1
	routeFlagGateway = 0x2

	vpnIfacePrefix = "tailscale"

	probeTarget  = "1.1.1.1:443"
	probeTimeout = 3 * time.Second
)

// Liniya turlari gateway subneti bo'yicha aniqlanadi (D-005). Tailscale ham
// 100.64.0.0/10 manzil oladi, shuning uchun "100." prefiksiga ishonilmaydi.
var (
	usbModemNet = netip.MustParsePrefix("192.168.8.0/24")
	fiberNet    = netip.MustParsePrefix("192.168.100.0/24")
)

// Kind — liniya turi.
type Kind string

const (
	KindFiber    Kind = "fiber"
	KindUSBModem Kind = "usb-modem"
	KindVPN      Kind = "vpn"
	KindOther    Kind = "other"
)

// Route — /proc/net/route dagi bitta qator.
type Route struct {
	Iface     string
	Gateway   netip.Addr
	Metric    int
	IsDefault bool
	IsUp      bool
}

// Counter — interfeys orqali o'tgan jami baytlar.
type Counter struct {
	Rx uint64 `json:"rx"`
	Tx uint64 `json:"tx"`
}

// Iface — interfeys va uning manzillari (tizimdan yoki testdan).
type Iface struct {
	Name  string
	IsUp  bool
	Addrs []netip.Prefix
}

// Link — ko'rsatiladigan bitta liniya.
type Link struct {
	Iface      string   `json:"iface"`
	Kind       Kind     `json:"kind"`
	IPv4       []string `json:"ipv4"`
	IPv6Global []string `json:"ipv6Global"`
	Gateway    string   `json:"gateway"`
	Metric     int      `json:"metric"`
	HasDefault bool     `json:"hasDefault"`
	IsActive   bool     `json:"isActive"`
	IsUp       bool     `json:"isUp"`
	Counter    Counter  `json:"counter"`
}

// ProbeResult — liniya orqali tashqi manzilga ulanish natijasi.
type ProbeResult struct {
	OK      bool          `json:"ok"`
	Latency time.Duration `json:"latency"`
	Err     string        `json:"err"`
}

// ParseRoutes /proc/net/route formatini o'qiydi.
func ParseRoutes(r io.Reader) ([]Route, error) {
	const (
		colIface, colDest, colGateway, colFlags, colMetric, colMask = 0, 1, 2, 3, 6, 7
		minCols                                                     = 8
	)
	var routes []Route
	sc := bufio.NewScanner(r)
	isHeader := true
	for sc.Scan() {
		if isHeader {
			isHeader = false
			continue
		}
		f := strings.Fields(sc.Text())
		if len(f) < minCols {
			continue
		}
		dest, err1 := parseHexIPv4(f[colDest])
		gw, err2 := parseHexIPv4(f[colGateway])
		mask, err3 := parseHexIPv4(f[colMask])
		flags, err4 := strconv.ParseUint(f[colFlags], 16, 32)
		metric, err5 := strconv.Atoi(f[colMetric])
		if err := errors.Join(err1, err2, err3, err4, err5); err != nil {
			return nil, fmt.Errorf("route qatori %q: %w", sc.Text(), err)
		}
		routes = append(routes, Route{
			Iface:     f[colIface],
			Gateway:   gw,
			Metric:    metric,
			IsDefault: dest.IsUnspecified() && mask.IsUnspecified() && flags&routeFlagGateway != 0,
			IsUp:      flags&routeFlagUp != 0,
		})
	}
	return routes, sc.Err()
}

// parseHexIPv4 — /proc/net/route manzillari little-endian hex ko'rinishida.
func parseHexIPv4(s string) (netip.Addr, error) {
	b, err := hex.DecodeString(s)
	if err != nil || len(b) != net.IPv4len {
		return netip.Addr{}, fmt.Errorf("noto'g'ri hex manzil %q", s)
	}
	var a [4]byte
	binary.BigEndian.PutUint32(a[:], binary.LittleEndian.Uint32(b))
	return netip.AddrFrom4(a), nil
}

// ParseDev /proc/net/dev dan interfeys hisoblagichlarini o'qiydi.
func ParseDev(r io.Reader) (map[string]Counter, error) {
	const (
		colRxBytes = 0
		colTxBytes = 8
		minCols    = 9
		headerRows = 2
	)
	counters := make(map[string]Counter)
	sc := bufio.NewScanner(r)
	for row := 0; sc.Scan(); row++ {
		if row < headerRows {
			continue
		}
		name, rest, ok := strings.Cut(sc.Text(), ":")
		if !ok {
			continue
		}
		f := strings.Fields(rest)
		if len(f) < minCols {
			continue
		}
		rx, err1 := strconv.ParseUint(f[colRxBytes], 10, 64)
		tx, err2 := strconv.ParseUint(f[colTxBytes], 10, 64)
		if err := errors.Join(err1, err2); err != nil {
			return nil, fmt.Errorf("dev qatori %q: %w", sc.Text(), err)
		}
		counters[strings.TrimSpace(name)] = Counter{Rx: rx, Tx: tx}
	}
	return counters, sc.Err()
}

// BuildLinks route, hisoblagich va interfeyslardan liniyalar ro'yxatini yasaydi.
// Faqat default route'i bor interfeyslar va VPN olinadi (docker, veth, lo emas).
func BuildLinks(routes []Route, counters map[string]Counter, ifaces []Iface) []Link {
	active := activeIface(routes, ifaces)
	var links []Link
	for _, ifc := range ifaces {
		def, hasDefault := defaultRoute(routes, ifc.Name)
		isVPN := strings.HasPrefix(ifc.Name, vpnIfacePrefix)
		if !hasDefault && !isVPN {
			continue
		}
		l := Link{
			Iface:      ifc.Name,
			IsUp:       ifc.IsUp,
			HasDefault: hasDefault,
			IsActive:   ifc.Name == active,
			Counter:    counters[ifc.Name],
		}
		for _, p := range ifc.Addrs {
			switch {
			case p.Addr().Is4():
				l.IPv4 = append(l.IPv4, p.String())
			case p.Addr().IsGlobalUnicast() && !p.Addr().IsPrivate():
				l.IPv6Global = append(l.IPv6Global, p.Addr().String())
			}
		}
		if hasDefault {
			l.Gateway = def.Gateway.String()
			l.Metric = def.Metric
		}
		l.Kind = classify(ifc, def.Gateway, isVPN)
		links = append(links, l)
	}
	slices.SortFunc(links, func(a, b Link) int {
		if a.HasDefault != b.HasDefault {
			if a.HasDefault {
				return -1
			}
			return 1
		}
		return a.Metric - b.Metric
	})
	return links
}

func classify(ifc Iface, gw netip.Addr, isVPN bool) Kind {
	if isVPN {
		return KindVPN
	}
	inNet := func(p netip.Prefix) bool {
		if gw.IsValid() && p.Contains(gw) {
			return true
		}
		return slices.ContainsFunc(ifc.Addrs, func(a netip.Prefix) bool { return p.Contains(a.Addr()) })
	}
	switch {
	case inNet(usbModemNet):
		return KindUSBModem
	case inNet(fiberNet):
		return KindFiber
	}
	return KindOther
}

func defaultRoute(routes []Route, iface string) (Route, bool) {
	best, found := Route{}, false
	for _, r := range routes {
		if r.Iface == iface && r.IsDefault && (!found || r.Metric < best.Metric) {
			best, found = r, true
		}
	}
	return best, found
}

// activeIface — trafik haqiqatda chiqadigan interfeys: ishlayotgan
// default route'lar ichida eng kichik metric'lisi.
func activeIface(routes []Route, ifaces []Iface) string {
	isUp := func(name string) bool {
		return slices.ContainsFunc(ifaces, func(i Iface) bool { return i.Name == name && i.IsUp })
	}
	best, found := Route{}, false
	for _, r := range routes {
		if r.IsDefault && r.IsUp && isUp(r.Iface) && (!found || r.Metric < best.Metric) {
			best, found = r, true
		}
	}
	return best.Iface
}

// Links tizimdan joriy liniyalarni o'qiydi.
func Links() ([]Link, error) {
	routes, err := readFile(procRoute, ParseRoutes)
	if err != nil {
		return nil, err
	}
	counters, err := readFile(procDev, ParseDev)
	if err != nil {
		return nil, err
	}
	ifaces, err := systemIfaces()
	if err != nil {
		return nil, err
	}
	return BuildLinks(routes, counters, ifaces), nil
}

func readFile[T any](path string, parse func(io.Reader) (T, error)) (T, error) {
	var zero T
	f, err := os.Open(path)
	if err != nil {
		return zero, err
	}
	defer f.Close()
	v, err := parse(f)
	if err != nil {
		return zero, fmt.Errorf("%s: %w", path, err)
	}
	return v, nil
}

func systemIfaces() ([]Iface, error) {
	nis, err := net.Interfaces()
	if err != nil {
		return nil, fmt.Errorf("interfeyslar: %w", err)
	}
	ifaces := make([]Iface, 0, len(nis))
	for _, ni := range nis {
		ifc := Iface{Name: ni.Name, IsUp: ni.Flags&net.FlagUp != 0}
		addrs, err := ni.Addrs()
		if err != nil {
			return nil, fmt.Errorf("%s manzillari: %w", ni.Name, err)
		}
		for _, a := range addrs {
			if ipn, ok := a.(*net.IPNet); ok {
				if p, err := netip.ParsePrefix(ipn.String()); err == nil {
					ifc.Addrs = append(ifc.Addrs, p)
				}
			}
		}
		ifaces = append(ifaces, ifc)
	}
	return ifaces, nil
}

// Probe iface orqali (SO_BINDTODEVICE, Linux 5.7+ da imtiyozsiz) tashqi
// manzilga TCP ulanish ochadi va yopadi. Hech narsa yuborilmaydi.
func Probe(ctx context.Context, iface string) ProbeResult {
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	start := time.Now()
	conn, err := BoundDialer(iface).DialContext(ctx, "tcp4", probeTarget)
	if err != nil {
		return ProbeResult{Err: shortNetError(err)}
	}
	latency := time.Since(start)
	conn.Close()
	return ProbeResult{OK: true, Latency: latency}
}

// BoundDialer — faqat iface orqali chiqadigan dialer.
func BoundDialer(iface string) *net.Dialer {
	return &net.Dialer{
		Control: func(_, _ string, c syscall.RawConn) error {
			var sockErr error
			err := c.Control(func(fd uintptr) {
				sockErr = syscall.SetsockoptString(int(fd), syscall.SOL_SOCKET, syscall.SO_BINDTODEVICE, iface)
			})
			return errors.Join(err, sockErr)
		},
	}
}

func shortNetError(err error) string {
	var ne net.Error
	switch {
	case errors.As(err, &ne) && ne.Timeout():
		return "timeout"
	case errors.Is(err, syscall.ECONNREFUSED):
		return "rad etildi"
	case errors.Is(err, syscall.ENETUNREACH), errors.Is(err, syscall.EHOSTUNREACH):
		return "tarmoq yo'q"
	case errors.Is(err, syscall.EPERM):
		return "ruxsat yo'q"
	}
	return "xato"
}

// Rate — ikki o'lchov orasidagi soniyadagi bayt.
func Rate(prev, cur Counter, elapsed time.Duration) (rx, tx float64) {
	secs := elapsed.Seconds()
	if secs <= 0 || cur.Rx < prev.Rx || cur.Tx < prev.Tx {
		return 0, 0
	}
	return float64(cur.Rx-prev.Rx) / secs, float64(cur.Tx-prev.Tx) / secs
}
