package target

import (
	"errors"
	"strings"
	"testing"
)

type stub struct {
	id      string
	targets []Target
	err     error
}

func (s stub) ID() string { return s.id }

func (s stub) List() ([]Target, error) { return s.targets, s.err }

func named(provider, name string) Target {
	return Target{Provider: provider, Name: name, Status: StatusRunning}
}

func TestAllKeepsProviderOrder(t *testing.T) {
	reg := NewRegistry(
		stub{id: "local", targets: []Target{named("local", "host")}},
		stub{id: "lima", targets: []Target{named("lima", "forge"), named("lima", "tokyo")}},
		stub{id: "remote", targets: []Target{named("remote", "dojo")}},
	)
	all, err := reg.All()
	if err != nil {
		t.Fatalf("all: %v", err)
	}
	var got []string
	for _, target := range all {
		got = append(got, target.Name)
	}
	if want := "host forge tokyo dojo"; strings.Join(got, " ") != want {
		t.Errorf("order = %q, want %q", strings.Join(got, " "), want)
	}
}

func TestAllReportsDuplicatesWithoutDroppingTheRest(t *testing.T) {
	reg := NewRegistry(
		stub{id: "lima", targets: []Target{named("lima", "dojo")}},
		stub{id: "remote", targets: []Target{named("remote", "dojo"), named("remote", "other")}},
	)
	all, err := reg.All()
	if err == nil {
		t.Fatal("a name collision must be reported")
	}
	if !strings.Contains(err.Error(), "dojo") {
		t.Errorf("error should name the collision, got %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("want the first claim plus the non-colliding target, got %d", len(all))
	}
	if all[0].Provider != "lima" || all[1].Name != "other" {
		t.Errorf("unexpected survivors: %+v", all)
	}
}

func TestAllSurvivesAFailingProvider(t *testing.T) {
	reg := NewRegistry(
		stub{id: "lima", err: errors.New("limactl exploded")},
		stub{id: "remote", targets: []Target{named("remote", "dojo")}},
	)
	all, err := reg.All()
	if err == nil || !strings.Contains(err.Error(), "limactl exploded") {
		t.Fatalf("provider failure should surface, got %v", err)
	}
	if len(all) != 1 || all[0].Name != "dojo" {
		t.Fatalf("healthy providers should still contribute, got %+v", all)
	}
}

func TestResolve(t *testing.T) {
	reg := NewRegistry(
		stub{id: "local", targets: []Target{named("local", "host")}},
		stub{id: "lima", targets: []Target{named("lima", "forge")}},
	)

	found, provider, err := reg.Resolve("forge")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if found.Name != "forge" || provider.ID() != "lima" {
		t.Errorf("resolved %q from %q", found.Name, provider.ID())
	}

	_, _, err = reg.Resolve("nope")
	if err == nil {
		t.Fatal("resolving an unknown target must fail")
	}
	for _, want := range []string{"nope", "forge", "host"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error should mention %q, got %v", want, err)
		}
	}
}

func TestModes(t *testing.T) {
	bare := Target{}
	if bare.DefaultMode().Name != "" {
		t.Errorf("a target with no modes has an empty default")
	}

	t2 := Target{Modes: []Mode{{Name: "tmux"}, {Name: "shell"}}}
	if t2.DefaultMode().Name != "tmux" {
		t.Errorf("the first mode is the default, got %q", t2.DefaultMode().Name)
	}
	if _, ok := t2.Mode("shell"); !ok {
		t.Errorf("shell should be found by name")
	}
	if _, ok := t2.Mode("nope"); ok {
		t.Errorf("unknown mode must not resolve")
	}
	if strings.Join(t2.ModeNames(), ",") != "tmux,shell" {
		t.Errorf("mode names = %v", t2.ModeNames())
	}
}
