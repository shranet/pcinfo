package system

import (
	"context"
	"testing"

	"github.com/shirou/gopsutil/v4/cpu"
)

func TestBusyDelta(t *testing.T) {
	a := cpu.TimesStat{User: 10, System: 5, Idle: 80, Iowait: 5}
	b := cpu.TimesStat{User: 20, System: 10, Idle: 120, Iowait: 10}
	busy, total := busyDelta(a, b)
	if total != 60 || busy != 15 {
		t.Errorf("busy=%v total=%v", busy, total)
	}
	if busy, total := busyDelta(b, a); busy != 0 || total != 0 {
		t.Errorf("hisoblagich orqaga ketsa 0: %v %v", busy, total)
	}
}

func TestSamplerSmoke(t *testing.T) {
	s := NewSampler()
	first, err := s.Collect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if first.CPU.Total != 0 {
		t.Errorf("birinchi o'lchovda foiz 0 bo'lishi kerak: %v", first.CPU.Total)
	}
	second, err := s.Collect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(second.CPU.PerCore) == 0 || second.Memory.Total == 0 {
		t.Errorf("snapshot = %+v", second)
	}
}
