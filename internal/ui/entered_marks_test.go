package ui

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/internal/astrologstyle"
	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/internal/store"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// the specification — after Enter, the entered row's exact
// aspect is bold in Aspects and, for an ingress or station, its moving body in
// Bodies, for as long as the entered row holds.

// markFixture is enteredFixture whose text compute answers a floating chart
// with the bodies and own aspects the sky rows below name, and whose cross
// compute answers chart ① and chart ② each a list that holds their transit
// row's aspect beside a decoy. cfg is the screen's.
func markFixture(t *testing.T, cfg Config) (Model, *[]textCall) {
	t.Helper()
	m, text := enteredFixture(t, 2, true, cfg)
	inner := firstCallTransits
	firstCallTransits = func(ctx context.Context, p *lookup.Probers, when time.Time, place location.PlaceInfo, lens lookup.Lens) (lookup.Result, error) {
		res, err := inner(ctx, p, when, place, lens)
		res.Positions = []aspect.Position{
			{Body: aspect.Sun, Longitude: 200}, {Body: aspect.Moon, Longitude: 244.25}, {Body: aspect.Mars, Longitude: 130},
			{Body: aspect.Pluto, Longitude: 301},
		}
		own := func(a, b aspect.Body, k aspect.AspectKind, orb aspect.Orb) aspect.Aspect {
			return aspect.Aspect{A: a, B: b, Kind: k, Orb: orb, Power: 10 - float64(orb)}
		}
		res.Aspects = []aspect.Aspect{
			own(aspect.Sun, aspect.Moon, aspect.Square, 0.5), own(aspect.Sun, aspect.Moon, aspect.Sextile, 1),
			own(aspect.Moon, aspect.Mars, aspect.Trine, 0), own(aspect.Moon, aspect.Mars, aspect.Opposition, 2),
			own(aspect.Moon, aspect.Mars, aspect.Square, 3), own(aspect.Sun, aspect.Mars, aspect.Trine, 4),
		}
		res.AspectStyles = astrologstyle.Aspects{
			{First: aspect.Moon, Second: aspect.Mars, Kind: aspect.Trine}: {First: "\x1b[31m", Kind: "\x1b[32m", Second: "\x1b[34m"},
		}
		return res, err
	}
	innerNatal := firstCallNatalPositions
	firstCallNatalPositions = func(ctx context.Context, p *lookup.Probers, chart store.AnchorChart, lens lookup.Lens) (lookup.Result, error) {
		res, err := innerNatal(ctx, p, chart, lens)
		res.Positions = []aspect.Position{
			{Body: aspect.Moon, Longitude: 7}, {Body: aspect.Sun, Longitude: 54}, {Body: aspect.Mars, Longitude: 100}, {Body: aspect.Pluto, Longitude: 250},
		}
		return res, err
	}
	overlayCrossAspects = func(_ context.Context, _ lookup.Lens, natal store.AnchorChart, when time.Time, _ location.PlaceInfo) (lookup.OverlayAspects, error) {
		*text = append(*text, textCall{kind: "aspects", when: when})
		x := func(transit, natalBody aspect.Body, k aspect.AspectKind, orb aspect.Orb) lookup.OverlayAspect {
			return lookup.OverlayAspect{Transit: transit, Natal: natalBody, Kind: k, Orb: orb, Power: 10 - float64(orb)}
		}
		if natal.When.Equal(enteredSecondNatal) {
			return lookup.OverlayAspects{Aspects: []lookup.OverlayAspect{
				x(aspect.Sun, aspect.Venus, aspect.Opposition, 0), x(aspect.Venus, aspect.Sun, aspect.Opposition, 1), x(aspect.Sun, aspect.Venus, aspect.Square, 2),
			}}, nil
		}
		return lookup.OverlayAspects{Aspects: []lookup.OverlayAspect{
			x(aspect.Moon, aspect.Moon, aspect.Conjunction, 0), x(aspect.Mars, aspect.Moon, aspect.Square, 3), x(aspect.Moon, aspect.Mars, aspect.Conjunction, 4),
			// The same cell as chart ②'s transit row, in chart ①'s list: it is
			// bold only on the tab of the chart whose row was entered.
			x(aspect.Sun, aspect.Venus, aspect.Opposition, 0),
		}}, nil
	}
	return m, text
}

