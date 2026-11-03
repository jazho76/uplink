package docker

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os/exec"
	"slices"
	"strings"
	"time"

	"github.com/jazho76/uplink/internal/inspection/stats"
	"github.com/jazho76/uplink/internal/inventory"
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
	ping     = func(ctx context.Context) error {
		_, err := shell.NonInteractive(ctx, pingTimeout, "", "docker", "version", "--format", "{{.Server.Version}}")
		return err
	}
	list = func(ctx context.Context) (string, error) {
		return shell.NonInteractive(ctx, listTimeout, "", "docker", "ps", "-a", "--format", "{{json .}}")
	}
)

type container struct {
	Names string `json:"Names"`
	State string `json:"State"`
}

type Provider struct{}

func (Provider) Name() string { return name }

func (Provider) Discover(ctx context.Context) ([]inventory.Discovered, error) {
	if _, err := lookPath("docker"); err != nil {
		return nil, inventory.ErrUnavailable
	}
	out, err := list(ctx)
	if err != nil {
		if pingErr := ping(ctx); pingErr != nil && !shell.Interrupted(pingErr) {
			return nil, inventory.ErrUnavailable
		}
		return nil, err
	}
	containers, err := parse(out)
	if err != nil {
		return nil, err
	}
	found := make([]inventory.Discovered, len(containers))
	for i, c := range containers {
		found[i] = inventory.Discovered{Target: containerTarget(c.name()), Status: c.status(), Reason: "docker reports " + c.State}
	}
	return found, nil
}

func parse(out string) ([]container, error) {
	var containers []container
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
		containers = append(containers, c)
	}
	slices.SortFunc(containers, func(a, b container) int { return strings.Compare(a.name(), b.name()) })
	return containers, nil
}

func (c container) name() string {
	first, _, _ := strings.Cut(c.Names, ",")
	return first
}

func (c container) status() target.Status {
	switch c.State {
	case "running":
		return target.StatusUp
	case "exited", "created", "dead":
		return target.StatusDown
	default:
		return target.StatusUnknown
	}
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
