package tui

import (
	"fmt"
	"slices"
	"strings"
	"unicode"

	"github.com/charmbracelet/lipgloss"
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

func assignPaneKeys(panes []pane) {
	taken := reservedPaneKeys
	for i := range panes {
		if len(panes[i].items) == 0 {
			continue
		}
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
	i := slices.IndexFunc(panes, func(p pane) bool { return keyMatches(p.key, key) })
	return i, i >= 0
}

func itemForKey(p pane, key string) (int, bool) {
	i := slices.IndexFunc(p.items, func(it item) bool { return keyMatches(it.key, key) })
	return i, i >= 0
}

func keyMatches(assigned rune, pressed string) bool {
	return assigned != 0 && string(assigned) == pressed
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
		for i, extra := range shares(slack, len(panes)) {
			rows[i] += extra
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

func (m model) paneSummary(p pane) string {
	if len(p.items) == 0 {
		return ""
	}
	running := 0
	for _, it := range p.items {
		switch m.statusOf(it.t) {
		case statusUnknown:
			return ""
		case statusUp:
			running++
		}
	}
	return fmt.Sprintf("%d up", running)
}

func (m model) renderPanes(width, height int) string {
	if len(m.panes) == 0 {
		return emptyPane(width, height)
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
	interior := max(width-borderCells, 1)
	start := scrollOffset(p.cursor, len(p.items), rows)

	summary := labelStyle.Render(m.paneSummary(p))
	blank := strings.Repeat(" ", interior)
	side := border.Render(boxBorder.Left)
	lines := []string{paneEdge(boxBorder.TopLeft, boxBorder.TopRight,
		accentLetter(p.section, p.key, title), summary, width, border)}
	for i := start; i < start+rows; i++ {
		row := blank
		if i < len(p.items) {
			row = m.paneRow(p.items[i], i == p.cursor, focused, interior)
		}
		lines = append(lines, side+row+side)
	}

	hint := labelStyle.Render(scrollHint(start, len(p.items), rows))
	return strings.Join(append(lines,
		paneEdge(boxBorder.BottomLeft, boxBorder.BottomRight, "", hint, width, border)), "\n")
}

func paneEdge(head, tail, left, right string, width int, border lipgloss.Style) string {
	l, r := border.Render(head), border.Render(tail)
	space := border.Render(" ")
	if visible(left) {
		l += space + left + space
	}
	if visible(right) {
		r = space + right + space + r
	}

	fill := width - lipgloss.Width(l) - lipgloss.Width(r)
	switch {
	case fill >= 0:
		return l + border.Render(strings.Repeat(boxBorder.Top, fill)) + r
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
	base := rowBase(selected, focused)
	space := base.Render(" ")

	key := rune(0)
	if focused {
		key = it.key
	}
	name := layer(base, valueStyle).Bold(selected)
	left := m.glyph(it, base) + space + accentLetter(it.name(), key, name)

	badge := ""
	if selected && focused && m.modeIdx != 0 {
		badge = layer(base, modeMarker).Render("["+m.mode().Name+"]") + space
	}

	return spread(space+left, badge, width, base)
}

func rowBase(selected, focused bool) lipgloss.Style {
	if selected && focused {
		return rowFill
	}
	return plainStyle
}

func emptyPane(width, height int) string {
	return boxStyle.Width(width - borderCells).Height(height - borderCells).Render(
		valueStyle.Render("no targets") + "\n" +
			keyStyle.Render("^e") + " " + labelStyle.Render("to edit the config"))
}
