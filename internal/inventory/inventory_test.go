package inventory

import (
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
	name string
	on   bool
}

func (f fakeProvider) Name() string  { return f.name }
func (f fakeProvider) Default() bool { return f.on }
func (f fakeProvider) Targets() ([]target.Target, error) {
	return []target.Target{{
		ID:    target.ID{Key: "box"},
		Name:  f.name + "-box",
		Group: f.name,
		Modes: []target.Mode{{Name: "shell"}},
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

func (bareProvider) Name() string  { return "bare" }
func (bareProvider) Default() bool { return true }
func (bareProvider) Targets() ([]target.Target, error) {
	return []target.Target{{ID: target.ID{Key: "found"}}}, nil
}

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

type fakeSource struct {
	name    string
	targets []target.Target
	err     error
}

func (f fakeSource) Name() string                      { return f.name }
func (f fakeSource) Targets() ([]target.Target, error) { return f.targets, f.err }

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
	config := fakeSource{name: target.ConfigSource, targets: []target.Target{keyed("box")}}
	lima := fakeSource{name: "lima", targets: []target.Target{keyed("box"), keyed("dev")}}

	targets, err := Merge("", config, lima)
	if err != nil {
		t.Fatal(err)
	}
	if got := ids(targets); got != "box lima/box lima/dev" {
		t.Errorf("want config first, then lima in its order, got %q", got)
	}
}

func TestMergeFillsWhatSourcesLeaveOut(t *testing.T) {
	targets, err := Merge("", fakeSource{name: "lima", targets: []target.Target{
		{ID: target.ID{Key: "dev"}, Name: "Dev box", Group: "work"},
		{ID: target.ID{Key: "bare"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if got := targets[0]; got.Name != "Dev box" || got.ID.Key != "dev" || got.Group != "work" {
		t.Errorf("a source's own values stay, got %+v", got)
	}
	if got := targets[1]; got.Name != "bare" || got.Group != target.DefaultGroup {
		t.Errorf("want name from key and the default group, got %+v", got)
	}
}

func TestFailingSourceOnlyLosesItsOwnTargets(t *testing.T) {
	config := fakeSource{name: target.ConfigSource, targets: []target.Target{keyed("host")}}
	lima := fakeSource{name: "lima", err: errors.New("limactl not found")}

	targets, err := Merge("", config, lima)
	if len(targets) != 1 || targets[0].Name != "host" {
		t.Errorf("config targets should survive, got %v", targets)
	}
	if err == nil || err.Error() != "lima: limactl not found" {
		t.Errorf("want the failure prefixed with its source, got %v", err)
	}
}

func TestDuplicateSourcesAndBrokenTargetsAreReported(t *testing.T) {
	first := fakeSource{name: "lima", targets: []target.Target{keyed("bad name"), {Modes: shellOnly}}}
	second := fakeSource{name: "lima", targets: []target.Target{keyed("y")}}

	targets, err := Merge("", first, second)
	if len(targets) != 0 {
		t.Errorf("want no targets, got %v", targets)
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

func TestDefaultModeOrdersTheAutomaticModes(t *testing.T) {
	withFakeMultiplexer(t)
	source := fakeSource{name: "test", targets: []target.Target{{ID: target.ID{Key: "box"}}}}

	for defaultMode, want := range map[string][]string{
		"":      {"tmux", "fake", "shell"},
		"tmux":  {"tmux", "fake", "shell"},
		"fake":  {"fake", "tmux", "shell"},
		"shell": {"shell", "tmux", "fake"},
	} {
		targets, err := Merge(defaultMode, source)
		if err != nil {
			t.Fatalf("%q: %v", defaultMode, err)
		}
		if got := modeNames(targets[0]); !slices.Equal(got, want) {
			t.Errorf("%q: want %v, got %v", defaultMode, want, got)
		}
	}
}

func TestZellijCanBeTheDefaultMode(t *testing.T) {
	targets, err := Merge("zellij", fakeSource{name: "test", targets: []target.Target{{ID: target.ID{Key: "box"}}}})
	if err != nil {
		t.Fatal(err)
	}
	if got := modeNames(targets[0]); !slices.Equal(got, []string{"zellij", "tmux", "shell"}) {
		t.Errorf("want zellij, tmux, shell, got %v", got)
	}
}

func TestAutomaticModesNameTheirMultiplexer(t *testing.T) {
	targets, err := Merge("", fakeSource{name: "test", targets: []target.Target{{ID: target.ID{Key: "box"}}}})
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range targets[0].Modes {
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

	targets, err := Merge("tmux", fakeSource{name: "test", targets: []target.Target{declared}})
	if err != nil {
		t.Fatal(err)
	}
	if got := modeNames(targets[0]); !slices.Equal(got, []string{"root", "tmux"}) {
		t.Errorf("explicit modes keep their order, got %v", got)
	}
}

func TestUnknownDefaultModeIsReportedAndTargetsStillLoad(t *testing.T) {
	targets, err := Merge("zelij", fakeSource{name: "test", targets: []target.Target{{ID: target.ID{Key: "box"}}}})
	if err == nil || !strings.Contains(err.Error(), `unknown mode "zelij"`) || !strings.Contains(err.Error(), "tmux, zellij, shell") {
		t.Errorf("want the value and the choices reported, got %v", err)
	}
	if len(targets) != 1 || modeNames(targets[0])[0] != "tmux" {
		t.Errorf("want the target with the implicit default, got %+v", targets)
	}
}