func timelineRowWith(t *testing.T, rows []lookup.TimelineRow, frag string) int {
	t.Helper()
	return rowIndex(t, rows, frag, func(r lookup.TimelineRow) bool { return strings.Contains(r.Line, frag) })
}

var boldSpanRe = regexp.MustCompile("\x1b\\[1m(.*?)\x1b\\[0m")

// leftPanels cuts a View's left column and splits out the body lines (the
// border rows left out) of Bodies and of Aspects, escapes kept.
// leftCells is the first width cells of a plain line: the left column alone,
// so the Details caption (which now names its subject, e.g. `[0] Aspects`)
// on the same terminal row is never mistaken for a left panel's title.
func leftCells(plain string, width int) string {
	r := []rune(plain)
	if len(r) > width {
		r = r[:width]
	}
	return string(r)
}

func leftPanels(t *testing.T, view string) (bodies, aspects []string) {
	t.Helper()
	lines := strings.Split(view, "\n")
	width := 0
	for i, r := range []rune(ansi.Strip(lines[0])) {
		if r == '╮' {
			width = i + 1
			break
		}
	}
	if width == 0 {
		t.Fatalf("no Chart panel corner in the first line %q", ansi.Strip(lines[0]))
	}
	var into *[]string
	for _, ln := range lines {
		plain := ansi.Strip(ln)
		switch {
		case strings.HasPrefix(plain, "╭") && strings.Contains(leftCells(plain, width), "[2] "):
			into = &bodies
			continue
		case strings.HasPrefix(plain, "╭") && strings.Contains(leftCells(plain, width), aspectsStripPrefix):
			into = &aspects
			continue
		case strings.HasPrefix(plain, "╰"):
			into = nil
		}
		if into != nil {
			*into = append(*into, ansi.Truncate(ln, width-1, ""))
		}
	}
	return bodies, aspects
}

// boldSpanTexts is the plain text of every bold span in the lines.
func boldSpanTexts(lines []string) []string {
	var out []string
	for _, ln := range lines {
		for _, m := range boldSpanRe.FindAllStringSubmatch(ln, -1) {
			out = append(out, strings.TrimSpace(ansi.Strip(m[1])))
		}
	}
	return out
}

// boldLines is the lines that carry any bold, plain.
func boldLines(lines []string) []string {
	var out []string
	for _, ln := range lines {
		if strings.Contains(ln, sgrBold) {
			out = append(out, strings.TrimSpace(ansi.Strip(ln)))
		}
	}
	return out
}

func glyphs(parts ...aspect.Body) string {
	var out []string
	for _, b := range parts {
		out = append(out, bodyGlyph(b))
	}
	return strings.Join(out, " ")
}

// Rule 3: the cell of the entered row is bold in Aspects, and no other. A
// transit row marks natal-left, transiting-right on its pair tab; a sky row
// marks its two bodies in either order on the `Aspects` tab; the decoys that
// share a body, a pair or an aspect with the row stay plain.
func TestEnteredAspectCellIsBold(t *testing.T) {
	cfg := Config{Color: ColorNever}
	// A transit row of chart ②: Sun opposite natal Venus. The cell reads
	// natal Venus first, the transiting Sun second.
	m, _ := markFixture(t, cfg)
	m = enter(t, m, timelineRowWith(t, m.timeline.rows, "Opp natal (Ari) Venus"))
	if got := activeAspectsTitle(m); got != "3 in 2" {
		t.Fatalf("fixture: the Aspects tab is %q, want 3 in 2", got)
	}
	_, aspects := leftPanels(t, m.View())
	got := boldSpanTexts(aspects)
	if len(got) != 1 || !strings.HasPrefix(got[0], bodyGlyph(aspect.Venus)+" "+aspectGlyph(aspect.Opposition)+" "+bodyGlyph(aspect.Sun)+" ") {
		t.Errorf("transit row: bold cells = %q, want exactly the natal-Venus-opposite-Sun cell\n%s", got, strings.Join(aspects, "\n"))
	}

	// A sky row: Moon trine Mars on the `Aspects` tab, bodies in either order.
	m, _ = markFixture(t, cfg)
	m = enter(t, m, timelineRowWith(t, m.timeline.rows, "Moon (Sag) Tri (Leo) Mars"))
	if got := activeAspectsTitle(m); got != "Aspects" {
		t.Fatalf("fixture: the Aspects tab is %q, want Aspects", got)
	}
	_, aspects = leftPanels(t, m.View())
	got = boldSpanTexts(aspects)
	if len(got) != 1 || !strings.HasPrefix(got[0], glyphs(aspect.Moon)+" "+aspectGlyph(aspect.Trine)+" "+glyphs(aspect.Mars)+" ") {
		t.Errorf("sky row: bold cells = %q, want exactly the Moon-trine-Mars cell\n%s", got, strings.Join(aspects, "\n"))
	}

	// A list without a matching cell has none: Sun trine Moon is not listed.
	m, _ = markFixture(t, cfg)
	m = enter(t, m, timelineRowWith(t, m.timeline.rows, "Moon (Sag) Tri [Ari] Saturn"))
	_, aspects = leftPanels(t, m.View())
	if got := boldSpanTexts(aspects); len(got) != 0 {
		t.Errorf("a row whose aspect the list does not hold: bold cells = %q, want none", got)
	}
}

