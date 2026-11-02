package target

import (
	"cmp"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/jazho76/uplink/internal/multiplexer"
	"github.com/jazho76/uplink/internal/stats"
)

const (
	namePlaceholder = "{name}"
	shellMode       = "shell"
)

var namePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

type ModeSpec struct {
	Name string `yaml:"name"`
	Run  string `yaml:"run"`

	Requires string `yaml:"-"`
}

func defaultModeChoices() []string {
	choices := make([]string, 0, len(multiplexer.All)+1)
	for _, m := range multiplexer.All {
		choices = append(choices, m.Tool())
	}
	return append(choices, shellMode)
}

func automaticModes(defaultMode string) []ModeSpec {
	modes := make([]ModeSpec, 0, len(multiplexer.All)+1)
	for _, m := range multiplexer.All {
		modes = append(modes, ModeSpec{
			Name:     m.Tool(),
			Requires: m.Tool(),
			Run:      "command -v " + m.Tool() + " >/dev/null && exec " + m.Attach() + "; " + loginShell,
		})
	}
	modes = append(modes, ModeSpec{Name: shellMode})

	first := cmp.Or(defaultMode, multiplexer.Tmux{}.Tool())
	rank := func(mode ModeSpec) int {
		if mode.Name == first {
			return 0
		}
		return 1
	}
	slices.SortStableFunc(modes, func(a, b ModeSpec) int { return cmp.Compare(rank(a), rank(b)) })
	return modes
}

type Spec struct {
	Name   string         `yaml:"name"`
	Group  string         `yaml:"group"`
	Via    string         `yaml:"via"`
	Exec   string         `yaml:"exec"`
	Status string         `yaml:"status"`
	Up     string         `yaml:"up"`
	Down   string         `yaml:"down"`
	Logs   *string        `yaml:"logs"`
	Watch  *bool          `yaml:"watch"`
	Stats  []stats.Metric `yaml:"stats"`
	Modes  []ModeSpec     `yaml:"modes"`

	Display  string `yaml:"-"`
	Local    bool   `yaml:"-"`
	Kind     string `yaml:"-"`
	Defaults *Spec  `yaml:"-"`
}

func resolve(source string, spec Spec, defaultMode string) (Target, error) {
	if spec.Name == "" {
		return Target{}, errors.New("no name")
	}
	if !namePattern.MatchString(spec.Name) {
		return Target{}, errors.New("name may only hold letters, digits, '.', '_' and '-'")
	}
	if spec.Defaults != nil {
		spec = spec.over(*spec.Defaults)
	}
	if len(spec.Modes) == 0 {
		spec.Modes = automaticModes(defaultMode)
	}
	if spec.Stats == nil && spec.Kind == KindContainer {
		spec.Stats = []stats.Metric{{Preset: stats.PresetContainer}}
	}
	builtinStats := spec.Stats == nil
	expanded, err := stats.Expand(spec.Stats)
	if err != nil {
		return Target{}, fmt.Errorf("stats: %w", err)
	}
	spec.Stats = expanded
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
		Logs:   valueOr(spec.Logs, ""),
		Watch:  valueOr(spec.Watch, false),
		Stats:  spec.Stats,
		Local:  spec.Local,

		BuiltinStats: builtinStats,
		GuestLogs:    spec.Logs == nil,
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

func valueOr[T any](value *T, fallback T) T {
	if value == nil {
		return fallback
	}
	return *value
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
	s.Via, s.Exec = fill(s.Via), fill(s.Exec)
	if s.Logs != nil {
		s.Logs = new(fill(*s.Logs))
	}
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
		modes = append(modes, Mode{Name: s.Name, Argv: t.ArgvFor(cmp.Or(s.Run, loginShell)), Requires: s.Requires})
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
