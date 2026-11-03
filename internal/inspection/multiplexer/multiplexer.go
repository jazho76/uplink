package multiplexer

import "github.com/jazho76/uplink/internal/target"

type Multiplexer interface {
	Tool() string
	Attach() string
	Script() string
	Parse(rows []string) []target.Session
}

var All = []Multiplexer{Tmux{}, Zellij{}}

func Tools() []string {
	tools := make([]string, len(All))
	for i, m := range All {
		tools[i] = m.Tool()
	}
	return tools
}
