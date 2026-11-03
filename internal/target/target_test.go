package target

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestIDsPrintQualifiedOutsideConfig(t *testing.T) {
	for _, c := range []struct {
		id   ID
		want string
	}{
		{ID{Source: ConfigSource, Key: "prod"}, "prod"},
		{ID{Source: "lima", Key: "default"}, "lima/default"},
	} {
		if got := c.id.String(); got != c.want {
			t.Errorf("%+v printed %q, want %q", c.id, got, c.want)
		}
	}
}

func TestReachUsesBackgroundForProbesOnly(t *testing.T) {
	r := Reach{Interactive: []string{"ssh", "-t", "box"}, Background: []string{"ssh", "box"}}
	if got := r.Session("x"); !slices.Equal(got, []string{"ssh", "-t", "box", "x"}) {
		t.Errorf("session = %q", got)
	}
	if got := r.Probe("x"); !slices.Equal(got, []string{"ssh", "box", "x"}) {
		t.Errorf("probe = %q", got)
	}
	if got := (Reach{Interactive: []string{"ssh", "-t", "box"}}).Probe("x"); !slices.Equal(got, []string{"ssh", "-t", "box", "x"}) {
		t.Errorf("without background, probes use interactive, got %q", got)
	}
	if got := (Reach{}).Session("x"); !slices.Equal(got, []string{"sh", "-c", "x"}) {
		t.Errorf("no reach runs here, got %q", got)
	}
}

func TestAvailableModesFollowInspection(t *testing.T) {
	box := Target{Modes: []Mode{
		{Name: "tmux", Requires: "tmux"},
		{Name: "zellij", Requires: "zellij"},
		{Name: "shell"},
	}}
	names := func(modes []Mode) string {
		var out []string
		for _, m := range modes {
			out = append(out, m.Name)
		}
		return strings.Join(out, " ")
	}
	if got := names(AvailableModes(box, Fact[[]string]{})); got != "tmux shell" {
		t.Errorf("before inspection the default is assumed, got %q", got)
	}
	if got := names(AvailableModes(box, Fact[[]string]{Value: []string{"zellij"}, Known: true})); got != "zellij shell" {
		t.Errorf("after inspection only what exists, got %q", got)
	}
}

func TestPowerIsOptional(t *testing.T) {
	if got := (Target{}).StartCommand() + (Target{}).StopCommand(); got != "" {
		t.Errorf("no power means no commands, got %q", got)
	}
	powered := Target{Power: &Power{Up: "start", Down: "stop"}}
	if powered.StartCommand() != "start" || powered.StopCommand() != "stop" {
		t.Errorf("got %+v", powered.Power)
	}
}

func TestValidateKey(t *testing.T) {
	if err := ValidateKey("ok-1.a_b"); err != nil {
		t.Error(err)
	}
	for _, bad := range []string{"", "my box", "db:prod", "-flag"} {
		if ValidateKey(bad) == nil {
			t.Errorf("%q should be refused", bad)
		}
	}
}

func TestMetricValidate(t *testing.T) {
	for _, m := range []Metric{
		{Type: TypeText, Label: "up", Value: "x", Unit: UnitSeconds},
		{Type: TypeGauge, Label: "gpu", Value: "x", Max: "100", Unit: UnitBytes},
		{Type: TypeText, Label: "agents", Value: "x"},
	} {
		if err := m.Validate(); err != nil {
			t.Errorf("%+v should be valid: %v", m, err)
		}
	}

	for want, m := range map[string]Metric{
		"type must be one of":                {Type: "cpu"},
		"gauge needs a max":                  {Type: TypeGauge, Label: "gpu", Value: "x"},
		"gauge needs a label":                {Type: TypeGauge, Value: "x", Max: "1"},
		"text needs a value":                 {Type: TypeText, Label: "x"},
		"text takes no max":                  {Type: TypeText, Label: "x", Value: "y", Max: "1"},
		"unit must be one of bytes, seconds": {Type: TypeGauge, Label: "x", Value: "y", Max: "1", Unit: "percent"},
	} {
		if err := m.Validate(); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%+v: want %q, got %v", m, want, err)
		}
	}
}

func TestFactKeepsItsValueThroughFailure(t *testing.T) {
	var fact Fact[string]
	at := time.Unix(100, 0)
	if fact.Current() {
		t.Error("an unobserved fact is not current")
	}
	fact.Observe("v", at)
	if !fact.Current() || fact.Value != "v" || fact.At != at {
		t.Errorf("got %+v", fact)
	}
	later := at.Add(time.Second)
	fact.Fail(errors.New("boom"), later)
	if fact.Current() || !fact.Known || fact.Value != "v" || fact.At != later {
		t.Errorf("a failure keeps the last value and is not current, got %+v", fact)
	}
}
