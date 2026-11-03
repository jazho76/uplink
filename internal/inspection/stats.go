package inspection

import (
	"github.com/jazho76/uplink/internal/stats"
	"github.com/jazho76/uplink/internal/target"
)

var statsSection = section{
	name:    "stats",
	missing: func(missing *target.MissingSections) { missing.Stats = true },
	script:  stats.Script,
	parse: func(metrics []target.Metric, out string, environment *target.Environment) {
		environment.Readings = stats.Parse(out, metrics)
	},
}
