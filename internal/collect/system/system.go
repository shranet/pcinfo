// Package system CPU, xotira, disk, temperatura va host ma'lumotini o'qiydi.
package system

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/load"
	"github.com/shirou/gopsutil/v4/mem"
	"github.com/shirou/gopsutil/v4/sensors"
)

// realFilesystems — ko'rsatiladigan disk turlari (tmpfs, overlay, squashfs emas).
var realFilesystems = []string{"ext4", "ext3", "xfs", "btrfs", "zfs", "vfat", "f2fs", "ntfs", "exfat"}

const maxTemps = 8

// CPU — yadrolar bo'yicha yuklanish.
type CPU struct {
	Total   float64
	PerCore []float64
}

// Memory — RAM va swap.
type Memory struct {
	Total, Used, Available, Cached uint64
	SwapTotal, SwapUsed            uint64
}

// Disk — bitta mount nuqtasi.
type Disk struct {
	Mount, Device, FSType string
	Total, Used           uint64
	Percent               float64
}

// Temp — bitta sensor.
type Temp struct {
	Name    string
	Celsius float64
	High    float64
}

// Host — umumiy ma'lumot.
type Host struct {
	Hostname, Kernel, Platform string
	Uptime                     time.Duration
	Load1, Load5, Load15       float64
	Procs                      uint64
}

// Snapshot — bir o'lchov natijasi.
type Snapshot struct {
	CPU    CPU
	Memory Memory
	Disks  []Disk
	Temps  []Temp
	Host   Host
}

// Sampler CPU foizini ikki o'lchov farqidan hisoblaydi, shuning uchun
// oldingi o'lchovni saqlaydi. Bir vaqtda bitta goroutine'dan chaqiriladi.
type Sampler struct {
	prev []cpu.TimesStat
}

// NewSampler yangi sampler.
func NewSampler() *Sampler { return &Sampler{} }

// Collect hammasini o'qiydi; ayrim qism o'qilmasa qolganlari qaytariladi.
func (s *Sampler) Collect(ctx context.Context) (Snapshot, error) {
	var snap Snapshot
	var errs []error
	if c, err := s.cpu(ctx); err == nil {
		snap.CPU = c
	} else {
		errs = append(errs, fmt.Errorf("cpu: %w", err))
	}
	if m, err := memory(ctx); err == nil {
		snap.Memory = m
	} else {
		errs = append(errs, fmt.Errorf("xotira: %w", err))
	}
	snap.Disks = disks(ctx)
	snap.Temps = temps(ctx)
	snap.Host = hostInfo(ctx)
	return snap, errors.Join(errs...)
}

func (s *Sampler) cpu(ctx context.Context) (CPU, error) {
	cur, err := cpu.TimesWithContext(ctx, true)
	if err != nil {
		return CPU{}, err
	}
	prev := s.prev
	s.prev = cur
	if len(prev) != len(cur) {
		return CPU{PerCore: make([]float64, len(cur))}, nil
	}
	out := CPU{PerCore: make([]float64, len(cur))}
	var busySum, totalSum float64
	for i := range cur {
		busy, total := busyDelta(prev[i], cur[i])
		if total > 0 {
			out.PerCore[i] = busy / total * 100
		}
		busySum += busy
		totalSum += total
	}
	if totalSum > 0 {
		out.Total = busySum / totalSum * 100
	}
	return out, nil
}

func busyDelta(a, b cpu.TimesStat) (busy, total float64) {
	idle := func(t cpu.TimesStat) float64 { return t.Idle + t.Iowait }
	all := func(t cpu.TimesStat) float64 {
		return t.User + t.System + t.Idle + t.Nice + t.Iowait + t.Irq + t.Softirq + t.Steal
	}
	total = all(b) - all(a)
	busy = total - (idle(b) - idle(a))
	if total < 0 || busy < 0 {
		return 0, 0
	}
	return busy, total
}

func memory(ctx context.Context) (Memory, error) {
	vm, err := mem.VirtualMemoryWithContext(ctx)
	if err != nil {
		return Memory{}, err
	}
	m := Memory{Total: vm.Total, Used: vm.Used, Available: vm.Available, Cached: vm.Cached + vm.Buffers}
	if sw, err := mem.SwapMemoryWithContext(ctx); err == nil {
		m.SwapTotal, m.SwapUsed = sw.Total, sw.Used
	}
	return m, nil
}

func disks(ctx context.Context) []Disk {
	parts, err := disk.PartitionsWithContext(ctx, false)
	if err != nil {
		return nil
	}
	var out []Disk
	seen := map[string]bool{}
	for _, p := range parts {
		if !slices.Contains(realFilesystems, p.Fstype) || seen[p.Device] {
			continue
		}
		u, err := disk.UsageWithContext(ctx, p.Mountpoint)
		if err != nil || u.Total == 0 {
			continue
		}
		seen[p.Device] = true
		out = append(out, Disk{Mount: p.Mountpoint, Device: p.Device, FSType: p.Fstype, Total: u.Total, Used: u.Used, Percent: u.UsedPercent})
	}
	slices.SortFunc(out, func(a, b Disk) int { return strings.Compare(a.Mount, b.Mount) })
	return out
}

// temps har sensor guruhidan eng issiq qiymatni oladi (yadrolar soni ko'p bo'lsa ekran to'lmasin).
func temps(ctx context.Context) []Temp {
	list, _ := sensors.TemperaturesWithContext(ctx)
	best := map[string]Temp{}
	for _, t := range list {
		if t.Temperature <= 0 {
			continue
		}
		group, _, _ := strings.Cut(t.SensorKey, "_")
		if cur, ok := best[group]; !ok || t.Temperature > cur.Celsius {
			best[group] = Temp{Name: group, Celsius: t.Temperature, High: t.High}
		}
	}
	out := make([]Temp, 0, len(best))
	for _, t := range best {
		out = append(out, t)
	}
	slices.SortFunc(out, func(a, b Temp) int { return strings.Compare(a.Name, b.Name) })
	if len(out) > maxTemps {
		out = out[:maxTemps]
	}
	return out
}

func hostInfo(ctx context.Context) Host {
	var h Host
	if hi, err := host.InfoWithContext(ctx); err == nil {
		h.Hostname, h.Kernel, h.Procs = hi.Hostname, hi.KernelVersion, hi.Procs
		h.Platform = strings.TrimSpace(hi.Platform + " " + hi.PlatformVersion)
		h.Uptime = time.Duration(hi.Uptime) * time.Second
	}
	if l, err := load.AvgWithContext(ctx); err == nil {
		h.Load1, h.Load5, h.Load15 = l.Load1, l.Load5, l.Load15
	}
	return h
}
