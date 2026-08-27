package tui

import (
	"regexp"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/jazho76/uplink/internal/probe"
	"github.com/jazho76/uplink/internal/target"
	"github.com/jazho76/uplink/internal/ui"
)

func TestMeterSpansItsWidthAtEveryFraction(t *testing.T) {
	for _, f := range []float64{-1, 0, 0.01, 0.33, 0.5, 0.99, 1, 4} {
		if got := lipgloss.Width(meter(f, 20, thinGlyphs)); got != 20 {
			t.Errorf("meter(%v, 20) spans %d cells, want 20", f, got)
		}
	}
	if got := meter(0.5, 0, thinGlyphs); got != "" {
		t.Errorf("a meter with no room renders nothing, got %q", got)
	}
}

func TestMeterMarksThePartialCell(t *testing.T) {
	for _, c := range []struct {
		fraction float64
		want     string
	}{
		{fraction: 0, want: "────"},
		{fraction: 0.5, want: "━━──"},
		{fraction: 0.6, want: "━━╸─"},
		{fraction: 1, want: "━━━━"},
		{fraction: 2, want: "━━━━"},
	} {
		if got := ansi.Strip(meter(c.fraction, 4, thinGlyphs)); got != c.want {
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

func TestSparklineSharesTheMeterGeometry(t *testing.T) {
	plain := lipgloss.NewStyle()

	if got := lipgloss.Width(sparkline(nil, 12, plain)); got != 12 {
		t.Errorf("an empty sparkline still spans its width, got %d", got)
	}
	if got := ansi.Strip(sparkline(nil, 4, plain)); got != strings.Repeat(thinGlyphs.track, 4) {
		t.Errorf("unfilled history backfills with the meter track, got %q", got)
	}

	partial := ansi.Strip(sparkline([]float64{0, 1}, 5, plain))
	if want := "───▁█"; partial != want {
		t.Errorf("a short history hugs the right edge, got %q want %q", partial, want)
	}

	overflow := ansi.Strip(sparkline([]float64{0, 0, 0, 1}, 2, plain))
	if want := "▁█"; overflow != want {
		t.Errorf("an overlong history keeps its newest samples, got %q want %q", overflow, want)
	}
}

func TestLiveHistoryKeepsTheNewestSamples(t *testing.T) {
	var history []float64
	for i := 0; i < maxLiveSamples*2; i++ {
		history = appendSample(history, float64(i))
	}

	if len(history) != maxLiveSamples {
		t.Fatalf("history should cap at %d, got %d", maxLiveSamples, len(history))
	}
	if last := history[len(history)-1]; last != float64(maxLiveSamples*2-1) {
		t.Errorf("the newest sample should survive, got %v", last)
	}
}

func TestNarrowLiveBlockDropsTheBarNotTheReading(t *testing.T) {
	gauges := []gaugeRow{{"memory", 0.5, "7.6/31GiB"}}

	cramped := ansi.Strip(gaugeFields(gauges, 24)[0].value)
	if strings.ContainsAny(cramped, blockGlyphs.fill+blockGlyphs.track) {
		t.Errorf("a pane with no room for a reading draws no bar, got %q", cramped)
	}
	if !strings.Contains(cramped, "7.6/31GiB") {
		t.Errorf("the reading survives however narrow the pane, got %q", cramped)
	}

	roomy := ansi.Strip(gaugeFields(gauges, 70)[0].value)
	if !strings.ContainsRune(roomy, []rune(blockGlyphs.fill)[0]) {
		t.Errorf("a roomy pane should draw a bar, got %q", roomy)
	}

	if got := ansi.Strip(gauge("", "7.6GiB / 31GiB", gaugeGap)); got != "7.6GiB / 31GiB" {
		t.Errorf("an absent bar leaves the reading alone, got %q", got)
	}
	if got := ansi.Strip(gauge(meter(0.5, 4, thinGlyphs), "1.32", gaugeGap)); got != "━━──   1.32" {
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

func TestHostBarFitsByPriorityAndRendersInReadingOrder(t *testing.T) {
	m := model{hostName: "GLaDOS", hostStats: probe.Stats{Cores: 12, Load: 4.1,
		MemUsed: 8 << 30, MemTotal: 31 << 30, DiskUsed: 780 << 30, DiskTotal: 930 << 30},
		panes: panesOf(
			item{t: target.Target{Name: "kyoto", Status: target.StatusRunning, CPUs: 6, Memory: 12 << 30}},
			item{t: target.Target{Name: "forge", Status: target.StatusRunning, CPUs: 8, Memory: 16 << 30}},
		)}

	for _, width := range []int{minWidth, 60, 80, 100, 140} {
		bar := m.renderHostBar(width)
		for _, line := range strings.Split(bar, "\n") {
			if got := lipgloss.Width(line); got != width {
				t.Errorf("%d cols: host bar line spans %d", width, got)
			}
		}
		if !strings.Contains(ansi.Strip(bar), "GLaDOS") {
			t.Errorf("%d cols: the host name must never be the segment that goes", width)
		}
	}

	roomy := ansi.Strip(m.renderHostBar(140))
	disk, committed := strings.Index(roomy, "disk"), strings.Index(roomy, "committed")
	if disk < 0 || committed < 0 {
		t.Fatalf("a wide bar should carry every segment: %q", roomy)
	}
	if disk > committed {
		t.Errorf("host meters read together, before the allocation summary: %q", roomy)
	}

	narrow := ansi.Strip(m.renderHostBar(60))
	if !strings.Contains(narrow, "load") {
		t.Errorf("load outranks every other reading: %q", narrow)
	}
	if strings.Contains(narrow, "disk") {
		t.Errorf("a narrow bar drops the lower-priority reading: %q", narrow)
	}
}

func TestHostBarSegmentsShareTheSlackEvenly(t *testing.T) {
	parts := []string{"host", "load", "ram", "disk"}
	for _, width := range []int{20, 33, 40, 61} {
		bar := spaceEvenly(parts, width)
		if got := lipgloss.Width(bar); got != width {
			t.Errorf("%d cols: spaced segments span %d", width, got)
		}
		if strings.HasPrefix(bar, " ") || strings.HasSuffix(bar, " ") {
			t.Errorf("%d cols: the outer segments sit flush against the edges: %q", width, bar)
		}

		gaps := regexp.MustCompile(" +").FindAllString(bar, -1)
		if len(gaps) != len(parts)-1 {
			t.Fatalf("%d cols: %d gaps between %d segments: %q", width, len(gaps), len(parts), bar)
		}
		widest, narrowest := len(gaps[0]), len(gaps[0])
		for _, gap := range gaps {
			widest, narrowest = max(widest, len(gap)), min(narrowest, len(gap))
		}
		if widest-narrowest > 1 {
			t.Errorf("%d cols: gaps range from %d to %d: %q", width, narrowest, widest, bar)
		}
	}
}

func TestHostBarReachesBothEdges(t *testing.T) {
	m := model{hostName: "GLaDOS", hostStats: probe.Stats{Cores: 12, Load: 4.1,
		MemUsed: 8 << 30, MemTotal: 31 << 30, DiskUsed: 780 << 30, DiskTotal: 930 << 30}}

	for _, width := range []int{60, 100, 160} {
		interior := strings.Split(ansi.Strip(m.renderHostBar(width)), "\n")[1]
		if !strings.HasPrefix(interior, "│ G") || strings.HasSuffix(interior, "  │") {
			t.Errorf("%d cols: content should span the whole bar: %q", width, interior)
		}
	}
}

func TestSelectedRowFillsEveryCell(t *testing.T) {
	stylingEnabled(t)

	m := model{spinner: spinner.New(), tasks: map[string]string{}, modeIdx: 0}
	it := item{t: target.Target{Provider: target.ProviderLima, Name: "kyoto",
		Status: target.StatusRunning}, autostart: true, key: 'k'}

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

func TestMeterRendersEitherGlyphSet(t *testing.T) {
	for _, c := range []struct {
		glyphs meterGlyphs
		want   string
	}{
		{glyphs: thinGlyphs, want: "━━╸─"},
		{glyphs: blockGlyphs, want: "██▓░"},
	} {
		if got := ansi.Strip(meter(0.6, 4, c.glyphs)); got != c.want {
			t.Errorf("meter(0.6, 4, %v) = %q, want %q", c.glyphs, got, c.want)
		}
	}
}

func TestAreaChartFillsItsBox(t *testing.T) {
	plain := lipgloss.NewStyle()
	spiky := []float64{0.1, 0.9, 0.2, 0.8, 0.05}

	for _, c := range []struct {
		name    string
		samples []float64
	}{
		{name: "spiky", samples: spiky},
		{name: "flat", samples: []float64{0.3, 0.3, 0.3}},
		{name: "zeroes", samples: []float64{0, 0, 0}},
		{name: "empty", samples: nil},
		{name: "overlong", samples: append(append([]float64{}, spiky...), spiky...)},
	} {
		plot := areaChart(c.samples, 8, 3, chartCeiling(c.samples), plain)
		if len(plot) != 3 {
			t.Errorf("%s: got %d rows, want 3", c.name, len(plot))
		}
		for r, line := range plot {
			if got := lipgloss.Width(line); got != 8 {
				t.Errorf("%s: row %d spans %d cells, want 8", c.name, r, got)
			}
		}
	}

	if got := areaChart(spiky, 0, 3, chartCeiling(spiky), plain); got != nil {
		t.Errorf("a chart with no room renders nothing, got %v", got)
	}
}

func TestAreaChartScalesToItsPeak(t *testing.T) {
	plain := lipgloss.NewStyle()

	quietWindow := []float64{0.01, 0.04, 0.02}
	quiet := ansi.Strip(strings.Join(areaChart(quietWindow, 3, 2, chartCeiling(quietWindow), plain), "\n"))
	if !strings.ContainsRune(quiet, sparkLevels[len(sparkLevels)-1]) {
		t.Errorf("an idle window should still reach the top of the chart, got %q", quiet)
	}

	if got := chartCeiling(nil); got != minChartCeiling {
		t.Errorf("an empty window falls back to the floor, got %v", got)
	}
	if got := chartCeiling([]float64{0, 0}); got != minChartCeiling {
		t.Errorf("an all-zero window must not divide by zero, got %v", got)
	}
	if got := chartCeiling([]float64{0.1, 0.7, 0.3}); got != 0.7 {
		t.Errorf("the ceiling is the window peak, got %v", got)
	}
}

func TestChartHeightStaysWithinItsCap(t *testing.T) {
	for _, c := range []struct{ available, want int }{
		{available: 40, want: maxChartRows},
		{available: 16, want: maxChartRows},
		{available: 8, want: minChartRows},
		{available: 2, want: minChartRows},
	} {
		if got := chartHeight(c.available); got != c.want {
			t.Errorf("chartHeight(%d) = %d, want %d", c.available, got, c.want)
		}
	}
}

func TestFractionsGuardAgainstZeroTotals(t *testing.T) {
	if got := loadFraction(2, 0); got != 0 {
		t.Errorf("load with no core count is unknowable, got %v", got)
	}
	if got := usedFraction(5, 0); got != 0 {
		t.Errorf("usage of nothing is unknowable, got %v", got)
	}
	if got := loadFraction(3, 6); got != 0.5 {
		t.Errorf("load normalises against cores, got %v", got)
	}
}
