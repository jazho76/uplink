package tui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/jazho76/uplink/internal/humanize"
	"github.com/jazho76/uplink/internal/target"
	"github.com/jazho76/uplink/internal/ui"
)

const (
	minWidth  = 40
	minHeight = 14
)

var (
	spinnerStyle = lipgloss.NewStyle().Foreground(ui.Cyan)

	titleStyle   = lipgloss.NewStyle().Bold(true).Foreground(ui.Cyan)
	pointerStyle = lipgloss.NewStyle().Foreground(ui.Cyan)
	selectedRow  = lipgloss.NewStyle().Foreground(ui.Fg).Bold(true)
	dimRow       = lipgloss.NewStyle().Foreground(ui.Fg)
	autoMarker   = lipgloss.NewStyle().Foreground(ui.Magenta)
	modeMarker   = lipgloss.NewStyle().Foreground(ui.Yellow)

	keyStyle       = lipgloss.NewStyle().Foreground(ui.Magenta)
	footerStyle    = lipgloss.NewStyle().Foreground(ui.Comment)
	statusStyle    = lipgloss.NewStyle().Foreground(ui.Yellow)
	labelStyle     = lipgloss.NewStyle().Foreground(ui.Comment)
	sectionStyle   = lipgloss.NewStyle().Foreground(ui.Blue)
	detailLogStyle = lipgloss.NewStyle().Foreground(ui.Comment)
	valueStyle     = lipgloss.NewStyle().Foreground(ui.Fg)

	hostGlyph    = lipgloss.NewStyle().Foreground(ui.Cyan).Render("⬢")
	runningGlyph = lipgloss.NewStyle().Foreground(ui.Green).Render("●")
	idleGlyph    = lipgloss.NewStyle().Foreground(ui.Comment).Render("○")

	listBorder = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ui.Comment).
			Padding(0, 1)
	previewBorder = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ui.Comment).
			Padding(0, 1)
	hostBarBorder = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ui.Comment).
			Padding(0, 1)
)

func glyph(it item) string {
	switch {
	case it.t.Provider == target.ProviderLocal:
		return hostGlyph
	case it.running():
		return runningGlyph
	default:
		return idleGlyph
	}
}

func (m model) View() string {
	if m.width == 0 {
		return "loading..."
	}
	if m.width < minWidth || m.height < minHeight {
		return fmt.Sprintf("terminal too small (min %d×%d)", minWidth, minHeight)
	}

	if m.screen == screenLogs {
		return m.renderLogs()
	}

	listW := m.width * 38 / 100
	if listW < 20 {
		listW = 20
	}
	previewW := m.width - listW - 6
	bodyH := m.height - 7
	if bodyH < 5 {
		bodyH = 5
	}

	list := listBorder.Width(listW).Height(bodyH).Render(clampBlock(m.renderList(), listW-4, bodyH))
	preview := previewBorder.Width(previewW).Height(bodyH).Render(m.renderPreview(previewW, bodyH))
	body := lipgloss.JoinHorizontal(lipgloss.Top, list, preview)

	return lipgloss.JoinVertical(lipgloss.Left, body, m.renderHostBar(), m.renderFooter())
}

func (m model) renderList() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("uplink") + "\n\n")

	section := ""
	for i, it := range m.items {
		if it.t.Section != section {
			section = it.t.Section
			if section != "" {
				fmt.Fprintf(&b, "   %s %s %s\n",
					footerStyle.Render("──"), sectionStyle.Render(section), footerStyle.Render("──"))
			}
		}

		label := " "
		if i < 9 {
			label = keyStyle.Render(strconv.Itoa(i + 1))
		}
		marker := "  "
		name := dimRow.Render(it.name())
		if i == m.cursor {
			marker = pointerStyle.Render("▌ ")
			name = selectedRow.Render(it.name())
		}
		trailing := ""
		if i == m.cursor && m.modeIdx != 0 {
			trailing += " " + modeMarker.Render("["+m.mode().Name+"]")
		}
		if it.autostart {
			trailing += " " + autoMarker.Render("↻")
		}
		if verb := m.tasks[it.name()]; verb != "" {
			trailing += " " + m.spinner.View() + " " + labelStyle.Render(verb)
		}
		fmt.Fprintf(&b, "%s %s%s %s%s\n", label, marker, glyph(it), name, trailing)
	}
	return b.String()
}

