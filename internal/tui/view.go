package tui

import (
	"cmp"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/jazho76/uplink/internal/humanize"
	"github.com/jazho76/uplink/internal/stats"
	"github.com/jazho76/uplink/internal/ui"
)

const (
	gaugeGap     = 3
	percentGap   = 2
	percentWidth = len("100%")
	fieldGap     = 2
	minLiveBar   = 8
)

var (
	plainStyle = lipgloss.NewStyle()
	rowFill    = lipgloss.NewStyle().Background(ui.Selection)

	titleStyle = lipgloss.NewStyle().Bold(true).Foreground(ui.Cyan)
	modeMarker = lipgloss.NewStyle().Foreground(ui.Yellow)

	keyStyle       = lipgloss.NewStyle().Foreground(ui.Magenta)
	footerStyle    = lipgloss.NewStyle().Foreground(ui.Comment)
	noticeStyle    = lipgloss.NewStyle().Foreground(ui.Yellow)
	labelStyle     = lipgloss.NewStyle().Foreground(ui.Comment)
	ruleStyle      = lipgloss.NewStyle().Foreground(ui.Blue)
	detailLogStyle = lipgloss.NewStyle().Foreground(ui.Comment)
	valueStyle     = lipgloss.NewStyle().Foreground(ui.Fg)

	paneBorder      = lipgloss.NewStyle().Foreground(ui.Comment)
	paneBorderFocus = lipgloss.NewStyle().Foreground(ui.Cyan)
	paneTitle       = lipgloss.NewStyle().Foreground(ui.Comment)
	paneTitleFocus  = lipgloss.NewStyle().Foreground(ui.Cyan).Bold(true)

	boxBorder = lipgloss.RoundedBorder()
	boxStyle  = lipgloss.NewStyle().
			Border(boxBorder).
			BorderForeground(ui.Comment).
			Padding(0, 1)
)

func (m model) glyph(it item, base lipgloss.Style) string {
	mark, color := glyphOf(m.statusOf(it.target))
	return base.Foreground(color).Render(mark)
}

