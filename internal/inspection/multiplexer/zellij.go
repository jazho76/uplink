package multiplexer

import (
	"cmp"
	"encoding/json"
	"github.com/jazho76/uplink/internal/target"
	"slices"
	"strings"
)

const (
	zellijSessionTag = "session"
	zellijClientTag  = "client"
	zellijPaneTag    = "pane"

	zellijClientHeader = "CLIENT_ID"
)

type Zellij struct{}

func (Zellij) Tool() string { return "zellij" }

func (Zellij) Attach() string { return "zellij attach -c 0" }

func (Zellij) Script() string {
	return `$limit zellij list-sessions --no-formatting --short </dev/null | while IFS= read -r name; do
  printf '%s\n' "` + zellijSessionTag + ` $name"
  $limit zellij --session "$name" action list-clients </dev/null | while IFS= read -r row; do printf '%s\n' "` + zellijClientTag + ` $row"; done
  $limit zellij --session "$name" action list-panes --json --tab </dev/null | while IFS= read -r row; do printf '%s\n' "` + zellijPaneTag + ` $row"; done
done`
}

func (Zellij) Parse(rows []string) []target.Session {
	var (
		sessions []target.Session
		panes    [][]string
	)
	for _, row := range rows {
		tag, rest, _ := strings.Cut(row, " ")
		if tag == zellijSessionTag {
			sessions = append(sessions, target.Session{Name: rest})
			panes = append(panes, nil)
			continue
		}
		if len(sessions) == 0 {
			continue
		}
		current := len(sessions) - 1
		switch tag {
		case zellijClientTag:
			if rest != "" && !strings.HasPrefix(rest, zellijClientHeader) {
				sessions[current].Attached = true
			}
		case zellijPaneTag:
			panes[current] = append(panes[current], rest)
		}
	}
	for i := range sessions {
		sessions[i].Windows = zellijWindows(panes[i])
	}
	slices.SortStableFunc(sessions, func(a, b target.Session) int { return cmp.Compare(a.Name, b.Name) })
	return sessions
}

type zellijPane struct {
	IsPlugin    bool   `json:"is_plugin"`
	TabID       int    `json:"tab_id"`
	TabPosition int    `json:"tab_position"`
	TabName     string `json:"tab_name"`
}

func zellijWindows(rows []string) []target.Window {
	var panes []zellijPane
	if err := json.Unmarshal([]byte(strings.Join(rows, "\n")), &panes); err != nil {
		return nil
	}

	var tabs []zellijPane
	var windows []target.Window
	for _, pane := range panes {
		if pane.IsPlugin {
			continue
		}
		tab := slices.IndexFunc(tabs, func(t zellijPane) bool { return t.TabID == pane.TabID })
		if tab < 0 {
			tab = len(tabs)
			tabs = append(tabs, pane)
			windows = append(windows, target.Window{Name: pane.TabName})
		}
		windows[tab].Panes++
	}

	order := make([]int, len(tabs))
	for i := range order {
		order[i] = i
	}
	slices.SortStableFunc(order, func(a, b int) int { return cmp.Compare(tabs[a].TabPosition, tabs[b].TabPosition) })
	ordered := make([]target.Window, 0, len(order))
	for _, i := range order {
		ordered = append(ordered, windows[i])
	}
	if len(ordered) == 0 {
		return nil
	}
	return ordered
}
