package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cruiserxyz/sextile/internal/recipe"
)

// specs/details-follows-its-tab.md, slice D2, C9 and C11: after every Update the
// reconciler starts what a pane is owed, so no entry point can leave a pane
// waiting for ever by forgetting to ask, and a failure is not retried in a loop.

// settleMsg is a message in which nothing happens: a resize to the size the
// screen already has. Delivering it is how a test lets the reconciler look.
func settleMsg(m Model) tea.Msg { return tea.WindowSizeMsg{Width: m.width, Height: m.height} }

// look delivers settleMsg and returns what the reconciler started.
func look(t *testing.T, m Model) (Model, tea.Cmd) {
	t.Helper()
	tm, cmd := m.Update(settleMsg(m))
	return tm.(Model), cmd
}

// bothPanesCurrent says Bodies and Aspects draw the frame they ask for.
func bothPanesCurrent(m Model) bool {
	return m.textPaneState(PanelBodies) == paneCurrent && m.textPaneState(PanelAspects) == paneCurrent
}

func TestReconcilerStartsWhatIsOwed(t *testing.T) {
	// C11. Space on the Chart changes what the cursor compares, and with it the
	// Details tab that lists the compared charts: the Timeline's key names them.
	// Nothing on the Space path asks for that search; the reconciler does, from
	// the Update that carried the key. (No report's key depends on the
	// comparison from the Chart panel: its report is the cursor chart's alone,
	// and the text frame keeps its own fenced request, TestSpaceOnTheChartFencesTheOldFrame.)
	t.Run("space_on_chart", func(t *testing.T) {
		stubTimeline(t, nil)
		// Wide enough for Details beside two charts' columns.
		m, stub := followFixture(t, Config{Accessible: true}, 156, 40)
		m = m.withDetailsTab(PanelChart, TabDetailsTimeline)
		m, cmd := pressOnly(t, m, keyUp) // Synthetic 1 joins the selection
		m = pump(t, m, cmd)
		m, cmd = pressOnly(t, m, keySpace)
		m = pump(t, m, cmd)
		m, cmd = pressOnly(t, m, keyDown) // back on Now, whose Timeline tab it is
		m = pump(t, m, cmd)
		if m.activeTabID(PanelDetails) != TabDetailsTimeline || m.timelinePaneState() != paneCurrent || !bothPanesCurrent(m) {
			t.Fatalf("fixture: tab %s, Timeline %v, Bodies %v", m.activeTabID(PanelDetails), m.timelinePaneState(), m.textPaneState(PanelBodies))
		}
		before, _ := m.timelineKeyFor()
		reports := stub.reports

		// The entry point asks for the text frame (its own fenced request) and
		// for nothing of Details: the search is not started and not even keyed.
		m, entryCmd := m.toggleFocusedSelection()
		after, _ := m.timelineKeyFor()
		if after.natals == before.natals {
			t.Fatal("fixture: Space did not change the charts the Timeline compares")
		}
		if entryCmd == nil {
			t.Error("Space on the Chart no longer asks for its text frame")
		}
		if m.timeline.key == after {
			t.Fatal("the Space path itself started the Timeline's search")
		}
		// The next message carries the reconciler's turn.
		m, cmd = look(t, m)
		if cmd == nil || m.timeline.key != after {
			t.Fatal("Space on the Chart started no Timeline search: Details waits for ever")
		}
		if m.timelinePaneState() == paneCurrent {
			t.Errorf("before the search lands the Timeline draws the old comparison bright")
		}
		m = pump(t, m, cmd, entryCmd)
		if m.timelinePaneState() != paneCurrent || m.timeline.content != after || !bothPanesCurrent(m) {
			t.Errorf("after: Timeline %v (content %+v), Bodies %v Aspects %v", m.timelinePaneState(), m.timeline.content, m.textPaneState(PanelBodies), m.textPaneState(PanelAspects))
		}
		if stub.reports != reports {
			t.Errorf("%d report spawns for a change that does not touch the Chart report", stub.reports-reports)
		}
		assertQuiet(t, m, stub, "after Space on the Chart")
	})

	// The text frame's request on the same path is fenced: Space on the Chart
	// while the previous comparison's frame is still being computed. That
	// frame's result arrives after the key and must not commit.
	t.Run("space_on_chart_fences_the_old_frame", func(t *testing.T) {
		m, _ := followFixture(t, Config{Accessible: true}, 156, 40)
		m, cmd := pressOnly(t, m, keyUp)
		m = pump(t, m, cmd)
		m, cmd = pressOnly(t, m, keySpace)
		m = pump(t, m, cmd)
		m, cmd = pressOnly(t, m, keyDown) // Now, with Synthetic 1 selected
		m = pump(t, m, cmd)
		settled := m.shown.key

		m, oldRun := pressOnly(t, m, keySpace) // Now joins: a comparison; its frame is out
		comparing := mustTextKey(t, m)
		if comparing == settled {
			t.Fatal("fixture: Space did not change the frame's key")
		}
		m, newRun := pressOnly(t, m, keySpace) // Now leaves again, before that frame is back
		if mustTextKey(t, m) == comparing {
			t.Fatal("fixture: the second Space did not change the key back")
		}
		// The old comparison's frame arrives now.
		committed := false
		m = pumpSeen(t, m, 400_000_000, func(m Model) { committed = committed || m.shown.key == comparing }, oldRun)
		if committed {
			t.Fatal("the previous comparison's frame committed after Space on the Chart: the request is not fenced")
		}
		m = pumpSeen(t, m, 400_000_000, func(m Model) { committed = committed || m.shown.key == comparing }, newRun)
		if committed || !bothPanesCurrent(m) {
			t.Errorf("after the last frame: Bodies %v Aspects %v, the old comparison's frame committed: %v", m.textPaneState(PanelBodies), m.textPaneState(PanelAspects), committed)
		}
	})

	// C11. A chooser's live preview recasts the panels behind it in the lens it
	// previews. Details follows that lens too (the operator's table: the same
	// subject in a newer lens stays faint until its report lands): the
	// reconciler asks for the report once the preview frame is there.
	t.Run("chooser_preview", func(t *testing.T) {
		m, stub := followFixture(t, Config{Accessible: true}, 132, 40)
		m.aspectRegistry = testRegistry()
		m.objectRegistry = testObjectRegistry()
		before, _ := m.detailsKeyFor()
		reports := stub.reports

		m, cmd := pressOnly(t, m, tea.KeyMsg{Type: tea.KeyCtrlA})
		m = pump(t, m, cmd)
		if m.startup != startupAspectOpen {
			t.Fatalf("fixture: the chooser did not open (%s)", m.startup)
		}
		m, cmd = pressOnly(t, m, keySpace) // a tick flips: the preview
		now, ok := m.detailsKeyFor()
		if !ok || now == before {
			t.Fatalf("fixture: the preview did not change the report's key (ok %v)", ok)
		}
		if m.detailsReportState() != paneUpdating {
			t.Errorf("Details behind the previewing chooser: %v, want the same subject in another lens (updating)", m.detailsReportState())
		}
		if cmd == nil {
			t.Fatal("the preview started nothing")
		}
		m = pump(t, m, cmd)
		if stub.reports == reports {
			t.Fatal("the report was never asked for in the previewed lens: Details stays faint for ever")
		}
		if m.detailsReportState() != paneCurrent || m.details.key.raw != now {
			t.Errorf("Details after the preview settled: %v, key %+v, want current for %+v", m.detailsReportState(), m.details.key.raw, now)
		}
		if !bothPanesCurrent(m) {
			t.Errorf("Bodies %v Aspects %v behind the chooser", m.textPaneState(PanelBodies), m.textPaneState(PanelAspects))
		}
		assertQuiet(t, m, stub, "behind the previewing chooser")
	})

	// An entry point that changed the key and forgot to ask: the Bodies cursor
	// moved by hand. The next Update asks for the Sun's report.
	t.Run("forgotten_report", func(t *testing.T) {
		m, stub := followFixture(t, Config{Accessible: true}, 132, 40)
		m.focused = PanelBodies // by hand: no entry point ran
		if m.detailsReportState() == paneCurrent {
			t.Fatal("fixture: the Chart report is the Sun's")
		}
		reports := stub.reports
		m, cmd := look(t, m)
		if cmd == nil {
			t.Fatal("the reconciler started nothing for a report nobody asked for")
		}
		m = pump(t, m, cmd)
		if stub.reports != reports+1 || m.detailsReportState() != paneCurrent || !strings.Contains(detailsPanelText(m), "SUBJECT body Sun floating") {
			t.Errorf("reports %d (want %d), Details %v:\n%s", stub.reports, reports+1, m.detailsReportState(), detailsPanelText(m))
		}
		assertQuiet(t, m, stub, "after the forgotten report")
	})

	// The same for a frame: the lens changed by hand, with no want registered.
	t.Run("forgotten_frame", func(t *testing.T) {
		m, stub := followFixture(t, Config{Accessible: true}, 132, 40)
		m.lens.Aspects = recipe.AspectPolicy{Explicit: true, Indices: []int{1, 2}}
		if want, _ := m.textKeyFor(); m.shown.key == want {
			t.Fatal("fixture: the lens change did not change the frame's key")
		}
		casts := stub.casts
		m, cmd := look(t, m)
		if cmd == nil {
			t.Fatal("the reconciler started no frame for a key nobody asked for")
		}
		m = pump(t, m, cmd)
		if stub.casts == casts || !bothPanesCurrent(m) {
			t.Errorf("casts %d (was %d), Bodies %v Aspects %v", stub.casts, casts, m.textPaneState(PanelBodies), m.textPaneState(PanelAspects))
		}
		assertQuiet(t, m, stub, "after the forgotten frame")
	})

	// A failure is not retried: the Sun's report fails, and no later message
	// asks again, however many arrive. The wanted key moving on and coming back
	// is a new question and runs once.
	t.Run("failure_is_not_retried", func(t *testing.T) {
		m, stub := followFixture(t, Config{Accessible: true}, 132, 40)
		stub.failBodies = true
		m, cmd := pressOnly(t, m, runes("2"))
		m = pump(t, m, cmd)
		if m.detailsReportState() != paneFailed {
			t.Fatalf("fixture: %v, want the Sun's report failed", m.detailsReportState())
		}
		reports := stub.reports
		for i := 0; i < 5; i++ {
			var c tea.Cmd
			m, c = look(t, m)
			if c != nil {
				t.Fatalf("message %d after the failure started something", i+1)
			}
		}
		if stub.reports != reports {
			t.Errorf("%d spawns after the failure, want none", stub.reports-reports)
		}
		// Away and back: the failure was the Sun's, and is left behind.
		stub.failBodies = false
		m, cmd = pressOnly(t, m, keyDown)
		m = pump(t, m, cmd)
		m, cmd = pressOnly(t, m, keyUp)
		m = pump(t, m, cmd)
		if m.detailsReportState() != paneCurrent {
			t.Errorf("the Sun's report after Astrolog recovered: %v, want current", m.detailsReportState())
		}
	})

	// One run per key: a second message while the report is scheduled or in
	// flight asks for nothing.
	t.Run("one_run_per_key", func(t *testing.T) {
		m, stub := followFixture(t, Config{Accessible: true}, 132, 40)
		m, cmd := pressOnly(t, m, runes("2"))
		if cmd == nil {
			t.Fatal("2 started nothing")
		}
		for i := 0; i < 3; i++ {
			var c tea.Cmd
			m, c = look(t, m)
			if c != nil {
				t.Errorf("message %d while the report is owed started a second run", i+1)
			}
		}
		reports := stub.reports
		m = pump(t, m, cmd)
		if stub.reports != reports+1 {
			t.Errorf("%d report spawns for one key, want 1", stub.reports-reports)
		}
	})

	// A Details tab that is not the active one is owed nothing; choosing it is
	// what owes it, and the report then catches up to the key it wants.
	t.Run("hidden_tab_owes_nothing", func(t *testing.T) {
		stubTimeline(t, nil)
		m, stub := followFixture(t, Config{Accessible: true}, 132, 40)
		m = m.withDetailsTab(PanelChart, TabDetailsTimeline)
		reports := stub.reports
		m, cmd := pressOnly(t, m, runes("d")) // the floating moment moves: the report's key with it
		m = pump(t, m, cmd)
		if stub.reports != reports {
			t.Errorf("%d report spawns while the Timeline is the active tab, want none", stub.reports-reports)
		}
		m, cmd = m.cycleTab(PanelDetails, +1) // back to the report
		m = pump(t, m, cmd)
		if stub.reports != reports+1 || m.detailsReportState() != paneCurrent {
			t.Errorf("report spawns %d (want %d), state %v after choosing the report tab", stub.reports, reports+1, m.detailsReportState())
		}
	})
}

// mustTextKey is the key the screen asks for, for a test that has no use for ok.
func mustTextKey(t *testing.T, m Model) textKey {
	t.Helper()
	k, ok := m.textKeyFor()
	if !ok {
		t.Fatal("no text key")
	}
	return k
}

// assertQuiet holds the settled Model to what C9 promises: nothing is owed, so
// nothing starts, and no run, timer or fence is left behind.
func assertQuiet(t *testing.T, m Model, stub *followStub, where string) {
	t.Helper()
	reports, casts := stub.reports, stub.casts
	m, cmd := look(t, m)
	if cmd != nil {
		t.Errorf("%s: the reconciler still starts something on a settled Model", where)
	}
	if stub.reports != reports || stub.casts != casts {
		t.Errorf("%s: a settled Model spawned", where)
	}
}
