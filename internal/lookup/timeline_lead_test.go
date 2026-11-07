package lookup

import (
	"testing"

	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// obj is an object as Astrolog's registry numbers it (internal/astrolog/
// testdata/ho-all.txt); the lead rule reads only the number.
func obj(n int, name string) aspect.Object { return aspect.Object{Number: n, Name: name} }

var (
	oSun     = obj(1, "Sun")
	oMoon    = obj(2, "Moon")
	oMars    = obj(5, "Mars")
	oJupiter = obj(6, "Jupiter")
	oPluto   = obj(10, "Pluto")
	oCeres   = obj(12, "Ceres")
	oVesta   = obj(15, "Vesta")
	oStar1   = obj(60, "Alpha Star")
	oStar2   = obj(61, "Beta Star")
)

func TestFasterObjectFollowsTheRank(t *testing.T) {
	for _, tc := range []struct {
		name          string
		first, second aspect.Object
		want          aspect.Object
	}{
		{"Moon over Sun", oSun, oMoon, oMoon},
		{"Moon first stays", oMoon, oSun, oMoon},
		{"Vesta over Jupiter", oJupiter, oVesta, oVesta},
		{"Mars over Ceres", oCeres, oMars, oMars},
		{"Ceres over Pluto", oPluto, oCeres, oCeres},
		{"a tie keeps the first", oMars, oMars, oMars},
		{"an unlisted object loses to a listed one", oStar1, oPluto, oPluto},
		{"a listed object first stays against an unlisted one", oPluto, oStar1, oPluto},
		{"two unlisted objects keep the first", oStar1, oStar2, oStar1},
	} {
		if got := FasterObject(tc.first, tc.second); got != tc.want {
			t.Errorf("%s: FasterObject(%v, %v) = %v, want %v", tc.name, tc.first, tc.second, got, tc.want)
		}
	}
}

func TestLeadObjectIsTheTransitingOrFasterObject(t *testing.T) {
	sky := TimelineRow{Kind: TimelineKindAspect, Object: oSun, OtherObject: oMoon, Resolved: true}
	if o, ok := LeadObject(sky); !ok || o != oMoon {
		t.Errorf("sky Sun/Moon row leads to %v, %v; want Moon", o, ok)
	}
	chart := TimelineRow{Kind: TimelineKindAspect, Transit: true, Object: oMars, OtherObject: oMoon, Resolved: true}
	if o, ok := LeadObject(chart); !ok || o != oMars {
		t.Errorf("chart row leads to %v, %v; want the transiting Mars", o, ok)
	}
	ceres := TimelineRow{Kind: TimelineKindAspect, Object: oPluto, OtherObject: oCeres, Resolved: true}
	if o, ok := LeadObject(ceres); !ok || o != oCeres {
		t.Errorf("a Ceres sky row against Pluto leads to %v, %v; want Ceres", o, ok)
	}
	same := TimelineRow{Kind: TimelineKindAspect, Object: oMoon, OtherObject: oMoon, Resolved: true}
	if o, ok := LeadObject(same); !ok || o != oMoon {
		t.Errorf("a Moon/Moon sky row leads to %v, %v; want the Moon", o, ok)
	}
	ingress := TimelineRow{Kind: TimelineKindIngress, Object: oCeres, Resolved: true}
	if o, ok := LeadObject(ingress); !ok || o != oCeres {
		t.Errorf("a Ceres ingress leads to %v, %v; want Ceres", o, ok)
	}
	if _, ok := LeadObject(TimelineRow{Kind: TimelineKindAspect, Object: oSun, OtherObject: oMoon}); ok {
		t.Errorf("an unresolved row leads somewhere")
	}
}
