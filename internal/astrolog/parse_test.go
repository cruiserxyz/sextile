package astrolog

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// Fixture: astrolog -qa 3 21 1985 14:30 0 0:10W 51:30N, the natal cast the day after
// the 1985 vernal equinox at Greenwich (Sun at 0 Aries 55).
// The positions file is the default chart output; the aspects file is the
// same chart with -a appended.

func TestParsePositions(t *testing.T) {
	data := readFixture(t, "natal_1985_positions.txt")
	positions := parsePositions(data)

	// Default astrolog output: the classical ten + North Node + the two
	// angles emitted by default (Ascendant, Midheaven) — thirteen bodies.
	// (Slice B1 stopped dropping Asce/Midh.)
	if got, want := len(positions), 13; got != want {
		t.Fatalf("parsePositions returned %d positions, want %d", got, want)
	}

	// Angles are now first-class (Slice B1): Ascendant in the 1st house,
	// Midheaven in the 10th, per the fixture.
	asc := findPosition(t, positions, aspect.Ascendant)
	if asc.House != 1 {
		t.Errorf("Ascendant.House = %d, want 1", asc.House)
	}
	mc := findPosition(t, positions, aspect.Midheaven)
	if mc.House != 10 {
		t.Errorf("Midheaven.House = %d, want 10", mc.House)
	}

	// Sun at 0°Ari55' — vernal-equinox baseline, non-retrograde.
	sun := findPosition(t, positions, aspect.Sun)
	const sunWant = 0.0 + 55.0/60.0
	if math.Abs(sun.Longitude-sunWant) > 0.02 {
		t.Errorf("Sun longitude = %v, want ~%v", sun.Longitude, sunWant)
	}
	if sun.Retrograde() {
		t.Error("Sun must not be retrograde")
	}

	// Venus: signed velocity -0.320 — retrograde.
	venus := findPosition(t, positions, aspect.Venus)
	if !venus.Retrograde() {
		t.Errorf("Venus retrograde = false, want true (velocity %v)", venus.Speed)
	}

	// Jupiter in Aquarius — longitude between 300° and 330°.
	jupiter := findPosition(t, positions, aspect.Jupiter)
	if jupiter.Longitude < 300 || jupiter.Longitude >= 330 {
		t.Errorf("Jupiter longitude %v, expected in [300, 330) for Aquarius", jupiter.Longitude)
	}

	// Houses: per-body house assignment is location-dependent and is the
	// primary reason a chart visibly differs between cities (geocentric
	// longitudes don't shift with observer location). Astrolog emits the
	// house inline as "[ Nth house]"; the parser must capture it. cp3.
	if sun.House != 8 {
		t.Errorf("Sun.House = %d, want 8 (fixture reports '[ 8th house]')", sun.House)
	}
	moon := findPosition(t, positions, aspect.Moon)
	if moon.House != 8 {
		t.Errorf("Moon.House = %d, want 8 (fixture reports '[ 8th house]')", moon.House)
	}
	merc := findPosition(t, positions, aspect.Mercury)
	if merc.House != 9 {
		t.Errorf("Mercury.House = %d, want 9 (fixture reports '[ 9th house]')", merc.House)
	}
}

// TestParsePositions_HouseNumberOrdinals covers the four ordinal-suffix
// variants astrolog emits ("1st", "2nd", "3rd", "Nth") to make sure the
// regex doesn't get fooled by a single one of them.
func TestParsePositions_HouseNumberOrdinals(t *testing.T) {
	fixture := strings.Join([]string{
		"Sun :  0Ari55   - 0:00' (X) [ 1st house] [-] +0.993  -  House cusp  1:  0Ari00",
		"Moon:  2Ari04   - 3:45' (-) [ 2nd house] [-] +11.93  -  House cusp  2:  0Tau00",
		"Merc: 17Ari59   + 2:57' (-) [ 3rd house] [-] +0.421  -  House cusp  3:  0Gem00",
		"Venu: 21Ari02 R + 7:55' (-) [11th house] [-] -0.320  -  House cusp  4:  0Can00",
		"Mars: 15Ari13   - 0:53' (R) [12th house] [-] +1.463  -  House cusp  5:  0Leo00",
	}, "\n")
	positions := parsePositions(fixture)
	if len(positions) != 5 {
		t.Fatalf("parsed %d positions, want 5", len(positions))
	}
	want := map[aspect.Body]aspect.House{
		aspect.Sun:     1,
		aspect.Moon:    2,
		aspect.Mercury: 3,
		aspect.Venus:   11,
		aspect.Mars:    12,
	}
	for _, p := range positions {
		if p.House != want[p.Body] {
			t.Errorf("%v.House = %d, want %d", p.Body, p.House, want[p.Body])
		}
	}
}

