package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/jazho76/uplink/internal/probe"
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
		{fraction: 0, want: "────"},
		{fraction: 0.5, want: "━━──"},
		{fraction: 0.6, want: "━━╸─"},
		{fraction: 1, want: "━━━━"},
		{fraction: 2, want: "━━━━"},
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

func TestSparklineSharesTheMeterGeometry(t *testing.T) {
	plain := lipgloss.NewStyle()

	if got := lipgloss.Width(sparkline(nil, 12, plain)); got != 12 {
		t.Errorf("an empty sparkline still spans its width, got %d", got)
	}
	if got := ansi.Strip(sparkline(nil, 4, plain)); got != strings.Repeat(meterTrack, 4) {
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
	if got := liveBarWidth(12); got != 0 {
		t.Errorf("a pane with no room for a reading draws no bar, got width %d", got)
	}
	if got := liveBarWidth(58); got < minLiveBar {
		t.Errorf("a roomy pane should draw a bar, got width %d", got)
	}

	if got := ansi.Strip(gauge("", "7.6GiB / 31GiB")); got != "7.6GiB / 31GiB" {
		t.Errorf("an absent bar leaves the reading alone, got %q", got)
	}
	if got := ansi.Strip(gauge(meter(0.5, 4), "1.32")); got != "━━──   1.32" {
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
		items: []item{
			{t: target.Target{Name: "kyoto", Status: target.StatusRunning, CPUs: 6, Memory: 12 << 30}},
			{t: target.Target{Name: "forge", Status: target.StatusRunning, CPUs: 8, Memory: 16 << 30}},
		}}

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
