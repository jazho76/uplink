package inspect

import (
	"strconv"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

const sessionTag = "session"

type Session struct {
	Name     string
	Windows  int
	Attached bool
}

var sessionsSection = section{
	script: func(Request) string {
		return `$limit tmux list-sessions -F '` + sessionTag + ` #{session_windows} #{session_attached} #{session_name}' </dev/null 2>/dev/null`
	},
	parse: func(_ Request, out string, snapshot *Snapshot) {
		for _, fields := range tagged(out, sessionTag) {
			if session, ok := parseSession(fields); ok {
				snapshot.Sessions = append(snapshot.Sessions, session)
			}
		}
	},
}

func parseSession(fields string) (Session, bool) {
	windowsField, rest, _ := strings.Cut(fields, " ")
	attachedField, name, _ := strings.Cut(rest, " ")
	windows, windowsErr := strconv.Atoi(windowsField)
	attached, attachedErr := strconv.Atoi(attachedField)
	name = ansi.Strip(name)
	if windowsErr != nil || attachedErr != nil || name == "" {
		return Session{}, false
	}
	return Session{Name: name, Windows: windows, Attached: attached > 0}, true
}
