package config

import (
	"cmp"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/jazho76/uplink/internal/inspection/multiplexer"
	"github.com/jazho76/uplink/internal/inspection/stats"
	"github.com/jazho76/uplink/internal/shell"
	"github.com/jazho76/uplink/internal/target"
)

const namePlaceholder = "{name}"

type Spec struct {
	Name  string       `yaml:"name"`
	Group string       `yaml:"group"`
	Via   string       `yaml:"via"`
	Exec  string       `yaml:"exec"`
	Check string       `yaml:"check"`
	Up    string       `yaml:"up"`
	Down  string       `yaml:"down"`
	Logs  *string      `yaml:"logs"`
	Watch bool         `yaml:"watch"`
	Stats []metricSpec `yaml:"stats"`
	Modes []modeSpec   `yaml:"modes"`
}

type modeSpec struct {
	Name        string `yaml:"name"`
	Run         string `yaml:"run"`
	Multiplexer string `yaml:"multiplexer"`
}

type metricSpec struct {
	Type   string `yaml:"type"`
	Label  string `yaml:"label"`
	Value  string `yaml:"value"`
	Max    string `yaml:"max"`
	Unit   string `yaml:"unit"`
	preset string
}

func (m *metricSpec) UnmarshalYAML(unmarshal func(any) error) error {
	if unmarshal(&m.preset) == nil {
		return nil
	}
	type plain metricSpec
	return unmarshal((*plain)(m))
}

func (s Spec) resolve() (target.Target, error) {
	if err := target.ValidateKey(s.Name); err != nil {
		return target.Target{}, err
	}
	if s.Up != "" && s.Check == "" {
		return target.Target{}, errors.New("up needs a check, or uplink can't tell when to start it")
	}
	fill := func(v string) string { return strings.ReplaceAll(v, namePlaceholder, s.Name) }

	metrics, err := s.metrics(fill)
	if err != nil {
		return target.Target{}, err
	}
	interactive, err := prefix("via", fill(s.Via))
	if err != nil {
		return target.Target{}, err
	}
	background, err := prefix("exec", fill(s.Exec))
	if err != nil {
		return target.Target{}, err
	}
	for i, m := range metrics {
		if err := m.Validate(); err != nil {
			return target.Target{}, fmt.Errorf("stats #%d: %w", i+1, err)
		}
	}

	t := target.Target{
		ID:    target.ID{Source: target.ConfigSource, Key: s.Name},
		Name:  s.Name,
		Group: s.Group,
		Reach: target.Reach{Interactive: interactive, Background: background},
		Check: fill(s.Check),
		Logs:  s.logSource(fill),
		Watch: s.Watch,
		Stats: target.StatsPolicy{Builtin: s.Stats == nil, Metrics: metrics},
	}
	if s.Up != "" || s.Down != "" {
		t.Power = &target.Power{Up: fill(s.Up), Down: fill(s.Down)}
	}
	t.Modes, err = s.modes(t.Reach, fill)
	if err != nil {
		return target.Target{}, err
	}
	return t, nil
}

func (s Spec) logSource(fill func(string) string) target.LogSource {
	if s.Logs == nil {
		return target.LogSource{Journal: true}
	}
	return target.LogSource{Command: fill(*s.Logs)}
}

func (s Spec) metrics(fill func(string) string) ([]target.Metric, error) {
	if s.Stats == nil {
		return nil, nil
	}
	metrics := []target.Metric{}
	for _, m := range s.Stats {
		if m.preset != "" {
			preset, ok := stats.Preset(m.preset)
			if !ok {
				return nil, fmt.Errorf("stats: unknown preset %q", m.preset)
			}
			metrics = append(metrics, preset...)
			continue
		}
		metrics = append(metrics, target.Metric{Type: m.Type, Label: m.Label, Value: fill(m.Value), Max: fill(m.Max), Unit: m.Unit})
	}
	return metrics, nil
}

func (s Spec) modes(reach target.Reach, fill func(string) string) ([]target.Mode, error) {
	if len(s.Modes) == 0 {
		return nil, nil
	}
	seen := map[string]bool{}
	modes := make([]target.Mode, 0, len(s.Modes))
	for i, m := range s.Modes {
		if m.Name == "" {
			return nil, fmt.Errorf("mode %d has no name", i+1)
		}
		if seen[m.Name] {
			return nil, fmt.Errorf("duplicate mode %q", m.Name)
		}
		seen[m.Name] = true
		if m.Multiplexer != "" && !slices.Contains(multiplexer.Tools(), m.Multiplexer) {
			return nil, fmt.Errorf("mode %q: unknown multiplexer %q; use one of %s", m.Name, m.Multiplexer, strings.Join(multiplexer.Tools(), ", "))
		}
		modes = append(modes, target.Mode{
			Name:        m.Name,
			Argv:        reach.Session(shell.POSIX(cmp.Or(fill(m.Run), shell.LoginShell))),
			Multiplexer: m.Multiplexer,
		})
	}
	return modes, nil
}

func prefix(field, value string) ([]string, error) {
	if value == "" {
		return nil, nil
	}
	words, err := shell.Words(value)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", field, err)
	}
	if len(words) == 0 {
		return nil, fmt.Errorf("%s is blank; leave it out to run on this machine", field)
	}
	return expandHome(words), nil
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
