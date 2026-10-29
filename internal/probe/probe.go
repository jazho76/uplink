package probe

import (
	"strconv"
	"time"
)

type Stats struct {
	Cores     int
	Load      float64
	MemUsed   uint64
	MemTotal  uint64
	DiskUsed  uint64
	DiskTotal uint64
	Uptime    time.Duration
}

func atof(s string) float64 {
	v, _ := strconv.ParseFloat(s, 64)
	return v
}

func atou(s string) uint64 {
	n, _ := strconv.ParseUint(s, 10, 64)
	return n
}

func seconds(s string) time.Duration {
	return time.Duration(atof(s)) * time.Second
}
