package host

import (
	"os"
	"runtime"
	"strconv"
	"strings"
)

type Stats struct {
	Cores    int
	Load     string
	MemUsed  uint64
	MemTotal uint64
}

func Read() Stats {
	s := Stats{Cores: runtime.NumCPU()}

	if data, err := os.ReadFile("/proc/loadavg"); err == nil {
		f := strings.Fields(string(data))
		if len(f) >= 3 {
			s.Load = strings.Join(f[:3], " ")
		}
	}

	if data, err := os.ReadFile("/proc/meminfo"); err == nil {
		var total, avail uint64
		for _, line := range strings.Split(string(data), "\n") {
			f := strings.Fields(line)
			if len(f) < 2 {
				continue
			}
			switch f[0] {
			case "MemTotal:":
				total = kib(f[1])
			case "MemAvailable:":
				avail = kib(f[1])
			}
		}
		s.MemTotal = total
		if total > avail {
			s.MemUsed = total - avail
		}
	}
	return s
}

func kib(s string) uint64 {
	n, _ := strconv.ParseUint(s, 10, 64)
	return n * 1024
}
