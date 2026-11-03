package inventory

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"sync"

	"github.com/jazho76/uplink/internal/config"
	"github.com/jazho76/uplink/internal/shell"
	"github.com/jazho76/uplink/internal/target"
)

type Plan struct {
	Providers   []string
	Explicit    bool
	Invalid     bool
	Declared    []target.Target
	DefaultMode string
	Problems    error
}

type Catalog struct {
	Path      string
	Providers []Provider
}

func (c Catalog) ReadPlan() Plan {
	doc, err := config.Read(c.Path)
	if err != nil {
		names, _ := enabled(config.Document{}, c.Providers)
		return Plan{Providers: names, Invalid: true, Problems: err}
	}
	names, explicit := enabled(doc, c.Providers)
	var (
		known []string
		errs  []error
	)
	for _, name := range names {
		if !slices.ContainsFunc(c.Providers, func(p Provider) bool { return p.Name() == name }) {
			errs = append(errs, fmt.Errorf("discover: unknown provider %q", name))
			continue
		}
		known = append(known, name)
	}
	declared, err := doc.Resolve()
	return Plan{
		Providers:   known,
		Explicit:    explicit,
		Declared:    declared,
		DefaultMode: doc.DefaultMode,
		Problems:    errors.Join(append(errs, err)...),
	}
}

func (c Catalog) Discover(ctx context.Context, name string) ([]Discovered, error) {
	i := slices.IndexFunc(c.Providers, func(p Provider) bool { return p.Name() == name })
	if i < 0 {
		return nil, fmt.Errorf("unknown provider %q", name)
	}
	return c.Providers[i].Discover(ctx)
}

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
		results[name] = result{found: unknownStatus(i.results[name].found, shell.Describe(err)), err: err}
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
	found := make([]Found, 0, len(i.plan.Providers)+1)
	for _, name := range i.plan.Providers {
		r := i.results[name]
		found = append(found, Found{Source: name, Discovered: r.found, Err: r.err})
	}
	found = append(found, declaredFound(i.plan.Declared))
	merged, err := Merge(i.plan.DefaultMode, found...)
	return merged, errors.Join(i.plan.Problems, err)
}

func Load(ctx context.Context, catalog Catalog) ([]target.Target, error) {
	inventory := Inventory{}.WithPlan(catalog.ReadPlan())
	names := inventory.Providers()
	results := make([]result, len(names))
	var running sync.WaitGroup
	for i, name := range names {
		running.Go(func() {
			results[i].found, results[i].err = catalog.Discover(ctx, name)
		})
	}
	running.Wait()
	for i, name := range names {
		inventory = inventory.WithResult(name, results[i].found, results[i].err)
	}
	merged, err := inventory.Merged()
	return Targets(merged), err
}
