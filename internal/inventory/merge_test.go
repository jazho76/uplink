package inventory

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/jazho76/uplink/internal/multiplexer"
	"github.com/jazho76/uplink/internal/target"
)

var shellOnly = []target.Mode{{Name: "shell"}}

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

func ids(targets []target.Target) string {
	var out []string
	for _, t := range targets {
		out = append(out, t.ID.String())
	}
	return strings.Join(out, " ")
}
