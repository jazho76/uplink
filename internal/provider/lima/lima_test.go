package lima

import (
	"context"
	"errors"
	"os"
	"slices"
	"strings"
	"testing"

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
	out, err := os.ReadFile("testdata/list.json")
	if err != nil {
		t.Fatal(err)
	}
	listing(t, string(out), nil)
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

func TestListsEveryInstanceByName(t *testing.T) {
	fixture(t)
	var ids []string
	for _, vm := range resolved(t) {
		ids = append(ids, vm.ID.String())
	}
	if got := strings.Join(ids, " "); got != "lima/bsd lima/dev" {
		t.Errorf("want instances sorted by name, got %q", got)
	}
}

func TestInstanceFacts(t *testing.T) {
	fixture(t)
	dev := resolved(t)[1]

	config := "/home/me/lima vms/dev/ssh.config"
	if want := []string{"ssh", "-t", "-F", config, "lima-dev"}; !slices.Equal(dev.Reach.Interactive, want) {
		t.Errorf("via: got %q, want %q", dev.Reach.Interactive, want)
	}
	if want := []string{"ssh", "-F", config, "lima-dev"}; !slices.Equal(dev.Reach.Background, want) {
		t.Errorf("exec: got %q, want %q", dev.Reach.Background, want)
	}
	for _, c := range []struct{ field, got, want string }{
		{"up", dev.StartCommand(), "limactl start dev"},
		{"down", dev.StopCommand(), "limactl stop dev"},
		{"logs", dev.Logs.Command, `tail -n 500 '/home/me/lima vms/dev/serial.log'`},
	} {
		if c.got != c.want {
			t.Errorf("%s: got %q, want %q", c.field, c.got, c.want)
		}
	}
}

func TestInstanceDefaults(t *testing.T) {
	fixture(t)
	targets := resolved(t)
	dev := targets[1]

	if dev.Group != "vms" || !dev.Watch || !dev.Stats.Builtin {
		t.Errorf("want a watched vms target with built-in stats, got %q %v %v", dev.Group, dev.Watch, dev.Stats.Builtin)
	}
}

func TestFailuresBelongToTheSource(t *testing.T) {
	for name, c := range map[string]struct {
		out string
		err error
	}{
		"limactl fails": {err: errors.New("limactl: exit 1")},
		"bad json":      {out: "{not json"},
	} {
		listing(t, c.out, c.err)
		targets, err := merge()
		if len(targets) != 0 || err == nil || !strings.HasPrefix(err.Error(), "lima: ") {
			t.Errorf("%s: want no targets and a lima error, got %v and %v", name, targets, err)
		}
	}
}

func TestReportsStatus(t *testing.T) {
	for state, want := range map[string]target.Status{
		"Running": target.StatusUp,
		"Stopped": target.StatusDown,
		"Broken":  target.StatusUnknown,
	} {
		listing(t, `{"name":"dev","status":"`+state+`"}`, nil)
		merged, err := merge()
		if err != nil || len(merged) != 1 || merged[0].Status != want {
			t.Errorf("%s: want %s, got %+v and %v", state, want, merged, err)
		}
	}
}

func TestUnavailableWithoutLimactl(t *testing.T) {
	listing(t, "", nil)
	lookPath = func(string) (string, error) { return "", errors.New("not found") }
	if _, err := (Provider{}).Discover(context.Background()); !errors.Is(err, inventory.ErrUnavailable) {
		t.Errorf("got %v", err)
	}
}

func TestProviderOwnsStatus(t *testing.T) {
	fixture(t)
	for _, found := range resolved(t) {
		if found.Check != "" || !found.Discovered() {
			t.Errorf("status comes from discovery, got %+v", found)
		}
	}
}
