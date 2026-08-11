package local

import (
	"os"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/jazho76/uplink/internal/probe"
	"github.com/jazho76/uplink/internal/run"
	"github.com/jazho76/uplink/internal/target"
)

const Name = "host"

const Section = "local"

type Config struct {
	Modes []target.ModeSpec `yaml:"modes"`
}

func (c Config) Validate() error { return target.ValidateSpecs(Section, c.Modes) }

type Provider struct {
	hostname string
	kernel   string
	modes    []target.Mode
}

func New(cfg Config) *Provider {
	p := &Provider{modes: buildModes(cfg.Modes)}
	p.hostname, _ = os.Hostname()
	if v, err := run.Output("uname", "-sr"); err == nil {
		p.kernel = v
	}
	return p
}

func buildModes(specs []target.ModeSpec) []target.Mode {
	if len(specs) == 0 {
		return []target.Mode{
			{Name: "tmux", Argv: tmuxArgv()},
			{Name: "shell", Argv: shellArgv("")},
		}
	}
	modes := make([]target.Mode, 0, len(specs))
	for _, s := range specs {
		modes = append(modes, target.Mode{Name: s.Name, Argv: shellArgv(s.Run), Back: s.Back})
	}
	return modes
}

func shellArgv(run string) []string {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/sh"
	}
	if run == "" {
		return []string{shell}
	}
	return []string{shell, "-c", run}
}

func (p *Provider) ID() string { return target.ProviderLocal }

func (p *Provider) List() ([]target.Target, error) {
	return []target.Target{{
		Provider: p.ID(),
		Name:     Name,
		Status:   target.StatusRunning,
		Modes:    p.modes,
		Detail: []target.Field{
			{Key: "host", Value: p.hostname},
			{Key: "os", Value: p.kernel},
		},
	}}, nil
}

func tmuxArgv() []string {
	if insideTmux() {
		return detachedSessionThenSwitchClientArgv()
	}
	return []string{"tmux", "new-session", "-A", "-s", Name}
}

func insideTmux() bool { return os.Getenv("TMUX") != "" }

func detachedSessionThenSwitchClientArgv() []string {
	return []string{"tmux", "new-session", "-dA", "-s", Name, ";", "switch-client", "-t", Name}
}

func (p *Provider) Probe(string) (probe.Stats, error) {
	s := probe.Stats{Cores: runtime.NumCPU()}

	if data, err := os.ReadFile("/proc/loadavg"); err == nil {
		if f := strings.Fields(string(data)); len(f) > 0 {
			s.Load = f[0]
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

	return s, nil
}

func kib(s string) uint64 {
	n, _ := strconv.ParseUint(s, 10, 64)
	return n * 1024
}
