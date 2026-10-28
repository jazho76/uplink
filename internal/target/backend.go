package target

import (
	"regexp"
	"strings"
	"time"

	"github.com/jazho76/uplink/internal/probe"
	"github.com/jazho76/uplink/internal/run"
)

const logsTimeout = 2 * time.Second

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
	return probe.Run(t.ArgvFor("sh")...)
}

func (System) Reachable(t Target) error {
	if t.Local() {
		return nil
	}
	argv := t.ArgvFor("true")
	_, err := run.NonInteractive("", argv[0], argv[1:]...)
	return err
}

func (System) Logs(t Target, lines int) string {
	if t.Logs == "" {
		return ""
	}
	out, err := run.NonInteractiveWithin(logsTimeout, "", "sh", "-c", t.Logs)
	if err != nil && out == "" {
		return ""
	}
	return lastLines(sanitize(out), lines)
}

var ansiRe = regexp.MustCompile("\x1b\\[[0-9;?]*[ -/]*[@-~]|\x1b[@-Z\\\\-_]")

func sanitize(s string) string {
	s = ansiRe.ReplaceAllString(s, "")
	return strings.Map(func(r rune) rune {
		switch {
		case r == '\n':
			return r
		case r == '\t':
			return ' '
		case r < 0x20 || r == 0x7f:
			return -1
		default:
			return r
		}
	}, s)
}

func lastLines(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}
