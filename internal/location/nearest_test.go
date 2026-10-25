package location

import "testing"

func TestNearest_PicksClosestWithinThreshold(t *testing.T) {
	entries := []PlaceInfo{
		{Name: "Munich", Country: "DE", Lon: 11.5755, Lat: 48.1374, TZ: "Europe/Berlin"},
		{Name: "Berlin", Country: "DE", Lon: 13.4050, Lat: 52.5200, TZ: "Europe/Berlin"},
		{Name: "Paris", Country: "FR", Lon: 2.3522, Lat: 48.8566, TZ: "Europe/Paris"},
	}
	// Equinox-2026-03's place coords (11:34E 48:08N) — closest is Munich.
	got, ok := Nearest(entries, 11.5667, 48.1333)
	if !ok {
		t.Fatal("Nearest returned no match within threshold")
	}
	if got.Name != "Munich" {
		t.Errorf("Nearest = %q, want Munich", got.Name)
	}
	if got.TZ != "Europe/Berlin" || got.Country != "DE" {
		t.Errorf("borrowed metadata = (%q, %q), want (Europe/Berlin, DE)", got.TZ, got.Country)
	}
}

func TestNearest_RejectsTooFar(t *testing.T) {
	entries := []PlaceInfo{
		{Name: "Munich", Country: "DE", Lon: 11.5755, Lat: 48.1374, TZ: "Europe/Berlin"},
	}
	// (0,0) is in the Atlantic, far from any entry → no match.
	if _, ok := Nearest(entries, 0, 0); ok {
		t.Error("Nearest matched a coordinate beyond NearestMaxDeg")
	}
}

func TestNearest_EmptyAtlas(t *testing.T) {
	if _, ok := Nearest(nil, 11.5, 48.1); ok {
		t.Error("Nearest on an empty atlas reported a match")
	}
}

// cos(latitude) weighting must not let a far-east city beat a true-nearest one
// at high latitude where a degree of longitude is much shorter than a degree of
// latitude.
func TestNearest_LongitudeScaledByLatitude(t *testing.T) {
	entries := []PlaceInfo{
		{Name: "North", Lon: 10.0, Lat: 60.5}, // 0.5° latitude away
		{Name: "East", Lon: 10.9, Lat: 60.0},  // 0.9° longitude away (~0.45° scaled at 60°N)
	}
	got, ok := Nearest(entries, 10.0, 60.0)
	if !ok {
		t.Fatal("no match")
	}
	if got.Name != "East" {
		t.Errorf("Nearest = %q, want East (0.9°·cos60° ≈ 0.45 < 0.5)", got.Name)
	}
}
