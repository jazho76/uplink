package lima

import (
	"encoding/json"
	"errors"
	"io"
	"os/exec"
	"slices"
	"strings"
	"time"

	"github.com/jazho76/uplink/internal/provider/presets"
	"github.com/jazho76/uplink/internal/run"
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
		return run.NonInteractive(listTimeout, "", "limactl", "list", "--json")
	}
)

type instance struct {
	Name          string `json:"name"`
	Hostname      string `json:"hostname"`
	Dir           string `json:"dir"`
	SSHConfigFile string `json:"sshConfigFile"`
	Config        struct {
		OS string `json:"os"`
	} `json:"config"`
}

type Provider struct{}

func (Provider) Name() string { return name }

func (Provider) Default() bool {
	_, err := lookPath("limactl")
	return err == nil
}

func (Provider) Specs() ([]target.Spec, error) {
	out, err := list()
	if err != nil {
		return nil, err
	}
	instances, err := parse(out)
	if err != nil {
		return nil, err
	}
	specs := make([]target.Spec, len(instances))
	for i, vm := range instances {
		specs[i] = vm.spec()
	}
	return specs, nil
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

func (vm instance) spec() target.Spec {
	quote := func(args ...string) string { return target.Quote(args) }
	watch := true
	return target.Spec{
		Name:   vm.Name,
		Via:    quote("ssh", "-t", "-F", vm.SSHConfigFile, vm.Hostname),
		Exec:   quote("ssh", "-F", vm.SSHConfigFile, vm.Hostname),
		Status: "limactl list --format '{{.Status}}' " + quote(vm.Name) + " | grep -qx Running",
		Up:     quote("limactl", "start", vm.Name),
		Down:   quote("limactl", "stop", vm.Name),
		Logs:   quote("tail", "-n", "500", vm.Dir+"/serial.log"),
		Defaults: &target.Spec{
			Group: group,
			Watch: &watch,
			Stats: presets.For(strings.ToLower(vm.Config.OS)),
			Modes: []target.ModeSpec{
				{Name: "tmux", Run: "tmux new-session -A -s {name}"},
				{Name: "shell"},
			},
		},
	}
}
