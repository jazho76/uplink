package probe

import "testing"

func TestLocalReadsHostCounters(t *testing.T) {
	s := Local()
	if s.Cores < 1 {
		t.Errorf("cores = %d", s.Cores)
	}
	if s.MemTotal == 0 || s.MemUsed == 0 || s.MemUsed > s.MemTotal {
		t.Errorf("mem = %d/%d", s.MemUsed, s.MemTotal)
	}
	if s.DiskTotal == 0 || s.DiskUsed > s.DiskTotal {
		t.Errorf("disk = %d/%d", s.DiskUsed, s.DiskTotal)
	}
	if s.Uptime <= 0 {
		t.Errorf("uptime = %s", s.Uptime)
	}
}
