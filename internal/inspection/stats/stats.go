package stats

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/jazho76/uplink/internal/shell"
	"github.com/jazho76/uplink/internal/target"
)

func constantMax(m target.Metric) (float64, bool) {
	v, err := strconv.ParseFloat(strings.TrimSpace(m.Max), 64)
	return v, err == nil && !math.IsNaN(v)
}

func maxCommand(m target.Metric) string {
	if _, constant := constantMax(m); constant {
		return ""
	}
	return m.Max
}

func capture(tag, command string) string {
	return fmt.Sprintf(`printf '%%s\n' %s"$($limit sh -c %s </dev/null 2>/dev/null | head -n 1)"`, shell.SingleQuote(tag+" "), shell.SingleQuote(command))
}

func Script(metrics []target.Metric) string {
	var lines []string
	for i, m := range metrics {
		lines = append(lines, capture(fmt.Sprintf("%d value", i), m.Value))
		if command := maxCommand(m); command != "" {
			lines = append(lines, capture(fmt.Sprintf("%d max", i), command))
		}
	}
	return strings.Join(lines, "\n")
}

func Parse(out string, metrics []target.Metric) []target.Reading {
	values := map[int]string{}
	maxes := map[int]string{}
	for _, line := range strings.Split(out, "\n") {
		index, rest, ok := strings.Cut(line, " ")
		i, err := strconv.Atoi(index)
		if !ok || err != nil || i < 0 || i >= len(metrics) {
			continue
		}
		field, text, _ := strings.Cut(rest, " ")
		switch field {
		case "value":
			values[i] = strings.TrimSpace(text)
		case "max":
			maxes[i] = strings.TrimSpace(text)
		}
	}

	readings := make([]target.Reading, len(metrics))
	for i, m := range metrics {
		r := target.Reading{Metric: m, Text: shell.Plain(values[i])}
		value, valueErr := strconv.ParseFloat(values[i], 64)
		switch {
		case m.Type == target.TypeText && m.Unit == "":
			r.OK = r.Text != ""
		case m.Type == target.TypeText:
			r.Value, r.OK = value, valueErr == nil
		default:
			ceiling, constant := constantMax(m)
			var maxErr error
			if !constant {
				ceiling, maxErr = strconv.ParseFloat(maxes[i], 64)
			}
			r.Value, r.Max, r.OK = value, ceiling, valueErr == nil && maxErr == nil
		}
		readings[i] = r
	}
	return readings
}
