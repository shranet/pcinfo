// Package snapshot kollektorlarni yig'adi va claude tomoni bilan JSON
// kontraktini belgilaydi (D-002). Kontrakt turlarida map yo'q: safetext.CleanAll
// barcha satrlarni tozalay olishi kerak.
package snapshot

import (
	"context"
	"os"
	"os/user"
	"strconv"
	"time"

	"github.com/shranet/pcinfo/internal/collect/dockerapi"
	"github.com/shranet/pcinfo/internal/collect/projects"
)

// Schema — `collect --json` chiqishi versiyasi. Kontrakt o'zgarsa oshiriladi:
// eski o'rnatilgan binary bilan yangi TUI bir-birini noto'g'ri o'qimasin.
const Schema = 1

// ClaudeSide — claude huquqi bilan yig'iladigan ma'lumot.
type ClaudeSide struct {
	Schema      int                 `json:"schema"`
	User        string              `json:"user"`
	CollectedAt time.Time           `json:"collectedAt"`
	Docker      []dockerapi.Summary `json:"docker"`
	Projects    projects.Result     `json:"projects"`
	Errors      []string            `json:"errors"`
}

// CurrentUser — joriy foydalanuvchi uid va nomi.
func CurrentUser() (int, string) {
	uid := os.Getuid()
	name := strconv.Itoa(uid)
	if u, err := user.LookupId(name); err == nil {
		name = u.Username
	}
	return uid, name
}

// CollectClaudeSide joriy foydalanuvchining rootless Docker'i va proyektlarini yig'adi.
func CollectClaudeSide(ctx context.Context) ClaudeSide {
	uid, name := CurrentUser()
	out := ClaudeSide{Schema: Schema, User: name, CollectedAt: time.Now().UTC()}

	var containers []projects.Container
	if ep := dockerapi.UserEndpoint(uid, name); ep.Exists() {
		s := dockerapi.Collect(ctx, ep)
		out.Docker = append(out.Docker, s)
		containers = toProjectContainers(s.Containers)
	}
	res, errs := projects.Collect(ctx, projects.DefaultPaths(), containers)
	out.Projects, out.Errors = res, errs
	return out
}

// CollectLocalDocker TUI foydalanuvchisiga ochiq Docker'larni yig'adi:
// tizim Docker'i va (bo'lsa) o'zining rootless Docker'i.
func CollectLocalDocker(ctx context.Context) []dockerapi.Summary {
	uid, name := CurrentUser()
	var out []dockerapi.Summary
	for _, ep := range []dockerapi.Endpoint{dockerapi.SystemEndpoint(), dockerapi.UserEndpoint(uid, name)} {
		if ep.Exists() {
			out = append(out, dockerapi.Collect(ctx, ep))
		}
	}
	return out
}

func toProjectContainers(cs []dockerapi.Container) []projects.Container {
	out := make([]projects.Container, 0, len(cs))
	for _, c := range cs {
		out = append(out, projects.Container{
			Name:              c.Name,
			Image:             c.Image,
			State:             c.State,
			Status:            c.Status,
			ComposeProject:    c.ComposeProject,
			ComposeService:    c.ComposeService,
			ComposeWorkingDir: c.ComposeWorkingDir,
			Networks:          c.Networks,
			PublishedPorts:    c.PublishedPorts,
			MemLimit:          c.MemLimit,
			MemUsage:          c.MemUsage,
			CPUPercent:        c.CPUPercent,
		})
	}
	return out
}
