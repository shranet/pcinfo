// Package dockerapi Docker Engine API'dan konteynerlar va ularning resurs
// sarfini o'qiydi. Unix socket orqali faqat GET so'rovlar yuboriladi:
// mijozda boshqa HTTP metod yo'q, shuning uchun u hech narsani o'zgartira olmaydi.
package dockerapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"
)

const (
	systemSocket  = "/var/run/docker.sock"
	userSocketFmt = "/run/user/%d/docker.sock"

	// apiVersion — kontrakt barqaror bo'lishi uchun aniq versiya (server min 1.40).
	apiVersion = "/v1.43"
	// Hostname so'rov URL'i uchun kerak, socket orqali hech qayerga ketmaydi.
	baseURL = "http://docker" + apiVersion

	requestTimeout   = 5 * time.Second
	statsTimeout     = 8 * time.Second
	maxResponseBytes = 8 << 20
	// stream=false har konteynerda ~1 s kutadi (CPU farqi uchun), shuning uchun keng parallel.
	statsConcurrency = 32

	labelComposeProject    = "com.docker.compose.project"
	labelComposeService    = "com.docker.compose.service"
	labelComposeWorkingDir = "com.docker.compose.project.working_dir"

	stateRunning = "running"
)

var containerIDPattern = regexp.MustCompile(`^[0-9a-f]{12,64}$`)

// Endpoint — bitta Docker daemon.
type Endpoint struct {
	Name   string `json:"name"`
	Socket string `json:"socket"`
}

// SystemEndpoint — tizim (root) Docker'i.
func SystemEndpoint() Endpoint {
	return Endpoint{Name: "tizim", Socket: systemSocket}
}

// UserEndpoint — foydalanuvchining rootless Docker'i.
func UserEndpoint(uid int, user string) Endpoint {
	return Endpoint{Name: user + " (rootless)", Socket: fmt.Sprintf(userSocketFmt, uid)}
}

// Exists — socket fayli bormi (yo'q bo'lsa endpoint umuman ko'rsatilmaydi).
func (e Endpoint) Exists() bool {
	_, err := os.Stat(e.Socket)
	return err == nil
}

// Container — ko'rsatiladigan konteyner ma'lumoti.
type Container struct {
	ID                string   `json:"id"`
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

// Summary — bitta endpoint bo'yicha natija.
type Summary struct {
	Endpoint   Endpoint    `json:"endpoint"`
	Reachable  bool        `json:"reachable"`
	Error      string      `json:"error"`
	Version    string      `json:"version"`
	Images     int         `json:"images"`
	Containers []Container `json:"containers"`
}

// Client — bitta socket uchun faqat-o'qish mijozi.
type Client struct {
	http *http.Client
}

// NewClient socket bo'yicha mijoz yaratadi.
func NewClient(socket string) *Client {
	var d net.Dialer
	return &Client{http: &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return d.DialContext(ctx, "unix", socket)
			},
		},
	}}
}

// get — mijozdagi yagona so'rov turi.
func (c *Client) get(ctx context.Context, path string, timeout time.Duration, out any) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+path, nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("docker %s: HTTP %d", path, resp.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, maxResponseBytes)).Decode(out)
}

// Collect endpoint'dagi konteynerlarni, ularning limit va sarfini yig'adi.
func Collect(ctx context.Context, ep Endpoint) Summary {
	s := Summary{Endpoint: ep}
	c := NewClient(ep.Socket)

	var info struct {
		ServerVersion string `json:"ServerVersion"`
		Images        int    `json:"Images"`
	}
	if err := c.get(ctx, "/info", requestTimeout, &info); err != nil {
		s.Error = shortError(err)
		return s
	}
	s.Reachable, s.Version, s.Images = true, info.ServerVersion, info.Images

	containers, err := c.containers(ctx)
	if err != nil {
		s.Error = shortError(err)
		return s
	}
	c.fillDetails(ctx, containers)
	s.Containers = containers
	return s
}

type listItem struct {
	ID     string            `json:"Id"`
	Names  []string          `json:"Names"`
	Image  string            `json:"Image"`
	State  string            `json:"State"`
	Status string            `json:"Status"`
	Labels map[string]string `json:"Labels"`
	Ports  []struct {
		IP         string `json:"IP"`
		PublicPort int    `json:"PublicPort"`
		Type       string `json:"Type"`
	} `json:"Ports"`
	NetworkSettings struct {
		Networks map[string]json.RawMessage `json:"Networks"`
	} `json:"NetworkSettings"`
}

