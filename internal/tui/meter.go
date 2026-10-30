package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/jazho76/uplink/internal/ui"
)

const (
	meterFill    = "█"
	meterPartial = "▓"
	meterTrack   = "░"

	meterWarn     = 0.75
	meterCritical = 0.90
)

func clampFraction(f float64) float64 {
	return min(max(f, 0), 1)
}

func meterStyle(fraction float64) lipgloss.Style {
	switch shade := clampFraction(fraction); {
	case shade >= meterCritical:
		return lipgloss.NewStyle().Foreground(ui.Red)
	case shade >= meterWarn:
		return lipgloss.NewStyle().Foreground(ui.Yellow)
	default:
		return lipgloss.NewStyle().Foreground(ui.Green)
	}
}

func meter(fraction float64, width int) string {
	if width < 1 {
		return ""
	}
	fraction = clampFraction(fraction)

	exact := fraction * float64(width)
	filled := int(exact)
	bar := strings.Repeat(meterFill, filled)
	if filled < width && exact > float64(filled) {
		bar += meterPartial
		filled++
	}

	return meterStyle(fraction).Render(bar) +
		labelStyle.Render(strings.Repeat(meterTrack, width-filled))
}