func (m model) renderPreview(width, height int) string {
	cw := width - 4
	if cw < 10 {
		cw = 10
	}

	it := m.selected()
	if it.name() == "" {
		return ""
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%s  %s %s\n\n", titleStyle.Render(it.name()), glyph(it), string(it.t.Status))

	if n := len(it.t.Modes); n > 1 {
		kv(&b, "mode", fmt.Sprintf("%s   %s", modeMarker.Render(m.mode().Name),
			labelStyle.Render(fmt.Sprintf("%d of %d", m.modeIdx+1, n))))
	}
	for _, f := range it.t.Detail {
		kv(&b, f.Key, f.Value)
	}
	if it.caps.autostart {
		kv(&b, "auto", autostartLabel(it))
	}

	if it.worthProbing() {
		b.WriteString("\n" + rule("live", cw))
		m.renderLive(&b, it.name())
	}

	if m.logPeek != "" {
		used := strings.Count(b.String(), "\n")
		fit := height - used - 2
		if fit > 0 {
			lines := strings.Split(m.logPeek, "\n")
			if len(lines) > fit {
				lines = lines[len(lines)-fit:]
			}
			b.WriteString("\n" + rule("logs", cw))
			for _, line := range lines {
				b.WriteString(detailLogStyle.Render(clip(line, cw)) + "\n")
			}
		}
	}
	return clampBlock(b.String(), cw, height)
}

func (m model) renderLive(b *strings.Builder, name string) {
	e, ok := m.live[name]
	if !ok {
		b.WriteString(labelStyle.Render("…") + "\n")
		return
	}
	if e.err {
		kv(b, "load", labelStyle.Render("unavailable"))
		return
	}
	s := e.stats
	kv(b, "load", s.Load)
	kv(b, "ram", fmt.Sprintf("%s / %s", humanize.Bytes(s.MemUsed), humanize.Bytes(s.MemTotal)))
	kv(b, "used", fmt.Sprintf("%s / %s", humanize.Bytes(s.DiskUsed), humanize.Bytes(s.DiskTotal)))
	if up := humanize.Duration(s.Uptime); up != "" {
		kv(b, "up", up)
	}
}

func autostartLabel(it item) string {
	if it.autostart {
		return autoMarker.Render("↻ on")
	}
	return "off"
}

func kv(b *strings.Builder, key, value string) {
	fmt.Fprintf(b, "%s %s\n", labelStyle.Render(fmt.Sprintf("%-6s", key)), value)
}

func (m model) renderLogs() string {
	header := titleStyle.Render("logs: " + m.logName)
	footer := keyStyle.Render("esc") + " " + footerStyle.Render("back")

	bodyH := m.height - 2
	if bodyH < 1 {
		bodyH = 1
	}
	lines := strings.Split(m.logView, "\n")
	if len(lines) > bodyH {
		lines = lines[len(lines)-bodyH:]
	}
	for i, line := range lines {
		lines[i] = detailLogStyle.Render(clip(line, m.width))
	}
	body := strings.Join(lines, "\n")

	return lipgloss.JoinVertical(lipgloss.Left, truncate(header, m.width), body, footer)
}

func (m model) renderFooter() string {
	if m.screen == screenConfirm {
		prompt := statusStyle.Render(fmt.Sprintf("delete %s? type the name: ", m.selected().name()))
		return prompt + m.input.View() + "\n"
	}

	it := m.selected()
	pairs := [][2]string{{"↵", "connect"}}
	if len(it.t.Modes) > 1 {
		pairs = append(pairs, [2]string{"tab", "mode"})
	}
	if it.caps.tail {
		pairs = append(pairs, [2]string{"^l", "logs"})
	}
	if it.caps.lifecycle {
		pairs = append(pairs, [2]string{"^s", "stop"}, [2]string{"^r", "restart"})
	}
	if it.caps.autostart {
		pairs = append(pairs, [2]string{"^a", "auto"})
	}
	if it.caps.lifecycle {
		pairs = append(pairs, [2]string{"^x", "del"})
	}
	pairs = append(pairs, [2]string{"q", "quit"})

	var parts []string
	for _, p := range pairs {
		parts = append(parts, keyStyle.Render(p[0])+" "+footerStyle.Render(p[1]))
	}
	keys := truncate(strings.Join(parts, footerStyle.Render("  ")), m.width)
	return keys + "\n" + truncate(statusStyle.Render(oneLine(m.status)), m.width)
}

func (m model) renderHostBar() string {
	h := m.hostStats
	var running, vcpu int
	var committedMem uint64
	for _, it := range m.items {
		if !it.running() {
			continue
		}
		if it.t.CPUs == 0 && it.t.Memory == 0 {
			continue
		}
		running++
		vcpu += it.t.CPUs
		committedMem += it.t.Memory
	}

	seg := func(label, value string) string {
		return labelStyle.Render(label+" ") + valueStyle.Render(value)
	}
	gap := footerStyle.Render("   ")

	left := strings.Join([]string{
		titleStyle.Render(m.hostName),
		seg("cores", strconv.Itoa(h.Cores)),
		seg("load", h.Load),
		seg("ram", fmt.Sprintf("%s/%s", humanize.Bytes(h.MemUsed), humanize.Bytes(h.MemTotal))),
	}, gap)

	committed := seg("committed", fmt.Sprintf("%d vCPU / %s across %d running", vcpu, humanize.Bytes(committedMem), running))

	content := truncate(left+gap+committed, m.width-4)
	return hostBarBorder.Width(m.width - 2).Render(content)
}

func rule(title string, width int) string {
	head := sectionStyle.Render(title) + " "
	dashes := width - lipgloss.Width(head)
	if dashes < 0 {
		dashes = 0
	}
	return head + footerStyle.Render(strings.Repeat("─", dashes)) + "\n"
}

func oneLine(s string) string {
	return strings.ReplaceAll(strings.TrimRight(s, "\n"), "\n", "; ")
}

func clip(s string, width int) string {
	s = strings.Map(func(r rune) rune {
		switch {
		case r == '\t':
			return ' '
		case r < 0x20 || r == 0x7f:
			return -1
		default:
			return r
		}
	}, s)
	r := []rune(s)
	if len(r) > width {
		r = r[:width]
	}
	return string(r)
}

func truncate(s string, width int) string {
	if width < 0 {
		width = 0
	}
	return ansi.Truncate(s, width, "")
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
