package tailscale

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
		if peer.Logs != target.DetectedLogs() {
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
		"status times out": {err: context.DeadlineExceeded},
		"bad json":         {out: "{not json"},
	} {
		listing(t, c.out, c.err)
		targets, err := merge()
		if len(targets) != 0 || err == nil || !strings.HasPrefix(err.Error(), "tailscale: ") {
			t.Errorf("%s: want no targets and a tailscale error, got %v and %v", name, targets, err)
		}
	}
}

func TestReportsOnlineAsStatus(t *testing.T) {
	listing(t, `{"BackendState":"Running","CurrentTailnet":{"MagicDNSEnabled":true},"Peer":{
		"a":{"DNSName":"on.ts.net.","OS":"linux","Online":true},
		"b":{"DNSName":"off.ts.net.","OS":"linux","Online":false,"Active":true}}}`, nil)
	merged, err := merge()
	if err != nil || len(merged) != 2 {
		t.Fatalf("got %+v and %v", merged, err)
	}
	if merged[0].Target.ID.Key != "off" || merged[0].Status != target.StatusDown {
		t.Errorf("only Online counts, not Active, got %+v", merged[0])
	}
	if merged[1].Status != target.StatusUp {
		t.Errorf("got %+v", merged[1])
	}
}

func TestUnavailable(t *testing.T) {
	for name, c := range map[string]struct {
		out string
		err error
	}{
		"stopped":    {out: `{"BackendState": "Stopped"}`},
		"logged out": {out: `{"BackendState": "NeedsLogin"}`},
		"no daemon":  {err: errors.New("exit 1")},
	} {
		listing(t, c.out, c.err)
		if _, err := (Provider{}).Discover(context.Background()); !errors.Is(err, inventory.ErrUnavailable) {
			t.Errorf("%s: got %v", name, err)
		}
	}
	listing(t, "", nil)
	lookPath = func(string) (string, error) { return "", errors.New("not found") }
	if _, err := (Provider{}).Discover(context.Background()); !errors.Is(err, inventory.ErrUnavailable) {
		t.Errorf("no cli: got %v", err)
	}
}
