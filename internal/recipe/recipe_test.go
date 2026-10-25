package recipe

import (
	"testing"

	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// TestNewChartSpec_RejectsUnknownBody: aspect.Body is a uint8, so an
// out-of-range value is constructible. The policy must reject it, so a
// validated spec is always compilable (no body without an astrolog number).
func TestNewChartSpec_RejectsUnknownBody(t *testing.T) {
	if _, err := NewChartSpec(
		MomentAtPlace{JD: 2446232.0, Lon: 0, Lat: 0},
		WholeSign, Tropical,
		ObjectPolicy{Baseline: AstrologDefault, Add: NewBodySet(aspect.Body(99))},
	); err == nil {
		t.Error("Add with an out-of-range body must be rejected")
	}
}

// TestObjectPolicy_AddRemoveValidation: Add must be outside the baseline,
// Remove must be inside it.
func TestObjectPolicy_AddRemoveValidation(t *testing.T) {
	moment := MomentAtPlace{JD: 1, Lon: 0, Lat: 0}
	if _, err := NewChartSpec(moment, WholeSign, Tropical,
		ObjectPolicy{Baseline: AstrologDefault, Add: NewBodySet(aspect.Sun)}); err == nil {
		t.Error("Add of a baseline body (Sun) must be rejected")
	}
	if _, err := NewChartSpec(moment, WholeSign, Tropical,
		ObjectPolicy{Baseline: AstrologDefault, Remove: NewBodySet(aspect.Chiron)}); err == nil {
		t.Error("Remove of a non-baseline body (Chiron) must be rejected")
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
		{"body", BodyRawReport(aspect.Moon), true},
		{"aspects", AspectsRawReport(), true},
		{"zero", RawReport{}, false},
		{"unknown body", BodyRawReport(aspect.BodyUnknown), false},
		{"out-of-range body", BodyRawReport(aspect.Body(99)), false},
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
