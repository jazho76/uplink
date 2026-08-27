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
	minPreviewTextW = 10

	gaugeGap        = 3
	minGaugeReading = 16
	liveGutter      = len("memory") + 2
	minLiveBar      = 8
	maxLiveBar      = 20
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

	paneBorder      = lipgloss.NewStyle().Foreground(ui.Comment)
	paneBorderFocus = lipgloss.NewStyle().Foreground(ui.Cyan)
	paneTitle       = lipgloss.NewStyle().Foreground(ui.Comment)
	paneTitleFocus  = lipgloss.NewStyle().Foreground(ui.Cyan).Bold(true)

	hostGlyph = lipgloss.NewStyle().Foreground(ui.Cyan).Render("⬢")

	vmRunningGlyph = lipgloss.NewStyle().Foreground(ui.Green).Render("●")
	vmStoppedGlyph = lipgloss.NewStyle().Foreground(ui.Comment).Render("○")

	remoteReachableGlyph   = lipgloss.NewStyle().Foreground(ui.Green).Render("◆")
	remoteUnprobedGlyph    = lipgloss.NewStyle().Foreground(ui.Comment).Render("◇")
	remoteUnreachableGlyph = lipgloss.NewStyle().Foreground(ui.Red).Render("×")

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
	switch it.t.Provider {
	case target.ProviderLocal:
		return hostGlyph
	case target.ProviderRemote:
		return remoteGlyph(it.t.Status)
	default:
		if it.running() {
			return vmRunningGlyph
		}
		return vmStoppedGlyph
	}
}

func remoteGlyph(status target.Status) string {
	switch status {
	case target.StatusRunning:
		return remoteReachableGlyph
	case target.StatusUnreachable:
		return remoteUnreachableGlyph
	default:
		return remoteUnprobedGlyph
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

	l := computeLayout(m.width, m.height)
	previewH := l.bodyOuterH - borderCells

	panes := m.renderPanes(l.paneColumnOuterW, l.bodyOuterH)
	preview := previewBorder.Width(l.previewOuterW - borderCells).Height(previewH).
		Render(m.renderPreview(l.previewOuterW-chromeCells, previewH))
	body := lipgloss.JoinHorizontal(lipgloss.Top, panes, preview)

	return lipgloss.JoinVertical(lipgloss.Left, body, m.renderHostBar(l.hostBarOuterW), m.renderFooter())
}

func (m model) renderPreview(width, height int) string {
	width = max(width, minPreviewTextW)

	it := m.selected()
	if it.name() == "" {
		return ""
	}

	var b strings.Builder
	header := titleStyle.Render(it.name()) + "  " + glyph(it) + " " + string(it.t.Status)
	b.WriteString(spread(header, labelStyle.Render(m.uptimeOf(it)), width) + "\n")

	if it.worthProbing() {
		b.WriteString("\n" + rule("live", width))
		b.WriteString(m.renderLive(it.name(), width))
	}

	b.WriteString("\n" + rule("spec", width))
	b.WriteString(renderFields(m.specFields(it), width))

	if m.logPeek != "" {
		used := strings.Count(b.String(), "\n")
		fit := height - used - 2
		if fit > 0 {
			lines := strings.Split(m.logPeek, "\n")
			if len(lines) > fit {
				lines = lines[len(lines)-fit:]
			}
			b.WriteString("\n" + rule("logs", width))
			for _, line := range lines {
				b.WriteString(detailLogStyle.Render(clip(line, width)) + "\n")
			}
		}
	}
	return clampBlock(b.String(), width, height)
}

func (m model) renderLive(name string, width int) string {
	e, ok := m.live[name]
	if !ok {
		return labelStyle.Render("…") + "\n"
	}
	if e.err {
		return renderFields([]field{{"load", labelStyle.Render("unavailable")}}, width)
	}

	s := e.stats
	bar := liveBarWidth(width)
	load := loadFraction(s.Load, s.Cores)
	memory := usedFraction(s.MemUsed, s.MemTotal)
	disk := usedFraction(s.DiskUsed, s.DiskTotal)

	return renderFields([]field{
		{"load", gauge(sparkline(e.history, bar, meterStyle(load)), fmt.Sprintf("%.2f", s.Load))},
		{"memory", gauge(meter(memory, bar), amount(s.MemUsed, s.MemTotal))},
		{"disk", gauge(meter(disk, bar), amount(s.DiskUsed, s.DiskTotal))},
	}, width)
}

func gauge(bar, reading string) string {
	if !visible(bar) {
		return valueStyle.Render(reading)
	}
	return bar + strings.Repeat(" ", gaugeGap) + valueStyle.Render(reading)
}

func amount(used, total uint64) string {
	return humanize.Bytes(used) + " / " + humanize.Bytes(total)
}

func liveBarWidth(width int) int {
	bar := min(max(width/3, minLiveBar), maxLiveBar)
	if !readingFits(width, bar) {
		return 0
	}
	return bar
}

func readingFits(width, bar int) bool {
	return width-liveGutter-bar-gaugeGap >= minGaugeReading
}

func (m model) uptimeOf(it item) string {
	e, ok := m.live[it.name()]
	if !it.worthProbing() || !ok || e.err {
		return ""
	}
	if up := humanize.Duration(e.stats.Uptime); up != "" {
		return "up " + up
	}
	return ""
}

func (m model) specFields(it item) []field {
	var fields []field
	if n := len(it.t.Modes); n > 1 {
		fields = append(fields, field{"mode", fmt.Sprintf("%s   %s", modeMarker.Render(m.mode().Name),
			labelStyle.Render(fmt.Sprintf("%d of %d", m.modeIdx+1, n)))})
	}
	for _, f := range it.t.Detail {
		fields = append(fields, field{f.Key, f.Value})
	}
	if it.caps.autostart {
		fields = append(fields, field{"auto", autostartLabel(it)})
	}
	return fields
}

func autostartLabel(it item) string {
	if it.autostart {
		return autoMarker.Render("↻ on")
	}
	return "off"
}

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
		label := labelStyle.Render(pad(f.key, gutter))
		b.WriteString(truncate(label+"  "+f.value, width) + "\n")
	}
	return b.String()
}

