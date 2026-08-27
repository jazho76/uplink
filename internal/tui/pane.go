package tui

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/charmbracelet/lipgloss"
	"github.com/jazho76/uplink/internal/target"
)

const (
	minPaneRows      = borderCells + 1
	reservedPaneKeys = "qjk"
)

type pane struct {
	section string
	key     rune
	items   []item
	cursor  int
}

func sectionOf(t target.Target) string {
	if t.Section != "" {
		return t.Section
	}
	return t.Provider
}

func groupPanes(items []item) []pane {
	var panes []pane
	at := map[string]int{}
	for _, it := range items {
		section := sectionOf(it.t)
		i, grouped := at[section]
		if !grouped {
			i = len(panes)
			at[section] = i
			panes = append(panes, pane{section: section})
		}
		panes[i].items = append(panes[i].items, it)
	}
	assignPaneKeys(panes)
	assignItemKeys(panes)
	return panes
}

func assignPaneKeys(panes []pane) {
	taken := reservedPaneKeys
	for i := range panes {
		panes[i].key, taken = claimLetter(panes[i].section, taken)
	}
}

func assignItemKeys(panes []pane) {
	for i := range panes {
		taken := reservedPaneKeys + otherPaneKeys(panes, i)
		for j := range panes[i].items {
			panes[i].items[j].key, taken = claimLetter(panes[i].items[j].name(), taken)
		}
	}
}

func claimLetter(word, taken string) (rune, string) {
	key := firstFreeLetter(word, taken)
	if key == 0 {
		return 0, taken
	}
	return key, taken + string(key)
}

func otherPaneKeys(panes []pane, skip int) string {
	var keys string
	for i, p := range panes {
		if i != skip && p.key != 0 {
			keys += string(p.key)
		}
	}
	return keys
}

func firstFreeLetter(word, taken string) rune {
	for _, r := range word {
		if r = unicode.ToLower(r); r >= 'a' && r <= 'z' && !strings.ContainsRune(taken, r) {
			return r
		}
	}
	return 0
}

func paneForKey(panes []pane, key string) (int, bool) {
	for i, p := range panes {
		if p.key != 0 && string(p.key) == key {
			return i, true
		}
	}
	return 0, false
}

func itemForKey(p pane, key string) (int, bool) {
	for i, it := range p.items {
		if it.key != 0 && string(it.key) == key {
			return i, true
		}
	}
	return 0, false
}

func distributeRows(panes []pane, total int) []int {
	rows := make([]int, len(panes))
	if len(panes) == 0 {
		return rows
	}

	sum := 0
	for i, p := range panes {
		rows[i] = max(len(p.items)+borderCells, minPaneRows)
		sum += rows[i]
	}

	if slack := total - sum; slack > 0 {
		share, extra := slack/len(rows), slack%len(rows)
		for i := range rows {
			rows[i] += share
			if i < extra {
				rows[i]++
			}
		}
		return rows
	}

	for sum > total && shrinkTallest(rows) {
		sum--
	}
	return rows
}

func shrinkTallest(rows []int) bool {
	tallest := 0
	for i := range rows {
		if rows[i] > rows[tallest] {
			tallest = i
		}
	}
	if rows[tallest] <= minPaneRows {
		return false
	}
	rows[tallest]--
	return true
}

func scrollOffset(cursor, count, rows int) int {
	if rows < 1 || count <= rows {
		return 0
	}
	return min(max(cursor-rows+1, 0), count-rows)
}

func scrollHint(start, count, rows int) string {
	if count <= rows {
		return ""
	}
	return fmt.Sprintf("%d-%d of %d", start+1, start+rows, count)
}

func paneSummary(p pane) string {
	if len(p.items) == 0 || !allStatusesKnown(p.items) {
		return ""
	}
	running := 0
	for _, it := range p.items {
		if it.running() {
			running++
		}
	}
	return fmt.Sprintf("%d up", running)
}

func allStatusesKnown(items []item) bool {
	for _, it := range items {
		if it.t.Status == target.StatusUnknown {
			return false
		}
	}
	return true
}

func (m model) renderPanes(width, height int) string {
	if len(m.panes) == 0 {
		return ""
	}
	rows := distributeRows(m.panes, height)
	blocks := make([]string, 0, len(m.panes))
	for i, p := range m.panes {
		blocks = append(blocks, m.renderPane(p, i == m.focus, width, rows[i]))
	}
	return clampBlock(strings.Join(blocks, "\n"), width, height)
}

func (m model) renderPane(p pane, focused bool, width, height int) string {
	border, title := paneBorder, paneTitle
	if focused {
		border, title = paneBorderFocus, paneTitleFocus
	}

	rows := max(height-borderCells, 1)
	inner := max(width-chromeCells, 1)
	start := scrollOffset(p.cursor, len(p.items), rows)

	summary := labelStyle.Render(paneSummary(p))
	lines := []string{paneEdge("╭", "╮", accentLetter(p.section, p.key, title), summary, width, border)}
	for i := start; i < start+rows; i++ {
		row := ""
		if i < len(p.items) {
			row = m.paneRow(p.items[i], i == p.cursor, focused, inner)
		}
		lines = append(lines, border.Render("│")+" "+pad(row, inner)+" "+border.Render("│"))
	}

	hint := labelStyle.Render(scrollHint(start, len(p.items), rows))
	return strings.Join(append(lines, paneEdge("╰", "╯", "", hint, width, border)), "\n")
}

func paneEdge(head, tail, left, right string, width int, border lipgloss.Style) string {
	l, r := border.Render(head), border.Render(tail)
	if visible(left) {
		l += border.Render(" ") + left + border.Render(" ")
	}
	if visible(right) {
		r = border.Render(" ") + right + border.Render(" ") + r
	}

	fill := width - lipgloss.Width(l) - lipgloss.Width(r)
	switch {
	case fill >= 0:
		return l + border.Render(strings.Repeat("─", fill)) + r
	case visible(right):
		return paneEdge(head, tail, left, "", width, border)
	default:
		return truncate(l, width-lipgloss.Width(r)) + r
	}
}

func accentLetter(text string, key rune, base lipgloss.Style) string {
	for i, r := range text {
		if key != 0 && unicode.ToLower(r) == key {
			head, tail := text[:i], text[i+len(string(r)):]
			return base.Render(head) + base.Underline(true).Render(string(r)) + base.Render(tail)
		}
	}
	return base.Render(text)
}

func (m model) paneRow(it item, selected, focused bool, width int) string {
	nameStyle, marker := dimRow, "  "
	if selected {
		nameStyle = selectedRow
		if focused {
			marker = pointerStyle.Render("▌ ")
		}
	}

	key := rune(0)
	if focused {
		key = it.key
	}
	name := accentLetter(it.name(), key, nameStyle)

	var status []string
	if selected && focused && m.modeIdx != 0 {
		status = append(status, modeMarker.Render("["+m.mode().Name+"]"))
	}
	if it.autostart {
		status = append(status, autoMarker.Render("↻"))
	}
	if verb := m.tasks[it.name()]; verb != "" {
		status = append(status, m.spinner.View()+" "+labelStyle.Render(verb))
	}

	return spread(marker+glyph(it)+" "+name, strings.Join(status, " "), width)
}

func pad(s string, width int) string {
	s = truncate(s, width)
	if gap := width - lipgloss.Width(s); gap > 0 {
		s += strings.Repeat(" ", gap)
	}
	return s
}
