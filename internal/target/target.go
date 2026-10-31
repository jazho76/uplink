package target

import (
	"slices"
	"strings"

	"github.com/jazho76/uplink/internal/stats"
)

const (
	ConfigSource = "config"
	DefaultGroup = "ungrouped"
	loginShell   = `exec "${SHELL:-/bin/sh}"`
)

var localVia = []string{"sh", "-c"}

type Mode struct {
	Name string
	Argv []string
}

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

type Target struct {
	ID     ID
	Name   string
	Group  string
	Via    []string
	Exec   []string
	Status string
	Up     string
	Down   string
	Logs   string
	Watch  bool
	Stats  []stats.Metric
	Modes  []Mode
}

func (t Target) DefaultMode() Mode {
	if len(t.Modes) == 0 {
		return Mode{}
	}
	return t.Modes[0]
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
