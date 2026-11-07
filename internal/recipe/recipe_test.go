package recipe

import (
	"testing"

	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// TestNewChartSpec_RejectsNegativeObjectNumber: an object number is 0 or more
// (object 0 is Earth). The policy must reject a negative one in Add and in
// Remove, so a validated spec only ever names objects Astrolog can number.
func TestNewChartSpec_RejectsNegativeObjectNumber(t *testing.T) {
	moment := MomentAtPlace{JD: 2446232.0, Lon: 0, Lat: 0}
	if _, err := NewChartSpec(moment, WholeSign, Tropical,
		ObjectPolicy{Baseline: AstrologDefault, Add: NewObjectSet(-1)}); err == nil {
		t.Error("Add of object -1 must be rejected")
	}
	if _, err := NewChartSpec(moment, WholeSign, Tropical,
		ObjectPolicy{Baseline: AstrologDefault, Remove: NewObjectSet(-1)}); err == nil {
		t.Error("Remove of object -1 must be rejected")
	}
}

// TestObjectPolicy_AddRemoveValidation: the checks are structural. An object in
// both Add and Remove is refused; an Add the host already lists (the Sun) and a
// Remove it does not (Chiron) are not, because the policy holds no list of the
// host's default set (slice 8c: each draws what it says, whatever the host lists).
func TestObjectPolicy_AddRemoveValidation(t *testing.T) {
	moment := MomentAtPlace{JD: 1, Lon: 0, Lat: 0}
	if _, err := NewChartSpec(moment, WholeSign, Tropical,
		ObjectPolicy{Baseline: AstrologDefault, Add: NewObjectSet(11), Remove: NewObjectSet(11)}); err == nil {
		t.Error("an object in both Add and Remove must be rejected")
	}
	if _, err := NewChartSpec(moment, WholeSign, Tropical,
		ObjectPolicy{Baseline: AstrologDefault, Add: NewObjectSet(1)}); err != nil { // the Sun
		t.Errorf("Add of the Sun is structurally fine, got %v", err)
	}
	if _, err := NewChartSpec(moment, WholeSign, Tropical,
		ObjectPolicy{Baseline: AstrologDefault, Remove: NewObjectSet(11)}); err != nil { // Chiron
		t.Errorf("Remove of Chiron is structurally fine, got %v", err)
	}
}

// TestWheelImage_Validate: a zero render is rejected; DefaultWheelRender passes.
func TestWheelImage_Validate(t *testing.T) {
	if err := (WheelImage{}).Validate(); err == nil {
		t.Error("zero-value WheelImage (zero render) must be rejected")
	}
	if err := (WheelImage{Render: DefaultWheelRender()}).Validate(); err != nil {
		t.Errorf("DefaultWheelRender must validate, got %v", err)
	}
}

func TestRawReport_Validate(t *testing.T) {
	cases := []struct {
		name string
		r    RawReport
		ok   bool
	}{
		{"chart", ChartRawReport(), true},
		{"body", BodyRawReport(aspect.Object{Number: 2, Name: "Moon"}), true},
		{"aspects", AspectsRawReport(), true},
		{"zero", RawReport{}, false},
		{"zero object", BodyRawReport(aspect.Object{}), false},
	}
	for _, tc := range cases {
		if err := tc.r.Validate(); (err == nil) != tc.ok {
			t.Errorf("%s Validate error = %v, want ok=%v", tc.name, err, tc.ok)
		}
	}
}

// TestNewChartSpec_RejectsBadInputs spot-checks the moment/house/zodiac guards.
func TestNewChartSpec_RejectsBadInputs(t *testing.T) {
	good := ObjectPolicy{Baseline: AstrologDefault}
	cases := []struct {
		name   string
		source Source
		houses HouseSystem
		zodiac Zodiac
	}{
		{"zero JD", MomentAtPlace{JD: 0, Lon: 0, Lat: 0}, WholeSign, Tropical},
		{"lat out of range", MomentAtPlace{JD: 1, Lon: 0, Lat: 91}, WholeSign, Tropical},
		{"unknown house", MomentAtPlace{JD: 1, Lon: 0, Lat: 0}, HouseSystem(99), Tropical},
		{"unknown zodiac", MomentAtPlace{JD: 1, Lon: 0, Lat: 0}, WholeSign, Zodiac(99)},
		{"nil source", nil, WholeSign, Tropical},
	}
	for _, c := range cases {
		if _, err := NewChartSpec(c.source, c.houses, c.zodiac, good); err == nil {
			t.Errorf("%s: expected rejection", c.name)
		}
	}
}

func multiWheelRings(n int) []Source {
	rings := make([]Source, n)
	for i := range rings {
		rings[i] = MomentAtPlace{JD: 2446232.0 + float64(i), Lon: 11.5667, Lat: 48.1333}
	}
	return rings
}

// TestMultiWheelValidatesRingCount: three to six rings are a MultiWheel; a pair
// stays a Comparison and seven rings do not exist.
func TestMultiWheelValidatesRingCount(t *testing.T) {
	for n := 0; n <= 8; n++ {
		_, err := NewChartSpec(MultiWheel{Rings: multiWheelRings(n)}, WholeSign, Tropical,
			ObjectPolicy{Baseline: AstrologDefault})
		want := n >= MinRings && n <= MaxRings
		if (err == nil) != want {
			t.Errorf("%d rings: err=%v, want valid=%v", n, err, want)
		}
	}
}

// TestMultiWheelRejectsNestedAndBadRings: a ring is one moment, never a
// comparison or another multi-wheel, and each ring is validated.
func TestMultiWheelRejectsNestedAndBadRings(t *testing.T) {
	obj := ObjectPolicy{Baseline: AstrologDefault}
	cmp := Comparison{Primary: MomentAtPlace{JD: 1, Lon: 0, Lat: 0}, Secondary: MomentAtPlace{JD: 2, Lon: 0, Lat: 0}}
	for name, bad := range map[string]Source{
		"comparison ring":  cmp,
		"multiwheel ring":  MultiWheel{Rings: multiWheelRings(3)},
		"invalid moment":   MomentAtPlace{JD: 0, Lon: 0, Lat: 0},
		"nil ring":         nil,
		"local without tz": LocalMomentAtPlace{JD: 1, Lon: 0, Lat: 0},
	} {
		rings := multiWheelRings(3)
		rings[1] = bad
		if _, err := NewChartSpec(MultiWheel{Rings: rings}, WholeSign, Tropical, obj); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
	// And a Comparison cannot carry a MultiWheel as a sub-source.
	if _, err := NewChartSpec(Comparison{Primary: MultiWheel{Rings: multiWheelRings(3)}, Secondary: MomentAtPlace{JD: 2, Lon: 0, Lat: 0}},
		WholeSign, Tropical, obj); err == nil {
		t.Error("a MultiWheel inside a Comparison must be rejected")
	}
}

// TestBodyRawReportTakesAnyObject: a body report is for any object Astrolog's
// registry names (Fortune, a fixed star, a cusp point), not only the ones Sextile has glyphs for;
// only the zero Object is refused, and the other kinds refuse an object.
func TestBodyRawReportTakesAnyObject(t *testing.T) {
	for _, o := range []aspect.Object{
		{Number: 19, Name: "Fortune"},
		{Number: 23, Name: "2nd Cusp"},
		{Number: 60, Name: "Algol"},
	} {
		r := BodyRawReport(o)
		if err := r.Validate(); err != nil {
			t.Errorf("BodyRawReport(%v) refused: %v", o, err)
		}
		if r.Object() != o {
			t.Errorf("Object() = %v, want %v", r.Object(), o)
		}
	}
	if err := BodyRawReport(aspect.Object{}).Validate(); err == nil {
		t.Error("the zero Object was accepted for a body report")
	}
	if err := (RawReport{kind: RawReportChart, object: aspect.Object{Number: 19, Name: "Fortune"}}).Validate(); err == nil {
		t.Error("a chart report accepted an object")
	}
}
