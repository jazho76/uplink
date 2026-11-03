package local

import (
	"context"
	"errors"
	"testing"

	"github.com/jazho76/uplink/internal/inventory"
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

func TestDescribesThisMachine(t *testing.T) {
	on(t, "fedora")
	found, err := Provider{}.Discover(context.Background())
	if err != nil || len(found) != 1 || found[0].Status != target.StatusUp {
		t.Fatalf("want one live target, got %v and %v", found, err)
	}
	merged, err := inventory.Merge("", inventory.Found{Source: name, Discovered: found})
	if err != nil || len(merged) != 1 {
		t.Fatalf("got %v and %v", merged, err)
	}
	got := merged[0].Target
	if got.ID.Key != "localhost" || got.Name != "fedora" || got.Group != "local" {
		t.Errorf("want key localhost shown as fedora in the local group, got %+v", got)
	}
	if got.Watch {
		t.Errorf("this machine is not watched, got %+v", got)
	}
	if !got.Logs.Journal || !got.Stats.Builtin {
		t.Errorf("want the journal and built-in stats, got %+v", got)
	}
}

func TestUnknownHostnameShowsTheKey(t *testing.T) {
	on(t, "")
	found, _ := Provider{}.Discover(context.Background())
	merged, err := inventory.Merge("", inventory.Found{Source: name, Discovered: found})
	if err != nil || len(merged) != 1 || merged[0].Target.Name != "localhost" {
		t.Errorf("want a target named localhost, got %v and %v", merged, err)
	}
}
