package target

import (
	"cmp"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/jazho76/uplink/internal/stats"
)

const namePlaceholder = "{name}"

var namePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

type ModeSpec struct {
	Name string `yaml:"name"`
	Run  string `yaml:"run"`
}

type Spec struct {
	Name    string         `yaml:"name"`
	Profile string         `yaml:"profile"`
	Group   string         `yaml:"group"`
	Via     string         `yaml:"via"`
	Exec    string         `yaml:"exec"`
	Status  string         `yaml:"status"`
	Up      string         `yaml:"up"`
	Down    string         `yaml:"down"`
	Logs    string         `yaml:"logs"`
	Watch   *bool          `yaml:"watch"`
	Stats   []stats.Metric `yaml:"stats"`
	Modes   []ModeSpec     `yaml:"modes"`

	Display  string `yaml:"-"`
	Defaults *Spec  `yaml:"-"`
}

func resolve(source string, spec Spec, profiles map[string]Spec) (Target, error) {
	if spec.Name == "" {
		return Target{}, errors.New("no name")
	}
	if !namePattern.MatchString(spec.Name) {
		return Target{}, errors.New("name may only hold letters, digits, '.', '_' and '-'")
	}
	if spec.Profile != "" {
		profile, ok := profiles[spec.Profile]
		if !ok {
			return Target{}, fmt.Errorf("unknown profile %q", spec.Profile)
		}
		spec = spec.over(profile)
	}
	if spec.Defaults != nil {
		spec = spec.over(*spec.Defaults)
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
		ID:     ID{Source: source, Key: spec.Name},
		Name:   cmp.Or(spec.Display, spec.Name),
		Group:  cmp.Or(spec.Group, DefaultGroup),
		Via:    via,
		Exec:   exec,
		Status: spec.Status,
		Up:     spec.Up,
		Down:   spec.Down,
		Logs:   spec.Logs,
		Watch:  valueOr(spec.Watch, false),
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
		metrics := make([]stats.Metric, len(s.Stats))
		for i, m := range s.Stats {
			m.Value, m.Max = fill(m.Value), fill(m.Max)
			metrics[i] = m
		}
		s.Stats = metrics
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
		modes = append(modes, Mode{Name: s.Name, Argv: t.ArgvFor(cmp.Or(s.Run, loginShell))})
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
