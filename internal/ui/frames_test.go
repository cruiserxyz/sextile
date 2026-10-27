package ui

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/internal/recipe"
	"github.com/cruiserxyz/sextile/internal/store"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// S4a: the chart you are on frames the screen; the tab remembers who visits
// (operator 2026-10-22, council 2026-10-22). One fixture throughout: Anna ①,
// Ben ②, now ③ selected, and Carl ③ with now ④ where a row needs a fourth
// member. Titles are read with colours on, so they are the glyphs the operator
// sees.

// framesFixture is Anna ① Ben ② [Carl ③] and now selected, the cursor on
// cursor, colours on, the Chart panel focused. The tab has had no key pressed.
func framesFixture(t *testing.T, anchored, cursor int) Model {
	t.Helper()
	m := selectedSet(t, anchored, true, cursor)
	m.cfg = Config{Color: ColorAlways}
	m.focused = PanelChart
	return m
}

// bracket presses `[` or `]` on the Aspects panel and returns focus to the
// Chart panel, where the arrows walk the rows.
func bracket(t *testing.T, m Model, key string) Model {
	t.Helper()
	m.focused = PanelAspects
	m = send(t, m, runes(key))
	m.focused = PanelChart
	return m
}

// walk presses an arrow n times on the Chart panel.
func walk(t *testing.T, m Model, key tea.KeyMsg, n int) Model {
	t.Helper()
	m.focused = PanelChart
	for i := 0; i < n; i++ {
		m = send(t, m, key)
	}
	return m
}

func slotAt(m Model, i int) *store.Slot { return m.slots.All()[i] }

// wantTab asserts the active Aspects tab reads title.
func wantTab(t *testing.T, m Model, title, where string) {
	t.Helper()
	if got := activeAspectsTitle(m); got != title {
		t.Errorf("%s: the Aspects tab is %q, want %q", where, got, title)
	}
}

// A1: nothing but a chosen tab leaves the chart's own tab, on any row, and an
// arrival never chooses one.
func TestTabStartsAlone(t *testing.T) {
	for cursor := 0; cursor < 4; cursor++ {
		m := framesFixture(t, 3, cursor)
		sc := m.deriveScene()
		if sc.mode != sceneComparing || sc.pair != nil {
			t.Fatalf("cursor %d: mode %v pair %v, want comparing with no active pair", cursor, sc.mode, sc.pair)
		}
		wantTab(t, m, "Aspects", "start")
	}
	m := framesFixture(t, 3, 0)
	for i := 0; i < 3; i++ {
		m = walk(t, m, keyDown, 1)
		wantTab(t, m, "Aspects", "down")
		if m.activePairSlot() != nil {
			t.Fatalf("an arrival chose a pair")
		}
	}
	for i := 0; i < 3; i++ {
		m = walk(t, m, keyUp, 1)
		wantTab(t, m, "Aspects", "up")
	}
}

// A2, A3, A4, A12 and A14 pinned the retired "the chart you are on frames" for
// anchored rows; the operator inverted it on 2026-10-24 (the cursor is the
// chart you read, the tab picks the base). Their successors are
// TestTabPicksTheBase, TestLandingOnTheBaseSwaps, TestThreeSelectedKeepTheBase
// and TestNoFloatingBaseOnNatalRows (view_rule_test.go).

// A5: the frame is the active pair's left chart, else the cursor, while
// comparing; nil outside.
func TestFrameIsPairLeftElseCursor(t *testing.T) {
	sceneWorkspaces(t, func(t *testing.T, m Model, mask, cursor int) {
		for _, b := range pairChoices(m) {
			m.base = b
			sc := m.deriveScene()
			switch {
			case sc.mode != sceneComparing:
				if sc.frame != nil {
					t.Fatalf("mask %d cursor %d: frame outside a comparison", mask, cursor)
				}
			case sc.pair != nil:
				if sc.frame != sc.pair.left || sc.pair.right != sc.cursor {
					t.Fatalf("mask %d cursor %d: frame is not the pair's left chart, or the cursor not its right", mask, cursor)
				}
			default:
				if sc.frame != sc.cursor {
					t.Fatalf("mask %d cursor %d: with no pair the frame is not the cursor", mask, cursor)
				}
			}
		}
	})
}

