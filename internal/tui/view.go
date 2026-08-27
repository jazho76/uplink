package tui

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/jazho76/uplink/internal/humanize"
	"github.com/jazho76/uplink/internal/target"
	"github.com/jazho76/uplink/internal/ui"
)

const (
	minPreviewTextW = 10

	gaugeGap     = 3
	percentGap   = 2
	percentWidth = len("100%")
	hostBarGap   = 3
	fieldGap     = 2
	hostBarMeter = 10
	minLiveBar   = 8
)

var (
	plainStyle   = lipgloss.NewStyle()
	rowName      = lipgloss.NewStyle().Foreground(ui.Fg)
	rowFill      = lipgloss.NewStyle().Background(ui.Selection)
	spinnerStyle = lipgloss.NewStyle().Foreground(ui.Cyan)

	titleStyle = lipgloss.NewStyle().Bold(true).Foreground(ui.Cyan)
	autoMarker = lipgloss.NewStyle().Foreground(ui.Magenta)
	modeMarker = lipgloss.NewStyle().Foreground(ui.Yellow)

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

	previewBorder = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ui.Comment).
			Padding(0, 1)
	hostBarBorder = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ui.Comment).
			Padding(0, 1)
)

func glyph(it item, base lipgloss.Style) string {
	mark, color := glyphOf(it)
	return base.Foreground(color).Render(mark)
}

func glyphOf(it item) (string, lipgloss.TerminalColor) {
	switch it.t.Provider {
	case target.ProviderLocal:
		return "⬢", ui.Cyan
	case target.ProviderRemote:
		return remoteGlyphOf(it.t.Status)
	default:
		if it.running() {
			return "●", ui.Green
		}
		return "○", ui.Comment
	}
}