// sameOrderFixture is markFixture with chart ①'s rows in the sky's order, as
// Astrolog lists them for every chart: the walk finds the body's row in the
// rows on screen, so the fixture must not give the two charts different
// orders. The Bodies cursor starts on row 0 (the Sun), the Moon is row 1.
func sameOrderFixture(t *testing.T, cfg Config) Model {
	t.Helper()
	m, _ := markFixture(t, cfg)
	old := firstCallNatalPositions
	t.Cleanup(func() { firstCallNatalPositions = old })
	inner := firstCallNatalPositions
	firstCallNatalPositions = func(ctx context.Context, p *lookup.Probers, chart store.AnchorChart, lens lookup.Lens) (lookup.Result, error) {
		res, err := inner(ctx, p, chart, lens)
		res.Positions = []aspect.Position{
			{Body: aspect.Sun, Longitude: 54}, {Body: aspect.Moon, Longitude: 7}, {Body: aspect.Mars, Longitude: 100}, {Body: aspect.Pluto, Longitude: 250},
		}
		return res, err
	}
	m, cmd := m.afterFocusChange()
	m = pump(t, m, cmd)
	if m.bodyCursorIdx != 0 {
		t.Fatalf("fixture: the Bodies cursor starts on row %d, want 0", m.bodyCursorIdx)
	}
	return m
}

