package inventory

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jazho76/uplink/internal/target"
)

type fakeProvider struct {
	name string
	on   bool
}

func (f fakeProvider) Name() string  { return f.name }
func (f fakeProvider) Default() bool { return f.on }
func (f fakeProvider) Specs() ([]target.Spec, error) {
	return []target.Spec{{
		Name:     "box",
		Display:  f.name + "-box",
		Defaults: &target.Spec{Group: f.name, Modes: []target.ModeSpec{{Name: "shell"}}},
	}}, nil
}

var providers = []Provider{fakeProvider{name: "near", on: true}, fakeProvider{name: "far"}}

func load(t *testing.T, body string) ([]target.Target, error) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if body != "" {
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return Load(path, providers)
}

func ids(targets []target.Target) string {
	var out []string
	for _, t := range targets {
		out = append(out, t.ID.String())
	}
	return strings.Join(out, " ")
}

func TestDefaultProvidersRunWithoutADiscoverSection(t *testing.T) {
	for name, body := range map[string]string{"missing file": "", "no section": "profiles: {}\n"} {
		targets, err := load(t, body)
		if err != nil || ids(targets) != "near/box" {
			t.Errorf("%s: want only the default provider, got %q and %v", name, ids(targets), err)
		}
	}
}

func TestDiscoverListsExactlyWhatRuns(t *testing.T) {
	targets, err := load(t, "discover:\n  - provider: far\n")
	if err != nil || ids(targets) != "far/box" {
		t.Errorf("want only far, got %q and %v", ids(targets), err)
	}
	targets, err = load(t, "discover: []\n")
	if err != nil || len(targets) != 0 {
		t.Errorf("an empty list turns discovery off, got %q and %v", ids(targets), err)
	}
}

func TestUnknownProviderIsReported(t *testing.T) {
	targets, err := load(t, "discover:\n  - provider: nope\n  - provider: near\n")
	if ids(targets) != "near/box" {
		t.Errorf("known providers still run, got %q", ids(targets))
	}
	if err == nil || !strings.Contains(err.Error(), `unknown provider "nope"`) {
		t.Errorf("want the unknown provider reported, got %v", err)
	}
}

func TestEntryProfileOverridesDefaultsNotFacts(t *testing.T) {
	targets, err := load(t, `
profiles:
  mine: { group: work, modes: [{ name: tmux }] }
discover:
  - { provider: near, profile: mine }
`)
	if err != nil || len(targets) != 1 {
		t.Fatalf("want one target, got %v and %v", targets, err)
	}
	got := targets[0]
	if got.Group != "work" || got.Modes[0].Name != "tmux" {
		t.Errorf("the profile should beat the provider defaults, got %q and %q", got.Group, got.Modes[0].Name)
	}
	if got.ID.String() != "near/box" || got.Name != "near-box" {
		t.Errorf("the profile must not touch identity, got %s shown as %q", got.ID, got.Name)
	}
}

func TestDiscoveredTargetsComeFirst(t *testing.T) {
	targets, err := load(t, "targets:\n  - { name: box, modes: [{ name: shell }] }\n")
	if err != nil || ids(targets) != "near/box box" {
		t.Errorf("want discovered then config, got %q and %v", ids(targets), err)
	}
}

func TestEntryWithoutProviderIsReported(t *testing.T) {
	_, err := load(t, "discover:\n  - provider: near\n  - profile: x\n")
	if err == nil || !strings.Contains(err.Error(), "discover entry #2 has no provider") {
		t.Errorf("want the entry without a provider reported, got %v", err)
	}
}
