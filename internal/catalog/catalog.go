package catalog

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/jazho76/uplink/internal/config"
	"github.com/jazho76/uplink/internal/inventory"
	"github.com/jazho76/uplink/internal/process"
	"github.com/jazho76/uplink/internal/target"
)

type Catalog struct {
	Path      string
	Providers []inventory.Provider
}

func (c Catalog) ReadPlan() inventory.Plan {
	doc, err := config.Read(c.Path)
	if err != nil {
		names, _ := enabled(config.Document{}, c.Providers)
		return inventory.Plan{Providers: names, Invalid: true, Problems: err}
	}
	names, explicit := enabled(doc, c.Providers)
	var (
		known []string
		errs  []error
	)
	for _, name := range names {
		if !c.knows(name) {
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
	return inventory.Plan{
		Providers:   known,
		Explicit:    explicit,
		Declared:    declared,
		Overlays:    overlays,
		DefaultMode: doc.DefaultMode,
		Problems:    errors.Join(errs...),
	}
}

func (c Catalog) knows(name string) bool {
	return slices.ContainsFunc(c.Providers, func(p inventory.Provider) bool { return p.Name() == name })
}

func (c Catalog) unknownOverlayProviders(overlays []config.Overlay) []error {
	var errs []error
	for _, overlay := range overlays {
		name := overlay.Provider()
		isPattern := strings.ContainsAny(name, `*?[\`)
		if !isPattern && !c.knows(name) {
			errs = append(errs, fmt.Errorf("discovered: unknown provider %q in %q", name, overlay.Match))
		}
	}
	return errs
}

func (c Catalog) Discover(ctx context.Context, name string) ([]inventory.Discovered, error) {
	i := slices.IndexFunc(c.Providers, func(p inventory.Provider) bool { return p.Name() == name })
	if i < 0 {
		return nil, fmt.Errorf("unknown provider %q", name)
	}
	found, err := c.Providers[i].Discover(ctx)
	if err != nil && !errors.Is(err, inventory.ErrUnavailable) {
		return found, inventory.Failure{Reason: process.Describe(err), Err: err}
	}
	return found, err
}

func enabled(doc config.Document, providers []inventory.Provider) (names []string, explicit bool) {
	if doc.Discover != nil {
		return doc.Discover, true
	}
	for _, p := range providers {
		names = append(names, p.Name())
	}
	return names, false
}

func Load(ctx context.Context, catalog Catalog) ([]target.Target, error) {
	current := inventory.Inventory{}.WithPlan(catalog.ReadPlan())
	names := current.Providers()
	type answer struct {
		found []inventory.Discovered
		err   error
	}
	answers := make([]answer, len(names))
	var running sync.WaitGroup
	for i, name := range names {
		running.Go(func() {
			answers[i].found, answers[i].err = catalog.Discover(ctx, name)
		})
	}
	running.Wait()
	for i, name := range names {
		current = current.WithResult(name, answers[i].found, answers[i].err)
	}
	merged, err := current.Merged()
	return inventory.Targets(merged), err
}
