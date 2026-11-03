package inspection

import (
	"cmp"

	"github.com/jazho76/uplink/internal/target"
)

const osTag = "os"

var osSection = section{
	name:    "os",
	missing: func(missing *target.MissingSections) { missing.Profile = true },
	script: func([]target.Metric) string {
		return `os=$(uname -s 2>/dev/null); echo "` + osTag + ` $os"`
	},
	parse: func(_ []target.Metric, out string, environment *target.Environment) {
		environment.Profile.OS = target.UnknownOS
		if lines := tagged(out, osTag); len(lines) > 0 {
			environment.Profile.OS = cmp.Or(lines[len(lines)-1], target.UnknownOS)
		}
	},
}
