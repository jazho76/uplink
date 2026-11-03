package dashboard

import (
	"fmt"
	"slices"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/jazho76/uplink/internal/target"
)

const (
	maxWindowName = 24

	attachedGlyph   = "●"
	windowSeparator = " · "
)

func (m model) modeSessions(id target.ID) []target.Session {
	found := m.state.Observation(id).Sessions
	tool := m.state.Mode().Multiplexer
	if !found.Current() || tool == "" {
		return nil
	}
	var sessions []target.Session
	for _, s := range found.Value {
		if s.Tool == tool {
			sessions = append(sessions, s)
		}
	}
	return sessions
}

func (m model) renderSessions(id target.ID, width int) string {
	sessions := m.modeSessions(id)
	if len(sessions) == 0 {
		return ""
	}
	fields := make([]field, len(sessions))
	for i, s := range sessions {
		fields[i].key = sessionMargin(s.Attached) + valueStyle.Render(truncate(s.Name, width/2))
	}
	room := width - widestKey(fields) - fieldGap
	for i, s := range sessions {
		fields[i].value = windowList(s.Windows, room)
	}
	return renderFields(fields, width)
}

func sessionMargin(attached bool) string {
	if attached {
		return attachedMarker.Render(attachedGlyph) + " "
	}
	return "  "
}

func windowList(windows []target.Window, width int) string {
	labels := make([]string, len(windows))
	for i, w := range windows {
		labels[i] = windowLabel(w, width)
	}
	gap := labelStyle.Render(windowSeparator)
	for shown := len(labels); shown >= 0; shown-- {
		parts := slices.Clone(labels[:shown])
		if hidden := len(labels) - shown; hidden > 0 {
			parts = append(parts, labelStyle.Render(fmt.Sprintf("+%d", hidden)))
		}
		if list := strings.Join(parts, gap); lipgloss.Width(list) <= width {
			return list
		}
	}
	return ""
}

func windowLabel(w target.Window, width int) string {
	suffix := ""
	if w.Panes > 1 {
		suffix = labelStyle.Render(fmt.Sprintf(" (%d)", w.Panes))
	}
	name := truncate(w.Name, min(maxWindowName, width-lipgloss.Width(suffix)))
	return valueStyle.Render(name) + suffix
}
