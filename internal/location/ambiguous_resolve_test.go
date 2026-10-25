package location

import (
	"strings"
	"testing"
)

// ambiguousIndex mirrors the real atlas ordering that produced the defect:
// Astrolog's atlas.as lists "New Amsterdam" (GY) at line 13021, well
// BEFORE "Amsterdam" (NL) at 23170, so any rule that breaks a tie by file
// order picks Guyana. Amsterdam-Zuidoost and Amsterdam NY are in the file too
// and matter: the first shares NL's timezone, the second shares its name.
func ambiguousIndex() []Indexed {
	return NewIndex([]PlaceInfo{
		{Name: "New Amsterdam", Country: "GY", Lat: 6.2492, Lon: -57.5158, TZ: "America/Guyana"},
		{Name: "Amsterdam", Country: "NL", Lat: 52.374, Lon: 4.8897, TZ: "Europe/Amsterdam"},
		{Name: "Amsterdam-Zuidoost", Country: "NL", Lat: 52.3075, Lon: 4.9722, TZ: "Europe/Amsterdam"},
		{Name: "Amsterdam", Country: "ny", Lat: 42.9387, Lon: -74.1882, TZ: "America/New_York"},
	})
}

// TestResolve_HostZoneDisambiguatesTheCity is the defect itself. A host in
// Europe/Amsterdam must not open its charts in Guyana: the zone that produced
// the preseed name is also the thing that says which Amsterdam was meant.
func TestResolve_HostZoneDisambiguatesTheCity(t *testing.T) {
	got, err := Resolve([]Candidate{
		{Source: "iana-preseed", Place: RawPlace{Name: "Amsterdam", TZ: "Europe/Amsterdam"}},
	}, ambiguousIndex())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Place.Country != "NL" || got.Place.Name != "Amsterdam" {
		t.Errorf("resolved to %q, %q; want Amsterdam, NL", got.Place.Name, got.Place.Country)
	}
	if got.Place.TZ != "Europe/Amsterdam" {
		t.Errorf("resolved TZ = %q, want Europe/Amsterdam", got.Place.TZ)
	}
	// The offsets are six hours apart, which is what made this silent rather
	// than obvious: the frame renders, it is just the wrong place.
	if got.Place.Lon < 0 {
		t.Errorf("resolved to a western longitude (%v) — that is the Guyana row", got.Place.Lon)
	}
}

// TestResolve_ExactNameBeatsALongerNameInTheSameZone: the host zone alone
// leaves Amsterdam and Amsterdam-Zuidoost, both Europe/Amsterdam. The city
// whose name IS the query is the one that was meant.
func TestResolve_ExactNameBeatsALongerNameInTheSameZone(t *testing.T) {
	got, err := Resolve([]Candidate{
		{Source: "iana-preseed", Place: RawPlace{Name: "Amsterdam", TZ: "Europe/Amsterdam"}},
	}, ambiguousIndex())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Place.Name != "Amsterdam" {
		t.Errorf("resolved to %q, want the exact-name row \"Amsterdam\"", got.Place.Name)
	}
}

// TestResolve_RefusesAnAmbiguousNameRatherThanGuessing is the rule this file
// already states for the Berlin fallback, applied to the case it missed: with
// nothing to disambiguate by, refusing opens the picker and the operator sees
// what is happening. Silently casting a chart in the wrong hemisphere does not.
func TestResolve_RefusesAnAmbiguousNameRatherThanGuessing(t *testing.T) {
	got, err := Resolve([]Candidate{
		{Source: "iana-preseed", Place: RawPlace{Name: "Amsterdam"}}, // no TZ
	}, ambiguousIndex())
	if err == nil {
		t.Fatalf("an ambiguous name resolved to %q, %q instead of refusing",
			got.Place.Name, got.Place.Country)
	}
	joined := strings.Join(got.Warnings, " | ")
	if !strings.Contains(strings.ToLower(joined), "ambiguous") {
		t.Errorf("the refusal does not say the name was ambiguous: %v", got.Warnings)
	}
	if !strings.Contains(joined, "Amsterdam") {
		t.Errorf("the refusal does not name what was ambiguous: %v", got.Warnings)
	}
}

// TestResolve_UnambiguousNameStillResolves is the regression guard on the
// refusal: it must fire on real ambiguity only, or every startup meets a
// picker it did not need.
func TestResolve_UnambiguousNameStillResolves(t *testing.T) {
	for _, tc := range []struct {
		name string
		cand RawPlace
	}{
		{"name only, one match", RawPlace{Name: "Tokyo"}},
		{"name and zone", RawPlace{Name: "Berlin", TZ: "Europe/Berlin"}},
		{"zone that matches nothing falls back to the name", RawPlace{Name: "Tokyo", TZ: "Mars/Olympus"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Resolve([]Candidate{{Source: "iana-preseed", Place: tc.cand}}, fixtureIndex())
			if err != nil {
				t.Fatalf("unambiguous candidate %+v refused: %v (%v)", tc.cand, err, got.Warnings)
			}
			if !strings.EqualFold(got.Place.Name, tc.cand.Name) {
				t.Errorf("resolved to %q, want %q", got.Place.Name, tc.cand.Name)
			}
		})
	}
}

// TestResolve_JudgesAmbiguityOnEveryMatchNotAPage: the lookup used a cap of 5,
// so a sixth row could not be seen — and a set truncated to 5 can look
// unambiguous, or hide the row the zone would have chosen. Here the wanted
// row sits behind six decoys.
func TestResolve_JudgesAmbiguityOnEveryMatchNotAPage(t *testing.T) {
	places := []PlaceInfo{}
	for _, n := range []string{"Amsterdam Alpha", "Amsterdam Bravo", "Amsterdam Charlie",
		"Amsterdam Delta", "Amsterdam Echo", "Amsterdam Foxtrot"} {
		places = append(places, PlaceInfo{Name: n, Country: "GY", Lat: 6, Lon: -57, TZ: "America/Guyana"})
	}
	places = append(places, PlaceInfo{
		Name: "Amsterdam", Country: "NL", Lat: 52.374, Lon: 4.8897, TZ: "Europe/Amsterdam"})

	got, err := Resolve([]Candidate{
		{Source: "iana-preseed", Place: RawPlace{Name: "Amsterdam", TZ: "Europe/Amsterdam"}},
	}, NewIndex(places))
	if err != nil {
		t.Fatalf("the seventh row was unreachable, so the zone could not disambiguate: %v (%v)", err, got.Warnings)
	}
	if got.Place.Country != "NL" {
		t.Errorf("resolved to %q, %q; want Amsterdam, NL from beyond the old cap",
			got.Place.Name, got.Place.Country)
	}
}
