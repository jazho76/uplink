package target

const ToolJournal = "journalctl"

type Environment struct {
	OS       string
	Readings []Reading
	Tools    []string
	Sessions []Session
}

type Session struct {
	Tool     string
	Name     string
	Attached bool
	Windows  []Window
}

type Window struct {
	Name  string
	Panes int
}
