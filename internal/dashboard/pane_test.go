package dashboard

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/jazho76/uplink/internal/launcher"
	"github.com/jazho76/uplink/internal/target"
)

func TestGroupsBecomePanesInConfigOrder(t *testing.T) {
	m, _ := newTestModel()
	stray := remote("stray")
	stray.Group = target.DefaultGroup
	m = withTargets(m, remote("berlin"), vm("forge"), stray, remote("dojo"))

	var groups []string
	for _, p := range m.panes {
		groups = append(groups, p.group)
	}
	if got := strings.Join(groups, " "); got != "local remotes vms ungrouped" {
		t.Errorf("panes follow first appearance, got %q", got)
	}
	if remotes := m.panes[1]; len(remotes.items) != 2 || remotes.items[1].name() != "dojo" {
		t.Errorf("a later member joins its group's pane, got %+v", remotes.items)
	}
}

func TestCursorWrapsThroughPanes(t *testing.T) {
	m, _ := newTestModel()
	m = sized(m)

	for _, want := range []string{"forge", "tokyo"} {
		m = key(m, "down")
		if got := m.selected().name(); got != want {
			t.Fatalf("down should reach %q, got %q", want, got)
		}
	}
	if got := m.focusedGroup(); got != "vms" {
		t.Errorf("spilling should carry the focus with it, got %q", got)
	}

	m = key(m, "down")
	if got := m.selected().name(); got != "host" {
		t.Fatalf("down from the last item wraps to the first, got %q", got)
	}
	if got := m.focusedGroup(); got != "local" {
		t.Errorf("wrapping should carry the focus too, got %q", got)
	}

	m = key(m, "up")
	if got := m.selected().name(); got != "tokyo" {
		t.Fatalf("up from the first item lands on the last, got %q", got)
	}
}

func TestCursorSkipsAnEmptyGroup(t *testing.T) {
	m, _ := newTestModel()
	m = sized(withTargets(m))

	if len(m.panes) != 1 {
		t.Fatalf("a group with no targets gets no pane, got %d panes", len(m.panes))
	}
	m = key(m, "down")
	if got := m.selected().name(); got != "host" {
		t.Fatalf("down in a one-row pane stays on its only row, got %q", got)
	}
}

func TestPaneFocusRemembersItsCursor(t *testing.T) {
	m, _ := newTestModel()
	m = sized(m)

	m = key(m, "v")
	m = key(m, "down")
	if got := m.selected().name(); got != "tokyo" {
		t.Fatalf("v then down should land on tokyo, got %q", got)
	}

	m = key(m, "v")
	if got := m.selected().name(); got != "tokyo" {
		t.Fatalf("re-pressing the focused pane's key must be a no-op, got %q", got)
	}

	m = key(m, "l")
	if got := m.selected().name(); got != "host" {
		t.Fatalf("l should focus the local pane, got %q", got)
	}

	m = key(m, "v")
	if got := m.selected().name(); got != "tokyo" {
		t.Fatalf("a pane should keep its own cursor across a focus change, got %q", got)
	}
}

func TestPaneKeysDerivedFromGroup(t *testing.T) {
	m, _ := newTestModel()
	m = sized(m)

	if len(m.panes) != 2 {
		t.Fatalf("want one pane per group, got %d", len(m.panes))
	}
	if m.panes[0].group != "local" || m.panes[1].group != "vms" {
		t.Fatalf("unexpected groups: %q %q", m.panes[0].group, m.panes[1].group)
	}
	if m.panes[0].key != 'l' || m.panes[1].key != 'v' {
		t.Fatalf("unexpected pane keys: %q %q", m.panes[0].key, m.panes[1].key)
	}
	if !strings.Contains(m.View(), "local") {
		t.Errorf("the local target should land in a pane named for its group")
	}
}

func TestPaneKeysFallThroughOnCollision(t *testing.T) {
	panes := []pane{
		{group: "remotes", items: []item{{}}},
		{group: "rigs", items: []item{{}}},
		{group: "queue", items: []item{{}}},
	}
	assignPaneKeys(panes)

	if panes[0].key != 'r' || panes[1].key != 'i' || panes[2].key != 'u' {
		t.Errorf("keys should fall through collisions and reserved letters, got %q %q %q",
			panes[0].key, panes[1].key, panes[2].key)
	}
}

