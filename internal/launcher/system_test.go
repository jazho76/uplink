package launcher

import (
	"strings"
	"testing"

	"github.com/jazho76/uplink/internal/target"
)

func TestLogsDropEscapesAndKeepTheTail(t *testing.T) {
	out := System{}.Logs(target.Target{Logs: target.LogSource{Command: `printf 'one\n\033[31mtwo\033[0m\tx\nthree\n'`}}, 2)
	if out != "two\tx\nthree" {
		t.Errorf("logs = %q", out)
	}
	if got := (System{}).Logs(target.Target{}, 5); got != "" {
		t.Errorf("a target without logs yields nothing, got %q", got)
	}
}

func TestStatusPrefersTheHook(t *testing.T) {
	if err := (System{}).Status(target.Target{Check: "true", Reach: target.Reach{Interactive: []string{"false"}}}); err != nil {
		t.Errorf("a passing hook means up, regardless of via: %v", err)
	}
	if err := (System{}).Status(target.Target{Check: "false"}); err == nil {
		t.Errorf("a failing hook means down")
	}
	if err := (System{}).Status(target.Target{}); err != nil {
		t.Errorf("a local target without a hook is up: %v", err)
	}
	if err := (System{}).Status(target.Target{Reach: target.Reach{Interactive: []string{"false"}}}); err == nil {
		t.Errorf("without a hook, a failing transport means down")
	}
	if err := (System{}).Status(target.Target{Reach: target.Reach{Interactive: []string{"false"}, Background: []string{"sh", "-c"}}}); err != nil {
		t.Errorf("without a hook, exec carries the check: %v", err)
	}
}

func TestHookFailureCarriesItsStderr(t *testing.T) {
	err := (System{}).Up(target.Target{Power: &target.Power{Up: "echo progress; echo 'no such instance' >&2; exit 1"}})
	if err == nil || err.Error() != "no such instance" {
		t.Errorf("a failed hook should say why and nothing else, got %v", err)
	}
	if err := (System{}).Up(target.Target{Power: &target.Power{Up: "exit 3"}}); err == nil || !strings.Contains(err.Error(), "exit status 3") {
		t.Errorf("a silent failure still reports its exit, got %v", err)
	}
	if err := (System{}).Down(target.Target{}); err == nil {
		t.Errorf("a missing hook cannot succeed")
	}
}

func TestStatsFlowThroughExec(t *testing.T) {
	stats := target.StatsPolicy{Metrics: []target.Metric{{Type: target.TypeGauge, Label: "where", Value: "echo 1", Max: "2"}}}
	for name, found := range map[string]target.Target{
		"local": {Stats: stats},
		"via":   {Stats: stats, Reach: target.Reach{Interactive: []string{"false"}, Background: []string{"sh", "-c", "exec sh"}}},
	} {
		snapshot, err := System{}.Inspect(found)
		if err != nil {
			t.Fatalf("%s: stats through a local shell: %v", name, err)
		}
		readings := snapshot.Readings
		if len(readings) != 1 || !readings[0].OK || readings[0].Value != 1 {
			t.Errorf("%s: the declared metrics should come back, got %+v", name, readings)
		}
	}
}

func TestGuestLogsFlowThroughExec(t *testing.T) {
	guest := target.Target{Logs: target.LogSource{Journal: true}, Reach: target.Reach{Background: []string{"sh", "-c", `printf 'old\n\033[31m%s\033[0m\n' "$0"`}}}
	if got, want := (System{}).Logs(guest, 1), journalCommand; got != want {
		t.Errorf("want the journal command through exec, stripped and limited, got %q", got)
	}
	if got := (System{}).Logs(target.Target{}, 1); got != "" {
		t.Errorf("no logs at all should run nothing, got %q", got)
	}
}
