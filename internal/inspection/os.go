package inspection

import "github.com/jazho76/uplink/internal/target"

const osTag = "os"

var osSection = section{
	name: "os",
	script: func(target.StatsPolicy) string {
		return `os=$(uname -s 2>/dev/null); echo "` + osTag + ` $os"`
	},
	parse: func(_ target.StatsPolicy, out string, environment *target.Environment) {
		if lines := tagged(out, osTag); len(lines) > 0 {
			environment.OS = lines[len(lines)-1]
		}
	},
}
