package target

import "testing"

func TestResolveFillsOnlyDetectedLogs(t *testing.T) {
	journal := Profile{LogSystem: LogSystemJournal}
	unified := Profile{LogSystem: LogSystemUnified}
	for name, c := range map[string]struct {
		logs    LogSource
		profile Profile
		want    string
	}{
		"journal":          {DetectedLogs(), journal, logReaders[LogSystemJournal]},
		"unified":          {DetectedLogs(), unified, logReaders[LogSystemUnified]},
		"nothing detected": {DetectedLogs(), Profile{}, ""},
		"unknown system":   {DetectedLogs(), Profile{LogSystem: "other"}, ""},
		"declared wins":    {LogSource{Where: InTarget, Command: "tail app.log"}, journal, "tail app.log"},
		"host command":     {LogSource{Command: "tail serial.log"}, journal, "tail serial.log"},
		"no logs":          {LogSource{}, journal, ""},
	} {
		declared := Target{Logs: c.logs}
		resolved := declared.Resolve(c.profile)
		if resolved.Logs.Command != c.want || resolved.Logs.Where != c.logs.Where {
			t.Errorf("%s: want %q, got %+v", name, c.want, resolved.Logs)
		}
		if declared.Logs != c.logs {
			t.Errorf("%s: the declaration must not change, got %+v", name, declared.Logs)
		}
		if again := resolved.Resolve(unified); again.Logs != resolved.Logs && c.want != "" {
			t.Errorf("%s: resolving twice should change nothing, got %+v", name, again.Logs)
		}
	}
}
