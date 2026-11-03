package docker

import (
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
	previous := list
	t.Cleanup(func() { list = previous })
	list = func() (string, error) { return out, err }
}

func fixture(t *testing.T) {
	t.Helper()
	out, err := os.ReadFile("testdata/ps.json")
	if err != nil {
		t.Fatal(err)
	}
	listing(t, string(out), nil)
}

func resolved(t *testing.T) []target.Target {
	t.Helper()
	targets, err := inventory.Merge("", Provider{})
	if err != nil {
		t.Fatal(err)
	}
	return targets
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
	for name, c := range map[string]struct {
		out string
		err error
	}{
		"docker ps fails": {err: errors.New("docker: exit 1")},
		"bad json":        {out: "{not json"},
	} {
		listing(t, c.out, c.err)
		targets, err := inventory.Merge("", Provider{})
		if len(targets) != 0 || err == nil || !strings.HasPrefix(err.Error(), "docker: ") {
			t.Errorf("%s: want no targets and a docker error, got %v and %v", name, targets, err)
		}
	}
}

func TestOnByDefaultOnlyWithARunningDaemon(t *testing.T) {
	previousLook, previousPing := lookPath, ping
	t.Cleanup(func() { lookPath, ping = previousLook, previousPing })

	found := func(string) (string, error) { return "/usr/bin/docker", nil }
	missing := func(string) (string, error) { return "", errors.New("not found") }
	up := func() error { return nil }
	down := func() error { return errors.New("cannot connect") }

	for _, c := range []struct {
		name string
		look func(string) (string, error)
		ping func() error
		want bool
	}{
		{"cli and daemon", found, up, true},
		{"daemon down", found, down, false},
		{"no cli", missing, up, false},
	} {
		lookPath, ping = c.look, c.ping
		if got := (Provider{}).Default(); got != c.want {
			t.Errorf("%s: Default() = %v, want %v", c.name, got, c.want)
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
