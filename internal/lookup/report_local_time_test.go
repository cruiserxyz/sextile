package lookup

import (
	"context"
	"os/exec"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/cruiserxyz/sextile/internal/astrolog"
	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/recipe"
	"github.com/cruiserxyz/sextile/internal/store"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// the specification § Fixtures. The oracle for "everything after
// the header is Astrolog's, unchanged" is the same report through the old
// Julian-day path, computed live (utReport), never a frozen file.
type reportFixture struct {
	name  string
	when  time.Time
	place location.PlaceInfo
}

func utc(y int, mo time.Month, d, h, mi int) time.Time {
	return time.Date(y, mo, d, h, mi, 0, 0, time.UTC)
}

var reportFixtures = []reportFixture{
	{"F1", utc(2026, 7, 1, 10, 0), location.PlaceInfo{Name: "Amsterdam", Country: "NL", Lon: 4.9, Lat: 52.3667, TZ: "Europe/Amsterdam"}},
	{"F2", utc(2026, 1, 15, 10, 0), location.PlaceInfo{Name: "Amsterdam", Country: "NL", Lon: 4.9, Lat: 52.3667, TZ: "Europe/Amsterdam"}},
	{"F3", utc(2026, 10, 3, 2, 30), location.PlaceInfo{Name: "New York", Country: "US", Lon: -74.0, Lat: 40.7167, TZ: "America/New_York"}},
	{"F4", utc(2026, 1, 10, 20, 0), location.PlaceInfo{Name: "Sydney", Country: "AU", Lon: 151.2, Lat: -33.8667, TZ: "Australia/Sydney"}},
	{"F5", utc(2026, 3, 1, 6, 0), location.PlaceInfo{Name: "Delhi", Country: "IN", Lon: 77.2167, Lat: 28.6167, TZ: "Asia/Kolkata"}},
	{"F6", utc(2026, 3, 1, 6, 0), location.PlaceInfo{Name: "Kathmandu", Country: "NP", Lon: 85.3167, Lat: 27.7167, TZ: "Asia/Kathmandu"}},
	{"F7", utc(2026, 1, 10, 12, 0), location.PlaceInfo{Name: "Lord Howe", Country: "AU", Lon: 159.0833, Lat: -31.55, TZ: "Australia/Lord_Howe"}},
	{"F8", utc(2026, 3, 1, 6, 0), location.PlaceInfo{Name: "Kiritimati", Country: "KI", Lon: -157.4, Lat: 1.8667, TZ: "Pacific/Kiritimati"}},
	{"F9a", utc(2026, 10, 25, 0, 30), location.PlaceInfo{Name: "Amsterdam", Country: "NL", Lon: 4.9, Lat: 52.3667, TZ: "Europe/Amsterdam"}},
	{"F9b", utc(2026, 10, 25, 1, 30), location.PlaceInfo{Name: "Amsterdam", Country: "NL", Lon: 4.9, Lat: 52.3667, TZ: "Europe/Amsterdam"}},
	// Amsterdam Time, +0:19:32 (standard) and +1:19:32 (summer): an east zone
	// under one hour, where Astrolog would read `-0:19:32` as WEST.
	{"F10a", time.Date(1930, 1, 1, 11, 40, 28, 0, time.UTC), location.PlaceInfo{Name: "Amsterdam", Country: "NL", Lon: 4.9, Lat: 52.3667, TZ: "Europe/Amsterdam"}},
	{"F10b", time.Date(1930, 6, 1, 10, 40, 28, 0, time.UTC), location.PlaceInfo{Name: "Amsterdam", Country: "NL", Lon: 4.9, Lat: 52.3667, TZ: "Europe/Amsterdam"}},
}

func fixtureByName(t *testing.T, name string) reportFixture {
	t.Helper()
	for _, f := range reportFixtures {
		if f.name == name {
			return f
		}
	}
	t.Fatalf("no fixture %s", name)
	return reportFixture{}
}

func requireAstrolog(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("astrolog"); err != nil {
		t.Skip("astrolog not on PATH; skipping live report test")
	}
}

func singleReport(t *testing.T, f reportFixture) string {
	t.Helper()
	raw, _, err := ComputeRawReport(context.Background(), f.when, f.place, Hellenistic, RawReportRequest{Kind: RawReportChart})
	if err != nil {
		t.Fatalf("%s: ComputeRawReport: %v", f.name, err)
	}
	return raw
}

func pairReport(t *testing.T, natal, transit reportFixture) string {
	t.Helper()
	raw, _, err := CrossBodyRawReport(context.Background(), Hellenistic,
		store.AnchorChart{When: natal.when, Place: natal.place}, transit.when, transit.place, aspect.Sun)
	if err != nil {
		t.Fatalf("pair %s x %s: %v", natal.name, transit.name, err)
	}
	return raw
}

// utReport is the oracle: the same report through the UNCHANGED Julian-day
// path (specAt, `-qj JD -zl`), computed live. It is not a file, so it cannot be
// regenerated from the new code; specAt itself is pinned by the argv golden.
func utReport(t *testing.T, f reportFixture) string {
	t.Helper()
	projection, err := RawReportRequest{Kind: RawReportChart}.projection()
	if err != nil {
		t.Fatal(err)
	}
	spec, err := Hellenistic.specAt(astrolog.GregorianToJD(f.when), f.place)
	if err != nil {
		t.Fatal(err)
	}
	raw, _, err := astrolog.ComputeRawReport(context.Background(), spec, projection)
	if err != nil {
		t.Fatalf("%s: UT oracle: %v", f.name, err)
	}
	requireUTHeader(t, f.name, raw, 1)
	return raw
}

// utPairReport is the pair oracle through the unchanged comparisonSpecAt.
func utPairReport(t *testing.T, natal, transit reportFixture) string {
	t.Helper()
	spec, err := Hellenistic.comparisonSpecAt(
		astrolog.GregorianToJD(natal.when), natal.place, astrolog.GregorianToJD(transit.when), transit.place)
	if err != nil {
		t.Fatal(err)
	}
	raw, _, err := astrolog.ComputeRawReport(context.Background(), spec, recipe.BodyRawReport(aspect.Sun))
	if err != nil {
		t.Fatalf("UT pair oracle: %v", err)
	}
	requireUTHeader(t, "pair", raw, 2)
	return raw
}

// requireUTHeader proves the oracle really is the UT path: its first n header
// lines say "(ST Zone 0W)". Without this a broken oracle that took the new
// path would make every body comparison pass trivially.
func requireUTHeader(t *testing.T, name, raw string, n int) {
	t.Helper()
	lines := strings.SplitN(raw, "\n", n+1)
	for i := 0; i < n; i++ {
		if i >= len(lines) || !strings.Contains(lines[i], "(ST Zone 0W)") {
			t.Fatalf("%s: oracle header %d is not UT: %q", name, i+1, lines[i])
		}
	}
}

// afterLines is every line of s from index n on, rejoined: what the header
// change must not touch.
func afterLines(s string, n int) string {
	lines := strings.Split(s, "\n")
	if len(lines) <= n {
		return ""
	}
	return strings.Join(lines[n:], "\n")
}

// headerRe reads Astrolog's own header words: weekday, date, clock, marker,
// zone. The test parses them; Sextile prints none of them.
var headerRe = regexp.MustCompile(`(?:chart|comparison) for (\w{3} \w{3} +\d+, \d{4} +\d+:\d{2}) \((ST|DT) Zone ([^)]+)\)`)

// slotLine is what the Chart panel's slot line shows for the instant, in the
// slot's zone: ui.slotMoment's layout, "Mon Jan 02, 2006 15:04".
func slotLine(t *testing.T, f reportFixture) string {
	t.Helper()
	loc, err := time.LoadLocation(f.place.TZ)
	if err != nil {
		t.Fatal(err)
	}
	return f.when.In(loc).Format("Mon Jan 02, 2006 15:04")
}

// headerClock is the header's weekday, date and clock re-laid-out as the slot
// line prints them, plus its marker and zone words.
func headerClock(t *testing.T, line string) (clock, marker, zone string) {
	t.Helper()
	m := headerRe.FindStringSubmatch(line)
	if m == nil {
		t.Fatalf("not an Astrolog chart header: %q", line)
	}
	ts, err := time.Parse("Mon Jan _2, 2006 15:04", strings.Join(strings.Fields(m[1]), " "))
	if err != nil {
		// _2 pads with a space; the collapsed form has none.
		ts, err = time.Parse("Mon Jan 2, 2006 15:04", strings.Join(strings.Fields(m[1]), " "))
		if err != nil {
			t.Fatalf("header clock %q: %v", m[1], err)
		}
	}
	return ts.Format("Mon Jan 02, 2006 15:04"), m[2], m[3]
}

// Rules 1-3: the report's header is the slot's own clock, in Astrolog's words,
// and nothing after the header moved.
func TestReportLocalTimeKeepsTheChart(t *testing.T) {
	requireAstrolog(t)
	want := map[string]string{ // marker + zone as Astrolog words them
		"F1": "DT 1E", "F2": "ST 1E", "F3": "DT 5W", "F4": "DT 10E", "F5": "ST 5:30E",
		"F6": "ST 5:45E", "F7": "ST 11E", "F8": "ST 14E", "F9a": "DT 1E", "F9b": "ST 1E",
		"F10a": "ST 0:19E", "F10b": "DT 0:19E",
	}
	for _, f := range reportFixtures {
		t.Run(f.name, func(t *testing.T) {
			got := singleReport(t, f)
			if a, b := afterLines(got, 1), afterLines(utReport(t, f), 1); a != b {
				t.Errorf("lines after the header differ from the live -qj report")
			}
			clock, marker, zone := headerClock(t, strings.SplitN(got, "\n", 2)[0])
			if clock != slotLine(t, f) {
				t.Errorf("header clock = %q, slot line = %q", clock, slotLine(t, f))
			}
			if marker+" "+zone != want[f.name] {
				t.Errorf("header zone = %q, want %q", marker+" "+zone, want[f.name])
			}
		})
	}
}

// Rule 5: both chart headers of a pair report name their own slot's clock,
// every other line is the frozen pair report, and the staged files carry local
// time and zone (a `-zZ` line) instead of UT.
func TestPairReportLocalTime(t *testing.T) {
	requireAstrolog(t)
	natal, transit := fixtureByName(t, "F1"), fixtureByName(t, "F3")
	got := pairReport(t, natal, transit)
	if a, b := afterLines(got, 2), afterLines(utPairReport(t, natal, transit), 2); a != b {
		t.Errorf("lines after the two headers differ from the live -qj pair report")
	}
	lines := strings.Split(got, "\n")
	for i, f := range []reportFixture{natal, transit} {
		clock, marker, zone := headerClock(t, lines[i])
		if clock != slotLine(t, f) {
			t.Errorf("header %d clock = %q, slot line = %q", i+1, clock, slotLine(t, f))
		}
		if wantZone := map[string]string{"F1": "DT 1E", "F3": "DT 5W"}[f.name]; marker+" "+zone != wantZone {
			t.Errorf("header %d zone = %q, want %q", i+1, marker+" "+zone, wantZone)
		}
	}
	spec, err := Hellenistic.localComparisonSpecAt(
		astrolog.GregorianToJD(natal.when), natal.place, store.ReportLabel{}, astrolog.GregorianToJD(transit.when), transit.place, store.ReportLabel{})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := astrolog.Compile(spec, recipe.BodyRawReport(aspect.Sun))
	if err != nil {
		t.Fatal(err)
	}
	temps := plan.Invocations[0].TempInputs
	if len(temps) != 2 {
		t.Fatalf("staged files = %d, want 2", len(temps))
	}
	// The moment is the same -qj every call sends; the local clock is a -zZ line
	// after it (DT 1 with the standard offset: Amsterdam -1, New York 5).
	for _, want := range []string{"-qj " + jdWord(natal.when) + "\n-zl ", "-zZ -1 1\n"} {
		if !strings.Contains(temps[0].Content, want) {
			t.Errorf("staged chart 1 = %q, want it to contain %q", temps[0].Content, want)
		}
	}
	for _, want := range []string{"-qj " + jdWord(transit.when) + "\n-zl ", "-zZ 5 1\n"} {
		if !strings.Contains(temps[1].Content, want) {
			t.Errorf("staged chart 2 = %q, want it to contain %q", temps[1].Content, want)
		}
	}
	for i, tmp := range temps {
		if strings.Contains(tmp.Content, "-qb") || !strings.Contains(tmp.Content, "\n-zZ ") {
			t.Errorf("staged chart %d is in UT or carries calendar fields: %q", i+1, tmp.Content)
		}
	}
}

// The cross-aspect list and the wheel image share the comparison but must keep
// their UT chart files: only the report asks for the local clock.
func TestComparisonSpecAtStaysInUT(t *testing.T) {
	natal, transit := fixtureByName(t, "F1"), fixtureByName(t, "F3")
	spec, err := Hellenistic.comparisonSpecAt(
		astrolog.GregorianToJD(natal.when), natal.place, astrolog.GregorianToJD(transit.when), transit.place)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []recipe.Projection{recipe.Aspects{}, recipe.BodyRawReport(aspect.Sun)} {
		plan, err := astrolog.Compile(spec, p)
		if err != nil {
			t.Fatal(err)
		}
		for i, tmp := range plan.Invocations[0].TempInputs {
			if strings.Contains(tmp.Content, "-zZ") || !strings.Contains(tmp.Content, "\n-qj ") {
				t.Errorf("%s: staged chart %d left UT: %q", p.Label(), i+1, tmp.Content)
			}
		}
	}
}

// Rule 6 at the compiler seam: a zone that does not load is refused before any
// process starts (the spec is compiled, never run).
func TestReportRefusesAnUnknownZoneBeforeAstrolog(t *testing.T) {
	f := fixtureByName(t, "F1")
	f.place.TZ = "Mars/Olympus_Mons"
	if _, _, err := ComputeRawReport(context.Background(), f.when, f.place, Hellenistic, RawReportRequest{Kind: RawReportChart}); err == nil {
		t.Error("an unloadable zone was accepted by the single report")
	} else if !strings.Contains(err.Error(), "Mars/Olympus_Mons") {
		t.Errorf("error does not name the zone: %v", err)
	}
	natal, transit := fixtureByName(t, "F1"), fixtureByName(t, "F3")
	natal.place.TZ = "Mars/Olympus_Mons"
	_, command, err := CrossBodyRawReport(context.Background(), Hellenistic,
		store.AnchorChart{When: natal.when, Place: natal.place}, transit.when, transit.place, aspect.Sun)
	if err == nil {
		t.Error("an unloadable natal zone was accepted by the pair report")
	}
	if command != "" {
		t.Errorf("Astrolog ran (%q) despite the unloadable zone", command)
	}
}
