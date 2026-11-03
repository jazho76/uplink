package target

type Place int

const (
	InHost Place = iota
	InTarget
)

type LogSource struct {
	Where   Place
	Command string
}

func DetectedLogs() LogSource { return LogSource{Where: InTarget} }

func (l LogSource) Detected() bool { return l.Where == InTarget && l.Command == "" }

type LogSystem string

const (
	LogSystemJournal LogSystem = "journal"
	LogSystemUnified LogSystem = "unified"
)

var logReaders = map[LogSystem]string{
	LogSystemJournal: "journalctl --system -n 500 --no-pager -o short",
	LogSystemUnified: "log show --last 5m --style syslog | tail -n 500",
}

func (s LogSystem) Known() bool {
	_, known := logReaders[s]
	return known
}

type Profile struct {
	LogSystem LogSystem
}

func (t Target) Resolve(p Profile) Target {
	if t.Logs.Detected() {
		t.Logs.Command = logReaders[p.LogSystem]
	}
	return t
}
