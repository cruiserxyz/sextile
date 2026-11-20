package ui

import (
	"github.com/cruiserxyz/sextile/internal/ui/popup"
	"slices"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/internal/recipe"
	"github.com/cruiserxyz/sextile/internal/store"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// ephemeris_test.go: the absence of an ephemeris file is said once (design
// point 4) and the chooser marks the bodies from the same fact (point 1).

func absentSeas18() lookup.EphemerisAvailability {
	return lookup.EphemerisAvailability{
		Folder:  "folder-A",
		Absent:  map[int]string{12: "seas_18.se1", 13: "seas_18.se1", 14: "seas_18.se1", 43: "se00010s.se1"},
		Names:   map[int]string{12: "Ceres", 13: "Pallas", 14: "Juno", 43: "Hygiea"},
		Default: []int{1, 2, 12, 13, 14, 43},
	}
}

func countLines(m Model, sub string) int {
	n := 0
	for _, s := range logTexts(m) {
		if strings.Contains(s, sub) {
			n++
		}
	}
	return n
}

func TestStartupSaysWhichBodiesCannotBeDrawnInOneLine(t *testing.T) {
	m := timelineModel(t, Config{Accessible: true})
	m, cmd := applyEphemerisProbed(m, EphemerisProbedMsg{Availability: absentSeas18()})
	if cmd != nil {
		t.Fatal("a footer notice was started")
	}
	// Two files and four bodies: the count, not the names. In the large log only.
	const want = "4 bodies can't be drawn here. Press Ctrl+B to see which"
	if n := countConsole(m, "bodies: "+want); n != 1 {
		t.Errorf("the console carries %q %d times, want once\n%s", want, n, strings.Join(consoleLineTexts(m), "\n"))
	}
	if n := countLines(m, "can't be drawn"); n != 1 {
		t.Errorf("%d lines about bodies that can't be drawn, want one", n)
	}
	for _, l := range m.logPanelLines() {
		if strings.Contains(l, "can't be drawn") {
			t.Errorf("the Log panel says %q", l)
		}
	}
	if got := m.footerIndicator(); strings.Contains(got, "can't be drawn") {
		t.Errorf("footer = %q, want no notice", got)
	}

	// The same answer again says nothing; another data folder says it again.
	before := len(m.logLines)
	var again tea.Cmd
	m, again = applyEphemerisProbed(m, EphemerisProbedMsg{Availability: absentSeas18()})
	if len(m.logLines) != before {
		t.Errorf("the same account was logged again (%d lines)", len(m.logLines)-before)
	}
	if again != nil {
		t.Error("the same account started a footer notice")
	}
	other := absentSeas18()
	other.Folder = "folder-B"
	m, _ = applyEphemerisProbed(m, EphemerisProbedMsg{Availability: other})
	if len(m.logLines) != before+1 {
		t.Errorf("a changed data folder logged %d lines, want 1", len(m.logLines)-before)
	}
}

func TestStartupSaysNothingWhenEverythingIsPresent(t *testing.T) {
	m := timelineModel(t, Config{Accessible: true})
	before := len(m.logLines)
	av := lookup.EphemerisAvailability{Folder: "complete", Default: []int{1, 2, 12}}
	m, cmd := applyEphemerisProbed(m, EphemerisProbedMsg{Availability: av})
	if cmd != nil || len(m.logLines) != before || m.footerIndicator() != "" {
		t.Errorf("a complete folder said something: cmd %v, %d new lines, footer %q", cmd != nil, len(m.logLines)-before, m.footerIndicator())
	}
}

func TestStartupOnlyNamesBodiesTheViewWouldDraw(t *testing.T) {
	m := timelineModel(t, Config{Accessible: true})
	av := absentSeas18()
	av.Default = []int{1, 2}                       // the host does not list the asteroids
	av.Absent = map[int]string{52: "sepm9401.se1"} // a moon nobody ticked
	av.Names = map[int]string{52: "Phobos"}
	before := len(m.logLines)
	m, _ = applyEphemerisProbed(m, EphemerisProbedMsg{Availability: av})
	if len(m.logLines) != before {
		t.Errorf("an absent body that is not on screen was announced: %v", logTexts(m)[before:])
	}
}

func TestChooserMarksAbsentBodiesFromTheStartupAnswer(t *testing.T) {
	av := absentSeas18()
	reg := []aspect.RegistryObject{{Object: aspect.Object{Number: 12, Name: "Ceres"}}, {Object: aspect.Object{Number: 13, Name: "Pallas"}}, {Object: aspect.Object{Number: 14, Name: "Juno"}}}
	st := popup.Open(reg, recipe.ObjectPolicy{}, nil)
	st.SetAbsent(av.Absent, map[int]string{12: "SwissEph file 'seas_18.se1' not found in PATH '/x/'"})
	line, ok := popup.ProbeStatusFor(st, reg[0])
	if !ok || line != "Astrolog: SwissEph file 'seas_18.se1' not found in PATH '/x/'" {
		t.Errorf("chooser line for Ceres = %q, %v", line, ok)
	}
	if row := popup.Row(st, reg[0], false, plainCfg); !strings.Contains(row.Left, "! Ceres") || !row.Dim {
		t.Errorf("Ceres row = %+v, want it marked", row)
	}
}

func TestChooserCannotTickAnAbsentBody(t *testing.T) {
	reg := []aspect.RegistryObject{{Object: aspect.Object{Number: 12, Name: "Ceres"}}, {Object: aspect.Object{Number: 13, Name: "Pallas"}}}
	st := popup.Open(reg, recipe.ObjectPolicy{Baseline: recipe.Explicit, Selection: []int{}}, nil)
	st.Absent = map[int]string{12: "seas_18.se1"}
	st.ToggleAt(popup.GroupRows())     // Ceres
	st.ToggleAt(popup.GroupRows() + 1) // Pallas
	if st.Selected[12] {
		t.Error("Ceres has no ephemeris file and was ticked")
	}
	if !st.Selected[13] {
		t.Error("Pallas has its file and was not ticked")
	}
	// A group row ticks what can be ticked.
	st.Selected[13] = false
	for g := 0; g < popup.GroupRows(); g++ {
		st.ToggleGroup(g)
	}
	if st.Selected[12] {
		t.Error("a group toggle ticked Ceres")
	}
	// An object already ticked (an older session) can be unticked.
	st.Selected[12] = true
	st.ToggleAt(popup.GroupRows())
	if st.Selected[12] {
		t.Error("an absent body that was ticked could not be unticked")
	}
}

// A session restored after the probe's answer is in names a body this
// Astrolog cannot draw: it is said once across the restore, never twice and
// never lost. (A session file names the legacy eighteen only and can only
// narrow the view, so Ceres cannot be restored; Chiron, whose file the folder lacks,
// is the body a session can carry.)
func TestARestoredSelectionNamingAnAbsentBodyIsSaidOnce(t *testing.T) {
	av := lookup.EphemerisAvailability{
		Folder:  "folder-A",
		Absent:  map[int]string{11: "seas_18.se1"},
		Names:   map[int]string{11: "Chiron"},
		Default: []int{1, 2},
	}
	var tm tea.Model = newTestModel(t, Config{Accessible: true}, FrozenClock(sessionFrozen))
	tm, _ = applyEphemerisProbed(tm.(Model), EphemerisProbedMsg{Availability: av})
	snap := snapshotChart()
	p := withSession(perimeter(), v1(&snap, true, true, []string{"Sun", "Chiron"}, nil))
	tm, _ = tm.Update(AtlasLoadedMsg{Index: location.NewIndex([]location.PlaceInfo{berlinP, fixtureOneP})})
	tm, _ = tm.Update(p)
	m := tm.(Model)
	if got := markedObjects(m.slots, 0); !slices.Contains(got, objChiron) {
		t.Fatalf("fixture: the restored selection is %v", got)
	}
	if n := countLines(m, "Chiron can't be drawn here: no file seas_18.se1"); n != 1 {
		t.Errorf("the Log names Chiron %d times, want once:\n%s", n, strings.Join(logTexts(m), "\n"))
	}
	// And the probe landing after the restore says it too.
	var late tea.Model = newTestModel(t, Config{Accessible: true}, FrozenClock(sessionFrozen))
	late, _ = late.Update(AtlasLoadedMsg{Index: location.NewIndex([]location.PlaceInfo{berlinP, fixtureOneP})})
	late, _ = late.Update(p)
	lm, _ := applyEphemerisProbed(late.(Model), EphemerisProbedMsg{Availability: av})
	if n := countLines(lm, "Chiron can't be drawn here: no file seas_18.se1"); n != 1 {
		t.Errorf("a probe after the restore: the Log names Chiron %d times, want once", n)
	}
}

// An absent body Astrolog wrote nothing for says so in its own terms.
func TestChooserStatusForAnAbsentBodyWithNoFileNameSaysNoData(t *testing.T) {
	reg := []aspect.RegistryObject{{Object: aspect.Object{Number: 12, Name: "Ceres"}}}
	st := popup.Open(reg, recipe.ObjectPolicy{}, nil)
	st.Absent = map[int]string{12: ""}
	got, ok := popup.ProbeStatusFor(st, reg[0])
	if !ok || got != "Astrolog cannot place Ceres here and did not say why" {
		t.Errorf("status = %q (%v), want the no-words sentence", got, ok)
	}
}

// Restore path: once the session is in, saying the same account again (as the
// probe landing again would) writes no line. An account that changes A -> B -> A
// is said on each change: "once per account" means not twice in a row (recorded,
// round 2). No account starts a footer notice.
func TestSayingTheSameAccountAgainWritesNothing(t *testing.T) {
	m := timelineModel(t, Config{Accessible: true})
	m, first := applyEphemerisProbed(m, EphemerisProbedMsg{Availability: absentSeas18()})
	if first != nil {
		t.Fatal("the first account started a footer notice")
	}
	lines := len(m.logLines)
	m = m.sayEphemeris()
	if len(m.logLines) != lines {
		t.Errorf("a repeated account: %d new lines", len(m.logLines)-lines)
	}
	b := absentSeas18()
	delete(b.Absent, 43)
	m, _ = applyEphemerisProbed(m, EphemerisProbedMsg{Availability: b})
	mid := len(m.logLines)
	m, _ = applyEphemerisProbed(m, EphemerisProbedMsg{Availability: absentSeas18()})
	if len(m.logLines) == mid {
		t.Error("A -> B -> A: the returning account was not said again")
	}
}

// A body the startup probe knows is absent, or this opening's per-body probe
// found undrawable, is never ticked by Space on its row or by its group row;
// an already-ticked one may still be unticked. The absent row starts OFF.
func TestAnUndrawableBodyCannotBeTicked(t *testing.T) {
	reg := append(testObjectRegistry(), aspect.RegistryObject{Object: aspect.Object{Number: 11, Name: "Chiron"}})
	for _, c := range []struct {
		name  string
		setup func(*popup.Body)
	}{
		{"startup absent", func(b *popup.Body) { b.Absent = map[int]string{11: "seas_18.se1"} }},
		{"stored negative probe", func(b *popup.Body) {
			b.SetProbe(11, b.Inputs, lookup.BodyProbe{Drawable: false, MissingFiles: []string{"seas_12.se1"}})
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			st := popup.Open(reg, recipe.ObjectPolicy{Baseline: recipe.Explicit, Selection: []int{1}}, nil)
			c.setup(&st)
			if st.Selected[11] {
				t.Fatal("fixture: Chiron starts ticked")
			}
			row := -1
			for i, o := range st.Matches {
				if o.Number == 11 {
					row = popup.GroupRows() + i
				}
			}
			if row < 0 {
				t.Fatal("Chiron is not a row")
			}
			st.ToggleAt(row)
			if st.Selected[11] {
				t.Error("Space on the row ticked a body that cannot be drawn")
			}
			st.ToggleGroup(recipe.ObjectGroupOf(11))
			if st.Selected[11] {
				t.Error("the group row ticked a body that cannot be drawn")
			}
			// Ticked already (a saved selection): it may be unticked.
			st.Selected[11] = true
			st.ToggleAt(row)
			if st.Selected[11] {
				t.Error("an already-ticked body could not be unticked")
			}
		})
	}
}

// The date backstop names the date of ONE chart call. A comparison's names are
// joined from several calls, so no single date belongs to them and none is said.
func TestUnavailableBackstopOmitsTheDateWhenNamesWereUnioned(t *testing.T) {
	for _, c := range []struct {
		name     string
		union    bool
		wantDate bool
	}{{"single call", false, true}, {"unioned from several calls", true, false}} {
		t.Run(c.name, func(t *testing.T) {
			m := timelineModel(t, Config{Accessible: true})
			m.slots.SetFloatingTime(time.Date(1500, 3, 1, 9, 30, 0, 0, time.UTC))
			m = m.focusFloatingForTest()
			m, _ = commitTextFrame(m, FirstFrameReadyMsg{Unavailable: []string{"Eris"}, UnavailableUnion: c.union}, false)
			var got string
			for _, l := range logTexts(m) {
				if strings.Contains(l, "Eris can't be drawn: no ephemeris data") {
					got = l
				}
			}
			if got == "" {
				t.Fatalf("the backstop line is missing:\n%s", strings.Join(logTexts(m), "\n"))
			}
			if hasDate := strings.Contains(got, " for "); hasDate != c.wantDate {
				t.Errorf("line %q: has a date = %v, want %v", got, hasDate, c.wantDate)
			}
		})
	}
}

// The date is the moment the frame was cast for, not where Now stands when it
// lands: stepping fast, a frame cast for 44 BC can land while Now still reads
// a later year.
func TestUnavailableBackstopNamesTheCastMoment(t *testing.T) {
	m := timelineModel(t, Config{Accessible: true})
	m.slots.SetFloatingTime(time.Date(2025, 11, 15, 17, 0, 0, 0, time.UTC))
	m = m.focusFloatingForTest()
	cast := time.Date(-43, 11, 15, 17, 0, 0, 0, time.UTC)
	m, _ = commitTextFrame(m, FirstFrameReadyMsg{Unavailable: []string{"Eris"}, TransitTime: cast}, false)
	want := "bodies: Eris can't be drawn: no ephemeris data for " + slotDate(&store.Slot{Floating: true, Time: cast, Location: m.slots.Floating().Location})
	if !slices.Contains(logTexts(m), want) {
		t.Errorf("want %q in the console:\n%s", want, strings.Join(logTexts(m), "\n"))
	}
}

func (m Model) focusFloatingForTest() Model {
	m.cursorIdx = len(m.slots.All()) - 1
	return m
}
