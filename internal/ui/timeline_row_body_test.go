package ui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/internal/store"
	"github.com/cruiserxyz/sextile/internal/ui/term"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// the specification (specs/timeline-row-body.md): a Timeline row leads the
// Bodies cursor to its body. The fixture's Bodies rows are Sun 0, Moon 1,
// Mars 2, Pluto 3, and its cursor starts on the Sun.

func shownRowWith(t *testing.T, m Model, frag string) int {
	t.Helper()
	return rowIndex(t, m.timelineShown(), frag, func(r lookup.TimelineRow) bool { return strings.Contains(r.Line, frag) })
}

// boldBodiesRows is the Bodies rows that name a body and carry bold.
func boldBodiesRows(t *testing.T, m Model) []string {
	t.Helper()
	bodies, _ := leftPanels(t, m.View())
	var rows []string
	for _, ln := range bodies {
		ln = trailingSGR.ReplaceAllString(ln, "")
		if !strings.Contains(ln, term.SGRBold) {
			continue
		}
		if txt := strings.TrimSpace(strings.Trim(ansi.Strip(ln), "│▏▕ ")); txt != "" {
			rows = append(rows, txt)
		}
	}
	return rows
}

func enterRowLeadsTo(t *testing.T, row string, want int) Model {
	t.Helper()
	freshModels(t)
	m := sameOrderFixture(t, Config{Color: ColorNever})
	if m.bodyCursorIdx != 0 {
		t.Fatalf("fixture: the Bodies cursor starts on %d", m.bodyCursorIdx)
	}
	m = enter(t, m, shownRowWith(t, m, row))
	if m.bodyCursorIdx != want {
		t.Errorf("Enter on %q: the Bodies cursor is on row %d, want %d", row, m.bodyCursorIdx, want)
	}
	return m
}

// 1. As today.
func TestIngressAndStationRowsLeadBodiesToTheMovingBody(t *testing.T) {
	m := enterRowLeadsTo(t, "Moon (Sco) --> Sagittarius", 1)
	if got := boldBodiesRows(t, m); len(got) != 1 || !strings.Contains(got[0], "Moon") {
		t.Errorf("ingress: bold Bodies rows = %q, want the Moon row", got)
	}
	enterRowLeadsTo(t, "Pluto <Aqu> S/D", 3)
}

// 2.
func TestAspectRowLeadsBodiesToTheFasterBody(t *testing.T) {
	m := enterRowLeadsTo(t, "Moon (Sag) Tri (Leo) Mars", 1)
	if got := boldBodiesRows(t, m); len(got) != 1 || !strings.Contains(got[0], "Moon") {
		t.Errorf("bold Bodies rows = %q, want the Moon row", got)
	}
}

// 3. Astrolog printed the slower body first.
func TestAspectRowWithTheSlowerBodyFirstLeadsToTheFasterBody(t *testing.T) {
	enterRowLeadsTo(t, "Sun (Lib) Squ (Cap) Moon", 1)
	// Mars before Pluto in Astrolog's order stays Mars; the Sun leads Mars.
	enterRowLeadsTo(t, "Mars (Leo) Tri [Ari] Saturn", 2) // Saturn is not listed, Mars is
}

// 4. The transiting body, never the natal one.
func TestChartRowLeadsBodiesToTheTransitingBody(t *testing.T) {
	// Mars (row 2) transits natal Ascendant.
	enterRowLeadsTo(t, "Mars (Leo) Con natal (Leo) Ascendant", 2)
	// Sun (row 0) transits natal Venus: from a Bodies cursor already on Pluto
	// it must come back to the Sun, not stay and not go to a natal body.
	m := sameOrderFixture(t, Config{Color: ColorNever})
	m = m.walkBodyCursorTo(objPluto)
	m = enter(t, m, shownRowWith(t, m, "Opp natal (Ari) Venus"))
	if m.bodyCursorIdx != 0 {
		t.Errorf("Sun opposite natal Venus: the Bodies cursor is on row %d, want the Sun (0)", m.bodyCursorIdx)
	}
}

