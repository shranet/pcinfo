package projects

import (
	"bufio"
	"io"
	"net"
	"slices"
	"strings"
)

const publicMarker = "# pvds-public"

// SiteKind — sayt qaysi router'da: long.uz preview yoki faqat shu PC (localhost).
type SiteKind string

const (
	SitePreview SiteKind = "preview"
	SiteLocal   SiteKind = "local"
)

// Site — router fayldagi bitta host.
type Site struct {
	Host        string   `json:"host"`
	Kind        SiteKind `json:"kind"`
	File        string   `json:"file"`
	Upstreams   []string `json:"upstreams"`
	PublicPaths []string `json:"publicPaths"`
}

// ParseCaddy router faylini o'qiydi: sayt bloklari, ulardagi reverse_proxy
// konteynerlari va `# pvds-public HOST PATH` qatorlari. Faqat proyektlarda
// ishlatilgan oddiy sintaksis qo'llanadi (snippet/import yo'q).
func ParseCaddy(r io.Reader, file string, kind SiteKind) ([]Site, error) {
	var (
		sites   []Site
		current []int // ochiq blokdagi saytlar indekslari
		depth   int
		public  = map[string][]string{}
	)
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if rest, ok := strings.CutPrefix(line, publicMarker); ok {
			if f := strings.Fields(rest); len(f) == 2 {
				public[f[0]] = append(public[f[0]], f[1])
			}
			continue
		}
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if depth == 0 && strings.HasSuffix(line, "{") {
			current = current[:0]
			for _, addr := range fields[:len(fields)-1] {
				if host := siteHost(addr); host != "" {
					current = append(current, len(sites))
					sites = append(sites, Site{Host: host, Kind: kind, File: file})
				}
			}
		} else if depth > 0 && fields[0] == "reverse_proxy" {
			for _, up := range fields[1:] {
				if name := upstreamName(up); name != "" {
					for _, i := range current {
						if !slices.Contains(sites[i].Upstreams, name) {
							sites[i].Upstreams = append(sites[i].Upstreams, name)
						}
					}
				}
			}
		}
		depth += strings.Count(line, "{") - strings.Count(line, "}")
		depth = max(depth, 0)
	}
	for i := range sites {
		sites[i].PublicPaths = public[sites[i].Host]
	}
	return sites, sc.Err()
}

// siteHost "http://lens.long.uz," dan "lens.long.uz" ni oladi.
func siteHost(addr string) string {
	addr = strings.TrimSuffix(addr, ",")
	for _, p := range []string{"http://", "https://"} {
		addr = strings.TrimPrefix(addr, p)
	}
	if h, _, err := net.SplitHostPort(addr); err == nil {
		addr = h
	}
	if addr == "" || strings.ContainsAny(addr, "{}@/") {
		return ""
	}
	return addr
}

// upstreamName "lens-web-1:8080" dan konteyner nomini oladi; matcher va
// placeholder'lar ("@api", "{upstream}") tashlanadi.
func upstreamName(up string) string {
	if strings.HasPrefix(up, "@") || strings.HasPrefix(up, "{") || up == "{" {
		return ""
	}
	up = strings.TrimPrefix(strings.TrimPrefix(up, "http://"), "https://")
	if h, _, err := net.SplitHostPort(up); err == nil {
		up = h
	}
	return up
}
