package lookup

import (
	"bytes"
	"context"
	"regexp"
	"strings"
	"testing"

	"github.com/cruiserxyz/sextile/internal/astrolog"
	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/recipe"
	"github.com/cruiserxyz/sextile/internal/store"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// chosenLens is a lens with both choosers engaged: an explicit body list (no
// Moon) and an explicit aspect list, so a band that fails to reach a call
// shows up in its argv.
func chosenLens() Lens {
	l := Hellenistic
	l.Objects = recipe.ObjectPolicy{Baseline: recipe.Explicit, Selection: []int{1, 3, 4, 5, 6, 7, 8, 9, 10}, TrueNodeMode: true}
	l.Aspects = recipe.AspectPolicy{Explicit: true, Indices: []int{1, 5}}
	return l
}

func hasTok(argv []string, tok string) bool {
	for _, a := range argv {
		if a == tok {
			return true
		}
	}
	return false
}

// A synastry pair is astrolog's -r0 with chart 1 first, never the -rt transit
// relation, and carries both chooser bands (C1c, C15).
func TestSynastrySpecIsR0AndCarriesTheBand(t *testing.T) {
	a, b := fixtureByName(t, "F1"), fixtureByName(t, "F3")
	jd1, jd2 := astrolog.GregorianToJD(a.when), astrolog.GregorianToJD(b.when)
	lens := chosenLens()
	for name, build := range map[string]func() (recipe.ChartSpec, error){
		"utc": func() (recipe.ChartSpec, error) { return lens.synastrySpecAt(jd1, a.place, jd2, b.place) },
		"local": func() (recipe.ChartSpec, error) {
			return lens.localSynastrySpecAt(jd1, a.place, store.ReportLabel{}, jd2, b.place, store.ReportLabel{})
		},
	} {
		spec, err := build()
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		for _, p := range []recipe.Projection{recipe.Aspects{}, recipe.BodyRawReport(aspect.Sun)} {
			plan, err := astrolog.Compile(spec, p)
			if err != nil {
				t.Fatalf("%s/%s: %v", name, p.Label(), err)
			}
			inv := plan.Invocations[0]
			if inv.Argv[0] != "-r0" || hasTok(inv.Argv, "-rt") {
				t.Errorf("%s/%s: argv %v, want -r0 and no -rt", name, p.Label(), inv.Argv)
			}
			if !hasTok(inv.Argv, "-R0") || !hasTok(inv.Argv, "-RA0") {
				t.Errorf("%s/%s: argv %v lacks the object or aspect band", name, p.Label(), inv.Argv)
			}
			// Chart 1 is the baseline: staged first.
			if len(inv.TempInputs) != 2 || inv.TempInputs[0].ArgIndex != 1 {
				t.Errorf("%s/%s: staged inputs %+v", name, p.Label(), inv.TempInputs)
			}
		}
	}
	// And today's overlay stays -rt.
	spec, err := lens.comparisonSpecAt(jd1, a.place, jd2, b.place)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := astrolog.Compile(spec, recipe.Aspects{})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Invocations[0].Argv[0] != "-rt" {
		t.Errorf("overlay argv %v, want -rt", plan.Invocations[0].Argv)
	}
}

// The N-ring spec stages the set in numeral order and carries the band.
func TestMultiWheelSpecStagesNumeralOrderWithTheBand(t *testing.T) {
	var jds []float64
	var places []location.PlaceInfo
	for _, n := range []string{"F1", "F3", "F4"} {
		f := fixtureByName(t, n)
		jds, places = append(jds, astrolog.GregorianToJD(f.when)), append(places, f.place)
	}
	spec, err := chosenLens().multiWheelSpecAt(jds, places)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := astrolog.Compile(spec, recipe.WheelImage{Render: recipe.DefaultWheelRender()})
	if err != nil {
		t.Fatal(err)
	}
	inv := plan.Invocations[0]
	if got := strings.Join(inv.Argv[:7], " "); got != "-i  -i2  -i3  -r3" {
		t.Errorf("argv head %q", got)
	}
	if !hasTok(inv.Argv, "-R0") || !hasTok(inv.Argv, "-RA0") {
		t.Errorf("argv %v lacks the chooser band", inv.Argv)
	}
	if len(inv.TempInputs) != 3 {
		t.Fatalf("%d staged charts", len(inv.TempInputs))
	}
	// Ring order is the order given: chart 1's file carries chart 1's year.
	for i, n := range []string{"F1", "F3", "F4"} {
		f := fixtureByName(t, n)
		if want := "\n-qj " + jdWord(f.when) + "\n"; !strings.Contains(inv.TempInputs[i].Content, want) {
			t.Errorf("ring %d (%s) staged without its moment %q: %q", i+1, n, want, inv.TempInputs[i].Content)
		}
	}
}

func TestRenderMultiWheelImageRefusesBadSets(t *testing.T) {
	f := fixtureByName(t, "F1")
	ring := WheelRing{When: f.when, Place: f.place}
	for n, rings := range map[string][]WheelRing{
		"none": nil, "one": {ring}, "seven": {ring, ring, ring, ring, ring, ring, ring},
		"untimed ring": {ring, ring, {When: f.when, Place: location.PlaceInfo{Name: "Nowhere"}}},
	} {
		if _, err := RenderMultiWheelImage(context.Background(), rings, Hellenistic); err == nil {
			t.Errorf("%s: want an error", n)
		}
	}
}

// Live: three rings draw two separator circles; a set of two is today's
// bi-wheel, byte for byte what RenderOverlayWheelImage draws.
func TestRenderMultiWheelImageAgainstLiveAstrolog(t *testing.T) {
	requireAstrolog(t)
	t.Setenv("SEXTILE_PREVIEW_MODE", "http")
	var rings []WheelRing
	for _, n := range []string{"F1", "F3", "F4"} {
		f := fixtureByName(t, n)
		rings = append(rings, WheelRing{When: f.when, Place: f.place})
	}
	three, err := RenderMultiWheelImage(context.Background(), rings, Hellenistic)
	if err != nil {
		t.Fatalf("3 rings: %v", err)
	}
	group := regexp.MustCompile(`(?s)<g style="stroke:#007f00">(.*?)</g>`).FindSubmatch(three)
	if group == nil || bytes.Count(group[1], []byte("<circle")) != 2 {
		t.Errorf("3 rings: want two #007f00 separator circles, group = %q", group)
	}
	pair, err := RenderMultiWheelImage(context.Background(), rings[:2], Hellenistic)
	if err != nil {
		t.Fatalf("2 rings: %v", err)
	}
	overlay, err := RenderOverlayWheelImage(context.Background(), store.AnchorChart{When: rings[0].When, Place: rings[0].Place}, rings[1].When, rings[1].Place, Hellenistic)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(pair, overlay) {
		t.Error("a set of two is not today's bi-wheel")
	}
}

// Live: the synastry producers run -r0 (not -rt), honour the body choice, and
// restrict the body report to its body.
func TestSynastryProducersAgainstLiveAstrolog(t *testing.T) {
	requireAstrolog(t)
	a, b := fixtureByName(t, "F10a"), fixtureByName(t, "F3")
	chart1 := store.AnchorChart{When: a.when, Place: a.place}
	lens := Hellenistic
	lens.Objects = recipe.ObjectPolicy{Baseline: recipe.Explicit, Selection: []int{1, 3, 4, 5, 6, 7, 8, 9, 10}, TrueNodeMode: true}

	cross, err := SynastryCrossAspects(context.Background(), lens, chart1, b.when, b.place, store.ReportLabel{})
	if err != nil {
		t.Fatalf("SynastryCrossAspects: %v", err)
	}
	if len(cross.Aspects) == 0 {
		t.Fatal("no synastry aspects")
	}
	last := cross.Commands[len(cross.Commands)-1]
	if !strings.Contains(last, " -r0 ") || strings.Contains(last, "-rt") {
		t.Errorf("synastry ran %q, want -r0 and no -rt", last)
	}
	if regexp.MustCompile(`\bMoon\b`).MatchString(cross.Report) {
		t.Errorf("synastry report names the unticked Moon:\n%s", cross.Report)
	}
	for _, x := range cross.Aspects {
		if x.Natal == aspect.Moon || x.Transit == aspect.Moon {
			t.Errorf("a typed row names the unticked Moon: %+v", x)
		}
	}
	// Chart 1 is Primary: a Sun row's Natal side is chart 1's Sun, and the
	// -rt overlay differs from synastry for the same two charts.
	overlay, err := OverlayCrossAspects(context.Background(), lens, chart1, b.when, b.place)
	if err != nil {
		t.Fatal(err)
	}
	if overlay.Report == cross.Report {
		t.Error("the overlay (-rt) report equals the synastry (-r0) report")
	}

	raw, command, err := SynastryCrossBodyRawReport(context.Background(), lens, chart1, b.when, b.place, store.ReportLabel{}, aspect.Sun)
	if err != nil {
		t.Fatalf("SynastryCrossBodyRawReport: %v", err)
	}
	if !strings.Contains(command, " -r0 ") || strings.Contains(command, "-rt") || !strings.Contains(command, "-RO") {
		t.Errorf("body report ran %q", command)
	}
	rows := 0
	for _, line := range strings.Split(raw, "\n") {
		if aspectRowRE.MatchString(line) {
			rows++
			if !strings.Contains(line, "Sun") {
				t.Errorf("row without the restricted body: %q", line)
			}
		}
	}
	if rows == 0 {
		t.Errorf("no rows in the synastry body report:\n%s", raw)
	}
}

func TestSynastryProducersRefuseUntimedPlaces(t *testing.T) {
	f := fixtureByName(t, "F1")
	timed := store.AnchorChart{When: f.when, Place: f.place}
	bare := location.PlaceInfo{Name: "Nowhere"}
	if _, _, err := SynastryCrossBodyRawReport(context.Background(), Hellenistic, timed, f.when, bare, store.ReportLabel{}, aspect.Sun); err == nil {
		t.Error("an untimed second place was accepted")
	}
	if _, _, err := SynastryCrossBodyRawReport(context.Background(), Hellenistic, store.AnchorChart{When: f.when, Place: bare}, f.when, f.place, store.ReportLabel{}, aspect.Sun); err == nil {
		t.Error("an untimed first place was accepted")
	}
}
