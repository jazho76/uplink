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
	for _, name := range enabled(doc, providers) {
		i := slices.IndexFunc(providers, func(p Provider) bool { return p.Name() == name })
		if i < 0 {
			errs = append(errs, fmt.Errorf("discover: unknown provider %q", name))
			continue
		}
		sources = append(sources, providers[i])
	}

	targets, err := target.MergeWithDefault(doc.DefaultMode, append(sources, doc.Config())...)
	return targets, errors.Join(append(errs, err)...)
}

func enabled(doc target.Document, providers []Provider) []string {
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
