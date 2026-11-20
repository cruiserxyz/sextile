package astrolog

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cruiserxyz/sextile/internal/astrologstyle"
	"github.com/cruiserxyz/sextile/internal/recipe"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// aspect_identity_test.go: an aspect is what Astrolog's list says (the
// specification). Every aspect of the binary's registry is read by the three
// parsers and by the two colour readers; a row the registry does not hold
// refuses the whole list.

// withoutAspect is the registry minus the aspect with the three letters abbrev.
func withoutAspect(reg aspect.AspectRegistry, abbrev string) aspect.AspectRegistry {
	var rows []aspect.RegistryAspect
	for _, r := range reg.Rows() {
		if r.Kind.Abbrev != abbrev {
			rows = append(rows, r)
		}
	}
	return aspect.NewAspectRegistry(rows)
}

func singleLine(abbrev string) string {
	return fmt.Sprintf("  1:     Sun (Ari) %s (Ari) Moon       - orb: a0:10' - power:  1.00\n", abbrev)
}

func pairLine(abbrev string) string {
	return fmt.Sprintf("  1:     Sun (Ari) %s with (Ari) Moon  - orb: a0:10' - power:  1.00\n", abbrev)
}

func timelineLine(abbrev string) string {
	return fmt.Sprintf("Thu 10/ 1/2026  3:43    Moon (Gem) %s [Gem] Ceres\n", abbrev)
}

// Acceptance 2: each aspect of the registry shows its rows in the single list,
// on a pair tab and in the Timeline, whatever its three letters are (Astrolog
// ends five of them in a digit: `Un1` to `Un5`).
func TestEveryRegistryAspectIsReadInThreeLists(t *testing.T) {
	withClock(t, Clock{})
	reg, asp := testRegistry(t), testAspects(t)
	if n := len(asp.Rows()); n != 24 {
		t.Fatalf("the fixture registry holds %d aspects, want 24", n)
	}
	for _, row := range asp.Rows() {
		ab := row.Kind.Abbrev
		t.Run(ab+"/single", func(t *testing.T) {
			got, err := parseAspects(singleLine(ab), reg, asp)
			if err != nil || len(got) != 1 || got[0].Kind != row.Kind {
				t.Fatalf("single list: %+v, err %v; want one %+v", got, err, row.Kind)
			}
		})
		t.Run(ab+"/pair", func(t *testing.T) {
			got, err := parseCrossAspects(pairLine(ab), reg, asp)
			if err != nil || len(got) != 1 || got[0].Kind != row.Kind {
				t.Fatalf("pair list: %+v, err %v; want one %+v", got, err, row.Kind)
			}
		})
		t.Run(ab+"/timeline", func(t *testing.T) {
			rows, err := ParseTimeline(timelineLine(ab), time.UTC, reg, asp)
			if err != nil || len(rows) != 1 || rows[0].AspectID != row.Kind || !rows[0].Resolved {
				t.Fatalf("timeline: %+v, err %v; want one %+v, resolved", rows, err, row.Kind)
			}
		})
		t.Run(ab+"/colour", func(t *testing.T) {
			style := astrologstyle.AspectStyle{First: "f", Kind: "k", Second: "s"}
			rows := []astrologstyle.AspectRow{{FirstName: "Sun", KindName: ab, SecondName: "Moon", Style: style}}
			id := astrologstyle.AspectIdentity{First: objSun, Second: objMoon, Kind: row.Kind}
			if got := mapAspectStyles(rows, reg, asp)[id]; got != style {
				t.Errorf("single list colour = %+v, want %+v", got, style)
			}
			if got := mapCrossAspectStyles(rows, reg, asp)[id]; got != style {
				t.Errorf("pair list colour = %+v, want %+v", got, style)
			}
		})
	}
}

// Acceptance 5, the single list: a row whose three letters the registry does
// not hold refuses the whole list, naming them, and no row is read.
func TestSingleListRefusesARowTheRegistryDoesNotHold(t *testing.T) {
	reg, asp := testRegistry(t), withoutAspect(testAspects(t), "Sep")
	got, err := parseAspects(singleLine("Con")+singleLine("Sep")+singleLine("Opp"), reg, asp)
	if err == nil || !strings.Contains(err.Error(), `"Sep"`) {
		t.Fatalf("err = %v, want a refusal naming Sep", err)
	}
	if got != nil {
		t.Errorf("read %d aspects from a refused list", len(got))
	}
	if _, err := parseAspects(singleLine("Sep"), reg, aspect.AspectRegistry{}); err == nil {
		t.Error("a registry not yet read held an aspect")
	}
}

