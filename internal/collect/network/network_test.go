package network

import (
	"net/netip"
	"os"
	"strings"
	"testing"
	"time"
)

func mustOpen(t *testing.T, path string) *os.File {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.Close() })
	return f
}

func TestParseRoutes(t *testing.T) {
	routes, err := ParseRoutes(mustOpen(t, "testdata/route"))
	if err != nil {
		t.Fatal(err)
	}
	var defaults []Route
	for _, r := range routes {
		if r.IsDefault {
			defaults = append(defaults, r)
		}
	}
	if len(defaults) != 2 {
		t.Fatalf("default route'lar = %d, want 2", len(defaults))
	}
	if defaults[0].Iface != "enp6s0" || defaults[0].Gateway.String() != "192.168.100.1" || defaults[0].Metric != 100 {
		t.Errorf("birinchi default = %+v", defaults[0])
	}
	if defaults[1].Gateway.String() != "192.168.8.1" || defaults[1].Metric != 700 {
		t.Errorf("ikkinchi default = %+v", defaults[1])
	}
}

func TestParseRoutesErrors(t *testing.T) {
	tests := []struct{ name, in string }{
		{"noto'g'ri hex", "Iface\tDestination\nenp6s0\tZZZZZZZZ\t00000000\t0003\t0\t0\t100\t00000000\n"},
		{"qisqa hex", "h\nenp6s0\t0000\t00000000\t0003\t0\t0\t100\t00000000\n"},
		{"metric son emas", "h\nenp6s0\t00000000\t00000000\t0003\t0\t0\tx\t00000000\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := ParseRoutes(strings.NewReader(tt.in)); err == nil {
				t.Error("xato kutilgan edi")
			}
		})
	}
}

func TestParseRoutesShortLinesIgnored(t *testing.T) {
	routes, err := ParseRoutes(strings.NewReader("header\nfoo bar\n\n"))
	if err != nil || len(routes) != 0 {
		t.Errorf("routes=%v err=%v", routes, err)
	}
}

func TestParseDev(t *testing.T) {
	c, err := ParseDev(mustOpen(t, "testdata/dev"))
	if err != nil {
		t.Fatal(err)
	}
	if got := c["enp6s0"]; got.Rx != 987654321 || got.Tx != 123456789 {
		t.Errorf("enp6s0 = %+v", got)
	}
	if got := c["enx0c5b8f279a64"]; got.Rx != 5555 || got.Tx != 4444 {
		t.Errorf("enx = %+v", got)
	}
}

func prefixes(ss ...string) []netip.Prefix {
	var ps []netip.Prefix
	for _, s := range ss {
		ps = append(ps, netip.MustParsePrefix(s))
	}
	return ps
}

func testIfaces() []Iface {
	return []Iface{
		{Name: "lo", IsUp: true, Addrs: prefixes("127.0.0.1/8")},
		{Name: "enp6s0", IsUp: true, Addrs: prefixes("192.168.100.19/24", "2a05:45c2:50e6:3100::1/64", "fe80::1/64")},
		{Name: "tailscale0", IsUp: true, Addrs: prefixes("100.64.192.72/32")},
		{Name: "docker0", IsUp: true, Addrs: prefixes("172.17.0.1/16")},
		{Name: "enx0c5b8f279a64", IsUp: true, Addrs: prefixes("192.168.8.100/24")},
	}
}

func TestBuildLinks(t *testing.T) {
	routes, err := ParseRoutes(mustOpen(t, "testdata/route"))
	if err != nil {
		t.Fatal(err)
	}
	links := BuildLinks(routes, map[string]Counter{"enp6s0": {Rx: 1}}, testIfaces())

	got := map[string]Link{}
	for _, l := range links {
		got[l.Iface] = l
	}
	if len(links) != 3 {
		t.Fatalf("liniyalar = %d (%v), want 3 (docker/lo kirmasligi kerak)", len(links), links)
	}
	if l := got["enp6s0"]; l.Kind != KindFiber || !l.IsActive || l.Counter.Rx != 1 || len(l.IPv6Global) != 1 {
		t.Errorf("optika = %+v", l)
	}
	if l := got["enx0c5b8f279a64"]; l.Kind != KindUSBModem || l.IsActive || l.Metric != 700 {
		t.Errorf("USB = %+v", l)
	}
	if l := got["tailscale0"]; l.Kind != KindVPN || l.IsActive {
		t.Errorf("tailscale optika deb adashtirilmasligi kerak: %+v", l)
	}
	if links[0].Iface != "enp6s0" {
		t.Errorf("tartib: birinchi %s", links[0].Iface)
	}
}

func TestBuildLinksFailover(t *testing.T) {
	routes, err := ParseRoutes(mustOpen(t, "testdata/route"))
	if err != nil {
		t.Fatal(err)
	}
	ifaces := testIfaces()
	for i := range ifaces {
		if ifaces[i].Name == "enp6s0" {
			ifaces[i].IsUp = false
		}
	}
	for _, l := range BuildLinks(routes, nil, ifaces) {
		if l.IsActive != (l.Iface == "enx0c5b8f279a64") {
			t.Errorf("optika o'chganda USB aktiv bo'lishi kerak: %+v", l)
		}
	}
}

func TestRate(t *testing.T) {
	rx, tx := Rate(Counter{Rx: 100, Tx: 50}, Counter{Rx: 300, Tx: 150}, 2*time.Second)
	if rx != 100 || tx != 50 {
		t.Errorf("rate = %v %v", rx, tx)
	}
	if rx, tx := Rate(Counter{Rx: 300}, Counter{Rx: 100}, time.Second); rx != 0 || tx != 0 {
		t.Errorf("hisoblagich qaytganda 0 bo'lishi kerak: %v %v", rx, tx)
	}
	if rx, _ := Rate(Counter{}, Counter{Rx: 5}, 0); rx != 0 {
		t.Errorf("nol vaqt: %v", rx)
	}
}
