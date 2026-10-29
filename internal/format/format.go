// Package format — hajm, tezlik, vaqt va foizni bir xil ko'rinishda chiqaradi.
package format

import (
	"fmt"
	"time"
)

const (
	unit       = 1024
	byteUnits  = "KMGTPE"
	secPerMin  = 60
	minPerHour = 60
	hourPerDay = 24
)

// Bytes ikkilik birliklarda: 512 B, 1.5 KiB, 3.2 GiB.
func Bytes(n uint64) string {
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := uint64(unit), 0
	for m := n / unit; m >= unit && exp < len(byteUnits)-1; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), byteUnits[exp])
}

// Rate soniyasiga baytni ko'rsatadi: 0 B/s, 1.2 MiB/s.
func Rate(bytesPerSec float64) string {
	if bytesPerSec < 0 {
		bytesPerSec = 0
	}
	return Bytes(uint64(bytesPerSec)) + "/s"
}

// Duration qisqa ko'rinishda: 45s, 12d 03s, 5s 20d, 3k 4s.
// k — kun, s — soat, d — daqiqa.
func Duration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	secs := int64(d / time.Second)
	mins := secs / secPerMin
	hours := mins / minPerHour
	days := hours / hourPerDay
	switch {
	case days > 0:
		return fmt.Sprintf("%dk %ds", days, hours%hourPerDay)
	case hours > 0:
		return fmt.Sprintf("%ds %02dd", hours, mins%minPerHour)
	case mins > 0:
		return fmt.Sprintf("%dd %02dsek", mins, secs%secPerMin)
	default:
		return fmt.Sprintf("%dsek", secs)
	}
}

// Ago "3d 05sek oldin" ko'rinishida; nol vaqt uchun "—".
func Ago(t, now time.Time) string {
	if t.IsZero() {
		return "—"
	}
	return Duration(now.Sub(t)) + " oldin"
}

// Percent bitta kasr bilan: 12.5%.
func Percent(p float64) string {
	return fmt.Sprintf("%.1f%%", p)
}
