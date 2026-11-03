package inventory

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/jazho76/uplink/internal/config"
	"github.com/jazho76/uplink/internal/inspection/multiplexer"
	"github.com/jazho76/uplink/internal/shell"
	"github.com/jazho76/uplink/internal/target"
)

const shellMode = "shell"

var ErrUnavailable = errors.New("unavailable")

type Provider interface {
	Name() string
	Discover(ctx context.Context) ([]Discovered, error)
}

type Discovered struct {
	Target target.Target
	Status target.Status
}

type Found struct {
	Source     string
	Discovered []Discovered
	Err        error
}

func Load(ctx context.Context, path string, providers []Provider) ([]target.Target, error) {
	doc, err := config.Read(path)
	if err != nil {
		return nil, err
	}
	names, explicit := enabled(doc, providers)
	found, errs := discoverAll(ctx, names, explicit, providers)

	declared, err := doc.Resolve()
	errs = append(errs, err)
	found = append(found, declaredFound(declared))
	merged, err := Merge(doc.DefaultMode, found...)
	return Targets(merged), errors.Join(append(errs, err)...)
}

func enabled(doc config.Document, providers []Provider) (names []string, explicit bool) {
	if doc.Discover != nil {
		return doc.Discover, true
	}
	for _, p := range providers {
		names = append(names, p.Name())
	}
	return names, false
}

func discoverAll(ctx context.Context, names []string, explicit bool, providers []Provider) ([]Found, []error) {
	found := make([]Found, len(names))
	errs := make([]error, len(names))
	var running sync.WaitGroup
	for i, name := range names {
		found[i].Source = name
		at := slices.IndexFunc(providers, func(p Provider) bool { return p.Name() == name })
		if at < 0 {
			errs[i] = fmt.Errorf("discover: unknown provider %q", name)
			continue
		}
		running.Go(func() {
			found[i].Discovered, found[i].Err = providers[at].Discover(ctx)
			if !explicit && errors.Is(found[i].Err, ErrUnavailable) {
				found[i].Err = nil
			}
		})
	}
	running.Wait()
	return found, errs
}

func declaredFound(declared []target.Target) Found {
	found := Found{Source: target.ConfigSource, Discovered: make([]Discovered, len(declared))}
	for i, t := range declared {
		found.Discovered[i] = Discovered{Target: t, Status: target.StatusUnknown}
	}
	return found
}

func Targets(merged []Discovered) []target.Target {
	var targets []target.Target
	for _, d := range merged {
		targets = append(targets, d.Target)
	}
	return targets
}

func Merge(defaultMode string, found ...Found) ([]Discovered, error) {
	var (
		merged  []Discovered
		errs    []error
		sources = map[string]bool{}
		seen    = map[target.ID]bool{}
	)
	if choices := modeChoices(); defaultMode != "" && !slices.Contains(choices, defaultMode) {
		errs = append(errs, fmt.Errorf("default_mode: unknown mode %q; use one of %s", defaultMode, strings.Join(choices, ", ")))
		defaultMode = ""
	}
	for _, source := range found {
		if sources[source.Source] {
			errs = append(errs, fmt.Errorf("duplicate source %q", source.Source))
			continue
		}
		sources[source.Source] = true
		if source.Err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", source.Source, source.Err))
		}
		for i, d := range source.Discovered {
			t := d.Target
			t.ID.Source = source.Source
			if err := target.ValidateKey(t.ID.Key); err != nil {
				errs = append(errs, fmt.Errorf("target %s: %w", label(t, i), err))
				continue
			}
			if seen[t.ID] {
				errs = append(errs, fmt.Errorf("duplicate target %q", t.ID))
				continue
			}
			seen[t.ID] = true
			merged = append(merged, Discovered{Target: withDefaults(t, defaultMode), Status: d.Status})
		}
	}
	return merged, errors.Join(errs...)
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