// A6: `[` and `]` on an anchored row write the tab's left chart as the base:
// every other anchored chart, never now.
func TestBracketsWriteTheBase(t *testing.T) {
	m := framesFixture(t, 3, 0)
	for _, step := range []struct {
		key  string
		want int // list index of the base, -1 for none
	}{{"]", 1}, {"]", 2}, {"]", -1}, {"[", 2}, {"[", 1}} {
		m = bracket(t, m, step.key)
		switch {
		case step.want < 0 && m.base != nil:
			t.Fatalf("%s: base %v, want none (the chart's own tab)", step.key, m.base)
		case step.want >= 0 && m.base != slotAt(m, step.want):
			t.Fatalf("%s: base is not the slot at %d", step.key, step.want)
		}
	}
}

// A7: on the floating row each anchored chart is a base in turn.
func TestBracketsOnNowRowWriteTheBase(t *testing.T) {
	m := framesFixture(t, 3, 3)
	for _, step := range []struct {
		title string
		base  int
	}{
		{"④ in ①", 0},
		{"④ in ②", 1},
		{"④ in ③", 2},
		{"Aspects", -1},
	} {
		m = bracket(t, m, "]")
		wantTab(t, m, step.title, "bracket")
		if (step.base < 0) != (m.base == nil) || (step.base >= 0 && m.base != slotAt(m, step.base)) {
			t.Fatalf("%s: base %v, want slot %d", step.title, m.base, step.base)
		}
	}
}

// A8: an arrival that does not land on the base keeps it.
func TestArrivalKeepsTheBase(t *testing.T) {
	m := framesFixture(t, 3, 0)
	carl := slotAt(m, 2)
	m.base = carl
	for _, key := range []tea.KeyMsg{keyDown, keyUp, keyDown, keyUp} {
		m = walk(t, m, key, 1)
		if m.base != carl {
			t.Fatalf("an arrival on row %d changed the base", m.cursorIdx)
		}
	}
	wantTab(t, m, "① in ③", "back on Anna")
}

// A8b: ⌃k and ⌃j reorder slots; the base keeps its slot, only numerals and
// titles change.
func TestReorderKeepsTheBase(t *testing.T) {
	m := framesFixture(t, 3, 1) // the cursor on Ben
	m = bracket(t, m, "]")      // Ben read in Anna: Ben's tabs are 2 in 1, 2 in 3
	anna, ben, carl := slotAt(m, 0), slotAt(m, 1), slotAt(m, 2)
	if m.base != anna {
		t.Fatalf("fixture: base %v, want Anna", m.base)
	}
	wantTab(t, m, "② in ①", "before")
	m = send(t, m, keyCtrlK) // Ben becomes ①, Anna ②
	if m.base != anna {
		t.Fatalf("⌃k changed the base %v", m.base)
	}
	if got := m.slots.All(); got[0] != ben || got[1] != anna || got[2] != carl {
		t.Fatalf("fixture: ⌃k did not swap Ben and Anna")
	}
	wantTab(t, m, "① in ②", "after ⌃k: the same pair, new numerals")
	m = send(t, m, keyCtrlJ)
	if m.base != anna {
		t.Errorf("⌃j changed the base")
	}
	wantTab(t, m, "② in ①", "after ⌃j")
}

// A9: deselecting or removing the base clears it. The cursor stands on the
// base only when it arrived from a moment (the own tab shows).
func TestDeselectClearsTheBase(t *testing.T) {
	m := framesFixture(t, 3, 3)
	carl := slotAt(m, 2)
	m.base = carl
	m = walk(t, m, keyUp, 1) // from now onto Carl, the base: the own tab
	if m.base != carl {
		t.Fatalf("fixture: arriving from now changed the base to %v", m.base)
	}
	m = send(t, m, keySpace) // deselect Carl, the base, under the cursor
	if m.base != nil {
		t.Errorf("the deselected base is still stored")
	}
	m = send(t, m, keySpace) // select him again: he must not come back as the base
	if m.base != nil {
		t.Errorf("reselecting Carl brought the base back")
	}

	// Deselecting a chart that is not the base leaves the base alone.
	m = framesFixture(t, 3, 0)
	m.base = slotAt(m, 2)
	m = walk(t, m, keyDown, 1) // on Ben
	m = send(t, m, keySpace)   // deselect Ben
	if m.base != slotAt(m, 2) {
		t.Errorf("deselecting Ben, who is not the base, changed the base")
	}

	// Removing the base's chart clears it too.
	m = framesFixture(t, 3, 3)
	carl = slotAt(m, 2)
	m.base = carl
	m = walk(t, m, keyUp, 1)
	m = send(t, m, keyDelete)
	if m.base != nil {
		t.Errorf("the removed base is still stored")
	}
}

