package docker

import (
	"encoding/json"
	"errors"
	"io"
	"os/exec"
	"slices"
	"strings"
	"time"

	"github.com/jazho76/uplink/internal/inspection/stats"
	"github.com/jazho76/uplink/internal/shell"
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
		_, err := shell.NonInteractive(pingTimeout, "", "docker", "version", "--format", "{{.Server.Version}}")
		return err
	}
	list = func() (string, error) {
		return shell.NonInteractive(listTimeout, "", "docker", "ps", "-a", "--format", "{{json .}}")
	}
)

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

func (Provider) Targets() ([]target.Target, error) {
	out, err := list()
	if err != nil {
		return nil, err
	}
	names, err := parse(out)
	if err != nil {
		return nil, err
	}
	targets := make([]target.Target, len(names))
	for i, n := range names {
		targets[i] = containerTarget(n)
	}
	return targets, nil
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

func containerTarget(container string) target.Target {
	quote := func(args ...string) string { return shell.Quote(args) }
	metrics, _ := stats.Preset(stats.PresetContainer)
	return target.Target{
		ID:    target.ID{Key: container},
		Group: group,
		Reach: target.Reach{
			Interactive: []string{"docker", "exec", "-it", container, "sh", "-c"},
			Background:  []string{"docker", "exec", "-i", container, "sh", "-c"},
		},
		Check: "docker inspect -f '{{.State.Running}}' " + quote(container) + " 2>/dev/null | grep -qx true",
		Power: &target.Power{Up: quote("docker", "start", container), Down: quote("docker", "stop", container)},
		Logs:  target.LogSource{Command: events(container)},
		Watch: true,
		Stats: target.StatsPolicy{Metrics: metrics},
	}
}

func events(container string) string {
	return `docker events --since 24h --until "$(date +%s)" ` + shell.Quote([]string{"--filter", "container=" + container}) +
		` --format '{{.Time}} {{.Action}}{{with .Actor.Attributes.exitCode}} (exit {{.}}){{end}}'` +
		` | grep -Ev ' exec_(create|die)| exec_start: sh -c sh$'` +
		` | awk '{ $1 = strftime("%b %d %H:%M:%S", $1); print }' | tail -n 500`
}
