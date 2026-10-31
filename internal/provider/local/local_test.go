package local

import (
	"errors"
	"runtime"
	"slices"
	"testing"
	"time"

	"github.com/jazho76/uplink/internal/stats"
	"github.com/jazho76/uplink/internal/target"
)

func on(t *testing.T, os, host string, tools ...string) {
	t.Helper()
	previousOS, previousHost, previousLook := goos, hostname, lookPath
	t.Cleanup(func() { goos, hostname, lookPath = previousOS, previousHost, previousLook })

	goos = os
	hostname = func() (string, error) {
		if host == "" {
			return "", errors.New("no hostname")
		}
		return host, nil
	}
	lookPath = func(tool string) (string, error) {
		if !slices.Contains(tools, tool) {
			return "", errors.New("not found")
		}
		return "/usr/bin/" + tool, nil
	}
}

func spec(t *testing.T) target.Spec {
	t.Helper()
	specs, err := Provider{}.Specs()
	if err != nil || len(specs) != 1 {
		t.Fatalf("want one spec, got %v and %v", specs, err)
	}
	return specs[0]
}

func modeNames(s target.Spec) []string {
	var names []string
	for _, m := range s.Defaults.Modes {
		names = append(names, m.Name)
	}
	return names
}

func TestDescribesThisMachine(t *testing.T) {
	on(t, "linux", "fedora", "tmux", "journalctl")
	s := spec(t)
	if s.Name != "localhost" || s.Display != "fedora" {
		t.Errorf("want key localhost shown as fedora, got %q and %q", s.Name, s.Display)
	}
	if s.Defaults.Group != "machines" || !slices.Equal(s.Defaults.Stats, linux) {
		t.Errorf("want the machines group with linux stats, got %+v", s.Defaults)
	}
	if got := modeNames(s); !slices.Equal(got, []string{"tmux", "shell"}) {
		t.Errorf("want tmux then shell, got %v", got)
	}
	if s.Defaults.Logs != journal {
		t.Errorf("want the journal as logs, got %q", s.Defaults.Logs)
	}
}

func TestStatsFollowTheOS(t *testing.T) {
	for goos, want := range map[string][]stats.Metric{"darwin": macOS, "plan9": nil} {
		on(t, goos, "box")
		if got := spec(t).Defaults.Stats; !slices.Equal(got, want) {
			t.Errorf("%s: got %v", goos, got)
		}
	}
}

func TestMissingToolsDropTheirFeatures(t *testing.T) {
	on(t, "linux", "fedora")
	s := spec(t)
	if got := modeNames(s); !slices.Equal(got, []string{"shell"}) {
		t.Errorf("without tmux want shell only, got %v", got)
	}
	if s.Defaults.Logs != "" {
		t.Errorf("without journalctl want no logs, got %q", s.Defaults.Logs)
	}
}

func TestUnknownHostnameShowsTheKey(t *testing.T) {
	on(t, "linux", "")
	targets, err := target.Merge(nil, Provider{})
	if err != nil || len(targets) != 1 || targets[0].Name != "localhost" {
		t.Errorf("want a target named localhost, got %v and %v", targets, err)
	}
}

func TestPresetsAreValid(t *testing.T) {
	for goos, metrics := range map[string][]stats.Metric{"linux": linux, "darwin": macOS} {
		for _, m := range metrics {
			if err := m.Validate(); err != nil {
				t.Errorf("%s %s: %v", goos, m.Label, err)
			}
		}
	}
}

func TestPresetReadsThisMachine(t *testing.T) {
	metrics := preset(runtime.GOOS)
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