// the specification rule 2: the Bodies panel recognises a ticked
// plain cusp by the label Astrolog prints for its object row. Rows captured
// from Astrolog 8.00 (-R0 with the cusp objects toggled on).
func TestParsePositions_PlainCuspObjectLabels(t *testing.T) {
	fixture := strings.Join([]string{
		"2nd :  3Aqu58   + 0:00'     [ 2nd house]     +452.5  -  House cusp  1:  4Cap47",
		"3rd : 13Pis26   + 0:00'     [ 3rd house]     +599.9  -  House cusp  2:  3Aqu58",
		"5th : 19Tau38   + 0:00'     [ 5th house]     +380.1  -  House cusp  3: 13Pis26",
		"6th : 11Gem11   + 0:00'     [ 6th house]     +300.2  -  House cusp  4: 28Ari28",
		"8th :  3Leo58   + 0:00'     [ 8th house]     +452.5  -  House cusp  5: 19Tau38",
		"9th : 13Vir26   + 0:00'     [ 9th house]     +599.9  -  House cusp  6: 11Gem11",
		"11th: 19Sco38   + 0:00'     [11th house]     +380.1  -  House cusp  7:  4Can47",
		"12th: 11Sag11   + 0:00'     [12th house]     +297.5  -  House cusp  8:  3Leo58",
	}, "\n")
	positions := parsePositions(fixture)
	want := []string{"2nd", "3rd", "5th", "6th", "8th", "9th", "11th", "12th"}
	if len(positions) != len(want) {
		t.Fatalf("parsed %d rows, want %d", len(positions), len(want))
	}
	for i, p := range positions {
		if p.Body != aspect.BodyUnknown || p.Label != want[i] {
			t.Errorf("row %d = (%v, %q), want (BodyUnknown, %q)", i, p.Body, p.Label, want[i])
		}
	}
}

func TestParseAspects(t *testing.T) {
	data := readFixture(t, "natal_1985_positions_aspects.txt")
	aspects := parseAspects(data)

	// The fixture reports 22 aspects; several involve Ascendant or Midheaven
	// and drop out. Expect at least ten retained aspects between
	// enum-recognised bodies.
	if got := len(aspects); got < 10 {
		t.Fatalf("parseAspects returned %d aspects, want >= 10", got)
	}

	// The fixture's top-ranked aspect is a Sun-Moon conjunction — the
	// chart was captured at a New Moon by design. Verify it survives
	// canonicalisation through aspect.NewAspect.
	want := aspect.NewAspect(aspect.Sun, aspect.Moon, aspect.Conjunction, 0)
	var found aspect.Aspect
	var ok bool
	for _, a := range aspects {
		if a.A == want.A && a.B == want.B && a.Kind == want.Kind {
			found, ok = a, true
			break
		}
	}
	if !ok {
		t.Fatal("expected a Sun-Moon conjunction in parsed aspects")
	}
	// D1: the aspect power column is parsed (it drives the waterfall sort). The
	// fixture's Sun-Moon conjunction reports "power: 23.99".
	if found.Power < 23.9 || found.Power > 24.1 {
		t.Errorf("Sun-Moon conjunction power = %v, want ~23.99", found.Power)
	}
}

func readFixture(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return string(data)
}

func findPosition(t *testing.T, ps []aspect.Position, body aspect.Body) aspect.Position {
	t.Helper()
	for _, p := range ps {
		if p.Body == body {
			return p
		}
	}
	t.Fatalf("no Position for body %v in %d parsed positions", body, len(ps))
	return aspect.Position{}
}