func TestFirstFreeLetterSkipsTaken(t *testing.T) {
	for _, c := range []struct {
		word, taken string
		want        rune
	}{
		{word: "vms", taken: reservedPaneKeys, want: 'v'},
		{word: "remotes", taken: reservedPaneKeys + "r", want: 'e'},
		{word: "Vms", taken: reservedPaneKeys, want: 'v'},
		{word: "kqj", taken: reservedPaneKeys, want: 0},
		{word: "42", taken: "", want: 0},
	} {
		if got := firstFreeLetter(c.word, c.taken); got != c.want {
			t.Errorf("firstFreeLetter(%q, %q) = %q, want %q", c.word, c.taken, got, c.want)
		}
	}
}

func TestItemKeysComeFromTargetNames(t *testing.T) {
	m, _ := newTestModel()

	for name, want := range map[string]rune{"host": 'h', "forge": 'f', "tokyo": 't'} {
		if got := itemNamed(m, name).key; got != want {
			t.Errorf("%s should claim %q, got %q", name, want, got)
		}
	}
}

func TestItemKeysAvoidOtherPaneKeys(t *testing.T) {
	panes := []pane{
		{group: "local", items: []item{named("host")}},
		{group: "vms", items: []item{named("vault")}},
		{group: "remotes", items: []item{named("riga"), named("lisbon")}},
	}
	assignPaneKeys(panes)
	assignItemKeys(panes)

	keys := map[string]rune{}
	for _, p := range panes {
		for _, it := range p.items {
			keys[it.name()] = it.key
		}
	}

	if keys["riga"] != 'r' || keys["vault"] != 'v' {
		t.Errorf("an item may claim its own pane's letter, got riga=%q vault=%q", keys["riga"], keys["vault"])
	}
	if keys["lisbon"] != 'i' {
		t.Errorf("l belongs to the local pane, so lisbon should fall through to i, got %q", keys["lisbon"])
	}
}

func TestItemKeysFallThroughWithinPane(t *testing.T) {
	panes := panesOf(named("dojo"), named("dublin"), named("denver"))

	want := []rune{'d', 'u', 'e'}
	for i, it := range panes[0].items {
		if it.key != want[i] {
			t.Errorf("%s took %q, want %q", it.name(), it.key, want[i])
		}
	}
}

func TestItemKeyExhaustionLeavesItemUnaddressable(t *testing.T) {
	panes := panesOf(named("ab"), named("ba"), named("aab"), named("jkq"))

	last := panes[0].items[2]
	if last.key != 0 {
		t.Errorf("an item with no free letter must stay unaddressable, got %q", last.key)
	}
	if reserved := panes[0].items[3]; reserved.key != 0 {
		t.Errorf("reserved letters are never handed out, got %q", reserved.key)
	}
	if got := accentLetter(last.name(), last.key, plainStyle); got != plainStyle.Render(last.name()) {
		t.Errorf("an unaddressable item carries no accent, got %q", got)
	}
	if _, ok := itemForKey(panes[0], "a"); !ok {
		t.Errorf("the first claimant of a letter should still resolve")
	}
}

func TestPaneLetterThenItemLetterSelects(t *testing.T) {
	m, _ := newTestModel()
	m = sized(m)

	if got := m.selected().name(); got != "host" {
		t.Fatalf("focus should start on the first pane, got %q", got)
	}

	m = key(m, "v")
	if got := m.selected().name(); got != "forge" {
		t.Fatalf("v should focus the vms pane, got %q", got)
	}

	m = key(m, "t")
	if got := m.selected().name(); got != "tokyo" {
		t.Fatalf("t should select tokyo inside the focused pane, got %q", got)
	}

	m = key(m, "z")
	if got := m.selected().name(); got != "tokyo" {
		t.Fatalf("a letter no item holds must change nothing, got %q", got)
	}
	if m.state.Notice() != "" {
		t.Errorf("a miss must stay silent, got status %q", m.state.Notice())
	}
}

func TestFocusedPaneLetterFallsThroughToItsItems(t *testing.T) {
	m, _ := newTestModel()
	m = sized(withTargets(m, vm("forge"), vm("vault")))

	m = key(m, "v")
	if got := m.selected().name(); got != "forge" {
		t.Fatalf("v should focus the vms pane, got %q", got)
	}

	m = key(m, "v")
	if got := m.selected().name(); got != "vault" {
		t.Fatalf("the focused pane's own letter should reach its items, got %q", got)
	}
}

func TestItemLettersOnlyRenderInFocusedPane(t *testing.T) {
	stylingEnabled(t)

	m, _ := newTestModel()
	m = sized(m)

	forge := m.panes[1].items[0]
	if forge.key == 0 {
		t.Fatalf("forge should have claimed a letter")
	}
	plain := m.paneRow(item{target: forge.target}, false, true, 30)

	if accented := m.paneRow(forge, false, true, 30); accented == plain {
		t.Errorf("the focused pane should mark its item letters: %q", accented)
	}
	if unfocused := m.paneRow(forge, false, false, 30); unfocused != plain {
		t.Errorf("an unfocused pane must not advertise letters that would not work: %q", unfocused)
	}
}

