package process

import (
	"context"
	"os"
	"testing"
)

func TestSampleFindsSelf(t *testing.T) {
	s := NewSampler()
	if _, err := s.Sample(context.Background()); err != nil {
		t.Fatal(err)
	}
	procs, err := s.Sample(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	self := int32(os.Getpid())
	for _, p := range procs {
		if p.PID == self {
			if p.User == "" || p.Name == "" || p.RSS == 0 {
				t.Errorf("o'zimiz = %+v", p)
			}
			return
		}
	}
	t.Error("o'z jarayonimiz topilmadi")
}

func TestLookupUserCaches(t *testing.T) {
	s := NewSampler()
	s.userName[4242] = "keshdan"
	if got := s.lookupUser(4242); got != "keshdan" {
		t.Errorf("got %q", got)
	}
	if got := s.lookupUser(4_000_000); got != "4000000" {
		t.Errorf("noma'lum uid son bo'lib qolishi kerak: %q", got)
	}
}