// A10: the synastry round trip, read off the strip.
func TestSynastryRoundTrip(t *testing.T) {
	m := framesFixture(t, 2, 0)
	m = bracket(t, m, "]")
	wantTab(t, m, "① in ②", "Anna ]")
	m = walk(t, m, keyDown, 1)
	wantTab(t, m, "② in ①", "Down to Ben: landing on the base swaps")
	m = walk(t, m, keyDown, 1)
	wantTab(t, m, "③ in ①", "Down to now: Anna stays the base")
	m = walk(t, m, keyUp, 1)
	wantTab(t, m, "② in ①", "Up to Ben")
	m = walk(t, m, keyUp, 1)
	wantTab(t, m, "① in ②", "Up to Anna: landing on the base swaps")
}

// A11: the round trip from now's row: the base carries onto the natal rows.
func TestNowRowRoundTrip(t *testing.T) {
	m := framesFixture(t, 2, 2)
	m = bracket(t, m, "]")
	wantTab(t, m, "③ in ①", "chosen on now")
	m = walk(t, m, keyUp, 1)
	wantTab(t, m, "② in ①", "Up to Ben: Anna stays the base")
	m = walk(t, m, keyDown, 1)
	wantTab(t, m, "③ in ①", "Down to now")
}

// A13: a pair tab's title is `<right numeral> in <left numeral>`: the cursor's
// chart in the base.
func TestPairTitleNamesTheBase(t *testing.T) {
	if got := pairTabTitle(1, 2, Config{Color: ColorAlways}); got != "② in ①" {
		t.Errorf("title %q, want ② in ①", got)
	}
	if got := pairTabTitle(1, 2, Config{Color: ColorNever}); got != "2 in 1" {
		t.Errorf("title %q, want 2 in 1", got)
	}
	m := framesFixture(t, 2, 1)
	if got := titlesOf(m.panelTabs(PanelAspects)); !reflect.DeepEqual(got, []string{"Aspects", "② in ①"}) {
		t.Errorf("tabs on Ben = %q", got)
	}
}

// A15: at 80x24 with six selected, the active tab's title is whole in the strip,
// for every tab, with colours on and off.
func TestActiveTabTitleNeverTruncates(t *testing.T) {
	for _, mode := range []Config{{Color: ColorAlways}, {Color: ColorNever}} {
		for tab := 1; tab <= 4; tab++ {
			m := selectedSet(t, 5, true, 0)
			m.cfg = mode
			m.width, m.height = 80, 24
			m.focused = PanelAspects
			for i := 0; i < tab; i++ {
				m = send(t, m, runes("]"))
			}
			title := activeAspectsTitle(m)
			want := numeralForColour(1, mode) + " in " + numeralForColour(tab+1, mode)
			if title != want {
				t.Fatalf("tab %d: title %q, want %q", tab, title, want)
			}
			line := ansi.Strip(aspectsTopBorder(t, m))
			if !strings.Contains(line, title) {
				t.Errorf("colours %v, tab %d: the strip %q does not hold the whole title %q", mode.Color, tab, line, title)
			}
			if mode.Color == ColorNever && !strings.Contains(line, "["+title+"]") {
				t.Errorf("tab %d: the strip %q does not mark %q active", tab, line, title)
			}
		}
	}
}