// 5. A body Bodies does not show: nothing moves, nothing is said.
func TestARowWhoseBodyBodiesDoesNotShowMovesNothing(t *testing.T) {
	m := sameOrderFixture(t, Config{Color: ColorNever})
	// Enter on the Moon's ingress first: the Chart cursor is then on now's
	// line and the Bodies cursor on the Moon, so the next Enter moves neither.
	m = enter(t, m, shownRowWith(t, m, "Moon (Sco) --> Sagittarius"))
	if m.bodyCursorIdx != 1 {
		t.Fatalf("fixture: the Bodies cursor is on %d, want the Moon (1)", m.bodyCursorIdx)
	}
	logs, notice := len(m.logLines), m.yank.notice
	m = enter(t, m, shownRowWith(t, m, "Mercury (Sco) Con natal [Sco] Uranus"))
	if m.bodyCursorIdx != 1 {
		t.Errorf("Mercury is not in Bodies: the cursor moved to %d, want it left on the Moon (1)", m.bodyCursorIdx)
	}
	for _, l := range m.logLines[min(logs, len(m.logLines)):] {
		if strings.Contains(l.text, "Bodies") || strings.Contains(l.text, "Mercury") {
			t.Errorf("a Log line about the Bodies cursor appeared: %q", l.text)
		}
	}
	if m.yank.notice != notice {
		t.Errorf("a notice appeared: %q", m.yank.notice)
	}
}

// 6. Only Enter leads Bodies; the arrows leave its cursor where it is
// (operator, 2026-11-01: "only pre-select the body when enter on it").
func TestArrowOnARowLeavesTheBodiesCursorAlone(t *testing.T) {
	for _, tc := range []struct {
		row  string
		step int
	}{
		{"Moon (Sag) Tri (Leo) Mars", +1}, // aspect
		{"Moon (Sag) --> Capricorn", -1},  // ingress
		{"Pluto <Aqu> S/D", +1},           // station
	} {
		m := sameOrderFixture(t, Config{Color: ColorNever})
		m = m.walkBodyCursorTo(objMars) // none of the rows' bodies is Mars
		if m.bodyCursorIdx != 2 {
			t.Fatalf("fixture: the Bodies cursor is on %d, want Mars (2)", m.bodyCursorIdx)
		}
		i := shownRowWith(t, m, tc.row)
		m.details.cursor = i - tc.step
		chart, floating := m.cursorIdx, m.slots.Floating().Time
		key := keyDown
		if tc.step < 0 {
			key = keyUp
		}
		m, cmd := tpress(t, m, key)
		m = pump(t, m, cmd)
		if m.details.cursor != i {
			t.Fatalf("%s: fixture: the row cursor is %d, want %d", tc.row, m.details.cursor, i)
		}
		if m.bodyCursorIdx != 2 {
			t.Errorf("%s: the arrow moved the Bodies cursor to row %d, want it left on 2", tc.row, m.bodyCursorIdx)
		}
		if m.cursorIdx != chart || !m.slots.Floating().Time.Equal(floating) {
			t.Errorf("%s: an arrow moved the Chart cursor or the floating moment", tc.row)
		}
	}
}

// 7. Bold only, or reversed bold with Bodies focused.
func TestLedBodiesRowIsReversedOnlyWhenBodiesIsFocused(t *testing.T) {
	m := enterRowLeadsTo(t, "Moon (Sag) Tri (Leo) Mars", 1)
	bodies, _ := leftPanels(t, m.View())
	for _, ln := range bodies {
		if strings.Contains(ln, term.SGRReverse) {
			t.Errorf("with Details focused a Bodies row is reversed: %q", ansi.Strip(ln))
		}
	}
	m, _ = tpress(t, m, keyEsc)
	m, _ = tpress(t, m, runes("2"))
	bodies, _ = leftPanels(t, m.View())
	found := false
	for _, ln := range bodies {
		if strings.Contains(ln, term.SGRReverse) && strings.Contains(ansi.Strip(ln), "Moon") {
			found = true
		}
	}
	if !found {
		t.Errorf("with Bodies focused the Moon row is not reversed:\n%s", strings.Join(bodies, "\n"))
	}
}

