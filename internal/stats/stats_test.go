package stats

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/jazho76/uplink/internal/process"
	"github.com/jazho76/uplink/internal/shell"
	"github.com/jazho76/uplink/internal/target"
)

var metricTimeout = 5 * time.Second

func TestMain(m *testing.M) {
	os.Setenv("SHELL", "/nonexistent/login-shell")
	os.Exit(m.Run())
}

func runLocally(t *testing.T, metrics []target.Metric) []target.Reading {
	t.Helper()
	out, err := process.NonInteractive(context.Background(), 10*time.Second, shell.Preamble(metricTimeout)+"\n"+Script(metrics), "sh")
	if err != nil {
		t.Fatalf("stats: %v", err)
	}
	readings := Parse(out, metrics)
	if len(readings) != len(metrics) {
		t.Fatalf("one reading per metric, got %d for %d", len(readings), len(metrics))
	}
	return readings
}

func TestUnitTextReadsANumber(t *testing.T) {
	readings := runLocally(t, []target.Metric{
		{Type: target.TypeText, Label: "up", Value: "echo 3600", Unit: target.UnitSeconds},
		{Type: target.TypeText, Label: "up", Value: "echo soon", Unit: target.UnitSeconds},
	})
	if r := readings[0]; !r.OK || r.Value != 3600 {
		t.Errorf("text with a unit should read its output as a number, got %+v", r)
	}
	if r := readings[1]; r.OK {
		t.Errorf("text with a unit and no number is missing, got %+v", r)
	}
}

