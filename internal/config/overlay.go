package config

import (
	"errors"
	"fmt"
	"path"
	"strings"

	"github.com/jazho76/uplink/internal/target"
)

const overlayValidationName = "validation"

type Overlay struct {
	Match string       `yaml:"match"`
	Hide  *bool        `yaml:"hide"`
	Group string       `yaml:"group"`
	Logs  *string      `yaml:"logs"`
	Stats []metricSpec `yaml:"stats"`
	Modes []modeSpec   `yaml:"modes"`
}

func (o Overlay) Matches(id target.ID) bool {
	matched, err := path.Match(o.Match, id.String())
	return err == nil && matched
}

func (o Overlay) Provider() string {
	provider, _, _ := strings.Cut(o.Match, "/")
	return provider
}

func (o Overlay) Apply(t target.Target) target.Target {
	spec, fill := o.specFor(t.ID.Key)
	if o.Group != "" {
		t.Group = o.Group
	}
	if o.Logs != nil {
		t.Logs = spec.logSource(fill)
	}
	if o.Stats != nil {
		metrics, _ := spec.metrics(fill)
		t.Stats = target.StatsPolicy{Metrics: metrics}
	}
	if len(o.Modes) > 0 {
		t.Modes, _ = spec.modes(t.Reach, fill)
	}
	return t
}

func (o Overlay) specFor(name string) (Spec, func(string) string) {
	spec := Spec{Name: name, Logs: o.Logs, Stats: o.Stats, Modes: o.Modes}
	return spec, func(v string) string { return strings.ReplaceAll(v, namePlaceholder, name) }
}

func (o Overlay) validate() error {
	if o.Match == "" {
		return errors.New("no match")
	}
	if strings.Count(o.Match, "/") != 1 {
		return fmt.Errorf("match %q must look like provider/name", o.Match)
	}
	if _, err := path.Match(o.Match, ""); err != nil {
		return fmt.Errorf("match %q: %w", o.Match, err)
	}
	spec, fill := o.specFor(overlayValidationName)
	metrics, err := spec.metrics(fill)
	if err != nil {
		return err
	}
	if err := validateMetrics(metrics); err != nil {
		return err
	}
	_, err = spec.modes(target.Reach{}, fill)
	return err
}

func (d Document) Overlays() ([]Overlay, error) {
	var (
		overlays []Overlay
		errs     []error
	)
	for i, o := range d.Discovered {
		if err := o.validate(); err != nil {
			errs = append(errs, fmt.Errorf("discovered #%d: %w", i+1, err))
			continue
		}
		overlays = append(overlays, o)
	}
	return overlays, errors.Join(errs...)
}