// A17: with an active pair the wheel's first chart, the scene's frame (the
// base), the title's last numeral and the overlay compute's frame are one
// slot, for every pair state of the fixture.
func TestFrameIdentity(t *testing.T) {
	for cursor := 0; cursor < 4; cursor++ {
		for base := 0; base < 3; base++ { // every anchored base
			if base == cursor {
				continue
			}
			m := framesFixture(t, 3, cursor)
			m.base = slotAt(m, base)
			sc := m.deriveScene()
			if sc.pair == nil {
				t.Fatalf("cursor %d base %d: no pair", cursor, base)
			}
			frame := sc.frame
			plan, ok := m.wheelPlan()
			if !ok || plan.mode != wheelPair || len(plan.rings) != 2 {
				t.Fatalf("cursor %d base %d: the wheel is %+v", cursor, base, plan)
			}
			if plan.inner != anchorOf(frame) || plan.rings[0].Place != frame.Location || (!frame.Floating && plan.rings[0].When != frame.Time) {
				t.Errorf("cursor %d base %d: the wheel's first chart is not the frame", cursor, base)
			}
			// The title ends in the frame's own numeral: "<cursor> in <base>".
			title := activeAspectsTitle(m)
			if want := " in " + numeralForColour(sc.numeral(frame), m.cfg); !strings.HasSuffix(title, want) {
				t.Errorf("cursor %d base %d: title %q does not end in %q", cursor, base, title, want)
			}
			ofFrame, _, opts := m.overlayInputs(sc, transitTimeFromWorkspace(m))
			if ofFrame != anchorOf(frame) {
				t.Errorf("cursor %d base %d: the overlay compute's frame is not the scene's", cursor, base)
			}
			var p overlayPlan
			for _, o := range opts {
				o(&p)
			}
			if p.crossLeft != anchorOf(frame) {
				t.Errorf("cursor %d base %d: the cross call's left chart is not the frame", cursor, base)
			}
			// The comparison lens carries the frame's own house system: with a
			// distinct system on every slot, the slot that owns it is the frame.
			systems := []recipe.HouseSystem{recipe.Placidus, recipe.Koch, recipe.Campanus, recipe.Regiomontanus}
			ml := withOwnHouses(m, systems...)
			owner := -1
			for i, hs := range systems {
				if ml.comparisonLens().Houses == hs {
					owner = i
				}
			}
			if owner < 0 || slotAt(ml, owner) != frame {
				t.Errorf("cursor %d base %d: the comparison lens carries the house system of slot %d, not the frame's", cursor, base, owner)
			}
		}
	}
}

// bodiesStubs makes every chart cast at distinct, repeatable longitudes with
// twelve cusps, so the Bodies panel can show a house in another chart's cusps.
func bodiesStubs(t *testing.T) {
	t.Helper()
	stubColumnLookups(t, nil)
	cuspsFor := func(shift float64) aspect.Cusps {
		c := fullChartCusps()
		for i := range c {
			c[i] = float64(int(c[i]+shift) % 360)
		}
		return c
	}
	firstCallNatalPositions = func(_ context.Context, _ *lookup.Probers, c store.AnchorChart, _ lookup.Lens) (lookup.Result, error) {
		return lookup.Result{Positions: shiftedChart(shiftOf(c.When)), Cusps: cuspsFor(shiftOf(c.When))}, nil
	}
	firstCallTransits = func(_ context.Context, _ *lookup.Probers, when time.Time, _ location.PlaceInfo, _ lookup.Lens) (lookup.Result, error) {
		return lookup.Result{Positions: shiftedChart(shiftOf(when) + 11), Cusps: cuspsFor(shiftOf(when) + 11)}, nil
	}
}

// bodiesFrame is Anna ①, Ben ②, now ③ with Anna the base and the cursor on
// row (on now: "③ in ①"), its frame committed, colours off, 132x40, Bodies
// focused.
func bodiesFrame(t *testing.T, row int) Model {
	t.Helper()
	m := selectedSet(t, 2, true, row)
	m.cfg = Config{Color: ColorNever}
	m.width, m.height = 132, 40
	m.focused = PanelChart
	m.base = slotAt(m, 0)
	m, cmd := m.afterComparisonChange()
	m = pump(t, m, cmd)
	m.focused = PanelBodies
	return m
}

// A18 and V2 (TestBodiesOverlayOnlyOnTheVisitorRow) are replaced by
// TestCursorIsTheChartYouRead, TestBoldIsTheCursorFaintIsTheBase and
// TestNowRowIsUnchanged (view_rule_test.go): since 2026-10-24 every row with a
// base reads the cursor's bodies in the base's houses, as now's row did.

// A19: Enter on a chart's timeline row, with the floating slot selected,
// walks to the floating slot with that chart as the base ("now in k": a moment
// is never a base, operator 2026-10-24); a sky row walks to now and clears it.
func TestEnterStoresTheTab(t *testing.T) {
	m, _ := enteredFixture(t, 2, true, Config{Color: ColorAlways})
	ben := slotAt(m, 1)
	m = enter(t, m, chartRowOf(t, m.timeline.rows, 2))
	if !m.focusedIsFloating() || m.base != ben {
		t.Fatalf("Enter on ②'s row: cursor %d base %v, want now and Ben", m.cursorIdx, m.base)
	}
	wantTab(t, m, "③ in ②", "after Enter on ②'s row")
	m = enter(t, m, skyAspectRow(t, m.timeline.rows))
	if !m.focusedIsFloating() || m.base != nil {
		t.Fatalf("Enter on a sky row: on now %v base %v, want now and none", m.focusedIsFloating(), m.base)
	}
	wantTab(t, m, "Aspects", "after Enter on a sky row")
}

