package target

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/jazho76/uplink/internal/config"
)

const (
	profilesSection = "profiles"
	targetsSection  = "targets"
	namePlaceholder = "{name}"
)

var legacySections = []string{"local", "lima", "remotes"}

type ModeSpec struct {
	Name string `yaml:"name"`
	Run  string `yaml:"run"`
	Back bool   `yaml:"back"`
}

type Spec struct {
	Name    string     `yaml:"name"`
	Profile string     `yaml:"profile"`
	Kind    Kind       `yaml:"kind"`
	Via     string     `yaml:"via"`
	Logs    string     `yaml:"logs"`
	Stats   *bool      `yaml:"stats"`
	Modes   []ModeSpec `yaml:"modes"`
}

var defaultHost = Spec{
	Name: "host",
	Kind: KindLocal,
	Modes: []ModeSpec{
		{Name: "tmux", Run: "tmux new-session -A -s host"},
		{Name: "shell"},
	},
}

func Load(cfg *config.File) ([]Target, error) {
	var errs []error
	for _, name := range legacySections {
		var ignored any
		if present, _ := cfg.Section(name, &ignored); present {
			errs = append(errs, fmt.Errorf("section %q is no longer supported; migrate to %s: and %s:, see the README",
				name, profilesSection, targetsSection))
		}
	}

	profiles := map[string]Spec{}
	if _, err := cfg.Section(profilesSection, &profiles); err != nil {
		errs = append(errs, err)
	}
	var specs []Spec
	present, err := cfg.Section(targetsSection, &specs)
	if err != nil {
		errs = append(errs, err)
	}
	if !present {
		specs = []Spec{defaultHost}
	}

	targets, resolveErr := resolveAll(specs, profiles)
	return targets, errors.Join(append(errs, resolveErr)...)
}

func resolveAll(specs []Spec, profiles map[string]Spec) ([]Target, error) {
	var (
		targets []Target
		errs    []error
		seen    = map[string]bool{}
		locals  int
	)
	for i, spec := range specs {
		t, err := resolve(spec, profiles)
		if err != nil {
			errs = append(errs, fmt.Errorf("target %s: %w", label(spec, i), err))
			continue
		}
		if seen[t.Name] {
			errs = append(errs, fmt.Errorf("duplicate target %q", t.Name))
			continue
		}
		if t.Local() {
			locals++
			if locals > 1 {
				errs = append(errs, fmt.Errorf("target %q: only one target can be %s", t.Name, KindLocal))
				continue
			}
		}
		seen[t.Name] = true
		targets = append(targets, t)
	}
	if locals == 0 {
		errs = append(errs, fmt.Errorf("no target is %s; declare the host machine", KindLocal))
	}
	return targets, errors.Join(errs...)
}

func label(spec Spec, index int) string {
	if spec.Name != "" {
		return fmt.Sprintf("%q", spec.Name)
	}
	return fmt.Sprintf("#%d", index+1)
}

func resolve(spec Spec, profiles map[string]Spec) (Target, error) {
	if spec.Name == "" {
		return Target{}, errors.New("no name")
	}
	if spec.Profile != "" {
		profile, ok := profiles[spec.Profile]
		if !ok {
			return Target{}, fmt.Errorf("unknown profile %q", spec.Profile)
		}
		spec = spec.over(profile)
	}
	spec = spec.interpolate()

	if !slices.Contains(kinds, spec.Kind) {
		return Target{}, fmt.Errorf("kind must be one of %s", kindList())
	}
	if spec.Kind == KindLocal && spec.Via != "" {
		return Target{}, fmt.Errorf("a %s target runs on this machine and takes no via", KindLocal)
	}
	if spec.Kind != KindLocal && spec.Via == "" {
		return Target{}, errors.New("via is required to reach it")
	}

	via, err := ShellWords(spec.Via)
	if err != nil {
		return Target{}, fmt.Errorf("via: %w", err)
	}

	t := Target{
		Name:   spec.Name,
		Kind:   spec.Kind,
		Via:    expandHome(via),
		Logs:   spec.Logs,
		Stats:  spec.Stats == nil || *spec.Stats,
		Status: StatusUnknown,
	}
	if t.Local() {
		t.Status = StatusUp
	}

	modes, err := t.modes(spec.Modes)
	if err != nil {
		return Target{}, err
	}
	t.Modes = modes
	return t, nil
}

func (s Spec) over(base Spec) Spec {
	if s.Kind == "" {
		s.Kind = base.Kind
	}
	if s.Via == "" {
		s.Via = base.Via
	}
	if s.Logs == "" {
		s.Logs = base.Logs
	}
	if s.Stats == nil {
		s.Stats = base.Stats
	}
	if len(s.Modes) == 0 {
		s.Modes = base.Modes
	}
	return s
}

func (s Spec) interpolate() Spec {
	fill := func(v string) string { return strings.ReplaceAll(v, namePlaceholder, s.Name) }
	s.Via, s.Logs = fill(s.Via), fill(s.Logs)
	modes := make([]ModeSpec, len(s.Modes))
	for i, m := range s.Modes {
		m.Run = fill(m.Run)
		modes[i] = m
	}
	s.Modes = modes
	return s
}

func (t Target) modes(specs []ModeSpec) ([]Mode, error) {
	if len(specs) == 0 {
		return nil, errors.New("no modes")
	}
	seen := map[string]bool{}
	modes := make([]Mode, 0, len(specs))
	for i, s := range specs {
		if s.Name == "" {
			return nil, fmt.Errorf("mode %d has no name", i+1)
		}
		if seen[s.Name] {
			return nil, fmt.Errorf("duplicate mode %q", s.Name)
		}
		seen[s.Name] = true
		modes = append(modes, Mode{Name: s.Name, Run: s.Run, Argv: t.ArgvFor(s.Run), Back: s.Back})
	}
	return modes, nil
}

func expandHome(words []string) []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return words
	}
	out := make([]string, len(words))
	for i, w := range words {
		switch {
		case w == "~":
			w = home
		case strings.HasPrefix(w, "~/"):
			w = filepath.Join(home, w[2:])
		}
		out[i] = w
	}
	return out
}

func kindList() string {
	names := make([]string, len(kinds))
	for i, k := range kinds {
		names[i] = string(k)
	}
	return strings.Join(names, ", ")
}
