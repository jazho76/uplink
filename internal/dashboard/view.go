package dashboard

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/jazho76/uplink/internal/launcher"
	"github.com/jazho76/uplink/internal/target"
)

func (m model) View() string {
	if m.width == 0 {
		return "loading..."
	}
	if m.width < minWidth || m.height < minHeight {
		return fmt.Sprintf("terminal too small (min %d×%d)", minWidth, minHeight)
	}

	if m.state.LogsOpen() {
		return m.renderLogs()
	}

	l := computeLayout(m.width, m.height)
	body := lipgloss.JoinHorizontal(lipgloss.Top,
		m.renderPanes(l.paneColumnOuterW, l.bodyOuterH),
		m.renderPreview(l.previewOuterW, l.bodyOuterH))

	return lipgloss.JoinVertical(lipgloss.Left, body, m.renderFooter())
}

func (m model) renderFooter() string {
	it := m.selected()
	var pairs [][2]string
	if !it.empty() {
		pairs = append(pairs, [2]string{"↵", "connect"})
	}
	if len(m.state.Modes(it.target)) > 1 {
		pairs = append(pairs, [2]string{"tab", "mode"})
	}
	if m.state.HasLogs(it.target) {
		pairs = append(pairs, [2]string{"^l", "logs"})
	}
	if m.state.CanPower(it.target, launcher.OperationStarting) {
		pairs = append(pairs, [2]string{"^s", "start"})
	}
	if m.state.CanPower(it.target, launcher.OperationStopping) {
		pairs = append(pairs, [2]string{"^x", "stop"})
	}
	pairs = append(pairs, [2]string{"^e", "config"}, [2]string{"q", "quit"})

	var parts []string
	for _, p := range pairs {
		parts = append(parts, keyStyle.Render(p[0])+" "+footerStyle.Render(p[1]))
	}
	keys := truncate(strings.Join(parts, footerStyle.Render("  ")), m.width)
	return keys + "\n" + truncate(noticeStyle.Render(m.state.Notice()), m.width)
}

func (m model) glyph(it item, base lipgloss.Style) string {
	mark, color := glyphOf(m.statusOf(it.target))
	return base.Foreground(color).Render(mark)
}

func glyphOf(label string) (string, lipgloss.TerminalColor) {
	switch label {
	case string(target.StatusUp):
		return "●", colorGreen
	case string(target.StatusDown):
		return "○", colorComment
	case string(launcher.OperationStarting), string(launcher.OperationStopping):
		return "◐", colorYellow
	default:
		return "◌", colorComment
	}
}
