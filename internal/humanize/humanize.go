package humanize

import (
	"fmt"
	"time"
)

func Bytes(n uint64) string {
	units := []string{"B", "KiB", "MiB", "GiB", "TiB", "PiB"}
	v := float64(n)
	i := 0
	for v >= 1024 && i < len(units)-1 {
		v /= 1024
		i++
	}
	if v == float64(int64(v)) {
		return fmt.Sprintf("%d%s", int64(v), units[i])
	}
	return fmt.Sprintf("%.1f%s", v, units[i])
}

func Duration(d time.Duration) string {
	switch {
	case d <= 0:
		return ""
	case d >= 24*time.Hour:
		hours := int(d.Hours())
		return fmt.Sprintf("%dd %dh", hours/24, hours%24)
	case d >= time.Hour:
		return fmt.Sprintf("%dh %dm", int(d.Hours()), int(d.Minutes())%60)
	case d >= time.Minute:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	default:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
}
