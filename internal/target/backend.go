package target

import (
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/jazho76/uplink/internal/probe"
	"github.com/jazho76/uplink/internal/run"
)

const (
	probeCeiling = 30 * time.Second
	logsTimeout  = 2 * time.Second
)

type Backend interface {
	Stats(Target) (probe.Stats, error)
	Reachable(Target) error
	Logs(t Target, lines int) string
}

type System struct{}

func (System) Stats(t Target) (probe.Stats, error) {
	if t.Local() {
		return probe.Local(), nil
	}
	return probe.Run(probeCeiling, t.ArgvFor("sh")...)
}

func (System) Reachable(t Target) error {
	if t.Local() {
		return nil
	}
	argv := t.ArgvFor("true")
	_, err := run.NonInteractive(probeCeiling, "", argv[0], argv[1:]...)
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
