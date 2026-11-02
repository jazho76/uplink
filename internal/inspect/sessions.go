package inspect

import (
	"slices"
	"strconv"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

const (
	sessionTag = "session"
	windowTag  = "window"
)

type Window struct {
	Name  string
	Panes int
}

type Session struct {
	Name     string
	Attached bool
	Windows  []Window
}

var sessionsSection = section{
	script: func(Request) string {
		return `$limit tmux list-sessions -F '` + sessionTag + ` #{session_id} #{session_attached} #{session_name}' </dev/null 2>/dev/null
$limit tmux list-windows -a -F '` + windowTag + ` #{session_id} #{window_panes} #{window_name}' </dev/null 2>/dev/null`
	},
	parse: func(_ Request, out string, snapshot *Snapshot) {
		var ids []string
		for _, line := range tagged(out, sessionTag) {
			if id, attached, name, ok := parseRow(line); ok {
				ids = append(ids, id)
				snapshot.Sessions = append(snapshot.Sessions, Session{Name: name, Attached: attached > 0})
			}
		}
		for _, line := range tagged(out, windowTag) {
			id, panes, name, ok := parseRow(line)
			owner := slices.Index(ids, id)
			if !ok || owner < 0 {
				continue
			}
			session := &snapshot.Sessions[owner]
			session.Windows = append(session.Windows, Window{Name: name, Panes: panes})
		}
	},
}

func parseRow(line string) (id string, count int, text string, ok bool) {
	id, rest, _ := strings.Cut(line, " ")
	countField, text, _ := strings.Cut(rest, " ")
	count, err := strconv.Atoi(countField)
	text = ansi.Strip(text)
	return id, count, text, err == nil && strings.HasPrefix(id, "$") && text != ""
}
