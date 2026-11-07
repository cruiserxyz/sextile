package astrolog

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/cruiserxyz/sextile/internal/recipe"
	"github.com/cruiserxyz/sextile/internal/testenv/astrodir"
)

// availability_test.go: an object without its ephemeris file is never computed
// (point 2), a `not found` line refuses the call that carries it (point 3), and
// the probe learns both from Astrolog alone (the order, 2026-11-02).

// withAvailability installs av for the test and puts the zero value back.
func withAvailability(t *testing.T, av Availability) {
	t.Helper()
	SetAvailability(av)
	t.Cleanup(func() { SetAvailability(hostDefaultAvailability) })
}

func ceresPallasJuno() Availability {
	return Availability{
		Absent:  map[int]string{12: "seas_18.se1", 13: "seas_18.se1", 14: "seas_18.se1"},
		Names:   map[int]string{12: "Ceres", 13: "Pallas", 14: "Juno"},
		Default: []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 12, 13, 14, 16, 22, 31},
	}
}

// ---- point 2: the object list --------------------------------------------

func TestExplicitObjectListLeavesOutAnAbsentBody(t *testing.T) {
	withAvailability(t, ceresPallasJuno())
	got, err := emitObjects(recipe.ObjectPolicy{Baseline: recipe.Explicit, Selection: []int{1, 2, 12, 13, 14}})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"-R0", "1", "2"}; !slices.Equal(got, want) {
		t.Errorf("explicit show-list = %v, want %v", got, want)
	}
}

func TestDeltaObjectListSwitchesAnAbsentDefaultOff(t *testing.T) {
	withAvailability(t, ceresPallasJuno())
	got, err := emitObjects(recipe.ObjectPolicy{})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"-R", "12", "-R", "13", "-R", "14"}; !slices.Equal(got, want) {
		t.Errorf("delta object list = %v, want the host's default asteroids flipped off %v", got, want)
	}
}

func TestDeltaObjectListNeverSwitchesAnAbsentBodyOn(t *testing.T) {
	av := ceresPallasJuno()
	av.Absent[11], av.Names[11] = "seas_18.se1", "Chiron"
	withAvailability(t, av)
	got, err := emitObjects(recipe.ObjectPolicy{Add: recipe.NewObjectSet(11)})
	if err != nil {
		t.Fatal(err)
	}
	if slices.Contains(got, "11") {
		t.Errorf("an Add of Chiron, which has no file, still flips object 11: %v", got)
	}
}

func TestTimelineBandLeavesOutAnAbsentBody(t *testing.T) {
	withAvailability(t, ceresPallasJuno())
	d := DefaultSets{Main: ceresPallasJuno().Default, Transit: []int{1, 3, 4, 5, 6, 7, 8, 9, 10, 16}}
	band, err := timelineBand(TimelineSky, recipe.ObjectPolicy{}, nil, recipe.AspectPolicy{}, d, false)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"-R0", "1", "2", "3", "4", "5", "6", "7", "8", "9", "10", "16", "22", "31"}
	if !slices.Equal(band.tokens, want) {
		t.Errorf("timeline band = %v, want %v", band.tokens, want)
	}
}

func TestCycleRefusesABodyWithoutItsFile(t *testing.T) {
	av := ceresPallasJuno()
	av.Absent[11], av.Names[11] = "seas_18.se1", "Chiron"
	withAvailability(t, av)
	_, err := CycleCall(CycleRequest{StartYear: 2026, Years: 1, Other: objChiron})
	if !errors.Is(err, ErrBodyUnavailable) {
		t.Fatalf("CycleCall(Chiron) = %v, want ErrBodyUnavailable", err)
	}
	if _, err := SunPairCall(SunPairRequest{When: time.Unix(0, 0), Other: objChiron}); !errors.Is(err, ErrBodyUnavailable) {
		t.Errorf("SunPairCall(Chiron) = %v, want ErrBodyUnavailable", err)
	}
	if _, err := CycleCall(CycleRequest{StartYear: 2026, Years: 1, Other: objMars}); err != nil {
		t.Errorf("CycleCall(Mars) refused: %v", err)
	}
}

