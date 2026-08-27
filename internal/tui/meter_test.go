package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
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
