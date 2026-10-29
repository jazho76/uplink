package target

import (
	"cmp"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/jazho76/uplink/internal/config"
	"github.com/jazho76/uplink/internal/probe"
)

const (
	profilesSection = "profiles"
	targetsSection  = "targets"
	namePlaceholder = "{name}"
)

var (
	legacySections = []string{"local", "lima", "remotes"}
	namePattern    = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
)

type ModeSpec struct {
	Name string `yaml:"name"`
	Run  string `yaml:"run"`
	Back bool   `yaml:"back"`
}

type Spec struct {
	Name    string        `yaml:"name"`
	Profile string        `yaml:"profile"`
	Kind    string        `yaml:"kind"`
	Group   string        `yaml:"group"`
	Via     string        `yaml:"via"`
	Exec    string        `yaml:"exec"`
	Status  string        `yaml:"status"`
	Up      string        `yaml:"up"`
	Down    string        `yaml:"down"`
	Logs    string        `yaml:"logs"`
	Watch   *bool         `yaml:"watch"`
	Expand  *bool         `yaml:"expand"`
	Stats   probe.Metrics `yaml:"stats"`
	Modes   []ModeSpec    `yaml:"modes"`
}

var defaultHost = Spec{
	Name:   "host",
	Group:  "local",
	Expand: new(true),
	Stats:  probe.Metrics{{Type: probe.TypeLoad}, {Type: probe.TypeMem}, {Type: probe.TypeDisk}, {Type: probe.TypeUptime}},
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
	for _, name := range slices.Sorted(maps.Keys(profiles)) {
		if profiles[name].Kind != "" {
			errs = append(errs, fmt.Errorf("profile %q: %w", name, errKind))
		}
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

var errKind = errors.New("kind is no longer supported; use group, see the README")

func resolveAll(specs []Spec, profiles map[string]Spec) ([]Target, error) {
	var (
		targets []Target
		errs    []error
		seen    = map[string]bool{}
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
		seen[t.Name] = true
		targets = append(targets, t)
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
	if !namePattern.MatchString(spec.Name) {
		return Target{}, errors.New("name may only hold letters, digits, '.', '_' and '-'")
	}
	if spec.Kind != "" {
		return Target{}, errKind
	}
	if spec.Profile != "" {
		profile, ok := profiles[spec.Profile]
		if !ok {
			return Target{}, fmt.Errorf("unknown profile %q", spec.Profile)
		}
		spec = spec.over(profile)
	}
	spec = spec.interpolate()

	via, err := prefix("via", spec.Via)
	if err != nil {
		return Target{}, err
	}
	exec, err := prefix("exec", spec.Exec)
	if err != nil {
		return Target{}, err
	}
	for i, m := range spec.Stats {
		if err := m.Validate(); err != nil {
			return Target{}, fmt.Errorf("stats #%d: %w", i+1, err)
		}
	}

	t := Target{
		Name:   spec.Name,
		Group:  cmp.Or(spec.Group, DefaultGroup),
		Via:    via,
		Exec:   exec,
		Status: spec.Status,
		Up:     spec.Up,
		Down:   spec.Down,
		Logs:   spec.Logs,
		Watch:  valueOr(spec.Watch, false),
		Expand: valueOr(spec.Expand, false),
		Stats:  spec.Stats,
	}

	modes, err := t.modes(spec.Modes)
	if err != nil {
		return Target{}, err
	}
	t.Modes = modes
	return t, nil
}

func prefix(field, value string) ([]string, error) {
	if value == "" {
		return nil, nil
	}
	words, err := shellWords(value)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", field, err)
	}
	if len(words) == 0 {
		return nil, fmt.Errorf("%s is blank; leave it out to run on this machine", field)
	}
	return expandHome(words), nil
}

func valueOr(flag *bool, fallback bool) bool {
	if flag == nil {
		return fallback
	}
	return *flag
}

func (s Spec) over(base Spec) Spec {
	s.Group = cmp.Or(s.Group, base.Group)
	s.Via = cmp.Or(s.Via, base.Via)
	s.Exec = cmp.Or(s.Exec, base.Exec)
	s.Status = cmp.Or(s.Status, base.Status)
	s.Up = cmp.Or(s.Up, base.Up)
	s.Down = cmp.Or(s.Down, base.Down)
	s.Logs = cmp.Or(s.Logs, base.Logs)
	s.Watch = cmp.Or(s.Watch, base.Watch)
	s.Expand = cmp.Or(s.Expand, base.Expand)
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
	s.Via, s.Exec, s.Logs = fill(s.Via), fill(s.Exec), fill(s.Logs)
	s.Status, s.Up, s.Down = fill(s.Status), fill(s.Up), fill(s.Down)
	modes := make([]ModeSpec, len(s.Modes))
	for i, m := range s.Modes {
		m.Run = fill(m.Run)
		modes[i] = m
	}
	s.Modes = modes
	if s.Stats != nil {
		stats := make(probe.Metrics, len(s.Stats))
		for i, m := range s.Stats {
			m.Path, m.Value, m.Max = fill(m.Path), fill(m.Value), fill(m.Max)
			stats[i] = m
		}
		s.Stats = stats
	}
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
		modes = append(modes, Mode{Name: s.Name, Argv: t.ArgvFor(cmp.Or(s.Run, loginShell)), Back: s.Back})
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
