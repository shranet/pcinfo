package format

import (
	"testing"
	"time"
)

func TestBytes(t *testing.T) {
	tests := []struct {
		in   uint64
		want string
	}{
		{0, "0 B"},
		{1023, "1023 B"},
		{1024, "1.0 KiB"},
		{1536, "1.5 KiB"},
		{5 << 30, "5.0 GiB"},
		{1 << 62, "4.0 EiB"},
	}
	for _, tt := range tests {
		if got := Bytes(tt.in); got != tt.want {
			t.Errorf("Bytes(%d) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestRateNegative(t *testing.T) {
	if got := Rate(-5); got != "0 B/s" {
		t.Errorf("Rate(-5) = %q", got)
	}
}

func TestDuration(t *testing.T) {
	tests := []struct {
		in   time.Duration
		want string
	}{
		{-time.Second, "0sek"},
		{45 * time.Second, "45sek"},
		{12*time.Minute + 3*time.Second, "12d 03sek"},
		{5*time.Hour + 20*time.Minute, "5s 20d"},
		{3*24*time.Hour + 4*time.Hour, "3k 4s"},
	}
	for _, tt := range tests {
		if got := Duration(tt.in); got != tt.want {
			t.Errorf("Duration(%v) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestAgoZero(t *testing.T) {
	if got := Ago(time.Time{}, time.Now()); got != "—" {
		t.Errorf("Ago(zero) = %q", got)
	}
}