// the specification: Enter on an ingress or a
// station walks now's Bodies row cursor to the moving body's row, in the frame
// of the key, under the hold; the panel then holds one bold row (the cursor's)
// and `2` lands on it. An aspect row, or a body the rows do not list, moves
// nothing. The fixture's Bodies cursor starts on row 0 (the Sun), and the
// anchored chart's rows are laid out as the sky's, so a cursor that never moved
// cannot pass for one that did.
func TestEnteredIngressMovesTheBodiesCursor(t *testing.T) {
	cfg := Config{Color: ColorNever}
	fixture := func() Model { return sameOrderFixture(t, cfg) }
	bodyBold := func(m Model) []string {
		t.Helper()
		bodies, _ := leftPanels(t, m.View())
		// leftPanels cuts the column but keeps the escape sequences that follow
		// the cut: the bold row of the Details panel beside a Bodies row would
		// make that row read as bold. Only the sequences before the last
		// visible cell belong to the column.
		for i, ln := range bodies {
			bodies[i] = trailingSGR.ReplaceAllString(ln, "")
		}
		var rows []string
		for _, ln := range boldLines(bodies) {
			// The scroll thumb is bold too: only lines that name a body count.
			if strings.TrimSpace(strings.Trim(ln, "│▏▕ ")) != "" {
				rows = append(rows, ln)
			}
		}
		return rows
	}
	for _, tc := range []struct {
		row, body string
		want      int
	}{
		{"Moon (Sco) --> Sagittarius", "Moon", 1},
		{"Pluto <Aqu> S/D", "Pluto", 3},
	} {
		m := fixture()
		m.details.cursor = timelineRowWith(t, m.timeline.rows, tc.row)
		held := m.shown.bodyCursor
		m1, cmd := tpress(t, m, keyEnter)
		// The frame of the key: the cursor has moved, the hold is untouched.
		if m1.bodyCursorIdx != tc.want {
			t.Errorf("%s: in the frame of the key the Bodies cursor is on row %d, want %d", tc.row, m1.bodyCursorIdx, tc.want)
		}
		if m1.holding() && m1.shown.bodyCursor != held {
			t.Errorf("%s: the held frame's Bodies row changed from %d to %d", tc.row, held, m1.shown.bodyCursor)
		}
		m2 := pump(t, m1, cmd)
		if m2.deriveScene().entered == nil {
			t.Fatalf("%s: fixture: the row is not entered", tc.row)
		}
		if m2.bodyCursorIdx != tc.want || m2.bodyRows[m2.focusedSlot()] != tc.want {
			t.Errorf("%s: Bodies cursor %d, remembered row %d, want both %d", tc.row, m2.bodyCursorIdx, m2.bodyRows[m2.focusedSlot()], tc.want)
		}
		if got := bodyBold(m2); len(got) != 1 || !strings.Contains(got[0], tc.body) {
			t.Errorf("%s: bold Bodies rows = %q, want exactly the %s row", tc.row, got, tc.body)
		}
		// `Esc`, `2`: Bodies is entered on that body.
		m2, _ = tpress(t, m2, keyEsc)
		m2, _ = tpress(t, m2, runes("2"))
		if m2.focused != PanelBodies || m2.bodyCursorIdx != tc.want {
			t.Errorf("%s: after Esc, 2 the focus is %v on row %d, want Bodies on row %d", tc.row, m2.focused, m2.bodyCursorIdx, tc.want)
		}
		// A later Up moves from there: it is a cursor, not a mark.
		m2, cmd = tpress(t, m2, keyUp)
		m2 = pump(t, m2, cmd)
		if m2.bodyCursorIdx != tc.want-1 {
			t.Errorf("%s: Up from the body's row lands on %d, want %d", tc.row, m2.bodyCursorIdx, tc.want-1)
		}
	}
	// A body the rows do not list: nothing moves.
	m := fixture()
	i := timelineRowWith(t, m.timeline.rows, "Moon (Sco) --> Sagittarius")
	m.timeline.rows[i].BodyID = aspect.Neptune
	m = enter(t, m, i)
	if m.bodyCursorIdx != 0 {
		t.Errorf("a body Bodies does not list moved the cursor to row %d", m.bodyCursorIdx)
	}
	// No move for an aspect row, sky or transit.
	for _, row := range []string{"Moon (Sag) Tri (Leo) Mars", "Opp natal (Ari) Venus"} {
		m := fixture()
		m = enter(t, m, timelineRowWith(t, m.timeline.rows, row))
		if m.bodyCursorIdx != 0 {
			t.Errorf("%s: an aspect row moved the Bodies cursor to row %d", row, m.bodyCursorIdx)
		}
	}
}

// Rule 2: a row whose names do not resolve marks nothing, anywhere, and logs
// nothing; the rest of the screen is the screen of the resolved row.
func TestUnresolvedRowMarksNothing(t *testing.T) {
	cfg := Config{Color: ColorNever}
	for _, frag := range []string{"Moon (Sag) Tri (Leo) Mars", "Opp natal (Ari) Venus", "Moon (Sco) --> Sagittarius"} {
		resolved := sameOrderFixture(t, cfg)
		resolved = enter(t, resolved, timelineRowWith(t, resolved.timeline.rows, frag))
		// An aspect row marks a cell; an ingress moves the Bodies cursor off row 0
		// (the specification).
		if resolved.enteredMark() == (enteredMark{}) && resolved.bodyCursorIdx == 0 {
			t.Fatalf("%s: control: the resolved row marks nothing, so the test proves nothing", frag)
		}
		m := sameOrderFixture(t, cfg)
		i := timelineRowWith(t, m.timeline.rows, frag)
		m.timeline.rows[i].Resolved = false
		logs := len(m.logLines)
		m = enter(t, m, i)
		if got := m.enteredMark(); got != (enteredMark{}) {
			t.Errorf("%s: an unresolved row marks %+v", frag, got)
		}
		if m.bodyCursorIdx != 0 {
			t.Errorf("%s: an unresolved row moved the Bodies cursor to row %d", frag, m.bodyCursorIdx)
		}
		bodies, aspects := leftPanels(t, m.View())
		cleared := m
		cleared.details.pressedSet = false
		clearedBodies, _ := leftPanels(t, cleared.View())
		if strings.Join(boldLines(bodies), "\n") != strings.Join(boldLines(clearedBodies), "\n") {
			t.Errorf("%s: an unresolved row changed the bold rows of Bodies: %q vs %q", frag, boldLines(bodies), boldLines(clearedBodies))
		}
		if got := boldSpanTexts(aspects); len(got) != 0 {
			t.Errorf("%s: an unresolved row made Aspects cells bold: %q", frag, got)
		}
		if len(m.logLines) != logs {
			t.Errorf("%s: an unresolved row wrote %d Log line(s): %+v", frag, len(m.logLines)-logs, m.logLines[logs:])
		}
	}
}

