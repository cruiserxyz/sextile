package location

import (
	"strings"
	"testing"
)

// fixture index for resolver tests — deterministic, three cities.
func fixtureIndex() []Indexed {
	return NewIndex([]PlaceInfo{
		{Name: "Berlin", Country: "DE", Lat: 52.5244, Lon: 13.4105, TZ: "Europe/Berlin"},
		{Name: "Bern", Country: "CH", Lat: 46.948, Lon: 7.4474, TZ: "Europe/Zurich"},
		{Name: "Tokyo", Country: "JP", Lat: 35.6762, Lon: 139.6503, TZ: "Asia/Tokyo"},
	})
}

func TestResolve_CoordsPresent_PassThrough(t *testing.T) {
	candidates := []Candidate{
		{Source: "stored-location", Place: RawPlace{
			Name: "Munich", Country: "DE",
			Lat: 48.1374, Lon: 11.5761, TZ: "Europe/Berlin",
			LatPresent: true, LonPresent: true,
		}},
	}
	got, err := Resolve(candidates, fixtureIndex())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Source != "stored-location" {
		t.Errorf("Source = %q, want %q", got.Source, "stored-location")
	}
	if got.Place.Name != "Munich" || got.Place.Lat != 48.1374 {
		t.Errorf("place not passed through: %+v", got.Place)
	}
	if got.UsedAtlas {
		t.Error("atlas should NOT be consulted when coords are present")
	}
	if len(got.Warnings) != 0 {
		t.Errorf("unexpected warnings: %v", got.Warnings)
	}
}

func TestResolve_NameOnly_AtlasLookup(t *testing.T) {
	candidates := []Candidate{
		{Source: "test-source", Place: RawPlace{Name: "Berlin", Country: "DE"}},
	}
	got, err := Resolve(candidates, fixtureIndex())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Source != "test-source" {
		t.Errorf("Source = %q, want %q", got.Source, "test-source")
	}
	if !got.UsedAtlas {
		t.Error("UsedAtlas = false, want true")
	}
	if got.Place.Name != "Berlin" || got.Place.Country != "DE" {
		t.Errorf("atlas hit not propagated: %+v", got.Place)
	}
	if got.Place.Lat == 0 || got.Place.Lon == 0 {
		t.Errorf("atlas hit lacks coords: %+v", got.Place)
	}
	if got.Place.TZ != "Europe/Berlin" {
		t.Errorf("atlas hit lacks tz: %+v", got.Place)
	}
}

func TestResolve_AtlasMiss_FallsThroughChain(t *testing.T) {
	candidates := []Candidate{
		{Source: "test-source", Place: RawPlace{Name: "Atlantis", Country: "??"}},
		{Source: "iana-preseed", Place: RawPlace{Name: "Berlin"}},
	}
	got, err := Resolve(candidates, fixtureIndex())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Source != "iana-preseed" {
		t.Errorf("Source = %q, want %q (chain should have fallen through)", got.Source, "iana-preseed")
	}
	if !got.UsedAtlas {
		t.Error("UsedAtlas = false, want true")
	}
	if len(got.Warnings) == 0 {
		t.Error("expected at least one warning for the failed first candidate")
	}
	hadAtlantisWarning := false
	for _, w := range got.Warnings {
		if strings.Contains(w, "Atlantis") {
			hadAtlantisWarning = true
		}
	}
	if !hadAtlantisWarning {
		t.Errorf("expected warning about Atlantis miss; got %v", got.Warnings)
	}
}

func TestResolve_AllMiss_ReturnsError(t *testing.T) {
	candidates := []Candidate{
		{Source: "stored-location", Place: RawPlace{Name: "Atlantis"}},
		{Source: "iana-preseed", Place: RawPlace{Name: "Mu"}},
	}
	got, err := Resolve(candidates, fixtureIndex())
	if err == nil {
		t.Fatal("expected error on full-chain miss")
	}
	if got.Place.Name != "" {
		t.Errorf("unresolved Place should be zero, got %+v", got.Place)
	}
	if len(got.Warnings) < 2 {
		t.Errorf("expected per-candidate warnings, got %v", got.Warnings)
	}
}

func TestResolve_OneOfTwoCoords_FallsThrough(t *testing.T) {
	candidates := []Candidate{
		{Source: "stored-location", Place: RawPlace{
			Name: "Berlin", Lat: 52.5, LatPresent: true,
			// Lon missing
			TZ: "Europe/Berlin",
		}},
		{Source: "iana-preseed", Place: RawPlace{Name: "Tokyo"}},
	}
	got, err := Resolve(candidates, fixtureIndex())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Source != "iana-preseed" {
		t.Errorf("Source = %q, want %q (single-coord candidate should fall through)", got.Source, "iana-preseed")
	}
	if got.Place.Name != "Tokyo" {
		t.Errorf("expected Tokyo, got %+v", got.Place)
	}
	hadPartialWarning := false
	for _, w := range got.Warnings {
		if strings.Contains(w, "only one of lat/lon") {
			hadPartialWarning = true
		}
	}
	if !hadPartialWarning {
		t.Errorf("expected partial-coord warning; got %v", got.Warnings)
	}
}

func TestResolve_EmptyCandidate_Skipped(t *testing.T) {
	candidates := []Candidate{
		{Source: "test-source"}, // empty
		{Source: "iana-preseed", Place: RawPlace{Name: "Berlin"}},
	}
	got, err := Resolve(candidates, fixtureIndex())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Source != "iana-preseed" {
		t.Errorf("empty candidate should be silently skipped, got Source=%q", got.Source)
	}
	for _, w := range got.Warnings {
		if strings.Contains(w, "test-source") {
			t.Errorf("empty candidate should NOT produce a warning; got %v", got.Warnings)
		}
	}
}

func TestResolve_CoordsButNoTZ_WarnsAndUses(t *testing.T) {
	candidates := []Candidate{
		{Source: "stored-location", Place: RawPlace{
			Lat: 52.5, Lon: 13.4,
			LatPresent: true, LonPresent: true,
			// TZ empty
		}},
	}
	got, err := Resolve(candidates, fixtureIndex())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Source != "stored-location" {
		t.Errorf("coords-present should resolve as-is, got Source=%q", got.Source)
	}
	if got.Place.TZ != "" {
		t.Errorf("TZ should remain empty (coords are authoritative); got %q", got.Place.TZ)
	}
	if got.UsedAtlas {
		t.Error("atlas must NOT be consulted when coords are present")
	}
	hadTZWarning := false
	for _, w := range got.Warnings {
		if strings.Contains(w, "no tz") {
			hadTZWarning = true
		}
	}
	if !hadTZWarning {
		t.Errorf("expected no-tz warning; got %v", got.Warnings)
	}
}

func TestResolve_AtlasNotLoaded_NameOnlyFallsThrough(t *testing.T) {
	candidates := []Candidate{
		{Source: "test-source", Place: RawPlace{Name: "Berlin"}},
	}
	_, err := Resolve(candidates, nil) // atlas not loaded
	if err == nil {
		t.Fatal("expected error when atlas is nil and only name-only candidates exist")
	}
}