// A19b: with the floating slot unselected, Enter on a sky row leaves the
// base and the selection and sets the floating target to the row's instant.
// The Timeline is there only with the cursor on the floating slot itself (a
// group without it has no tab), and that slot is the row's destination.
func TestEnterWithUnselectedFloating(t *testing.T) {
	m, _ := enteredFixtureUnselectedNow(t, 2, Config{Color: ColorAlways})
	selected := m.slots.SelectedCount()
	keep := slotAt(m, 1)
	m.base = keep
	i := skyAspectRow(t, m.timeline.rows)
	row := m.timeline.rows[i]
	m = enter(t, m, i)
	if m.base != keep {
		t.Errorf("Enter changed the base")
	}
	if m.slots.SelectedCount() != selected || m.slots.SlotSelected(len(m.slots.All())-1) {
		t.Errorf("Enter changed the selection")
	}
	if !m.focusedIsFloating() {
		t.Errorf("the cursor is at %d, want the floating slot (the sky row's destination)", m.cursorIdx)
	}
	if !m.targetFloatingTime.Equal(landedFor(row)) {
		t.Errorf("the floating target is %v, want the row's instant %v", m.targetFloatingTime, landedFor(row))
	}
}

// A8, the Enter half: a cursor walk made by Enter arrives only at its
// destination: the rows it passes never swap the base.
func TestEnterWalkArrivesOnlyAtItsDestination(t *testing.T) {
	m, _ := enteredFixture(t, 3, true, Config{Color: ColorAlways})
	i := chartRowOf(t, m.timeline.rows, 2) // ②'s row
	// From ①, with ② the base, the walk to now passes ② (the base) and ③.
	m.cursorIdx, m.at, m.base = 0, slotAt(m, 0), slotAt(m, 1)
	m = enter(t, m, i)
	if !m.focusedIsFloating() || m.at != slotAt(m, 3) || m.base != slotAt(m, 1) {
		t.Errorf("after Enter: cursor %d at %v base %v, want now, arrived on now, base Ben", m.cursorIdx, m.at, m.base)
	}
}

// A20: after Enter on a chart's transit row, one time step leaves the tab alone.
func TestEnterThenStepKeepsTransits(t *testing.T) {
	m, _ := enteredFixture(t, 2, true, Config{Color: ColorAlways})
	m = enter(t, m, chartRowOf(t, m.timeline.rows, 2))
	wantTab(t, m, "③ in ②", "after Enter")
	m.focused = PanelChart
	m, cmd := tpress(t, m, runes("d"))
	m = pump(t, m, cmd)
	wantTab(t, m, "③ in ②", "after one day step")
	if m.deriveScene().entered != nil {
		t.Error("the step should end the entered row")
	}
}

// A21: `,` then `b` changes nothing and logs nothing, on the main screen and in
// attached Details.
func TestLeaderBIsUnbound(t *testing.T) {
	for _, attached := range []bool{false, true} {
		var m Model
		if attached {
			m, _ = enteredFixture(t, 2, true, Config{Color: ColorAlways})
			if !m.details.attached {
				t.Fatal("fixture: Details is not attached")
			}
		} else {
			m = framesFixture(t, 2, 1)
			m = bracket(t, m, "]")
		}
		sigBefore, base, wheelBefore, tab := m.comparisonSig(), m.base, m.wheelKey, activeAspectsTitle(m)
		logBefore := len(m.logLines)
		m, _ = tpress(t, m, runes(m.leaderKey))
		if !m.leader.pending {
			t.Fatalf("attached=%v: the leader did not arm", attached)
		}
		m, cmd := tpress(t, m, runes("b"))
		if cmd != nil {
			t.Errorf("attached=%v: `,` `b` returned a command", attached)
		}
		if m.leader.pending {
			t.Errorf("attached=%v: `b` left the leader armed", attached)
		}
		if m.comparisonSig() != sigBefore || m.base != base || m.wheelKey != wheelBefore || activeAspectsTitle(m) != tab {
			t.Errorf("attached=%v: `,` `b` changed the frame, the pair or the wheel", attached)
		}
		for _, l := range m.logLines[logBefore:] {
			if strings.Contains(strings.ToLower(l.text), "base") {
				t.Errorf("attached=%v: a base flip was logged: %q", attached, l.text)
			}
		}
	}
}