func TestPaneScrollFollowsCursor(t *testing.T) {
	for _, c := range []struct{ cursor, count, rows, want int }{
		{cursor: 0, count: 10, rows: 4, want: 0},
		{cursor: 3, count: 10, rows: 4, want: 0},
		{cursor: 4, count: 10, rows: 4, want: 1},
		{cursor: 9, count: 10, rows: 4, want: 6},
		{cursor: 2, count: 3, rows: 5, want: 0},
	} {
		if got := scrollOffset(c.cursor, c.count, c.rows); got != c.want {
			t.Errorf("scrollOffset(%d, %d, %d) = %d, want %d", c.cursor, c.count, c.rows, got, c.want)
		}
	}
}

func TestPaneRowsSpreadSlackEvenly(t *testing.T) {
	panes := []pane{{items: make([]item, 1)}, {items: make([]item, 2)}, {items: make([]item, 2)}}

	rows := distributeRows(panes, 20)
	sum := 0
	for _, r := range rows {
		sum += r
	}
	if sum != 20 {
		t.Fatalf("panes should fill the column, got %d of 20", sum)
	}
	if rows[0] != 6 || rows[1] != 7 || rows[2] != 7 {
		t.Errorf("slack should spread evenly, got %v", rows)
	}

	for _, r := range distributeRows(panes, 8) {
		if r < minPaneRows {
			t.Errorf("an overfull column must not starve a pane, got %d", r)
		}
	}
}

func TestLocalPaneKeepsItsHeight(t *testing.T) {
	panes := []pane{{items: make([]item, 1), compact: true}, {items: make([]item, 2)}, {items: make([]item, 2)}}
	rows := distributeRows(panes, 20)
	if rows[0] != 1+borderCells || rows[0]+rows[1]+rows[2] != 20 {
		t.Errorf("the local pane should stay one row tall and the rest fill the column, got %v", rows)
	}

	alone := distributeRows([]pane{{items: make([]item, 1), compact: true}}, 20)
	if alone[0] != 20 {
		t.Errorf("a lone local pane still fills the column, got %v", alone)
	}
}

func TestPaneIsCompactWhenNothingInItIsWatched(t *testing.T) {
	watched := vm("forge")
	watched.Watch = true
	stray := vm("stray")
	stray.Group, stray.Watch = "local", true

	m := testModel([]target.Target{hostTarget(), watched}, &fakeBackend{})
	if !m.panes[0].compact || m.panes[1].compact {
		t.Errorf("only the unwatched pane should be compact, got %v and %v", m.panes[0].compact, m.panes[1].compact)
	}
	m = testModel([]target.Target{hostTarget(), stray}, &fakeBackend{})
	if m.panes[0].compact {
		t.Error("a pane mixing in watched targets is not compact")
	}
}

func TestPaneSummaryWaitsForKnownStatuses(t *testing.T) {
	m, _ := newTestModel()
	up, off, unprobed := item{target: remote("up")}, item{target: remote("off")}, item{target: remote("new")}
	m = probed(probed(m, "up", true), "off", false)

	if got := m.paneSummary(pane{items: []item{up, off}}); got != "1 up" {
		t.Errorf("want %q, got %q", "1 up", got)
	}
	if got := m.paneSummary(pane{items: []item{up, off, unprobed}}); got != "" {
		t.Errorf("one unprobed member must suppress the count, got %q", got)
	}
	if got := m.paneSummary(pane{items: []item{up}}); got != "1 up" {
		t.Errorf("a lone member is counted like any other, got %q", got)
	}
	if got := m.paneSummary(pane{}); got != "" {
		t.Errorf("an empty pane has nothing to count, got %q", got)
	}
	if got := m.paneSummary(pane{items: []item{up}, compact: true}); got != "" {
		t.Errorf("the local pane shows no count, got %q", got)
	}
}

func TestPaneSummaryOnlyHidesForNeverChecked(t *testing.T) {
	forge, tokyo := vm("forge"), vm("tokyo")
	forge.Watch, tokyo.Watch = true, true
	m, _ := newTestModel()
	m = sized(withTargets(m, forge, tokyo))
	vms := m.panes[1]
	if got := m.paneSummary(vms); got != "" {
		t.Errorf("nothing is known yet, got %q", got)
	}
	for _, name := range []string{"forge", "tokyo"} {
		next, _ := m.Update(launcher.Checked{ID: configID(name), Status: target.StatusUnknown, Reason: "unreachable"})
		m = next.(model)
	}
	if got := m.paneSummary(vms); got != "0 up" {
		t.Errorf("a stuck unknown must not hide the summary, got %q", got)
	}
}

