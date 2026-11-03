package tailscale

import (
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
	previous := list
	t.Cleanup(func() { list = previous })
	list = func() (string, error) { return out, err }
}

func fixture(t *testing.T) string {
	t.Helper()
	out, err := os.ReadFile("testdata/status.json")
	if err != nil {
		t.Fatal(err)
	}
	listing(t, string(out), nil)
	return string(out)
}

func resolved(t *testing.T) []target.Target {
	t.Helper()
	targets, err := inventory.Merge("", Provider{})
	if err != nil {
		t.Fatal(err)
	}
	return targets
}

func TestListsReachablePeersByMagicDNSName(t *testing.T) {
	fixture(t)
	var got []string
	for _, peer := range resolved(t) {
		got = append(got, peer.ID.String()+"="+peer.Name)
	}
	want := []string{"tailscale/anas-macbook=anas-macbook", "tailscale/berlin=berlin"}
	if !slices.Equal(got, want) {
		t.Errorf("want peers without phones or Mullvad nodes, sorted, got %q", got)
	}
}

func TestPeerFacts(t *testing.T) {
	fixture(t)
	berlin := resolved(t)[1]
	if want := []string{"ssh", "-t", "-o", "ConnectTimeout=2", "berlin.tail0000.ts.net"}; !slices.Equal(berlin.Reach.Interactive, want) {
		t.Errorf("via: got %q, want %q", berlin.Reach.Interactive, want)
	}
	if want := []string{"ssh", "-o", "BatchMode=yes", "-o", "StrictHostKeyChecking=accept-new", "-o", "ConnectTimeout=2", "berlin.tail0000.ts.net"}; !slices.Equal(berlin.Reach.Background, want) {
		t.Errorf("exec: got %q, want %q", berlin.Reach.Background, want)
	}
	if berlin.Power != nil {
		t.Errorf("uplink does not power remote machines, got %+v", berlin.Power)
	}
}

func TestAddressFallsBackToTheIPWithoutMagicDNS(t *testing.T) {
	out := strings.Replace(fixture(t), `"MagicDNSEnabled": true`, `"MagicDNSEnabled": false`, 1)
	listing(t, out, nil)
	if via := resolved(t)[1].Reach.Interactive; via[len(via)-1] != "100.64.0.2" {
		t.Errorf("want the first tailscale IP, got %q", via)
	}
}

func TestDefaultsFollowThePeerOS(t *testing.T) {
	fixture(t)
	targets := resolved(t)
	mac, berlin := targets[0], targets[1]

	if berlin.Group != "machines" || !berlin.Watch || !berlin.Stats.Builtin {
		t.Errorf("want a watched machine with built-in stats, got %q %v %v", berlin.Group, berlin.Watch, berlin.Stats.Builtin)
	}
	for _, peer := range []target.Target{berlin, mac} {
		if !peer.Logs.Journal || peer.Logs.Command != "" {
			t.Errorf("%s: the core decides on guest logs, got %+v", peer.Name, peer.Logs)
		}
	}
	if !mac.Stats.Builtin {
		t.Errorf("want built-in stats, got %v", mac.Stats.Builtin)
	}
}

func TestFailuresBelongToTheSource(t *testing.T) {
	for name, c := range map[string]struct {
		out string
		err error
	}{
		"status fails": {err: errors.New("tailscale: exit 1")},
		"bad json":     {out: "{not json"},
		"stopped":      {out: `{"BackendState": "Stopped"}`},
	} {
		listing(t, c.out, c.err)
		targets, err := inventory.Merge("", Provider{})
		if len(targets) != 0 || err == nil || !strings.HasPrefix(err.Error(), "tailscale: ") {
			t.Errorf("%s: want no targets and a tailscale error, got %v and %v", name, targets, err)
		}
	}
}

func TestOnByDefaultOnlyWhenConnected(t *testing.T) {
	previousLook, previousPing := lookPath, ping
	t.Cleanup(func() { lookPath, ping = previousLook, previousPing })

	found := func(string) (string, error) { return "/usr/bin/tailscale", nil }
	missing := func(string) (string, error) { return "", errors.New("not found") }
	state := func(s string) func() (string, error) {
		return func() (string, error) { return `{"BackendState": "` + s + `"}`, nil }
	}
	failing := func() (string, error) { return "", errors.New("no daemon") }

	for _, c := range []struct {
		name string
		look func(string) (string, error)
		ping func() (string, error)
		want bool
	}{
		{"connected", found, state("Running"), true},
		{"logged out", found, state("NeedsLogin"), false},
		{"no daemon", found, failing, false},
		{"no cli", missing, state("Running"), false},
	} {
		lookPath, ping = c.look, c.ping
		if got := (Provider{}).Default(); got != c.want {
			t.Errorf("%s: Default() = %v, want %v", c.name, got, c.want)
		}
	}
}
