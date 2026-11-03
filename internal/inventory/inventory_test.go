package inventory

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/jazho76/uplink/internal/inspection/multiplexer"
	"github.com/jazho76/uplink/internal/target"
)

type fakeProvider struct {
	name      string
	available bool
}

func (f fakeProvider) Name() string { return f.name }
func (f fakeProvider) Discover(context.Context) ([]Discovered, error) {
	if !f.available {
		return nil, ErrUnavailable
	}
	return []Discovered{{Status: target.StatusUp, Target: target.Target{
		ID:    target.ID{Key: "box"},
		Name:  f.name + "-box",
		Group: f.name,
		Modes: []target.Mode{{Name: "shell"}},
	}}}, nil
}

var providers = []Provider{fakeProvider{name: "near", available: true}, fakeProvider{name: "far"}, fakeProvider{name: "other", available: true}}

func load(t *testing.T, body string) ([]target.Target, error) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if body != "" {
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return Load(context.Background(), Catalog{Path: path, Providers: providers})
}

func ids(targets []target.Target) string {
	var out []string
	for _, t := range targets {
		out = append(out, t.ID.String())
	}
	return strings.Join(out, " ")
}

func TestEveryProviderRunsWithoutADiscoverSection(t *testing.T) {
	for name, body := range map[string]string{"missing file": "", "no section": "targets: []\n"} {
		targets, err := load(t, body)
		if err != nil || ids(targets) != "near/box other/box" {
			t.Errorf("%s: unavailable providers stay silent, got %q and %v", name, ids(targets), err)
		}
	}
}

func TestDiscoverListsExactlyWhatRuns(t *testing.T) {
	targets, err := load(t, "discover: [other]\n")
	if err != nil || ids(targets) != "other/box" {
		t.Errorf("want only other, got %q and %v", ids(targets), err)
	}
	targets, err = load(t, "discover: []\n")
	if err != nil || len(targets) != 0 {
		t.Errorf("an empty list turns discovery off, got %q and %v", ids(targets), err)
	}
}

func TestListedProviderThatIsUnavailableIsReported(t *testing.T) {
	targets, err := load(t, "discover: [far, near]\n")
	if ids(targets) != "near/box" {
		t.Errorf("others still run, got %q", ids(targets))
	}
	if err == nil || !strings.Contains(err.Error(), "far: unavailable") {
		t.Errorf("you asked for it by name, so it is reported, got %v", err)
	}
}

func TestUnknownProviderIsReported(t *testing.T) {
	targets, err := load(t, "discover: [nope, near]\n")
	if ids(targets) != "near/box" {
		t.Errorf("known providers still run, got %q", ids(targets))
	}
	if err == nil || !strings.Contains(err.Error(), `unknown provider "nope"`) {
		t.Errorf("want the unknown provider reported, got %v", err)
	}
}

func TestDiscoveredTargetsComeFirst(t *testing.T) {
	targets, err := load(t, "targets:\n  - { name: box, modes: [{ name: shell }] }\n")
	if err != nil || ids(targets) != "near/box other/box box" {
		t.Errorf("want discovered then config, got %q and %v", ids(targets), err)
	}
}

func TestDiscoverTakesProviderNamesOnly(t *testing.T) {
	targets, err := load(t, "discover:\n  - { provider: near }\n")
	if err == nil || ids(targets) != "near/box other/box" {
		t.Errorf("a mapping entry refuses the config, and providers still run, got %q and %v", ids(targets), err)
	}
}

type bareProvider struct{}

func (bareProvider) Name() string { return "bare" }
func (bareProvider) Discover(context.Context) ([]Discovered, error) {
	return []Discovered{{Target: target.Target{ID: target.ID{Key: "found"}}}}, nil
}

func loadBare(t *testing.T, body string) ([]target.Target, error) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return Load(context.Background(), Catalog{Path: path, Providers: []Provider{bareProvider{}}})
}