// Acceptance 5, the pair list.
func TestPairListRefusesARowTheRegistryDoesNotHold(t *testing.T) {
	reg, asp := testRegistry(t), withoutAspect(testAspects(t), "Sep")
	got, err := parseCrossAspects(pairLine("Con")+pairLine("Sep")+pairLine("Opp"), reg, asp)
	if err == nil || !strings.Contains(err.Error(), `"Sep"`) {
		t.Fatalf("err = %v, want a refusal naming Sep", err)
	}
	if got != nil {
		t.Errorf("read %d aspects from a refused list", len(got))
	}
}

// Acceptance 5, the Timeline.
func TestTimelineRefusesARowTheRegistryDoesNotHold(t *testing.T) {
	withClock(t, Clock{})
	reg, asp := testRegistry(t), withoutAspect(testAspects(t), "Sep")
	rows, err := ParseTimeline(timelineLine("Con")+timelineLine("Sep")+timelineLine("Opp"), time.UTC, reg, asp)
	if err == nil || !strings.Contains(err.Error(), `"Sep"`) {
		t.Fatalf("err = %v, want a refusal naming Sep", err)
	}
	if rows != nil {
		t.Errorf("read %d rows from a refused list", len(rows))
	}
}

// A call keeps its aspect registry through every copy the spawn path makes of
// it: the staged chart path, the second invocation, the extra tokens.
func TestACallCarriesItsAspectRegistryThroughEveryCopy(t *testing.T) {
	call := testCall([]string{"-qj", "2451545.0", "-zl", "0:08W", "51:30N", "PLACEHOLDER"})
	if !call.Aspects.Ready() {
		t.Fatal("the test call holds no aspect registry")
	}
	derived, err := call.derive(aspectsArgv)
	if err != nil {
		t.Fatal(err)
	}
	staged, err := call.substitute(5, "chart.as")
	if err != nil {
		t.Fatal(err)
	}
	for name, c := range map[string]BodyCall{"derive": derived, "without": call.without([]int{1}), "substitute": staged} {
		if !c.Aspects.Ready() || len(c.Aspects.Rows()) != len(call.Aspects.Rows()) {
			t.Errorf("%s dropped the aspect registry", name)
		}
	}
}

// Acceptance 1: Kepler's chart with the septile ticked lists what
// `astrolog -RA0 13 -aa` lists: Mercury septile Saturn at 0°20'.
func TestKeplerSeptileIsReadAsAstrologListsIt(t *testing.T) {
	needAstrolog(t)
	b, err := os.ReadFile(filepath.Join("testdata", "moments", "Kepler.as"))
	if err != nil {
		t.Fatal(err)
	}
	cf, err := ResolveChartBytes(context.Background(), b, "Kepler.as")
	if err != nil {
		t.Fatal(err)
	}
	spec := fixtureSpec(t, recipe.FileChart{Record: cf.Record, JD: cf.JD, Lon: cf.Lon, Lat: cf.Lat}, recipe.Placidus, recipe.Tropical)
	spec.Aspects = recipe.AspectPolicy{Explicit: true, Indices: []int{13}}
	out, err := ComputeChart(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Aspects) != 1 {
		t.Fatalf("Kepler lists %d aspects with only the septile ticked, want 1: %+v", len(out.Aspects), out.Aspects)
	}
	a := out.Aspects[0]
	if a.Kind != kindOf("Sep") || a.AObject != objMercury || a.BObject != objSaturn {
		t.Errorf("the septile row = %+v, want Mercury septile Saturn", a)
	}
	if got := float64(a.Orb) * 60; got < 19.9 || got > 20.1 {
		t.Errorf("orb = %.2f minutes, want 20", got)
	}
	if !strings.Contains(out.AspectsReport, "Mercury (Cap) Sep (Sco) Saturn") {
		t.Errorf("Astrolog's own list lacks the row:\n%s", out.AspectsReport)
	}
}

