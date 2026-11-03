package inspection

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jazho76/uplink/internal/shell"
	"github.com/jazho76/uplink/internal/target"
)

const commandTimeout = 5 * time.Second

type section struct {
	script func(target.StatsPolicy) string
	parse  func(policy target.StatsPolicy, out string, environment *target.Environment)
}

var sections = []section{osSection, statsSection, toolsSection, sessionsSection}

func Run(ctx context.Context, limit time.Duration, policy target.StatsPolicy, argv []string) (target.Environment, error) {
	if len(argv) == 0 {
		return target.Environment{}, errors.New("inspection: no command to run")
	}
	out, err := shell.NonInteractive(ctx, limit, script(policy), argv[0], argv[1:]...)
	if err != nil {
		return target.Environment{}, err
	}
	return Parse(out, policy), nil
}

func script(policy target.StatsPolicy) string {
	lines := []string{shell.Preamble(commandTimeout)}
	for _, s := range sections {
		lines = append(lines, s.script(policy))
	}
	return strings.Join(append(lines, "true"), "\n")
}

func Parse(out string, policy target.StatsPolicy) target.Environment {
	var environment target.Environment
	for _, s := range sections {
		s.parse(policy, out, &environment)
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
