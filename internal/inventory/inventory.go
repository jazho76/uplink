package inventory

import (
	"errors"
	"fmt"
	"slices"

	"github.com/jazho76/uplink/internal/target"
)

type Provider interface {
	target.Source
	Default() bool
}

func Load(path string, providers []Provider) ([]target.Target, error) {
	doc, err := target.Read(path)
	if err != nil {
		return nil, err
	}

	var (
		sources []target.Source
		errs    []error
	)
	for n, entry := range entries(doc, providers) {
		if entry.Provider == "" {
			errs = append(errs, fmt.Errorf("discover entry #%d has no provider", n+1))
			continue
		}
		i := slices.IndexFunc(providers, func(p Provider) bool { return p.Name() == entry.Provider })
		if i < 0 {
			errs = append(errs, fmt.Errorf("discover: unknown provider %q", entry.Provider))
			continue
		}
		sources = append(sources, withProfile(providers[i], entry.Profile))
	}

	targets, err := target.Merge(doc.Profiles, append(sources, doc.Config())...)
	return targets, errors.Join(append(errs, err)...)
}

func entries(doc target.Document, providers []Provider) []target.Entry {
	if doc.Discover != nil {
		return doc.Discover
	}
	var defaults []target.Entry
	for _, p := range providers {
		if p.Default() {
			defaults = append(defaults, target.Entry{Provider: p.Name()})
		}
	}
	return defaults
}

type profiled struct {
	target.Source
	profile string
}

func withProfile(source target.Source, profile string) target.Source {
	if profile == "" {
		return source
	}
	return profiled{source, profile}
}

func (p profiled) Specs() ([]target.Spec, error) {
	specs, err := p.Source.Specs()
	for i := range specs {
		specs[i].Profile = p.profile
	}
	return specs, err
}
