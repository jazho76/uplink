package inspection

import (
	"strings"

	"github.com/jazho76/uplink/internal/inspection/multiplexer"
)

const (
	ToolJournal = "journalctl"

	toolTag = "tool"
)

func tools() []string {
	names := make([]string, 0, len(multiplexer.All)+1)
	for _, m := range multiplexer.All {
		names = append(names, m.Tool())
	}
	return append(names, ToolJournal)
}

var toolsSection = section{
	script: func(Request) string {
		return `for tool in ` + strings.Join(tools(), " ") + `; do command -v "$tool" >/dev/null 2>&1 && echo "` + toolTag + ` $tool"; done`
	},
	parse: func(_ Request, out string, snapshot *Snapshot) {
		snapshot.Tools = tagged(out, toolTag)
	},
}
