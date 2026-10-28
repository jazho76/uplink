package target

import (
	"fmt"
	"os"
	"slices"
	"strings"
)

type Kind string

const (
	KindLocal  Kind = "local"
	KindVM     Kind = "vm"
	KindRemote Kind = "remote"
)

var Kinds = []Kind{KindLocal, KindVM, KindRemote}

type Mode struct {
	Name string
	Argv []string
	Back bool
}

type Target struct {
	Name  string
	Kind  Kind
	Via   []string
	Logs  string
	Stats bool
	Modes []Mode
}

func (t Target) Local() bool { return t.Kind == KindLocal }

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
	if t.Local() {
		shell := userShell()
		if command == "" {
			return []string{shell}
		}
		return []string{shell, "-c", command}
	}
	argv := append([]string(nil), t.Via...)
	if command == "" {
		return argv
	}
	return append(argv, command)
}

func userShell() string {
	if shell := os.Getenv("SHELL"); shell != "" {
		return shell
	}
	return "/bin/sh"
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
