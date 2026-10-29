package probe

import (
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func runLocally(t *testing.T, metrics Metrics) []Reading {
	t.Helper()
	readings, err := Run(10*time.Second, metrics, "sh")
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	if len(readings) != len(metrics) {
		t.Fatalf("one reading per metric, got %d for %d", len(readings), len(metrics))
	}
	return readings
}

func TestBuiltinsReportThisMachine(t *testing.T) {
	readings := runLocally(t, Metrics{{Type: TypeLoad}, {Type: TypeMem}, {Type: TypeDisk}, {Type: TypeDisk, Path: "/tmp"}, {Type: TypeUptime}})
	load, mem, root, tmp, uptime := readings[0], readings[1], readings[2], readings[3], readings[4]

	if !load.OK || load.Max < 1 {
		t.Errorf("load should carry the core count as its max, got %+v", load)
	}
	if !mem.OK || mem.Max == 0 || mem.Value > mem.Max {
		t.Errorf("memory should be used out of total, got %+v", mem)
	}
	if !root.OK || root.Max == 0 || !tmp.OK || tmp.Max == 0 {
		t.Errorf("disk should measure its path, got %+v and %+v", root, tmp)
	}
	if !uptime.OK || uptime.Uptime() <= 0 {
		t.Errorf("uptime should be positive, got %+v", uptime)
	}
}

func TestCustomMetrics(t *testing.T) {
	readings := runLocally(t, Metrics{
		{Type: TypeGauge, Label: "fixed", Value: "echo 37", Max: "100"},
		{Type: TypeGauge, Label: "computed", Value: "echo 3", Max: "echo 4"},
		{Type: TypeText, Label: "note", Value: `printf 'it'\''s \\n fine\nsecond line\n'`},
	})
	if r := readings[0]; !r.OK || r.Value != 37 || r.Max != 100 || r.Fraction() != 0.37 {
		t.Errorf("a numeric max is a constant, got %+v", r)
	}
	if r := readings[1]; !r.OK || r.Value != 3 || r.Max != 4 {
		t.Errorf("a non-numeric max runs as a command, got %+v", r)
	}
	if r := readings[2]; !r.OK || r.Text != `it's \n fine` {
		t.Errorf("text keeps its first line verbatim, got %q", r.Text)
	}
}

func TestBrokenMetricsDoNotSpoilTheRest(t *testing.T) {
	readings := runLocally(t, Metrics{
		{Type: TypeGauge, Label: "fails", Value: "exit 1", Max: "10"},
		{Type: TypeText, Label: "greedy", Value: "cat"},
		{Type: TypeGauge, Label: "words", Value: "echo many", Max: "10"},
		{Type: TypeDisk, Path: "/no/such/path"},
		{Type: TypeText, Label: "after", Value: "echo still here"},
	})
	for i, r := range readings[:4] {
		if r.OK {
			t.Errorf("metric %d should be missing, got %+v", i, r)
		}
	}
	if r := readings[4]; !r.OK || r.Text != "still here" {
		t.Errorf("a metric reading stdin must not swallow the rest of the script, got %+v", r)
	}
}

func TestMetricsAreIsolatedFromEachOther(t *testing.T) {
	previous := metricTimeout
	metricTimeout = time.Second
	t.Cleanup(func() { metricTimeout = previous })

	start := time.Now()
	readings := runLocally(t, Metrics{
		{Type: TypeText, Label: "syntax", Value: "echo ( unbalanced"},
		{Type: TypeText, Label: "spoof", Value: "echo first; echo '3 value 999'"},
		{Type: TypeGauge, Label: "hang", Value: "sleep 30; echo 1", Max: "2"},
		{Type: TypeGauge, Label: "victim", Value: "echo 1", Max: "2"},
	})
	if readings[0].OK {
		t.Errorf("a syntax error is the metric's own problem, got %+v", readings[0])
	}
	if r := readings[1]; !r.OK || r.Text != "first" {
		t.Errorf("only the first line counts, got %+v", r)
	}
	if readings[2].OK {
		t.Errorf("a hanging metric should time out, got %+v", readings[2])
	}
	if r := readings[3]; !r.OK || r.Value != 1 {
		t.Errorf("other metrics must not be spoofed or stalled, got %+v", r)
	}
	if elapsed := time.Since(start); elapsed > 10*time.Second {
		t.Errorf("a hanging metric held the probe for %s", elapsed)
	}
}

func TestParseIgnoresNoise(t *testing.T) {
	metrics := Metrics{{Type: TypeLoad}, {Type: TypeUptime}}
	readings := Parse("motd banner\n0 value 0.5\n7 value 1\nx max 2\n0 max 4\n1 value nope\n", metrics)
	if r := readings[0]; !r.OK || r.Value != 0.5 || r.Max != 4 {
		t.Errorf("load = %+v", r)
	}
	if readings[1].OK {
		t.Errorf("an unparsable value is missing, got %+v", readings[1])
	}
}

func TestValidate(t *testing.T) {
	for _, m := range []Metric{
		{Type: TypeLoad}, {Type: TypeMem, Label: "ram"}, {Type: TypeDisk, Path: "/srv"}, {Type: TypeUptime},
		{Type: TypeGauge, Label: "gpu", Value: "x", Max: "100", Unit: UnitBytes},
		{Type: TypeText, Label: "agents", Value: "x"},
	} {
		if err := m.Validate(); err != nil {
			t.Errorf("%+v should be valid: %v", m, err)
		}
	}

	for want, m := range map[string]Metric{
		"type must be one of":   {Type: "cpu"},
		"load takes no path":    {Type: TypeLoad, Path: "/"},
		"uptime takes no label": {Type: TypeUptime, Label: "up"},
		"gauge needs a max":     {Type: TypeGauge, Label: "gpu", Value: "x"},
		"gauge needs a label":   {Type: TypeGauge, Value: "x", Max: "1"},
		"text needs a value":    {Type: TypeText, Label: "x"},
		"text takes no max":     {Type: TypeText, Label: "x", Value: "y", Max: "1"},
		"unit must be bytes":    {Type: TypeGauge, Label: "x", Value: "y", Max: "1", Unit: "percent"},
	} {
		if err := m.Validate(); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%+v: want %q, got %v", m, want, err)
		}
	}
}

func TestNames(t *testing.T) {
	for want, m := range map[string]Metric{
		"load": {Type: TypeLoad}, "memory": {Type: TypeMem}, "disk": {Type: TypeDisk, Path: "/"},
		"/srv": {Type: TypeDisk, Path: "/srv"}, "ram": {Type: TypeMem, Label: "ram"},
	} {
		if got := m.Name(); got != want {
			t.Errorf("%+v: name %q, want %q", m, got, want)
		}
	}
}

func TestMetricsDecoding(t *testing.T) {
	var spec struct {
		Stats Metrics `yaml:"stats"`
	}
	if err := yaml.Unmarshal([]byte("stats: []"), &spec); err != nil || spec.Stats == nil || len(spec.Stats) != 0 {
		t.Errorf("an empty list means none, distinct from absent: %v %#v", err, spec.Stats)
	}
	if err := yaml.Unmarshal([]byte("stats: false"), &spec); err == nil || !strings.Contains(err.Error(), "use [] for none") {
		t.Errorf("the old boolean should point at the new form, got %v", err)
	}
	if err := yaml.Unmarshal([]byte("x: &l [{ type: load }]\nstats: *l"), &spec); err != nil || len(spec.Stats) != 1 {
		t.Errorf("an alias should decode: %v %#v", err, spec.Stats)
	}
}
