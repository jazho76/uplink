package inspect

import (
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/jazho76/uplink/internal/run"
	"github.com/jazho76/uplink/internal/stats"
)

const (
	toolTag    = "tool"
	sessionTag = "session"

	environmentScript = `command -v tmux >/dev/null 2>&1 && echo "tool tmux"
$limit tmux list-sessions -F 'session #{session_windows} #{session_attached} #{session_name}' </dev/null 2>/dev/null || true`
)

type Session struct {
	Name     string
	Windows  int
	Attached bool
}

type Snapshot struct {
	Readings []stats.Reading
	Tools    []string
	Sessions []Session
}

func Run(limit time.Duration, metrics []stats.Metric, argv []string) (Snapshot, error) {
	if len(argv) == 0 {
		return Snapshot{}, errors.New("inspect: no command to run")
	}
	out, err := run.NonInteractive(limit, script(metrics), argv[0], argv[1:]...)
	if err != nil {
		return Snapshot{}, err
	}
	return Parse(out, metrics), nil
}

func script(metrics []stats.Metric) string {
	return stats.Script(metrics) + "\n" + environmentScript
}

func Parse(out string, metrics []stats.Metric) Snapshot {
	snapshot := Snapshot{Readings: stats.Parse(out, metrics)}
	for _, line := range strings.Split(out, "\n") {
		tag, rest, _ := strings.Cut(line, " ")
		switch tag {
		case toolTag:
			snapshot.Tools = append(snapshot.Tools, rest)
		case sessionTag:
			if session, ok := parseSession(rest); ok {
				snapshot.Sessions = append(snapshot.Sessions, session)
			}
		}
	}
	return snapshot
}

func parseSession(fields string) (Session, bool) {
	windowsField, rest, _ := strings.Cut(fields, " ")
	attachedField, name, _ := strings.Cut(rest, " ")
	windows, windowsErr := strconv.Atoi(windowsField)
	attached, attachedErr := strconv.Atoi(attachedField)
	name = ansi.Strip(name)
	if windowsErr != nil || attachedErr != nil || name == "" {
		return Session{}, false
	}
	return Session{Name: name, Windows: windows, Attached: attached > 0}, true
}