func (c *Client) containers(ctx context.Context) ([]Container, error) {
	var items []listItem
	if err := c.get(ctx, "/containers/json?all=1", requestTimeout, &items); err != nil {
		return nil, err
	}
	out := make([]Container, 0, len(items))
	for _, it := range items {
		if !containerIDPattern.MatchString(it.ID) {
			continue
		}
		ct := Container{
			ID:                it.ID,
			Image:             it.Image,
			State:             it.State,
			Status:            it.Status,
			ComposeProject:    it.Labels[labelComposeProject],
			ComposeService:    it.Labels[labelComposeService],
			ComposeWorkingDir: it.Labels[labelComposeWorkingDir],
		}
		if len(it.Names) > 0 {
			ct.Name = strings.TrimPrefix(it.Names[0], "/")
		}
		for name := range it.NetworkSettings.Networks {
			ct.Networks = append(ct.Networks, name)
		}
		slices.Sort(ct.Networks)
		for _, p := range it.Ports {
			if p.PublicPort != 0 {
				ct.PublishedPorts = append(ct.PublishedPorts, fmt.Sprintf("%s:%d/%s", p.IP, p.PublicPort, p.Type))
			}
		}
		slices.Sort(ct.PublishedPorts)
		ct.PublishedPorts = slices.Compact(ct.PublishedPorts)
		out = append(out, ct)
	}
	slices.SortFunc(out, func(a, b Container) int { return strings.Compare(a.Name, b.Name) })
	return out, nil
}

// fillDetails har konteyner uchun limit (inspect) va ishlayotganlari uchun
// sarf (stats) ni parallel o'qiydi. Bitta konteyner xatosi qolganlarini to'xtatmaydi.
func (c *Client) fillDetails(ctx context.Context, containers []Container) {
	sem := make(chan struct{}, statsConcurrency)
	var wg sync.WaitGroup
	for i := range containers {
		wg.Go(func() {
			sem <- struct{}{}
			defer func() { <-sem }()
			ct := &containers[i]
			ct.MemLimit = c.memLimit(ctx, ct.ID)
			if ct.IsRunning() {
				ct.MemUsage, ct.CPUPercent = c.usage(ctx, ct.ID)
			}
		})
	}
	wg.Wait()
}

func (c *Client) memLimit(ctx context.Context, id string) int64 {
	var inspect struct {
		HostConfig struct {
			Memory int64 `json:"Memory"`
		} `json:"HostConfig"`
	}
	if err := c.get(ctx, "/containers/"+id+"/json", requestTimeout, &inspect); err != nil {
		return 0
	}
	return inspect.HostConfig.Memory
}

// Stats — /containers/{id}/stats?stream=false javobining kerakli qismi.
type Stats struct {
	CPUStats    cpuStats `json:"cpu_stats"`
	PreCPUStats cpuStats `json:"precpu_stats"`
	MemoryStats struct {
		Usage uint64            `json:"usage"`
		Stats map[string]uint64 `json:"stats"`
	} `json:"memory_stats"`
}

type cpuStats struct {
	CPUUsage struct {
		TotalUsage uint64 `json:"total_usage"`
	} `json:"cpu_usage"`
	SystemUsage uint64 `json:"system_cpu_usage"`
	OnlineCPUs  uint64 `json:"online_cpus"`
}

func (c *Client) usage(ctx context.Context, id string) (uint64, float64) {
	var st Stats
	if err := c.get(ctx, "/containers/"+id+"/stats?stream=false", statsTimeout, &st); err != nil {
		return 0, 0
	}
	return st.MemoryUsed(), st.CPUPercent()
}

// MemoryUsed — `docker stats` kabi: sahifa keshi (inactive_file) hisobga olinmaydi.
func (s Stats) MemoryUsed() uint64 {
	cache := s.MemoryStats.Stats["inactive_file"]
	if cache > s.MemoryStats.Usage {
		return 0
	}
	return s.MemoryStats.Usage - cache
}

// CPUPercent — `docker stats` formulasi: 100% = bitta yadro.
func (s Stats) CPUPercent() float64 {
	cpuDelta := float64(s.CPUStats.CPUUsage.TotalUsage) - float64(s.PreCPUStats.CPUUsage.TotalUsage)
	sysDelta := float64(s.CPUStats.SystemUsage) - float64(s.PreCPUStats.SystemUsage)
	if cpuDelta <= 0 || sysDelta <= 0 {
		return 0
	}
	return cpuDelta / sysDelta * float64(s.CPUStats.OnlineCPUs) * 100
}

func shortError(err error) string {
	switch {
	case errors.Is(err, os.ErrPermission):
		return "ruxsat yo'q"
	case errors.Is(err, os.ErrNotExist):
		return "socket yo'q"
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	}
	var ne *net.OpError
	if errors.As(err, &ne) {
		if errors.Is(ne.Err, os.ErrPermission) || strings.Contains(ne.Err.Error(), "permission denied") {
			return "ruxsat yo'q"
		}
		return "ulanib bo'lmadi"
	}
	return "xato"
}
