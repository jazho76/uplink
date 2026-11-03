package stats

import (
	"runtime"
	"testing"

	"github.com/jazho76/uplink/internal/target"
)

func TestPresetReadsThisMachine(t *testing.T) {
	metrics := target.BuiltinMetrics(runtime.GOOS)
	if len(metrics) == 0 {
		t.Skipf("no preset for %s", runtime.GOOS)
	}
	for _, r := range runLocally(t, metrics) {
		if !r.OK {
			t.Errorf("%s gave no reading", r.Metric.Label)
		}
	}
}
