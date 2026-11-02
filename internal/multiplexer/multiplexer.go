package multiplexer

type Window struct {
	Name  string
	Panes int
}

type Session struct {
	Tool     string
	Name     string
	Attached bool
	Windows  []Window
}

type Multiplexer interface {
	Tool() string
	Attach() string
	Script() string
	Parse(rows []string) []Session
}

var All = []Multiplexer{Tmux{}}
