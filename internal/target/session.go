package target

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
