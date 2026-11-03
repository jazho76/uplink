package inventory

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