// TestParseAspects_PhaseMarkers: under -aa Astrolog prints the orb as
// `a`/`s`; the parser reads it into Applying. Plain -a lines (+/- offsets)
// still parse, with Applying false.
func TestParseAspects_PhaseMarkers(t *testing.T) {
	out := parseAspects(`
  1:   Venus (Tau) Squ (Leo) Midheaven  - orb: a0:20' - power:  9.51
  2:    Moon (Pis) Tri (Can) Jupiter    - orb: s1:23' - power:  8.68
  3:    Moon (Pis) Sex (Tau) Venus      - orb: +1:38' - power:  8.04
`)
	if len(out) != 3 {
		t.Fatalf("parsed %d aspects, want 3 (Midheaven is kept since 2026-10-05)", len(out))
	}
	byKind := map[aspect.AspectKind]aspect.Aspect{}
	for _, a := range out {
		byKind[a.Kind] = a
	}
	if a := byKind[aspect.Square]; !a.Applying || a.A != aspect.Venus || a.B != aspect.Midheaven {
		t.Errorf("Venus Squ Midheaven marked `a` = %+v, want applying Venus/Midheaven", a)
	}
	if a := byKind[aspect.Trine]; a.Applying {
		t.Errorf("Moon Tri Jupiter marked `s` parsed as applying: %+v", a)
	}
	if a := byKind[aspect.Sextile]; a.Applying {
		t.Errorf("plain +orb line parsed as applying: %+v", a)
	}
	single := parseAspects("  6:     Sun (Gem) Squ (Pis) Moon       - orb: a6:57' - power:  4.99 (Half Moon)\n")
	if len(single) != 1 || !single[0].Applying {
		t.Errorf("Sun Squ Moon marked `a` = %+v, want applying", single)
	}
}

// TestParseCrossAspects_PhaseMarkers: the -rt -aa relationship list
// carries the same `a`/`s` marker; roles stay positional.
func TestParseCrossAspects_PhaseMarkers(t *testing.T) {
	out := parseCrossAspects(`
Chart #2 comparison for Mon Aug 24, 2026 12:00pm (ST Zone 0W)  13:24E 52:31N

  1:    Moon (Pis) Opp with (Vir) Midheaven - orb: a2:18' - power: 11.37
  2:    Moon (Pis) Sex with (Cap) Moon      - orb: s2:05' - power: 11.07
  4: Mercury (Gem) Con with (Gem) Uranus    - orb: a0:22' - power:  8.52
`)
	if len(out) != 3 {
		t.Fatalf("parsed %d cross-aspects, want 3", len(out))
	}
	if !out[0].Applying || out[0].Primary != aspect.Moon || out[0].Secondary != aspect.Midheaven {
		t.Errorf("row 1 = %+v, want natal Moon / transit Midheaven applying", out[0])
	}
	if out[1].Applying {
		t.Errorf("row 2 = %+v, want separating", out[1])
	}
	if !out[2].Applying || out[2].Primary != aspect.Mercury || out[2].Secondary != aspect.Uranus || out[2].Power != 8.52 {
		t.Errorf("row 4 = %+v, want natal Mercury / transit Uranus applying, power 8.52", out[2])
	}
}

// parseCusps must recover the chart's house frame from the same stdout
// parsePositions reads — no second invocation, no extra switch.
func TestParseCusps(t *testing.T) {
	cusps := parseCusps(readFixture(t, "natal_1985_positions.txt"))
	if !cusps.Valid() {
		t.Fatalf("parseCusps returned an invalid frame: %v", cusps)
	}
	for i, c := range cusps {
		if c < 0 || c >= 360 {
			t.Errorf("cusp %d out of range: %.4f", i+1, c)
		}
	}
}

// The strongest check available: the frame we parsed, run through
// Cusps.HouseFor, must reproduce the house Astrolog itself printed beside
// every body in the same output. If cusp parsing, longitude reconstruction or
// the boundary rule is wrong, this disagrees with the ephemeris rather than
// merely with our own expectations.
func TestParseCusps_ReproducesAstrologHouseColumn(t *testing.T) {
	out := readFixture(t, "natal_1985_positions.txt")
	cusps := parseCusps(out)
	if !cusps.Valid() {
		t.Fatal("parseCusps returned an invalid frame")
	}
	positions := parsePositions(out)
	if len(positions) == 0 {
		t.Fatal("no positions parsed")
	}
	checked := 0
	for _, p := range positions {
		if !p.House.Valid() {
			continue
		}
		checked++
		if got := cusps.HouseFor(p.Longitude); got != p.House {
			t.Errorf("body %d at %.4f: HouseFor = %d, Astrolog printed %d",
				p.Body, p.Longitude, got, p.House)
		}
	}
	if checked == 0 {
		t.Fatal("no bodies carried a house to check against")
	}
}

