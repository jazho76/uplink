package local

import (
	"strings"
	"testing"
)

func TestTmuxArgvNesting(t *testing.T) {
	t.Setenv("TMUX", "")
	plain := strings.Join(tmuxArgv(), " ")
	if plain != "tmux new-session -A -s host" {
		t.Errorf("outside tmux, want a plain attach-or-create, got %q", plain)
	}

	t.Setenv("TMUX", "/tmp/tmux-1000/default,4242,0")
	nested := strings.Join(tmuxArgv(), " ")
	if !strings.Contains(nested, "-dA") || !strings.Contains(nested, "switch-client") {
		t.Errorf("inside tmux, want a detached create plus switch-client, got %q", nested)
	}
}

func TestProbeReadsHostCounters(t *testing.T) {
	s, err := New(Config{}).Probe("")
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	if s.Cores < 1 {
		t.Errorf("cores = %d", s.Cores)
	}
	if s.MemTotal == 0 || s.MemUsed == 0 || s.MemUsed > s.MemTotal {
		t.Errorf("mem = %d/%d", s.MemUsed, s.MemTotal)
	}
	if s.DiskTotal == 0 || s.DiskUsed > s.DiskTotal {
		t.Errorf("disk = %d/%d", s.DiskUsed, s.DiskTotal)
	}
	if s.Load == "" {
		t.Errorf("load is empty")
	}
	if s.Uptime <= 0 {
		t.Errorf("uptime = %s", s.Uptime)
	}
}

func TestListYieldsOneRunningHost(t *testing.T) {
	targets, err := New(Config{}).List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(targets) != 1 {
		t.Fatalf("want exactly one host target, got %d", len(targets))
	}
	host := targets[0]
	if host.Name != Name || !host.Running() {
		t.Errorf("want a running %q, got %q %q", Name, host.Name, host.Status)
	}
	if host.Section != "" {
		t.Errorf("the host row carries no section header, got %q", host.Section)
	}
	if len(host.Modes) == 0 {
		t.Errorf("host should have at least one launch mode")
	}
}
