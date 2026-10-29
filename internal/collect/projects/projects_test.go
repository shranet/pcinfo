package projects

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const shopCaddy = `# izoh { qavs } bilan
# pvds-public api-shop.long.uz /webhook/click/
# pvds-public shop.long.uz /api/health
# pvds-public noto'g'ri
http://shop.long.uz {
	handle /api/* {
		reverse_proxy shop-api-1:8080
	}
	handle {
		reverse_proxy shop-web-1:80
	}
}
http://api-shop.long.uz, http://admin-shop.long.uz:80 {
	@blocked path /dev
	reverse_proxy @blocked {upstream}
	reverse_proxy http://shop-api-1:8080 shop-api-2:8080
}
`

func TestParseCaddy(t *testing.T) {
	sites, err := ParseCaddy(strings.NewReader(shopCaddy), "shop.caddy", SitePreview)
	if err != nil {
		t.Fatal(err)
	}
	if len(sites) != 3 {
		t.Fatalf("saytlar = %d: %+v", len(sites), sites)
	}
	if s := sites[0]; s.Host != "shop.long.uz" || strings.Join(s.Upstreams, ",") != "shop-api-1,shop-web-1" ||
		strings.Join(s.PublicPaths, ",") != "/api/health" || s.Kind != SitePreview || s.File != "shop.caddy" {
		t.Errorf("shop = %+v", s)
	}
	for _, s := range sites[1:] {
		if strings.Join(s.Upstreams, ",") != "shop-api-1,shop-api-2" {
			t.Errorf("%s upstreams = %v", s.Host, s.Upstreams)
		}
	}
	if sites[2].Host != "admin-shop.long.uz" {
		t.Errorf("port olib tashlanishi kerak: %q", sites[2].Host)
	}
	if strings.Join(sites[1].PublicPaths, ",") != "/webhook/click/" {
		t.Errorf("api public = %v", sites[1].PublicPaths)
	}
}

func TestParseCaddyEmpty(t *testing.T) {
	sites, err := ParseCaddy(strings.NewReader(""), "x", SiteLocal)
	if err != nil || len(sites) != 0 {
		t.Errorf("sites=%v err=%v", sites, err)
	}
}

