package config

import (
	"errors"
	"fmt"
	"path"
	"strings"

	"github.com/jazho76/uplink/internal/inventory"
	"github.com/jazho76/uplink/internal/target"
)

const overlayValidationName = "validation"

type overlaySpec struct {
	Match string       `yaml:"match"`
	Hide  *bool        `yaml:"hide"`
	Group string       `yaml:"group"`
	Logs  *string      `yaml:"logs"`
	Stats []metricSpec `yaml:"stats"`
	Modes []modeSpec   `yaml:"modes"`
}

func (o overlaySpec) adjust(t target.Target) target.Target {
	spec, fill := o.specFor(t.ID.Key)
	if o.Group != "" {
		t.Group = o.Group
	}
	if o.Logs != nil {
		t.Logs = spec.logSource(fill)
	}
	if o.Stats != nil {
		t.Metrics, _ = spec.metrics(fill)
	}
	if len(o.Modes) > 0 {
		t.Modes, _ = spec.modes(t.Reach, fill)
	}
	return t
}

func (o overlaySpec) specFor(name string) (Spec, func(string) string) {
	spec := Spec{Name: name, Logs: o.Logs, Stats: o.Stats, Modes: o.Modes}
	return spec, func(v string) string { return strings.ReplaceAll(v, namePlaceholder, name) }
}

func (o overlaySpec) validate() error {
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

func (d Document) Overlays() ([]inventory.Overlay, error) {
	var (
		overlays []inventory.Overlay
		errs     []error
	)
	for i, o := range d.Discovered {
		if err := o.validate(); err != nil {
			errs = append(errs, fmt.Errorf("discovered #%d: %w", i+1, err))
			continue
		}
		overlays = append(overlays, inventory.Overlay{Match: o.Match, Hide: o.Hide, Adjust: o.adjust})
	}
	return overlays, errors.Join(errs...)
}