// Rule 5: a time step, a cursor move or a selection change ends the marks
// with the entered row; `]` leaves them on the row's own tab and `[` brings
// them back.
func TestEnteredMarksEndWithTheRow(t *testing.T) {
	cfg := Config{Color: ColorNever}
	fresh := func() Model {
		m, _ := markFixture(t, cfg)
		return enter(t, m, timelineRowWith(t, m.timeline.rows, "Opp natal (Ari) Venus"))
	}
	marks := func(m Model) int {
		_, aspects := leftPanels(t, m.View())
		return len(boldSpanTexts(aspects))
	}
	m := fresh()
	if marks(m) != 1 || m.enteredMark() == (enteredMark{}) {
		t.Fatalf("control: the entered row marks %d cells, want 1", marks(m))
	}
	// A time step. With the cursor on ② (C2 of the specification)
	// the step key moves the floating target from there (time-keys-everywhere
	// rule 1), so the commit ends the row on that path too.
	m, _ = tpress(t, m, keyEsc)
	m, cmd := tpress(t, m, runes("d"))
	m = pump(t, m, cmd)
	if marks(m) != 0 || m.enteredMark() != (enteredMark{}) {
		t.Errorf("a step key on the anchored cursor left %d marked cells, mark %+v", marks(m), m.enteredMark())
	}
	// And the path a step takes, called directly, from a fresh entered row.
	m = fresh()
	m, _ = tpress(t, m, keyEsc)
	m, cmd = m.setFloatingTarget(m.targetFloatingTime.Add(24 * time.Hour))
	m = pump(t, m, cmd)
	if marks(m) != 0 || m.enteredMark() != (enteredMark{}) {
		t.Errorf("a time step left %d marked cells, mark %+v", marks(m), m.enteredMark())
	}
	// A cursor arrival.
	m = fresh()
	m, _ = tpress(t, m, keyEsc)
	m, cmd = tpress(t, m, keyUp)
	m = pump(t, m, cmd)
	if marks(m) != 0 || m.enteredMark() != (enteredMark{}) {
		t.Errorf("a cursor arrival left %d marked cells, mark %+v", marks(m), m.enteredMark())
	}
	// A selection change.
	m = fresh()
	m, _ = tpress(t, m, keyEsc)
	m, cmd = tpress(t, m, keySpace)
	m = pump(t, m, cmd)
	if marks(m) != 0 || m.enteredMark() != (enteredMark{}) {
		t.Errorf("a selection change left %d marked cells, mark %+v", marks(m), m.enteredMark())
	}
	// `[` moves to the other pair tab of the cursor's chart (the cursor is on
	// now since Enter, read in ②: `3 in 1` is the other), whose list holds the
	// same cell (the transit answer below): the row is still entered, its cell
	// is not bold there; `]` returns to the row's own tab and the cell is bold
	// again.
	oldOver := overlayCrossAspects
	t.Cleanup(func() { overlayCrossAspects = oldOver })
	overlayCrossAspects = func(context.Context, lookup.Lens, store.AnchorChart, time.Time, location.PlaceInfo) (lookup.OverlayAspects, error) {
		return lookup.OverlayAspects{Aspects: []lookup.OverlayAspect{{Transit: aspect.Sun, Natal: aspect.Venus, Kind: aspect.Opposition, Orb: 0, Power: 10}}}, nil
	}
	m = fresh()
	m, _ = tpress(t, m, keyEsc)
	m, _ = tpress(t, m, runes("3"))
	m, cmd = tpress(t, m, runes("["))
	m = pump(t, m, cmd)
	if m.deriveScene().entered == nil || activeAspectsTitle(m) != "3 in 1" {
		t.Fatalf("fixture: after `[` the tab is %q, entered %v, want 3 in 1", activeAspectsTitle(m), m.deriveScene().entered != nil)
	}
	if _, aspects := leftPanels(t, m.View()); !strings.Contains(strings.Join(aspects, "\n"), aspectGlyph(aspect.Opposition)) {
		t.Fatalf("fixture: chart 1's list does not hold the row's cell:\n%s", strings.Join(aspects, "\n"))
	}
	if got := marks(m); got != 0 {
		t.Errorf("on another tab (%s) %d cells are bold, want none", activeAspectsTitle(m), got)
	}
	m, cmd = tpress(t, m, runes("]"))
	m = pump(t, m, cmd)
	if activeAspectsTitle(m) != "3 in 2" || marks(m) != 1 {
		t.Errorf("back on the row's own tab (%s) %d cells are bold, want 1 on 3 in 2", activeAspectsTitle(m), marks(m))
	}
}

