package projects

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
)

const (
	claudeMDName     = "CLAUDE.md"
	deployFileName   = "deploy.json"
	localMarker      = "preview: local"
	maxClaudeMDBytes = 256 << 10
	maxDeployBytes   = 1 << 20
	maxGitOutput     = 1 << 20

	// gitBinary — PATH orqali emas, aniq yo'l: PATH almashtirilsa ham boshqa dastur ishlamaydi.
	gitBinary  = "/usr/bin/git"
	gitTimeout = 5 * time.Second
)

// skippedDirs — repo qidirishda kirilmaydigan papkalar.
var skippedDirs = []string{"node_modules", "vendor", "dist", "build"}

// Repo — workspace ichidagi git repo holati.
type Repo struct {
	Path        string `json:"path"` // workspace'ga nisbatan; "." — workspace'ning o'zi
	Branch      string `json:"branch"`
	HasUpstream bool   `json:"hasUpstream"`
	Ahead       int    `json:"ahead"`
	Behind      int    `json:"behind"`
	Dirty       int    `json:"dirty"`
	Err         string `json:"err"`
}

// Workspace — projects/ ning birinchi darajali papkasi.
type Workspace struct {
	Name        string   `json:"name"`
	Path        string   `json:"path"`
	IsLocalOnly bool     `json:"isLocalOnly"`
	Repos       []Repo   `json:"repos"`
	DeployTags  []string `json:"deployTags"`
}

// ScanWorkspaces root ichidagi workspace'larni, ularning repolarini va
// deploy.json teglarini o'qiydi. Symlink'larga kirilmaydi.
func ScanWorkspaces(ctx context.Context, root string) ([]Workspace, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", root, err)
	}
	var out []Workspace
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		out = append(out, scanWorkspace(ctx, filepath.Join(root, e.Name())))
	}
	return out, nil
}

func scanWorkspace(ctx context.Context, dir string) Workspace {
	ws := Workspace{Name: filepath.Base(dir), Path: dir}
	candidates := []string{"."}
	if entries, err := os.ReadDir(dir); err == nil {
		for _, e := range entries {
			if e.IsDir() && !strings.HasPrefix(e.Name(), ".") && !slices.Contains(skippedDirs, e.Name()) {
				candidates = append(candidates, e.Name())
			}
		}
	}
	for _, rel := range candidates {
		full := filepath.Join(dir, rel)
		if hasLocalMarker(filepath.Join(full, claudeMDName)) {
			ws.IsLocalOnly = true
		}
		for _, p := range []string{full, filepath.Join(full, "docs")} {
			ws.DeployTags = append(ws.DeployTags, deployTags(filepath.Join(p, deployFileName))...)
		}
		if isGitRepo(full) {
			ws.Repos = append(ws.Repos, gitStatus(ctx, full, rel))
		}
	}
	slices.Sort(ws.DeployTags)
	ws.DeployTags = slices.Compact(ws.DeployTags)
	return ws
}

func isGitRepo(dir string) bool {
	_, err := os.Lstat(filepath.Join(dir, ".git"))
	return err == nil
}

// readLimited oddiy faylni (symlink emas) hajm chegarasi bilan o'qiydi.
func readLimited(name string, limit int64) ([]byte, error) {
	fi, err := os.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !fi.Mode().IsRegular() {
		return nil, fmt.Errorf("%s: oddiy fayl emas", name)
	}
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(io.LimitReader(f, limit))
}

func hasLocalMarker(name string) bool {
	data, err := readLimited(name, maxClaudeMDBytes)
	if err != nil {
		return false
	}
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		line := strings.Trim(strings.TrimSpace(sc.Text()), "-*` ")
		if line == localMarker {
			return true
		}
	}
	return false
}

