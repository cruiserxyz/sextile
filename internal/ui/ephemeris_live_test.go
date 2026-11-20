package ui

import (
	"context"
	"errors"
	"fmt"
	"github.com/cruiserxyz/sextile/internal/ui/popup"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/internal/astrolog"
	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/internal/recipe"
	"github.com/cruiserxyz/sextile/internal/store"
	"github.com/cruiserxyz/sextile/internal/testenv"
	"github.com/cruiserxyz/sextile/internal/testenv/astrodir"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// ephemeris_live_test.go is design point 5 of the 2026-11-02 order, against the
// real binary: the host's astrolog.as lists Ceres, Pallas and Juno (and the
// folder may lack the file they need). Whatever the folder holds, no panel may
// show an object at a position Astrolog never computed.

// Labels the objects of the two kinds of file print under, in the long form the
// Timeline and reports use and the short form the Bodies table uses.
var (
	mainAsteroidWords = []string{"Ceres", "Pallas", "Juno", "Chiron", "Cere ", "Pall ", "Chir"}
	numberedWords     = []string{"Hygiea", "Eris", "Hygi", "Eris "}
)

// hasAsteroid reports the asteroid labels a screen carries, apart from the
// account that says they cannot be drawn (the Log line and the footer notice).
func hasAsteroid(s string, words []string) []string {
	var kept []string
	for _, l := range strings.Split(s, "\n") {
		if !strings.Contains(l, "can't be drawn") {
			kept = append(kept, l)
		}
	}
	s = strings.Join(kept, "\n")
	var hit []string
	for _, w := range words {
		if strings.Contains(s, w) {
			hit = append(hit, w)
		}
	}
	return hit
}

// liveScreens runs the model's own paths for the text surface, the Details
// report and the Timeline against the real binary and returns the plain text of
// every panel an operator could look at, plus the model.
// resetEphemeris drops the cached probe answers and puts back the host default
// every test starts from (TestMain), where lookup.ForgetEphemeris alone leaves
// the empty availability, which the compiler refuses to draw on.
func resetEphemeris() {
	lookup.ForgetEphemeris()
	testenv.InstallHostDefault()
}

func liveScreens(t *testing.T, afterProbe ...func()) (Model, map[string]string) {
	t.Helper()
	if _, err := exec.LookPath("astrolog"); err != nil {
		t.Skip("astrolog not on PATH; skipping live-binary test")
	}
	t.Cleanup(resetEphemeris)
	lookup.ForgetEphemeris()

	m := timelineModel(t, Config{Accessible: true})
	av, err := lookup.ProbeEphemeris(context.Background())
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	m, _ = applyEphemerisProbed(m, EphemerisProbedMsg{Availability: av})
	for _, f := range afterProbe {
		f()
	}

	m, cmd := m.noteWant(surfaceText, true)
	m = pumpPatient(t, m, cmd)
	m, dcmd := m.requestDetails()
	m = pumpPatient(t, m, dcmd)
	m, tcmd := syncTimelineVisited(m)
	m = pumpPatient(t, m, tcmd)

	screens := map[string]string{}
	view := func(name string, m Model) { screens[name] = ansi.Strip(m.View()) }
	view("default", m)
	view("aspects", m.withDetailsTab(PanelAspects, TabAspects))
	view("report", m.withDetailsTab(PanelChart, TabDetailsReport))
	view("timeline", m.withDetailsTab(PanelChart, TabDetailsTimeline))
	return m, screens
}

func TestReducedFolderShowsNoBodyAtAFixedPosition(t *testing.T) {
	for _, c := range []struct {
		name    string
		without []string
		lines   []string // the host's astrolog.as lists these objects
		words   []string
		log     string // the one account line, also the footer notice
	}{
		{"main asteroids", []string{"seas_18.se1", "seas_12.se1"}, []string{"-R 12 13 14"}, mainAsteroidWords,
			"4 bodies can't be drawn here. Press Ctrl+B to see which."},
		{"numbered asteroid and dwarf", []string{"se00010s.se1", "s136199s.se1"}, []string{"-R 43 45"}, numberedWords,
			"2 bodies can't be drawn here. Press Ctrl+B to see which."},
	} {
		t.Run(c.name, func(t *testing.T) {
			astrodir.Reduced(t, c.without, c.lines...)
			m, screens := liveScreens(t)
			requirePanelsNonBlank(t, m)
			for name, text := range screens {
				if hit := hasAsteroid(text, c.words); len(hit) > 0 {
					t.Errorf("%s screen names %v, bodies Astrolog could not compute:\n%s", name, hit, text)
				}
			}
			for _, l := range m.logPanelLines() {
				if strings.Contains(l, "can't be drawn") {
					t.Errorf("the Log panel says %q, a body without data is left out quietly", l)
				}
			}
			if n := countConsole(m, "bodies: "+strings.TrimSuffix(c.log, ".")); n != 1 {
				t.Errorf("the console carries %q %d times, want once:\n%s", c.log, n, strings.Join(consoleLineTexts(m), "\n"))
			}
			if got := m.footerIndicator(); strings.Contains(got, "can't be drawn") {
				t.Errorf("footer = %q, want no notice", got)
			}
			if len(m.timeline.rows) == 0 {
				t.Error("the timeline has no rows (the search was refused, or the narrowing removed more than the absent bodies)")
			}
		})
	}
}

func TestCompleteFolderDrawsThemAndSaysNothing(t *testing.T) {
	astrodir.Reduced(t, nil, "-R 12 13 14 43 45")
	m, screens := liveScreens(t)
	requirePanelsNonBlank(t, m)
	for _, words := range [][]string{mainAsteroidWords, numberedWords} {
		if hit := hasAsteroid(screens["timeline"], words); len(hit) == 0 {
			t.Errorf("with every file present the timeline shows none of %v:\n%s", words, screens["timeline"])
		}
	}
	if got := m.footerIndicator(); got != "" {
		t.Errorf("footer = %q, want nothing said", got)
	}
	for _, s := range logTexts(m) {
		if strings.Contains(s, "can't be drawn") {
			t.Errorf("the Log says %q with every file present", s)
		}
	}
}

// A comparing Timeline whose old chart needs a file the folder lacks leaves
// that body out quietly and lists the rest: no line in the Log panel, and the
// large log holds Astrolog's own line and the command.
func TestComparingTimelineWithAnOldChartLeavesTheBodyOutAndListsTheRest(t *testing.T) {
	if _, err := exec.LookPath("astrolog"); err != nil {
		t.Skip("astrolog not on PATH; skipping live-binary test")
	}
	astrodir.Reduced(t, []string{"seas_12.se1"}, "-R 12 13 14")
	t.Cleanup(resetEphemeris)
	lookup.ForgetEphemeris()

	m := detailsTimelineModel(t, Config{Accessible: true})
	if err := addAnchor(m.slots, time.Date(1500, 3, 1, 9, 30, 0, 0, time.UTC), testPlaceAmsterdam()); err != nil {
		t.Fatal(err)
	}
	m.slots.SetSlotSelected(0, true)
	m.slots.SetSlotSelected(1, true)
	m.width = max(m.width, dualColumnMinWidth(m.shownCount()))
	m = m.noteArrival()
	m, cmd := syncTimelineShown(m)
	m = pumpPatient(t, m, cmd)

	if len(m.timeline.rows) == 0 || !m.timeline.complete {
		t.Errorf("the timeline committed %d rows (complete=%v), want the list without the body", len(m.timeline.rows), m.timeline.complete)
	}
	for _, l := range m.logPanelLines() {
		if strings.Contains(l, "not found") || strings.Contains(l, "Couldn't") {
			t.Errorf("the Log panel says %q", l)
		}
	}
	if n := countLines(m, "not found in PATH"); n == 0 {
		t.Errorf("the console carries no `not found` line:\n%s", strings.Join(logTexts(m), "\n"))
	}
	if n := countLines(m, "astrolog "); n == 0 {
		t.Errorf("the Log carries no command:\n%s", strings.Join(logTexts(m), "\n"))
	}
}

// argvShim puts an `astrolog` first on PATH that appends its argv to a log and
// runs the real one. It returns a function reading the log: one argv per spawn.
func argvShim(t *testing.T) func() [][]string {
	t.Helper()
	real, err := exec.LookPath("astrolog")
	if err != nil {
		t.Skip("astrolog not on PATH; skipping live-binary test")
	}
	dir := t.TempDir()
	log := filepath.Join(dir, "argv.log")
	script := "#!/bin/sh\n{ printf '%s\\n' \"$@\"; echo '@@'; } >> '" + log + "'\nexec '" + real + "' \"$@\"\n"
	if err := os.WriteFile(filepath.Join(dir, "astrolog"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return func() [][]string {
		b, err := os.ReadFile(log)
		if err != nil {
			return nil
		}
		var out [][]string
		var cur []string
		for _, l := range strings.Split(strings.TrimSuffix(string(b), "\n"), "\n") {
			if l == "@@" {
				out = append(out, cur)
				cur = nil
				continue
			}
			cur = append(cur, l)
		}
		return out
	}
}

// objectLists reads the object numbers an argv names after each -R0 / -RT0
// (an explicit list) and after each -R / -RT (a toggle), up to the next switch.
func objectLists(argv []string) (explicit, toggled []int) {
	for i := 0; i < len(argv); i++ {
		var into *[]int
		switch argv[i] {
		case "-R0", "-RT0":
			into = &explicit
		case "-R", "-RT":
			into = &toggled
		default:
			continue
		}
		for j := i + 1; j < len(argv); j++ {
			n, err := strconv.Atoi(argv[j])
			if err != nil {
				break
			}
			*into = append(*into, n)
		}
	}
	return
}

// Whatever the operator does after the probe, no argv enables an object whose
// file is missing: an explicit list never names one, and a toggle names one only
// to switch off an object the host's own astrolog.as turned on.
func TestNoArgvEnablesAbsent(t *testing.T) {
	astrodir.Reduced(t, []string{"seas_18.se1", "seas_12.se1"}, "-R 11 12 13 14", "-RT 11 12 13 14")
	read := argvShim(t)
	mark := 0
	m, _ := liveScreens(t, func() { mark = len(read()) })
	av := m.ephemeris
	for _, n := range []int{11, 12, 13, 14} {
		if !av.IsAbsent(n) {
			t.Fatalf("fixture: object %d is not absent (%v)", n, av.Absent)
		}
	}

	// What the UI drove up to here, classified by the switches that identify it:
	// the Timeline's sky, transit and return searches. Each side must be there,
	// and must carry its own object band.
	liveArgvs := read()[mark:]
	ctx := context.Background()
	lens := m.viewLens()
	natal := store.AnchorChart{When: tlNatalWhen, Place: testPlaceAmsterdam()}
	place := testPlaceAmsterdam()
	rings := []lookup.WheelRing{{When: tlNatalWhen, Place: place}, {When: tlFloatingTime, Place: place}, {When: tlFloatingTime.AddDate(0, 1, 0), Place: place}}
	type driven struct {
		class  string
		listed bool // every argv of the class must carry an object band
		wantRO bool // ... or an -RO target
		argvs  [][]string
	}
	var classes []driven
	has := func(argv []string, sw string) bool { return slices.Contains(argv, sw) }
	for _, c := range []struct {
		class string
		pick  func([]string) bool
	}{
		{"timeline sky void search (-dY)", func(a []string) bool { return has(a, "-dY") }},
		{"timeline transit side (-t)", func(a []string) bool { return has(a, "-t") }},
		{"timeline return side (-tr)", func(a []string) bool { return has(a, "-tr") }},
	} {
		d := driven{class: c.class, listed: true}
		for _, a := range liveArgvs {
			if c.pick(a) {
				d.argvs = append(d.argvs, a)
			}
		}
		classes = append(classes, d)
	}
	for _, f := range []struct {
		name   string
		listed bool
		wantRO bool
		run    func() error
	}{
		{"body report", false, true, func() error {
			_, _, err := lookup.ComputeRawReport(ctx, tlFloatingTime, place, lens, lookup.RawReportRequest{Kind: lookup.RawReportBody, Object: objMars})
			return err
		}},
		{"cross report", false, true, func() error {
			_, _, err := lookup.CrossBodyRawReport(ctx, lens, natal, tlFloatingTime, place, objMars)
			return err
		}},
		{"synastry cross report", false, true, func() error {
			_, _, err := lookup.SynastryCrossBodyRawReport(ctx, lens, natal, store.AnchorChart{When: tlFloatingTime, Place: place}, objMars)
			return err
		}},
		{"pair tab (-rt)", false, false, func() error {
			_, err := lookup.OverlayCrossAspects(ctx, lens, natal, tlFloatingTime, place)
			return err
		}},
		{"pair tab (-r0)", false, false, func() error {
			_, err := lookup.SynastryCrossAspects(ctx, lens, natal, store.AnchorChart{When: tlFloatingTime, Place: place})
			return err
		}},
		{"cycle search", false, false, func() error {
			_, err := lookup.SearchCycle(ctx, objMars, lookup.CycleSpanFor(tlFloatingTime, time.UTC), lens, astrolog.RunTimelineSearch)
			return err
		}},
		{"sun pair", false, false, func() error {
			_, err := lookup.SunPair(ctx, objMars, tlFloatingTime, place, lens, astrolog.RunTimelineSearch)
			return err
		}},
		{"wheel (single)", true, false, func() error {
			_, err := lookup.RenderWheelImage(ctx, store.AnchorChart{When: tlFloatingTime, Place: place}, lens, lookup.WheelHighlight{})
			return err
		}},
		{"wheel (overlay)", true, false, func() error {
			_, err := lookup.RenderOverlayWheelImage(ctx, natal, tlFloatingTime, place, lens)
			return err
		}},
		{"wheel (three rings)", true, false, func() error {
			_, err := lookup.RenderMultiWheelImage(ctx, rings, lens)
			return err
		}},
		{"cycle search of an absent body", false, false, func() error {
			_, err := lookup.SearchCycle(ctx, objChiron, lookup.CycleSpanFor(tlFloatingTime, time.UTC), lens, astrolog.RunTimelineSearch)
			if err == nil {
				return errors.New("a cycle search of Chiron without its file was not refused")
			}
			return nil
		}},
	} {
		before := len(read())
		if err := f.run(); err != nil {
			t.Errorf("%s: %v", f.name, err)
		}
		d := driven{class: f.name, listed: f.listed, wantRO: f.wantRO, argvs: read()[before:]}
		if f.name == "cycle search of an absent body" {
			if len(d.argvs) != 0 {
				t.Errorf("a cycle search of an absent body spawned %v", d.argvs)
			}
			continue
		}
		classes = append(classes, d)
	}

	// The real chooser, on a body the startup probe knows is absent: resting the
	// cursor on it starts no probe and so spawns nothing.
	reg, err := lookup.ObjectRegistry(ctx)
	if err != nil {
		t.Fatal(err)
	}
	m.objectRegistry = reg
	next, _ := openBodySelector(m)
	cm := next.(Model)
	for i, o := range cm.bodyModal.Matches {
		if o.Number == 11 {
			cm.bodyModal.Cursor = popup.GroupRows() + i
		}
	}
	if cur, ok := cm.bodyModal.ObjectUnderCursor(); !ok || cur.Number != 11 {
		t.Fatalf("fixture: the chooser cursor is on %v (%v), want Chiron", cur, ok)
	}
	beforeChooser := len(read())
	var restCmd tea.Cmd
	cm, _ = cm.armRest()
	cm, restCmd = applyBodyProbeRest(cm, bodyProbeRestMsg{seq: cm.bodyModal.Seq, restID: cm.bodyModal.RestID, obj: 11})
	if restCmd != nil || len(read()) != beforeChooser {
		t.Errorf("the chooser probed a body the startup probe knows is absent (cmd=%v, %d new spawns)", restCmd != nil, len(read())-beforeChooser)
	}

	hostDefault := map[int]bool{}
	for _, n := range av.Default {
		hostDefault[n] = true
	}
	inspected := 0
	for _, d := range classes {
		if len(d.argvs) == 0 {
			t.Errorf("%s: no spawn recorded, so it proves nothing", d.class)
			continue
		}
		for _, argv := range d.argvs {
			explicit, toggled := objectLists(argv)
			if d.listed && len(explicit)+len(toggled) == 0 {
				t.Errorf("%s: a spawn carries no object band at all: %v", d.class, argv)
			}
			ro := false
			for i, a := range argv {
				if a == "-RO" && i+1 < len(argv) {
					ro = true
					if n, err := strconv.Atoi(argv[i+1]); err == nil && av.IsAbsent(n) {
						t.Errorf("%s: -RO targets absent object %d: %v", d.class, n, argv)
					}
				}
			}
			if d.wantRO && !ro {
				t.Errorf("%s: no -RO target in %v", d.class, argv)
			}
			for _, n := range explicit {
				if av.IsAbsent(n) {
					t.Errorf("%s: an explicit object list names absent object %d (%s): %v", d.class, n, av.Names[n], argv)
				}
			}
			for _, n := range toggled {
				if av.IsAbsent(n) && !hostDefault[n] {
					t.Errorf("%s: a toggle would switch ON absent object %d (%s): %v", d.class, n, av.Names[n], argv)
				}
			}
			if slices.Contains(argv, "-rt") {
				// -rt reads its second chart from the transit set, which -R
				// never touches: each absent object the host lists there
				// must be flipped off with -RT.
				for _, n := range av.DefaultTransit {
					flipped := false
					for i, a := range argv {
						if a == "-RT" && i+1 < len(argv) && argv[i+1] == strconv.Itoa(n) {
							flipped = true
						}
					}
					if av.IsAbsent(n) && !flipped {
						t.Errorf("%s: the host's transit set lists absent object %d (%s) and the -rt argv does not switch it off with -RT: %v", d.class, n, av.Names[n], argv)
					}
				}
			}
			for _, a := range argv {
				for _, name := range av.Names {
					if strings.EqualFold(a, name) {
						t.Errorf("%s: argv carries the absent body's name %q: %v", d.class, name, argv)
					}
				}
			}
			inspected++
		}
	}
	t.Logf("%d classes, %d argvs inspected", len(classes), inspected)
}

// The real refusal through the UI's commit contract: a good frame first, then
// the floating chart moves to 1500, which needs a file this folder lacks. The
// panels keep the good frame, drawn faint, and the floating chart keeps the
// moment it was cast at; the Log carries the real `not found` line and the
// command. No panel shows a 0 Aries row.
func TestRefusedOldChartKeepsThePanelsAndLogsTheRealLine(t *testing.T) {
	if _, err := exec.LookPath("astrolog"); err != nil {
		t.Skip("astrolog not on PATH; skipping live-binary test")
	}
	astrodir.Reduced(t, []string{"seas_12.se1"}, "-R 12 13 14")
	lookup.ForgetEphemeris()
	t.Cleanup(resetEphemeris)

	m := timelineModel(t, Config{Accessible: true})
	av, err := lookup.ProbeEphemeris(context.Background())
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	m, _ = applyEphemerisProbed(m, EphemerisProbedMsg{Availability: av})
	m, cmd := m.noteWant(surfaceText, true)
	m = pumpPatient(t, m, cmd)
	if len(m.chart.positions) == 0 {
		t.Fatalf("fixture: no good first frame:\n%s", strings.Join(logTexts(m), "\n"))
	}
	good := ansi.Strip(m.View())
	if !strings.Contains(good, "Sun") {
		t.Fatalf("fixture: the good frame does not show the Sun:\n%s", good)
	}

	// Details held a report for the good frame; the refusal must not leave it.
	m, dcmd := m.requestDetails()
	m = pumpPatient(t, m, dcmd)

	kept := len(m.chart.positions)
	if kept == 0 {
		t.Fatal("fixture: the good frame has no positions")
	}
	old := time.Date(1500, 3, 1, 9, 30, 0, 0, time.UTC)
	m.slots.SetFloatingTime(old)
	m.targetFloatingTime = old
	seen := len(logTexts(m)) // the Log as it stood before the old-date request
	m, cmd = m.noteWant(surfaceText, true)
	m = pumpPatient(t, m, cmd)
	m, dcmd = m.requestDetails()
	m = pumpPatient(t, m, dcmd)

	fresh := logTexts(m)[seen:]
	hasLine := func(sub string) bool {
		for _, l := range fresh {
			if strings.Contains(l, sub) {
				return true
			}
		}
		return false
	}
	if !hasLine("SwissEph file 'seas_12.se1' not found in PATH") {
		t.Errorf("the new Log lines do not carry the real `not found` line:\n%s", strings.Join(fresh, "\n"))
	}
	// The command that failed, appended NOW and for the old date (its Julian day).
	jdDay := fmt.Sprintf("-qj %d", int(astrolog.SlotJD(old)))
	if !hasLine("astrolog ") || !hasLine(jdDay) {
		t.Errorf("no newly appended command for the old date (%q) among:\n%s", jdDay, strings.Join(fresh, "\n"))
	}
	if len(m.chart.positions) != kept {
		t.Errorf("the refused chart replaced the good frame: %d positions, want %d", len(m.chart.positions), kept)
	}
	if got := m.textPaneState(PanelBodies); got != paneRefused {
		t.Errorf("Bodies state = %v, want the kept frame drawn faint", got)
	}
	for name, panel := range map[string]string{
		"default":  ansi.Strip(m.View()),
		"aspects":  ansi.Strip(m.withDetailsTab(PanelAspects, TabAspects).View()),
		"report":   ansi.Strip(m.withDetailsTab(PanelChart, TabDetailsReport).View()),
		"timeline": ansi.Strip(m.withDetailsTab(PanelChart, TabDetailsTimeline).View()),
	} {
		if zeroAriesRow.MatchString(panel) {
			t.Errorf("%s screen carries a 0 Aries row:\n%s", name, panel)
		}
	}
}

// The chooser through its real opening: the bodies it marks are exactly the
// probe's absent set, a tick on one is refused and the status line says why,
// and the same probe answer arriving again says nothing more.
func TestChooserMarksExactlyTheProbesAbsentSet(t *testing.T) {
	astrodir.Reduced(t, []string{"seas_18.se1", "seas_12.se1", "se00010s.se1"}, "-R 11 12 13 14 43")
	m, _ := liveScreens(t)
	reg, err := lookup.ObjectRegistry(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	m.objectRegistry = reg
	next, _ := openBodySelector(m)
	m = next.(Model)
	if m.startup != startupBodyOpen {
		t.Fatalf("the chooser did not open (startup = %v)", m.startup)
	}

	marked := map[int]bool{}
	for _, o := range reg {
		if _, ok := popup.ProbeStatusFor(m.bodyModal, o); ok {
			marked[o.Number] = true
		}
	}
	absent := map[int]bool{}
	for n := range m.ephemeris.Absent {
		absent[n] = true
	}
	if len(absent) == 0 {
		t.Fatal("fixture: the probe found nothing absent")
	}
	if !reflect.DeepEqual(marked, absent) {
		t.Errorf("the chooser marks %v, the probe's absent set is %v", sortedObjectNumbers(marked), sortedObjectNumbers(absent))
	}

	// A tick on an absent body is refused, with the account on the status line.
	var target aspect.RegistryObject
	cursor := -1
	for i, o := range m.bodyModal.Matches {
		if absent[o.Number] {
			target, cursor = o, popup.GroupRows()+i
			break
		}
	}
	if cursor < 0 {
		t.Fatal("no absent body among the chooser's rows")
	}
	st := m.bodyModal
	st.Cursor = cursor
	// The row starts OFF (the host's default had ticked it): Space must leave it off.
	st.Selected[target.Number] = false
	st, _ = updateBodyModalKey(st, tea.KeyMsg{Type: tea.KeySpace}, 20)
	if st.Selected[target.Number] {
		t.Errorf("%s (no ephemeris file) was ticked by Space", target.Name)
	}
	st.ToggleGroup(recipe.ObjectGroupOf(target.Number))
	if st.Selected[target.Number] {
		t.Errorf("%s (no ephemeris file) was ticked by its group row", target.Name)
	}
	line, ok := popup.ProbeStatusFor(st, target)
	if !ok || !strings.HasPrefix(line, "Astrolog: ") {
		t.Errorf("status line = %q (%v), want Astrolog's own words for %s", line, ok, target.Name)
	}
	// A stored negative per-body verdict on a body the startup probe did not
	// name (the chooser's own probe, e.g. a chart dated outside a file's range):
	// the same refusal, for the row and for the group.
	mars := 5
	if m.ephemeris.IsAbsent(mars) {
		t.Fatal("fixture: Mars is absent")
	}
	for i, o := range st.Matches {
		if o.Number == mars {
			st.Cursor = popup.GroupRows() + i
		}
	}
	st.Selected[mars] = false
	st.SetProbe(mars, st.Inputs, lookup.BodyProbe{Drawable: false, MissingFiles: []string{"seas_12.se1"}})
	st, _ = updateBodyModalKey(st, tea.KeyMsg{Type: tea.KeySpace}, 20)
	if st.Selected[mars] {
		t.Error("Space ticked a body this opening's probe found undrawable")
	}
	st.ToggleGroup(recipe.ObjectGroupOf(mars))
	if st.Selected[mars] {
		t.Error("the group row ticked a body this opening's probe found undrawable")
	}

	// The same answer again: no second Log line and no footer command.
	lines, logged := countLines(m, "can't be drawn"), len(m.logLines)
	m, cmd := applyEphemerisProbed(m, EphemerisProbedMsg{Availability: m.ephemeris})
	if cmd != nil || len(m.logLines) != logged || countLines(m, "can't be drawn") != lines {
		t.Errorf("a second identical answer wrote %d new Log lines and returned cmd=%v", len(m.logLines)-logged, cmd != nil)
	}
}

func sortedObjectNumbers(m map[int]bool) []int {
	out := make([]int, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Ints(out)
	return out
}

// A restored selection whose only body has no ephemeris file, through the real
// startup and the real binary. (Chiron stands in for the order's Ceres: a
// session file names the legacy eighteen only, and Ceres is not one.) A narrowing that
// drains to nothing only because of the missing file never asks Astrolog for
// nothing: the chart is cast with the default set, the Log says which body and
// why, the chooser shows Chiron unticked and marked, and no row sits at 0 Aries.
func TestRestoredSelectionOfOnlyAnAbsentBodyShowsTheDefaultSetAndSaysWhy(t *testing.T) {
	if _, err := exec.LookPath("astrolog"); err != nil {
		t.Skip("astrolog not on PATH; skipping live-binary test")
	}
	astrodir.Reduced(t, []string{"seas_18.se1", "seas_12.se1"})
	lookup.ForgetEphemeris()
	t.Cleanup(resetEphemeris)
	av, err := lookup.ProbeEphemeris(context.Background())
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	if !av.IsAbsent(11) {
		t.Fatalf("fixture: Chiron is not absent: %v", av.Absent)
	}

	var tm tea.Model = newTestModel(t, Config{Accessible: true}, FrozenClock(sessionFrozen))
	tm, _ = tm.Update(tea.WindowSizeMsg{Width: 144, Height: 40})
	tm, _ = applyEphemerisProbed(tm.(Model), EphemerisProbedMsg{Availability: av})
	snap := snapshotChart()
	p := withSession(perimeter(), v1(&snap, true, true, nil, []string{"Chiron"}))
	tm, _ = tm.Update(AtlasLoadedMsg{Index: location.NewIndex([]location.PlaceInfo{berlinP, fixtureOneP})})
	tm, cmd := tm.Update(p)
	m := pumpPatient(t, tm.(Model), cmd)

	if got := markedObjects(m.slots, len(m.slots.All())-1); !slices.Equal(got, []aspect.Object{objChiron}) {
		t.Fatalf("fixture: the restored selection is %v", got)
	}
	if n := countLines(m, "Chiron can't be drawn here: no file seas_18.se1"); n != 1 {
		t.Errorf("the Log names Chiron's availability %d times, want once:\n%s", n, strings.Join(logTexts(m), "\n"))
	}
	if n := countLines(m, "Chiron selected but can't be drawn here, all are shown"); n != 1 {
		t.Errorf("the Log carries the drain line %d times, want once:\n%s", n, strings.Join(logTexts(m), "\n"))
	}
	if n := countLines(m, " not cast,"); n != 0 {
		t.Errorf("the cast failed:\n%s", strings.Join(logTexts(m), "\n"))
	}
	bodies := chooserLeftPanels(ansi.Strip(m.View()))["bodies"]
	for _, want := range []string{"Sun", "Moon"} {
		if !strings.Contains(bodies, want) {
			t.Errorf("the Bodies panel lacks %s (the default set):\n%s", want, bodies)
		}
	}
	if strings.Contains(bodies, "Chiron") || strings.Contains(bodies, "Chir") {
		t.Errorf("the Bodies panel names Chiron:\n%s", bodies)
	}
	reg, err := lookup.ObjectRegistry(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	m.objectRegistry = reg
	cm := chooserOf(t, m)
	if _, bad := cm.bodyModal.Undrawable(11); !bad || cm.bodyModal.Selected[11] {
		t.Errorf("the chooser shows Chiron ticked=%v marked=%v, want unticked and marked", cm.bodyModal.Selected[11], bad)
	}
	for name, panel := range map[string]string{
		"default": ansi.Strip(m.View()),
		"aspects": ansi.Strip(m.withDetailsTab(PanelAspects, TabAspects).View()),
		"report":  ansi.Strip(m.withDetailsTab(PanelChart, TabDetailsReport).View()),
	} {
		if zeroAriesRow.MatchString(panel) {
			t.Errorf("%s screen carries a 0 Aries row:\n%s", name, panel)
		}
	}
}

// The text files of the share: a folder without the fixed-star catalogue or
// the Uranian elements still probes (startup does not abort), names the objects
// absent with no file, and the Log says it only when the view would draw them.
func TestFolderWithoutStarsOrUranianElementsProbesAndSaysOnlyWhatIsDrawn(t *testing.T) {
	for _, c := range []struct {
		name    string
		folder  func(testing.TB, ...string) string
		object  int
		drawn   string // astrolog.as line that puts it in the view
		account string
	}{
		{"uranian elements", astrodir.ReducedWithoutUranians, 34, "-R 34", "Vulcan can't be drawn here: no data for this date"},
		{"star catalogue", astrodir.ReducedWithoutStars, 84, "-R 84", "Sirius can't be drawn here: no data for this date"},
	} {
		t.Run(c.name, func(t *testing.T) {
			c.folder(t)
			m, _ := liveScreens(t)
			if m.cantStart.any() {
				t.Fatalf("the startup probe aborted: %+v\n%s", m.cantStart, strings.Join(logTexts(m), "\n"))
			}
			if f, ok := m.ephemeris.Absent[c.object]; !ok || f != "" {
				t.Errorf("object %d: absent=%v file %q, want absent with no file", c.object, ok, f)
			}
			if countLines(m, c.account) != 0 {
				t.Errorf("the Log says %q though the view does not draw it", c.account)
			}
		})
		t.Run(c.name+" in the view", func(t *testing.T) {
			c.folder(t, c.drawn)
			m, _ := liveScreens(t)
			if n := countLines(m, c.account); n != 1 {
				t.Errorf("the Log carries %q %d times, want once:\n%s", c.account, n, strings.Join(logTexts(m), "\n"))
			}
		})
	}
}

// namesAnAvailableBody reports whether text names the Sun or the Moon, the two
// bodies no reduced folder can lack.
func namesAnAvailableBody(text string) bool {
	return strings.Contains(text, "Sun") || strings.Contains(text, "Moon")
}

// requirePanelsNonBlank is the positive half of every "no absent body is shown"
// check: each panel's OWN state holds rows, and an available body is named in
// that panel's own text, so a blank panel cannot pass. One assertion per panel.
func requirePanelsNonBlank(t *testing.T, m Model) {
	t.Helper()
	bodies := false
	for _, p := range m.chart.positions {
		if p.Object == objSun || p.Object == objMoon {
			bodies = true
		}
	}
	if len(m.chart.positions) == 0 || !bodies {
		t.Errorf("Bodies: %d positions, Sun or Moon among them = %v", len(m.chart.positions), bodies)
	}
	if len(m.chart.aspects)+len(m.chart.crossAspects) == 0 {
		t.Error("Aspects: no rows")
	}
	if rep := m.chart.aspectsReport + m.chart.crossAspectsReport; !namesAnAvailableBody(rep) {
		t.Errorf("Aspects: its report names neither Sun nor Moon:\n%.300s", rep)
	}
	if len(m.timeline.rows) == 0 {
		t.Error("Timeline: no rows")
	}
	named := false
	for _, r := range m.timeline.rows {
		if namesAnAvailableBody(r.Line) {
			named = true
			break
		}
	}
	if !named {
		t.Error("Timeline: no row names Sun or Moon")
	}
	if strings.TrimSpace(m.details.body) == "" {
		t.Error("Details: the report body is empty")
	} else if !namesAnAvailableBody(m.details.body) {
		t.Errorf("Details: the report names neither Sun nor Moon:\n%.300s", m.details.body)
	}
}

// The pair tab's comparison cast (operator, 2026-11-02): two charts, Aspects
// focused, the `② in ①` tab. In a folder without seas_18 whose astrolog.as
// lists the asteroids, no comparison spawn (-rt with the now slot, -r0 with two
// anchored charts) may name an absent object, and the cast is not refused.
func TestPairTabNoAbsent(t *testing.T) {
	for _, c := range []struct {
		name     string
		extra    bool // a second anchored chart: the pair is -r0, not -rt
		switches []string
		lines    []string // the host's astrolog.as
	}{
		{"with now", false, []string{"-rt"}, []string{"-R 12 13 14"}},
		{"two anchored", true, []string{"-r0"}, []string{"-R 12 13 14"}},
		{"now+RT", false, []string{"-rt"}, []string{"-R 12 13 14", "-RT 12 13 14"}},
		{"two+RT", true, []string{"-r0"}, []string{"-R 12 13 14", "-RT 12 13 14"}},
	} {
		t.Run(c.name, func(t *testing.T) { pairTabNoAbsent(t, c.extra, c.switches, c.lines) })
	}
}

func pairTabNoAbsent(t *testing.T, extra bool, switches, lines []string) {
	astrodir.Reduced(t, []string{"seas_18.se1", "seas_12.se1"}, lines...)
	read := argvShim(t)
	mark := 0
	m, _ := liveScreens(t, func() { mark = len(read()) })
	if extra {
		if err := addAnchor(m.slots, tlNatalWhen.AddDate(1, 0, 0), testPlaceAmsterdam()); err != nil {
			t.Fatal(err)
		}
	}
	for i := range m.slots.All() {
		m.slots.SetSlotSelected(i, true)
	}
	m.cursorIdx = len(m.slots.All()) - 1
	if extra {
		m.cursorIdx--
	}
	m.width = max(m.width, dualColumnMinWidth(len(m.slots.SelectedInOrder())))
	m = m.noteArrival()
	m.base = m.slots.All()[0] // ② read in ①
	m.focused = PanelAspects
	var pair TabID
	for _, tb := range m.panelTabs(PanelAspects) {
		if isAspectsPairTab(tb.ID) {
			pair = tb.ID
			break
		}
	}
	if pair == "" {
		t.Fatalf("fixture: no pair tab in %v", titlesOf(m.panelTabs(PanelAspects)))
	}
	m = m.withDetailsTab(PanelAspects, pair)
	m, cmd := m.afterComparisonChange()
	m = pumpPatient(t, m, cmd)
	m, cmd = m.noteWant(surfaceText, true)
	m = pumpPatient(t, m, cmd)
	m, cmd = m.requestDetails()
	m = pumpPatient(t, m, cmd)

	av := m.ephemeris
	for _, n := range []int{11, 12, 13, 14, 15} {
		if !av.IsAbsent(n) {
			t.Fatalf("fixture: object %d is not absent (%v)", n, av.Absent)
		}
	}
	hostDefault := map[int]bool{}
	for _, n := range slices.Concat(av.Default, av.DefaultTransit) {
		hostDefault[n] = true
	}
	cmp := 0
	argvs := read()[mark:]
	for _, argv := range argvs {
		if !slices.ContainsFunc(switches, func(sw string) bool { return slices.Contains(argv, sw) }) {
			continue
		}
		cmp++
		explicit, toggled := objectLists(argv)
		for _, n := range explicit {
			if av.IsAbsent(n) {
				t.Errorf("a comparison argv names absent object %d: %v", n, argv)
			}
		}
		for _, n := range toggled {
			if av.IsAbsent(n) && !hostDefault[n] {
				t.Errorf("a comparison argv switches ON absent object %d: %v", n, argv)
			}
		}
		if len(explicit)+len(toggled) == 0 {
			t.Errorf("a comparison argv carries no object band, so the host default rules: %v", argv)
		}
		// -rt reads its second chart from the transit set, which -R never
		// touches: an absent object the host lists there must be flipped off.
		if slices.Contains(argv, "-rt") {
			var flipped []int // the numbers after each -RT, not after -R
			for i, a := range argv {
				if a == "-RT" && i+1 < len(argv) {
					if n, err := strconv.Atoi(argv[i+1]); err == nil {
						flipped = append(flipped, n)
					}
				}
			}
			for _, n := range av.DefaultTransit {
				if av.IsAbsent(n) && !slices.Contains(flipped, n) {
					t.Errorf("the host's transit set lists absent object %d (%s) and the -rt argv does not switch it off with -RT: %v", n, av.Names[n], argv)
				}
			}
		}
	}
	if cmp == 0 {
		t.Fatalf("no %v spawn recorded (%d spawns): %v", switches, len(argvs), argvs)
	}
	if n := countLines(m, " not cast,"); n != 0 {
		t.Errorf("the pair tab's cast was refused:\n%s", strings.Join(logTexts(m), "\n"))
	}
	if screen := ansi.Strip(m.View()); !strings.Contains(screen, "☉") && !strings.Contains(screen, "Sun") {
		t.Errorf("the pair tab shows no Sun:\n%s", screen)
	}
}

// countConsole is how many console lines equal text.
func countConsole(m Model, text string) int {
	n := 0
	for _, l := range consoleLineTexts(m) {
		if l == text {
			n++
		}
	}
	return n
}
