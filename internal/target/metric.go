package target

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

const (
	TypeGauge = "gauge"
	TypeText  = "text"

	UnitBytes   = "bytes"
	UnitSeconds = "seconds"
)

var (
	metricTypes = []string{TypeGauge, TypeText}
	metricUnits = []string{UnitBytes, UnitSeconds}
)

type Metric struct {
	Type  string
	Label string
	Value string
	Max   string
	Unit  string
}

func (m Metric) Gauge() bool { return m.Type == TypeGauge }

func (m Metric) Validate() error {
	if !slices.Contains(metricTypes, m.Type) {
		return fmt.Errorf("type must be one of %s, got %q", strings.Join(metricTypes, ", "), m.Type)
	}
	if m.Type == TypeText && m.Max != "" {
		return errors.New("text takes no max")
	}
	switch {
	case m.Label == "":
		return fmt.Errorf("%s needs a label", m.Type)
	case m.Value == "":
		return fmt.Errorf("%s needs a value", m.Type)
	case m.Gauge() && m.Max == "":
		return errors.New("gauge needs a max")
	}
	if m.Unit != "" && !slices.Contains(metricUnits, m.Unit) {
		return fmt.Errorf("unit must be one of %s, got %q", strings.Join(metricUnits, ", "), m.Unit)
	}
	return nil
}

type Reading struct {
	Metric Metric
	Value  float64
	Max    float64
	Text   string
	OK     bool
}

func (r Reading) Fraction() float64 {
	if !r.OK || r.Max <= 0 {
		return 0
	}
	return r.Value / r.Max
}
