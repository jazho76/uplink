package config

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/jazho76/uplink/internal/shell"
	"github.com/jazho76/uplink/internal/target"
)

func read(t *testing.T, body string) Document {
	t.Helper()
	path := filepath.Join(t.TempDir(), fileName)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	doc, err := Read(path)
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func TestOverlayAppliesOnlyWhatItNames(t *testing.T) {
	doc := read(t, `
discovered:
  - match: lima/nakama
    group: work
    logs: "tail {name}.log"
    stats: [{ type: text, label: who, value: "whoami {name}" }]
    modes:
      - { name: tmux, multiplexer: tmux, run: "tmux new -s {name}" }
`)
	overlays, err := doc.Overlays()
	if err != nil || len(overlays) != 1 {
		t.Fatalf("got %v and %v", overlays, err)
	}
	reach := target.Reach{Interactive: []string{"ssh", "nakama"}}
	power := &target.Power{Up: "start"}
	got := overlays[0].Adjust(target.Target{
		ID:    target.ID{Source: "lima", Key: "nakama"},
		Group: "vms",
		Reach: reach,
		Power: power,
		Stats: target.StatsPolicy{Builtin: true},
		Logs:  target.DetectedLogs(),
		Watch: true,
	})
	if got.Group != "work" || got.Logs != (target.LogSource{Where: target.InHost, Command: "tail nakama.log"}) {
		t.Errorf("got %+v", got)
	}
	if got.Stats.Builtin || got.Stats.Metrics[0].Value != "whoami nakama" {
		t.Errorf("got %+v", got.Stats)
	}
	if mode := got.Modes[0]; mode.Multiplexer != "tmux" || !slices.Equal(mode.Argv, []string{"ssh", "nakama", shell.POSIX("tmux new -s nakama")}) {
		t.Errorf("modes go through the target's reach, got %+v", mode)
	}
	if got.Power != power || !got.Watch || got.ID.Key != "nakama" {
		t.Errorf("what the provider owns stays, got %+v", got)
	}

	untouched := overlaySpec{Match: "lima/nakama", Group: "only"}.adjust(target.Target{Logs: target.DetectedLogs(), Stats: target.StatsPolicy{Builtin: true}})
	if untouched.Logs != target.DetectedLogs() || !untouched.Stats.Builtin || untouched.Modes != nil {
		t.Errorf("fields left out stay as they were, got %+v", untouched)
	}
}

func TestOverlayCanTurnThingsOff(t *testing.T) {
	doc := read(t, "discovered:\n  - { match: lima/a, logs: \"\", stats: [] }\n")
	overlays, _ := doc.Overlays()
	got := overlays[0].Adjust(target.Target{Logs: target.DetectedLogs(), Stats: target.StatsPolicy{Builtin: true}})
	if got.Logs != (target.LogSource{}) || got.Stats.Builtin || len(got.Stats.Metrics) != 0 {
		t.Errorf("got %+v", got)
	}
}

func TestInvalidOverlaysAreSkippedAndReported(t *testing.T) {
	doc := read(t, `
discovered:
  - { match: lima/good, group: work }
  - { group: nomatch }
  - { match: nakama, group: noslash }
  - { match: "lima/[", group: badpattern }
  - { match: lima/a, stats: [linx] }
  - { match: lima/b, stats: [{ type: gauge, label: g, value: x }] }
  - { match: lima/c, modes: [{ name: a }, { name: a }] }
  - { match: lima/d, modes: [{ name: a, multiplexer: screen }] }
`)
	overlays, err := doc.Overlays()
	if len(overlays) != 1 || overlays[0].Match != "lima/good" {
		t.Errorf("the good one survives, got %+v", overlays)
	}
	for _, want := range []string{
		"discovered #2: no match",
		"discovered #3: match",
		"discovered #4: match",
		`discovered #5: stats: unknown preset "linx"`,
		"discovered #6: stats #1: gauge needs a max",
		`discovered #7: duplicate mode "a"`,
		`discovered #8: mode "a": unknown multiplexer "screen"`,
	} {
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("want %q in %v", want, err)
		}
	}
}

func TestOverlayRefusesWhatTheProviderOwns(t *testing.T) {
	for _, field := range []string{"via: ssh x", "exec: ssh x", "check: \"true\"", "up: start", "down: stop", "watch: true", "name: x"} {
		path := filepath.Join(t.TempDir(), fileName)
		if err := os.WriteFile(path, []byte("discovered:\n  - { match: lima/a, "+field+" }\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := Read(path); err == nil || !strings.Contains(err.Error(), "not found") {
			t.Errorf("%s should be refused, got %v", field, err)
		}
	}
}
