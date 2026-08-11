package probe

import (
	"testing"
	"time"
)

func TestParse(t *testing.T) {
	out := `load 0.15
cores 6
mem 622088192 12522029056
disk 18157756416 160953249792
uptime 4416.90`

	s := Parse(out)
	if s.Load != "0.15" {
		t.Errorf("load = %q", s.Load)
	}
	if s.Cores != 6 {
		t.Errorf("cores = %d", s.Cores)
	}
	if s.MemUsed != 622088192 || s.MemTotal != 12522029056 {
		t.Errorf("mem = %d/%d", s.MemUsed, s.MemTotal)
	}
	if s.DiskUsed != 18157756416 || s.DiskTotal != 160953249792 {
		t.Errorf("disk = %d/%d", s.DiskUsed, s.DiskTotal)
	}
	if s.Uptime != 4416*time.Second {
		t.Errorf("uptime = %s", s.Uptime)
	}
}

func TestParseTolerantOfMissingLines(t *testing.T) {
	cases := map[string]string{
		"empty":          "",
		"noise":          "sh: nproc: not found\n",
		"partial":        "load 1.00\n",
		"truncated mem":  "mem 100\n",
		"unparsed nums":  "cores x\nuptime y\n",
		"trailing blank": "load 1.00\n\n",
	}
	for name, out := range cases {
		t.Run(name, func(t *testing.T) {
			Parse(out)
		})
	}

	if got := Parse("load 1.00\n").Load; got != "1.00" {
		t.Errorf("partial output should still yield load, got %q", got)
	}
	if got := Parse("mem 100\n").MemTotal; got != 0 {
		t.Errorf("a short mem line should be ignored, got %d", got)
	}
}
