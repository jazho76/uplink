package catalog

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/jazho76/uplink/internal/inventory"
	"github.com/jazho76/uplink/internal/target"
)

type fakeProvider struct {
	name      string
	available bool
}

func (f fakeProvider) Name() string { return f.name }
func (f fakeProvider) Discover(context.Context) ([]inventory.Discovered, error) {
	if !f.available {
		return nil, inventory.ErrUnavailable
	}
	return []inventory.Discovered{{Status: target.StatusUp, Target: target.Target{
		ID:    target.ID{Key: "box"},
		Name:  f.name + "-box",
		Group: f.name,
		Modes: []target.Mode{{Name: "shell"}},
	}}}, nil
}

var providers = []inventory.Provider{fakeProvider{name: "near", available: true}, fakeProvider{name: "far"}, fakeProvider{name: "other", available: true}}

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
func (bareProvider) Discover(context.Context) ([]inventory.Discovered, error) {
	return []inventory.Discovered{{Target: target.Target{ID: target.ID{Key: "found"}}}}, nil
}

func loadBare(t *testing.T, body string) ([]target.Target, error) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return Load(context.Background(), Catalog{Path: path, Providers: []inventory.Provider{bareProvider{}}})
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

func TestBadConfigStillRunsProviders(t *testing.T) {
	targets, err := load(t, "targets: [\n")
	if err == nil || ids(targets) != "near/box other/box" {
		t.Errorf("a bad config contributes nothing and providers still run, got %q and %v", ids(targets), err)
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

func discoveredBox(key string) inventory.Discovered {
	return inventory.Discovered{Status: target.StatusDown, Target: target.Target{ID: target.ID{Key: key}, Reach: target.Reach{Interactive: []string{"ssh", key}}}}
}

func overlaid(t *testing.T, body string) []inventory.Discovered {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	catalog := Catalog{Path: path, Providers: []inventory.Provider{fakeProvider{name: "lima", available: true}, fakeProvider{name: "tailscale", available: true}}}
	plan := catalog.ReadPlan()
	if plan.Problems != nil {
		t.Fatalf("got %v", plan.Problems)
	}
	current := inventory.Inventory{}.WithPlan(plan).
		WithResult("lima", []inventory.Discovered{discoveredBox("nakama"), discoveredBox("tokyo")}, nil).
		WithResult("tailscale", []inventory.Discovered{discoveredBox("berlin"), discoveredBox("phone")}, nil)
	merged, err := current.Merged()
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
	if got := ids(inventory.Targets(merged)); got != "lima/nakama lima/tokyo tailscale/berlin" {
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
	if got := ids(inventory.Targets(merged)); got != "mine" {
		t.Errorf("got %q", got)
	}
}

func TestBadConfigDropsOverlays(t *testing.T) {
	merged := overlaid(t, "discovered:\n  - { match: lima/nakama, hide: true }\n")
	if len(merged) != 3 {
		t.Fatalf("got %d", len(merged))
	}
	current := inventory.Inventory{}.WithPlan(inventory.Plan{Providers: []string{"lima"}, Overlays: nil, Invalid: true})
	current = current.WithResult("lima", []inventory.Discovered{discoveredBox("nakama")}, nil)
	if all, _ := current.Merged(); len(all) != 1 {
		t.Errorf("a config that can't be read contributes nothing, got %v", all)
	}
}

func modeNames(t target.Target) []string {
	names := make([]string, len(t.Modes))
	for i, mode := range t.Modes {
		names[i] = mode.Name
	}
	return names
}

type failingProvider struct{ err error }

func (failingProvider) Name() string { return "broken" }
func (f failingProvider) Discover(context.Context) ([]inventory.Discovered, error) {
	return nil, f.err
}

func TestTimeoutReasonReadsPlainly(t *testing.T) {
	providers := []inventory.Provider{failingProvider{fmt.Errorf("limactl list: %w", context.DeadlineExceeded)}}
	_, err := Catalog{Providers: providers}.Discover(context.Background(), "broken")

	current := inventory.Inventory{}.WithPlan(inventory.Plan{Providers: []string{"broken"}})
	current = current.WithResult("broken", []inventory.Discovered{{Target: target.Target{ID: target.ID{Key: "box"}, Modes: []target.Mode{{Name: "shell"}}}, Status: target.StatusUp}}, nil)
	current = current.WithResult("broken", nil, err)
	merged, problems := current.Merged()
	if len(merged) != 1 || merged[0].Status != target.StatusUnknown || merged[0].Reason != "timed out" {
		t.Errorf("want the last known target, unknown because it timed out, got %+v", merged)
	}
	if problems == nil || !strings.Contains(problems.Error(), "limactl list") {
		t.Errorf("the problem keeps the full error, got %v", problems)
	}
}

func TestUnavailableStaysUnavailable(t *testing.T) {
	providers := []inventory.Provider{failingProvider{inventory.ErrUnavailable}}
	_, err := Catalog{Providers: providers}.Discover(context.Background(), "broken")
	var failure inventory.Failure
	if !errors.Is(err, inventory.ErrUnavailable) || errors.As(err, &failure) {
		t.Errorf("unavailable is an answer, not a failure, got %v", err)
	}
}
