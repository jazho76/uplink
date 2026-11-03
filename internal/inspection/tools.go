package inspection

import (
	"slices"
	"strings"

	"github.com/jazho76/uplink/internal/multiplexer"
	"github.com/jazho76/uplink/internal/target"
)

const toolTag = "tool"

func tools() []string {
	return append(multiplexer.Tools(), target.ToolJournal)
}

var toolsSection = section{
	name:    "tools",
	missing: func(missing *target.MissingSections) { missing.Tools = true },
	script: func(target.StatsPolicy) string {
		return `for tool in ` + strings.Join(tools(), " ") + `; do command -v "$tool" >/dev/null 2>&1 && echo "` + toolTag + ` $tool"; done`
	},
	parse: func(_ target.StatsPolicy, out string, environment *target.Environment) {
		known := tools()
		environment.Tools = slices.DeleteFunc(tagged(out, toolTag), func(tool string) bool {
			return !slices.Contains(known, tool)
		})
	},
}