func (m model) renderLogs() string {
	header := titleStyle.Render("logs: " + m.logName)
	footer := keyStyle.Render("esc") + " " + footerStyle.Render("back")

	bodyH := max(m.height-2, 1)
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

func (m model) renderHostBar(width int) string {
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
		seg("load", fmt.Sprintf("%.2f", h.Load)),
		seg("ram", fmt.Sprintf("%s/%s", humanize.Bytes(h.MemUsed), humanize.Bytes(h.MemTotal))),
	}, gap)

	committed := seg("committed", fmt.Sprintf("%d vCPU / %s across %d running", vcpu, humanize.Bytes(committedMem), running))

	content := truncate(left+gap+committed, width-chromeCells)
	return hostBarBorder.Width(width - borderCells).Render(content)
}

func rule(title string, width int) string {
	head := sectionStyle.Render(title) + " "
	dashes := max(width-lipgloss.Width(head), 0)
	return head + footerStyle.Render(strings.Repeat("─", dashes)) + "\n"
}

func oneLine(s string) string {
	return strings.ReplaceAll(strings.TrimRight(s, "\n"), "\n", "; ")
}

func clip(s string, width int) string {
	return truncate(stripControl(s), width)
}

func stripControl(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r == '\t':
			return ' '
		case r < 0x20 || r == 0x7f:
			return -1
		default:
			return r
		}
	}, s)
}

func visible(s string) bool { return lipgloss.Width(s) > 0 }

func spread(left, right string, width int) string {
	gap := width - lipgloss.Width(left) - lipgloss.Width(right)
	if !visible(right) || gap < 1 {
		return truncate(left, width)
	}
	return left + strings.Repeat(" ", gap) + right
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
