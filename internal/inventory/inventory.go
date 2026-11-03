package inventory

import (
	"cmp"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/jazho76/uplink/internal/config"
	"github.com/jazho76/uplink/internal/inspection/multiplexer"
	"github.com/jazho76/uplink/internal/shell"
	"github.com/jazho76/uplink/internal/target"
)

const shellMode = "shell"

type Source interface {
	Name() string
	Targets() ([]target.Target, error)
}

type Provider interface {
	Source
	Default() bool
}

type configSource []target.Target

func (configSource) Name() string { return target.ConfigSource }

func (c configSource) Targets() ([]target.Target, error) { return c, nil }

func Load(path string, providers []Provider) ([]target.Target, error) {
	doc, err := config.Read(path)
	if err != nil {
		return nil, err
	}

	var (
		sources []Source
		errs    []error
	)
	for _, name := range enabled(doc, providers) {
		i := slices.IndexFunc(providers, func(p Provider) bool { return p.Name() == name })
		if i < 0 {
			errs = append(errs, fmt.Errorf("discover: unknown provider %q", name))
			continue
		}
		sources = append(sources, providers[i])
	}

	declared, err := doc.Resolve()
	errs = append(errs, err)
	targets, err := Merge(doc.DefaultMode, append(sources, configSource(declared))...)
	return targets, errors.Join(append(errs, err)...)
}

func enabled(doc config.Document, providers []Provider) []string {
	if doc.Discover != nil {
		return doc.Discover
	}
	var defaults []string
	for _, p := range providers {
		if p.Default() {
			defaults = append(defaults, p.Name())
		}
	}
	return defaults
}

func Merge(defaultMode string, sources ...Source) ([]target.Target, error) {
	var (
		targets []target.Target
		errs    []error
		merged  = map[string]bool{}
		seen    = map[target.ID]bool{}
	)
	if choices := modeChoices(); defaultMode != "" && !slices.Contains(choices, defaultMode) {
		errs = append(errs, fmt.Errorf("default_mode: unknown mode %q; use one of %s", defaultMode, strings.Join(choices, ", ")))
		defaultMode = ""
	}
	for _, source := range sources {
		name := source.Name()
		if merged[name] {
			errs = append(errs, fmt.Errorf("duplicate source %q", name))
			continue
		}
		merged[name] = true

		found, err := source.Targets()
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", name, err))
			continue
		}
		for i, t := range found {
			t.ID.Source = name
			if err := target.ValidateKey(t.ID.Key); err != nil {
				errs = append(errs, fmt.Errorf("target %s: %w", label(t, i), err))
				continue
			}
			if seen[t.ID] {
				errs = append(errs, fmt.Errorf("duplicate target %q", t.ID))
				continue
			}
			seen[t.ID] = true
			targets = append(targets, withDefaults(t, defaultMode))
		}
	}
	return targets, errors.Join(errs...)
}

func withDefaults(t target.Target, defaultMode string) target.Target {
	t.Name = cmp.Or(t.Name, t.ID.Key)
	t.Group = cmp.Or(t.Group, target.DefaultGroup)
	if len(t.Modes) == 0 {
		t.Modes = automaticModes(t.Reach, defaultMode)
	}
	return t
}

func label(t target.Target, index int) string {
	if t.ID.Key == "" {
		return fmt.Sprintf("%s #%d", t.ID.Source, index+1)
	}
	return fmt.Sprintf("%q", t.ID)
}

func modeChoices() []string {
	return append(multiplexer.Tools(), shellMode)
}

func automaticModes(reach target.Reach, defaultMode string) []target.Mode {
	modes := make([]target.Mode, 0, len(multiplexer.All)+1)
	for _, m := range multiplexer.All {
		modes = append(modes, target.Mode{
			Name:        m.Tool(),
			Argv:        reach.Session(shell.POSIX(attachOrShell(m))),
			Multiplexer: m.Tool(),
			Requires:    m.Tool(),
		})
	}
	modes = append(modes, target.Mode{Name: shellMode, Argv: reach.Session(shell.POSIX(shell.LoginShell))})

	first := cmp.Or(defaultMode, multiplexer.Tmux{}.Tool())
	rank := func(mode target.Mode) int {
		if mode.Name == first {
			return 0
		}
		return 1
	}
	slices.SortStableFunc(modes, func(a, b target.Mode) int { return cmp.Compare(rank(a), rank(b)) })
	return modes
}

func attachOrShell(m multiplexer.Multiplexer) string {
	return `shell="${SHELL:-/bin/sh}"; "$shell" -lc 'command -v ` + m.Tool() + `' </dev/null >/dev/null 2>&1 && exec "$shell" -lc 'exec ` + m.Attach() + `'; ` + shell.LoginShell
}
