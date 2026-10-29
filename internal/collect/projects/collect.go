package projects

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

const (
	caddyExt      = ".caddy"
	maxCaddyBytes = 256 << 10
)

// Paths — o'qiladigan joylar (testda vaqtinchalik papkalar beriladi).
type Paths struct {
	Root         string
	PreviewSites string
	LocalSites   string
}

// DefaultPaths — shu PC dagi haqiqiy joylar.
func DefaultPaths() Paths {
	return Paths{Root: DefaultRoot, PreviewSites: DefaultPreviewSites, LocalSites: DefaultLocalSites}
}

// Collect workspace va router fayllarini o'qib, konteynerlar bilan guruhlaydi.
// Xatolar natijani to'xtatmaydi, errs ga yig'iladi.
func Collect(ctx context.Context, p Paths, containers []Container) (Result, []string) {
	var errs []string
	workspaces, err := ScanWorkspaces(ctx, p.Root)
	if err != nil {
		errs = append(errs, err.Error())
	}
	var sites []Site
	for _, dir := range []struct {
		path string
		kind SiteKind
	}{{p.PreviewSites, SitePreview}, {p.LocalSites, SiteLocal}} {
		s, err := readSites(dir.path, dir.kind)
		if err != nil {
			errs = append(errs, err.Error())
		}
		sites = append(sites, s...)
	}
	return Group(p.Root, workspaces, containers, sites), errs
}

func readSites(dir string, kind SiteKind) ([]Site, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var sites []Site
	for _, e := range entries {
		if !e.Type().IsRegular() || !strings.HasSuffix(e.Name(), caddyExt) {
			continue
		}
		data, err := readLimited(filepath.Join(dir, e.Name()), maxCaddyBytes)
		if err != nil {
			continue
		}
		s, err := ParseCaddy(bytes.NewReader(data), e.Name(), kind)
		if err != nil {
			continue
		}
		sites = append(sites, s...)
	}
	slices.SortFunc(sites, func(a, b Site) int { return strings.Compare(a.Host, b.Host) })
	return sites, nil
}
