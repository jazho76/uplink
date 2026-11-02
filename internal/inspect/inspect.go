package inspect

import (
	"errors"
	"strings"
	"time"

	"github.com/jazho76/uplink/internal/run"
	"github.com/jazho76/uplink/internal/stats"
)

type Request struct {
	Metrics []stats.Metric
}

type Snapshot struct {
	Readings []stats.Reading
	Tools    []string
	Sessions []Session
}

type section struct {
	script func(Request) string
	parse  func(request Request, out string, snapshot *Snapshot)
}

var sections = []section{statsSection, toolsSection, sessionsSection}

func Run(limit time.Duration, request Request, argv []string) (Snapshot, error) {
	if len(argv) == 0 {
		return Snapshot{}, errors.New("inspect: no command to run")
	}
	out, err := run.NonInteractive(limit, script(request), argv[0], argv[1:]...)
	if err != nil {
		return Snapshot{}, err
	}
	return Parse(out, request), nil
}

func script(request Request) string {
	lines := []string{stats.Preamble()}
	for _, s := range sections {
		lines = append(lines, s.script(request))
	}
	return strings.Join(append(lines, "true"), "\n")
}

func Parse(out string, request Request) Snapshot {
	var snapshot Snapshot
	for _, s := range sections {
		s.parse(request, out, &snapshot)
	}
	return snapshot
}

func tagged(out, tag string) []string {
	var rests []string
	for _, line := range strings.Split(out, "\n") {
		if rest, ok := strings.CutPrefix(line, tag+" "); ok {
			rests = append(rests, rest)
		}
	}
	return rests
}
