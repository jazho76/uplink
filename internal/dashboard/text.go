package dashboard

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

const fieldGap = 2

type field struct{ key, value string }

func widestKey(fields []field) int {
	widest := 0
	for _, f := range fields {
		widest = max(widest, lipgloss.Width(f.key))
	}
	return widest
}

func renderFields(fields []field, width int) string {
	gutter := widestKey(fields)

	var b strings.Builder
	for _, f := range fields {
		label := labelStyle.Render(pad(f.key, gutter, plainStyle))
		b.WriteString(truncate(label+strings.Repeat(" ", fieldGap)+f.value, width) + "\n")
	}
	return b.String()
}

func rule(title string, width int) string {
	head := ruleStyle.Render(title) + " "
	dashes := max(width-lipgloss.Width(head), 0)
	return head + footerStyle.Render(strings.Repeat("─", dashes)) + "\n"
}

func visible(s string) bool { return lipgloss.Width(s) > 0 }

func spread(left, right string, width int, base lipgloss.Style) string {
	gap := width - lipgloss.Width(left) - lipgloss.Width(right)
	if !visible(right) || gap < 1 {
		return pad(left, width, base)
	}
	return left + base.Render(strings.Repeat(" ", gap)) + right
}

func pad(s string, width int, base lipgloss.Style) string {
	s = truncate(s, width)
	if gap := width - lipgloss.Width(s); gap > 0 {
		s += base.Render(strings.Repeat(" ", gap))
	}
	return s
}

func layer(base, style lipgloss.Style) lipgloss.Style {
	return base.Foreground(style.GetForeground()).Bold(style.GetBold())
}

func truncate(s string, width int) string {
	return ansi.Truncate(s, max(width, 0), "")
}

func clampBlock(s string, width, height int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) > height {
		lines = lines[:height]
	}
	for i, line := range lines {
		lines[i] = truncate(line, width)
	}
	return strings.Join(lines, "\n")
}
