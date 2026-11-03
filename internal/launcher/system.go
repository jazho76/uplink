package launcher

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jazho76/uplink/internal/inspection"
	"github.com/jazho76/uplink/internal/shell"
	"github.com/jazho76/uplink/internal/target"
)

const (
	probeCeiling = 30 * time.Second
	checkTimeout = 5 * time.Second
	powerTimeout = 10 * time.Minute
	logsTimeout  = 2 * time.Second

	exitCannotRun = 126
	exitNotFound  = 127

	journalTimeout = 10 * time.Second
	journalCommand = "journalctl --system -n 500 --no-pager -o short"
)

type Backend interface {
	Inspect(context.Context, target.Target) (target.Environment, error)
	Status(context.Context, target.Target) (target.Status, error)
	Up(context.Context, target.Target) error
	Down(context.Context, target.Target) error
	Logs(ctx context.Context, t target.Target, lines int) string
}

type System struct{}

func (System) Inspect(ctx context.Context, t target.Target) (target.Environment, error) {
	return inspection.Run(ctx, probeCeiling, t.Stats, t.Reach.Probe("sh"))
}

func (System) Status(ctx context.Context, t target.Target) (target.Status, error) {
	if t.Check != "" {
		_, err := shell.NonInteractive(ctx, checkTimeout, "", "sh", "-c", t.Check)
		return checkStatus(err), err
	}
	argv := t.Reach.Probe("true")
	_, err := shell.NonInteractive(ctx, probeCeiling, "", argv[0], argv[1:]...)
	if err != nil {
		return target.StatusUnknown, err
	}
	return target.StatusUp, nil
}

func checkStatus(err error) target.Status {
	code, exited := shell.ExitCode(err)
	switch {
	case err == nil:
		return target.StatusUp
	case shell.Interrupted(err), !exited, code == exitCannotRun, code == exitNotFound:
		return target.StatusUnknown
	default:
		return target.StatusDown
	}
}

func (System) Up(ctx context.Context, t target.Target) error {
	return hook(ctx, powerTimeout, t.StartCommand())
}

func (System) Down(ctx context.Context, t target.Target) error {
	return hook(ctx, powerTimeout, t.StopCommand())
}

func hook(ctx context.Context, limit time.Duration, command string) error {
	if command == "" {
		return errors.New("no hook configured")
	}
	_, err := shell.NonInteractive(ctx, limit, "", "sh", "-c", command)
	if reason := shell.StderrTail(err); reason != "" {
		return errors.New(reason)
	}
	return err
}

func (System) Logs(ctx context.Context, t target.Target, lines int) string {
	var (
		out string
		err error
	)
	switch {
	case t.Logs.Command != "":
		out, err = shell.NonInteractive(ctx, logsTimeout, "", "sh", "-c", t.Logs.Command)
	case t.Logs.Journal:
		argv := t.Reach.Probe(journalCommand)
		out, err = shell.NonInteractive(ctx, journalTimeout, "", argv[0], argv[1:]...)
	default:
		return ""
	}
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
