package inspection

import (
	"github.com/jazho76/uplink/internal/shell"
	"github.com/jazho76/uplink/internal/target"
)

const logSystemTag = "logsystem"

var systemdMarker = "/run/systemd/system"

var logsSection = section{
	name:    "logs",
	missing: func(missing *target.MissingSections) { missing.Profile = true },
	script: func(target.StatsPolicy) string {
		return `case "$os" in
Linux) [ -d ` + shell.SingleQuote(systemdMarker) + ` ] && command -v journalctl >/dev/null 2>&1 && echo "` + logSystemTag + ` ` + string(target.LogSystemJournal) + `" ;;
Darwin) command -v log >/dev/null 2>&1 && echo "` + logSystemTag + ` ` + string(target.LogSystemUnified) + `" ;;
esac`
	},
	parse: func(_ target.StatsPolicy, out string, environment *target.Environment) {
		lines := tagged(out, logSystemTag)
		if len(lines) == 0 {
			return
		}
		if reported := target.LogSystem(lines[len(lines)-1]); reported.Known() {
			environment.Profile.LogSystem = reported
		}
	},
}
