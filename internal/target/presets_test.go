package target

import "testing"

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

func TestPresetsAreCopies(t *testing.T) {
	metrics, _ := Preset(PresetLinux)
	metrics[0].Label = "changed"
	BuiltinMetrics("linux")[0].Label = "changed"
	if linuxMetrics[0].Label == "changed" {
		t.Error("callers must not be able to change a preset")
	}
}

func TestBuiltinMetricsFollowTheKernel(t *testing.T) {
	for _, kernel := range []string{"Linux", "Darwin", "linux", "darwin"} {
		if len(BuiltinMetrics(kernel)) == 0 {
			t.Errorf("%s: want metrics", kernel)
		}
	}
	if got := BuiltinMetrics("Plan9"); got == nil || len(got) != 0 {
		t.Errorf("an unknown kernel has no metrics but is not undetected, got %#v", got)
	}
}
