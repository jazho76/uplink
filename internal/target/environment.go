package target

const ToolJournal = "journalctl"

type Environment struct {
	OS       string
	Readings []Reading
	Tools    []string
	Sessions []Session
	Missing  MissingSections
}

type MissingSections struct {
	Stats    bool
	Tools    bool
	Sessions bool
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
