package probe

import (
	"cmp"
	"errors"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/jazho76/uplink/internal/run"
	"gopkg.in/yaml.v3"
)

const (
	TypeLoad   = "load"
	TypeMem    = "mem"
	TypeDisk   = "disk"
	TypeUptime = "uptime"
	TypeGauge  = "gauge"
	TypeText   = "text"

	UnitBytes = "bytes"
)

type Metric struct {
	Type  string `yaml:"type"`
	Label string `yaml:"label"`
	Path  string `yaml:"path"`
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
	TypeLoad:   {allowed: []string{"label"}},
	TypeMem:    {allowed: []string{"label"}},
	TypeDisk:   {allowed: []string{"label", "path"}},
	TypeUptime: {},
	TypeGauge:  {allowed: []string{"label", "value", "max", "unit"}, required: []string{"label", "value", "max"}},
	TypeText:   {allowed: []string{"label", "value"}, required: []string{"label", "value"}},
}

var (
	types      = []string{TypeLoad, TypeMem, TypeDisk, TypeUptime, TypeGauge, TypeText}
	fieldNames = []string{"label", "path", "value", "max", "unit"}
)

func (m *Metrics) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind == yaml.AliasNode {
		node = node.Alias
	}
	if node.Kind != yaml.SequenceNode {
		return errors.New("stats takes a list of metrics; use [] for none")
	}
	var list []Metric
	if err := node.Decode(&list); err != nil {
		return err
	}
	*m = append(Metrics{}, list...)
	return nil
}

func (m Metric) fields() map[string]string {
	return map[string]string{"label": m.Label, "path": m.Path, "value": m.Value, "max": m.Max, "unit": m.Unit}
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
	if m.Unit != "" && m.Unit != UnitBytes {
		return fmt.Errorf("unit must be %s", UnitBytes)
	}
	return nil
}

func (m Metric) Name() string {
	switch {
	case m.Label != "":
		return m.Label
	case m.Type == TypeMem:
		return "memory"
	case m.Type == TypeDisk && m.Path != "" && m.Path != "/":
		return m.Path
	default:
		return m.Type
	}
}

func (m Metric) Gauge() bool { return m.Type != TypeUptime && m.Type != TypeText }

func (m Metric) Bytes() bool {
	return m.Type == TypeMem || m.Type == TypeDisk || m.Unit == UnitBytes
}

func (m Metric) constantMax() (float64, bool) {
	v, err := strconv.ParseFloat(strings.TrimSpace(m.Max), 64)
	return v, err == nil && !math.IsNaN(v)
}

func (m Metric) commands() (value, ceiling string) {
	switch m.Type {
	case TypeLoad:
		return "cut -d ' ' -f 1 /proc/loadavg", "nproc"
	case TypeMem:
		return `awk '/^MemTotal:/{t=$2} /^MemAvailable:/{a=$2} END{print (t-a)*1024}' /proc/meminfo`,
			`awk '/^MemTotal:/{print $2*1024}' /proc/meminfo`
	case TypeDisk:
		df := "df -P -k " + quote(cmp.Or(m.Path, "/"))
		return df + " | awk 'NR==2{print $3*1024}'", df + " | awk 'NR==2{print $2*1024}'"
	case TypeUptime:
		return "cut -d ' ' -f 1 /proc/uptime", ""
	}
	if _, constant := m.constantMax(); constant {
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

func Compile(metrics Metrics) string {
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
		case m.Type == TypeText:
			r.OK = r.Text != ""
		case m.Type == TypeUptime:
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

func (r Reading) Uptime() time.Duration {
	return time.Duration(r.Value) * time.Second
}

func Run(limit time.Duration, metrics Metrics, transport ...string) ([]Reading, error) {
	if len(transport) == 0 {
		return nil, errors.New("probe: no transport given")
	}
	out, err := run.NonInteractive(limit, Compile(metrics), transport[0], transport[1:]...)
	if err != nil {
		return nil, err
	}
	return Parse(out, metrics), nil
}
