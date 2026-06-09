package ui

import (
	"fmt"
	"strconv"
)

func Bytes(s string) string {
	n, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return s
	}
	units := []string{"B", "KiB", "MiB", "GiB", "TiB", "PiB"}
	i := 0
	for n >= 1024 && i < len(units)-1 {
		n /= 1024
		i++
	}
	if n == float64(int64(n)) {
		return fmt.Sprintf("%d%s", int64(n), units[i])
	}
	return fmt.Sprintf("%.1f%s", n, units[i])
}
