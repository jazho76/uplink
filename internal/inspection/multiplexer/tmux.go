package multiplexer

import (
	"github.com/jazho76/uplink/internal/target"
	"slices"
	"strconv"
	"strings"
)

const (
	tmuxSessionTag = "session"
	tmuxWindowTag  = "window"
)

type Tmux struct{}

func (Tmux) Tool() string { return "tmux" }

func (Tmux) Attach() string { return "tmux new-session -A -s 0" }

func (Tmux) Script() string {
	return `$limit tmux list-sessions -F '` + tmuxSessionTag + ` #{session_id} #{session_attached} #{session_name}' </dev/null
$limit tmux list-windows -a -F '` + tmuxWindowTag + ` #{session_id} #{window_panes} #{window_name}' </dev/null`
}

func (Tmux) Parse(rows []string) []target.Session {
	var (
		sessions []target.Session
		ids      []string
	)
	for _, row := range rows {
		if rest, ok := strings.CutPrefix(row, tmuxSessionTag+" "); ok {
			if id, attached, name, ok := parseTmuxRow(rest); ok {
				ids = append(ids, id)
				sessions = append(sessions, target.Session{Name: name, Attached: attached > 0})
			}
		}
	}
	for _, row := range rows {
		if rest, ok := strings.CutPrefix(row, tmuxWindowTag+" "); ok {
			id, panes, name, ok := parseTmuxRow(rest)
			owner := slices.Index(ids, id)
			if !ok || owner < 0 {
				continue
			}
			session := &sessions[owner]
			session.Windows = append(session.Windows, target.Window{Name: name, Panes: panes})
		}
	}
	return sessions
}

func parseTmuxRow(row string) (id string, count int, text string, ok bool) {
	id, rest, _ := strings.Cut(row, " ")
	countField, text, _ := strings.Cut(rest, " ")
	count, err := strconv.Atoi(countField)
	return id, count, text, err == nil && strings.HasPrefix(id, "$") && text != ""
}
