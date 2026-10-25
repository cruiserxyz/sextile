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

// Berlin and its district Mitte sit within an arc-minute of each other; a
// point between them is closer to Mitte, and only the file's own label says
// which city the chart was cast in.
func berlinAndMitte() []PlaceInfo {
	return []PlaceInfo{
		{Name: "Berlin", Country: "DE", Lon: 13.4105, Lat: 52.5244, TZ: "Europe/Berlin"},
		{Name: "Mitte", Country: "DE", Lon: 13.4049, Lat: 52.52, TZ: "Europe/Berlin"},
		{Name: "Hamburg", Country: "DE", Lon: 10.0, Lat: 53.55, TZ: "Europe/Berlin"},
	}
}

func TestNearestNamed_LabelWinsOverACloserNeighbour(t *testing.T) {
	// Berlin's atlas point rounded to whole arc-minutes: 13:25E 52:31N.
	lon, lat := 13.0+25.0/60, 52.0+31.0/60
	if n, _ := Nearest(berlinAndMitte(), lon, lat); n.Name != "Mitte" {
		t.Fatalf("setup: Nearest = %q, want Mitte (the neighbour the label must beat)", n.Name)
	}
	for _, label := range []string{"Berlin, DE", "Berlin"} {
		got, ok := NearestNamed(berlinAndMitte(), lon, lat, label)
		if !ok || got.Name != "Berlin" || got.TZ != "Europe/Berlin" {
			t.Errorf("NearestNamed(%q) = %+v, %v, want Berlin", label, got, ok)
		}
	}
}

func TestNearestNamed_NoLabelIsNearest(t *testing.T) {
	lon, lat := 13.0+25.0/60, 52.0+31.0/60
	got, ok := NearestNamed(berlinAndMitte(), lon, lat, "")
	if !ok || got.Name != "Mitte" {
		t.Errorf("NearestNamed with no label = %+v, %v, want Mitte as Nearest says", got, ok)
	}
}

func TestNearestNamed_LabelBeyondAnArcMinuteIsIgnored(t *testing.T) {
	lon, lat := 13.0+25.0/60, 52.0+31.0/60
	for _, label := range []string{"Hamburg, DE", "Berlin, FR", "Atlantis"} {
		got, ok := NearestNamed(berlinAndMitte(), lon, lat, label)
		if !ok || got.Name != "Mitte" {
			t.Errorf("NearestNamed(%q) = %+v, %v, want Nearest's Mitte", label, got, ok)
		}
	}
}

// A Display over 54 bytes is cut by the save and ends in an ellipsis; the
// reopened label then names every city that starts with what is left.
func TestNearestNamed_CutLabelMatchesByPrefix(t *testing.T) {
	long := "Llanfairpwllgwyngyllgogerychwyrndrobwllllantysiliogogogoch, GB"
	entries := []PlaceInfo{
		{Name: "Llanfairpwllgwyngyllgogerychwyrndrobwllllantysiliogogogoch", Country: "GB", Lon: -4.2, Lat: 53.22, TZ: "Europe/London"},
		{Name: "Menai Bridge", Country: "GB", Lon: -4.2005, Lat: 53.2201, TZ: "Europe/London"},
	}
	cut := long[:50] + "…"
	got, ok := NearestNamed(entries, -4.2004, 53.2201, cut)
	if !ok || got.Name != entries[0].Name {
		t.Errorf("NearestNamed(%q) = %+v, %v, want the long-named city", cut, got, ok)
	}
	if n, _ := Nearest(entries, -4.2004, 53.2201); n.Name != "Menai Bridge" {
		t.Fatalf("setup: Nearest = %q, want the neighbour the cut label must beat", n.Name)
	}
}

// A quote in a Display is a single quote in the saved label.
func TestNearestNamed_QuoteInDisplayMatchesTheSavedLabel(t *testing.T) {
	entries := []PlaceInfo{
		{Name: `Ha"penny`, Country: "GB", Lon: 1.0, Lat: 51.0, TZ: "Europe/London"},
		{Name: "Next Door", Country: "GB", Lon: 1.0002, Lat: 51.0, TZ: "Europe/London"},
	}
	got, ok := NearestNamed(entries, 1.00015, 51.0, "Ha'penny, GB")
	if !ok || got.Name != `Ha"penny` {
		t.Errorf("NearestNamed = %+v, %v, want Ha\"penny", got, ok)
	}
}
