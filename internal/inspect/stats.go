package inspect

import "github.com/jazho76/uplink/internal/stats"

var statsSection = section{
	script: func(request Request) string { return stats.Script(request.Metrics) },
	parse: func(request Request, out string, snapshot *Snapshot) {
		snapshot.Readings = stats.Parse(out, request.Metrics)
	},
}
