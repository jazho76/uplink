package tui

import (
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
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
		{fraction: 0, want: colorGreen},
		{fraction: meterWarn - 0.01, want: colorGreen},
		{fraction: meterWarn, want: colorYellow},
		{fraction: meterCritical - 0.01, want: colorYellow},
		{fraction: meterCritical, want: colorRed},
		{fraction: 5, want: colorRed},
	} {
		if got := meterStyle(c.fraction).GetForeground(); got != c.want {
			t.Errorf("meterStyle(%v) = %v, want %v", c.fraction, got, c.want)
		}
	}
}
