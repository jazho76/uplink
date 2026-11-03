package inventory

import (
	"errors"
	"maps"
	"slices"

	"github.com/jazho76/uplink/internal/config"
	"github.com/jazho76/uplink/internal/process"
	"github.com/jazho76/uplink/internal/target"
)

type result struct {
	found []Discovered
	err   error
}

type Inventory struct {
	plan    Plan
	planned bool
	results map[string]result
}

func (i Inventory) WithPlan(plan Plan) Inventory {
	if plan.Invalid && i.planned {
		plan.Providers, plan.Explicit = i.plan.Providers, i.plan.Explicit
	}
	i.plan, i.planned = plan, true
	results := map[string]result{}
	for _, name := range plan.Providers {
		if previous, ok := i.results[name]; ok {
			results[name] = previous
		}
	}
	i.results = results
	return i
}

func (i Inventory) Providers() []string { return i.plan.Providers }

func (i Inventory) Plans(name string) bool { return slices.Contains(i.plan.Providers, name) }

func (i Inventory) Answered() bool {
	return !slices.ContainsFunc(i.plan.Providers, func(name string) bool {
		_, answered := i.results[name]
		return !answered
	})
}

func (i Inventory) WithResult(name string, found []Discovered, err error) Inventory {
	if !i.Plans(name) {
		return i
	}
	results := maps.Clone(i.results)
	if results == nil {
		results = map[string]result{}
	}
	switch {
	case err == nil:
		results[name] = result{found: found}
	case errors.Is(err, ErrUnavailable):
		results[name] = result{err: reportedOnlyIf(i.plan.Explicit, err)}
	default:
		results[name] = result{found: unknownStatus(i.results[name].found, process.Describe(err)), err: err}
	}
	i.results = results
	return i
}

func reportedOnlyIf(explicit bool, err error) error {
	if explicit {
		return err
	}
	return nil
}

func unknownStatus(found []Discovered, reason string) []Discovered {
	unknown := slices.Clone(found)
	for i := range unknown {
		unknown[i].Status, unknown[i].Reason = target.StatusUnknown, reason
	}
	return unknown
}

func (i Inventory) Merged() ([]Discovered, error) {
	found := make([]Found, 0, len(i.plan.Providers))
	for _, name := range i.plan.Providers {
		r := i.results[name]
		found = append(found, Found{Source: name, Discovered: r.found, Err: r.err})
	}
	discovered, errs := collect(found...)
	adjusted := applyOverlays(discovered, i.plan.Overlays)
	declared, declaredErrs := collect(declaredFound(i.plan.Declared))
	all, err := applyDefaults(append(adjusted, declared...), i.plan.DefaultMode)
	return all, errors.Join(append(append([]error{i.plan.Problems}, errs...), append(declaredErrs, err)...)...)
}

func applyOverlays(discovered []Discovered, overlays []config.Overlay) []Discovered {
	var kept []Discovered
	for _, d := range discovered {
		hidden := false
		for _, overlay := range overlays {
			if !overlay.Matches(d.Target.ID) {
				continue
			}
			d.Target = overlay.Apply(d.Target)
			if overlay.Hide != nil {
				hidden = *overlay.Hide
			}
		}
		if !hidden {
			kept = append(kept, d)
		}
	}
	return kept
}

func Targets(merged []Discovered) []target.Target {
	var targets []target.Target
	for _, d := range merged {
		targets = append(targets, d.Target)
	}
	return targets
}
