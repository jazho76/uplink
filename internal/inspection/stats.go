package inspection

import (
	"strings"

	"github.com/jazho76/uplink/internal/inspection/stats"
)

var builtinKernels = []string{"Linux", "Darwin"}

var statsSection = section{
	script: func(request Request) string {
		if request.BuiltinStats {
			return builtinStatsScript()
		}
		return stats.Script(request.Metrics)
	},
	parse: func(request Request, out string, snapshot *Snapshot) {
		metrics := request.Metrics
		if request.BuiltinStats {
			metrics = builtinMetrics(snapshot.OS)
		}
		snapshot.Readings = stats.Parse(out, metrics)
	},
}

func builtinMetrics(kernel string) []stats.Metric {
	return stats.ForOS(strings.ToLower(kernel))
}

func builtinStatsScript() string {
	lines := []string{`case "$os" in`}
	for _, kernel := range builtinKernels {
		lines = append(lines, kernel+")", stats.Script(builtinMetrics(kernel)), ";;")
	}
	return strings.Join(append(lines, "esac"), "\n")
}
