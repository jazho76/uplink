package dashboard

import (
	"cmp"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/jazho76/uplink/internal/target"
)

const (
	gaugeGap     = 3
	percentGap   = 2
	percentWidth = len("100%")
	fieldGap     = 2
	minLiveBar   = 8

	maxWindowName = 24

	attachedGlyph   = "●"
	windowSeparator = " · "
)

var (
	plainStyle = lipgloss.NewStyle()
	rowFill    = lipgloss.NewStyle().Background(colorSelection)

	titleStyle = lipgloss.NewStyle().Bold(true).Foreground(colorCyan)
	modeMarker = lipgloss.NewStyle().Foreground(colorYellow)

	attachedMarker = lipgloss.NewStyle().Foreground(colorCyan)

	keyStyle       = lipgloss.NewStyle().Foreground(colorMagenta)
	footerStyle    = lipgloss.NewStyle().Foreground(colorComment)
	noticeStyle    = lipgloss.NewStyle().Foreground(colorYellow)
	labelStyle     = lipgloss.NewStyle().Foreground(colorComment)
	ruleStyle      = lipgloss.NewStyle().Foreground(colorBlue)
	detailLogStyle = lipgloss.NewStyle().Foreground(colorComment)
	valueStyle     = lipgloss.NewStyle().Foreground(colorFg)

	paneBorder      = lipgloss.NewStyle().Foreground(colorComment)
	paneBorderFocus = lipgloss.NewStyle().Foreground(colorCyan)
	paneTitle       = lipgloss.NewStyle().Foreground(colorComment)
	paneTitleFocus  = lipgloss.NewStyle().Foreground(colorCyan).Bold(true)

	boxBorder = lipgloss.RoundedBorder()
	boxStyle  = lipgloss.NewStyle().
			Border(boxBorder).
			BorderForeground(colorComment).
			Padding(0, 1)
)

func (m model) glyph(it item, base lipgloss.Style) string {
	mark, color := glyphOf(m.statusOf(it.target))
	return base.Foreground(color).Render(mark)
}

