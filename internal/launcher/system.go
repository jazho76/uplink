package launcher

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/jazho76/uplink/internal/inspection"
	"github.com/jazho76/uplink/internal/shell"
	"github.com/jazho76/uplink/internal/target"
)

const (
	probeCeiling = 30 * time.Second
	checkTimeout = 5 * time.Second
	powerTimeout = 10 * time.Minute
	logsTimeout  = 2 * time.Second

	journalTimeout = 10 * time.Second
	journalCommand = "journalctl --system -n 500 --no-pager -o short"
)

type Backend interface {
	Inspect(context.Context, target.Target) (target.Environment, error)
	Status(context.Context, target.Target) error
	Up(context.Context, target.Target) error
	Down(context.Context, target.Target) error
	Logs(ctx context.Context, t target.Target, lines int) string
}

type System struct{}

func (System) Inspect(ctx context.Context, t target.Target) (target.Environment, error) {
	return inspection.Run(ctx, probeCeiling, t.Stats, t.Reach.Probe("sh"))
}

func (System) Status(ctx context.Context, t target.Target) error {
	if t.Check != "" {
		return hook(ctx, checkTimeout, t.Check)
	}
	argv := t.Reach.Probe("true")
	_, err := shell.NonInteractive(ctx, probeCeiling, "", argv[0], argv[1:]...)
	return err
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
	return ansi.Strip(lastLines(out, lines))
}

func lastLines(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}
