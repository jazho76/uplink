package dashboard

import (
	"cmp"
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/jazho76/uplink/internal/target"
)

const (
	gaugeGap     = 3
	percentGap   = 2
	percentWidth = len("100%")
	minLiveBar   = 8
)

func (m model) showsLive(t target.Target) bool {
	if !t.Stats.Builtin {
		return len(t.Stats.Metrics) > 0
	}
	stats := m.state.Observation(t.ID).Stats
	if stats.Known {
		return len(stats.Value) > 0 || stats.Err != nil
	}
	return stats.At.IsZero()
}

func (m model) renderLive(id target.ID, width int) string {
	stats := m.state.Observation(id).Stats
	if stats.At.IsZero() {
		return labelStyle.Render("…") + "\n"
	}
	if stats.Err != nil {
		return labelStyle.Render("unavailable") + "\n"
	}

	var gauges []gaugeRow
	var labels []string
	for _, r := range stats.Value {
		if r.Metric.Gauge() {
			gauges = append(gauges, gaugeOf(r))
		}
		labels = append(labels, r.Metric.Label)
	}

	indent := labelIndent(labels)
	gaugeRows := gaugeFields(gauges, width-indent)
	var fields []field
	for _, r := range stats.Value {
		switch {
		case r.Metric.Gauge():
			fields, gaugeRows = append(fields, gaugeRows[0]), gaugeRows[1:]
		case r.Metric.Type == target.TypeText:
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