func TestCustomMetrics(t *testing.T) {
	readings := runLocally(t, []target.Metric{
		{Type: target.TypeGauge, Label: "fixed", Value: "echo 37", Max: "100"},
		{Type: target.TypeGauge, Label: "computed", Value: "echo 3", Max: "echo 4"},
		{Type: target.TypeText, Label: "note", Value: `printf 'it'\''s \\n fine\nsecond line\n'`},
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
	readings := runLocally(t, []target.Metric{
		{Type: target.TypeGauge, Label: "fails", Value: "exit 1", Max: "10"},
		{Type: target.TypeText, Label: "greedy", Value: "cat"},
		{Type: target.TypeGauge, Label: "words", Value: "echo many", Max: "10"},
		{Type: target.TypeText, Label: "up", Value: "exit 1", Unit: target.UnitSeconds},
		{Type: target.TypeText, Label: "after", Value: "echo still here"},
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
	assertMetricsIsolated(t)
}

func TestMetricsAreIsolatedWithoutTimeoutOnMacOSShell(t *testing.T) {
	dir := t.TempDir()
	for name, tool := range map[string]string{"sh": "bash", "head": "head", "sleep": "sleep"} {
		path, err := exec.LookPath(tool)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(path, filepath.Join(dir, name)); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir)

	assertMetricsIsolated(t)
}

func assertMetricsIsolated(t *testing.T) {
	t.Helper()
	previous := metricTimeout
	metricTimeout = time.Second
	t.Cleanup(func() { metricTimeout = previous })

	start := time.Now()
	readings := runLocally(t, []target.Metric{
		{Type: target.TypeText, Label: "syntax", Value: "echo ( unbalanced"},
		{Type: target.TypeText, Label: "spoof", Value: "echo first; echo '3 value 999'"},
		{Type: target.TypeGauge, Label: "hang", Value: "sleep 30; echo 1", Max: "2"},
		{Type: target.TypeGauge, Label: "victim", Value: "echo 1", Max: "2"},
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

func TestHangingLoginProfileOnlyCostsTheLimit(t *testing.T) {
	previous := metricTimeout
	metricTimeout = time.Second
	t.Cleanup(func() { metricTimeout = previous })

	shell := filepath.Join(t.TempDir(), "login-shell")
	if err := os.WriteFile(shell, []byte("#!/bin/sh\nsleep 30\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SHELL", shell)

	start := time.Now()
	readings := runLocally(t, []target.Metric{{Type: target.TypeGauge, Label: "after", Value: "echo 1", Max: "2"}})
	if r := readings[0]; !r.OK || r.Value != 1 {
		t.Errorf("metrics should survive a hanging profile, got %+v", r)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("a hanging profile held the probe for %s", elapsed)
	}
}

func TestParseIgnoresNoise(t *testing.T) {
	metrics := []target.Metric{{Type: target.TypeGauge, Label: "load", Value: "x", Max: "x"}, {Type: target.TypeText, Label: "up", Value: "x", Unit: target.UnitSeconds}}
	readings := Parse("motd banner\n0 value 0.5\n7 value 1\nx max 2\n0 max 4\n1 value nope\n", metrics)
	if r := readings[0]; !r.OK || r.Value != 0.5 || r.Max != 4 {
		t.Errorf("load = %+v", r)
	}
	if readings[1].OK {
		t.Errorf("an unparsable value is missing, got %+v", readings[1])
	}
}

func TestTextIsCleanedAtTheBoundary(t *testing.T) {
	metrics := []target.Metric{{Type: target.TypeText, Label: "t", Value: "x"}}
	readings := Parse("0 value \x1b]0;pwned\x07hi\x1b[31m there\n", metrics)
	if got := readings[0].Text; got != "hi there" {
		t.Errorf("got %q", got)
	}
}

func TestValuesThatCannotBeShownAreMissing(t *testing.T) {
	gauge := target.Metric{Type: target.TypeGauge, Label: "g", Value: "x", Max: "10"}
	bytes := target.Metric{Type: target.TypeGauge, Label: "b", Value: "x", Max: "10", Unit: target.UnitBytes}
	up := target.Metric{Type: target.TypeText, Label: "u", Value: "x", Unit: target.UnitSeconds}
	for _, c := range []struct {
		metric target.Metric
		value  string
		ok     bool
	}{
		{gauge, "5", true},
		{gauge, "-1", true},
		{gauge, "NaN", false},
		{gauge, "nan", false},
		{gauge, "Inf", false},
		{gauge, "-Inf", false},
		{gauge, "1e999", false},
		{bytes, "-1", false},
		{bytes, "1024", true},
		{up, "-5", false},
		{up, "NaN", false},
		{up, "60", true},
	} {
		r := Parse("0 value "+c.value+"\n", []target.Metric{c.metric})[0]
		if r.OK != c.ok {
			t.Errorf("%s %q: want ok %v, got %+v", c.metric.Label, c.value, c.ok, r)
		}
	}

	computed := target.Metric{Type: target.TypeGauge, Label: "g", Value: "x", Max: "y"}
	for _, ceiling := range []string{"NaN", "Inf"} {
		if r := Parse("0 value 1\n0 max "+ceiling+"\n", []target.Metric{computed})[0]; r.OK {
			t.Errorf("max %q should be missing, got %+v", ceiling, r)
		}
	}
	constantInf := target.Metric{Type: target.TypeGauge, Label: "g", Value: "x", Max: "Inf"}
	if _, constant := constantMax(constantInf); constant {
		t.Error("Inf is no constant to divide by")
	}
}

func TestBackgroundedMetricCannotStallTheScript(t *testing.T) {
	previous := metricTimeout
	metricTimeout = time.Second
	t.Cleanup(func() { metricTimeout = previous })

	start := time.Now()
	readings := runLocally(t, []target.Metric{
		{Type: target.TypeText, Label: "silent", Value: "sleep 30 &"},
		{Type: target.TypeText, Label: "chatty", Value: "sleep 30 & echo early"},
		{Type: target.TypeText, Label: "after", Value: "echo fine"},
	})
	if readings[0].OK {
		t.Errorf("nothing came out, got %+v", readings[0])
	}
	if r := readings[1]; !r.OK || r.Text != "early" {
		t.Errorf("got %+v", r)
	}
	if r := readings[2]; !r.OK || r.Text != "fine" {
		t.Errorf("got %+v", r)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("a backgrounded child held the probe for %s", elapsed)
	}
}