// Rule 6: a phase or eclipse label rides on an aspect row and marks that
// aspect; a (v/c) tail changes nothing; a return row marks its body conjunct
// itself. Each marks exactly one cell.
func TestLabelledRowsMarkTheirAspect(t *testing.T) {
	cfg := Config{Color: ColorNever}
	for _, tc := range []struct {
		name, frag string
		want       string
		cross      bool
	}{
		{"a phase label and a void tail", "Sun (Lib) Squ (Cap) Moon (Half Moon) (v/c 9:27)", glyphs(aspect.Sun) + " " + aspectGlyph(aspect.Square) + " " + glyphs(aspect.Moon), false},
		{"an occultation label", "Moon (Aqu) Opp (Leo) Mars (Total Occultation)", glyphs(aspect.Moon) + " " + aspectGlyph(aspect.Opposition) + " " + glyphs(aspect.Mars), false},
		{"a void tail", "Sun (Lib) Sex (Sag) Moon (v/c 15:01)", glyphs(aspect.Sun) + " " + aspectGlyph(aspect.Sextile) + " " + glyphs(aspect.Moon), false},
		{"a return row", "Moon (Cap) Con natal (Cap) Moon (Lunar Return)", bodyGlyph(aspect.Moon) + " " + aspectGlyph(aspect.Conjunction) + " " + bodyGlyph(aspect.Moon), true},
	} {
		m, _ := markFixture(t, cfg)
		m = enter(t, m, timelineRowWith(t, m.timeline.rows, tc.frag))
		_, aspects := leftPanels(t, m.View())
		got := boldSpanTexts(aspects)
		if len(got) != 1 || !strings.HasPrefix(got[0], tc.want+" ") {
			t.Errorf("%s: bold cells = %q, want exactly %q\n%s", tc.name, got, tc.want, strings.Join(aspects, "\n"))
		}
		if tc.cross {
			if got := activeAspectsTitle(m); got != "3 in 1" {
				t.Errorf("%s: the Aspects tab is %q, want 3 in 1", tc.name, got)
			}
		}
	}
}

var sgrRe = regexp.MustCompile("\x1b\\[[0-9;]*m")

