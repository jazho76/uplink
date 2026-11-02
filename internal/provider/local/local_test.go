package local

import (
	"errors"
	"testing"

	"github.com/jazho76/uplink/internal/target"
)

func on(t *testing.T, host string) {
	t.Helper()
	previous := hostname
	t.Cleanup(func() { hostname = previous })

	hostname = func() (string, error) {
		if host == "" {
			return "", errors.New("no hostname")
		}
		return host, nil
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
	on(t, "fedora")
	s := spec(t)
	if s.Name != "localhost" || s.Display != "fedora" || !s.Local {
		t.Errorf("want local key localhost shown as fedora, got %q and %q, local %v", s.Name, s.Display, s.Local)
	}
	if s.Defaults.Group != "local" {
		t.Errorf("want the local group, got %+v", s.Defaults)
	}
}

func TestLeavesTheInsideToTheCore(t *testing.T) {
	on(t, "fedora")
	targets, err := target.Merge(Provider{})
	if err != nil || len(targets) != 1 {
		t.Fatalf("want one target, got %v and %v", targets, err)
	}
	if got := targets[0]; !got.GuestLogs || !got.BuiltinStats {
		t.Errorf("logs and stats are the core's to decide, got %+v", got)
	}
}

func TestUnknownHostnameShowsTheKey(t *testing.T) {
	on(t, "")
	targets, err := target.Merge(Provider{})
	if err != nil || len(targets) != 1 || targets[0].Name != "localhost" {
		t.Errorf("want a target named localhost, got %v and %v", targets, err)
	}
}
