// Package projects /srv/data/claude/projects dagi workspace'larni ularning
// compose proyektlari, konteynerlari va router yo'nalishlari bilan bog'laydi (D-004).
package projects

import (
	"cmp"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
)

const (
	DefaultRoot         = "/srv/data/claude/projects"
	DefaultPreviewSites = "/srv/data/claude/router/sites"
	DefaultLocalSites   = "/srv/data/claude/router-local/sites"

	previewNetwork = "long-preview"
	localNetwork   = "long-local"

	// workspacePrefix — ko'p workspace nomi shu bilan boshlanadi (ruvex-lens → lens).
	workspacePrefix = "ruvex-"

	stateRunning = "running"
)

// dataStoreImages — tashqi tarmoqqa ulanmasligi kerak bo'lgan servislar (global qoida).
var dataStoreImages = []string{
	"postgres", "pgvector", "redis", "valkey", "mysql", "mariadb", "mongo",
	"minio", "mosquitto", "rabbitmq", "clickhouse", "elasticsearch", "memcached", "nats",
}

// Container — guruhlash uchun kerakli konteyner ma'lumoti.
type Container struct {
	Name              string   `json:"name"`
	Image             string   `json:"image"`
	State             string   `json:"state"`
	Status            string   `json:"status"`
	ComposeProject    string   `json:"composeProject"`
	ComposeService    string   `json:"composeService"`
	ComposeWorkingDir string   `json:"composeWorkingDir"`
	Networks          []string `json:"networks"`
	PublishedPorts    []string `json:"publishedPorts"`
	MemLimit          int64    `json:"memLimit"`
	MemUsage          uint64   `json:"memUsage"`
	CPUPercent        float64  `json:"cpuPercent"`
}

// IsRunning — konteyner ishlayaptimi.
func (c Container) IsRunning() bool { return c.State == stateRunning }

// Compose — bitta compose proyekt.
type Compose struct {
	Name       string      `json:"name"`
	Containers []Container `json:"containers"`
}

// Project — workspace va unga tegishli hamma narsa.
type Project struct {
	Workspace  Workspace `json:"workspace"`
	Compose    []Compose `json:"compose"`
	Sites      []Site    `json:"sites"`
	Warnings   []string  `json:"warnings"`
	Running    int       `json:"running"`
	Total      int       `json:"total"`
	MemUsage   uint64    `json:"memUsage"`
	CPUPercent float64   `json:"cpuPercent"`
}

// Result — guruhlash natijasi; hech qaysi workspace'ga bog'lanmaganlari alohida.
type Result struct {
	Projects      []Project `json:"projects"`
	OrphanCompose []Compose `json:"orphanCompose"`
	OrphanSites   []Site    `json:"orphanSites"`
}

// Group workspace, konteyner va saytlarni bog'laydi.
func Group(root string, workspaces []Workspace, containers []Container, sites []Site) Result {
	byName := make(map[string]*Project, len(workspaces))
	projects := make([]Project, len(workspaces))
	for i, ws := range workspaces {
		projects[i].Workspace = ws
	}
	for i := range projects {
		byName[projects[i].Workspace.Name] = &projects[i]
	}

	containerOwner := map[string]*Project{}
	orphans := map[string][]Container{}
	for _, c := range containers {
		p := findWorkspace(root, c, byName)
		if p == nil {
			key := cmp.Or(c.ComposeProject, "compose'siz")
			orphans[key] = append(orphans[key], c)
			continue
		}
		containerOwner[c.Name] = p
		p.addContainer(c)
	}

	var res Result
	running := map[string]bool{}
	for _, c := range containers {
		running[c.Name] = c.IsRunning()
	}
	for _, s := range sites {
		p := siteOwner(s, containerOwner)
		if p == nil {
			res.OrphanSites = append(res.OrphanSites, s)
			continue
		}
		p.Sites = append(p.Sites, s)
		for _, up := range s.Upstreams {
			isRunning, exists := running[up]
			switch {
			case !exists:
				p.Warnings = append(p.Warnings, fmt.Sprintf("%s: %s konteyneri yo'q", s.Host, up))
			case !isRunning:
				p.Warnings = append(p.Warnings, fmt.Sprintf("%s: %s ishlamayapti (502)", s.Host, up))
			}
		}
	}

	for i := range projects {
		projects[i].finish()
	}
	res.Projects = projects
	for name, cs := range orphans {
		res.OrphanCompose = append(res.OrphanCompose, Compose{Name: name, Containers: cs})
	}
	slices.SortFunc(res.OrphanCompose, func(a, b Compose) int { return strings.Compare(a.Name, b.Name) })
	return res
}