// A21b: nothing on the key bar (idle, leader pending, Details attached), in the
// help popup or among the leader entries names a base, in any form: not
// "base: cursor", not "base: first", not a "base" key description.
func TestNoBaseSwitchText(t *testing.T) {
	baseWord := regexp.MustCompile(`(?i)\bbase\b`)
	for _, cfg := range []Config{{Color: ColorNever}, {Color: ColorAlways}, {Accessible: true, Color: ColorNever}} {
		main := framesFixture(t, 2, 1)
		main.cfg = cfg
		main.width, main.height = 132, 40
		main = bracket(t, main, "]")
		attached, _ := enteredFixture(t, 2, true, cfg)
		if !attached.details.attached {
			t.Fatal("fixture: Details is not attached")
		}
		pending, _ := tpress(t, main, runes(main.leaderKey))
		if !pending.leader.pending {
			t.Fatal("fixture: the leader did not arm")
		}
		attachedPending, _ := tpress(t, attached, runes(attached.leaderKey))
		if !attachedPending.leader.pending {
			t.Fatal("fixture: the leader did not arm in Details")
		}
		for name, m := range map[string]Model{"main": main, "main, leader pending": pending, "Details attached": attached, "Details attached, leader pending": attachedPending} {
			if m.width < 100 {
				m.width = 132
			}
			bar := ansi.Strip(renderKeyBar(m.width, m.keyBarEntries(), m.cfg, m.footerIndicator()))
			if baseWord.MatchString(bar) {
				t.Errorf("%s: the key bar says base: %q", name, bar)
			}
			m.helpOpen = true
			if v := ansi.Strip(m.View()); baseWord.MatchString(v) {
				t.Errorf("%s: the help popup says base", name)
			}
			if lines := helpLinesText(m); baseWord.MatchString(lines) {
				t.Errorf("%s: the help list names a base:\n%s", name, lines)
			}
			// Every leader entry's own description, in both maps.
			for _, lb := range append(append([]leaderBinding(nil), m.leaderBindings...), detailsLeaderMap()...) {
				if d := bindingDesc(m.keys, lb.ID); baseWord.MatchString(d) {
					t.Errorf("%s: leader entry %v is described %q", name, lb.Seq, d)
				}
			}
		}
	}
}

// helpLinesText is every help line, plain.
func helpLinesText(m Model) string {
	var b strings.Builder
	for _, l := range m.helpLines() {
		b.WriteString(ansi.Strip(l.text))
		b.WriteString("\n")
	}
	return b.String()
}

// A24: while a pair is active, moving the cursor between selected rows
// recomputes the text frame and asks an open wheel for the new pair's bi-wheel.
func TestMovingWithinTheSetReframes(t *testing.T) {
	wheels, _ := stubWheels(t)
	stubPairCompute(t)
	m := framesFixture(t, 3, 0)
	m = bracket(t, m, "]")
	m = bracket(t, m, "]") // Anna read in Carl
	m = openWheel(t, m)
	wheels.reset()
	before := m.refresh[surfaceText].wantID
	var cmd tea.Cmd
	m, cmd = tpress(t, m, keyDown) // onto Ben: (Carl, Ben)
	m = pump(t, m, cmd)
	if m.refresh[surfaceText].wantID == before {
		t.Error("the move asked for no new text frame")
	}
	if len(wheels.overlay) != 1 || wheels.total() != 1 {
		t.Fatalf("wheel calls %+v, want one bi-wheel", wheels)
	}
	if got := wheels.overlay[0]; got.chart != anchorOf(slotAt(m, 2)) || !got.when.Equal(slotAt(m, 1).Time) {
		t.Errorf("the bi-wheel is %+v, want Carl, the base, passed first and Ben second", got)
	}
}