func TestDefaultModeReachesEveryTarget(t *testing.T) {
	targets, err := loadBare(t, "default_mode: shell\ntargets:\n  - name: mine\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 2 {
		t.Fatalf("want the discovered and the configured target, got %q", ids(targets))
	}
	for _, target := range targets {
		if got := target.Modes[0].Name; got != "shell" {
			t.Errorf("%s: want shell first, got %q", target.ID, got)
		}
	}
}

func TestUnknownDefaultModeIsReportedAndProvidersStillRun(t *testing.T) {
	targets, err := loadBare(t, "default_mode: nope\n")
	if ids(targets) != "bare/found" {
		t.Errorf("providers still run, got %q", ids(targets))
	}
	if err == nil || !strings.Contains(err.Error(), `unknown mode "nope"`) {
		t.Errorf("want the unknown mode reported, got %v", err)
	}
}

func from(name string, targets ...target.Target) Found {
	found := Found{Source: name}
	for _, t := range targets {
		found.Discovered = append(found.Discovered, Discovered{Target: t, Status: target.StatusUp})
	}
	return found
}

func merged(t *testing.T, defaultMode string, found ...Found) []target.Target {
	t.Helper()
	all, err := Merge(defaultMode, found...)
	if err != nil {
		t.Fatal(err)
	}
	return Targets(all)
}

var shellOnly = []target.Mode{{Name: "shell"}}

func keyed(key string) target.Target { return target.Target{ID: target.ID{Key: key}, Modes: shellOnly} }

func modeNames(t target.Target) []string {
	names := make([]string, len(t.Modes))
	for i, m := range t.Modes {
		names[i] = m.Name
	}
	return names
}

func TestSourcesMergeInOrderWithDistinctIDs(t *testing.T) {
	got := ids(merged(t, "", from(target.ConfigSource, keyed("box")), from("lima", keyed("box"), keyed("dev"))))
	if got != "box lima/box lima/dev" {
		t.Errorf("want config first, then lima in its order, got %q", got)
	}
}

func TestMergeKeepsStatus(t *testing.T) {
	all, err := Merge("", Found{Source: "lima", Discovered: []Discovered{{Target: keyed("dev"), Status: target.StatusDown}}})
	if err != nil || all[0].Status != target.StatusDown {
		t.Errorf("got %+v and %v", all, err)
	}
}

func TestMergeFillsWhatSourcesLeaveOut(t *testing.T) {
	targets := merged(t, "", from("lima",
		target.Target{ID: target.ID{Key: "dev"}, Name: "Dev box", Group: "work"},
		target.Target{ID: target.ID{Key: "bare"}},
	))
	if got := targets[0]; got.Name != "Dev box" || got.ID.Key != "dev" || got.Group != "work" {
		t.Errorf("a source's own values stay, got %+v", got)
	}
	if got := targets[1]; got.Name != "bare" || got.Group != target.DefaultGroup {
		t.Errorf("want name from key and the default group, got %+v", got)
	}
}

func TestFailingSourceOnlyLosesItsOwnTargets(t *testing.T) {
	all, err := Merge("", from(target.ConfigSource, keyed("host")), Found{Source: "lima", Err: errors.New("limactl not found")})
	if len(all) != 1 || all[0].Target.Name != "host" {
		t.Errorf("config targets should survive, got %v", all)
	}
	if err == nil || err.Error() != "lima: limactl not found" {
		t.Errorf("want the failure prefixed with its source, got %v", err)
	}
}

func TestDuplicateSourcesAndBrokenTargetsAreReported(t *testing.T) {
	all, err := Merge("", from("lima", keyed("bad name"), target.Target{Modes: shellOnly}), from("lima", keyed("y")))
	if len(all) != 0 {
		t.Errorf("want no targets, got %v", all)
	}
	for _, want := range []string{`target "lima/bad name": name may only hold`, "target lima #2: no name", `duplicate source "lima"`} {
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("errors should mention %q, got %v", want, err)
		}
	}
}

type fakeMultiplexer struct{}

