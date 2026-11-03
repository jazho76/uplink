package docker

import (
	"context"
	"errors"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/jazho76/uplink/internal/inspection/stats"
	"github.com/jazho76/uplink/internal/inventory"
	"github.com/jazho76/uplink/internal/target"
)

func listing(t *testing.T, out string, err error) {
	t.Helper()
	previousList, previousLook := list, lookPath
	t.Cleanup(func() { list, lookPath = previousList, previousLook })
	list = func(context.Context) (string, error) { return out, err }
	lookPath = func(string) (string, error) { return "/usr/bin/tool", nil }
}

func fixture(t *testing.T) {
	t.Helper()
	out, err := os.ReadFile("testdata/ps.json")
	if err != nil {
		t.Fatal(err)
	}
	listing(t, string(out), nil)
}

func withPing(t *testing.T, err error) {
	t.Helper()
	previous := ping
	t.Cleanup(func() { ping = previous })
	ping = func(context.Context) error { return err }
}

func resolved(t *testing.T) []target.Target {
	t.Helper()
	merged, err := merge()
	if err != nil {
		t.Fatal(err)
	}
	return inventory.Targets(merged)
}

func merge() ([]inventory.Discovered, error) {
	found, err := Provider{}.Discover(context.Background())
	return inventory.Merge("", inventory.Found{Source: name, Discovered: found, Err: err})
}

func TestListsEveryContainerByFirstName(t *testing.T) {
	fixture(t)
	var ids []string
	for _, c := range resolved(t) {
		ids = append(ids, c.ID.String())
	}
	if got := strings.Join(ids, " "); got != "docker/db docker/web" {
		t.Errorf("want containers sorted by their first name, got %q", got)
	}
}

func TestContainerFacts(t *testing.T) {
	fixture(t)
	web := resolved(t)[1]

	if want := []string{"docker", "exec", "-it", "web", "sh", "-c"}; !slices.Equal(web.Reach.Interactive, want) {
		t.Errorf("via: got %q, want %q", web.Reach.Interactive, want)
	}
	if want := []string{"docker", "exec", "-i", "web", "sh", "-c"}; !slices.Equal(web.Reach.Background, want) {
		t.Errorf("exec: got %q, want %q", web.Reach.Background, want)
	}
	for _, c := range []struct{ field, got, want string }{
		{"status", web.Check, `docker inspect -f '{{.State.Running}}' web 2>/dev/null | grep -qx true`},
		{"up", web.StartCommand(), "docker start web"},
		{"down", web.StopCommand(), "docker stop web"},
		{"logs", web.Logs.Command, `docker events --since 24h --until "$(date +%s)" --filter container=web --format '{{.Time}} {{.Action}}{{with .Actor.Attributes.exitCode}} (exit {{.}}){{end}}' | grep -Ev ' exec_(create|die)| exec_start: sh -c sh$' | awk '{ $1 = strftime("%b %d %H:%M:%S", $1); print }' | tail -n 500`},
	} {
		if c.got != c.want {
			t.Errorf("%s: got %q, want %q", c.field, c.got, c.want)
		}
	}
}

func TestContainerDefaults(t *testing.T) {
	fixture(t)
	web := resolved(t)[1]
	if web.Group != "containers" || !web.Watch || !slices.Equal(web.Stats.Metrics, containerPreset(t)) || web.Stats.Builtin {
		t.Errorf("want a watched containers target with the memory gauge, got %q %v %v", web.Group, web.Watch, web.Stats.Metrics)
	}
}

func TestFailuresBelongToTheSource(t *testing.T) {
	withPing(t, nil)
	for name, c := range map[string]struct {
		out string
		err error
	}{
		"docker ps fails": {err: errors.New("docker: exit 1")},
		"bad json":        {out: "{not json"},
	} {
		listing(t, c.out, c.err)
		targets, err := merge()
		if len(targets) != 0 || err == nil || !strings.HasPrefix(err.Error(), "docker: ") {
			t.Errorf("%s: want no targets and a docker error, got %v and %v", name, targets, err)
		}
	}
}

func containerPreset(t *testing.T) []target.Metric {
	t.Helper()
	metrics, ok := stats.Preset(stats.PresetContainer)
	if !ok || len(metrics) == 0 {
		t.Fatal("the container preset should exist")
	}
	return metrics
}

func TestReportsStatusFromState(t *testing.T) {
	for state, want := range map[string]target.Status{
		"running":    target.StatusUp,
		"exited":     target.StatusDown,
		"created":    target.StatusDown,
		"dead":       target.StatusDown,
		"paused":     target.StatusUnknown,
		"restarting": target.StatusUnknown,
	} {
		listing(t, `{"Names":"web","State":"`+state+`"}`, nil)
		merged, err := merge()
		if err != nil || len(merged) != 1 || merged[0].Status != want {
			t.Errorf("%s: want %s, got %+v and %v", state, want, merged, err)
		}
	}
}

func TestUnavailableWithoutTheCLIOrDaemon(t *testing.T) {
	listing(t, "", errors.New("cannot connect"))
	withPing(t, errors.New("cannot connect"))
	if _, err := (Provider{}).Discover(context.Background()); !errors.Is(err, inventory.ErrUnavailable) {
		t.Errorf("a stopped daemon is unavailable, got %v", err)
	}
	withPing(t, context.DeadlineExceeded)
	if _, err := (Provider{}).Discover(context.Background()); errors.Is(err, inventory.ErrUnavailable) {
		t.Errorf("a slow daemon is a failure, so containers do not vanish, got %v", err)
	}
	lookPath = func(string) (string, error) { return "", errors.New("not found") }
	if _, err := (Provider{}).Discover(context.Background()); !errors.Is(err, inventory.ErrUnavailable) {
		t.Errorf("a missing cli is unavailable, got %v", err)
	}
}
