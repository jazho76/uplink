package stats

import (
	"runtime"
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

func TestPresetsAreCopies(t *testing.T) {
	metrics, _ := Preset(PresetLinux)
	metrics[0].Label = "changed"
	ForOS("linux")[0].Label = "changed"
	if linux[0].Label == "changed" {
		t.Error("callers must not be able to change a preset")
	}
}
