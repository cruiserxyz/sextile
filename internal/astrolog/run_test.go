package astrolog

import (
	"context"
	"errors"
	"os/exec"
	"slices"
	"strings"
	"testing"

	"github.com/cruiserxyz/sextile/internal/astrologstyle"
	"github.com/cruiserxyz/sextile/internal/ir"
	"github.com/cruiserxyz/sextile/internal/recipe"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// TestRunAgainstLiveAstrolog exercises the full astrolog path — two real
// astrolog invocations, sanity check, parse — against the same vernal-
// equinox fixture captured in testdata/. Skipped when astrolog is not on
// PATH so the rest of the suite stays hermetic on machines without the
// toolchain.
func TestRunAgainstLiveAstrolog(t *testing.T) {
	if _, err := exec.LookPath("astrolog"); err != nil {
		t.Skip("astrolog not on PATH; skipping live-binary integration test")
	}

	cs := ir.CommandSpec{
		CommandID: "test-fixture-1985",
		Argv: []string{
			"-qa", "3", "21", "1985", "14:30", "0", "0:10W", "51:30N",
		},
	}

	out, err := Run(context.Background(), testCall(cs.Argv))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	// Default chart: classical ten + North Node + Ascendant + Midheaven (the
	// two angles emitted by default; Slice B1 stopped dropping them).
	if got := len(out.Positions); got != 13 {
		t.Errorf("Run returned %d positions, want 13", got)
	}
	if got := len(out.Aspects); got < 10 {
		t.Errorf("Run returned %d aspects, want >= 10", got)
	}
	if got := len(out.AspectStyles); got < 10 {
		t.Errorf("Run returned %d aspect styles, want >= 10", got)
	}
	conStyle := out.AspectStyles[astrologstyle.AspectIdentity{First: aspect.Sun, Second: aspect.Moon, Kind: aspect.Conjunction}]
	if conStyle.First == "" || conStyle.Kind == "" || conStyle.Second == "" {
		t.Errorf("Sun-Moon conjunction styles incomplete: %#v", conStyle)
	}

	// The fixture chart is cast at the vernal equinox — the Sun sits close
	// to 0° Aries (longitude < 2° in tropical coordinates).
	sun := findPosition(t, out.Positions, aspect.Sun)
	if sun.Longitude > 2.0 {
		t.Errorf("Sun at vernal equinox expected near 0° Aries, got %v", sun.Longitude)
	}
}

// TestRunPositionsOnlyAgainstLiveAstrolog exercises the new one-spawn
// positions-only path against the same vernal-equinox
// fixture as TestRunAgainstLiveAstrolog. Asserts argv assembly, sanity
// check, and position parsing all work through the new path without
// the second `-a` aspects invocation. the review flagged that
// the original WR-6 tests only stubbed ChartComputer and so could not
// catch a regression in the real positions-only astrolog wrapper.
//
// The returned RawOutput must have Positions populated identically to
// Run's positions chart and Aspects empty (no aspects spawn was made).
func TestRunPositionsOnlyAgainstLiveAstrolog(t *testing.T) {
	if _, err := exec.LookPath("astrolog"); err != nil {
		t.Skip("astrolog not on PATH; skipping live-binary integration test")
	}

	cs := ir.CommandSpec{
		CommandID: "test-fixture-1985-positions-only",
		Argv: []string{
			"-qa", "3", "21", "1985", "14:30", "0", "0:10W", "51:30N",
		},
	}

	out, err := RunPositionsOnly(context.Background(), testCall(cs.Argv))
	if err != nil {
		t.Fatalf("RunPositionsOnly: %v", err)
	}
	if got := len(out.Positions); got != 13 {
		t.Errorf("RunPositionsOnly returned %d positions, want 13 (same as Run)", got)
	}
	if got := len(out.Aspects); got != 0 {
		t.Errorf("RunPositionsOnly returned %d aspects, want 0 (the -a spawn must be skipped)", got)
	}
	sun := findPosition(t, out.Positions, aspect.Sun)
	if sun.Longitude > 2.0 {
		t.Errorf("Sun at vernal equinox expected near 0° Aries, got %v", sun.Longitude)
	}
}

func TestComputeCrossAspectsAgainstLiveAstrologCarriesStyles(t *testing.T) {
	if _, err := exec.LookPath("astrolog"); err != nil {
		t.Skip("astrolog not on PATH; skipping live-binary integration test")
	}
	spec, err := recipe.NewChartSpec(
		recipe.Comparison{
			Primary:   recipe.MomentAtPlace{JD: 2446232.0, Lon: 11.5667, Lat: 48.1333},
			Secondary: recipe.MomentAtPlace{JD: 2451545.0, Lon: -0.1278, Lat: 51.5074},
		},
		recipe.WholeSign,
		recipe.Tropical,
		recipe.ObjectPolicy{Baseline: recipe.AstrologDefault},
	)
	if err != nil {
		t.Fatalf("comparison spec: %v", err)
	}
	out, err := ComputeCrossAspects(context.Background(), spec)
	if err != nil {
		t.Fatalf("ComputeCrossAspects: %v", err)
	}
	if len(out.Aspects) == 0 || len(out.AspectStyles) == 0 {
		t.Fatalf("cross output missing aspects/styles: aspects=%d styles=%d", len(out.Aspects), len(out.AspectStyles))
	}
	first := out.Aspects[0]
	style := out.AspectStyles[astrologstyle.AspectIdentity{First: first.Primary, Second: first.Secondary, Kind: first.Kind}]
	if style.First == "" || style.Kind == "" || style.Second == "" {
		t.Fatalf("first cross aspect styles incomplete: aspect=%+v style=%#v", first, style)
	}
}

// TestComputeChart_RestrictionSurfacesFullBodySet proves B2 end-to-end: the -R
// switches SURFACE the v1 body set — Chiron, both nodes, Lilith, and the
// Descendant/Imum Coeli angles on top of the default thirteen. (-Yn chooses
// true-vs-mean for the node/Lilith once enabled; -R 18 is what makes Lilith
// visible at all.)
func TestComputeChart_RestrictionSurfacesFullBodySet(t *testing.T) {
	if _, err := exec.LookPath("astrolog"); err != nil {
		t.Skip("astrolog not on PATH; skipping live-binary integration test")
	}
	const jd = 2446232.104 // ~1985-06-15 14:30 UT (value irrelevant to the body set)
	out, err := ComputeChart(context.Background(), hellenisticSpecAt(t, jd, 11.5667, 48.1333))
	if err != nil {
		t.Fatalf("ComputeChart: %v", err)
	}
	// Default thirteen + the five enabled extras = eighteen.
	if got := len(out.Positions); got != 18 {
		t.Errorf("ComputeChart returned %d positions, want 18", got)
	}
	for _, b := range []aspect.Body{aspect.Chiron, aspect.SouthNode, aspect.Lilith, aspect.Descendant, aspect.ImumCoeli} {
		findPosition(t, out.Positions, b) // fatals if the restriction didn't surface it
	}
}

func TestValidateArgvRequiresChartSwitch(t *testing.T) {
	err := validateArgv([]string{"-a"})
	if !errors.Is(err, ErrUnsafeArgv) {
		t.Errorf("expected ErrUnsafeArgv, got %v", err)
	}
}

func TestValidateArgvRejectsNakedB(t *testing.T) {
	argv := []string{"-qa", "3", "21", "1985", "14:30", "0", "0:10W", "51:30N", "-b"}
	err := validateArgv(argv)
	if !errors.Is(err, ErrUnsafeArgv) {
		t.Errorf("expected ErrUnsafeArgv for naked -b, got %v", err)
	}
}

func TestValidateArgvAcceptsSafeBaseline(t *testing.T) {
	argv := []string{"-qa", "3", "21", "1985", "14:30", "0", "0:10W", "51:30N"}
	if err := validateArgv(argv); err != nil {
		t.Errorf("expected nil for safe baseline, got %v", err)
	}
}

func TestColorAspectsArgvAddsColorExactlyOnce(t *testing.T) {
	base := []string{"-qa", "3", "21", "1985", "14:30", "0", "0:10W", "51:30N", "-a"}
	got := colorAspectsArgv(base)
	if !slices.Equal(got, append(append([]string{}, base...), "-k")) {
		t.Fatalf("colorAspectsArgv = %#v", got)
	}
	if got := colorAspectsArgv(got); len(got) != len(base)+1 {
		t.Fatalf("colorAspectsArgv duplicated -k: %#v", got)
	}
}

func TestMapAspectStylesCanonicalizesBodyOrderWithSpans(t *testing.T) {
	rows := []astrologstyle.AspectRow{{
		FirstName: "Moon", KindName: "Squ", SecondName: "Sun",
		Style: astrologstyle.AspectStyle{First: "moon", Kind: "square", Second: "sun"},
	}}
	got := mapAspectStyles(rows)[astrologstyle.AspectIdentity{First: aspect.Sun, Second: aspect.Moon, Kind: aspect.Square}]
	if got != (astrologstyle.AspectStyle{First: "sun", Kind: "square", Second: "moon"}) {
		t.Fatalf("canonical style = %#v", got)
	}
}

func TestMapCrossAspectStylesPreservesPrimarySecondaryRoles(t *testing.T) {
	rows := []astrologstyle.AspectRow{{
		FirstName: "Ascenda", KindName: "Sex", SecondName: "Moon",
		Style: astrologstyle.AspectStyle{First: "primary", Kind: "sextile", Second: "secondary"},
	}}
	got := mapCrossAspectStyles(rows)[astrologstyle.AspectIdentity{First: aspect.Ascendant, Second: aspect.Moon, Kind: aspect.Sextile}]
	if got != (astrologstyle.AspectStyle{First: "primary", Kind: "sextile", Second: "secondary"}) {
		t.Fatalf("cross style = %#v", got)
	}
}

// TestAspectsArgv_UsesPhaseMode: the second spawn of Run is the -aa
// list (Astrolog's own applying/separating markers), never plain -a, and it
// carries the chart switches of the positions argv plus color.
func TestAspectsArgv_UsesPhaseMode(t *testing.T) {
	got := aspectsArgv([]string{"-qj", "2451545.000000", "-zl", "0:08W", "51:30N", "-c", "0", "-v0"})
	want := []string{"-qj", "2451545.000000", "-zl", "0:08W", "51:30N", "-c", "0", "-v0", "-aa", "-k"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("aspectsArgv = %v, want %v", got, want)
	}
	for _, tok := range got {
		if tok == "-a" {
			t.Errorf("aspectsArgv emitted plain -a (no phase markers): %v", got)
		}
	}
}

// TestValidateArgv_AcceptsTransitComparison: -rt is a chart-entry
// switch like -r0; a bare argv with neither is still refused.
func TestValidateArgv_AcceptsTransitComparison(t *testing.T) {
	if err := validateArgv([]string{"-rt", "a.as", "b.as", "-c", "14", "-aa"}); err != nil {
		t.Errorf("-rt refused: %v", err)
	}
	if err := validateArgv([]string{"-r0", "a.as", "b.as", "-c", "14", "-aa"}); err != nil {
		t.Errorf("-r0 refused: %v", err)
	}
	if err := validateArgv([]string{"-c", "14", "-aa"}); err == nil {
		t.Error("argv without a chart-entry switch was accepted")
	}
}
