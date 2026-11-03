package dashboard

import (
	"fmt"
	"strings"

	"github.com/jazho76/uplink/internal/target"
)

func (m model) renderPreview(width, height int) string {
	return boxStyle.Width(width - borderCells).Height(height - borderCells).
		Render(m.previewBody(width-chromeCells, height-borderCells))
}

func (m model) previewBody(width, height int) string {
	it := m.selected()
	if it.empty() {
		return ""
	}

	var b strings.Builder
	header := titleStyle.Render(it.name()) + "  " + m.glyph(it, plainStyle) + " " + m.statusOf(it.target)
	b.WriteString(spread(header, m.modeTag(it), width, plainStyle) + "\n")
	if observation := m.state.Observation(it.id()); observation.Status == target.StatusUnknown && observation.Reason != "" {
		b.WriteString(labelStyle.Render(truncate(observation.Reason, width)) + "\n")
	}
	if connection := m.state.Connection(); connection.Target == it.id() && connection.Cancellable() {
		b.WriteString(modeMarker.Render("connecting") + "  " + keyStyle.Render("esc") + " " + labelStyle.Render("cancel") + "\n")
	}

	if m.showsStats(it.target) {
		b.WriteString("\n" + rule("live", width))
		b.WriteString(m.renderStats(it.id(), width))
	}

	if sessions := m.renderSessions(it.id(), width); sessions != "" {
		b.WriteString("\n" + rule("sessions", width))
		b.WriteString(sessions)
	}

	used := strings.Count(b.String(), "\n")
	b.WriteString(logTail(m.state.Logs(it.target), width, height-used-2))
	return clampBlock(b.String(), width, height)
}

func (m model) modeTag(it item) string {
	n := len(m.state.Modes(it.target))
	if n < 2 {
		return ""
	}
	name := m.state.Mode().Name
	if m.modeUnconfirmed(it.target) {
		name += "?"
	}
	return modeMarker.Render(name) + "  " + labelStyle.Render(fmt.Sprintf("%d/%d", m.state.ModeIndex()+1, n))
}

func (m model) modeUnconfirmed(t target.Target) bool {
	return m.state.Mode().Requires != "" && !m.state.Observation(t.ID).Tools.Known && m.statusOf(t) == string(target.StatusDown)
}
