package lima

import (
	"io"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/jazho76/uplink/internal/humanize"
	"github.com/jazho76/uplink/internal/probe"
	"github.com/jazho76/uplink/internal/run"
	"github.com/jazho76/uplink/internal/target"
)

const Section = "lima"

const listHeader = "vms"

type Config struct {
	Modes []target.ModeSpec `yaml:"modes"`
}

func (c Config) Validate() error { return target.ValidateSpecs(Section, c.Modes) }

type Provider struct {
	specs []target.ModeSpec
}

func New(cfg Config) *Provider {
	specs := cfg.Modes
	if len(specs) == 0 {
		specs = defaultSpecs()
	}
	return &Provider{specs: specs}
}

func defaultSpecs() []target.ModeSpec {
	return []target.ModeSpec{
		{Name: "tmux", Run: "tmux new-session -A -s 0"},
		{Name: "shell"},
	}
}

func (p *Provider) modes(instance string) []target.Mode {
	modes := make([]target.Mode, 0, len(p.specs))
	for _, s := range p.specs {
		modes = append(modes, target.Mode{Name: s.Name, Argv: shellArgv(instance, s.Run), Back: s.Back})
	}
	return modes
}

func shellArgv(instance, payload string) []string {
	argv := []string{bin, "shell", instance}
	if payload != "" {
		argv = append(argv, "--", "sh", "-c", payload)
	}
	return argv
}

func (p *Provider) ID() string { return target.ProviderLima }

func (p *Provider) List() ([]target.Target, error) {
	instances, err := List()
	if err != nil {
		return nil, err
	}
	targets := make([]target.Target, 0, len(instances))
	for _, inst := range instances {
		targets = append(targets, p.asTarget(inst))
	}
	sort.Slice(targets, func(i, j int) bool { return targets[i].Name < targets[j].Name })
	return targets, nil
}

func (p *Provider) asTarget(inst Instance) target.Target {
	t := target.Target{
		Provider: p.ID(),
		Section:  listHeader,
		Name:     inst.Name,
		Status:   target.Status(strings.ToLower(inst.Status)),
		CPUs:     atoi(inst.CPUs),
		Memory:   atou(inst.Memory),
		Modes:    p.modes(inst.Name),
	}

	if inst.TemplateDir != "" {
		t.Detail = append(t.Detail, target.Field{Key: "template", Value: filepath.Base(inst.TemplateDir)})
	}
	t.Detail = append(t.Detail,
		target.Field{Key: "cpus", Value: inst.CPUs},
		target.Field{Key: "mem", Value: humanize.Bytes(t.Memory)},
		target.Field{Key: "disk", Value: humanize.Bytes(atou(inst.Disk))},
	)
	if inst.SSHAddress != "" {
		t.Detail = append(t.Detail, target.Field{Key: "ssh", Value: inst.SSHAddress + ":" + inst.SSHLocalPort})
	}
	for _, f := range []target.Field{
		{Key: "type", Value: inst.VMType},
		{Key: "arch", Value: inst.Arch},
		{Key: "host", Value: inst.Hostname},
		{Key: "dir", Value: inst.Dir},
	} {
		if f.Value != "" {
			t.Detail = append(t.Detail, f)
		}
	}
	return t
}

func (*Provider) Start(name string, progress io.Writer) error {
	if progress == nil {
		return run.Silent(bin, "start", name)
	}
	return run.StreamTo(progress, bin, "start", name)
}

func (*Provider) Stop(name string) error   { return Stop(name) }
func (*Provider) Delete(name string) error { return Delete(name) }

func (*Provider) Autostart(name string) bool { return AutostartEnabled(name) }

func (*Provider) SetAutostart(name string, on bool) error { return SetAutostart(name, on) }

func (*Provider) Tail(name string, lines int) string { return Tail(name, lines) }

func (*Provider) Probe(name string) (probe.Stats, error) {
	return probe.Run(bin, "shell", name, "--", "sh")
}

func atoi(s string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(s))
	return n
}

func atou(s string) uint64 {
	n, _ := strconv.ParseUint(strings.TrimSpace(s), 10, 64)
	return n
}
