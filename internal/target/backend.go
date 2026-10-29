package target

import (
	"errors"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/jazho76/uplink/internal/probe"
	"github.com/jazho76/uplink/internal/run"
)

const (
	probeCeiling  = 30 * time.Second
	statusTimeout = 5 * time.Second
	hookTimeout   = 10 * time.Minute
	logsTimeout   = 2 * time.Second
)

type Backend interface {
	Stats(Target) (probe.Stats, error)
	Status(Target) error
	Up(Target) error
	Down(Target) error
	Logs(t Target, lines int) string
}

type System struct{}

func (System) Stats(t Target) (probe.Stats, error) {
	return probe.Run(probeCeiling, t.ExecArgvFor("sh")...)
}

func (System) Status(t Target) error {
	if t.Status != "" {
		return hook(statusTimeout, t.Status)
	}
	argv := t.ExecArgvFor("true")
	_, err := run.NonInteractive(probeCeiling, "", argv[0], argv[1:]...)
	return err
}

func (System) Up(t Target) error { return hook(hookTimeout, t.Up) }

func (System) Down(t Target) error { return hook(hookTimeout, t.Down) }

func hook(limit time.Duration, command string) error {
	if command == "" {
		return errors.New("no hook configured")
	}
	_, err := run.NonInteractive(limit, "", "sh", "-c", command)
	if reason := run.StderrTail(err); reason != "" {
		return errors.New(reason)
	}
	return err
}

func (System) Logs(t Target, lines int) string {
	if t.Logs == "" {
		return ""
	}
	out, err := run.NonInteractive(logsTimeout, "", "sh", "-c", t.Logs)
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
