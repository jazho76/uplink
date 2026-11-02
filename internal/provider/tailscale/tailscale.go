package tailscale

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"slices"
	"strings"
	"time"

	"github.com/jazho76/uplink/internal/run"
	"github.com/jazho76/uplink/internal/stats"
	"github.com/jazho76/uplink/internal/target"
)

const (
	name        = "tailscale"
	group       = "machines"
	running     = "Running"
	pingTimeout = 2 * time.Second
	listTimeout = 5 * time.Second
	journal     = "journalctl --system -n 500 --no-pager -o short"
)

var (
	lookPath = exec.LookPath
	ping     = func() (string, error) {
		return run.NonInteractive(pingTimeout, "", "tailscale", "status", "--json", "--peers=false")
	}
	list = func() (string, error) {
		return run.NonInteractive(listTimeout, "", "tailscale", "status", "--json")
	}
)

var unreachableOS = []string{"iOS", "android", "tvOS"}

type status struct {
	BackendState   string `json:"BackendState"`
	CurrentTailnet *struct {
		MagicDNSEnabled bool `json:"MagicDNSEnabled"`
	} `json:"CurrentTailnet"`
	Peer map[string]peer `json:"Peer"`
}

type peer struct {
	HostName     string   `json:"HostName"`
	DNSName      string   `json:"DNSName"`
	OS           string   `json:"OS"`
	TailscaleIPs []string `json:"TailscaleIPs"`
	Location     any      `json:"Location"`
}

type Provider struct{}

func (Provider) Name() string { return name }

func (Provider) Default() bool {
	if _, err := lookPath("tailscale"); err != nil {
		return false
	}
	out, err := ping()
	if err != nil {
		return false
	}
	var s status
	return json.Unmarshal([]byte(out), &s) == nil && s.BackendState == running
}

func (Provider) Specs() ([]target.Spec, error) {
	out, err := list()
	if err != nil {
		return nil, err
	}
	return parse(out)
}

func parse(out string) ([]target.Spec, error) {
	var s status
	if err := json.Unmarshal([]byte(out), &s); err != nil {
		return nil, err
	}
	if s.BackendState != running {
		return nil, fmt.Errorf("not running (state %q)", s.BackendState)
	}
	magicDNS := s.CurrentTailnet != nil && s.CurrentTailnet.MagicDNSEnabled
	var specs []target.Spec
	for _, p := range s.Peer {
		if spec, ok := p.spec(magicDNS); ok {
			specs = append(specs, spec)
		}
	}
	slices.SortFunc(specs, func(a, b target.Spec) int { return strings.Compare(a.Name, b.Name) })
	return specs, nil
}

func (p peer) spec(magicDNS bool) (target.Spec, bool) {
	dnsName := strings.TrimSuffix(p.DNSName, ".")
	key, _, _ := strings.Cut(dnsName, ".")
	if key == "" || p.Location != nil || slices.Contains(unreachableOS, p.OS) {
		return target.Spec{}, false
	}
	address := dnsName
	if !magicDNS {
		if len(p.TailscaleIPs) == 0 {
			return target.Spec{}, false
		}
		address = p.TailscaleIPs[0]
	}

	quote := func(args ...string) string { return target.Quote(args) }
	probe := quote("ssh", "-o", "BatchMode=yes", "-o", "StrictHostKeyChecking=accept-new", "-o", "ConnectTimeout=2", address)
	goos := strings.ToLower(p.OS)
	if p.OS == "macOS" {
		goos = "darwin"
	}
	watch := true
	defaults := &target.Spec{
		Group: group,
		Watch: &watch,
		Stats: stats.ForOS(goos),
	}
	if goos == "linux" {
		defaults.Logs = probe + " " + quote(journal)
	}
	return target.Spec{
		Name:     key,
		Display:  p.HostName,
		Via:      quote("ssh", "-t", "-o", "ConnectTimeout=2", address),
		Exec:     probe,
		Defaults: defaults,
	}, true
}
