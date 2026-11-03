package lima

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os/exec"
	"slices"
	"strings"
	"time"

	"github.com/jazho76/uplink/internal/inventory"
	"github.com/jazho76/uplink/internal/process"
	"github.com/jazho76/uplink/internal/shell"
	"github.com/jazho76/uplink/internal/target"
)

const (
	name        = "lima"
	group       = "vms"
	listTimeout = 5 * time.Second
)

var (
	lookPath = exec.LookPath
	list     = func(ctx context.Context) (string, error) {
		return process.NonInteractive(ctx, listTimeout, "", "limactl", "list", "--json")
	}
)

type instance struct {
	Name          string `json:"name"`
	Status        string `json:"status"`
	Hostname      string `json:"hostname"`
	Dir           string `json:"dir"`
	SSHConfigFile string `json:"sshConfigFile"`
}

type Provider struct{}

func (Provider) Name() string { return name }

func (Provider) Discover(ctx context.Context) ([]inventory.Discovered, error) {
	if _, err := lookPath("limactl"); err != nil {
		return nil, inventory.ErrUnavailable
	}
	out, err := list(ctx)
	if err != nil {
		return nil, err
	}
	instances, err := parse(out)
	if err != nil {
		return nil, err
	}
	found := make([]inventory.Discovered, len(instances))
	for i, vm := range instances {
		found[i] = inventory.Discovered{Target: vm.target(), Status: vm.status(), Reason: "lima reports " + vm.Status}
	}
	return found, nil
}

func (vm instance) status() target.Status {
	switch vm.Status {
	case "Running":
		return target.StatusUp
	case "Stopped":
		return target.StatusDown
	default:
		return target.StatusUnknown
	}
}

func parse(out string) ([]instance, error) {
	var instances []instance
	decoder := json.NewDecoder(strings.NewReader(out))
	for {
		var vm instance
		err := decoder.Decode(&vm)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		instances = append(instances, vm)
	}
	slices.SortFunc(instances, func(a, b instance) int { return strings.Compare(a.Name, b.Name) })
	return instances, nil
}

func (vm instance) target() target.Target {
	return target.Target{
		ID:    target.ID{Key: vm.Name},
		Group: group,
		Reach: target.Reach{
			Interactive: []string{"ssh", "-t", "-F", vm.SSHConfigFile, vm.Hostname},
			Background:  []string{"ssh", "-F", vm.SSHConfigFile, vm.Hostname},
		},
		Power: &target.Power{
			Up:   shell.Quote([]string{"limactl", "start", vm.Name}),
			Down: shell.Quote([]string{"limactl", "stop", vm.Name}),
		},
		Logs:  target.LogSource{Command: shell.Quote([]string{"tail", "-n", "500", vm.Dir + "/serial.log"})},
		Watch: true,
		Stats: target.StatsPolicy{Builtin: true},
	}
}
