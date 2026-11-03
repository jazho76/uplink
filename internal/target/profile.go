package target

const UnknownOS = "unknown"

type Profile struct {
	OS        string
	LogSystem LogSystem
}

func (t Target) Resolve(p Profile) Target {
	if t.Logs.Detected() {
		t.Logs.Command = logReaders[p.LogSystem]
	}
	if t.DetectsMetrics() && p.OS != "" {
		t.Metrics = BuiltinMetrics(p.OS)
	}
	return t
}
