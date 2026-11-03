package dashboard

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func (m model) renderLogs() string {
	header := titleStyle.Render("logs: " + m.state.LogTarget().Name)
	footer := keyStyle.Render("esc") + " " + footerStyle.Render("back")

	body := labelStyle.Render("no log output")
	if m.state.LogText() != "" {
		bodyH := max(m.height-2, 1)
		lines := strings.Split(m.state.LogText(), "\n")
		if len(lines) > bodyH {
			lines = lines[len(lines)-bodyH:]
		}
		for i, line := range lines {
			lines[i] = detailLogStyle.Render(truncate(line, m.width))
		}
		body = strings.Join(lines, "\n")
	}

	return lipgloss.JoinVertical(lipgloss.Left, truncate(header, m.width), body, footer)
}