// ---- point 3: a not-found line refuses --------------------------------------

func TestMissingEphemerisLineRefusesAWarnedCall(t *testing.T) {
	line := "SwissEph file 'seas_18.se1' not found in PATH '/x/'\n"
	if got := warningLines(line); len(got) != 1 {
		t.Errorf("warningLines(%q) = %v, want the line kept as a warning", line, got)
	}
	if got := warningLinesFor(line, true); len(got) != 0 {
		t.Errorf("a collecting call kept %v, want none", got)
	}
}

// ---- the probe, against real folders ---------------------------------------

// A file the complete share has and a reduced folder lacks. Evidence from the
// 2026-11-02 measurement; the code asks Astrolog and does not read this table.
var reducedFolders = []struct {
	name    string
	without []string
	file    string
	objects []int
}{
	{"main asteroids", []string{"seas_18.se1", "seas_12.se1"}, "seas_18.se1", []int{11, 12, 13, 14, 15, 44}},
	{"a numbered asteroid", []string{"se00010s.se1"}, "se00010s.se1", []int{43}},
	{"a dwarf planet", []string{"s136199s.se1"}, "s136199s.se1", []int{45}},
}

func TestProbeNamesEveryObjectAFolderLacks(t *testing.T) {
	needAstrolog(t)
	for _, c := range reducedFolders {
		t.Run(c.name, func(t *testing.T) {
			astrodir.Reduced(t, c.without)
			av, err := ProbeAvailability(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			for _, n := range c.objects {
				if av.Absent[n] != c.file {
					t.Errorf("object %d: absent file %q, want %q", n, av.Absent[n], c.file)
				}
			}
			// Nothing else of the folder's own is lost: only the planetary moons
			// may be absent too, on a share without their sepm files.
			for n, f := range av.Absent {
				if !slices.Contains(c.objects, n) && !strings.HasPrefix(f, "sepm") {
					t.Errorf("object %d (%s) reported absent for %q", n, av.Names[n], f)
				}
			}
		})
	}
}

func TestProbeOfTheCompleteFolderLeavesTheMainBodiesAvailable(t *testing.T) {
	needAstrolog(t)
	astrodir.Reduced(t, nil)
	av, err := ProbeAvailability(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range []int{1, 2, 10, 11, 12, 13, 14, 15, 43, 45} {
		if av.IsAbsent(n) {
			t.Errorf("object %d reported absent in the complete folder: %q", n, av.Absent[n])
		}
	}
}

// The incident: the host's astrolog.as lists Ceres, Pallas and Juno, the folder
// lacks seas_18.se1, and the timeline search listed them at 0 Aries.
func TestTimelineNeverComputesABodyWithoutItsFile(t *testing.T) {
	needAstrolog(t)
	astrodir.Reduced(t, []string{"seas_18.se1", "seas_12.se1"}, "-R 12 13 14")
	ctx := context.Background()
	defaults, err := ReadDefaultSets(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(defaults.Main, 12) {
		t.Fatalf("the fixture's default main set %v does not list Ceres", defaults.Main)
	}
	req := TimelineRequest{Year: 2026, Month: time.November, Zone: time.UTC, Defaults: defaults}
	run := func() ([]TimelineRow, error) {
		rows, _, err := ComputeTimeline(ctx, req, []TimelineSource{TimelineSky}, nil)
		return rows, err
	}

	// Unprobed (the old behaviour): the call is refused, never shown.
	if _, err := run(); !errors.Is(err, ErrAstrologWarned) {
		t.Fatalf("with the object list unnarrowed the search returned %v, want a refusal (ErrAstrologWarned)", err)
	}

	av, err := ProbeAvailability(ctx)
	if err != nil {
		t.Fatal(err)
	}
	withAvailability(t, av)
	rows, err := run()
	if err != nil {
		t.Fatalf("narrowed search: %v", err)
	}
	if len(rows) == 0 {
		t.Fatal("narrowed search returned no rows")
	}
	for _, r := range rows {
		if strings.Contains(r.Line, "Ceres") || strings.Contains(r.Line, "Pallas") || strings.Contains(r.Line, "Juno") {
			t.Errorf("a row names a body without its file: %q", r.Line)
		}
	}
}

func TestChartNeverComputesABodyWithoutItsFile(t *testing.T) {
	needAstrolog(t)
	astrodir.Reduced(t, []string{"seas_18.se1", "seas_12.se1"})
	spec := recipe.ChartSpec{
		Source:   recipe.MomentAtPlace{JD: 2451545.0, Lon: 4.9, Lat: 52.37},
		Objects:  recipe.ObjectPolicy{Baseline: recipe.Explicit, Selection: []int{1, 2, 12, 13, 14}},
		Registry: testRegistry(t),
	}
	if _, err := ComputeChart(context.Background(), spec); !errors.Is(err, ErrAstrologWarned) {
		t.Fatalf("a chart asking for Ceres without its file returned %v, want a refusal", err)
	}
	av, err := ProbeAvailability(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	withAvailability(t, av)
	raw, err := ComputeChart(context.Background(), spec)
	if err != nil {
		t.Fatalf("narrowed chart: %v", err)
	}
	if len(raw.Positions) != 2 || len(raw.Unavailable) != 0 {
		t.Errorf("narrowed chart: %d positions, unavailable %v; want the Sun and Moon only", len(raw.Positions), raw.Unavailable)
	}
	if strings.Contains(raw.AspectsReport, "Ceres") {
		t.Errorf("the aspect report names Ceres:\n%s", raw.AspectsReport)
	}
}

// Point 3, one assertion per way Sextile calls Astrolog: a `not found` line on
// stderr refuses the result, whatever the call is for.
func TestEveryCallKindRefusesAMissingEphemerisLine(t *testing.T) {
	warnShim(t, "echo 'OUTPUT'; echo \""+missingLine("seas_18.se1")+"\" >&2")
	ctx := context.Background()
	spec, err := recipe.NewChartSpec(hookOther, recipe.WholeSign, recipe.Tropical, recipe.ObjectPolicy{})
	if err != nil {
		t.Fatal(err)
	}
	cycle, err := CycleCall(CycleRequest{StartYear: 2026, Years: 1, Other: objMars})
	if err != nil {
		t.Fatal(err)
	}
	tl := TimelineRequest{Year: 2026, Month: time.November, Zone: time.UTC}
	kinds := map[string]func() error{
		"chart":     func() error { _, err := ComputeChart(ctx, spec); return err },
		"positions": func() error { _, err := ComputeChartPositionsOnly(ctx, spec); return err },
		"body report": func() error {
			_, _, err := ComputeRawReport(ctx, spec, recipe.BodyRawReport(objMoon))
			return err
		},
		"aspect report": func() error { _, _, err := ComputeRawReport(ctx, spec, recipe.AspectsRawReport()); return err },
		"wheel":         func() error { _, err := RenderWheelImage(ctx, spec, recipe.DefaultWheelRender()); return err },
		"cycle":         func() error { _, _, err := RunTimelineSearch(ctx, cycle); return err },
		"timeline": func() error {
			_, _, err := ComputeTimeline(ctx, tl, []TimelineSource{TimelineSky}, nil)
			return err
		},
	}
	for name, run := range kinds {
		if err := run(); !errors.Is(err, ErrAstrologWarned) {
			t.Errorf("%s: err = %v, want a refusal (ErrAstrologWarned)", name, err)
		}
	}
}

// The folder key names the Astrolog that runs too: the same variables with
// another binary first on PATH is another answer.
func TestAvailabilityFolderNamesTheAstrologBinary(t *testing.T) {
	needAstrolog(t)
	a := AvailabilityFolder()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "astrolog"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	if b := AvailabilityFolder(); b == a {
		t.Errorf("another astrolog first on PATH gives the same key %q", b)
	}
}

// "Absent" in the probe means asked for and not returned at a real position,
// whatever Astrolog says or does not say about it; a file name is extra.
//
// absentNumbers is the absent objects keep accepts: a comparison leaves out the
// planetary moons, which a share without their sepm files lacks anyway.
func absentNumbers(av Availability, keep func(n int, file string) bool) []int {
	var out []int
	for n, f := range av.Absent {
		if keep(n, f) {
			out = append(out, n)
		}
	}
	slices.Sort(out)
	return out
}

func TestProbeNamesEveryFixedStarWhenTheCatalogueIsMissing(t *testing.T) {
	needAstrolog(t)
	astrodir.ReducedWithoutStars(t)
	reg, err := ObjectRegistry(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var stars []int
	for _, o := range reg {
		if o.Star {
			stars = append(stars, o.Number)
		}
	}
	// Spica (99) and the Galactic Centre (132) keep a real row without the
	// catalogue (TestSpicaAndGalacticCentreKeepARowWithoutTheCatalogue).
	stars = slices.DeleteFunc(stars, func(n int) bool { return n == 99 || n == 132 })
	slices.Sort(stars)
	if len(stars) < 40 {
		t.Fatalf("fixture: only %d stars in the registry", len(stars))
	}
	av, err := ProbeAvailability(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	got := absentNumbers(av, func(n int, f string) bool { return slices.Contains(stars, n) })
	if !slices.Equal(got, stars) {
		t.Errorf("absent stars = %v, want exactly the registry's %v", got, stars)
	}
	for _, n := range got {
		if av.Absent[n] != "" {
			t.Errorf("star %d: file %q, want none (Astrolog names none)", n, av.Absent[n])
		}
	}
}

func TestProbeCompletesAndNamesVulcanWhenTheUranianElementsAreMissing(t *testing.T) {
	needAstrolog(t)
	astrodir.ReducedWithoutUranians(t)
	av, err := ProbeAvailability(context.Background())
	if err != nil {
		t.Fatalf("the probe aborted: %v", err)
	}
	// Apart from the moons (absent, named by their sepm file, on a share without
	// those files), exactly Vulcan is absent, with no file.
	got := absentNumbers(av, func(n int, f string) bool { return !strings.HasPrefix(f, "sepm") })
	if !slices.Equal(got, []int{34}) || av.Absent[34] != "" {
		t.Errorf("absent (besides moons) = %v, Vulcan's file %q; want exactly [34] with no file", got, av.Absent[34])
	}
}

// Titan and Titania print the same four-letter label. Neither claims the
// other: with their two moon files removed (a share may hold them), both are
// probed one by one and each gets its own file.
func TestSharedLabelObjectsAreProbedIndividually(t *testing.T) {
	needAstrolog(t)
	astrodir.Reduced(t, []string{"sepm9606.se1", "sepm9703.se1"})
	reg, err := ObjectRegistry(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	num := map[string]int{}
	for _, o := range reg {
		num[o.Name] = o.Number
	}
	titan, titania := num["Titan"], num["Titania"]
	if titan == 0 || titania == 0 {
		t.Skip("this Astrolog has no Titan/Titania")
	}
	av, err := ProbeAvailability(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if av.Absent[titan] == "" || av.Absent[titania] == "" || av.Absent[titan] == av.Absent[titania] {
		t.Errorf("Titan %q, Titania %q: want each absent with its own file", av.Absent[titan], av.Absent[titania])
	}
}

// Normal calls keep refusing on any stderr line; only the probing call takes
// the line as evidence.
func TestProbingCallTakesAnyStderrLineAsEvidenceNormalCallsRefuse(t *testing.T) {
	needAstrolog(t)
	astrodir.ReducedWithoutUranians(t)
	spec, err := recipe.NewChartSpec(hookOther, recipe.WholeSign, recipe.Tropical, recipe.ObjectPolicy{Baseline: recipe.Explicit, Selection: []int{34}})
	if err != nil {
		t.Fatal(err)
	}
	spec.Registry = testRegistry(t)
	if _, err := ComputeChartPositionsOnly(context.Background(), spec); !errors.Is(err, ErrAstrologWarned) {
		t.Errorf("a normal call returned %v, want ErrAstrologWarned", err)
	}
	out, err := ComputeChartProbe(context.Background(), spec)
	if err != nil {
		t.Fatalf("the probing call refused: %v", err)
	}
	if len(out.Positions) != 0 {
		t.Errorf("positions for Vulcan without its elements: %v", out.Positions)
	}
}