// Rule 7: with colours on, the marked cell keeps Astrolog's colour and gains
// bold and nothing else. Read at the renderer: take away the bold on and the
// resets that close it, and the marked list is the unmarked list, byte for
// byte; and the cell of a full screen is bold over Astrolog's own colours.
func TestEnteredBoldKeepsAstrologsColour(t *testing.T) {
	cfg := Config{Color: ColorAlways}
	moonMars := aspect.Aspect{A: aspect.Moon, B: aspect.Mars, Kind: aspect.Trine, Orb: 0, Power: 10}
	state := chartViewState{
		aspects: []aspect.Aspect{moonMars, {A: aspect.Sun, B: aspect.Moon, Kind: aspect.Square, Orb: 1, Power: 9}},
		aspectStyles: astrologstyle.Aspects{
			{First: aspect.Moon, Second: aspect.Mars, Kind: aspect.Trine}: {First: "\x1b[31m", Kind: "\x1b[32m", Second: "\x1b[34m"},
		},
		settled: true,
	}
	plain := renderAspectsBody(state, TabAspects, 64, 4, cfg)
	state.mark = enteredMark{tab: TabAspects, first: aspect.Mars, other: aspect.Moon, kind: aspect.Trine}
	marked := renderAspectsBody(state, TabAspects, 64, 4, cfg)
	if marked == plain {
		t.Fatal("the mark changed nothing")
	}
	colours := func(s string) string {
		return strings.Join(sgrRe.FindAllString(strings.ReplaceAll(strings.ReplaceAll(s, sgrBold, ""), sgrReset, ""), -1), "")
	}
	if got, want := colours(marked), colours(plain); got != want || !strings.Contains(want, "\x1b[31m") {
		t.Errorf("the marked cell's colour sequences are %q, want Astrolog's %q", got, want)
	}
	if ansi.Strip(marked) != ansi.Strip(plain) {
		t.Errorf("the mark changed the text: %q vs %q", ansi.Strip(marked), ansi.Strip(plain))
	}
	if n := strings.Count(marked, sgrBold) - strings.Count(plain, sgrBold); n < 1 {
		t.Errorf("the marked cell is not bold")
	}
	// The same on a full screen: the Aspects cell is bold over Astrolog's SGR.
	m, _ := markFixture(t, cfg)
	m = enter(t, m, timelineRowWith(t, m.timeline.rows, "Moon (Sag) Tri (Leo) Mars"))
	_, aspects := leftPanels(t, m.View())
	joined := strings.Join(aspects, "\n")
	if !strings.Contains(joined, sgrBold+"\x1b[31m") {
		t.Errorf("the screen's marked cell is not bold over Astrolog's red:\n%q", joined)
	}
}

// Gate 5: the marks are drawn from the scene and never on a held old frame:
// the key-press frame of Enter has no new bold, the result's frame has it.
func TestEnteredMarksNeverRideOnAHeldFrame(t *testing.T) {
	m, _ := markFixture(t, Config{Color: ColorNever})
	m, cmd := m.afterFocusChange()
	m = pump(t, m, cmd)
	if !m.shown.stamped {
		t.Fatal("fixture: no committed frame to hold")
	}
	m.details.cursor = timelineRowWith(t, m.timeline.rows, "Pluto <Aqu> S/D")
	m1, cmd := tpress(t, m, keyEnter)
	bodies, aspects := leftPanels(t, m1.View())
	cleared := m1
	cleared.details.pressedSet = false
	clearedBodies, _ := leftPanels(t, cleared.View())
	if !m1.holding() {
		t.Skip("the press did not hold a frame; nothing to prove")
	}
	if strings.Join(boldLines(bodies), "|") != strings.Join(boldLines(clearedBodies), "|") || len(boldSpanTexts(aspects)) != 0 {
		t.Errorf("the key-press frame carries the new marks: bodies %q aspects %q", boldLines(bodies), boldSpanTexts(aspects))
	}
	// The cursor has moved in the frame of the key (the specification
	// rule 4) yet the held Bodies frame still paints the row it was cast with.
	if strings.Contains(strings.Join(boldLines(bodies), "|"), "Pluto") {
		t.Errorf("the key-press frame already paints the new Bodies row: %q", boldLines(bodies))
	}
	m2 := pump(t, m1, cmd)
	bodies, _ = leftPanels(t, m2.View())
	if !strings.Contains(strings.Join(boldLines(bodies), "|"), "Pluto") {
		t.Errorf("the result's frame has its Bodies cursor off the Pluto row: %q", boldLines(bodies))
	}
}

// ---- frames (the specification) ----------------------

func TestEnteredAspectBoldFrame(t *testing.T) {
	m, _ := markFixture(t, Config{Color: ColorNever})
	m = enter(t, m, timelineRowWith(t, m.timeline.rows, "Opp natal (Ari) Venus"))
	view := m.View()
	assertFrameGeometry(t, view, m.width, 40)
	checkGolden(t, fmt.Sprintf("aspects-entered-aspect-bold-%dx40-never", m.width), view)
}

func TestEnteredIngressCursorFrame(t *testing.T) {
	m := sameOrderFixture(t, Config{Color: ColorNever})
	m = enter(t, m, timelineRowWith(t, m.timeline.rows, "Moon (Sco) --> Sagittarius"))
	view := m.View()
	assertFrameGeometry(t, view, m.width, 40)
	checkGolden(t, fmt.Sprintf("bodies-entered-ingress-cursor-%dx40-never", m.width), view)
}
