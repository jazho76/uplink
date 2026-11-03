package dashboard

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/jazho76/uplink/internal/target"
)

func formatBytes(n uint64) string {
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

func formatDuration(d time.Duration) string {
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

func inUnit(r target.Reading, v float64) string {
	switch r.Metric.Unit {
	case target.UnitBytes:
		return formatBytes(uint64(v))
	case target.UnitSeconds:
		return formatDuration(time.Duration(v * float64(time.Second)))
	default:
		return number(v)
	}
}

func number(v float64) string {
	if v == math.Trunc(v) {
		return strconv.FormatFloat(v, 'f', 0, 64)
	}
	return fmt.Sprintf("%.2f", v)
}

func amount(used, total uint64) string {
	u, t := formatBytes(used), formatBytes(total)
	if unit := unitOf(t); unit != "" && strings.HasSuffix(u, unit) {
		return strings.TrimSuffix(u, unit) + "/" + t
	}
	return u + "/" + t
}

func unitOf(size string) string {
	return strings.TrimLeft(size, "0123456789.")
}
