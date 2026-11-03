package inventory

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/jazho76/uplink/internal/config"
	"github.com/jazho76/uplink/internal/target"
)

type Plan struct {
	Providers   []string
	Explicit    bool
	Invalid     bool
	Declared    []target.Target
	Overlays    []config.Overlay
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
	errs = append(errs, err)
	overlays, err := doc.Overlays()
	errs = append(errs, err)
	errs = append(errs, c.unknownOverlayProviders(overlays)...)
	return Plan{
		Providers:   known,
		Explicit:    explicit,
		Declared:    declared,
		Overlays:    overlays,
		DefaultMode: doc.DefaultMode,
		Problems:    errors.Join(errs...),
	}
}

func (c Catalog) unknownOverlayProviders(overlays []config.Overlay) []error {
	var errs []error
	for _, overlay := range overlays {
		name := overlay.Provider()
		isPattern := strings.ContainsAny(name, `*?[\`)
		if !isPattern && !slices.ContainsFunc(c.Providers, func(p Provider) bool { return p.Name() == name }) {
			errs = append(errs, fmt.Errorf("discovered: unknown provider %q in %q", name, overlay.Match))
		}
	}
	return errs
}

func (c Catalog) Discover(ctx context.Context, name string) ([]Discovered, error) {
	i := slices.IndexFunc(c.Providers, func(p Provider) bool { return p.Name() == name })
	if i < 0 {
		return nil, fmt.Errorf("unknown provider %q", name)
	}
	return c.Providers[i].Discover(ctx)
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

func declaredFound(declared []target.Target) Found {
	found := Found{Source: target.ConfigSource, Discovered: make([]Discovered, len(declared))}
	for i, t := range declared {
		found.Discovered[i] = Discovered{Target: t, Status: target.StatusUnknown}
	}
	return found
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
