package target

import (
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/jazho76/uplink/internal/probe"
)

const (
	ProviderLocal  = "local"
	ProviderLima   = "lima"
	ProviderRemote = "remote"
)

type Status string

const (
	StatusUnknown     Status = "unknown"
	StatusRunning     Status = "running"
	StatusStopped     Status = "stopped"
	StatusUnreachable Status = "unreachable"
)

type Field struct {
	Key   string
	Value string
}

type Mode struct {
	Name string
	Argv []string
	Back bool
}

type ModeSpec struct {
	Name string `yaml:"name"`
	Run  string `yaml:"run"`
	Back bool   `yaml:"back"`
}

func ValidateSpecs(section string, specs []ModeSpec) error {
	seen := map[string]bool{}
	for i, s := range specs {
		if s.Name == "" {
			return fmt.Errorf("%s: mode %d has no name", section, i+1)
		}
		if seen[s.Name] {
			return fmt.Errorf("%s: duplicate mode %q", section, s.Name)
		}
		seen[s.Name] = true
	}
	return nil
}

type Target struct {
	Provider string
	Section  string
	Name     string
	Status   Status
	CPUs     int
	Memory   uint64
	Modes    []Mode
	Detail   []Field
}

func (t Target) Running() bool { return t.Status == StatusRunning }

func (t Target) DefaultMode() Mode {
	if len(t.Modes) == 0 {
		return Mode{}
	}
	return t.Modes[0]
}

func (t Target) Mode(name string) (Mode, bool) {
	for _, m := range t.Modes {
		if m.Name == name {
			return m, true
		}
	}
	return Mode{}, false
}

func (t Target) ModeNames() []string {
	names := make([]string, 0, len(t.Modes))
	for _, m := range t.Modes {
		names = append(names, m.Name)
	}
	return names
}

type Provider interface {
	ID() string
	List() ([]Target, error)
}

type Lifecycle interface {
	Start(name string, progress io.Writer) error
	Stop(name string) error
	Delete(name string) error
}

type Autostarter interface {
	Autostart(name string) bool
	SetAutostart(name string, on bool) error
}

type Tailer interface {
	Tail(name string, lines int) string
}

type Prober interface {
	Probe(name string) (probe.Stats, error)
}

type Registry struct {
	providers []Provider
}

func NewRegistry(providers ...Provider) Registry {
	return Registry{providers: providers}
}

func (r Registry) All() ([]Target, error) {
	var (
		all  []Target
		errs []error
		seen = map[string]string{}
	)
	for _, p := range r.providers {
		targets, err := p.List()
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", p.ID(), err))
			continue
		}
		for _, t := range targets {
			if owner, dup := seen[t.Name]; dup {
				errs = append(errs, fmt.Errorf("duplicate target %q from %s and %s; rename one", t.Name, owner, p.ID()))
				continue
			}
			seen[t.Name] = p.ID()
			all = append(all, t)
		}
	}
	return all, errors.Join(errs...)
}

func (r Registry) Provider(id string) Provider {
	for _, p := range r.providers {
		if p.ID() == id {
			return p
		}
	}
	return nil
}

func (r Registry) Resolve(name string) (Target, Provider, error) {
	all, err := r.All()
	for _, t := range all {
		if t.Name == name {
			return t, r.Provider(t.Provider), nil
		}
	}
	if err != nil {
		return Target{}, nil, fmt.Errorf("no such target %q: %w", name, err)
	}
	return Target{}, nil, fmt.Errorf("no such target %q; known targets: %s", name, strings.Join(names(all), ", "))
}

func names(targets []Target) []string {
	out := make([]string, 0, len(targets))
	for _, t := range targets {
		out = append(out, t.Name)
	}
	sort.Strings(out)
	return out
}
