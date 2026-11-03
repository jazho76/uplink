package tailscale

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"slices"
	"strings"
	"time"

	"github.com/jazho76/uplink/internal/shell"
	"github.com/jazho76/uplink/internal/target"
)

const (
	name        = "tailscale"
	group       = "machines"
	running     = "Running"
	pingTimeout = 2 * time.Second
	listTimeout = 5 * time.Second
)

var (
	lookPath = exec.LookPath
	ping     = func() (string, error) {
		return shell.NonInteractive(context.Background(), pingTimeout, "", "tailscale", "status", "--json", "--peers=false")
	}
	list = func() (string, error) {
		return shell.NonInteractive(context.Background(), listTimeout, "", "tailscale", "status", "--json")
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

func (Provider) Targets() ([]target.Target, error) {
	out, err := list()
	if err != nil {
		return nil, err
	}
	return parse(out)
}

func parse(out string) ([]target.Target, error) {
	var s status
	if err := json.Unmarshal([]byte(out), &s); err != nil {
		return nil, err
	}
	if s.BackendState != running {
		return nil, fmt.Errorf("not running (state %q)", s.BackendState)
	}
	magicDNS := s.CurrentTailnet != nil && s.CurrentTailnet.MagicDNSEnabled
	var targets []target.Target
	for _, p := range s.Peer {
		if t, ok := p.target(magicDNS); ok {
			targets = append(targets, t)
		}
	}
	slices.SortFunc(targets, func(a, b target.Target) int { return strings.Compare(a.ID.Key, b.ID.Key) })
	return targets, nil
}

func (p peer) target(magicDNS bool) (target.Target, bool) {
	dnsName := strings.TrimSuffix(p.DNSName, ".")
	key, _, _ := strings.Cut(dnsName, ".")
	if key == "" || p.Location != nil || slices.Contains(unreachableOS, p.OS) {
		return target.Target{}, false
	}
	address := dnsName
	if !magicDNS {
		if len(p.TailscaleIPs) == 0 {
			return target.Target{}, false
		}
		address = p.TailscaleIPs[0]
	}

	return target.Target{
		ID:    target.ID{Key: key},
		Group: group,
		Reach: target.Reach{
			Interactive: []string{"ssh", "-t", "-o", "ConnectTimeout=2", address},
			Background:  []string{"ssh", "-o", "BatchMode=yes", "-o", "StrictHostKeyChecking=accept-new", "-o", "ConnectTimeout=2", address},
		},
		Logs:  target.LogSource{Journal: true},
		Watch: true,
		Stats: target.StatsPolicy{Builtin: true},
	}, true
}
