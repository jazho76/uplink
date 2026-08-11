package probe

import (
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/jazho76/uplink/internal/run"
)

type Stats struct {
	Cores     int
	Load      string
	MemUsed   uint64
	MemTotal  uint64
	DiskUsed  uint64
	DiskTotal uint64
	Uptime    time.Duration
}

const script = `read l _ < /proc/loadavg; echo "load $l"
echo "cores $(nproc 2>/dev/null || echo 0)"
awk '/^MemTotal:/{t=$2}/^MemAvailable:/{a=$2}END{print "mem", (t-a)*1024, t*1024}' /proc/meminfo
df -B1 --output=used,size / 2>/dev/null | tail -1 | awk '{print "disk", $1, $2}'
read u _ < /proc/uptime; echo "uptime $u"`

func Run(transport ...string) (Stats, error) {
	if len(transport) == 0 {
		return Stats{}, errors.New("probe: no transport given")
	}
	out, err := run.Feed(script, transport[0], transport[1:]...)
	if err != nil {
		return Stats{}, err
	}
	return Parse(out), nil
}

func Parse(out string) Stats {
	var s Stats
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) < 2 {
			continue
		}
		switch f[0] {
		case "load":
			s.Load = f[1]
		case "cores":
			s.Cores, _ = strconv.Atoi(f[1])
		case "mem":
			if len(f) > 2 {
				s.MemUsed, s.MemTotal = atou(f[1]), atou(f[2])
			}
		case "disk":
			if len(f) > 2 {
				s.DiskUsed, s.DiskTotal = atou(f[1]), atou(f[2])
			}
		case "uptime":
			s.Uptime = seconds(f[1])
		}
	}
	return s
}

func atou(s string) uint64 {
	n, _ := strconv.ParseUint(s, 10, 64)
	return n
}

func seconds(s string) time.Duration {
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return time.Duration(v) * time.Second
}
