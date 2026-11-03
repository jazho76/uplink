package dashboard

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/jazho76/uplink/internal/launcher"
	"github.com/jazho76/uplink/internal/stats"
	"github.com/jazho76/uplink/internal/target"
)

const liveRule = "live ─"

func focusedOnBuiltin() model {
	box := remote("box")
	box.Stats = target.StatsPolicy{Builtin: true}
	m, _ := newTestModel()
	return focus(sized(withTargets(m, box)), "box")
}

func inspectedWith(m model, readings []target.Reading) model {
	return reported(m, configID("box"), target.Environment{OS: "Linux", Readings: readings})
}

func TestPreviewLeadsWithLiveData(t *testing.T) {
	m, _ := newTestModel()
	m = sized(m)
	m = focus(probed(m, "forge", true), "forge")
	m = observed(m, "forge", target.Environment{Readings: linuxReadings(1, 4, 1<<30, 4<<30, 0)})

	preview := ansi.Strip(m.previewBody(60, 30))
	if !strings.Contains(preview, "live") {
		t.Fatalf("preview should carry the live block:\n%s", preview)
	}
	for _, want := range []string{"load", "memory", "disk"} {
		if !strings.Contains(preview, want) {
			t.Errorf("live block missing %q:\n%s", want, preview)
		}
	}
}

func TestPendingProbeShowsNoReadings(t *testing.T) {
	m, _ := newTestModel()
	m = sized(m)
	m = focus(m, "forge")
	m.send(launcher.Selected{ID: configID("forge")})

	preview := ansi.Strip(m.previewBody(60, 30))
	if strings.Contains(preview, "0.00") || !strings.Contains(preview, "…") {
		t.Errorf("a probe in flight has no readings yet:\n%s", preview)
	}
	if got := m.statusOf(itemNamed(m, "forge").target); got != statusUnknown {
		t.Errorf("a probe in flight leaves the status unknown, got %q", got)
	}
}

func TestDownTargetShowsNoStaleUptime(t *testing.T) {
	m, _ := newTestModel()
	m = sized(m)
	m = focus(probed(m, "forge", true), "forge")
	m = observed(m, "forge", target.Environment{Readings: linuxReadings(0, 4, 0, 0, 76*time.Hour)})

	if preview := ansi.Strip(m.previewBody(60, 30)); !strings.Contains(preview, "3d 4h") {
		t.Fatalf("a reachable target should report uptime in seconds as a duration:\n%s", preview)
	}

	m = focus(probed(m, "forge", false), "forge")
	if preview := ansi.Strip(m.previewBody(60, 30)); strings.Contains(preview, "3d 4h") {
		t.Errorf("a down target must not show the uptime from its last probe:\n%s", preview)
	}
	if preview := ansi.Strip(m.previewBody(60, 20)); !strings.Contains(preview, "unavailable") || strings.Contains(preview, "load") {
		t.Errorf("a failed probe should say its readings are unavailable, naming no metric:\n%s", preview)
	}
}

func TestLiveGaugesAlignTheirPercentColumn(t *testing.T) {
	block := ansi.Strip(renderFields(gaugeFields([]gaugeRow{
		{label: "load", fraction: 0.34, reading: "4.10 / 12"},
		{label: "memory", fraction: 0.26, reading: "8/31GiB"},
		{label: "gpu", reading: "?", missing: true},
		{label: "disk", fraction: 0.75, reading: "112/150GiB"},
	}, 12), 60))

	var columns []int
	for _, line := range strings.Split(strings.TrimRight(block, "\n"), "\n") {
		columns = append(columns, strings.LastIndexAny(line, "%?"))
	}
	for i, at := range columns {
		if at < 0 || at != columns[0] {
			t.Errorf("row %d puts its percent at column %d, want %d:\n%s", i, at, columns[0], block)
		}
	}
}

func TestLiveBlockFollowsTheDeclaredMetrics(t *testing.T) {
	gpu := target.Metric{Type: target.TypeGauge, Label: "gpu", Value: "x", Max: "100"}
	cache := target.Metric{Type: target.TypeGauge, Label: "cache", Value: "x", Max: "x", Unit: target.UnitBytes}
	agents := target.Metric{Type: target.TypeText, Label: "agents", Value: "x"}
	broken := target.Metric{Type: target.TypeGauge, Label: "broken", Value: "x", Max: "1"}

	m, _ := newTestModel()
	box := vm("box")
	box.Stats = target.StatsPolicy{Metrics: []target.Metric{gpu, cache, agents, broken}}
	m = sized(withTargets(m, box))
	m = focus(m, "box")
	m = observed(m, "box", target.Environment{Readings: []target.Reading{reading(gpu, 37, 100), reading(cache, 1<<30, 4<<30), {Metric: agents, Text: "3", OK: true}, {Metric: broken}}})

	preview := ansi.Strip(m.previewBody(70, 40))
	order := []string{"gpu", "37 / 100", "cache", "1/4GiB", "agents", "3", "broken", "?"}
	last := -1
	for _, want := range order {
		at := strings.Index(preview[last+1:], want)
		if at < 0 {
			t.Fatalf("live block missing %q after position %d:\n%s", want, last, preview)
		}
		last += at + 1
	}
	if strings.Contains(preview, "load") || strings.Contains(preview, "memory") {
		t.Errorf("only declared metrics are shown:\n%s", preview)
	}
}

func TestBuiltinStatsWaitForTheFirstCheck(t *testing.T) {
	got := preview(focusedOnBuiltin())
	if !strings.Contains(got, liveRule) || !strings.Contains(got, "…") {
		t.Errorf("want the live block pending, got:\n%s", got)
	}
}

