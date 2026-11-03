package target

import (
	"errors"
	"regexp"
	"slices"
)

const (
	ConfigSource = "config"
	DefaultGroup = "ungrouped"
)

var namePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

var localPrefix = []string{"sh", "-c"}

type ID struct {
	Source string
	Key    string
}

func (id ID) String() string {
	if id.Source == ConfigSource {
		return id.Key
	}
	return id.Source + "/" + id.Key
}

func ValidateKey(key string) error {
	if key == "" {
		return errors.New("no name")
	}
	if !namePattern.MatchString(key) {
		return errors.New("name may only hold letters, digits, '.', '_' and '-'")
	}
	return nil
}

type Target struct {
	ID    ID
	Name  string
	Group string
	Reach Reach
	Check string
	Power *Power
	Logs  LogSource
	Watch bool
	Stats StatsPolicy
	Modes []Mode
}

func (t Target) StartCommand() string {
	if t.Power == nil {
		return ""
	}
	return t.Power.Up
}

func (t Target) StopCommand() string {
	if t.Power == nil {
		return ""
	}
	return t.Power.Down
}

type Reach struct {
	Interactive []string
	Background  []string
}

func (r Reach) Session(command string) []string {
	return withCommand(r.Interactive, command)
}

func (r Reach) Probe(command string) []string {
	if len(r.Background) > 0 {
		return withCommand(r.Background, command)
	}
	return r.Session(command)
}

func withCommand(prefix []string, command string) []string {
	if len(prefix) == 0 {
		prefix = localPrefix
	}
	return append(slices.Clone(prefix), command)
}

type Power struct {
	Up   string
	Down string
}

type LogSource struct {
	Command string
	Journal bool
}

type StatsPolicy struct {
	Builtin bool
	Metrics []Metric
}

type Mode struct {
	Name        string
	Argv        []string
	Multiplexer string
	Requires    string
}

func AvailableModes(t Target, tools Fact[[]string]) []Mode {
	var modes []Mode
	for i, mode := range t.Modes {
		found := tools.Known && slices.Contains(tools.Value, mode.Requires)
		assumedDefault := !tools.Known && i == 0
		if mode.Requires == "" || found || assumedDefault {
			modes = append(modes, mode)
		}
	}
	return modes
}
