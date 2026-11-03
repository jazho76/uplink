package inventory

import (
	"errors"
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
