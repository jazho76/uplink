package presets

import (
	"runtime"
	"testing"
	"time"

	"github.com/jazho76/uplink/internal/stats"
)

func TestPresetsAreValid(t *testing.T) {
	for goos, metrics := range map[string][]stats.Metric{"linux": Linux, "darwin": MacOS} {
		for _, m := range metrics {
			if err := m.Validate(); err != nil {
				t.Errorf("%s %s: %v", goos, m.Label, err)
			}
		}
	}
}

func TestPresetReadsThisMachine(t *testing.T) {
	metrics := For(runtime.GOOS)
	if metrics == nil {
		t.Skipf("no preset for %s", runtime.GOOS)
	}
	readings, err := stats.Run(10*time.Second, metrics, []string{"sh"})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range readings {
		if !r.OK {
			t.Errorf("%s gave no reading", r.Metric.Label)
		}
	}
}
