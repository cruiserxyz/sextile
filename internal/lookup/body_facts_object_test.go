package lookup

import (
	"context"
	"errors"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/cruiserxyz/sextile/internal/astrolog"
	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

func fixtureRegistry(t *testing.T) aspect.Registry {
	t.Helper()
	b, err := os.ReadFile("../astrolog/testdata/ho-all.txt")
	if err != nil {
		t.Fatal(err)
	}
	reg, err := astrolog.ParseObjectRegistry(string(b))
	if err != nil {
		t.Fatal(err)
	}
	return reg
}

// relabel is the Venus fixture's events with Venus (object 4) replaced by
// Ceres (object 12) everywhere an event names an object: the same sky read as
// a Sun-Ceres cycle, so only the Objects tell the facts which one it is.
func relabel(events []astrolog.CycleEvent, to aspect.Object) []astrolog.CycleEvent {
	out := append([]astrolog.CycleEvent(nil), events...)
	for i := range out {
		if out[i].Object.Number == 4 {
			out[i].Object = to
		}
		if out[i].OtherObject.Number == 4 {
			out[i].OtherObject = to
		}
	}
	return out
}

// The @now facts compute for an object with no glyph of its own (Ceres, 12):
// the conjunction helpers, the stations and the inner-conjunction list key on
// the events' Objects, and the Venus pair never matches Ceres events.
func TestSunPairFactsForCeres(t *testing.T) {
	reg := fixtureRegistry(t)
	ceres, _ := reg.ByNumber(12)
	when := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
	events := relabel(cycleFixture(t), ceres)
	pairFor := func(o aspect.Object) SunPairResult {
		return SunPairResult{
			Sun:  astrolog.PrecisePosition{Object: objSun, Longitude: 180 + 21 + 9.0/60 + 41.0/3600, Sign: "Lib", Speed: 0.99},
			Body: astrolog.PrecisePosition{Object: o, Longitude: 216, Sign: "Sco", Speed: 1.2},
		}
	}
	cycle := CycleResult{Span: CycleSpanFor(when, time.UTC), Events: events}
	facts, err := NowFacts(pairFor(ceres), cycle, when, time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	if len(facts["synodic.conjunction.inferior.last5"].Items) != 5 || facts["station.last"].Term != "station-retrograde" {
		t.Errorf("Ceres facts: %+v / %+v", facts["synodic.conjunction.inferior.last5"], facts["station.last"])
	}
	// No brightness was printed for this pair: the lines are absent, not 0.0.
	if !facts["brightness.magnitude"].Absent || !facts["brightness.lit"].Absent {
		t.Errorf("brightness for a pair that printed none: %+v %+v", facts["brightness.magnitude"], facts["brightness.lit"])
	}
	// Venus asked of Ceres' events finds none of them.
	var missing *MissingEventError
	if _, err := NowFacts(pairFor(objVenus), cycle, when, time.UTC); !errors.As(err, &missing) {
		t.Errorf("Venus against Ceres events gave %v, want a MissingEventError", err)
	}
}

// SunPair and SearchCycle hand the object to the call: Astrolog is asked for
// 1 and 12, and the row read second is object 12.
func TestSunPairAndSearchCycleAskForTheObject(t *testing.T) {
	reg := fixtureRegistry(t)
	ceres, _ := reg.ByNumber(12)
	lens := Hellenistic
	lens.Registry = reg
	rec, err := os.ReadFile("../astrolog/testdata/sunpair-sun-fortune.txt")
	if err != nil {
		t.Fatal(err)
	}
	var asked [][]int
	search := func(_ context.Context, call astrolog.BodyCall) (string, string, error) {
		asked = append(asked, call.Objects)
		return string(rec), "astrolog …", nil
	}
	res, err := SunPair(context.Background(), ceres, time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC), location.PlaceInfo{Lat: 51}, lens, search)
	if err != nil {
		t.Fatal(err)
	}
	if res.Body.Object.Number != 12 || res.Sun.Object.Number != 1 {
		t.Errorf("rows %+v / %+v", res.Sun.Object, res.Body.Object)
	}
	_, _ = SearchCycle(context.Background(), ceres, CycleSpan{StartYear: 2016, Years: 13}, lens, search)
	if want := [][]int{{1, 12}, {1, 12}}; !reflect.DeepEqual(asked, want) {
		t.Errorf("asked %v, want %v", asked, want)
	}
}

// LastInferior counts the inner conjunctions of the object asked, by number:
// relabelled to Ceres they are Ceres' and no longer Venus'.
func TestLastInferiorKeysOnTheObject(t *testing.T) {
	ceres, _ := fixtureRegistry(t).ByNumber(12)
	when := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)
	events := relabel(cycleFixture(t), ceres)
	if got := LastInferior(events, ceres, when, 5); len(got) != 5 {
		t.Errorf("Ceres: %d inner conjunctions, want 5", len(got))
	}
	if got := LastInferior(events, objVenus, when, 5); len(got) != 0 {
		t.Errorf("Venus found %d inner conjunctions among Ceres' events", len(got))
	}
}
