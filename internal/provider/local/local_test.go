package local

import (
	"errors"
	"slices"
	"testing"

	"github.com/jazho76/uplink/internal/target"
)

func on(t *testing.T, host string, tools ...string) {
	t.Helper()
	previousHost, previousLook := hostname, lookPath
	t.Cleanup(func() { hostname, lookPath = previousHost, previousLook })

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

func TestDescribesThisMachine(t *testing.T) {
	on(t, "fedora", "journalctl")
	s := spec(t)
	if s.Name != "localhost" || s.Display != "fedora" || !s.Local {
		t.Errorf("want local key localhost shown as fedora, got %q and %q, local %v", s.Name, s.Display, s.Local)
	}
	if s.Defaults.Group != "local" {
		t.Errorf("want the local group, got %+v", s.Defaults)
	}
	if s.Defaults.Logs != journal {
		t.Errorf("want the journal as logs, got %q", s.Defaults.Logs)
	}
}

func TestMissingToolsDropTheirFeatures(t *testing.T) {
	on(t, "fedora")
	if got := spec(t).Defaults.Logs; got != "" {
		t.Errorf("without journalctl want no logs, got %q", got)
	}
}

func TestUnknownHostnameShowsTheKey(t *testing.T) {
	on(t, "")
	targets, err := target.Merge(Provider{})
	if err != nil || len(targets) != 1 || targets[0].Name != "localhost" {
		t.Errorf("want a target named localhost, got %v and %v", targets, err)
	}
}
