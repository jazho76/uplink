package tui

import (
	"math"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/jazho76/uplink/internal/ui"
)

type meterGlyphs struct{ fill, partial, track string }

var (
	thinGlyphs  = meterGlyphs{fill: "━", partial: "╸", track: "─"}
	blockGlyphs = meterGlyphs{fill: "█", partial: "▓", track: "░"}
)

const (
	meterWarn     = 0.75
	meterCritical = 0.90

	maxLiveSamples = 180
)

var sparkLevels = []rune("▁▂▃▄▅▆▇█")

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

func meter(fraction float64, width int, glyphs meterGlyphs) string {
	if width < 1 {
		return ""
	}
	fraction = clampFraction(fraction)

	exact := fraction * float64(width)
	filled := int(exact)
	bar := strings.Repeat(glyphs.fill, filled)
	if filled < width && exact > float64(filled) {
		bar += glyphs.partial
		filled++
	}

	return meterStyle(fraction).Render(bar) +
		labelStyle.Render(strings.Repeat(glyphs.track, width-filled))
}

func stepIndex(fraction float64, steps int) int {
	return min(max(int(math.Ceil(fraction*float64(steps)))-1, 0), steps-1)
}

func sparkline(samples []float64, width int, fill lipgloss.Style) string {
	if width < 1 {
		return ""
	}
	samples = newest(samples, width)

	var trace strings.Builder
	for _, s := range samples {
		trace.WriteRune(sparkLevels[stepIndex(clampFraction(s), len(sparkLevels))])
	}

	return labelStyle.Render(strings.Repeat(thinGlyphs.track, width-len(samples))) +
		fill.Render(trace.String())
}

func newest(samples []float64, n int) []float64 {
	if len(samples) <= n {
		return samples
	}
	return samples[len(samples)-n:]
}

func appendSample(history []float64, sample float64) []float64 {
	return newest(append(history, sample), maxLiveSamples)
}

func loadFraction(load float64, cores int) float64 {
	if cores <= 0 {
		return 0
	}
	return load / float64(cores)
}

func usedFraction(used, total uint64) float64 {
	if total == 0 {
		return 0
	}
	return float64(used) / float64(total)
}
