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
	for name, body := range map[string]string{"missing file": "", "no section": "targets: []\n"} {
		targets, err := load(t, body)
		if err != nil || ids(targets) != "near/box" {
			t.Errorf("%s: want only the default provider, got %q and %v", name, ids(targets), err)
		}
	}
}

func TestDiscoverListsExactlyWhatRuns(t *testing.T) {
	targets, err := load(t, "discover: [far]\n")
	if err != nil || ids(targets) != "far/box" {
		t.Errorf("want only far, got %q and %v", ids(targets), err)
	}
	targets, err = load(t, "discover: []\n")
	if err != nil || len(targets) != 0 {
		t.Errorf("an empty list turns discovery off, got %q and %v", ids(targets), err)
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
	if err != nil || ids(targets) != "near/box box" {
		t.Errorf("want discovered then config, got %q and %v", ids(targets), err)
	}
}

func TestDiscoverTakesProviderNamesOnly(t *testing.T) {
	targets, err := load(t, "discover:\n  - { provider: near }\n")
	if err == nil || len(targets) != 0 {
		t.Errorf("a mapping entry should refuse the config, got %q and %v", ids(targets), err)
	}
}

type bareProvider struct{}

func (bareProvider) Name() string                  { return "bare" }
func (bareProvider) Default() bool                 { return true }
func (bareProvider) Specs() ([]target.Spec, error) { return []target.Spec{{Name: "found"}}, nil }

func loadBare(t *testing.T, body string) ([]target.Target, error) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return Load(path, []Provider{bareProvider{}})
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
