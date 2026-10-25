package location

import (
	"errors"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestAtlasPath_FoundSystemFallback(t *testing.T) {
	t.Setenv("SEXTILE_ATLAS_PATH", "")
	// Isolate every tier before the system ones: the XDG data directory (a
	// developer machine may correctly carry a user-local Astrolog 8 atlas) and
	// the working directory. The system tiers are temp files, so the test needs
	// no real install and cannot be satisfied by one in the wrong place.
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Chdir(t.TempDir())
	local := writeAtlas(t, filepath.Join(t.TempDir(), "local"), "Local")
	shared := writeAtlas(t, filepath.Join(t.TempDir(), "shared"), "Shared")
	absent := filepath.Join(t.TempDir(), "absent", "atlas.as")
	saved := systemAtlasPaths
	t.Cleanup(func() { systemAtlasPaths = saved })

	// The first readable system path wins, an absent one before it is passed over.
	systemAtlasPaths = []string{absent, local, shared}
	path, err := AtlasPath()
	if err != nil || path != local {
		t.Fatalf("AtlasPath() = %q, %v, want the first readable system path %q", path, err, local)
	}
	// Both readable: the earlier tier, not the later.
	systemAtlasPaths = []string{shared, local}
	if path, err = AtlasPath(); err != nil || path != shared {
		t.Fatalf("AtlasPath() = %q, %v, want %q (system tiers keep their order)", path, err, shared)
	}
	// None readable: the error names every path it tried.
	systemAtlasPaths = []string{absent}
	if _, err = AtlasPath(); !errors.Is(err, os.ErrNotExist) || !strings.Contains(err.Error(), absent) {
		t.Fatalf("AtlasPath() error = %v, want ErrNotExist naming %q", err, absent)
	}
}

// The shipped system tiers are the two the documentation names, in this order.
func TestSystemAtlasPaths(t *testing.T) {
	want := []string{"/usr/local/share/astrolog/atlas.as", "/usr/share/astrolog/atlas.as"}
	if !slices.Equal(systemAtlasPaths, want) {
		t.Fatalf("systemAtlasPaths = %v, want %v", systemAtlasPaths, want)
	}
}

func TestAtlasPath_EnvOverride(t *testing.T) {
	path := filepath.Join(t.TempDir(), "atlas.as")
	if err := os.WriteFile(path, []byte(""), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	t.Setenv("SEXTILE_ATLAS_PATH", path)

	got, err := AtlasPath()
	if err != nil {
		t.Fatalf("AtlasPath() error = %v", err)
	}
	if got != path {
		t.Fatalf("AtlasPath() = %q, want %q", got, path)
	}
}

func TestAtlasPath_NotFound(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.as")
	t.Setenv("SEXTILE_ATLAS_PATH", path)

	_, err := AtlasPath()
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("AtlasPath() error = %v, want wrapped os.ErrNotExist", err)
	}
}

// TestAtlasPath_XDGDataHome covers the host install where atlas.as lives at
// $XDG_DATA_HOME/astrolog/atlas.as (or $HOME/.local/share/astrolog/atlas.as).
// Checked before the system /usr/{local,}/share fallbacks.
func TestAtlasPath_XDGDataHome(t *testing.T) {
	t.Setenv("SEXTILE_ATLAS_PATH", "")
	xdg := t.TempDir()
	t.Setenv("XDG_DATA_HOME", xdg)
	dir := filepath.Join(xdg, "astrolog")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	want := filepath.Join(dir, "atlas.as")
	if err := os.WriteFile(want, []byte(""), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	got, err := AtlasPath()
	if err != nil {
		t.Fatalf("AtlasPath() error = %v", err)
	}
	if got != want {
		t.Fatalf("AtlasPath() = %q, want %q", got, want)
	}
}

func TestLoadAtlas_FixtureRow(t *testing.T) {
	path := filepath.Join(t.TempDir(), "atlas.as")
	// TAB-separated, because that is what astrolog's atlas.as is: measured
	// 2026-09-20 over the shipped file, 33,297 rows of four tab columns and
	// 405 of five, with none outside that.
	//
	// This fixture used to be SPACE-separated, and that is why a real defect
	// stayed invisible behind a passing test. A space-separated row cannot be
	// parsed correctly at all -- in "us New York America/New_York" there is no
	// way to know whether the city is "New York" or "New York America/New_York"
	// without guessing, and the guess the parser made (a slash means a
	// timezone) put city names like Biel/Bienne into the timezone field and
	// carried them forward onto hundreds of later cities.
	fixture := "" +
		"; comment\n" +
		"@header\n" +
		"-13.4105\t52.5244\tde\tBerlin\tEurope/Berlin\n" +
		"-13\t52\tde\tInteger Skip\tEurope/Berlin\n" +
		"-11.5761\t48.1374\tde\tMunich\n" +
		"74.0060\t40.7128\tus\tNew York\tAmerica/New_York\n" +
		"123.1207\t49.2827\tbc\tVancouver\n" +
		"-114.0572\t51.0447\tab\tCalgary\tAmerica/Edmonton\n" +
		"-113.4938\t53.5461\tab\tEdmonton\n"
	if err := os.WriteFile(path, []byte(fixture), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	got, err := LoadAtlas(path)
	if err != nil {
		t.Fatalf("LoadAtlas() error = %v", err)
	}

	want := []PlaceInfo{
		{Name: "Berlin", Country: "DE", Lat: 52.5244, Lon: 13.4105, TZ: "Europe/Berlin"},
		{Name: "Munich", Country: "DE", Lat: 48.1374, Lon: 11.5761, TZ: "Europe/Berlin"},
		{Name: "New York", Country: "US", Lat: 40.7128, Lon: -74.0060, TZ: "America/New_York"},
		{Name: "Vancouver", Country: "BC", Lat: 49.2827, Lon: -123.1207, TZ: "America/New_York"},
		{Name: "Calgary", Country: "AB", Lat: 51.0447, Lon: 114.0572, TZ: "America/Edmonton"},
		{Name: "Edmonton", Country: "AB", Lat: 53.5461, Lon: 113.4938, TZ: "America/Edmonton"},
	}

	if len(got) != len(want) {
		t.Fatalf("len(LoadAtlas()) = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("LoadAtlas()[%d] = %#v, want %#v", i, got[i], want[i])
		}
	}
}

func TestLoadAtlas_LiveAtlas(t *testing.T) {
	// Wherever this machine installed Astrolog's data: /usr/local in the
	// container image, ~/.local on a stock host or in CI.
	path, err := AtlasPath()
	if err != nil {
		t.Skipf("live atlas unavailable: %v", err)
	}

	entries, err := LoadAtlas(path)
	if err != nil {
		t.Fatalf("LoadAtlas() error = %v", err)
	}
	if len(entries) < 24000 {
		t.Fatalf("len(LoadAtlas()) = %d, want at least 24000", len(entries))
	}

	var berlin *PlaceInfo
	for i := range entries {
		if entries[i].Name == "Berlin" && entries[i].Country == "DE" {
			berlin = &entries[i]
			break
		}
	}
	if berlin == nil {
		t.Fatal("Berlin, DE not found in live atlas")
	}
	if math.Abs(berlin.Lat-52.52) > 0.05 {
		t.Fatalf("Berlin.Lat = %f, want approx 52.52", berlin.Lat)
	}
	if math.Abs(berlin.Lon-13.41) > 0.05 {
		t.Fatalf("Berlin.Lon = %f, want approx 13.41", berlin.Lon)
	}
	if berlin.TZ != "Europe/Berlin" {
		t.Fatalf("Berlin.TZ = %q, want %q", berlin.TZ, "Europe/Berlin")
	}
}

// writeAtlas creates dir and writes a one-row atlas naming city, returning the
// file path. The row shape is astrolog's: lon, lat, country, name, IANA zone.
func writeAtlas(t *testing.T, dir, city string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", dir, err)
	}
	path := filepath.Join(dir, "atlas.as")
	body := "@AC800  ; test atlas\n-13.4050\t52.5200\tDE\t" + city + "\tEurope/Berlin\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", path, err)
	}
	return path
}

