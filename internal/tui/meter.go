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

	minChartRows    = 2
	maxChartRows    = 4
	chartPaneShare  = 4
	minChartCeiling = 0.02
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

func chartCeiling(samples []float64) float64 {
	ceiling := minChartCeiling
	for _, s := range samples {
		ceiling = max(ceiling, s)
	}
	return ceiling
}

func areaChart(samples []float64, width, height int, ceiling float64, fill lipgloss.Style) []string {
	if width < 1 || height < 1 {
		return nil
	}
	samples = newest(samples, width)

	levels := len(sparkLevels)
	rows := make([]strings.Builder, height)
	for r := range rows {
		rows[r].WriteString(strings.Repeat(" ", width-len(samples)))
	}
	for _, s := range samples {
		reached := clampFraction(s/ceiling) * float64(height*levels)
		for r := range rows {
			rows[r].WriteRune(chartCell(reached, float64((height-1-r)*levels)))
		}
	}

	out := make([]string, height)
	for r := range rows {
		out[r] = fill.Render(rows[r].String())
	}
	return out
}

func chartCell(reached, floor float64) rune {
	levels := float64(len(sparkLevels))
	switch {
	case reached >= floor+levels:
		return sparkLevels[len(sparkLevels)-1]
	case reached > floor:
		return sparkLevels[int(math.Ceil(reached-floor))-1]
	default:
		return ' '
	}
}

func chartHeight(available int) int {
	return min(maxChartRows, max(available/chartPaneShare, minChartRows))
}

func sparkline(samples []float64, width int, fill lipgloss.Style) string {
	if width < 1 {
		return ""
	}
	samples = newest(samples, width)

	var trace strings.Builder
	for _, s := range samples {
		reached := max(clampFraction(s)*float64(len(sparkLevels)), 1)
		trace.WriteRune(chartCell(reached, 0))
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
