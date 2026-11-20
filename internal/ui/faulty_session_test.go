package ui

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/internal/testenv/astrodir"
)

// faultySession is the lines of the session file the operator recovered after
// the Now chart rendered empty (2026-11-04, Docker, the stock astrolog.as): one
// chart entry, and the drawn set holds neither Mercury nor Mars, while the Now
// chart is marked with exactly those two.
const faultySession = `version: 3
drawn: Sun,Moon,Venus,Jupiter,Saturn,Uranus,Neptune,Pluto,Chiron,North Node,South Node,Ascendant,Nadir,Descendant,Midheaven
charts:
  - id: fixture-one
    name: Constructed One
    when: "2001-02-03T04:05:00Z"
    place: Test Bay
    country: TS
    lat: 12.34
    lon: 56.78
    tz: Etc/UTC
    selected: true
    bodies: ""
floating:
  selected: true
  bodies: Mercury,Mars
`

// startFaulty starts the app on the stock Astrolog share (nothing removed,
// nothing added to its astrolog.as) from a session file with text, through the
// real startup: the file is read by the perimeter read, restored by the session
// path (applySessionSelections, restoreDrawn), and the first frame is cast. It
// returns the model, a reader of every argv Astrolog was spawned with, and the
// clock the model runs on. The cursor ends on Now, the floating slot.
func startFaulty(t *testing.T, text string) (Model, func() [][]string, *fakeClock) {
	t.Helper()
	sessionSandbox(t)
	seedPreferences(t)
	seedSession(t, hereText(text))
	astrodir.Reduced(t, nil)
	read := argvShim(t)
	lookup.ForgetEphemeris()
	t.Cleanup(resetEphemeris)
	av, err := lookup.ProbeEphemeris(context.Background())
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	reg, err := lookup.ObjectRegistry(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	clock := &fakeClock{t: sessionFrozen}
	var tm tea.Model = newTestModel(t, Config{Accessible: true}, clock)
	tm, _ = tm.Update(tea.WindowSizeMsg{Width: 144, Height: 40})
	tm, _ = applyEphemerisProbed(tm.(Model), EphemerisProbedMsg{Availability: av})
	mm := tm.(Model)
	mm.objectRegistry = reg
	tm, _ = mm.Update(AtlasLoadedMsg{Index: location.NewIndex([]location.PlaceInfo{berlinP, fixtureOneP})})
	tm, cmd := tm.Update(realPerimeter(t))
	m := pumpPatient(t, tm.(Model), cmd)
	if !m.focusedIsFloating() {
		t.Fatalf("fixture: the cursor is on slot %d of %d, not on Now", m.cursorIdx, len(m.slots.All()))
	}
	return m, read, clock
}

// bareLists are the argvs that ask Astrolog for no body at all: a -R0 or -RT0
// with no object number after it, which restricts every body.
func bareLists(argvs [][]string) []string {
	var out []string
	for _, argv := range argvs {
		for i, a := range argv {
			if a != "-R0" && a != "-RT0" {
				continue
			}
			if i+1 == len(argv) {
				out = append(out, strings.Join(band(argv), " "))
			} else if _, err := strconv.Atoi(argv[i+1]); err != nil {
				out = append(out, strings.Join(band(argv), " "))
			}
		}
	}
	return out
}

// The operator's session, through the real startup and restore path on the
// stock Astrolog, never asks Astrolog for no body. The trigger is not a minute
// tick or a time step: the restored session leaves the cursor on Now, and the
// first text cast of a marked chart (buildTextCmd, withSelectedAspects) casts
// the chart a second time under showLens, whose Only {Mercury, Mars} met the
// drawn set that holds neither, and went out as a bare -R0. The minute tick that
// follows runs the same cast again. (Name kept short: the test's temp folders
// are in Astrolog's ephemeris path, which it truncates past 255 characters.)
func TestFaultySessionNeverCastsBare(t *testing.T) {
	m, read, clock := startFaulty(t, faultySession)
	if got := m.slots.SelectedObjects(len(m.slots.All()) - 1); len(got) != 2 || got[0].Name != "Mercury" || got[1].Name != "Mars" {
		t.Fatalf("fixture: the marks on Now are %v, want Mercury and Mars", got)
	}
	check := func(step string, m Model) {
		t.Helper()
		if bare := bareLists(read()); len(bare) > 0 {
			t.Errorf("%s: Astrolog was asked for no body: %q", step, bare)
		}
		if n := countLines(m, " not cast,"); n != 0 {
			t.Errorf("%s: a cast failed:\n%s", step, strings.Join(logTexts(m), "\n"))
		}
	}
	check("startup", m)

	casts := len(read())
	clock.advanceTo(sessionFrozen.Add(time.Minute))
	m, cmd := deliverTick(m)
	m = pumpPatient(t, m, cmd)
	if len(read()) == casts {
		t.Fatal("fixture: the minute tick cast nothing")
	}
	check("minute tick", m)

	const line = "the selected bodies are not drawn, all are shown" // two names do not fit the Log, so the general form
	if n := countLines(m, line); n != 1 {
		t.Errorf("the Log carries %q %d times, want once:\n%s", line, n, strings.Join(logTexts(m), "\n"))
	}
}

// The Bodies table of that session reserves no mark column: Mercury and Mars
// have no row in it, so a column for them would be an unexplained gap. Its rows
// are the rows of the same table with no marks at all.
func TestStaleMarkReservesNoColumn(t *testing.T) {
	m, _, _ := startFaulty(t, faultySession)
	idx := len(m.slots.All()) - 1
	marked := bodiesPanel(m)
	if !strings.Contains(marked, "Sun") {
		t.Fatalf("fixture: the Bodies table has no rows:\n%s", marked)
	}
	for _, o := range m.slots.SelectedObjects(idx) {
		m.slots.ToggleObjectSelection(idx, o)
	}
	if m.slots.HasMarks(idx) {
		t.Fatal("fixture: the marks were not cleared")
	}
	if plain := bodiesPanel(m); marked != plain {
		t.Errorf("the table with marks that have no row differs from the one without marks:\n%s\n--- without marks ---\n%s", marked, plain)
	}
}