// TestAtlasPath_UserLocalOutranksCwd pins the precedence flip: a populated
// user-local astrolog directory now outranks an atlas.as in the working
// directory, where cwd used to win.
//
// The proof is the path comparison: under the old cwd-first order AtlasPath
// returns the cwd fixture and this fails, verified by reverting the ordering
// and watching it fail by name. The fixtures carry different city names only so
// each is identifiable when read by hand; AtlasPath resolves on readability and
// never parses them, so nothing here depends on their contents.
func TestAtlasPath_UserLocalOutranksCwd(t *testing.T) {
	t.Setenv("SEXTILE_ATLAS_PATH", "")
	xdg := t.TempDir()
	t.Setenv("XDG_DATA_HOME", xdg)
	want := writeAtlas(t, filepath.Join(xdg, "astrolog"), "UserLocalCity")

	cwd := t.TempDir()
	writeAtlas(t, cwd, "CwdCity")
	t.Chdir(cwd)

	got, err := AtlasPath()
	if err != nil {
		t.Fatalf("AtlasPath() error = %v", err)
	}
	if got != want {
		t.Fatalf("AtlasPath() = %q, want the user-local atlas %q", got, want)
	}
}

// TestAtlasPath_SextileDataRootOutranksAstrologDir pins tier 2 above tier 3:
// Sextile's own data root wins over a plain astrolog user install.
func TestAtlasPath_SextileDataRootOutranksAstrologDir(t *testing.T) {
	t.Setenv("SEXTILE_ATLAS_PATH", "")
	xdg := t.TempDir()
	t.Setenv("XDG_DATA_HOME", xdg)
	want := writeAtlas(t, filepath.Join(xdg, "sextile", "astrolog"), "SextileRootCity")
	writeAtlas(t, filepath.Join(xdg, "astrolog"), "AstrologDirCity")

	got, err := AtlasPath()
	if err != nil {
		t.Fatalf("AtlasPath() error = %v", err)
	}
	if got != want {
		t.Fatalf("AtlasPath() = %q, want the Sextile data root atlas %q", got, want)
	}
}

