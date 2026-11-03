package inventory

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/jazho76/uplink/internal/target"
)

var plan = Plan{Providers: []string{"lima"}}

func vmFound(status target.Status) []Discovered {
	return []Discovered{{Target: keyed("dev"), Status: status}}
}

func statuses(t *testing.T, i Inventory) []target.Status {
	t.Helper()
	merged, _ := i.Merged()
	var out []target.Status
	for _, d := range merged {
		out = append(out, d.Status)
	}
	return out
}

func TestAnsweredWaitsForEveryPlannedProvider(t *testing.T) {
	two := Inventory{}.WithPlan(Plan{Providers: []string{"lima", "docker"}})
	if two.Answered() {
		t.Error("nothing has answered yet")
	}
	if two = two.WithResult("lima", nil, nil); two.Answered() {
		t.Error("docker has not answered")
	}
	if two = two.WithResult("docker", nil, ErrUnavailable); !two.Answered() {
		t.Error("unavailable is an answer")
	}
}

func TestFailureKeepsTargetsWithUnknownStatus(t *testing.T) {
	boom := errors.New("timed out")
	i := Inventory{}.WithPlan(plan).WithResult("lima", vmFound(target.StatusUp), nil).WithResult("lima", nil, boom)
	if got := statuses(t, i); !slices.Equal(got, []target.Status{target.StatusUnknown}) {
		t.Errorf("uplink could not find out, got %v", got)
	}
	if _, err := i.Merged(); !errors.Is(err, boom) {
		t.Errorf("the failure is reported, got %v", err)
	}
	recovered := i.WithResult("lima", vmFound(target.StatusDown), nil)
	if got := statuses(t, recovered); !slices.Equal(got, []target.Status{target.StatusDown}) {
		t.Errorf("got %v", got)
	}
	if _, err := recovered.Merged(); err != nil {
		t.Errorf("recovering clears the problem, got %v", err)
	}
}

func TestUnavailableClearsTargetsSilentlyUnlessListed(t *testing.T) {
	seen := Inventory{}.WithPlan(plan).WithResult("lima", vmFound(target.StatusUp), nil)
	gone := seen.WithResult("lima", nil, ErrUnavailable)
	merged, err := gone.Merged()
	if len(merged) != 0 || err != nil {
		t.Errorf("want nothing and no complaint, got %v and %v", merged, err)
	}

	listed := Inventory{}.WithPlan(Plan{Providers: []string{"lima"}, Explicit: true}).WithResult("lima", nil, ErrUnavailable)
	if _, err := listed.Merged(); !errors.Is(err, ErrUnavailable) {
		t.Errorf("you asked for it by name, got %v", err)
	}
}

func TestResultsForUnplannedProvidersAreDropped(t *testing.T) {
	i := Inventory{}.WithPlan(plan).WithResult("docker", vmFound(target.StatusUp), nil)
	if merged, _ := i.Merged(); len(merged) != 0 {
		t.Errorf("got %v", merged)
	}
}

func TestPlanKeepsResultsOfProvidersItStillNames(t *testing.T) {
	i := Inventory{}.WithPlan(plan).WithResult("lima", vmFound(target.StatusUp), nil)
	if merged, _ := i.WithPlan(plan).Merged(); len(merged) != 1 {
		t.Errorf("a reload keeps what is known, got %v", merged)
	}
	if merged, _ := i.WithPlan(Plan{}).Merged(); len(merged) != 0 {
		t.Errorf("a provider no longer planned loses its targets, got %v", merged)
	}
}

func TestInvalidPlanKeepsTheLastGoodProviders(t *testing.T) {
	i := Inventory{}.WithPlan(plan).WithResult("lima", vmFound(target.StatusUp), nil)
	broken := i.WithPlan(Plan{Providers: []string{"lima", "docker"}, Invalid: true, Problems: errors.New("bad yaml")})
	if got := broken.Providers(); !slices.Equal(got, []string{"lima"}) {
		t.Errorf("a typo must not re-enable disabled providers, got %v", got)
	}
	merged, err := broken.Merged()
	if len(merged) != 1 || err == nil {
		t.Errorf("targets stay and the problem shows, got %v and %v", merged, err)
	}
}