// A truncated frame is refused whole: a partial set would place some bodies
// plausibly and others wrongly, with nothing on screen to say which.
func TestParseCusps_PartialFrameRefused(t *testing.T) {
	if got := parseCusps("House cusp  1: 22Leo22\nHouse cusp  2: 11Vir10\n"); got.Valid() {
		t.Errorf("partial frame accepted: %v", got)
	}
}

// findAspect returns the parsed aspect pairing a and b (either order, since
// NewAspect canonicalises) with the given kind.
func findAspect(t *testing.T, got []aspect.Aspect, a, b aspect.Body, kind aspect.AspectKind) aspect.Aspect {
	t.Helper()
	want := aspect.NewAspect(a, b, kind, 0)
	for _, g := range got {
		if g.A == want.A && g.B == want.B && g.Kind == kind {
			return g
		}
	}
	t.Fatalf("no %v %v %v aspect in %d parsed aspects", a, kind, b, len(got))
	return aspect.Aspect{}
}

// TestParseAspectsKeepsAnglesLilithAndNodes: a real Astrolog 8 -aa list names
// the angles, Lilith and the nodes in full ("Ascendant", "South Node"); every
// such line used to be dropped, so the Aspects panel never showed an angle.
// Fixture: constructed moment 2461156.0 JD at 13:25E 52:31N, no person.
func TestParseAspectsKeepsAnglesLilithAndNodes(t *testing.T) {
	got := parseAspects(readFixture(t, "aspects_angles_lilith_nodes.txt"))
	if len(got) != 26 {
		t.Fatalf("parsed %d aspects, want all 26 lines of the fixture", len(got))
	}
	cases := []struct {
		a, b     aspect.Body
		kind     aspect.AspectKind
		orb      float64
		applying bool
	}{
		{aspect.Uranus, aspect.Ascendant, aspect.Square, 9.0 / 60, false},
		{aspect.Moon, aspect.Midheaven, aspect.Square, 3 + 24.0/60, true},
		{aspect.Lilith, aspect.ImumCoeli, aspect.Conjunction, 23.0 / 60, false},
		{aspect.Sun, aspect.SouthNode, aspect.Trine, 2 + 3.0/60, true},
		{aspect.Jupiter, aspect.Lilith, aspect.Trine, 48.0 / 60, false},
	}
	for _, c := range cases {
		g := findAspect(t, got, c.a, c.b, c.kind)
		if math.Abs(float64(g.Orb)-c.orb) > 1e-9 || g.Applying != c.applying {
			t.Errorf("%v %v %v = orb %v applying %v, want orb %v applying %v",
				c.a, c.kind, c.b, g.Orb, g.Applying, c.orb, c.applying)
		}
		if g.ALabel != "" || g.BLabel != "" {
			t.Errorf("%v %v %v carries labels %q/%q, want none for typed bodies", c.a, c.kind, c.b, g.ALabel, g.BLabel)
		}
	}
}

// TestParseAspectsKeepsGlyphlessBodies: a body the enum cannot name (Ceres,
// Vertex, East Point) is kept as BodyUnknown with the printed name, on the side
// it was printed. Fixture adds objects 12, 20 and 21 to the same moment.
func TestParseAspectsKeepsGlyphlessBodies(t *testing.T) {
	got := parseAspects(readFixture(t, "aspects_glyphless.txt"))
	byPair := func(a, b string) (aspect.Aspect, bool) {
		for _, g := range got {
			if (g.ALabel == a && g.BLabel == b) || (g.ALabel == b && g.BLabel == a) {
				return g, true
			}
		}
		return aspect.Aspect{}, false
	}
	// Ceres (Tau) Con (Tau) Midheaven: BodyUnknown is the zero Body, so it sorts
	// first; the label stays with the body that was printed as Ceres.
	g := findAspect(t, got, aspect.Midheaven, aspect.BodyUnknown, aspect.Conjunction)
	if g.A != aspect.BodyUnknown || g.ALabel != "Ceres" || g.B != aspect.Midheaven || g.BLabel != "" {
		t.Errorf("Ceres/Midheaven = %+v, want A unknown labelled Ceres, B Midheaven (no label)", g)
	}
	if g.Orb < 3 || g.Orb > 3.1 || g.Applying {
		t.Errorf("Ceres/Midheaven orb %v applying %v, want 3:05 separating", g.Orb, g.Applying)
	}
	// Chiron (Ari) Squ (Cap) Vertex: printed Chiron first, but the reorder
	// moves Vertex to A; its label must move with it.
	g = findAspect(t, got, aspect.Chiron, aspect.BodyUnknown, aspect.Square)
	if g.A != aspect.BodyUnknown || g.ALabel != "Vertex" || g.B != aspect.Chiron || g.BLabel != "" {
		t.Errorf("Chiron/Vertex = %+v, want Vertex label on A and typed Chiron on B", g)
	}
	// Two glyphless bodies, one a two-word name that abuts nothing.
	g, ok := byPair("Ceres", "East Point")
	if !ok || g.A != aspect.BodyUnknown || g.B != aspect.BodyUnknown {
		t.Fatalf("Ceres/East Point missing or typed: %+v ok=%v", g, ok)
	}
	if g.ALabel != "Ceres" || g.BLabel != "East Point" {
		t.Errorf("both-unknown sides = %q/%q, want printed order Ceres/East Point", g.ALabel, g.BLabel)
	}
	if len(got) != 36 {
		t.Errorf("parsed %d aspects, want every line of the fixture (36)", len(got))
	}
}