// TestLoadAtlasKeepsSlashedCityNamesOutOfTheTimezone is the control for a
// defect the operator met in live testing on 2026-09-20: a saved chart listed
// in the picker and then refused to anchor with "an unknown timezone".
//
// The atlas is TAB-separated and its column count says whether a row carries a
// zone. The previous parser split on ANY whitespace, which destroys that
// count, and then guessed: it took the last field as a timezone if it
// contained a slash. City names contain slashes too, so "Biel/Bienne" was
// eaten as the zone -- and because the parser carries a row's zone forward to
// later rows that have only four columns, one such name poisoned every city
// after it. 831 of 33,627 real entries carried a city name where an IANA zone
// belongs.
//
// The fixture below is that exact shape: an explicit-zone row, then a
// slash-NAMED row with no zone column, then two plain rows that inherit.
// Against the old parser the last three rows all came back with TZ
// "Biel/Bienne"; against this one they inherit Europe/Zurich and the slashed
// name survives intact.
func TestLoadAtlasKeepsSlashedCityNamesOutOfTheTimezone(t *testing.T) {
	// lon <TAB> lat <TAB> country <TAB> name [<TAB> timezone]
	const rows = "" +
		"-8.0442\t47.3925\tCH\tAarau\tEurope/Zurich\n" +
		"-7.2461\t47.1371\tCH\tBiel/Bienne\n" +
		"-9.5333\t46.8500\tCH\tChur\n" +
		"-8.4000\t47.4000\tCH\tDietikon\n"

	path := filepath.Join(t.TempDir(), "atlas.as")
	if err := os.WriteFile(path, []byte(rows), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	got, err := LoadAtlas(path)
	if err != nil {
		t.Fatalf("LoadAtlas: %v", err)
	}
	if len(got) != 4 {
		t.Fatalf("LoadAtlas returned %d entries, want 4: %+v", len(got), got)
	}
	for i, want := range []struct{ name, tz string }{
		{"Aarau", "Europe/Zurich"},
		{"Biel/Bienne", "Europe/Zurich"},
		{"Chur", "Europe/Zurich"},
		{"Dietikon", "Europe/Zurich"},
	} {
		if got[i].Name != want.name {
			t.Errorf("entry %d name = %q, want %q", i, got[i].Name, want.name)
		}
		if got[i].TZ != want.tz {
			t.Errorf("entry %d (%s) TZ = %q, want %q", i, want.name, got[i].TZ, want.tz)
		}
	}
}

// A timezone no Go runtime can load is unusable: the chart picker lists such a
// row (it only asks whether the field is non-empty) and anchoring it then
// refuses. So the parser must never produce one -- asserted here against the
// REAL atlas, because the defect this guards was a property of real data that
// no fixture would have predicted.
func TestLoadAtlasProducesOnlyLoadableTimezones(t *testing.T) {
	path, err := AtlasPath()
	if err != nil {
		t.Skipf("no atlas.as on this machine: %v", err)
	}
	entries, err := LoadAtlas(path)
	if err != nil {
		t.Fatalf("LoadAtlas(%q): %v", path, err)
	}
	if len(entries) < 1000 {
		t.Fatalf("LoadAtlas returned only %d entries; the format may have changed", len(entries))
	}
	bad := map[string]int{}
	var sample []string
	for _, e := range entries {
		if _, err := time.LoadLocation(e.TZ); err != nil {
			if bad[e.TZ] == 0 && len(sample) < 5 {
				sample = append(sample, e.Name+" ("+e.Country+") -> "+e.TZ)
			}
			bad[e.TZ]++
		}
	}
	if len(bad) != 0 {
		total := 0
		for _, n := range bad {
			total += n
		}
		t.Errorf("%d of %d atlas entries carry a timezone Go cannot load (%d distinct); samples: %v",
			total, len(entries), len(bad), sample)
	}
}

// A file whose rows are not tab-separated is a format mismatch, and LoadAtlas
// must say so rather than return an empty atlas. The callers read an empty
// result as "no place list" and carry on, so a silent empty would present the
// wrong format as a missing one and leave every chart unplaceable with no clue
// why. This is the shape the old space-separated fixture had.
func TestLoadAtlasRefusesAFileThatIsNotTabSeparated(t *testing.T) {
	path := filepath.Join(t.TempDir(), "atlas.as")
	spaceSeparated := "" +
		"-13.4105 52.5244 de Berlin Europe/Berlin\n" +
		"-11.5761 48.1374 de Munich\n"
	if err := os.WriteFile(path, []byte(spaceSeparated), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	got, err := LoadAtlas(path)
	if err == nil {
		t.Fatalf("LoadAtlas accepted a space-separated file, returning %d entries", len(got))
	}
	if !strings.Contains(err.Error(), "tab-separated") {
		t.Errorf("error = %v, want it to name the format problem", err)
	}
}
