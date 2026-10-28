package probe

import (
	"os"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func Local() Stats {
	s := Stats{Cores: runtime.NumCPU()}

	if data, err := os.ReadFile("/proc/loadavg"); err == nil {
		if f := strings.Fields(string(data)); len(f) > 0 {
			s.Load, _ = strconv.ParseFloat(f[0], 64)
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

	var fs syscall.Statfs_t
	if err := syscall.Statfs("/", &fs); err == nil {
		block := uint64(fs.Bsize)
		s.DiskTotal = fs.Blocks * block
		s.DiskUsed = (fs.Blocks - fs.Bfree) * block
	}

	if data, err := os.ReadFile("/proc/uptime"); err == nil {
		if f := strings.Fields(string(data)); len(f) > 0 {
			if secs, err := strconv.ParseFloat(f[0], 64); err == nil {
				s.Uptime = time.Duration(secs) * time.Second
			}
		}
	}

	return s
}

func kib(s string) uint64 {
	n, _ := strconv.ParseUint(s, 10, 64)
	return n * 1024
}
