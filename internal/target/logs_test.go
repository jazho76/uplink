package target

import "testing"

func TestDerivedFillsOnlyDetectedLogs(t *testing.T) {
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
		derived := declared.Derived(c.profile)
		if derived.Logs.Command != c.want || derived.Logs.Where != c.logs.Where {
			t.Errorf("%s: want %q, got %+v", name, c.want, derived.Logs)
		}
		if declared.Logs != c.logs {
			t.Errorf("%s: the declaration must not change, got %+v", name, declared.Logs)
		}
		if again := derived.Derived(unified); again.Logs != derived.Logs && c.want != "" {
			t.Errorf("%s: deriving twice should change nothing, got %+v", name, again.Logs)
		}
	}
}
