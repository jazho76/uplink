package target

import (
	"errors"
	"fmt"
)

type Source interface {
	Name() string
	Specs() ([]Spec, error)
}

type configSource []Spec

func (configSource) Name() string { return ConfigSource }

func (c configSource) Specs() ([]Spec, error) { return c, nil }

func Merge(profiles map[string]Spec, sources ...Source) ([]Target, error) {
	var (
		targets []Target
		errs    []error
		merged  = map[string]bool{}
		seen    = map[ID]bool{}
	)
	for _, source := range sources {
		name := source.Name()
		if merged[name] {
			errs = append(errs, fmt.Errorf("duplicate source %q", name))
			continue
		}
		merged[name] = true

		specs, err := source.Specs()
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", name, err))
			continue
		}
		for i, spec := range specs {
			t, err := resolve(name, spec, profiles)
			if err != nil {
				errs = append(errs, fmt.Errorf("target %s: %w", label(name, spec, i), err))
				continue
			}
			if seen[t.ID] {
				errs = append(errs, fmt.Errorf("duplicate target %q", t.ID))
				continue
			}
			seen[t.ID] = true
			targets = append(targets, t)
		}
	}
	return targets, errors.Join(errs...)
}

func label(source string, spec Spec, index int) string {
	switch {
	case spec.Name != "":
		return fmt.Sprintf("%q", ID{Source: source, Key: spec.Name})
	case source == ConfigSource:
		return fmt.Sprintf("#%d", index+1)
	default:
		return fmt.Sprintf("%s #%d", source, index+1)
	}
}