// deployTags deploy.json dagi "pull" havolalaridan "backend:v0.3.0" ko'rinishidagi
// teglarni oladi. Fayl formati to'liq tekshirilmaydi: faqat pull satrlari kerak.
func deployTags(name string) []string {
	data, err := readLimited(name, maxDeployBytes)
	if err != nil {
		return nil
	}
	var doc any
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil
	}
	var tags []string
	walkPulls(doc, func(ref string) {
		if t := imageTag(ref); t != "" {
			tags = append(tags, t)
		}
	})
	return tags
}

func walkPulls(v any, fn func(string)) {
	switch x := v.(type) {
	case map[string]any:
		for k, val := range x {
			if s, ok := val.(string); ok && k == "pull" {
				fn(s)
			} else {
				walkPulls(val, fn)
			}
		}
	case []any:
		for _, val := range x {
			walkPulls(val, fn)
		}
	}
}

// imageTag "127.0.0.1:5000/lens/backend:v0.3.0@sha256:…" → "backend:v0.3.0".
func imageTag(ref string) string {
	ref, _, _ = strings.Cut(ref, "@")
	name := path.Base(ref)
	if !strings.Contains(name, ":") {
		return ""
	}
	return name
}

// gitStatus repo holatini faqat o'qib oladi. --no-optional-locks va
// GIT_OPTIONAL_LOCKS=0 — `git status` indeksni yangilab yozmasligi uchun;
// fsmonitor o'chiq — repo konfiguratsiyasidagi tashqi dastur ishga tushmasin.
func gitStatus(ctx context.Context, dir, rel string) Repo {
	repo := Repo{Path: rel}
	ctx, cancel := context.WithTimeout(ctx, gitTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, gitBinary,
		"-C", dir,
		"-c", "core.fsmonitor=false",
		"-c", "core.untrackedCache=false",
		"--no-optional-locks",
		"status", "--porcelain=v2", "--branch")
	cmd.Env = gitEnv()
	var out bytes.Buffer
	cmd.Stdout = &limitedWriter{w: &out, remaining: maxGitOutput}
	if err := cmd.Run(); err != nil {
		repo.Err = gitError(err)
		return repo
	}
	parsePorcelain(out.Bytes(), &repo)
	return repo
}

func gitEnv() []string {
	env := []string{
		"PATH=/usr/bin:/bin",
		"LC_ALL=C",
		"GIT_OPTIONAL_LOCKS=0",
		"GIT_TERMINAL_PROMPT=0",
	}
	if home, err := os.UserHomeDir(); err == nil {
		env = append(env, "HOME="+home)
	}
	return env
}

func gitError(err error) string {
	var ee *exec.ExitError
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return "git timeout"
	case errors.Is(err, fs.ErrNotExist):
		return "git topilmadi"
	case errors.As(err, &ee):
		return "git xatosi"
	}
	return "git ishlamadi"
}

func parsePorcelain(out []byte, repo *Repo) {
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		line := sc.Text()
		rest, isHeader := strings.CutPrefix(line, "# ")
		if !isHeader {
			if line != "" {
				repo.Dirty++
			}
			continue
		}
		key, val, _ := strings.Cut(rest, " ")
		switch key {
		case "branch.head":
			repo.Branch = val
		case "branch.upstream":
			repo.HasUpstream = true
		case "branch.ab":
			a, b, _ := strings.Cut(val, " ")
			repo.Ahead, _ = strconv.Atoi(strings.TrimPrefix(a, "+"))
			repo.Behind, _ = strconv.Atoi(strings.TrimPrefix(b, "-"))
		}
	}
}

// limitedWriter chegaradan keyingi chiqishni jim tashlaydi.
type limitedWriter struct {
	w         io.Writer
	remaining int
}

func (l *limitedWriter) Write(p []byte) (int, error) {
	n := len(p)
	if l.remaining <= 0 {
		return n, nil
	}
	if len(p) > l.remaining {
		p = p[:l.remaining]
	}
	l.remaining -= len(p)
	_, err := l.w.Write(p)
	return n, err
}
