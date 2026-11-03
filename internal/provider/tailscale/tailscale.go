package tailscale

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"slices"
	"strings"
	"time"

	"github.com/jazho76/uplink/internal/inventory"
	"github.com/jazho76/uplink/internal/process"
	"github.com/jazho76/uplink/internal/target"
)

const (
	name        = "tailscale"
	group       = "machines"
	running     = "Running"
	listTimeout = 5 * time.Second
)

var (
	lookPath = exec.LookPath
	list     = func(ctx context.Context) (string, error) {
		return process.NonInteractive(ctx, listTimeout, "", "tailscale", "status", "--json")
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
	Online       bool     `json:"Online"`
	TailscaleIPs []string `json:"TailscaleIPs"`
	Location     any      `json:"Location"`
}

type Provider struct{}

func (Provider) Name() string { return name }

func (Provider) Discover(ctx context.Context) ([]inventory.Discovered, error) {
	if _, err := lookPath("tailscale"); err != nil {
		return nil, inventory.ErrUnavailable
	}
	out, err := list(ctx)
	if err != nil {
		if process.Interrupted(err) {
			return nil, err
		}
		return nil, inventory.ErrUnavailable
	}
	return parse(out)
}

func parse(out string) ([]inventory.Discovered, error) {
	var s status
	if err := json.Unmarshal([]byte(out), &s); err != nil {
		return nil, err
	}
	if s.BackendState != running {
		return nil, fmt.Errorf("%w: state %q", inventory.ErrUnavailable, s.BackendState)
	}
	magicDNS := s.CurrentTailnet != nil && s.CurrentTailnet.MagicDNSEnabled
	var found []inventory.Discovered
	for _, p := range s.Peer {
		if t, ok := p.target(magicDNS); ok {
			found = append(found, inventory.Discovered{Target: t, Status: p.status()})
		}
	}
	slices.SortFunc(found, func(a, b inventory.Discovered) int { return strings.Compare(a.Target.ID.Key, b.Target.ID.Key) })
	return found, nil
}

func (p peer) status() target.Status {
	if p.Online {
		return target.StatusUp
	}
	return target.StatusDown
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
		Logs:  target.DetectedLogs(),
		Watch: true,
		Stats: target.StatsPolicy{Builtin: true},
	}, true
}