func glyphOf(s status) (string, lipgloss.TerminalColor) {
	switch s {
	case statusUp:
		return "●", ui.Green
	case statusDown:
		return "○", ui.Comment
	case statusStarting, statusStopping:
		return "◐", ui.Yellow
	default:
		return "◌", ui.Comment
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
	body := lipgloss.JoinHorizontal(lipgloss.Top,
		m.renderPanes(l.paneColumnOuterW, l.bodyOuterH),
		m.renderPreview(l.previewOuterW, l.bodyOuterH))

	return lipgloss.JoinVertical(lipgloss.Left, body, m.renderFooter())
}

func (m model) renderPreview(width, height int) string {
	return boxStyle.Width(width - borderCells).Height(height - borderCells).
		Render(m.previewBody(width-chromeCells, height-borderCells))
}

func (m model) previewBody(width, height int) string {
	it := m.selected()
	if it.name() == "" {
		return ""
	}

	var b strings.Builder
	header := titleStyle.Render(it.name()) + "  " + m.glyph(it, plainStyle) + " " + string(m.statusOf(it.target))
	b.WriteString(spread(header, m.modeTag(it), width, plainStyle) + "\n")

	if len(it.target.Stats) > 0 {
		b.WriteString("\n" + rule("live", width))
		b.WriteString(m.renderLive(it.name(), width))
	}

	if logs := m.logsFor(it); logs != "" {
		used := strings.Count(b.String(), "\n")
		fit := height - used - 2
		if fit > 0 {
			lines := strings.Split(logs, "\n")
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

func (m model) logsFor(it item) string {
	if m.logPeek.name != it.name() {
		return ""
	}
	return m.logPeek.text
}

func (m model) renderLive(name string, width int) string {
	e := m.live[name]
	if e.statsAt.IsZero() {
		return labelStyle.Render("…") + "\n"
	}
	if !e.statsOK {
		return labelStyle.Render("unavailable") + "\n"
	}

	var gauges []gaugeRow
	var labels []string
	for _, r := range e.readings {
		if r.Metric.Gauge() {
			gauges = append(gauges, gaugeOf(r))
		}
		labels = append(labels, r.Metric.Label)
	}

	indent := labelIndent(labels)
	gaugeRows := gaugeFields(gauges, width-indent)
	var fields []field
	for _, r := range e.readings {
		switch {
		case r.Metric.Gauge():
			fields, gaugeRows = append(fields, gaugeRows[0]), gaugeRows[1:]
		case r.Metric.Type == stats.TypeText:
			fields = append(fields, field{r.Metric.Label, valueStyle.Render(cmp.Or(textOf(r), "?"))})
		}
	}

	return renderFields(fields, width)
}

type gaugeRow struct {
	label    string
	fraction float64
	reading  string
	missing  bool
}

func gaugeOf(r stats.Reading) gaugeRow {
	switch {
	case !r.OK:
		return gaugeRow{label: r.Metric.Label, reading: "?", missing: true}
	case r.Metric.Unit == stats.UnitBytes:
		return gaugeRow{label: r.Metric.Label, fraction: r.Fraction(), reading: amount(uint64(r.Value), uint64(r.Max))}
	default:
		return gaugeRow{label: r.Metric.Label, fraction: r.Fraction(), reading: inUnit(r, r.Value) + " / " + inUnit(r, r.Max)}
	}
}

func textOf(r stats.Reading) string {
	switch {
	case r.Metric.Unit == "":
		return r.Text
	case !r.OK:
		return ""
	default:
		return inUnit(r, r.Value)
	}
}

func inUnit(r stats.Reading, v float64) string {
	switch r.Metric.Unit {
	case stats.UnitBytes:
		return humanize.Bytes(uint64(v))
	case stats.UnitSeconds:
		return humanize.Duration(time.Duration(v * float64(time.Second)))
	default:
		return number(v)
	}
}

func number(v float64) string {
	if v == math.Trunc(v) {
		return strconv.FormatFloat(v, 'f', 0, 64)
	}
	return fmt.Sprintf("%.2f", v)
}

func labelIndent(labels []string) int {
	widest := 0
	for _, label := range labels {
		widest = max(widest, lipgloss.Width(label))
	}
	return widest + fieldGap
}

func gaugeFields(gauges []gaugeRow, width int) []field {
	readings := 0
	for _, g := range gauges {
		readings = max(readings, lipgloss.Width(g.reading))
	}

	bar := width - gaugeGap - readings - percentGap - percentWidth
	if bar < minLiveBar {
		bar = 0
	}

	fields := make([]field, 0, len(gauges))
	for _, g := range gauges {
		reading := valueStyle.Render(pad(g.reading, readings, plainStyle))
		percent := labelStyle.Render(fmt.Sprintf("%3.0f%%", g.fraction*100))
		if g.missing {
			percent = labelStyle.Render(fmt.Sprintf("%*s", percentWidth, "?"))
		}
		gap := strings.Repeat(" ", percentGap)
		fields = append(fields, field{g.label,
			gauge(meter(g.fraction, bar), reading+gap+percent, gaugeGap)})
	}
	return fields
}

func gauge(bar, reading string, gap int) string {
	if !visible(bar) {
		return valueStyle.Render(reading)
	}
	return bar + strings.Repeat(" ", gap) + valueStyle.Render(reading)
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

func (m model) modeTag(it item) string {
	n := len(it.target.Modes)
	if n < 2 {
		return ""
	}
	return modeMarker.Render(m.mode().Name) + "  " + labelStyle.Render(fmt.Sprintf("%d/%d", m.modeIdx+1, n))
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
	header := titleStyle.Render("logs: " + m.logTarget.Name)
	footer := keyStyle.Render("esc") + " " + footerStyle.Render("back")

	body := labelStyle.Render("no log output")
	if m.logView != "" {
		bodyH := max(m.height-2, 1)
		lines := strings.Split(m.logView, "\n")
		if len(lines) > bodyH {
			lines = lines[len(lines)-bodyH:]
		}
		for i, line := range lines {
			lines[i] = detailLogStyle.Render(clip(line, m.width))
		}
		body = strings.Join(lines, "\n")
	}

	return lipgloss.JoinVertical(lipgloss.Left, truncate(header, m.width), body, footer)
}

func (m model) renderFooter() string {
	it := m.selected()
	var pairs [][2]string
	if it.name() != "" {
		pairs = append(pairs, [2]string{"↵", "connect"})
	}
	if len(it.target.Modes) > 1 {
		pairs = append(pairs, [2]string{"tab", "mode"})
	}
	if it.target.Logs != "" {
		pairs = append(pairs, [2]string{"^l", "logs"})
	}
	if m.canHook(it.target, statusStarting) {
		pairs = append(pairs, [2]string{"^s", "start"})
	}
	if m.canHook(it.target, statusStopping) {
		pairs = append(pairs, [2]string{"^x", "stop"})
	}
	pairs = append(pairs, [2]string{"^e", "config"}, [2]string{"q", "quit"})

	var parts []string
	for _, p := range pairs {
		parts = append(parts, keyStyle.Render(p[0])+" "+footerStyle.Render(p[1]))
	}
	keys := truncate(strings.Join(parts, footerStyle.Render("  ")), m.width)
	return keys + "\n" + truncate(noticeStyle.Render(oneLine(m.notice)), m.width)
}

func rule(title string, width int) string {
	head := ruleStyle.Render(title) + " "
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
