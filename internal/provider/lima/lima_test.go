package lima

import (
	"errors"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/jazho76/uplink/internal/provider/presets"
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
	out, err := os.ReadFile("testdata/list.json")
	if err != nil {
		t.Fatal(err)
	}
	listing(t, string(out), nil)
}

func resolved(t *testing.T) []target.Target {
	t.Helper()
	targets, err := target.Merge(nil, Provider{})
	if err != nil {
		t.Fatal(err)
	}
	return targets
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
	if want := []string{"ssh", "-t", "-F", config, "lima-dev"}; !slices.Equal(dev.Via, want) {
		t.Errorf("via: got %q, want %q", dev.Via, want)
	}
	if want := []string{"ssh", "-F", config, "lima-dev"}; !slices.Equal(dev.Exec, want) {
		t.Errorf("exec: got %q, want %q", dev.Exec, want)
	}
	for _, c := range []struct{ field, got, want string }{
		{"status", dev.Status, `limactl list --format '{{.Status}}' dev | grep -qx Running`},
		{"up", dev.Up, "limactl start dev"},
		{"down", dev.Down, "limactl stop dev"},
		{"logs", dev.Logs, `tail -n 500 '/home/me/lima vms/dev/serial.log'`},
	} {
		if c.got != c.want {
			t.Errorf("%s: got %q, want %q", c.field, c.got, c.want)
		}
	}
}

func TestInstanceDefaults(t *testing.T) {
	fixture(t)
	targets := resolved(t)
	bsd, dev := targets[0], targets[1]

	if dev.Group != "vms" || !dev.Watch || !slices.Equal(dev.Stats, presets.Linux) {
		t.Errorf("want a watched vms target with linux stats, got %q %v %d stats", dev.Group, dev.Watch, len(dev.Stats))
	}
	if bsd.Stats != nil {
		t.Errorf("a non-Linux guest gets no stats, got %v", bsd.Stats)
	}
	var modes []string
	for _, m := range dev.Modes {
		modes = append(modes, m.Command())
	}
	if want := "tmux new-session -A -s dev"; !strings.Contains(modes[0], want) || len(modes) != 2 {
		t.Errorf("want tmux on the instance name then shell, got %q", modes)
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
		targets, err := target.Merge(nil, Provider{})
		if len(targets) != 0 || err == nil || !strings.HasPrefix(err.Error(), "lima: ") {
			t.Errorf("%s: want no targets and a lima error, got %v and %v", name, targets, err)
		}
	}
}

func TestOnByDefaultWhenInstalled(t *testing.T) {
	previous := lookPath
	t.Cleanup(func() { lookPath = previous })

	lookPath = func(string) (string, error) { return "/usr/bin/limactl", nil }
	if !(Provider{}).Default() {
		t.Errorf("an installed limactl turns lima on")
	}
	lookPath = func(string) (string, error) { return "", errors.New("not found") }
	if (Provider{}).Default() {
		t.Errorf("a missing limactl leaves lima off")
	}
}
