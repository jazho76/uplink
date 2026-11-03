package inspection

import (
	"slices"
	"strings"

	"github.com/jazho76/uplink/internal/multiplexer"
	"github.com/jazho76/uplink/internal/target"
)

const toolTag = "tool"

var toolsSection = section{
	name:    "tools",
	missing: func(missing *target.MissingSections) { missing.Tools = true },
	script: func([]target.Metric) string {
		return `for tool in ` + strings.Join(multiplexer.Tools(), " ") + `; do command -v "$tool" >/dev/null 2>&1 && echo "` + toolTag + ` $tool"; done`
	},
	parse: func(_ []target.Metric, out string, environment *target.Environment) {
		known := multiplexer.Tools()
		environment.Tools = slices.DeleteFunc(tagged(out, toolTag), func(tool string) bool {
			return !slices.Contains(known, tool)
		})
	},
}