func glyphOf(s status) (string, lipgloss.TerminalColor) {
	switch s {
	case statusUp:
		return "●", colorGreen
	case statusDown:
		return "○", colorComment
	case statusStarting, statusStopping:
		return "◐", colorYellow
	default:
		return "◌", colorComment
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
	if it.empty() {
		return ""
	}

	var b strings.Builder
	header := titleStyle.Render(it.name()) + "  " + m.glyph(it, plainStyle) + " " + string(m.statusOf(it.target))
	b.WriteString(spread(header, m.modeTag(it), width, plainStyle) + "\n")

	if m.showsLive(it.target) {
		b.WriteString("\n" + rule("live", width))
		b.WriteString(m.renderLive(it.id(), width))
	}

	if sessions := m.renderSessions(it.id(), width); sessions != "" {
		b.WriteString("\n" + rule("sessions", width))
		b.WriteString(sessions)
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
	if m.logPeek.id != it.id() || !m.hasLogs(it.target) {
		return ""
	}
	return m.logPeek.text
}

func (m model) showsLive(t target.Target) bool {
	if !t.Stats.Builtin {
		return len(t.Stats.Metrics) > 0
	}
	e := m.live[t.ID]
	if e.inspected {
		return len(e.snapshot.Readings) > 0
	}
	return e.inspectedAt.IsZero()
}

func (m model) renderLive(id target.ID, width int) string {
	e := m.live[id]
	if e.inspectedAt.IsZero() {
		return labelStyle.Render("…") + "\n"
	}
	if !e.inspectOK {
		return labelStyle.Render("unavailable") + "\n"
	}

	var gauges []gaugeRow
	var labels []string
	for _, r := range e.snapshot.Readings {
		if r.Metric.Gauge() {
			gauges = append(gauges, gaugeOf(r))
		}
		labels = append(labels, r.Metric.Label)
	}

	indent := labelIndent(labels)
	gaugeRows := gaugeFields(gauges, width-indent)
	var fields []field
	for _, r := range e.snapshot.Readings {
		switch {
		case r.Metric.Gauge():
			fields, gaugeRows = append(fields, gaugeRows[0]), gaugeRows[1:]
		case r.Metric.Type == target.TypeText:
			fields = append(fields, field{r.Metric.Label, valueStyle.Render(cmp.Or(textOf(r), "?"))})
		}
	}

	return renderFields(fields, width)
}

func (m model) modeSessions(id target.ID) []target.Session {
	e := m.live[id]
	if !e.inspectOK {
		return nil
	}
	tool := m.mode().Multiplexer
	if tool == "" {
		return nil
	}
	var sessions []target.Session
	for _, s := range e.snapshot.Sessions {
		if s.Tool == tool {
			sessions = append(sessions, s)
		}
	}
	return sessions
}

func (m model) renderSessions(id target.ID, width int) string {
	sessions := m.modeSessions(id)
	if len(sessions) == 0 {
		return ""
	}
	fields := make([]field, len(sessions))
	for i, s := range sessions {
		fields[i].key = sessionMargin(s.Attached) + valueStyle.Render(clip(s.Name, width/2))
	}
	room := width - widestKey(fields) - fieldGap
	for i, s := range sessions {
		fields[i].value = windowList(s.Windows, room)
	}
	return renderFields(fields, width)
}

func sessionMargin(attached bool) string {
	if attached {
		return attachedMarker.Render(attachedGlyph) + " "
	}
	return "  "
}

func windowList(windows []target.Window, width int) string {
	labels := make([]string, len(windows))
	for i, w := range windows {
		labels[i] = windowLabel(w, width)
	}
	gap := labelStyle.Render(windowSeparator)
	for shown := len(labels); shown >= 0; shown-- {
		parts := slices.Clone(labels[:shown])
		if hidden := len(labels) - shown; hidden > 0 {
			parts = append(parts, labelStyle.Render(fmt.Sprintf("+%d", hidden)))
		}
		if list := strings.Join(parts, gap); lipgloss.Width(list) <= width {
			return list
		}
	}
	return ""
}

func windowLabel(w target.Window, width int) string {
	suffix := ""
	if w.Panes > 1 {
		suffix = labelStyle.Render(fmt.Sprintf(" (%d)", w.Panes))
	}
	name := clip(w.Name, min(maxWindowName, width-lipgloss.Width(suffix)))
	return valueStyle.Render(name) + suffix
}

type gaugeRow struct {
	label    string
	fraction float64
	reading  string
	missing  bool
}

func gaugeOf(r target.Reading) gaugeRow {
	switch {
	case !r.OK:
		return gaugeRow{label: r.Metric.Label, reading: "?", missing: true}
	case r.Metric.Unit == target.UnitBytes:
		return gaugeRow{label: r.Metric.Label, fraction: r.Fraction(), reading: amount(uint64(r.Value), uint64(r.Max))}
	default:
		return gaugeRow{label: r.Metric.Label, fraction: r.Fraction(), reading: inUnit(r, r.Value) + " / " + inUnit(r, r.Max)}
	}
}

func textOf(r target.Reading) string {
	switch {
	case r.Metric.Unit == "":
		return r.Text
	case !r.OK:
		return ""
	default:
		return inUnit(r, r.Value)
	}
}

func inUnit(r target.Reading, v float64) string {
	switch r.Metric.Unit {
	case target.UnitBytes:
		return formatBytes(uint64(v))
	case target.UnitSeconds:
		return formatDuration(time.Duration(v * float64(time.Second)))
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
	u, t := formatBytes(used), formatBytes(total)
	if unit := unitOf(t); unit != "" && strings.HasSuffix(u, unit) {
		return strings.TrimSuffix(u, unit) + "/" + t
	}
	return u + "/" + t
}

func unitOf(size string) string {
	return strings.TrimLeft(size, "0123456789.")
}

func (m model) modeTag(it item) string {
	n := len(m.modes(it.target))
	if n < 2 {
		return ""
	}
	name := m.mode().Name
	if m.modeUnconfirmed(it.target) {
		name += "?"
	}
	return modeMarker.Render(name) + "  " + labelStyle.Render(fmt.Sprintf("%d/%d", m.modeIndex()+1, n))
}

func (m model) modeUnconfirmed(t target.Target) bool {
	return m.mode().Requires != "" && !m.live[t.ID].inspected && m.statusOf(t) == statusDown
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
	if !it.empty() {
		pairs = append(pairs, [2]string{"↵", "connect"})
	}
	if len(m.modes(it.target)) > 1 {
		pairs = append(pairs, [2]string{"tab", "mode"})
	}
	if m.hasLogs(it.target) {
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
