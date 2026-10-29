// Package process jarayonlar ro'yxatini (htop'dagi kabi) o'qiydi.
package process

import (
	"context"
	"os/user"
	"strconv"
	"time"

	"github.com/shirou/gopsutil/v4/mem"
	ps "github.com/shirou/gopsutil/v4/process"
)

// Process — bitta jarayon.
type Process struct {
	PID        int32
	User       string
	Name       string
	Cmdline    string
	State      string
	Threads    int32
	RSS        uint64
	MemPercent float64
	CPUPercent float64
}

// Sampler CPU foizini ikki o'lchov orasidagi CPU vaqti farqidan hisoblaydi.
// Bir vaqtda bitta goroutine'dan chaqiriladi.
type Sampler struct {
	prevCPU  map[int32]float64
	prevAt   time.Time
	userName map[uint32]string
}

// NewSampler yangi sampler.
func NewSampler() *Sampler {
	return &Sampler{prevCPU: map[int32]float64{}, userName: map[uint32]string{}}
}

// Sample hamma jarayonlarni o'qiydi. O'qishga ulgurmay tugagan jarayonlar tashlanadi.
func (s *Sampler) Sample(ctx context.Context) ([]Process, error) {
	procs, err := ps.ProcessesWithContext(ctx)
	if err != nil {
		return nil, err
	}
	var memTotal uint64
	if vm, err := mem.VirtualMemoryWithContext(ctx); err == nil {
		memTotal = vm.Total
	}
	now := time.Now()
	elapsed := now.Sub(s.prevAt).Seconds()
	cpuNow := make(map[int32]float64, len(procs))
	out := make([]Process, 0, len(procs))
	for _, p := range procs {
		times, err := p.TimesWithContext(ctx)
		if err != nil {
			continue
		}
		pr := Process{PID: p.Pid}
		cpuSecs := times.User + times.System
		cpuNow[p.Pid] = cpuSecs
		if prev, ok := s.prevCPU[p.Pid]; ok && elapsed > 0 && cpuSecs >= prev {
			pr.CPUPercent = (cpuSecs - prev) / elapsed * 100
		}
		pr.Name, _ = p.NameWithContext(ctx)
		pr.Cmdline, _ = p.CmdlineWithContext(ctx)
		if st, err := p.StatusWithContext(ctx); err == nil && len(st) > 0 {
			pr.State = st[0]
		}
		pr.Threads, _ = p.NumThreadsWithContext(ctx)
		if mi, err := p.MemoryInfoWithContext(ctx); err == nil {
			pr.RSS = mi.RSS
			if memTotal > 0 {
				pr.MemPercent = float64(mi.RSS) / float64(memTotal) * 100
			}
		}
		if uids, err := p.UidsWithContext(ctx); err == nil && len(uids) > 0 {
			pr.User = s.lookupUser(uids[0])
		}
		out = append(out, pr)
	}
	s.prevCPU, s.prevAt = cpuNow, now
	return out, nil
}

func (s *Sampler) lookupUser(uid uint32) string {
	if name, ok := s.userName[uid]; ok {
		return name
	}
	name := strconv.FormatUint(uint64(uid), 10)
	if u, err := user.LookupId(name); err == nil {
		name = u.Username
	}
	s.userName[uid] = name
	return name
}
