package system

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jazho76/uplink/internal/inspection"
	"github.com/jazho76/uplink/internal/process"
	"github.com/jazho76/uplink/internal/shell"
	"github.com/jazho76/uplink/internal/target"
)

const (
	probeCeiling = 30 * time.Second
	checkTimeout = 5 * time.Second
	powerTimeout = 10 * time.Minute

	hostLogsTimeout   = 2 * time.Second
	targetLogsTimeout = 10 * time.Second

	exitCannotRun = 126
	exitNotFound  = 127
)

type Backend interface {
	Inspect(context.Context, target.Target) (target.Environment, error)
	Status(context.Context, target.Target) (target.Status, error)
	Up(context.Context, target.Target) error
	Down(context.Context, target.Target) error
	Logs(ctx context.Context, t target.Target, lines int) string
}

type Commands struct{}

func (Commands) Inspect(ctx context.Context, t target.Target) (target.Environment, error) {
	return inspection.Run(ctx, probeCeiling, t.Metrics, t.Reach.Probe("sh"))
}

func (Commands) Status(ctx context.Context, t target.Target) (target.Status, error) {
	if t.Check != "" {
		_, err := process.NonInteractive(ctx, checkTimeout, "", "sh", "-c", t.Check)
		return checkStatus(err), err
	}
	argv := t.Reach.Probe("true")
	_, err := process.NonInteractive(ctx, probeCeiling, "", argv[0], argv[1:]...)
	if err != nil {
		return target.StatusUnknown, err
	}
	return target.StatusUp, nil
}

func checkStatus(err error) target.Status {
	code, exited := process.ExitCode(err)
	switch {
	case err == nil:
		return target.StatusUp
	case process.Interrupted(err), !exited, code == exitCannotRun, code == exitNotFound:
		return target.StatusUnknown
	default:
		return target.StatusDown
	}
}

func (Commands) Up(ctx context.Context, t target.Target) error {
	return hook(ctx, powerTimeout, t.StartCommand())
}

func (Commands) Down(ctx context.Context, t target.Target) error {
	return hook(ctx, powerTimeout, t.StopCommand())
}

func hook(ctx context.Context, limit time.Duration, command string) error {
	if command == "" {
		return errors.New("no hook configured")
	}
	_, err := process.NonInteractive(ctx, limit, "", "sh", "-c", command)
	if reason := process.StderrTail(err); reason != "" {
		return errors.New(reason)
	}
	return err
}

func (Commands) Logs(ctx context.Context, t target.Target, lines int) string {
	if t.Logs.Command == "" {
		return ""
	}
	limit, argv := hostLogsTimeout, []string{"sh", "-c", t.Logs.Command}
	if t.Logs.Where == target.InTarget {
		limit, argv = targetLogsTimeout, t.Reach.Probe(t.Logs.Command)
	}
	out, err := process.NonInteractive(ctx, limit, "", argv[0], argv[1:]...)
	if err != nil && out == "" {
		return ""
	}
	return shell.PlainLines(lastLines(out, lines))
}

func lastLines(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}
