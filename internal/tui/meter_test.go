package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/jazho76/uplink/internal/target"
	"github.com/jazho76/uplink/internal/ui"
)

func TestMeterSpansItsWidthAtEveryFraction(t *testing.T) {
	for _, f := range []float64{-1, 0, 0.01, 0.33, 0.5, 0.99, 1, 4} {
		if got := lipgloss.Width(meter(f, 20)); got != 20 {
			t.Errorf("meter(%v, 20) spans %d cells, want 20", f, got)
		}
	}
	if got := meter(0.5, 0); got != "" {
		t.Errorf("a meter with no room renders nothing, got %q", got)
	}
}

func TestMeterMarksThePartialCell(t *testing.T) {
	for _, c := range []struct {
		fraction float64
		want     string
	}{
		{fraction: 0, want: "░░░░"},
		{fraction: 0.5, want: "██░░"},
		{fraction: 0.6, want: "██▓░"},
		{fraction: 1, want: "████"},
		{fraction: 2, want: "████"},
	} {
		if got := ansi.Strip(meter(c.fraction, 4)); got != c.want {
			t.Errorf("meter(%v, 4) = %q, want %q", c.fraction, got, c.want)
		}
	}
}

func TestMeterThresholdsPickStatusColours(t *testing.T) {
	for _, c := range []struct {
		fraction float64
		want     lipgloss.TerminalColor
	}{
		{fraction: 0, want: ui.Green},
		{fraction: meterWarn - 0.01, want: ui.Green},
		{fraction: meterWarn, want: ui.Yellow},
		{fraction: meterCritical - 0.01, want: ui.Yellow},
		{fraction: meterCritical, want: ui.Red},
		{fraction: 5, want: ui.Red},
	} {
		if got := meterStyle(c.fraction).GetForeground(); got != c.want {
			t.Errorf("meterStyle(%v) = %v, want %v", c.fraction, got, c.want)
		}
	}
}

func TestNarrowLiveBlockDropsTheBarNotTheReading(t *testing.T) {
	gauges := []gaugeRow{{label: "memory", fraction: 0.5, reading: "7.6/31GiB"}}

	cramped := ansi.Strip(gaugeFields(gauges, 24)[0].value)
	if strings.ContainsAny(cramped, meterFill+meterTrack) {
		t.Errorf("a pane with no room for a reading draws no bar, got %q", cramped)
	}
	if !strings.Contains(cramped, "7.6/31GiB") {
		t.Errorf("the reading survives however narrow the pane, got %q", cramped)
	}

	roomy := ansi.Strip(gaugeFields(gauges, 70)[0].value)
	if !strings.Contains(roomy, meterFill) {
		t.Errorf("a roomy pane should draw a bar, got %q", roomy)
	}

	if got := ansi.Strip(gauge("", "7.6GiB / 31GiB", gaugeGap)); got != "7.6GiB / 31GiB" {
		t.Errorf("an absent bar leaves the reading alone, got %q", got)
	}
	if got := ansi.Strip(gauge(meter(0.5, 4), "1.32", gaugeGap)); got != "██░░   1.32" {
		t.Errorf("a bar keeps its reading beside it, got %q", got)
	}
}

func TestAmountCollapsesASharedUnit(t *testing.T) {
	for _, c := range []struct {
		used, total uint64
		want        string
	}{
		{used: 780 << 30, total: 930 << 30, want: "780/930GiB"},
		{used: 8 << 30, total: 31 << 30, want: "8/31GiB"},
		{used: 900 << 20, total: 31 << 30, want: "900MiB/31GiB"},
		{used: 0, total: 0, want: "0/0B"},
	} {
		if got := amount(c.used, c.total); got != c.want {
			t.Errorf("amount(%d, %d) = %q, want %q", c.used, c.total, got, c.want)
		}
	}
}

func TestSelectedRowFillsEveryCell(t *testing.T) {
	stylingEnabled(t)

	m := model{live: map[string]liveEntry{"kyoto": {at: time.Now()}}}
	it := item{t: target.Target{Name: "kyoto"}, key: 'k'}

	const width = 30
	filled := m.paneRow(it, true, true, width)
	if got := lipgloss.Width(filled); got != width {
		t.Fatalf("a filled row spans %d cells, want %d", got, width)
	}

	for _, run := range visibleRuns(filled) {
		if !run.styled {
			t.Errorf("an unstyled run breaks the fill: %q in %q", run.text, filled)
		}
	}

	plain := m.paneRow(it, false, true, width)
	if lipgloss.Width(plain) != width {
		t.Errorf("an unselected row still spans its width, got %d", lipgloss.Width(plain))
	}
	unstyled := 0
	for _, run := range visibleRuns(plain) {
		if !run.styled {
			unstyled++
		}
	}
	if unstyled == 0 {
		t.Errorf("only the selected row should be filled end to end: %q", plain)
	}
}

type textRun struct {
	text   string
	styled bool
}

func visibleRuns(line string) []textRun {
	var runs []textRun
	styled := false
	var current strings.Builder
	flush := func() {
		if current.Len() > 0 {
			runs = append(runs, textRun{current.String(), styled})
			current.Reset()
		}
	}
	for i := 0; i < len(line); i++ {
		if line[i] == 0x1b {
			j := strings.IndexByte(line[i:], 'm')
			flush()
			styled = !strings.HasSuffix(line[i:i+j+1], "[0m")
			i += j
			continue
		}
		current.WriteByte(line[i])
	}
	flush()
	return runs
}