func TestInvalidPlanKeepsDiscoveryOff(t *testing.T) {
	off := Inventory{}.WithPlan(Plan{Explicit: true})
	broken := off.WithPlan(Plan{Providers: []string{"lima"}, Invalid: true})
	if got := broken.Providers(); len(got) != 0 {
		t.Errorf("discover: [] followed by a typo stays off, got %v", got)
	}
}

func discoveredBox(key string) Discovered {
	return Discovered{Status: target.StatusDown, Target: target.Target{ID: target.ID{Key: key}, Reach: target.Reach{Interactive: []string{"ssh", key}}}}
}

func overlaid(t *testing.T, body string) []Discovered {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	catalog := Catalog{Path: path, Providers: []Provider{fakeProvider{name: "lima", available: true}, fakeProvider{name: "tailscale", available: true}}}
	plan := catalog.ReadPlan()
	if plan.Problems != nil {
		t.Fatalf("got %v", plan.Problems)
	}
	inventory := Inventory{}.WithPlan(plan).
		WithResult("lima", []Discovered{discoveredBox("nakama"), discoveredBox("tokyo")}, nil).
		WithResult("tailscale", []Discovered{discoveredBox("berlin"), discoveredBox("phone")}, nil)
	merged, err := inventory.Merged()
	if err != nil {
		t.Fatal(err)
	}
	return merged
}

func TestHiddenTargetsLeaveAndLaterEntriesCanBringThemBack(t *testing.T) {
	merged := overlaid(t, `
discovered:
  - { match: tailscale/*, hide: true }
  - { match: tailscale/berlin, hide: false }
`)
	if got := ids(Targets(merged)); got != "lima/nakama lima/tokyo tailscale/berlin" {
		t.Errorf("got %q", got)
	}
}

func TestLaterEntriesWinFieldByField(t *testing.T) {
	merged := overlaid(t, `
discovered:
  - { match: lima/*, group: all, logs: "first" }
  - { match: lima/nakama, group: mine }
`)
	nakama, tokyo := merged[0].Target, merged[1].Target
	if nakama.Group != "mine" || nakama.Logs.Command != "first" {
		t.Errorf("the later group wins and the earlier logs stay, got %+v", nakama)
	}
	if tokyo.Group != "all" {
		t.Errorf("got %+v", tokyo)
	}
}

func TestOverlaysKeepStatusAndModesKeepDefaults(t *testing.T) {
	merged := overlaid(t, `
default_mode: shell
discovered:
  - { match: lima/nakama, group: work }
  - { match: lima/tokyo, modes: [{ name: mine }] }
`)
	if merged[0].Status != target.StatusDown {
		t.Errorf("status stays the provider's, got %+v", merged[0])
	}
	if got := merged[0].Target.Modes[0].Name; got != "shell" {
		t.Errorf("an adjusted target still gets default modes, got %q", got)
	}
	if got := modeNames(merged[1].Target); !slices.Equal(got, []string{"mine"}) {
		t.Errorf("declared modes replace them, got %v", got)
	}
	if argv := merged[1].Target.Modes[0].Argv; argv[0] != "ssh" || argv[1] != "tokyo" {
		t.Errorf("modes go through the discovered reach, got %q", argv)
	}
}

func TestOverlaysLeaveDeclaredTargetsAlone(t *testing.T) {
	merged := overlaid(t, `
targets:
  - { name: mine, group: own }
discovered:
  - { match: "*/*", hide: true }
`)
	if got := ids(Targets(merged)); got != "mine" {
		t.Errorf("got %q", got)
	}
}

func TestBadConfigDropsOverlays(t *testing.T) {
	merged := overlaid(t, "discovered:\n  - { match: lima/nakama, hide: true }\n")
	if len(merged) != 3 {
		t.Fatalf("got %d", len(merged))
	}
	inventory := Inventory{}.WithPlan(Plan{Providers: []string{"lima"}, Overlays: nil, Invalid: true})
	inventory = inventory.WithResult("lima", []Discovered{discoveredBox("nakama")}, nil)
	if all, _ := inventory.Merged(); len(all) != 1 {
		t.Errorf("a config that can't be read contributes nothing, got %v", all)
	}
}