func remoteGlyphOf(status target.Status) (string, lipgloss.TerminalColor) {
	switch status {
	case target.StatusRunning:
		return "◆", ui.Green
	case target.StatusUnreachable:
		return "×", ui.Red
	default:
		return "◇", ui.Comment
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
	header := titleStyle.Render(it.name()) + "  " + glyph(it, plainStyle) + " " + string(it.t.Status)
	b.WriteString(spread(header, labelStyle.Render(m.uptimeOf(it)), width, plainStyle) + "\n")

	if it.worthProbing() {
		b.WriteString("\n" + rule("live", width))
		b.WriteString(m.renderLive(it.name(), width, height))
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

func (m model) renderLive(name string, width, height int) string {
	e, ok := m.live[name]
	if !ok {
		return labelStyle.Render("…") + "\n"
	}
	if e.err {
		return renderFields([]field{{"load", labelStyle.Render("unavailable")}}, width)
	}

	s := e.stats
	load := loadFraction(s.Load, s.Cores)
	gauges := []gaugeRow{
		{"load", load, fmt.Sprintf("%.2f / %d", s.Load, s.Cores)},
		{"memory", usedFraction(s.MemUsed, s.MemTotal), amount(s.MemUsed, s.MemTotal)},
		{"disk", usedFraction(s.DiskUsed, s.DiskTotal), amount(s.DiskUsed, s.DiskTotal)},
	}

	fields := gaugeFields(gauges, width)
	indent := widestKey(fields) + fieldGap
	return renderFields(fields, width) + m.renderLoadChart(e, load, width-indent, height, indent)
}

type gaugeRow struct {
	label    string
	fraction float64
	reading  string
}

func gaugeFields(gauges []gaugeRow, width int) []field {
	labels, readings := 0, 0
	for _, g := range gauges {
		labels = max(labels, lipgloss.Width(g.label))
		readings = max(readings, lipgloss.Width(g.reading))
	}

	bar := width - labels - fieldGap - gaugeGap - readings - percentGap - percentWidth
	if bar < minLiveBar {
		bar = 0
	}

	fields := make([]field, 0, len(gauges))
	for _, g := range gauges {
		reading := valueStyle.Render(pad(g.reading, readings, plainStyle))
		percent := labelStyle.Render(fmt.Sprintf("%3.0f%%", g.fraction*100))
		gap := strings.Repeat(" ", percentGap)
		fields = append(fields, field{g.label, gauge(meter(g.fraction, bar, blockGlyphs), reading+gap+percent)})
	}
	return fields
}

func (m model) renderLoadChart(e liveEntry, load float64, width, height, indent int) string {
	if width < minChartSpan {
		return ""
	}
	visible := newest(e.history, width)
	ceiling := chartCeiling(visible)

	plot := areaChart(visible, width, chartHeight(height), ceiling, meterStyle(load))
	if len(plot) == 0 {
		return ""
	}

	window := humanize.Duration(time.Duration(len(visible)) * liveInterval)
	head := labelStyle.Render("load · last " + window)
	peak := labelStyle.Render(fmt.Sprintf("peak %.2f", ceiling*float64(e.stats.Cores)))

	gutter := strings.Repeat(" ", indent)
	lines := append([]string{spread(head, peak, width, plainStyle)}, plot...)
	lines = append(lines, labelStyle.Render(strings.Repeat(thinGlyphs.track, width)))

	var b strings.Builder
	b.WriteString("\n")
	for _, line := range lines {
		b.WriteString(gutter + line + "\n")
	}
	return b.String()
}

func gauge(bar, reading string) string {
	if !visible(bar) {
		return valueStyle.Render(reading)
	}
	return bar + strings.Repeat(" ", gaugeGap) + valueStyle.Render(reading)
}

func amount(used, total uint64) string {
	u, t := humanize.Bytes(used), humanize.Bytes(total)
	if unit := unitOf(t); unit != "" && strings.HasSuffix(u, unit) {
		return strings.TrimSuffix(u, unit) + "/" + t
	}
	return u + "/" + t
}

func unitOf(size string) string {
	return strings.TrimLeft(size, "0123456789.")
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
		label := labelStyle.Render(pad(f.key, gutter, plainStyle))
		b.WriteString(truncate(label+strings.Repeat(" ", fieldGap)+f.value, width) + "\n")
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
	content := fitInReadingOrder(m.hostSegments(), width-chromeCells)
	return hostBarBorder.Width(width - borderCells).Render(content)
}

type barSegment struct {
	text     string
	priority int
}

func (m model) hostSegments() []barSegment {
	h := m.hostStats
	load := loadFraction(h.Load, h.Cores)

	return []barSegment{
		{text: titleStyle.Render(m.hostName), priority: 0},
		{text: hostGauge("load", sparkline(m.hostHistory, hostBarMeter, meterStyle(load)), fmt.Sprintf("%.2f", h.Load)), priority: 1},
		{text: hostGauge("ram", meter(usedFraction(h.MemUsed, h.MemTotal), hostBarMeter, thinGlyphs), amount(h.MemUsed, h.MemTotal)), priority: 2},
		{text: hostGauge("disk", meter(usedFraction(h.DiskUsed, h.DiskTotal), hostBarMeter, thinGlyphs), amount(h.DiskUsed, h.DiskTotal)), priority: 4},
		{text: labelStyle.Render("cores ") + valueStyle.Render(strconv.Itoa(h.Cores)), priority: 5},
		{text: m.committedSummary(), priority: 3},
	}
}

func hostGauge(label, bar, reading string) string {
	return labelStyle.Render(label+" ") + bar + " " + valueStyle.Render(reading)
}

func fitInReadingOrder(segments []barSegment, width int) string {
	order := make([]int, len(segments))
	for i := range order {
		order[i] = i
	}
	slices.SortStableFunc(order, func(a, b int) int {
		return segments[a].priority - segments[b].priority
	})

	keep := make([]bool, len(segments))
	used := 0
	for _, i := range order {
		if !visible(segments[i].text) {
			continue
		}
		cost := lipgloss.Width(segments[i].text)
		if used > 0 {
			cost += hostBarGap
		}
		if used+cost > width {
			continue
		}
		used += cost
		keep[i] = true
	}

	var kept []string
	for i, s := range segments {
		if keep[i] {
			kept = append(kept, s.text)
		}
	}
	return truncate(strings.Join(kept, strings.Repeat(" ", hostBarGap)), width)
}

func (m model) committedSummary() string {
	var vcpu int
	var memory uint64
	for _, it := range m.items {
		if !it.running() || (it.t.CPUs == 0 && it.t.Memory == 0) {
			continue
		}
		vcpu += it.t.CPUs
		memory += it.t.Memory
	}
	if vcpu == 0 && memory == 0 {
		return ""
	}
	return labelStyle.Render("committed ") + valueStyle.Render(
		fmt.Sprintf("%d vCPU · %s", vcpu, humanize.Bytes(memory)))
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

func spread(left, right string, width int, base lipgloss.Style) string {
	gap := width - lipgloss.Width(left) - lipgloss.Width(right)
	if !visible(right) || gap < 1 {
		return pad(left, width, base)
	}
	return left + base.Render(strings.Repeat(" ", gap)) + right
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
