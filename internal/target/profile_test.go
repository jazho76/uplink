package target

import (
	"slices"
	"testing"
)

func TestDerivedFillsOnlyDetectedMetrics(t *testing.T) {
	declared := []Metric{{Type: TypeText, Label: "mine", Value: "x"}}
	linux := Profile{OS: "Linux"}
	for name, c := range map[string]struct {
		metrics []Metric
		profile Profile
		want    []Metric
	}{
		"known kernel":    {DetectedMetrics(), linux, linuxMetrics},
		"darwin":          {DetectedMetrics(), Profile{OS: "Darwin"}, macosMetrics},
		"unknown kernel":  {DetectedMetrics(), Profile{OS: "Plan9"}, []Metric{}},
		"unknown os":      {DetectedMetrics(), Profile{OS: UnknownOS}, []Metric{}},
		"declared stays":  {declared, linux, declared},
		"stats off stays": {[]Metric{}, linux, []Metric{}},
	} {
		got := Target{Metrics: c.metrics}.Derived(c.profile)
		if !slices.Equal(got.Metrics, c.want) {
			t.Errorf("%s: want %+v, got %+v", name, c.want, got.Metrics)
		}
		if got.DetectsMetrics() {
			t.Errorf("%s: a known OS settles it, got nil", name)
		}
	}
	if got := (Target{}).Derived(Profile{}); !got.DetectsMetrics() {
		t.Errorf("no profile yet stays undetected, got %+v", got.Metrics)
	}
}