// A24b: with no pair, moving between selected rows asks an open wheel for the new
// cursor's single wheel.
func TestMovingOnOwnTabsDrawsSingles(t *testing.T) {
	wheels, _ := stubWheels(t)
	stubPairCompute(t)
	m := framesFixture(t, 3, 0)
	m = openWheel(t, m)
	if len(wheels.single) != 1 {
		t.Fatalf("fixture: %d single renders on opening, want 1", len(wheels.single))
	}
	wheels.reset()
	var cmd tea.Cmd
	m, cmd = tpress(t, m, keyDown)
	m = pump(t, m, cmd)
	if len(wheels.single) != 1 || wheels.total() != 1 || !wheels.single[0].Equal(slotAt(m, 1).Time) {
		t.Errorf("wheel calls %+v, want one single wheel of Ben", wheels)
	}
}

// A25: the reference page describes comparison in one sentence naming the
// "X in Y" tab, and neither it nor the README names the retired base switch.
func TestReadmeComparisonSentence(t *testing.T) {
	ref, err := os.ReadFile("../../docs/REFERENCE.md")
	if err != nil {
		t.Fatal(err)
	}
	const sentence = "Comparison reads `X in Y`: the chart under the cursor (X) is the one you read, and the Aspects tab, with `[` and `]`, picks the chart it is read against (Y), whose houses and zodiac frame the comparison — now is never Y."
	if strings.Count(string(ref), sentence) != 1 {
		t.Errorf("the reference page does not carry the comparison sentence exactly once:\n%s", sentence)
	}
	for _, doc := range []string{"../../README.md", "../../docs/REFERENCE.md"} {
		b, err := os.ReadFile(doc)
		if err != nil {
			t.Fatal(err)
		}
		text := string(b)
		lower := strings.ToLower(text)
		for _, bad := range []string{"base switch", "base: cursor", "base follows"} {
			if strings.Contains(lower, bad) {
				t.Errorf("%s mentions %q", doc, bad)
			}
		}
		if strings.Contains(text, "`,` `b`") || strings.Contains(text, "`,` and `b`") {
			t.Errorf("%s mentions the retired `, b` key", doc)
		}
	}
}

// A26: the Aspects cross request and the Details cross request use the active
// pair's left chart (the base) as chart 1 and its right chart (the cursor's) as
// the other, which is what the "X in Y" title says.
func TestPairLabelMatchesAspectsAndDetails(t *testing.T) {
	stubRawReports(t)
	for cursor := 0; cursor < 4; cursor++ {
		for base := 0; base < 3; base++ { // every anchored base
			if base == cursor {
				continue
			}
			m := framesFixture(t, 3, cursor)
			m.base = slotAt(m, base)
			sc := m.deriveScene()
			pair := sc.pair
			if pair == nil {
				t.Fatalf("cursor %d base %d: no pair", cursor, base)
			}
			_, _, opts := m.overlayInputs(sc, transitTimeFromWorkspace(m))
			var p overlayPlan
			for _, o := range opts {
				o(&p)
			}
			if p.crossLeft != anchorOf(pair.left) {
				t.Errorf("cursor %d base %d: the Aspects cross call's chart 1 is not the pair's left chart", cursor, base)
			}
			if pair.right.Floating != p.synastry.IsZero() || (!pair.right.Floating && p.synastry != anchorOf(pair.right)) {
				t.Errorf("cursor %d base %d: the Aspects cross call's other chart is not the pair's right chart", cursor, base)
			}
			m.focused = PanelBodies
			m.chart.positions = []aspect.Position{{Body: aspect.Venus}}
			req, ok := m.detailsRequestFor()
			if !ok || !req.cross {
				t.Fatalf("cursor %d base %d: no Details cross request", cursor, base)
			}
			if req.natalChart != anchorOf(pair.left) || !req.when.Equal(pair.right.Time) || req.place != pair.right.Location || req.synastry == pair.right.Floating {
				t.Errorf("cursor %d base %d: the Details cross request is not (left, right)", cursor, base)
			}
			if !strings.HasSuffix(req.title, activeAspectsTitle(m)) {
				t.Errorf("cursor %d base %d: Details title %q does not carry the active tab %q", cursor, base, req.title, activeAspectsTitle(m))
			}
		}
	}
}