// The computation boundary refuses an unread aspect registry before anything is
// staged or run: every entry point that lists aspects returns the refusal, the
// fake binary records no spawn, and no chart file reaches the temp folder.
func TestUnreadAspectRegistryStagesAndRunsNothing(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
	_, argvLog := fakeBinary(t, t.TempDir(), "exit 0")

	file := fixtureSpec(t, recipe.FileChart{Record: "@AI800\n-qb Dec 27 1571 14:37 ST LMT 8:52E 48:45N\n", JD: 2299000.1, Lon: 8.87, Lat: 48.75}, recipe.Placidus, recipe.Tropical)
	file.AspectRegistry = aspect.AspectRegistry{}
	pair := fixtureSpec(t, recipe.Comparison{Primary: fixLondon, Secondary: fixTromso, Relation: recipe.TransitToNatal}, recipe.Placidus, recipe.Tropical)
	pair.AspectRegistry = aspect.AspectRegistry{}
	req := TimelineRequest{Year: 2026, Month: time.October, Zone: time.UTC, Registry: testRegistry(t)}

	calls := map[string]func() error{
		"ComputeChart":        func() error { _, err := ComputeChart(context.Background(), file); return err },
		"ComputeCrossAspects": func() error { _, err := ComputeCrossAspects(context.Background(), pair); return err },
		"ComputeTimeline": func() error {
			_, _, err := ComputeTimeline(context.Background(), req, []TimelineSource{TimelineSky}, nil)
			return err
		},
		"ComputeTimelineCalls": func() error { _, err := ComputeTimelineCalls(context.Background(), req, TimelineSky, nil); return err },
		"RunVoidSearch": func() error {
			_, err := RunVoidSearch(context.Background(), VoidRequest{Moment: time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC), Registry: testRegistry(t)}, nil)
			return err
		},
	}
	for name, call := range calls {
		if err := call(); !errors.Is(err, ErrAspectsUnread) {
			t.Errorf("%s: err = %v, want ErrAspectsUnread", name, err)
		}
	}
	if got := spawnArgvs(t, argvLog); len(got) != 0 {
		t.Errorf("the binary was started %d times with no aspect registry: %v", len(got), got)
	}
	if left, _ := os.ReadDir(tmp); len(left) != 0 {
		t.Errorf("%d entries staged in the temp folder", len(left))
	}
}

// Every constructor of a body call that receives the aspect registry hands it
// to the call: the compiler, the Timeline's three searches, the void search,
// the cycle search and the Sun pair. A constructor that never received it
// leaves the call without it, and fails here.
func TestEveryBodyCallConstructorHoldsTheAspectRegistry(t *testing.T) {
	asp, reg := testAspects(t), testRegistry(t)
	held := func(name string, c BodyCall, err error) {
		t.Helper()
		if err != nil {
			t.Errorf("%s: %v", name, err)
			return
		}
		if !c.Aspects.Ready() || len(c.Aspects.Rows()) != len(asp.Rows()) {
			t.Errorf("%s: the call holds no aspect registry", name)
		}
	}
	cycle, err := CycleCall(CycleRequest{StartYear: 2016, Years: 3, Other: objVenus, Registry: reg, AspectRegistry: asp})
	held("CycleCall", cycle, err)
	pair, err := SunPairCall(SunPairRequest{When: time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC), Lat: 51, Other: objVenus, Registry: reg, AspectRegistry: asp})
	held("SunPairCall", pair, err)
	void, err := VoidCall(VoidRequest{Moment: time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC), Registry: reg, AspectRegistry: asp})
	held("VoidCall", void, err)
	req := bandReq(t, recipe.ObjectPolicy{Baseline: recipe.AstrologDefault}, recipe.AspectPolicy{})
	req.AspectRegistry = asp
	for _, src := range []TimelineSource{TimelineSky, TimelineTransit, TimelineReturn} {
		c, err := TimelineCall(src, TimelineSegments(2026, time.October, req.Zone)[0], req)
		held("TimelineCall "+src.String(), c, err)
	}
	plan, err := Compile(fixtureSpec(t, fixTromso, recipe.Placidus, recipe.Tropical), recipe.Positions{})
	if err != nil {
		t.Fatal(err)
	}
	held("Compile", plan.Invocations[0].call, nil)
}

// misfitRegistry is the aspect registry of a host whose Sextile is renamed
// "S:X": read in full, with that aspect recorded as one no list can be read with.
func misfitRegistry(t *testing.T) aspect.AspectRegistry {
	t.Helper()
	rows, err := ParseAspectRegistry(loadHA(t, "ha-colon.txt"))
	if err != nil {
		t.Fatal(err)
	}
	reg := NewAspectRegistry(rows)
	if reg.Refusal() == "" {
		t.Fatal("the fixture registry has no unreadable aspect")
	}
	return reg
}