func TestPaneEdgeIgnoresInvisibleCaptions(t *testing.T) {
	const styledButEmpty = "\x1b[94m\x1b[0m"
	closed := "╰" + strings.Repeat("─", 22) + "╯"

	for _, c := range []struct{ name, left, right string }{
		{name: "no captions"},
		{name: "invisible right", right: styledButEmpty},
		{name: "invisible left", left: styledButEmpty},
		{name: "invisible both", left: styledButEmpty, right: styledButEmpty},
	} {
		edge := ansi.Strip(paneEdge("╰", "╯", c.left, c.right, 24, paneBorder))
		if edge != closed {
			t.Errorf("%s: edge did not close: %q", c.name, edge)
		}
	}

	edge := ansi.Strip(paneEdge("╭", "╮", "vms", "2 up", 24, paneBorder))
	if lipgloss.Width(edge) != 24 {
		t.Errorf("a captioned edge must still span the pane, got %d: %q", lipgloss.Width(edge), edge)
	}
	if !strings.HasPrefix(edge, "╭ vms ") || !strings.HasSuffix(edge, " 2 up ╮") {
		t.Errorf("captions should sit against the corners, got %q", edge)
	}
}

func TestNoTargetsPointsAtTheConfig(t *testing.T) {
	m, backend := newTestModel()
	m = replaceTargets(m, nil)
	m = sized(m)

	for _, k := range []string{"j", "k", "tab", "enter", "ctrl+s", "ctrl+x", "ctrl+l"} {
		m = key(m, k)
	}
	if len(backend.sessions) != 0 || len(backend.hooks) != 0 || m.state.LogsOpen() {
		t.Errorf("keys with no target do nothing, ran %v and hooks %v", backend.sessions, backend.hooks)
	}

	view := ansi.Strip(m.View())
	for _, want := range []string{"no targets", "^e to edit the config", "^e config"} {
		if !strings.Contains(view, want) {
			t.Errorf("the empty dashboard should show %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "connect") {
		t.Errorf("with nothing selected, connect is not offered:\n%s", view)
	}
	if lines := strings.Split(m.View(), "\n"); len(lines) != 24 {
		t.Errorf("the empty dashboard should fill the terminal, got %d lines", len(lines))
	}
}

func TestEmptyDashboardSaysItIsDiscovering(t *testing.T) {
	m := startedWithProviders(t, "lima")
	if got := ansi.Strip(m.View()); !strings.Contains(got, "discovering") {
		t.Errorf("want a discovering pane, got:\n%s", got)
	}
	m = arrive(m, "lima")
	if got := ansi.Strip(m.View()); !strings.Contains(got, "no targets") {
		t.Errorf("once providers have answered, want no targets, got:\n%s", got)
	}
}

func TestSelectedRowFillsEveryCell(t *testing.T) {
	stylingEnabled(t)

	m := model{}
	it := named("kyoto")
	it.key = 'k'

	const width = 30
	filled := m.paneRow(it, true, true, width)
	if got := lipgloss.Width(filled); got != width {
		t.Fatalf("a filled row spans %d cells, want %d", got, width)
	}

	for _, run := range visibleRuns(filled) {
		if !run.styled {
			t.Errorf("an unstyled run breaks the fill: %q in %q", run.text, filled)
		}
	}

	plain := m.paneRow(it, false, true, width)
	if lipgloss.Width(plain) != width {
		t.Errorf("an unselected row still spans its width, got %d", lipgloss.Width(plain))
	}
	unstyled := 0
	for _, run := range visibleRuns(plain) {
		if !run.styled {
			unstyled++
		}
	}
	if unstyled == 0 {
		t.Errorf("only the selected row should be filled end to end: %q", plain)
	}
}

type textRun struct {
	text   string
	styled bool
}

func visibleRuns(line string) []textRun {
	var runs []textRun
	styled := false
	var current strings.Builder
	flush := func() {
		if current.Len() > 0 {
			runs = append(runs, textRun{current.String(), styled})
			current.Reset()
		}
	}
	for i := 0; i < len(line); i++ {
		if line[i] == 0x1b {
			j := strings.IndexByte(line[i:], 'm')
			flush()
			styled = !strings.HasSuffix(line[i:i+j+1], "[0m")
			i += j
			continue
		}
		current.WriteByte(line[i])
	}
	flush()
	return runs
}
