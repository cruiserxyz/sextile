package astrolog

import (
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// The cycle search names its other object by Astrolog's number, not by a
// Sextile-side list: Ceres is object 12 in the registry the fixtures come from.
func TestCycleCallForCeres(t *testing.T) {
	ceres, ok := testRegistry(t).ByNumber(12)
	if !ok || ceres.Name != "Ceres" {
		t.Fatalf("object 12 is %+v, want Ceres", ceres)
	}
	call, err := CycleCall(CycleRequest{StartYear: 2016, Years: 13, Other: ceres, Registry: testRegistry(t)})
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(call.argv, " "); !strings.HasSuffix(got, " =Yt _Yd -R0 1 12 -RA0 1") {
		t.Errorf("argv %q does not carry -R0 1 12", got)
	}
	if !reflect.DeepEqual(call.Objects, []int{1, 12}) {
		t.Errorf("call.Objects = %v, want [1 12]", call.Objects)
	}
}

// The Sun pair for Fortune (object 19): the band asks 1 and 19, and the
// parser, given a real recording of that call, returns Fortune's row.
func TestSunPairCallForFortune(t *testing.T) {
	reg := testRegistry(t)
	fortune, ok := reg.ByNumber(19)
	if !ok || fortune.Name != "Fortune" {
		t.Fatalf("object 19 is %+v, want Fortune", fortune)
	}
	call, err := SunPairCall(SunPairRequest{When: time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC), Lat: 51, Other: fortune, Registry: reg})
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(call.argv, " "); !strings.HasSuffix(got, " =Yt _Yd -R0 1 19 -RA0 1") {
		t.Errorf("argv %q does not carry -R0 1 19", got)
	}
	if !reflect.DeepEqual(call.Objects, []int{1, 19}) {
		t.Errorf("call.Objects = %v, want [1 19]", call.Objects)
	}
	rec, err := os.ReadFile("testdata/sunpair-sun-fortune.txt")
	if err != nil {
		t.Fatal(err)
	}
	sun, other, err := ParseSunPair(string(rec), call.Objects, call.Registry)
	if err != nil {
		t.Fatal(err)
	}
	if sun.Object.Number != 1 || other.Object.Number != 19 || other.Object.Name != "Fortune" {
		t.Errorf("rows %+v / %+v", sun.Object, other.Object)
	}
	if !sun.HasBrightness || other.HasBrightness {
		t.Errorf("brightness: Sun %v (want true), Fortune %v (want false: Astrolog printed none)", sun.HasBrightness, other.HasBrightness)
	}
	if other.Sign == "" || other.Longitude < 0 || other.Longitude >= 360 {
		t.Errorf("Fortune row not read: %+v", other)
	}
}

// The Sun itself and the zero Object are refused as the other object, by
// number, in both calls.
func TestCycleCallRefusesTheSunAndTheZeroObject(t *testing.T) {
	sun, ok := testRegistry(t).ByNumber(1)
	if !ok {
		t.Fatal("no object 1")
	}
	for name, o := range map[string]aspect.Object{"the Sun": sun, "the zero object": {}} {
		if _, err := CycleCall(CycleRequest{StartYear: 2016, Years: 13, Other: o}); err == nil {
			t.Errorf("CycleCall accepted %s", name)
		}
		if _, err := SunPairCall(SunPairRequest{When: time.Unix(0, 0), Other: o}); err == nil {
			t.Errorf("SunPairCall accepted %s", name)
		}
	}
}
