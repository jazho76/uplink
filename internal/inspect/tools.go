package inspect

import "strings"

const toolTag = "tool"

var tools = []string{"tmux"}

var toolsSection = section{
	script: func(Request) string {
		return `for tool in ` + strings.Join(tools, " ") + `; do command -v "$tool" >/dev/null 2>&1 && echo "` + toolTag + ` $tool"; done`
	},
	parse: func(_ Request, out string, snapshot *Snapshot) {
		snapshot.Tools = tagged(out, toolTag)
	},
}
