package probe

import (
	"errors"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/jazho76/uplink/internal/run"
)

const (
	TypeGauge = "gauge"
	TypeText  = "text"

	UnitBytes   = "bytes"
	UnitSeconds = "seconds"
)

type Metric struct {
	Type  string `yaml:"type"`
	Label string `yaml:"label"`
	Value string `yaml:"value"`
	Max   string `yaml:"max"`
	Unit  string `yaml:"unit"`
}

type Metrics []Metric

type Reading struct {
	Metric
	Value float64
	Max   float64
	Text  string
	OK    bool
}

type rule struct {
	allowed  []string
	required []string
}

var rules = map[string]rule{
	TypeGauge: {allowed: []string{"label", "value", "max", "unit"}, required: []string{"label", "value", "max"}},
	TypeText:  {allowed: []string{"label", "value", "unit"}, required: []string{"label", "value"}},
}

var (
	types      = []string{TypeGauge, TypeText}
	units      = []string{UnitBytes, UnitSeconds}
	fieldNames = []string{"label", "value", "max", "unit"}
)

func (m Metric) fields() map[string]string {
	return map[string]string{"label": m.Label, "value": m.Value, "max": m.Max, "unit": m.Unit}
}

func (m Metric) Validate() error {
	r, ok := rules[m.Type]
	if !ok {
		return fmt.Errorf("type must be one of %s, got %q", strings.Join(types, ", "), m.Type)
	}
	fields := m.fields()
	for _, field := range fieldNames {
		if fields[field] != "" && !slices.Contains(r.allowed, field) {
			return fmt.Errorf("%s takes no %s", m.Type, field)
		}
	}
	for _, field := range r.required {
		if fields[field] == "" {
			return fmt.Errorf("%s needs a %s", m.Type, field)
		}
	}
	if m.Unit != "" && !slices.Contains(units, m.Unit) {
		return fmt.Errorf("unit must be one of %s, got %q", strings.Join(units, ", "), m.Unit)
	}
	return nil
}

func (m Metric) Gauge() bool { return m.Type == TypeGauge }

func (m Metric) constantMax() (float64, bool) {
	v, err := strconv.ParseFloat(strings.TrimSpace(m.Max), 64)
	return v, err == nil && !math.IsNaN(v)
}

func (m Metric) commands() (value, ceiling string) {
	if _, constant := m.constantMax(); m.Max == "" || constant {
		return m.Value, ""
	}
	return m.Value, m.Max
}

func capture(tag, command string) string {
	return fmt.Sprintf(`printf '%%s\n' %s"$($limit sh -c %s </dev/null 2>/dev/null | head -n 1)"`, quote(tag+" "), quote(command))
}

func quote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

var metricTimeout = 5 * time.Second

func compile(metrics Metrics) string {
	lines := []string{fmt.Sprintf("limit=; command -v timeout >/dev/null 2>&1 && limit='timeout %d'", int(metricTimeout.Seconds()))}
	for i, m := range metrics {
		value, ceiling := m.commands()
		lines = append(lines, capture(fmt.Sprintf("%d value", i), value))
		if ceiling != "" {
			lines = append(lines, capture(fmt.Sprintf("%d max", i), ceiling))
		}
	}
	return strings.Join(lines, "\n")
}

func Parse(out string, metrics Metrics) []Reading {
	values := map[int]string{}
	maxes := map[int]string{}
	for _, line := range strings.Split(out, "\n") {
		index, rest, ok := strings.Cut(line, " ")
		i, err := strconv.Atoi(index)
		if !ok || err != nil || i < 0 || i >= len(metrics) {
			continue
		}
		field, text, _ := strings.Cut(rest, " ")
		switch field {
		case "value":
			values[i] = strings.TrimSpace(text)
		case "max":
			maxes[i] = strings.TrimSpace(text)
		}
	}

	readings := make([]Reading, len(metrics))
	for i, m := range metrics {
		r := Reading{Metric: m, Text: values[i]}
		value, valueErr := strconv.ParseFloat(values[i], 64)
		switch {
		case m.Type == TypeText && m.Unit == "":
			r.OK = r.Text != ""
		case m.Type == TypeText:
			r.Value, r.OK = value, valueErr == nil
		default:
			ceiling, constant := m.constantMax()
			var maxErr error
			if !constant {
				ceiling, maxErr = strconv.ParseFloat(maxes[i], 64)
			}
			r.Value, r.Max, r.OK = value, ceiling, valueErr == nil && maxErr == nil
		}
		readings[i] = r
	}
	return readings
}

func (r Reading) Fraction() float64 {
	if !r.OK || r.Max <= 0 {
		return 0
	}
	return r.Value / r.Max
}

func Run(limit time.Duration, metrics Metrics, transport ...string) ([]Reading, error) {
	if len(transport) == 0 {
		return nil, errors.New("probe: no transport given")
	}
	out, err := run.NonInteractive(limit, compile(metrics), transport[0], transport[1:]...)
	if err != nil {
		return nil, err
	}
	return Parse(out, metrics), nil
}