func TestImageTag(t *testing.T) {
	tests := map[string]string{
		"127.0.0.1:5000/lens/backend:v0.3.0@sha256:abc": "backend:v0.3.0",
		"127.0.0.1:5000/lens/backend@sha256:abc":        "",
		"nginx:1.27":                                    "nginx:1.27",
		"":                                              "",
	}
	for in, want := range tests {
		if got := imageTag(in); got != want {
			t.Errorf("imageTag(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParsePorcelain(t *testing.T) {
	var r Repo
	parsePorcelain([]byte("# branch.oid abc\n# branch.head main\n# branch.upstream origin/main\n# branch.ab +2 -1\n1 .M N... 100644 100644 100644 a b f.go\n? new.txt\n"), &r)
	if r.Branch != "main" || !r.HasUpstream || r.Ahead != 2 || r.Behind != 1 || r.Dirty != 2 {
		t.Errorf("repo = %+v", r)
	}
	var empty Repo
	parsePorcelain(nil, &empty)
	if empty.Dirty != 0 || empty.HasUpstream {
		t.Errorf("bo'sh = %+v", empty)
	}
}

func TestLimitedWriter(t *testing.T) {
	var sb strings.Builder
	w := &limitedWriter{w: &sb, remaining: 4}
	n, err := w.Write([]byte("abcdef"))
	if n != 6 || err != nil || sb.String() != "abcd" {
		t.Errorf("n=%d err=%v got=%q", n, err, sb.String())
	}
	w.Write([]byte("zz"))
	if sb.String() != "abcd" {
		t.Errorf("chegaradan keyin yozilmasligi kerak: %q", sb.String())
	}
}

func writeFile(t *testing.T, name, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func run(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command(gitBinary, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func TestScanWorkspaces(t *testing.T) {
	if _, err := os.Stat(gitBinary); err != nil {
		t.Skip("git yo'q")
	}
	root := t.TempDir()
	repo := filepath.Join(root, "ruvex-shop", "shop-api")
	writeFile(t, filepath.Join(repo, "main.go"), "package main\n")
	run(t, repo, "init", "-q", "-b", "main")
	run(t, repo, "add", ".")
	run(t, repo, "commit", "-qm", "init")
	writeFile(t, filepath.Join(repo, "dirty.txt"), "x")
	writeFile(t, filepath.Join(root, "ruvex-shop", "shop-docs", "deploy.json"),
		`{"apps":[{"pull":"127.0.0.1:5000/shop/api:v1.2.0@sha256:aa"},{"pull":"127.0.0.1:5000/shop/api:v1.2.0@sha256:aa"}]}`)
	writeFile(t, filepath.Join(root, "private", "app", "CLAUDE.md"), "# x\n\npreview: local\n")
	writeFile(t, filepath.Join(root, "other", "CLAUDE.md"), "bu yerda preview: local emas, matn ichida\n")
	writeFile(t, filepath.Join(root, ".hidden", "x"), "")
	if err := os.Symlink("/etc", filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	indexBefore, err := os.Stat(filepath.Join(repo, ".git", "index"))
	if err != nil {
		t.Fatal(err)
	}

	wss, err := ScanWorkspaces(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	names := []string{}
	byName := map[string]Workspace{}
	for _, ws := range wss {
		names = append(names, ws.Name)
		byName[ws.Name] = ws
	}
	if strings.Join(names, ",") != "other,private,ruvex-shop" {
		t.Fatalf("workspace'lar = %v (symlink va yashirin papka kirmasligi kerak)", names)
	}
	shop := byName["ruvex-shop"]
	if len(shop.Repos) != 1 || shop.Repos[0].Path != "shop-api" || shop.Repos[0].Branch != "main" || shop.Repos[0].Dirty != 1 || shop.Repos[0].HasUpstream {
		t.Errorf("repo = %+v", shop.Repos)
	}
	if !slices.Equal(shop.DeployTags, []string{"api:v1.2.0"}) {
		t.Errorf("deploy teglari = %v", shop.DeployTags)
	}
	if !byName["private"].IsLocalOnly || byName["other"].IsLocalOnly {
		t.Errorf("local belgisi: private=%v other=%v", byName["private"].IsLocalOnly, byName["other"].IsLocalOnly)
	}
	indexAfter, err := os.Stat(filepath.Join(repo, ".git", "index"))
	if err != nil {
		t.Fatal(err)
	}
	if !indexAfter.ModTime().Equal(indexBefore.ModTime()) {
		t.Error("git status indeksni yozmasligi kerak (faqat o'qish)")
	}
}

func TestScanWorkspacesMissingRoot(t *testing.T) {
	if _, err := ScanWorkspaces(context.Background(), filepath.Join(t.TempDir(), "yo'q")); err == nil {
		t.Error("xato kutilgan edi")
	}
}

func TestGroup(t *testing.T) {
	root := "/srv/p"
	wss := []Workspace{{Name: "ruvex-shop"}, {Name: "adbus-share"}, {Name: "ruvex-gapda"}, {Name: "secret", IsLocalOnly: true}, {Name: "empty"}}
	containers := []Container{
		{Name: "shop-web-1", ComposeProject: "shop", ComposeWorkingDir: "/srv/p/ruvex-shop/api", State: "running", MemLimit: 1, MemUsage: 100, CPUPercent: 1.5, Networks: []string{"long-preview", "shop_default"}},
		{Name: "shop-postgres-1", Image: "pgvector/pgvector:pg16", ComposeProject: "shop", ComposeWorkingDir: "/srv/p/ruvex-shop/api", State: "running", MemLimit: 1, MemUsage: 50, Networks: []string{"long-preview"}},
		// eskirgan working_dir: papka qayta nomlangan (gapda → ruvex-gapda)
		{Name: "gapda-api-1", ComposeProject: "gapda", ComposeWorkingDir: "/srv/p/gapda/preview", State: "exited"},
		// working_dir yo'q, compose nomi papka suffiksi bilan mos (adbus-share ← share)
		{Name: "share-api-1", ComposeProject: "share", State: "running", PublishedPorts: []string{"0.0.0.0:5432/tcp"}},
		{Name: "secret-web-1", ComposeProject: "secret", ComposeWorkingDir: "/srv/p/secret", State: "running", MemLimit: 1, Networks: []string{"long-preview"}},
		{Name: "long-router", State: "running"},
		{Name: "x-1", ComposeProject: "unknown", ComposeWorkingDir: "/elsewhere", State: "running"},
	}
	sites := []Site{
		{Host: "shop.long.uz", Kind: SitePreview, Upstreams: []string{"shop-web-1"}},
		{Host: "gapda.long.uz", Kind: SitePreview, Upstreams: []string{"gapda-api-1"}},
		{Host: "secret.long.uz", Kind: SitePreview, Upstreams: []string{"secret-web-1"}},
		{Host: "ghost.long.uz", Kind: SitePreview, Upstreams: []string{"ghost-1"}},
	}
	res := Group(root, wss, containers, sites)

	by := map[string]Project{}
	for _, p := range res.Projects {
		by[p.Workspace.Name] = p
	}
	shop := by["ruvex-shop"]
	if shop.Running != 2 || shop.Total != 2 || shop.MemUsage != 150 || shop.CPUPercent != 1.5 || len(shop.Sites) != 1 {
		t.Errorf("shop = %+v", shop)
	}
	assertWarning(t, shop.Warnings, "shop-postgres-1: ma'lumotlar servisi router tarmog'ida")

	gapda := by["ruvex-gapda"]
	if gapda.Total != 1 {
		t.Errorf("eskirgan yo'l bo'yicha topilmadi: %+v", gapda)
	}
	assertWarning(t, gapda.Warnings, "gapda.long.uz: gapda-api-1 ishlamayapti (502)")

	share := by["adbus-share"]
	if share.Total != 1 {
		t.Errorf("suffiks bo'yicha topilmadi: %+v", share)
	}
	assertWarning(t, share.Warnings, "share-api-1: hostga port ochiq (0.0.0.0:5432/tcp)")
	assertWarning(t, share.Warnings, "share-api-1: mem_limit yo'q")

	secret := by["secret"]
	assertWarning(t, secret.Warnings, "secret.long.uz: proyekt 'preview: local', lekin long.uz da ochiq")
	assertWarning(t, secret.Warnings, "secret-web-1: proyekt 'preview: local', lekin long-preview tarmog'ida")

	if by["empty"].Total != 0 || len(by["empty"].Warnings) != 0 {
		t.Errorf("bo'sh workspace = %+v", by["empty"])
	}
	if len(res.OrphanSites) != 1 || res.OrphanSites[0].Host != "ghost.long.uz" {
		t.Errorf("orphan saytlar = %+v", res.OrphanSites)
	}
	if len(res.OrphanCompose) != 2 {
		t.Errorf("orphan compose = %+v", res.OrphanCompose)
	}
}

func TestMatchByNameAmbiguous(t *testing.T) {
	by := map[string]*Project{"a-api": {}, "b-api": {}}
	if matchByName("api", by) != nil {
		t.Error("bir nechta mos kelsa — hech biri tanlanmasligi kerak")
	}
}

func assertWarning(t *testing.T, ws []string, want string) {
	t.Helper()
	if !slices.Contains(ws, want) {
		t.Errorf("ogohlantirish %q yo'q; bor: %q", want, ws)
	}
}
