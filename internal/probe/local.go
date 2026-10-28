package probe

import (
	"os"
	"runtime"
	"strings"
	"syscall"
)

func Local() Stats {
	s := Stats{Cores: runtime.NumCPU()}

	if data, err := os.ReadFile("/proc/loadavg"); err == nil {
		if f := strings.Fields(string(data)); len(f) > 0 {
			s.Load = atof(f[0])
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
				total = atou(f[1]) * 1024
			case "MemAvailable:":
				avail = atou(f[1]) * 1024
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
			s.Uptime = seconds(f[0])
		}
	}

	return s
}
