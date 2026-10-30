package stats

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

type Reading struct {
	Metric Metric
	Value  float64
	Max    float64
	Text   string
	OK     bool
}

var (
	types = []string{TypeGauge, TypeText}
	units = []string{UnitBytes, UnitSeconds}
)

func (m Metric) Validate() error {
	if !slices.Contains(types, m.Type) {
		return fmt.Errorf("type must be one of %s, got %q", strings.Join(types, ", "), m.Type)
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

func (m Metric) maxCommand() string {
	if _, constant := m.constantMax(); constant {
		return ""
	}
	return m.Max
}

func capture(tag, command string) string {
	return fmt.Sprintf(`printf '%%s\n' %s"$($limit sh -c %s </dev/null 2>/dev/null | head -n 1)"`, quote(tag+" "), quote(command))
}

func quote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

var metricTimeout = 5 * time.Second

func compile(metrics []Metric) string {
	lines := []string{fmt.Sprintf("limit=; command -v timeout >/dev/null 2>&1 && limit='timeout %d'", int(metricTimeout.Seconds()))}
	for i, m := range metrics {
		lines = append(lines, capture(fmt.Sprintf("%d value", i), m.Value))
		if command := m.maxCommand(); command != "" {
			lines = append(lines, capture(fmt.Sprintf("%d max", i), command))
		}
	}
	return strings.Join(lines, "\n")
}

func Parse(out string, metrics []Metric) []Reading {
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

func Run(limit time.Duration, metrics []Metric, argv []string) ([]Reading, error) {
	if len(argv) == 0 {
		return nil, errors.New("stats: no command to run")
	}
	out, err := run.NonInteractive(limit, compile(metrics), argv[0], argv[1:]...)
	if err != nil {
		return nil, err
	}
	return Parse(out, metrics), nil
}
