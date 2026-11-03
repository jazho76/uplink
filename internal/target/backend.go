package target

import (
	"errors"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/jazho76/uplink/internal/inspection"
	"github.com/jazho76/uplink/internal/shell"
)

const (
	probeCeiling  = 30 * time.Second
	statusTimeout = 5 * time.Second
	hookTimeout   = 10 * time.Minute
	logsTimeout   = 2 * time.Second

	guestLogsTimeout = 10 * time.Second
	journalCommand   = "journalctl --system -n 500 --no-pager -o short"
)

type Backend interface {
	Inspect(Target) (inspection.Snapshot, error)
	Status(Target) error
	Up(Target) error
	Down(Target) error
	Logs(t Target, lines int) string
}

type System struct{}

func (System) Inspect(t Target) (inspection.Snapshot, error) {
	return inspection.Run(probeCeiling, inspection.Request{Metrics: t.Stats, BuiltinStats: t.BuiltinStats}, t.ExecArgvFor("sh"))
}

func (System) Status(t Target) error {
	if t.Status != "" {
		return hook(statusTimeout, t.Status)
	}
	argv := t.ExecArgvFor("true")
	_, err := shell.NonInteractive(probeCeiling, "", argv[0], argv[1:]...)
	return err
}

func (System) Up(t Target) error { return hook(hookTimeout, t.Up) }

func (System) Down(t Target) error { return hook(hookTimeout, t.Down) }

func hook(limit time.Duration, command string) error {
	if command == "" {
		return errors.New("no hook configured")
	}
	_, err := shell.NonInteractive(limit, "", "sh", "-c", command)
	if reason := shell.StderrTail(err); reason != "" {
		return errors.New(reason)
	}
	return err
}

func (System) Logs(t Target, lines int) string {
	var (
		out string
		err error
	)
	switch {
	case t.Logs != "":
		out, err = shell.NonInteractive(logsTimeout, "", "sh", "-c", t.Logs)
	case t.GuestLogs:
		argv := t.ExecArgvFor(journalCommand)
		out, err = shell.NonInteractive(guestLogsTimeout, "", argv[0], argv[1:]...)
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
