package ui

import (
	"context"
	"slices"
	"strconv"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/internal/store"
	"github.com/cruiserxyz/sextile/internal/testenv/astrodir"
	"github.com/cruiserxyz/sextile/internal/ui/popup"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// chooser_ticks_live_test.go: the operator's finding of 2026-11-02 on a host
// whose Astrolog lists Chiron, the asteroids and the cusp points Fortune,
// Vertex and East Point by default and has no ephemeris file for Chiron and the
// asteroids. The default lens' first cast is
//
//	-Yn -R 11 -R 12 -R 13 -R 14 -R 15 -R 17 -R 18 -R 25 -R 28
//
// (the five flips off, then the lens' own four on). The chooser must tick what
// that call draws, and one tick must change one body.

// ticksHost is the operator's host: the reduced folder without the file Chiron
// and the asteroids need, whose astrolog.as lists 11-15 and 19-21 by default.
func ticksHost(t *testing.T) {
	t.Helper()
	astrodir.Reduced(t, []string{"seas_18.se1", "seas_12.se1"}, "-R 11 12 13 14 15 19 20 21")
}

// textArgv is the last argv the single-chart text surface spawned: the one with
// the object band and no search switch.
func textArgv(argvs [][]string) []string {
	for i := len(argvs) - 1; i >= 0; i-- {
		a := argvs[i]
		if slices.Contains(a, "-Yn") && slices.Contains(a, "-qj") &&
			!slices.Contains(a, "-dm") && !slices.Contains(a, "-t") && !slices.Contains(a, "-tr") {
			return a
		}
	}
	return nil
}

// drawnByArgv is the set of objects Astrolog draws for an argv: its exact list,
// or the host's default set with the flips applied.
func drawnByArgv(argv []string, hostDefault []int) map[int]bool {
	explicit, toggled := objectLists(argv)
	out := map[int]bool{}
	if slices.Contains(argv, "-R0") {
		for _, n := range explicit {
			out[n] = true
		}
		return out
	}
	for _, n := range hostDefault {
		out[n] = true
	}
	for _, n := range toggled {
		out[n] = !out[n]
	}
	for n, on := range out {
		if !on {
			delete(out, n)
		}
	}
	return out
}

// chooserLeftPanels cuts the left column of a screen into its Bodies and Aspects
// panels (the Details column beside them also names Fortune and Vertex).
func chooserLeftPanels(screen string) map[string]string {
	out := map[string]string{"bodies": "", "aspects": ""}
	into := ""
	for _, l := range strings.Split(screen, "\n") {
		switch {
		case strings.Contains(l, "[2] Bodies"):
			into = "bodies"
		case strings.Contains(l, "[3] [Aspects]"):
			into = "aspects"
		}
		if into == "" {
			continue
		}
		if i := strings.Index(l, "||"); i >= 0 {
			l = l[:i]
		}
		out[into] += l + "\n"
	}
	return out
}

// band is the part of an argv from the node switch on, the object band and what
// follows it: the neutralising prefix every call carries is noise in a failure.
func band(argv []string) []string {
	if i := slices.Index(argv, "-Yn"); i >= 0 {
		return argv[i:]
	}
	return argv
}

func sameSet(a, b map[int]bool) (only []int, only2 []int) {
	for n := range a {
		if !b[n] {
			only = append(only, n)
		}
	}
	for n := range b {
		if !a[n] {
			only2 = append(only2, n)
		}
	}
	slices.Sort(only)
	slices.Sort(only2)
	return
}

// ticksStart is the host after startup: the model, its recorded spawns and the
// index where the startup casts begin.
func ticksStart(t *testing.T) (Model, func() [][]string, int) {
	t.Helper()
	ticksHost(t)
	read := argvShim(t)
	mark := len(read())
	m, _ := liveScreens(t)
	reg, err := lookup.ObjectRegistry(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	m.objectRegistry = reg
	return m, read, mark
}

func chooserOf(t *testing.T, m Model) Model {
	t.Helper()
	next, _ := openBodySelector(m)
	cm := next.(Model)
	if cm.startup != startupBodyOpen {
		t.Fatalf("the chooser did not open (startup %q)", cm.startup)
	}
	return cm
}

// pressInChooser runs one key through the chooser and waits for what it starts.
func pressInChooser(t *testing.T, m Model, k tea.KeyMsg) Model {
	t.Helper()
	tm, cmd := applyBodyModalKey(m, k)
	return pumpPatient(t, tm.(Model), cmd)
}

func tickObject(t *testing.T, m Model, obj int) Model {
	t.Helper()
	i := slices.IndexFunc(m.bodyModal.Matches, func(o aspect.RegistryObject) bool { return o.Number == obj })
	if i < 0 {
		t.Fatalf("object %d is not in the chooser", obj)
	}
	m.bodyModal.Cursor = popup.GroupRows() + i
	return pressInChooser(t, m, tea.KeyMsg{Type: tea.KeySpace})
}

func TestChooserTickFollowsTheCall(t *testing.T) {
	m, read, mark := ticksStart(t)
	argv := textArgv(read()[mark:])
	if argv == nil {
		t.Fatal("fixture: no text-surface cast recorded")
	}
	if explicit, toggled := objectLists(argv); len(explicit) != 0 || !slices.Equal(toggled, []int{11, 12, 13, 14, 15, 17, 18, 25, 28}) {
		t.Fatalf("fixture: the call is not the operator's (explicit %v, flips %v): %v", explicit, toggled, band(argv))
	}
	drawn := drawnByArgv(argv, m.ephemeris.Default)
	for _, n := range []int{19, 20, 21} {
		if !drawn[n] {
			t.Fatalf("fixture: the call does not draw object %d", n)
		}
	}

	cm := chooserOf(t, m)
	for _, o := range cm.bodyModal.Registry {
		if got := cm.bodyModal.Selected[o.Number]; got != drawn[o.Number] {
			t.Errorf("%s (%d) is ticked=%v but the call draws it=%v", o.Name, o.Number, got, drawn[o.Number])
		}
	}
	if _, bad := cm.bodyModal.Undrawable(11); !bad || cm.bodyModal.Selected[11] {
		t.Errorf("Chiron must be unticked and marked (marked=%v ticked=%v)", bad, cm.bodyModal.Selected[11])
	}
	// Chiron has no file, so it is listed after the other asteroids: the window
	// goes to it.
	i := slices.IndexFunc(cm.bodyModal.Matches, func(o aspect.RegistryObject) bool { return o.Number == 11 })
	cm.bodyModal.Cursor = popup.GroupRows() + i
	cm.bodyModal.ScrollTo(i, popup.ListHeight(60))
	if line := rowLine(t, renderBodyModal(cm.bodyModal, plainCfg, 132, 60), "Chiron"); !strings.Contains(ansi.Strip(line), "[ ] ! Chiron") {
		t.Errorf("Chiron's row reads %q, want an empty box and the mark", ansi.Strip(line))
	}
}

func TestATickChangesOneBody(t *testing.T) {
	m, read, mark := ticksStart(t)
	s0 := drawnByArgv(textArgv(read()[mark:]), m.ephemeris.Default)

	// Untick Mars, close the chooser.
	cm := tickObject(t, chooserOf(t, m), 5)
	if cm.bodyModal.Selected[5] {
		t.Fatal("Mars is still ticked after the Space")
	}
	mark = len(read())
	cm = pressInChooser(t, cm, tea.KeyMsg{Type: tea.KeyEnter})
	m1 := cm
	argv1 := textArgv(read()[mark:])
	if argv1 == nil {
		argv1 = textArgv(read())
	}
	s1 := drawnByArgv(argv1, m1.ephemeris.Default)
	if gone, added := sameSet(s0, s1); !slices.Equal(gone, []int{5}) || len(added) != 0 {
		t.Errorf("unticking Mars changed %v away and %v in, want Mars alone away:\n%v", gone, added, band(argv1))
	}
	for _, n := range []int{19, 20, 21} {
		if !s1[n] {
			t.Errorf("object %d is no longer asked for after unticking Mars: %v", n, band(argv1))
		}
	}
	left := chooserLeftPanels(ansi.Strip(m1.View()))
	for _, word := range []string{"· Fort", "· Vert", "· East"} {
		if !strings.Contains(left["bodies"], word) {
			t.Errorf("the Bodies panel no longer shows %q after unticking Mars:\n%s", word, left["bodies"])
		}
	}
	// The cusp points carry the glyph "·" in the Aspects panel.
	if !strings.Contains(left["aspects"], "·") {
		t.Errorf("the Aspects panel no longer lists an aspect of a cusp point after unticking Mars:\n%s", left["aspects"])
	}

	// Tick Mars back: the first list again.
	cm = tickObject(t, chooserOf(t, m1), 5)
	mark = len(read())
	cm = pressInChooser(t, cm, tea.KeyMsg{Type: tea.KeyEnter})
	argv2 := textArgv(read()[mark:])
	if argv2 == nil {
		argv2 = textArgv(read())
	}
	if gone, added := sameSet(s0, drawnByArgv(argv2, cm.ephemeris.Default)); len(gone)+len(added) != 0 {
		t.Errorf("ticking Mars back left %v missing and %v extra against the first call:\n%v", gone, added, band(argv2))
	}
}

// The set you left is the set you get: Mars unticked before the quit is
// missing after the restart, and the chooser shows it unticked.
func TestRestartKeepsTheSameDrawnSet(t *testing.T) {
	sessionSandbox(t)
	m, read, mark := ticksStart(t)
	s0 := drawnByArgv(textArgv(read()[mark:]), m.ephemeris.Default)
	cm := tickObject(t, chooserOf(t, m), 5)
	cm = pressInChooser(t, cm, tea.KeyMsg{Type: tea.KeyEnter})

	// Quit through the model; start again from the file it wrote.
	if _, cmd := cm.quit(); cmd == nil {
		t.Fatal("q did not quit")
	}
	snap, ok, err := store.LoadSession()
	if err != nil || !ok {
		t.Fatalf("LoadSession: ok %v err %v", ok, err)
	}
	var tm tea.Model = newTestModel(t, Config{Accessible: true}, FrozenClock(sessionFrozen))
	tm, _ = tm.Update(tea.WindowSizeMsg{Width: 144, Height: 40})
	tm, _ = applyEphemerisProbed(tm.(Model), EphemerisProbedMsg{Availability: cm.ephemeris})
	{
		x := tm.(Model)
		x.objectRegistry = m.objectRegistry // the object list is in before the session loads
		tm = x
	}
	tm, _ = tm.Update(AtlasLoadedMsg{Index: location.NewIndex([]location.PlaceInfo{berlinP, fixtureOneP})})
	mark = len(read())
	p := perimeter()
	p.Session, p.HasSession, p.FolderID = snap, true, snap.Folder
	tm, cmd := tm.Update(p)
	m2 := pumpPatient(t, tm.(Model), cmd)
	argv := textArgv(read()[mark:])
	if argv == nil {
		t.Fatalf("the restarted session cast nothing; log:\n%s", strings.Join(logTexts(m2), "\n"))
	}
	want := map[int]bool{}
	for n := range s0 {
		if n != 5 {
			want[n] = true
		}
	}
	if gone, added := sameSet(want, drawnByArgv(argv, m2.ephemeris.Default)); len(gone)+len(added) != 0 {
		t.Errorf("after the restart %v are missing and %v extra against the first start without Mars:\n%v", gone, added, band(argv))
	}
	c2 := chooserOf(t, m2)
	if c2.bodyModal.Selected[5] {
		t.Error("after the restart the chooser ticks Mars")
	}
	if _, bad := c2.bodyModal.Undrawable(11); !bad || c2.bodyModal.Selected[11] {
		t.Errorf("after the restart Chiron is ticked=%v marked=%v, want unticked and marked", c2.bodyModal.Selected[11], bad)
	}
	for _, n := range []int{19, 20, 21} {
		if !c2.bodyModal.Selected[n] {
			t.Errorf("after the restart object %d is unticked", n)
		}
	}
}

// A selection of only an absent body draws the baseline, says so once, and
// never asks Astrolog for nothing.
func TestOnlyDrainSaid(t *testing.T) {
	m, read, _ := ticksStart(t)
	idx := len(m.slots.All()) - 1
	m.slots.ToggleObjectSelection(idx, objChiron)
	mark := len(read())
	m, cmd := m.noteWant(surfaceText, true)
	m = pumpPatient(t, m, cmd)
	argv := textArgv(read()[mark:])
	if argv == nil {
		t.Fatalf("no text-surface cast; log:\n%s", strings.Join(logTexts(m), "\n"))
	}
	for i, a := range argv {
		if a != "-R0" {
			continue
		}
		if i+1 == len(argv) {
			t.Errorf("a bare -R0 went out: %v", band(argv))
		} else if _, err := strconv.Atoi(argv[i+1]); err != nil {
			t.Errorf("a bare -R0 went out: %v", band(argv))
		}
	}
	if n := countLines(m, "Couldn't cast the chart"); n != 0 {
		t.Errorf("the selection of an undrawable body failed the cast:\n%s", strings.Join(logTexts(m), "\n"))
	}
	const line = "Chiron is selected but can't be drawn here, so all drawn bodies are shown"
	if n := countLines(m, line); n != 1 {
		t.Errorf("the Log carries %q %d times, want once:\n%s", line, n, strings.Join(logTexts(m), "\n"))
	}
}

// A mark on an object the chart does not draw (it has its file, the host's
// astrolog.as just does not list it) must not narrow the chart to nothing: the
// operator's Now chart rendered empty, with "couldn't cast the chart", when the
// host's astrolog.as was replaced by the stock one. The chart is cast with its
// full set and the Log says, once, that the mark is not drawn and all drawn
// bodies are shown. Names come from the object registry the screen already reads
// (m.objectRegistry, Astrolog's own).
func TestStaleMarkLogsAndDrawsChart(t *testing.T) {
	m, read, mark := ticksStart(t)
	first := textArgv(read()[mark:])
	if first == nil {
		t.Fatal("fixture: no text-surface cast recorded")
	}
	drawn := drawnByArgv(first, m.ephemeris.Default)
	var stale aspect.RegistryObject
	for _, o := range m.objectRegistry {
		// Not Earth (row zero of the registry): a geocentric chart never draws it.
		if o.Number > 0 && !m.ephemeris.IsAbsent(o.Number) && !drawn[o.Number] && !strings.Contains(o.Name, ",") {
			stale = o
			break
		}
	}
	if stale.Name == "" {
		t.Fatalf("fixture: every registered object is drawn or absent (drawn %v)", drawn)
	}

	idx := len(m.slots.All()) - 1
	m.slots.ToggleObjectSelection(idx, stale.Object)
	mark = len(read())
	m, cmd := m.noteWant(surfaceText, true)
	m = pumpPatient(t, m, cmd)
	argv := textArgv(read()[mark:])
	if argv == nil {
		t.Fatalf("no text-surface cast; log:\n%s", strings.Join(logTexts(m), "\n"))
	}
	for i, a := range argv {
		if a != "-R0" {
			continue
		}
		if i+1 == len(argv) {
			t.Errorf("a bare -R0 went out: %v", band(argv))
		} else if _, err := strconv.Atoi(argv[i+1]); err != nil {
			t.Errorf("a bare -R0 went out: %v", band(argv))
		}
	}
	if only, only2 := sameSet(drawn, drawnByArgv(argv, m.ephemeris.Default)); len(only) != 0 || len(only2) != 0 {
		t.Errorf("the marked chart draws a different set: without %v, extra %v (argv %v)", only, only2, band(argv))
	}
	if n := countLines(m, "Couldn't cast the chart"); n != 0 {
		t.Errorf("a mark on an undrawn object failed the cast:\n%s", strings.Join(logTexts(m), "\n"))
	}
	line := stale.Name + " is selected but not drawn, so all drawn bodies are shown"
	if n := countLines(m, line); n != 1 {
		t.Errorf("the Log carries %q %d times, want once:\n%s", line, n, strings.Join(logTexts(m), "\n"))
	}
	// Said once per change: the next cast of the same chart does not say it again.
	m, cmd = m.noteWant(surfaceText, true)
	m = pumpPatient(t, m, cmd)
	if n := countLines(m, line); n != 1 {
		t.Errorf("after a second cast the Log carries the line %d times, want once", n)
	}
}
