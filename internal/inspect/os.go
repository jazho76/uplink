package inspect

const osTag = "os"

var osSection = section{
	script: func(Request) string {
		return `os=$(uname -s 2>/dev/null); echo "` + osTag + ` $os"`
	},
	parse: func(_ Request, out string, snapshot *Snapshot) {
		if lines := tagged(out, osTag); len(lines) > 0 {
			snapshot.OS = lines[len(lines)-1]
		}
	},
}
