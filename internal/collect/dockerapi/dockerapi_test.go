package dockerapi

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

const (
	runningID = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	exitedID  = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
)

// fakeDocker unix socket ustida Docker API'ning kerakli qismini beradi va
// kelgan har bir so'rov metodini yozib boradi.
func fakeDocker(t *testing.T) (string, *[]string) {
	t.Helper()
	sock := filepath.Join(t.TempDir(), "docker.sock")
	l, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var methods []string
	mux := http.NewServeMux()
	mux.HandleFunc("/v1.43/info", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"ServerVersion":"29.9.2","Images":12}`)
	})
	mux.HandleFunc("/v1.43/containers/json", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `[
		 {"Id":"`+runningID+`","Names":["/lens-web-1"],"Image":"lens-web","State":"running","Status":"Up 2 hours",
		  "Labels":{"com.docker.compose.project":"lens","com.docker.compose.service":"web","com.docker.compose.project.working_dir":"/srv/x/ruvex-lens/lens-backend"},
		  "Ports":[{"IP":"0.0.0.0","PrivatePort":80,"PublicPort":8080,"Type":"tcp"},{"IP":"::","PrivatePort":80,"PublicPort":8080,"Type":"tcp"},{"PrivatePort":81,"Type":"tcp"}],
		  "NetworkSettings":{"Networks":{"long-preview":{},"lens_default":{}}}},
		 {"Id":"`+exitedID+`","Names":["/lens-migrate-1"],"Image":"lens","State":"exited","Status":"Exited (0)","Labels":{}},
		 {"Id":"../../etc","Names":["/evil"],"State":"running"}
		]`)
	})
	mux.HandleFunc("/v1.43/containers/{id}/json", func(w http.ResponseWriter, r *http.Request) {
		mem := 0
		if r.PathValue("id") == runningID {
			mem = 536870912
		}
		json.NewEncoder(w).Encode(map[string]any{"HostConfig": map[string]any{"Memory": mem}})
	})
	mux.HandleFunc("/v1.43/containers/{id}/stats", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"cpu_stats":{"cpu_usage":{"total_usage":3000},"system_cpu_usage":20000,"online_cpus":4},
		 "precpu_stats":{"cpu_usage":{"total_usage":1000},"system_cpu_usage":10000},
		 "memory_stats":{"usage":1000,"stats":{"inactive_file":400}}}`)
	})
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		methods = append(methods, r.Method+" "+r.URL.Path)
		mu.Unlock()
		mux.ServeHTTP(w, r)
	})}
	go srv.Serve(l)
	t.Cleanup(func() { srv.Close() })
	return sock, &methods
}

func TestCollect(t *testing.T) {
	sock, methods := fakeDocker(t)
	s := Collect(context.Background(), Endpoint{Name: "test", Socket: sock})
	if !s.Reachable || s.Error != "" || s.Version != "29.9.2" || s.Images != 12 {
		t.Fatalf("summary = %+v", s)
	}
	if len(s.Containers) != 2 {
		t.Fatalf("noto'g'ri ID li konteyner tashlanishi kerak: %d", len(s.Containers))
	}
	web := s.Containers[1]
	if web.Name != "lens-web-1" || web.ComposeProject != "lens" || web.ComposeWorkingDir != "/srv/x/ruvex-lens/lens-backend" {
		t.Errorf("web = %+v", web)
	}
	if strings.Join(web.Networks, ",") != "lens_default,long-preview" {
		t.Errorf("networks = %v", web.Networks)
	}
	if strings.Join(web.PublishedPorts, ",") != "0.0.0.0:8080/tcp,:::8080/tcp" {
		t.Errorf("ports = %v", web.PublishedPorts)
	}
	if web.MemLimit != 536870912 || web.MemUsage != 600 || web.CPUPercent != 80 {
		t.Errorf("sarf = limit %d usage %d cpu %v", web.MemLimit, web.MemUsage, web.CPUPercent)
	}
	if migrate := s.Containers[0]; migrate.MemUsage != 0 {
		t.Errorf("to'xtagan konteynerdan stats so'ralmasligi kerak: %+v", migrate)
	}
	for _, m := range *methods {
		if !strings.HasPrefix(m, "GET ") {
			t.Errorf("faqat GET bo'lishi kerak, keldi: %s", m)
		}
	}
}

func TestCollectUnreachable(t *testing.T) {
	s := Collect(context.Background(), Endpoint{Socket: filepath.Join(t.TempDir(), "none.sock")})
	if s.Reachable || s.Error == "" {
		t.Errorf("summary = %+v", s)
	}
}

func TestStats(t *testing.T) {
	var s Stats
	if s.CPUPercent() != 0 || s.MemoryUsed() != 0 {
		t.Error("bo'sh stats nol bo'lishi kerak")
	}
	s.MemoryStats.Usage = 10
	s.MemoryStats.Stats = map[string]uint64{"inactive_file": 20}
	if s.MemoryUsed() != 0 {
		t.Error("kesh sarfdan katta bo'lsa 0")
	}
}