// findWorkspace konteyner egasini topadi: avval compose working_dir bo'yicha,
// u eskirgan bo'lsa (papka qayta nomlangan) — nomlar bo'yicha.
func findWorkspace(root string, c Container, byName map[string]*Project) *Project {
	var dirName string
	if rel, err := filepath.Rel(root, c.ComposeWorkingDir); err == nil && c.ComposeWorkingDir != "" && !strings.HasPrefix(rel, "..") {
		dirName, _, _ = strings.Cut(filepath.ToSlash(rel), "/")
		if p, ok := byName[dirName]; ok {
			return p
		}
	}
	for _, n := range []string{dirName, c.ComposeProject} {
		if p := matchByName(n, byName); p != nil {
			return p
		}
	}
	return nil
}

// matchByName: aniq nom, "ruvex-<n>", keyin "*-<n>" (faqat bitta mos kelsa).
func matchByName(n string, byName map[string]*Project) *Project {
	if n == "" || n == "." {
		return nil
	}
	for _, cand := range []string{n, workspacePrefix + n} {
		if p, ok := byName[cand]; ok {
			return p
		}
	}
	var found *Project
	for name, p := range byName {
		if strings.HasSuffix(name, "-"+n) {
			if found != nil {
				return nil
			}
			found = p
		}
	}
	return found
}

func siteOwner(s Site, owner map[string]*Project) *Project {
	for _, up := range s.Upstreams {
		if p, ok := owner[up]; ok {
			return p
		}
	}
	return nil
}

func (p *Project) addContainer(c Container) {
	i := slices.IndexFunc(p.Compose, func(cp Compose) bool { return cp.Name == c.ComposeProject })
	if i < 0 {
		p.Compose = append(p.Compose, Compose{Name: c.ComposeProject})
		i = len(p.Compose) - 1
	}
	p.Compose[i].Containers = append(p.Compose[i].Containers, c)
	p.Total++
	if c.IsRunning() {
		p.Running++
		p.MemUsage += c.MemUsage
		p.CPUPercent += c.CPUPercent
	}
	p.Warnings = append(p.Warnings, containerWarnings(c, p.Workspace.IsLocalOnly)...)
}

func containerWarnings(c Container, isLocalOnly bool) []string {
	var w []string
	onPreview := slices.Contains(c.Networks, previewNetwork)
	onRouter := onPreview || slices.Contains(c.Networks, localNetwork)
	if onRouter && isDataStore(c.Image) {
		w = append(w, fmt.Sprintf("%s: ma'lumotlar servisi router tarmog'ida", c.Name))
	}
	if len(c.PublishedPorts) > 0 {
		w = append(w, fmt.Sprintf("%s: hostga port ochiq (%s)", c.Name, strings.Join(c.PublishedPorts, ", ")))
	}
	if c.IsRunning() && c.MemLimit == 0 {
		w = append(w, fmt.Sprintf("%s: mem_limit yo'q", c.Name))
	}
	if isLocalOnly && onPreview {
		w = append(w, fmt.Sprintf("%s: proyekt 'preview: local', lekin long-preview tarmog'ida", c.Name))
	}
	return w
}

func isDataStore(image string) bool {
	base := image
	if i := strings.LastIndex(base, "/"); i >= 0 {
		base = base[i+1:]
	}
	base, _, _ = strings.Cut(base, ":")
	return slices.ContainsFunc(dataStoreImages, func(s string) bool { return strings.Contains(base, s) })
}

func (p *Project) finish() {
	if p.Workspace.IsLocalOnly {
		for _, s := range p.Sites {
			if s.Kind == SitePreview {
				p.Warnings = append(p.Warnings, fmt.Sprintf("%s: proyekt 'preview: local', lekin long.uz da ochiq", s.Host))
			}
		}
	}
	slices.SortFunc(p.Compose, func(a, b Compose) int { return strings.Compare(a.Name, b.Name) })
	slices.SortFunc(p.Sites, func(a, b Site) int { return strings.Compare(a.Host, b.Host) })
	slices.Sort(p.Warnings)
	p.Warnings = slices.Compact(p.Warnings)
}
