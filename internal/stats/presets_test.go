package stats

import (
	"runtime"
	"slices"
	"strings"
	"testing"
)

func TestPresetsAreValid(t *testing.T) {
	for name, metrics := range presets {
		if len(metrics) == 0 {
			t.Errorf("%s: empty preset", name)
		}
		for _, m := range metrics {
			if err := m.Validate(); err != nil {
				t.Errorf("%s %s: %v", name, m.Label, err)
			}
		}
	}
}

func TestPresetReadsThisMachine(t *testing.T) {
	metrics := ForOS(runtime.GOOS)
	if metrics == nil {
		t.Skipf("no preset for %s", runtime.GOOS)
	}
	for _, r := range runLocally(t, metrics) {
		if !r.OK {
			t.Errorf("%s gave no reading", r.Metric.Label)
		}
	}
}

func TestExpandSplicesPresetsInPlace(t *testing.T) {
	gpu := Metric{Type: TypeGauge, Label: "gpu", Value: "gpu", Max: "100"}
	got, err := Expand([]Metric{gpu, {Preset: PresetLinux}, gpu})
	if err != nil {
		t.Fatal(err)
	}
	want := append(append([]Metric{gpu}, linux...), gpu)
	if !slices.Equal(got, want) {
		t.Errorf("want the preset spliced between the custom metrics, got %v", got)
	}
}

func TestExpandRejectsUnknownPresets(t *testing.T) {
	if _, err := Expand([]Metric{{Preset: "linx"}}); err == nil || !strings.Contains(err.Error(), `unknown preset "linx"`) {
		t.Errorf("want the unknown preset named, got %v", err)
	}
}

func TestExpandKeepsNoneApartFromUnset(t *testing.T) {
	if got, err := Expand(nil); got != nil || err != nil {
		t.Errorf("unset stays unset, got %v and %v", got, err)
	}
	if got, err := Expand([]Metric{}); got == nil || len(got) != 0 || err != nil {
		t.Errorf("an empty list stays empty, got %#v and %v", got, err)
	}
}

func TestPresetsAreCopies(t *testing.T) {
	metrics, _ := Preset(PresetLinux)
	metrics[0].Label = "changed"
	ForOS("linux")[0].Label = "changed"
	if linux[0].Label == "changed" {
		t.Error("callers must not be able to change a preset")
	}
}