// TestCrossAspectsKeepGlyphlessBodies: the -r0 parser keeps an unnameable body
// by its printed name, left (Primary) and right (Secondary) as printed.
func TestCrossAspectsKeepGlyphlessBodies(t *testing.T) {
	got := parseCrossAspects(readFixture(t, "cross_glyphless.txt"))
	var vertexLeft, ceresRight, ceresLeft, eastRight bool
	for _, c := range got {
		switch {
		case c.PrimaryLabel == "Vertex" && c.Primary == aspect.BodyUnknown && c.Secondary == aspect.Sun && c.Kind == aspect.Trine:
			vertexLeft = true
		case c.SecondaryLabel == "Ceres" && c.Secondary == aspect.BodyUnknown && c.Primary == aspect.Mars && c.Kind == aspect.Square:
			ceresRight = true
		case c.PrimaryLabel == "Ceres" && c.Primary == aspect.BodyUnknown && c.Secondary == aspect.Jupiter && c.Kind == aspect.Square:
			ceresLeft = true
		case c.SecondaryLabel == "East Point" && c.Primary == aspect.Saturn && c.Kind == aspect.Square:
			eastRight = true
		}
		if c.Primary != aspect.BodyUnknown && c.PrimaryLabel != "" || c.Secondary != aspect.BodyUnknown && c.SecondaryLabel != "" {
			t.Errorf("typed side carries a label: %+v", c)
		}
	}
	if !vertexLeft || !ceresRight || !ceresLeft || !eastRight {
		t.Errorf("vertexLeft=%v ceresRight=%v ceresLeft=%v eastRight=%v, want all true", vertexLeft, ceresRight, ceresLeft, eastRight)
	}
	if err := checkCrossAspectSanity(readFixture(t, "cross_glyphless.txt")); err != nil {
		t.Errorf("sanity check on the fixture: %v", err)
	}
	lines := 0
	for _, l := range strings.Split(readFixture(t, "cross_glyphless.txt"), "\n") {
		if crossAspectLinePat.MatchString(l) {
			lines++
		}
	}
	if len(got) != lines {
		t.Errorf("parsed %d of %d aspect lines", len(got), lines)
	}
}

// TestPositionsKeepNegativeZero: a `-v0` row at Astrolog's station minute reads
// `R … -0.000`; the parser keeps the sign bit so the position is retrograde,
// and `+0.000` stays direct (the specification C11).
func TestPositionsKeepNegativeZero(t *testing.T) {
	fixture := strings.Join([]string{
		"Merc: 17Sco22 R - 3:07' (-) [ 3rd house] [R] -0.000  -  House cusp  3:  5Sco49",
		"Venu:  5Sco18   - 7:26' (d) [ 2nd house] [R] +0.000  -  House cusp  4: 11Sag06",
	}, "\n")
	positions := parsePositions(fixture)
	if len(positions) != 2 {
		t.Fatalf("parsed %d positions, want 2", len(positions))
	}
	if !findPosition(t, positions, aspect.Mercury).Retrograde() {
		t.Error("Mercury at -0.000 is not retrograde; Astrolog printed R")
	}
	if findPosition(t, positions, aspect.Venus).Retrograde() {
		t.Error("Venus at +0.000 is retrograde; Astrolog printed no R")
	}
}
