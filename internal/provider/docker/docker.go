package docker

import (
	"encoding/json"
	"errors"
	"io"
	"os/exec"
	"slices"
	"strings"
	"time"

	"github.com/jazho76/uplink/internal/run"
	"github.com/jazho76/uplink/internal/stats"
	"github.com/jazho76/uplink/internal/target"
)

const (
	name        = "docker"
	group       = "containers"
	pingTimeout = 2 * time.Second
	listTimeout = 5 * time.Second
)

var (
	lookPath = exec.LookPath
	ping     = func() error {
		_, err := run.NonInteractive(pingTimeout, "", "docker", "version", "--format", "{{.Server.Version}}")
		return err
	}
	list = func() (string, error) {
		return run.NonInteractive(listTimeout, "", "docker", "ps", "-a", "--format", "{{json .}}")
	}
)

var memory = stats.Metric{
	Type:  stats.TypeGauge,
	Label: "memory",
	Unit:  stats.UnitBytes,
	Value: "cat /sys/fs/cgroup/memory.current",
	Max: `limit=$(cat /sys/fs/cgroup/memory.max)
[ "$limit" = max ] && limit=$(awk '/^MemTotal:/{print $2 * 1024}' /proc/meminfo)
echo "$limit"`,
}

type container struct {
	Names string `json:"Names"`
}

type Provider struct{}

func (Provider) Name() string { return name }

func (Provider) Default() bool {
	if _, err := lookPath("docker"); err != nil {
		return false
	}
	return ping() == nil
}

func (Provider) Specs() ([]target.Spec, error) {
	out, err := list()
	if err != nil {
		return nil, err
	}
	names, err := parse(out)
	if err != nil {
		return nil, err
	}
	specs := make([]target.Spec, len(names))
	for i, n := range names {
		specs[i] = spec(n)
	}
	return specs, nil
}

func parse(out string) ([]string, error) {
	var names []string
	decoder := json.NewDecoder(strings.NewReader(out))
	for {
		var c container
		err := decoder.Decode(&c)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		first, _, _ := strings.Cut(c.Names, ",")
		names = append(names, first)
	}
	slices.Sort(names)
	return names, nil
}

func spec(container string) target.Spec {
	quote := func(args ...string) string { return target.Quote(args) }
	watch := true
	return target.Spec{
		Name:   container,
		Via:    quote("docker", "exec", "-it", container, "sh", "-c"),
		Exec:   quote("docker", "exec", "-i", container, "sh", "-c"),
		Status: "docker inspect -f '{{.State.Running}}' " + quote(container) + " 2>/dev/null | grep -qx true",
		Up:     quote("docker", "start", container),
		Down:   quote("docker", "stop", container),
		Logs:   quote("docker", "logs", "--tail", "500", container) + " 2>&1",
		Defaults: &target.Spec{
			Group: group,
			Watch: &watch,
			Stats: []stats.Metric{memory},
			Modes: []target.ModeSpec{
				{Name: "tmux", Run: "tmux new-session -A -s {name}"},
				{Name: "shell"},
			},
		},
	}
}
