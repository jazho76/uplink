package target

import (
	"fmt"
	"slices"
	"strings"

	"github.com/jazho76/uplink/internal/probe"
)

const (
	DefaultGroup = "ungrouped"
	loginShell   = `exec "${SHELL:-/bin/sh}"`
)

var localVia = []string{"sh", "-c"}

type Mode struct {
	Name string
	Argv []string
}

type Target struct {
	Name   string
	Group  string
	Via    []string
	Exec   []string
	Status string
	Up     string
	Down   string
	Logs   string
	Watch  bool
	Stats  probe.Metrics
	Modes  []Mode
}

func (t Target) DefaultMode() Mode {
	if len(t.Modes) == 0 {
		return Mode{}
	}
	return t.Modes[0]
}

func (t Target) Mode(name string) (Mode, bool) {
	for _, m := range t.Modes {
		if m.Name == name {
			return m, true
		}
	}
	return Mode{}, false
}

func (t Target) ModeNames() []string {
	names := make([]string, 0, len(t.Modes))
	for _, m := range t.Modes {
		names = append(names, m.Name)
	}
	return names
}

func (t Target) ArgvFor(command string) []string {
	return withCommand(t.Via, command)
}

func (t Target) ExecArgvFor(command string) []string {
	if len(t.Exec) == 0 {
		return t.ArgvFor(command)
	}
	return withCommand(t.Exec, command)
}

func withCommand(prefix []string, command string) []string {
	if len(prefix) == 0 {
		prefix = localVia
	}
	return append(slices.Clone(prefix), command)
}

func (m Mode) Command() string { return Quote(m.Argv) }

func Quote(argv []string) string {
	parts := make([]string, 0, len(argv))
	for _, a := range argv {
		if a == "" || strings.ContainsAny(a, " \t'\"") {
			parts = append(parts, "'"+strings.ReplaceAll(a, "'", `'\''`)+"'")
			continue
		}
		parts = append(parts, a)
	}
	return strings.Join(parts, " ")
}

func Resolve(targets []Target, name string) (Target, error) {
	if i := slices.IndexFunc(targets, func(t Target) bool { return t.Name == name }); i >= 0 {
		return targets[i], nil
	}
	names := make([]string, 0, len(targets))
	for _, t := range targets {
		names = append(names, t.Name)
	}
	slices.Sort(names)
	return Target{}, fmt.Errorf("no such target %q; known targets: %s", name, strings.Join(names, ", "))
}
