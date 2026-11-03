package inspection

import (
	"strings"

	"github.com/jazho76/uplink/internal/stats"
	"github.com/jazho76/uplink/internal/target"
)

var builtinKernels = []string{"Linux", "Darwin"}

var statsSection = section{
	name:    "stats",
	missing: func(missing *target.MissingSections) { missing.Stats = true },
	script: func(policy target.StatsPolicy) string {
		if policy.Builtin {
			return builtinStatsScript()
		}
		return stats.Script(policy.Metrics)
	},
	parse: func(policy target.StatsPolicy, out string, environment *target.Environment) {
		metrics := policy.Metrics
		if policy.Builtin {
			metrics = builtinMetrics(environment.OS)
		}
		environment.Readings = stats.Parse(out, metrics)
	},
}

func builtinMetrics(kernel string) []target.Metric {
	return stats.ForOS(strings.ToLower(kernel))
}

func builtinStatsScript() string {
	lines := []string{`case "$os" in`}
	for _, kernel := range builtinKernels {
		lines = append(lines, kernel+")", stats.Script(builtinMetrics(kernel)), ";;")
	}
	return strings.Join(append(lines, "esac"), "\n")
}
