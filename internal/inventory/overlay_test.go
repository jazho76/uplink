package inventory

import (
	"testing"

	"github.com/jazho76/uplink/internal/target"
)

func TestOverlayMatches(t *testing.T) {
	for _, c := range []struct {
		match string
		id    target.ID
		want  bool
	}{
		{"lima/nakama", target.ID{Source: "lima", Key: "nakama"}, true},
		{"lima/nakama", target.ID{Source: "lima", Key: "tokyo"}, false},
		{"tailscale/*", target.ID{Source: "tailscale", Key: "berlin"}, true},
		{"tailscale/*", target.ID{Source: "lima", Key: "berlin"}, false},
		{"docker/heap*", target.ID{Source: "docker", Key: "heaplab-2.35"}, true},
		{"*/*", target.ID{Source: target.ConfigSource, Key: "mine"}, false},
	} {
		if got := (Overlay{Match: c.match}).Matches(c.id); got != c.want {
			t.Errorf("%q against %v: got %v", c.match, c.id, got)
		}
	}
}

func TestOverlaysApplyInOrder(t *testing.T) {
	hide, show := true, false
	group := func(name string) func(target.Target) target.Target {
		return func(t target.Target) target.Target {
			t.Group = name
			return t
		}
	}
	found := []Discovered{
		{Target: target.Target{ID: target.ID{Source: "lima", Key: "kept"}}},
		{Target: target.Target{ID: target.ID{Source: "lima", Key: "back"}}},
		{Target: target.Target{ID: target.ID{Source: "lima", Key: "gone"}}},
	}
	kept := applyOverlays(found, []Overlay{
		{Match: "lima/*", Adjust: group("first")},
		{Match: "lima/back", Hide: &hide},
		{Match: "lima/gone", Hide: &hide},
		{Match: "lima/back", Hide: &show},
		{Match: "lima/kept", Adjust: group("second")},
	})
	if len(kept) != 2 || kept[0].Target.ID.Key != "kept" || kept[1].Target.ID.Key != "back" {
		t.Fatalf("a later entry can bring a target back, got %+v", kept)
	}
	if kept[0].Target.Group != "second" || kept[1].Target.Group != "first" {
		t.Errorf("later entries win, got %+v", kept)
	}
}
