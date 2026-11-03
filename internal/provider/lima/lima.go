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
	list     = func() (string, error) {
		return shell.NonInteractive(context.Background(), listTimeout, "", "limactl", "list", "--json")
	}
)

type instance struct {
	Name          string `json:"name"`
	Hostname      string `json:"hostname"`
	Dir           string `json:"dir"`
	SSHConfigFile string `json:"sshConfigFile"`
}

type Provider struct{}

func (Provider) Name() string { return name }

func (Provider) Default() bool {
	_, err := lookPath("limactl")
	return err == nil
}

func (Provider) Targets() ([]target.Target, error) {
	out, err := list()
	if err != nil {
		return nil, err
	}
	instances, err := parse(out)
	if err != nil {
		return nil, err
	}
	targets := make([]target.Target, len(instances))
	for i, vm := range instances {
		targets[i] = vm.target()
	}
	return targets, nil
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
		Check: "limactl list --format '{{.Status}}' " + shell.Quote([]string{vm.Name}) + " | grep -qx Running",
		Power: &target.Power{
			Up:   shell.Quote([]string{"limactl", "start", vm.Name}),
			Down: shell.Quote([]string{"limactl", "stop", vm.Name}),
		},
		Logs:  target.LogSource{Command: shell.Quote([]string{"tail", "-n", "500", vm.Dir + "/serial.log"})},
		Watch: true,
		Stats: target.StatsPolicy{Builtin: true},
	}
}
