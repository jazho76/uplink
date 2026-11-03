package inspection

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/jazho76/uplink/internal/process"
	"github.com/jazho76/uplink/internal/shell"
	"github.com/jazho76/uplink/internal/target"
)

const (
	commandTimeout = 5 * time.Second
	endTag         = "section"
)

type section struct {
	name    string
	missing func(*target.MissingSections)
	script  func(target.StatsPolicy) string
	parse   func(policy target.StatsPolicy, out string, environment *target.Environment)
}

var sections = []section{osSection, logsSection, toolsSection, sessionsSection, statsSection}

func Run(ctx context.Context, limit time.Duration, policy target.StatsPolicy, argv []string) (target.Environment, error) {
	if len(argv) == 0 {
		return target.Environment{}, errors.New("inspection: no command to run")
	}
	out, err := process.NonInteractive(ctx, limit, script(policy), argv[0], argv[1:]...)
	if err != nil && !anySectionEnded(out) {
		return target.Environment{}, err
	}
	return Parse(out, policy), nil
}

func anySectionEnded(out string) bool {
	return slices.ContainsFunc(sections, func(s section) bool { return s.ended(out) })
}

func (s section) ended(out string) bool {
	return slices.Contains(strings.Split(strings.ReplaceAll(out, "\r", ""), "\n"), s.endLine())
}

func (s section) endLine() string {
	return endTag + " " + s.name + " end"
}

func script(policy target.StatsPolicy) string {
	lines := []string{shell.Preamble(commandTimeout)}
	for _, s := range sections {
		lines = append(lines, s.script(policy), "echo "+shell.SingleQuote(s.endLine()))
	}
	return strings.Join(append(lines, "true"), "\n")
}

func Parse(out string, policy target.StatsPolicy) target.Environment {
	out = strings.ReplaceAll(out, "\r", "")
	var environment target.Environment
	for _, s := range sections {
		switch {
		case s.ended(out):
			s.parse(policy, out, &environment)
		case s.missing != nil:
			s.missing(&environment.Missing)
		}
	}
	return environment
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