func (fakeMultiplexer) Tool() string                    { return "fake" }
func (fakeMultiplexer) Attach() string                  { return "fake attach" }
func (fakeMultiplexer) Script() string                  { return "" }
func (fakeMultiplexer) Parse([]string) []target.Session { return nil }

func withFakeMultiplexer(t *testing.T) {
	t.Helper()
	previous := multiplexer.All
	t.Cleanup(func() { multiplexer.All = previous })
	multiplexer.All = []multiplexer.Multiplexer{multiplexer.Tmux{}, fakeMultiplexer{}}
}

func bareBox() Found { return from("test", target.Target{ID: target.ID{Key: "box"}}) }

func TestDefaultModeOrdersTheAutomaticModes(t *testing.T) {
	withFakeMultiplexer(t)
	for defaultMode, want := range map[string][]string{
		"":      {"tmux", "fake", "shell"},
		"tmux":  {"tmux", "fake", "shell"},
		"fake":  {"fake", "tmux", "shell"},
		"shell": {"shell", "tmux", "fake"},
	} {
		if got := modeNames(merged(t, defaultMode, bareBox())[0]); !slices.Equal(got, want) {
			t.Errorf("%q: want %v, got %v", defaultMode, want, got)
		}
	}
}

func TestZellijCanBeTheDefaultMode(t *testing.T) {
	if got := modeNames(merged(t, "zellij", bareBox())[0]); !slices.Equal(got, []string{"zellij", "tmux", "shell"}) {
		t.Errorf("want zellij, tmux, shell, got %v", got)
	}
}

func TestAutomaticModesNameTheirMultiplexer(t *testing.T) {
	for _, mode := range merged(t, "", bareBox())[0].Modes {
		want := mode.Name
		if mode.Name == shellMode {
			want = ""
		}
		if mode.Multiplexer != want || mode.Requires != want {
			t.Errorf("%s: want multiplexer and requires %q, got %+v", mode.Name, want, mode)
		}
	}
}

func TestDefaultModeLeavesExplicitModesAlone(t *testing.T) {
	withFakeMultiplexer(t)
	declared := target.Target{ID: target.ID{Key: "declared"}, Modes: []target.Mode{{Name: "root"}, {Name: "tmux"}}}
	if got := modeNames(merged(t, "tmux", from("test", declared))[0]); !slices.Equal(got, []string{"root", "tmux"}) {
		t.Errorf("explicit modes keep their order, got %v", got)
	}
}

func TestUnknownDefaultModeIsReportedAndTargetsStillLoad(t *testing.T) {
	all, err := Merge("zelij", bareBox())
	if err == nil || !strings.Contains(err.Error(), `unknown mode "zelij"`) || !strings.Contains(err.Error(), "tmux, zellij, shell") {
		t.Errorf("want the value and the choices reported, got %v", err)
	}
	if len(all) != 1 || modeNames(all[0].Target)[0] != "tmux" {
		t.Errorf("want the target with the implicit default, got %+v", all)
	}
}

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

func TestBadConfigStillRunsProviders(t *testing.T) {
	targets, err := load(t, "targets: [\n")
	if err == nil || ids(targets) != "near/box other/box" {
		t.Errorf("a bad config contributes nothing and providers still run, got %q and %v", ids(targets), err)
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

func TestUnknownProviderInAMatchIsReported(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	body := "discovered:\n  - { match: lim/nakama, hide: true }\n  - { match: \"*/x\", hide: true }\n  - { match: near/box, group: g }\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	plan := Catalog{Path: path, Providers: providers}.ReadPlan()
	if plan.Problems == nil || !strings.Contains(plan.Problems.Error(), `unknown provider "lim"`) {
		t.Errorf("a typo must not be silently ignored, got %v", plan.Problems)
	}
	if strings.Contains(plan.Problems.Error(), "near") || strings.Contains(plan.Problems.Error(), "*") {
		t.Errorf("only the typo is reported, got %v", plan.Problems)
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