// 8. Chart cursor, selection and bases are those of an ingress Enter before.
func TestAspectRowEnterLeavesTheChartSideAlone(t *testing.T) {
	a := sameOrderFixture(t, Config{Color: ColorNever})
	b := sameOrderFixture(t, Config{Color: ColorNever})
	a = enter(t, a, shownRowWith(t, a, "Moon (Sag) Tri (Leo) Mars"))
	b.bodyCursorIdx = 0
	// The same Enter with the lead body removed from the row: only Bodies differs.
	i := shownRowWith(t, b, "Moon (Sag) Tri (Leo) Mars")
	b.timeline.rows[i].Object, b.timeline.rows[i].OtherObject = objNeptune, objNeptune
	b = enter(t, b, i)
	if a.cursorIdx != b.cursorIdx || a.slots.Numeral(lastSlot(a)) != b.slots.Numeral(lastSlot(b)) ||
		a.activeTabID(PanelAspects) != b.activeTabID(PanelAspects) || !a.slots.Floating().Time.Equal(b.slots.Floating().Time) {
		t.Errorf("Enter on an aspect row changed the Chart cursor, selection, tab or moment beyond the Bodies cursor")
	}
	if b.bodyCursorIdx != 0 || a.bodyCursorIdx != 1 {
		t.Errorf("control: Bodies cursors a=%d b=%d, want 1 and 0", a.bodyCursorIdx, b.bodyCursorIdx)
	}
}

// 9. An unresolved row, or none under the cursor, moves nothing.
func TestUnresolvedRowOrNoRowLeadsNowhere(t *testing.T) {
	m := sameOrderFixture(t, Config{Color: ColorNever})
	i := shownRowWith(t, m, "Moon (Sag) Tri (Leo) Mars")
	m.timeline.rows[i].Resolved = false
	m = enter(t, m, i)
	if m.bodyCursorIdx != 0 {
		t.Errorf("an unresolved row moved the Bodies cursor to %d", m.bodyCursorIdx)
	}
	m = sameOrderFixture(t, Config{Color: ColorNever})
	m.details.cursor = len(m.timelineShown()) + 5
	m, cmd := tpress(t, m, keyEnter)
	m = pump(t, m, cmd)
	if m.bodyCursorIdx != 0 {
		t.Errorf("no row under the cursor moved the Bodies cursor to %d", m.bodyCursorIdx)
	}
}

func lastSlot(m Model) *store.Slot { all := m.slots.All(); return all[len(all)-1] }

// The spec's second frame: Enter on an aspect row leaves the Moon the bold
// Bodies row.
func TestEnteredAspectRowBodiesFrame(t *testing.T) {
	m := sameOrderFixture(t, Config{Color: ColorNever})
	m = enter(t, m, shownRowWith(t, m, "Moon (Sag) Tri [Ari] Neptune"))
	view := m.View()
	assertFrameGeometry(t, view, m.width, 40)
	checkGolden(t, fmt.Sprintf("bodies-entered-aspect-cursor-%dx40-never", m.width), view)
}

// F8 for every object: a timeline row whose moving or transiting object is one
// with no glyph (Fortune here) leads the Bodies cursor to that object's
// row, and the bold follows, as it does for Mars.
func TestTimelineLeadsToFortune(t *testing.T) {
	m := sameOrderFixtureWith(t, Config{Color: ColorNever}, aspect.Position{Object: fixtureObject(t, 19), Longitude: 300})
	want := rowOfObject(t, m, 19)
	i := shownRowWith(t, m, "Moon (Sco) --> Sagittarius")
	m.timeline.rows[i].Object = fixtureObject(t, 19)
	m = enter(t, m, i)
	if m.bodyCursorIdx != want {
		t.Fatalf("Enter on a Fortune row: the Bodies cursor is on %d, want the Fortune row (%d)", m.bodyCursorIdx, want)
	}
	if got := boldBodiesRows(t, m); len(got) != 1 || !strings.Contains(got[0], "Fort") {
		t.Errorf("bold Bodies rows = %q, want the Fortune row", got)
	}
}

// An object the Bodies rows do not draw moves nothing, as a hidden body did.
func TestTimelineLeadToAnUndrawnObjectMovesNothing(t *testing.T) {
	m := sameOrderFixture(t, Config{Color: ColorNever})
	i := shownRowWith(t, m, "Moon (Sco) --> Sagittarius")
	m.timeline.rows[i].Object = fixtureObject(t, 12) // Ceres: not among the fixture's rows
	m = enter(t, m, i)
	if m.bodyCursorIdx != 0 {
		t.Errorf("an undrawn object moved the Bodies cursor to %d", m.bodyCursorIdx)
	}
}
