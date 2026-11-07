package astrolog

import (
	"strings"
	"testing"

	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// parse_unnamed_test.go covers the positions parser carrying objects Sextile
// has no glyph for.
//
// It exists because of a live failure, not a hypothetical: with the body
// selector wired but the parser unchanged, ticking Ceres logged "bodies: 19
// selected", put `-R0 … 12 …` in the argv, and showed eighteen rows in the
// Bodies panel. The object was computed and then silently dropped between
// Astrolog and the screen — a selector indistinguishable from a broken one.

const unnamedPositionsFixture = `Astrolog 8.00 chart for Sat Jun 15, 1985 12:00pm (ST Zone 0W)  11:34E 48:08N
Body  Locat. Ret. Lati. Rul.      House  Rul. Veloc.    Whole Houses

Sun : 24Gem19   - 0:00' (-) [ 9th house] [-] +0.969  -  House cusp  1:  0Lib00
Moon: 20Tau38   + 0:15' (X) [ 8th house] [f] +0.905  -  House cusp  2:  0Sco00
Cere:  2Can23   + 2:00' (-) [10th house] [-] +2.009  -  House cusp  3:  0Sag00
Vert:  9Ari22   + 0:00' (d) [ 7th house] [R] +1.764
`

func TestParsePositions_KeepsCeresAndTheVertex(t *testing.T) {
	ps := mustPositions(t, unnamedPositionsFixture, 1, 2, 12, 20)
	if len(ps) != 4 {
		t.Fatalf("parsed %d positions, want 4 — an object with no glyph must be kept, not dropped", len(ps))
	}

	byName := map[string]aspect.Position{}
	for _, p := range ps {
		byName[p.Object.Name] = p
	}
	ceres, ok := byName["Ceres"]
	if !ok {
		t.Fatal("Ceres was dropped; this is the exact failure the body selector exposed")
	}
	// The position itself must be real: a row that renders in the wrong place
	// is worse than one that does not render.
	if want := 90.0 + 2 + 23.0/60; ceres.Longitude < want-0.01 || ceres.Longitude > want+0.01 {
		t.Errorf("Ceres longitude = %v, want ~%v (2Can23)", ceres.Longitude, want)
	}
	if ceres.House != aspect.House(10) {
		t.Errorf("Ceres house = %v, want 10", ceres.House)
	}
	if ceres.Speed <= 0 {
		t.Errorf("Ceres speed = %v, want the printed +2.009", ceres.Speed)
	}
	if _, ok := byName["Vertex"]; !ok {
		t.Error("the Vertex was dropped; it is another object Sextile has no glyph for")
	}
}

// Every parsed row, whatever the object, carries Astrolog's Object: the one
// source of truth for who a row is.
func TestParsePositions_EveryRowCarriesItsObject(t *testing.T) {
	want := []aspect.Object{{Number: 1, Name: "Sun"}, {Number: 2, Name: "Moon"}, {Number: 12, Name: "Ceres"}, {Number: 20, Name: "Vertex"}}
	ps := mustPositions(t, unnamedPositionsFixture, 1, 2, 12, 20)
	if len(ps) != len(want) {
		t.Fatalf("parsed %d rows, want %d", len(ps), len(want))
	}
	for i, p := range ps {
		if p.Object != want[i] {
			t.Errorf("row %d Object = %+v, want %+v", i, p.Object, want[i])
		}
	}
}

// A number the registry does not list is the call's error: the row has no
// identity to carry.
func TestPositionsRefuseANumberOffTheList(t *testing.T) {
	var short aspect.Registry
	for _, o := range testRegistry(t) {
		if o.Number != 12 {
			short = append(short, o)
		}
	}
	_, err := parsePositions(unnamedPositionsFixture, []int{1, 2, 12, 20}, short)
	if err == nil || !strings.Contains(err.Error(), "object 12 is not in Astrolog's object list") {
		t.Fatalf("err = %v, want object 12 is not in Astrolog's object list", err)
	}
}

// --- objects the installation cannot compute ------------------------------

// noDataFixture is what Astrolog returns for an object it has no Swiss
// Ephemeris file for, beside two that it does. The sentinel is all three fields
// exactly zero: longitude 0Ari00, latitude 0:00', velocity +0.000.
const noDataFixture = `Astrolog 8.00 chart for Sat Jun 15, 1985 12:00pm (ST Zone 0W)  11:34E 48:08N
Body  Locat. Ret. Lati. Rul.      House  Rul. Veloc.    Whole Houses

Sun : 24Gem19   - 0:00' (-) [ 9th house] [-] +0.969  -  House cusp  1:  0Lib00
Asce:  3Lib52   + 0:00'     [ 1st house]     +0.734  -  House cusp  7:  0Ari00
Eris:  0Ari00   + 0:00' (d) [ 7th house] [R] +0.000  -  House cusp  8:  0Tau00
Io  :  0Ari00   + 0:00' (-) [ 7th house] [-] +0.000
`

// An object with no ephemeris is DROPPED, not drawn at 0° Aries. Operator
// decision, 2026-09-17: a row reading `Eris Ari 0°00'` is read by an
// astrologer as a real position, and it is not one.
func TestParsePositions_DropsObjectsWithNoEphemeris(t *testing.T) {
	ps := mustPositions(t, noDataFixture, 1, 22, 45, 56)
	if len(ps) != 2 {
		t.Fatalf("parsed %d positions, want 2 — Eris and Io have no ephemeris and must be dropped", len(ps))
	}
	for _, p := range ps {
		if p.Longitude == 0 && p.Speed == 0 && p.Latitude == 0 {
			t.Errorf("%q survived at the no-data sentinel", p.Object.Name)
		}
	}
}

// The Ascendant carries a zero latitude and a real velocity; the drop must not
// take it. This is the row a looser predicate loses first.
func TestParsePositions_KeepsAnglesWithZeroLatitude(t *testing.T) {
	var found bool
	for _, p := range mustPositions(t, noDataFixture, 1, 22, 45, 56) {
		if p.Object == objAscendant {
			found = true
		}
	}
	if !found {
		t.Error("the Ascendant was dropped; it has a zero latitude but a real position and a real velocity")
	}
}

// Dropping silently is the failure this replaced, so the names must survive for
// the perimeter to report.
func TestUnavailableObjectsNamesWhatWasDropped(t *testing.T) {
	got := unavailableObjects(noDataFixture)
	if len(got) != 2 || got[0] != "Eris" || got[1] != "Io" {
		t.Fatalf("unavailableObjects = %v, want [Eris Io] in Astrolog's own order", got)
	}
	if n := unavailableObjects(unnamedPositionsFixture); len(n) != 0 {
		t.Errorf("unavailableObjects on a healthy chart = %v, want none", n)
	}
}
