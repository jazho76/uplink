package local

import "testing"

func TestJournalEmptyMarkerReadsAsNoLogs(t *testing.T) {
	if got := dropEmptyMarker(journalEmpty); got != "" {
		t.Errorf("an empty journal should render nothing, got %q", got)
	}
	if got := dropEmptyMarker("boot line"); got != "boot line" {
		t.Errorf("real entries should survive, got %q", got)
	}
}