// A27: a restored session starts on the chart's own tab with no base, whatever
// the model held before the restore.
func TestRestoreStartsOnOwnTab(t *testing.T) {
	sessionSandbox(t)
	sess := v2([]store.SessionSlot{listEntry(1, true), listEntry(2, true)}, true)
	var tm tea.Model = NewModelWithClock(Config{Color: ColorAlways}, FrozenClock(sessionFrozen))
	m := tm.(Model)
	stale := &store.Slot{} // a slot of some earlier workspace
	m.base, m.at = stale, stale
	tm, _ = m.Update(AtlasLoadedMsg{Index: location.NewIndex([]location.PlaceInfo{berlinP, fixtureOneP})})
	tm, _ = tm.Update(withSession(perimeter(), sess))
	m = tm.(Model)
	if m.slots == nil || !m.slots.SlotSelected(0) || !m.slots.SlotSelected(1) {
		t.Fatalf("fixture: the session did not restore two selected charts")
	}
	if m.base != nil {
		t.Errorf("after the restore: base %v, want none", m.base)
	}
	if m.activePairSlot() != nil {
		t.Errorf("a restored session shows a pair before the first arrival")
	}
}

// A22, the applying half: a preferences.yaml whose frame: block still holds
// `base: cursor` beside `houses: koch` starts, applies koch, and says nothing
// about the base.
func TestPreferencesWithBaseStillApplyHouses(t *testing.T) {
	sessionSandbox(t)
	path := store.PreferencesPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("frame:\n  base: cursor\n  houses: koch\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := framesFixture(t, 2, 0).applyFrameConfig(realPerimeter(t).Frame)
	if got := m.ownHouses(slotAt(m, 0)); got.hs != recipe.Koch || got.src != houseDefault {
		t.Errorf("own houses = %+v, want the preferences default Koch", got)
	}
	for _, l := range m.logLines {
		if strings.Contains(strings.ToLower(l.text), "base") {
			t.Errorf("the retired key was mentioned: %q", l.text)
		}
	}
}

// TestStagedWheelMatchesTheFrame: the wheel request that is actually made, not
// only the plan, has the base as its inner chart and the cursor's chart as its
// outer one, for every cursor and base of the fixture, the floating row
// included.
func TestStagedWheelMatchesTheFrame(t *testing.T) {
	for cursor := 0; cursor < 4; cursor++ {
		for base := 0; base < 3; base++ {
			if base == cursor {
				continue
			}
			wheels, _ := stubWheels(t)
			stubPairCompute(t)
			m := framesFixture(t, 3, cursor)
			m.base = slotAt(m, base)
			sc := m.deriveScene()
			if sc.pair == nil {
				t.Fatalf("cursor %d base %d: no pair", cursor, base)
			}
			m = openWheel(t, m)
			if len(wheels.overlay) != 1 || wheels.total() != 1 {
				t.Fatalf("cursor %d base %d: wheel calls %+v, want one bi-wheel", cursor, base, wheels)
			}
			got := wheels.overlay[0]
			frame, outer := sc.pair.left, sc.pair.right
			wantWhen := outer.Time
			if outer.Floating {
				wantWhen = m.floatingImageTime()
			}
			if got.chart != anchorOf(frame) {
				t.Errorf("cursor %d base %d: staged chart 1 is %+v, want the base %+v", cursor, base, got.chart, anchorOf(frame))
			}
			if !got.when.Equal(wantWhen) || got.place != outer.Location {
				t.Errorf("cursor %d base %d: staged outer is %v at %+v, want the cursor's chart %v at %+v", cursor, base, got.when, got.place, wantWhen, outer.Location)
			}
		}
	}
}

// A city re-pick rebuilds every slot but is not a tab choice: the base goes
// over to the rebuilt slots.
func TestCityRepickKeepsTheBase(t *testing.T) {
	for _, tc := range []struct {
		name   string
		cursor int // the row the base is chosen on
		title  string
	}{{"now read in Anna", 2, "③ in ①"}, {"Ben read in Anna", 1, "② in ①"}} {
		m := framesFixture(t, 2, tc.cursor)
		m = bracket(t, m, "]")
		wantTab(t, m, tc.title, tc.name+" before")
		old := m.slots
		m = m.refreshSlotsForPlace(testPlaceAmsterdam())
		if m.slots == old {
			t.Fatal("fixture: the slots were not rebuilt")
		}
		if m.base != slotAt(m, 0) {
			t.Fatalf("%s: the base is %v, want the rebuilt Anna", tc.name, m.base)
		}
		// The cursor lands on now after a re-pick; walking to the row reads the same tab.
		m = walk(t, m, keyUp, 2-tc.cursor)
		wantTab(t, m, tc.title, tc.name+" after the re-pick, back on the row")
	}
}