// D1: with an aspect renamed to letters no list can be read with, the chart is
// still cast (Bodies work) with one process and no aspect list, and every list
// that needs the registry refuses before any process starts.
func TestARenamedAspectRefusesTheListsAndLeavesTheChart(t *testing.T) {
	needAstrolog(t)
	bad := misfitRegistry(t)
	spec := fixtureSpec(t, fixTromso, recipe.Placidus, recipe.Tropical)
	spec.AspectRegistry = bad
	out, err := ComputeChart(context.Background(), spec)
	if err != nil || len(out.Positions) == 0 || len(out.Aspects) != 0 || len(out.Commands) != 1 {
		t.Errorf("ComputeChart: %d positions, %d aspects, %d commands, err %v; want the positions from one process and no list", len(out.Positions), len(out.Aspects), len(out.Commands), err)
	}

	_, argvLog := fakeBinary(t, t.TempDir(), "exit 0")
	pair := fixtureSpec(t, recipe.Comparison{Primary: fixLondon, Secondary: fixTromso, Relation: recipe.TransitToNatal}, recipe.Placidus, recipe.Tropical)
	pair.AspectRegistry = bad
	req := TimelineRequest{Year: 2026, Month: time.October, Zone: time.UTC, Registry: testRegistry(t), AspectRegistry: bad}
	for name, call := range map[string]func() error{
		"ComputeCrossAspects": func() error { _, err := ComputeCrossAspects(context.Background(), pair); return err },
		"ComputeTimeline": func() error {
			_, _, err := ComputeTimeline(context.Background(), req, []TimelineSource{TimelineSky}, nil)
			return err
		},
		"RunVoidSearch": func() error {
			_, err := RunVoidSearch(context.Background(), VoidRequest{Moment: time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC), Registry: testRegistry(t), AspectRegistry: bad}, nil)
			return err
		},
	} {
		var refused *AspectsRefusedError
		if err := call(); !errors.As(err, &refused) || !strings.Contains(refused.Text, `Sextile is renamed "S:X"`) {
			t.Errorf("%s: err = %v, want the refusal naming Sextile and S:X", name, err)
		}
	}
	if got := spawnArgvs(t, argvLog); len(got) != 0 {
		t.Errorf("a refused list started the binary %d times", len(got))
	}
}

// R1: the cycle reader reads the conjunction by the letters of the binary in
// use, through the registry its call carries, and requires the conjunction.
func TestCycleReaderReadsARenamedConjunction(t *testing.T) {
	reg := renamedConjunct(t, "Cnj")
	const line = "Mon  6/ 6/2016 21:49:20     Sun (16Gem35) %s (16Gem35) Venus\n"
	evs, err := ParseCycleEvents(fmt.Sprintf(line, "Cnj"), testRegistry(t), reg)
	if err != nil || len(evs) != 1 || evs[0].Kind != CycleConjunction {
		t.Fatalf("a conjunction printed Cnj: %+v, err %v", evs, err)
	}
	if _, err := ParseCycleEvents(fmt.Sprintf(line, "Con"), testRegistry(t), reg); err == nil {
		t.Error("letters the registry does not hold were read as the conjunction")
	}
	if _, err := ParseCycleEvents(fmt.Sprintf(line, "Opp"), testRegistry(t), reg); err == nil {
		t.Error("another aspect was read as the conjunction the search asked for")
	}
	// A conjunction renamed to letters outside the form refuses the search, by name.
	_, err = ParseCycleEvents(fmt.Sprintf(line, "C:j"), testRegistry(t), renamedConjunct(t, "C:j"))
	var refused *AspectsRefusedError
	if !errors.As(err, &refused) || !strings.Contains(refused.Text, `Conjunct is renamed "C:j"`) {
		t.Errorf("err = %v, want the refusal naming Conjunct and C:j", err)
	}
	// A renamed aspect the search does not need leaves it alone.
	if _, err := ParseCycleEvents(fmt.Sprintf(line, "Con"), testRegistry(t), misfitRegistry(t)); err != nil {
		t.Errorf("a renamed Sextile refused the cycle search: %v", err)
	}
}

// R2: Run itself checks the registry first: a plan compiled with no aspect
// registry and handed straight to Run starts nothing.
func TestRunWithAnUnreadRegistryStartsNothing(t *testing.T) {
	_, argvLog := fakeBinary(t, t.TempDir(), "exit 0")
	spec := fixtureSpec(t, fixTromso, recipe.Placidus, recipe.Tropical)
	spec.AspectRegistry = aspect.AspectRegistry{}
	plan, err := Compile(spec, recipe.Positions{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Run(context.Background(), plan.Invocations[0].call); !errors.Is(err, ErrAspectsUnread) {
		t.Errorf("Run: err = %v, want ErrAspectsUnread", err)
	}
	if got := spawnArgvs(t, argvLog); len(got) != 0 {
		t.Errorf("Run started the binary %d times with no aspect registry", len(got))
	}
}

// renamedConjunct is the fixture registry with the conjunction's letters
// changed, in the list and in the unrestricted section, as `-YAD 1` does.
func renamedConjunct(t *testing.T, letters string) aspect.AspectRegistry {
	t.Helper()
	text := loadHA(t, "ha-default.txt")
	text = strings.Replace(text, "(Con)", "("+letters+")", 1)
	text = strings.Replace(text, "\nCon:", "\n"+letters+":", 1)
	rows, err := ParseAspectRegistry(text)
	if err != nil {
		t.Fatal(err)
	}
	return NewAspectRegistry(rows)
}
