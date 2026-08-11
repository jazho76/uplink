package lima

import (
	"strconv"
	"strings"

	"github.com/jazho76/uplink/internal/run"
)

type GuestStats struct {
	Load      string
	MemUsed   uint64
	MemTotal  uint64
	DiskUsed  uint64
	DiskTotal uint64
	Uptime    string
}

const guestProbe = `read l _ < /proc/loadavg; echo "load $l"
awk '/^MemTotal:/{t=$2}/^MemAvailable:/{a=$2}END{print "mem", (t-a)*1024, t*1024}' /proc/meminfo
df -B1 --output=used,size / 2>/dev/null | tail -1 | awk '{print "disk", $1, $2}'
echo "uptime $(uptime -p 2>/dev/null | sed 's/^up //' || true)"`

func Guest(name string) (GuestStats, error) {
	out, err := run.Output(bin, "shell", name, "--", "sh", "-c", guestProbe)
	if err != nil {
		return GuestStats{}, err
	}

	var s GuestStats
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		switch fields[0] {
		case "load":
			if len(fields) > 1 {
				s.Load = fields[1]
			}
		case "mem":
			if len(fields) > 2 {
				s.MemUsed = atou(fields[1])
				s.MemTotal = atou(fields[2])
			}
		case "disk":
			if len(fields) > 2 {
				s.DiskUsed = atou(fields[1])
				s.DiskTotal = atou(fields[2])
			}
		case "uptime":
			s.Uptime = strings.Join(fields[1:], " ")
		}
	}
	return s, nil
}

func atou(s string) uint64 {
	n, _ := strconv.ParseUint(s, 10, 64)
	return n
}