func TestBuiltinStatsShowTheirReadings(t *testing.T) {
	m := inspectedWith(focusedOnBuiltin(), linuxReadings(1, 4, 1<<30, 4<<30, 0))
	if got := preview(m); !strings.Contains(got, liveRule) || !strings.Contains(got, "memory") {
		t.Errorf("want readings, got:\n%s", got)
	}
	if got := preview(probed(m, "box", false)); !strings.Contains(got, "unavailable") {
		t.Errorf("a known target going down has its stats unavailable, got:\n%s", got)
	}
}

func TestUnsupportedKernelHidesTheLiveBlock(t *testing.T) {
	m := inspectedWith(focusedOnBuiltin(), nil)
	if got := preview(m); strings.Contains(got, liveRule) {
		t.Errorf("no built-in stats for this kernel, got:\n%s", got)
	}
	next, _ := m.Update(launcher.ConnectionEnded{})
	m = next.(model)
	if got := preview(m); strings.Contains(got, liveRule) {
		t.Errorf("going stale must not flash the block back, got:\n%s", got)
	}
}

func TestBuiltinStatsStayHiddenWhenNeverInspected(t *testing.T) {
	if got := preview(probed(focusedOnBuiltin(), "box", false)); strings.Contains(got, liveRule) {
		t.Errorf("a stopped target of unknown kernel has nothing to promise, got:\n%s", got)
	}
	if got := preview(failedInspection(focusedOnBuiltin(), configID("box"))); strings.Contains(got, liveRule) {
		t.Errorf("a failed first inspection has nothing to promise, got:\n%s", got)
	}
}

func TestReloadDropsReadingsOfChangedStats(t *testing.T) {
	m, _ := newTestModel()
	m = sized(m)
	m = probed(probed(m, "forge", true), "tokyo", true)

	changed := vm("forge")
	changed.Stats = target.StatsPolicy{Metrics: linuxMetrics[1:2]}
	m.runner.Catalog = planOf([]target.Target{hostTarget(), changed, vm("tokyo")}, nil)

	next, _ := reload(m, "")
	m = next.(model)
	if forge := m.state.Observation(configID("forge")); forge.Stats.Value != nil || forge.Status != target.StatusUp {
		t.Errorf("changed stats drop their readings but keep the status, got %+v", forge)
	}
	if m.state.Observation(configID("tokyo")).Stats.Value == nil {
		t.Errorf("unchanged stats keep their readings")
	}
}

func TestReloadDropsReadingsWhenStatsBecomeBuiltin(t *testing.T) {
	m, _ := newTestModel()
	m = probed(sized(m), "forge", true)

	changed := vm("forge")
	changed.Stats = target.StatsPolicy{Builtin: true}
	m.runner.Catalog = planOf([]target.Target{hostTarget(), changed, vm("tokyo")}, nil)

	next, _ := reload(m, "")
	if got := next.(model).state.Observation(configID("forge")).Stats.Value; got != nil {
		t.Errorf("the old metrics no longer apply, got %+v", got)
	}
}

func TestNarrowLiveBlockDropsTheBarNotTheReading(t *testing.T) {
	gauges := []gaugeRow{{label: "memory", fraction: 0.5, reading: "7.6/31GiB"}}

	cramped := ansi.Strip(gaugeFields(gauges, 24)[0].value)
	if strings.ContainsAny(cramped, meterFill+meterTrack) {
		t.Errorf("a pane with no room for a reading draws no bar, got %q", cramped)
	}
	if !strings.Contains(cramped, "7.6/31GiB") {
		t.Errorf("the reading survives however narrow the pane, got %q", cramped)
	}

	roomy := ansi.Strip(gaugeFields(gauges, 70)[0].value)
	if !strings.Contains(roomy, meterFill) {
		t.Errorf("a roomy pane should draw a bar, got %q", roomy)
	}

	if got := ansi.Strip(gauge("", "7.6GiB / 31GiB", gaugeGap)); got != "7.6GiB / 31GiB" {
		t.Errorf("an absent bar leaves the reading alone, got %q", got)
	}
	if got := ansi.Strip(gauge(meter(0.5, 4), "1.32", gaugeGap)); got != "██░░   1.32" {
		t.Errorf("a bar keeps its reading beside it, got %q", got)
	}
}

func TestNaNFromATargetShowsAQuestionMark(t *testing.T) {
	metric := target.Metric{Type: target.TypeGauge, Label: "gpu", Value: "x", Max: "10"}
	box := vm("box")
	box.Stats = target.StatsPolicy{Metrics: []target.Metric{metric}}
	m, _ := newTestModel()
	m = focus(sized(withTargets(m, box)), "box")

	readings := stats.Parse("0 value NaN\n", box.Stats.Metrics)
	m = reported(m, configID("box"), target.Environment{Readings: readings})
	if got := preview(m); !strings.Contains(got, "gpu") || !strings.Contains(got, "?") {
		t.Errorf("want the gauge marked missing, got:\n%s", got)
	}
	if got := m.View(); got == "" {
		t.Error("the dashboard must survive it")
	}
}

func TestMissingStatsShowUnavailableNotNothing(t *testing.T) {
	box := vm("box")
	box.Stats = target.StatsPolicy{Builtin: true}
	m, _ := newTestModel()
	m = focus(sized(withTargets(m, box)), "box")
	m = reported(m, configID("box"), target.Environment{Readings: []target.Reading{reading(linuxMetrics[0], 1, 4)}})
	m = reported(m, configID("box"), target.Environment{Missing: target.MissingSections{Stats: true}})
	if got := preview(m); !strings.Contains(got, liveRule) || !strings.Contains(got, "unavailable") {
		t.Errorf("the block stays and says so, got:\n%s", got)
	}
}
