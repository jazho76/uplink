package launcher

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jazho76/uplink/internal/shell"
	"github.com/jazho76/uplink/internal/target"
)

func TestLogsDropEscapesAndKeepTheTail(t *testing.T) {
	out := System{}.Logs(context.Background(), target.Target{Logs: target.LogSource{Command: `printf 'one\n\033[31mtwo\033[0m\tx\nthree\n'`}}, 2)
	if out != "two\tx\nthree" {
		t.Errorf("logs = %q", out)
	}
	if got := (System{}).Logs(context.Background(), target.Target{}, 5); got != "" {
		t.Errorf("a target without logs yields nothing, got %q", got)
	}
}

func TestStatusFromACheck(t *testing.T) {
	via := target.Reach{Interactive: []string{"false"}}
	for name, c := range map[string]struct {
		check string
		want  target.Status
	}{
		"exit 0 is up":             {"true", target.StatusUp},
		"other exits are down":     {"exit 3", target.StatusDown},
		"grep finding nothing":     {"false", target.StatusDown},
		"a missing tool can't say": {"uplink-no-such-tool", target.StatusUnknown},
		"not executable can't say": {"exit 126", target.StatusUnknown},
	} {
		got, _ := System{}.Status(context.Background(), target.Target{Check: c.check, Reach: via})
		if got != c.want {
			t.Errorf("%s: want %s, got %s", name, c.want, got)
		}
	}
}

func TestSlowCheckIsUnknown(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	got, err := System{}.Status(ctx, target.Target{Check: "sleep 5"})
	if got != target.StatusUnknown {
		t.Errorf("uplink could not find out, got %s", got)
	}
	if reason := shell.Describe(err); reason != "timed out" {
		t.Errorf("got %q", reason)
	}
}

func TestStatusByReachingIt(t *testing.T) {
	if got, err := (System{}).Status(context.Background(), target.Target{}); got != target.StatusUp || err != nil {
		t.Errorf("a local target is up, got %s and %v", got, err)
	}
	got, err := System{}.Status(context.Background(), target.Target{Reach: target.Reach{Background: []string{"sh", "-c", "echo 'Permission denied' >&2; exit 255"}}})
	if got != target.StatusUnknown {
		t.Errorf("failing to reach it says nothing about whether it is alive, got %s", got)
	}
	if reason := shell.Describe(err); reason != "Permission denied" {
		t.Errorf("the reason comes from stderr, got %q", reason)
	}
	if got, _ := (System{}).Status(context.Background(), target.Target{Reach: target.Reach{Interactive: []string{"false"}, Background: []string{"sh", "-c"}}}); got != target.StatusUp {
		t.Errorf("background carries the check, got %s", got)
	}
}

func TestHookFailureCarriesItsStderr(t *testing.T) {
	err := (System{}).Up(context.Background(), target.Target{Power: &target.Power{Up: "echo progress; echo 'no such instance' >&2; exit 1"}})
	if err == nil || err.Error() != "no such instance" {
		t.Errorf("a failed hook should say why and nothing else, got %v", err)
	}
	if err := (System{}).Up(context.Background(), target.Target{Power: &target.Power{Up: "exit 3"}}); err == nil || !strings.Contains(err.Error(), "exit status 3") {
		t.Errorf("a silent failure still reports its exit, got %v", err)
	}
	if err := (System{}).Down(context.Background(), target.Target{}); err == nil {
		t.Errorf("a missing hook cannot succeed")
	}
}

func TestStatsFlowThroughExec(t *testing.T) {
	stats := target.StatsPolicy{Metrics: []target.Metric{{Type: target.TypeGauge, Label: "where", Value: "echo 1", Max: "2"}}}
	for name, found := range map[string]target.Target{
		"local": {Stats: stats},
		"via":   {Stats: stats, Reach: target.Reach{Interactive: []string{"false"}, Background: []string{"sh", "-c", "exec sh"}}},
	} {
		snapshot, err := System{}.Inspect(context.Background(), found)
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
	if got, want := (System{}).Logs(context.Background(), guest, 1), journalCommand; got != want {
		t.Errorf("want the journal command through exec, stripped and limited, got %q", got)
	}
	if got := (System{}).Logs(context.Background(), target.Target{}, 1); got != "" {
		t.Errorf("no logs at all should run nothing, got %q", got)
	}
}
