package astrolog

// the specification C1, C4, N7, N11, N23: every display call pins the
// operator's clock; only the single-chart compute carries -zZ.

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/cruiserxyz/sextile/internal/payload"
	"github.com/cruiserxyz/sextile/internal/recipe"
)

var allClocks = []Clock{
	{Hours24, MonthFirst}, {Hours24, DayFirst}, {Hours12, MonthFirst}, {Hours12, DayFirst},
}

func clockName(c Clock) string {
	h, d := "24h", "month-first"
	if c.Hours == Hours12 {
		h = "12h"
	}
	if c.Dates == DayFirst {
		d = "day-first"
	}
	return h + "/" + d
}

// withClock holds c for the test and restores the default after it.
func withClock(t *testing.T, c Clock) {
	t.Helper()
	SetClock(c)
	t.Cleanup(func() { SetClock(Clock{}) })
}

func wantPinTokens(c Clock) (hours, dates string) {
	hours, dates = "=Yt", "_Yd"
	if c.Hours == Hours12 {
		hours = "_Yt"
	}
	if c.Dates == DayFirst {
		dates = "=Yd"
	}
	return
}

// checkPins requires exactly one hours pin and one dates pin, the preference's,
// and no bare -Yt / -Yd.
func checkPins(t *testing.T, label string, argv []string, c Clock) {
	t.Helper()
	wh, wd := wantPinTokens(c)
	var hours, dates []string
	for _, a := range argv {
		switch a {
		case "=Yt", "_Yt", "-Yt":
			hours = append(hours, a)
		case "=Yd", "_Yd", "-Yd":
			dates = append(dates, a)
		}
	}
	if !slices.Equal(hours, []string{wh}) || !slices.Equal(dates, []string{wd}) {
		t.Errorf("%s [%s]: hours pins %q dates pins %q, want exactly [%s] and [%s]: %q", label, clockName(c), hours, dates, wh, wd, argv)
	}
}

var zonedLondon = recipe.MomentAtPlace{JD: 2450000.5, Lon: 0, Lat: 51.5, TZ: "Europe/London"}

// C1, N7, N11.
func TestEveryDisplayCallPinsTheClock(t *testing.T) {
	for _, c := range allClocks {
		t.Run(clockName(c), func(t *testing.T) {
			withClock(t, c)
			sources := map[string]recipe.Source{
				"single":     zonedLondon,
				"local":      recipe.LocalMomentAtPlace{JD: 2450000.5, Lon: 0, Lat: 51.5, TZ: "Europe/London"},
				"comparison": recipe.Comparison{Primary: fixLondon, Secondary: fixTromso},
				"transit":    recipe.Comparison{Primary: fixLondon, Secondary: fixTromso, Relation: recipe.TransitToNatal},
				"multiwheel": recipe.MultiWheel{Rings: []recipe.Source{fixLondon, fixTromso, fixOslo}},
			}
			for name, src := range sources {
				spec := fixtureSpec(t, src, recipe.Placidus, recipe.Tropical)
				var projs []recipe.Projection
				switch name {
				case "single":
					projs = []recipe.Projection{recipe.Positions{}, recipe.Aspects{}, recipe.ChartRawReport(), recipe.AspectsRawReport(), recipe.WheelImage{Render: recipe.DefaultWheelRender()}}
				case "local":
					projs = []recipe.Projection{recipe.ChartRawReport(), recipe.AspectsRawReport()}
				case "multiwheel":
					projs = []recipe.Projection{recipe.WheelImage{Render: recipe.DefaultWheelRender()}}
				default:
					projs = []recipe.Projection{recipe.Aspects{}, recipe.AspectsRawReport(), recipe.WheelImage{Render: recipe.DefaultWheelRender()}}
				}
				for _, p := range projs {
					plan, err := Compile(spec, p)
					if err != nil {
						t.Fatalf("%s/%s: %v", name, p.Label(), err)
					}
					for _, inv := range plan.Invocations {
						checkPins(t, name+"/"+p.Label(), inv.Argv, c)
						checkPins(t, name+"/"+p.Label()+" call", inv.Call().Argv(), c)
					}
				}
			}
			// The second argv path: the timeline searches.
			natal := &TimelineNatal{When: time.Date(1990, 5, 1, 12, 0, 0, 0, time.UTC), Lon: 4.9, Lat: 52.37}
			req := TimelineRequest{Year: 2026, Month: time.October, Zone: time.UTC, Natal: natal}
			for _, src := range []TimelineSource{TimelineTransit, TimelineSky, TimelineReturn} {
				call, err := TimelineCall(src, TimelineSegment{Offset: 3600}, req)
				if err != nil {
					t.Fatal(err)
				}
				checkPins(t, "timeline/"+src.String(), call.Argv(), c)
			}
		})
	}
	// N11: the cross report runs a positions probe beside the relationship.
	t.Run("cross probe and relationship", func(t *testing.T) {
		needAstrolog(t)
		for _, c := range allClocks {
			withClock(t, c)
			spec := fixtureSpec(t, recipe.Comparison{Primary: fixLondon, Secondary: fixTromso}, recipe.Placidus, recipe.Tropical)
			out, err := ComputeCrossAspects(context.Background(), spec)
			if err != nil {
				t.Fatal(err)
			}
			if len(out.Commands) != 2 {
				t.Fatalf("commands = %d, want probe + relationship: %q", len(out.Commands), out.Commands)
			}
			for i, cmd := range out.Commands {
				var argv []string
				for _, f := range strings.Fields(cmd) {
					argv = append(argv, strings.Trim(f, "'"))
				}
				checkPins(t, "cross command "+string(rune('0'+i)), argv, c)
			}
		}
	})
}

// N23: -zZ is the single-chart compute's alone.
func TestChartZonePinIsChartOnly(t *testing.T) {
	zoned := func(argv []string) bool { return slices.Contains(argv, "-zZ") }
	spec := fixtureSpec(t, zonedLondon, recipe.Placidus, recipe.Tropical)
	for _, p := range []recipe.Projection{recipe.Positions{}, recipe.Aspects{}} {
		if !zoned(compileOne(t, spec, p).Argv) {
			t.Errorf("%s of a zoned chart lacks -zZ", p.Label())
		}
	}
	for _, p := range []recipe.Projection{recipe.ChartRawReport(), recipe.AspectsRawReport(), recipe.WheelImage{Render: recipe.DefaultWheelRender()}} {
		if got := compileOne(t, spec, p).Argv; zoned(got) {
			t.Errorf("%s carries -zZ: %q", p.Label(), got)
		}
	}
	// A zoned source inside a relationship never reaches the argv.
	cmp := fixtureSpec(t, recipe.Comparison{Primary: zonedLondon, Secondary: fixTromso}, recipe.Placidus, recipe.Tropical)
	for _, p := range []recipe.Projection{recipe.Aspects{}, recipe.WheelImage{Render: recipe.DefaultWheelRender()}} {
		if got := compileOne(t, cmp, p).Argv; zoned(got) {
			t.Errorf("comparison %s carries -zZ: %q", p.Label(), got)
		}
	}
	// The wheel differs from an unzoned wheel by nothing at all.
	plain := fixtureSpec(t, recipe.MomentAtPlace{JD: zonedLondon.JD, Lon: zonedLondon.Lon, Lat: zonedLondon.Lat}, recipe.Placidus, recipe.Tropical)
	w := recipe.WheelImage{Render: recipe.DefaultWheelRender()}
	if a, b := compileOne(t, spec, w).Argv, compileOne(t, plain, w).Argv; !slices.Equal(a, b) {
		t.Errorf("zoned wheel argv differs from the unzoned: %q vs %q", a, b)
	}
}

// ---- C4 / N2 / N12 / N16 / N26 ----

type zoneCase struct {
	name, tz string
	utc      time.Time
}

// namedZoneCases: an east and a west zone, summer and winter, a half-hour
// zone, Lord Howe in summer, and the Amsterdam repeated hour.
var namedZoneCases = []zoneCase{
	{"amsterdam summer", "Europe/Amsterdam", time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC)},
	{"amsterdam winter", "Europe/Amsterdam", time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC)},
	{"new york summer", "America/New_York", time.Date(2026, 7, 1, 3, 31, 0, 0, time.UTC)},
	{"new york winter", "America/New_York", time.Date(2026, 1, 15, 3, 31, 0, 0, time.UTC)},
	{"kolkata", "Asia/Kolkata", time.Date(2026, 3, 1, 6, 1, 0, 0, time.UTC)},
	{"kathmandu", "Asia/Kathmandu", time.Date(2026, 3, 1, 6, 1, 0, 0, time.UTC)},
	{"lord howe summer", "Australia/Lord_Howe", time.Date(2026, 1, 10, 12, 1, 0, 0, time.UTC)},
	{"lord howe winter", "Australia/Lord_Howe", time.Date(2026, 7, 10, 12, 1, 0, 0, time.UTC)},
	// Measured by the independent review: unnudged -qj prints cusp 5 as 0Sag19,
	// a chart call on a different instant as 0Sag20. Every call sends one.
	{"review instant 2026-10-05 00:00:16 UTC", "Europe/Amsterdam", time.Date(2026, 10, 5, 0, 0, 16, 0, time.UTC)},
	{"amsterdam repeated hour first", "Europe/Amsterdam", time.Date(2026, 10, 25, 0, 31, 0, 0, time.UTC)},
	{"amsterdam repeated hour second", "Europe/Amsterdam", time.Date(2026, 10, 25, 1, 31, 0, 0, time.UTC)},
}

// zoneTokens is what Compile put after -zZ.
func zoneTokens(t *testing.T, argv []string) (zone, dst string) {
	t.Helper()
	i := slices.Index(argv, "-zZ")
	if i < 0 || i+2 >= len(argv) {
		t.Fatalf("no -zZ <zone> <dst> in %q", argv)
	}
	return argv[i+1], argv[i+2]
}

func zoned(jd float64, tz string) recipe.MomentAtPlace {
	return recipe.MomentAtPlace{JD: jd, Lon: 4.9, Lat: 52.3667, TZ: tz}
}

// independentZZ derives the expected -zZ pair from the IANA data alone, by a
// different route than the compiler: the standard offset is the smallest
// offset the zone holds at any fortnight of the year.
func independentZZ(t *testing.T, tz string, utc time.Time) (zone, dst string, wall time.Time) {
	t.Helper()
	loc, err := time.LoadLocation(tz)
	if err != nil {
		t.Fatal(err)
	}
	wall = utc.In(loc)
	_, total := wall.Zone()
	std := total
	for d := -200; d <= 200; d += 14 {
		if _, o := wall.AddDate(0, 0, d).Zone(); o < std {
			std = o
		}
	}
	if wall.IsDST() && total-std == 3600 {
		return formatSearchZone(std), "1", wall
	}
	return formatSearchZone(total), "0", wall
}

// header24 is the header Astrolog prints for the default clock.
func header24(wall time.Time, dst string, zone string) string {
	marker := "ST"
	if dst == "1" {
		marker = "DT"
	}
	_, total := wall.Zone()
	if dst == "1" {
		total -= 3600
	}
	sign, a := "E", total
	if total <= 0 {
		sign, a = "W", -total
	}
	z := strconvI(a/3600) + sign
	if m := a % 3600 / 60; m != 0 {
		z = strconvI(a/3600) + ":" + two(m) + sign
	}
	return wall.Format("Mon Jan ") + pad2(wall.Day()) + wall.Format(", 2006 ") + pad2(wall.Hour()) + ":" + two(wall.Minute()) + " (" + marker + " Zone " + z + ")"
}

func strconvI(n int) string { return strings.TrimSpace(pad2(n)) }
func pad2(n int) string {
	if n < 10 {
		return " " + string(rune('0'+n))
	}
	return string(rune('0'+n/10)) + string(rune('0'+n%10))
}
func two(n int) string { return string(rune('0'+n/10)) + string(rune('0'+n%10)) }

// headerOf is the "Mon Oct  5, 2026 22:30 (ST Zone 0W)" part of a report's
// first chart line.
func headerOf(t *testing.T, text string) string {
	t.Helper()
	for _, l := range strings.Split(text, "\n") {
		if i := strings.Index(l, "chart for "); i >= 0 {
			rest := l[i+len("chart for "):]
			if j := strings.Index(rest, ")"); j >= 0 {
				return rest[:j+1]
			}
		}
	}
	t.Fatalf("no chart header in:\n%.300s", text)
	return ""
}

func chartSpecFor(t *testing.T, src recipe.Source) recipe.ChartSpec {
	return fixtureSpec(t, src, recipe.Placidus, recipe.Tropical)
}

// C4, N2, N12, N16, N26.
func TestChartCallShowsItsOwnClock(t *testing.T) {
	needAstrolog(t)
	withClock(t, Clock{})

	check := func(t *testing.T, tz string, utc time.Time) {
		t.Helper()
		jd := GregorianToJD(utc)
		wantZone, wantDst, wall := independentZZ(t, tz, utc)

		// The compiled pin is the independently derived one.
		inv := compileOne(t, chartSpecFor(t, zoned(jd, tz)), recipe.Positions{})
		if z, d := zoneTokens(t, inv.Argv); z != wantZone || d != wantDst {
			t.Errorf("%s %s: -zZ %s %s, want %s %s", tz, utc, z, d, wantZone, wantDst)
		}

		// The chart call's own header reads the chart's clock.
		got, err := ComputeChart(context.Background(), chartSpecFor(t, zoned(jd, tz)))
		if err != nil {
			t.Fatal(err)
		}
		if h, want := headerOf(t, got.AspectsReport), header24(wall, wantDst, wantZone); h != want {
			t.Errorf("%s %s: header %q, want %q", tz, utc, h, want)
		}

		// It equals the same chart's local raw-report header.
		local := recipe.LocalMomentAtPlace{JD: jd, Lon: 4.9, Lat: 52.3667, TZ: tz}
		report, _, err := ComputeRawReport(context.Background(), chartSpecFor(t, local), recipe.AspectsRawReport())
		if err != nil {
			t.Fatal(err)
		}
		if a, b := headerOf(t, got.AspectsReport), headerOf(t, report); a != b {
			t.Errorf("%s %s: chart header %q != local raw header %q", tz, utc, a, b)
		}

		compareValues(t, tz, utc, got, true)
	}

	for _, c := range namedZoneCases {
		t.Run(c.name, func(t *testing.T) { check(t, c.tz, c.utc) })
	}

	// The property: every zone the atlas can produce, instants on both sides
	// of each transition in the year and of local midnight. The pin is checked
	// for all of them; the live header for a spread of them (a spawn each).
	zones := atlasZones(t)
	if len(zones) < 100 {
		t.Fatalf("atlas gave %d zones", len(zones))
	}
	live, valueChecks := 0, 0
	valuesDone := map[string]int{}
	for zi, tz := range zones {
		loc, err := time.LoadLocation(tz)
		if err != nil {
			t.Fatalf("atlas zone %s: %v", tz, err)
		}
		for _, utc := range zoneProbeInstants(loc) {
			_, _, wall := independentZZ(t, tz, utc)
			if _, off := wall.Zone(); off%60 != 0 {
				continue
			}
			inv := compileOne(t, chartSpecFor(t, zoned(GregorianToJD(utc), tz)), recipe.Positions{})
			wz, wd, _ := independentZZ(t, tz, utc)
			if z, d := zoneTokens(t, inv.Argv); z != wz || d != wd {
				t.Errorf("%s %s: -zZ %s %s, want %s %s", tz, utc, z, d, wz, wd)
			}
			// Values (C4 as a property): at EVERY probe instant, both sides of
			// every transition in the year and of local midnight, for every zone.
			valuesDone[tz]++
			valueChecks++
			got, err := ComputeChart(context.Background(), chartSpecFor(t, zoned(GregorianToJD(utc), tz)))
			if err != nil {
				t.Fatalf("%s %s: %v", tz, utc, err)
			}
			compareValues(t, tz, utc, got, true)
			if zi%12 == 0 && live < 40 {
				live++
				out := runInvocation(t, inv)
				if h, want := headerOf(t, out), header24(wall, wd, wz); h != want {
					t.Errorf("%s %s: header %q, want %q", tz, utc, h, want)
				}
			}
		}
	}
	if live < 20 {
		t.Errorf("only %d live headers checked", live)
	}
	if len(valuesDone) != len(zones) {
		t.Errorf("values compared for %d of %d zones", len(valuesDone), len(zones))
	}
	t.Logf("value comparisons: %d instants over %d zones", valueChecks, len(valuesDone))
}

// compareValues is C4's value clause: the chart call (with -zZ) parses to the
// same positions, cusps, aspects, styles and unavailable set as the same call
// without -zZ and, when wheel is set, as the call every other projection of the
// slot makes (no -zZ at all): the argv-minus-zZ twin cannot see an instant the compiler itself shifted.
func compareValues(t *testing.T, tz string, utc time.Time, got payload.RawOutput, wheel bool) {
	t.Helper()
	jd := GregorianToJD(utc)
	plain := compileOne(t, chartSpecFor(t, zoned(jd, tz)), recipe.Positions{})
	var argv []string
	for i := 0; i < len(plain.Argv); i++ {
		if plain.Argv[i] == "-zZ" {
			i += 2
			continue
		}
		argv = append(argv, plain.Argv[i])
	}
	bare, err := Run(context.Background(), newBodyCall(argv, plain.call.band))
	if err != nil {
		t.Fatalf("%s %s: %v", tz, utc, err)
	}
	if wheel {
		wheelPath, err := ComputeChart(context.Background(), chartSpecFor(t, recipe.MomentAtPlace{JD: jd, Lon: 4.9, Lat: 52.3667}))
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got.Cusps, wheelPath.Cusps) || !reflect.DeepEqual(got.Positions, wheelPath.Positions) {
			t.Errorf("%s %s: the chart call's cusps/positions differ from the wheel path's", tz, utc)
		}
	}
	for name, pair := range map[string][2]any{
		"positions": {got.Positions, bare.Positions}, "cusps": {got.Cusps, bare.Cusps},
		"aspects": {got.Aspects, bare.Aspects}, "body styles": {got.BodyStyles, bare.BodyStyles},
		"aspect styles": {got.AspectStyles, bare.AspectStyles}, "unavailable": {got.Unavailable, bare.Unavailable},
	} {
		if !reflect.DeepEqual(pair[0], pair[1]) {
			t.Errorf("%s %s: %s differ with -zZ", tz, utc, name)
		}
	}
}

// zoneProbeInstants are whole-minute instants a minute either side of each
// transition in 2026 and of local midnight on the first day of each month.
func zoneProbeInstants(loc *time.Location) []time.Time {
	var out []time.Time
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, loc)
	end := time.Date(2027, 1, 1, 0, 0, 0, 0, loc)
	for cur := start; cur.Before(end); {
		_, next := cur.ZoneBounds()
		if next.IsZero() || !next.Before(end) {
			break
		}
		out = append(out, next.Add(-time.Minute).UTC(), next.UTC())
		cur = next
	}
	for m := time.January; m <= time.December; m += 3 {
		mid := time.Date(2026, m, 1, 0, 0, 0, 0, loc)
		out = append(out, mid.Add(-time.Minute).UTC(), mid.Add(time.Minute).UTC())
	}
	return out
}

// atlasZones are the distinct IANA zones in the installed atlas.as (its fifth
// column), which is where every place Sextile can show gets its zone.
func atlasZones(t *testing.T) []string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(originalDataDir, "atlas.as"))
	if err != nil {
		t.Skipf("no atlas.as: %v", err)
	}
	seen := map[string]bool{}
	var out []string
	for _, l := range strings.Split(string(b), "\n") {
		if l == "" || l[0] == ';' || l[0] == '@' {
			continue
		}
		cols := strings.Split(l, "\t")
		if len(cols) >= 5 {
			if z := strings.TrimSpace(cols[4]); z != "" && !seen[z] {
				seen[z] = true
				out = append(out, z)
			}
		}
	}
	slices.Sort(out)
	return out
}

// N27: a pair moved to local-clock chart files keeps the UT positions probe
// that refuses a disabled ephemeris, and the relationship never runs after it.
func TestPairReportRefusesADisabledEphemeris(t *testing.T) {
	dir := t.TempDir()
	collapsed := writeTestFile(t, dir, "collapsed.txt", "Sun :  0Ari00   - 0:00' (-) [ 1st house]\nMoon:  0Ari00   - 0:00' (-) [ 1st house]\n")
	_, argvLog := fakeBinary(t, dir, "cat '"+collapsed+"'")
	pair := recipe.Comparison{
		Primary:   recipe.LocalMomentAtPlace{JD: 2450000.5, Lon: 0, Lat: 51.5, TZ: "Europe/London"},
		Secondary: recipe.LocalMomentAtPlace{JD: 2451545.25, Lon: 18.95, Lat: 69.6, TZ: "Europe/Oslo"},
		Relation:  recipe.Synastry,
	}
	_, err := ComputeCrossAspects(context.Background(), fixtureSpec(t, pair, recipe.Placidus, recipe.Tropical))
	if !errors.Is(err, ErrEphemerisDisabled) {
		t.Fatalf("err = %v, want ErrEphemerisDisabled", err)
	}
	if n := len(spawnArgvs(t, argvLog)); n != 1 {
		t.Errorf("%d processes started, want only the probe", n)
	}
}

// reviewInstant is the measured case: unnudged `-qj 2461318.500185` prints house
// cusp 5 as 0Sag19, the nudged `2461318.500186` as 0Sag20.
var reviewInstant = time.Date(2026, 10, 5, 0, 0, 16, 0, time.UTC)

func qjWord(t *testing.T, label string, argv []string) string {
	t.Helper()
	i := slices.Index(argv, "-qj")
	if i < 0 || i+1 >= len(argv) {
		t.Fatalf("%s: no -qj in %q", label, argv)
	}
	return argv[i+1]
}

// Every call for one slot asks Astrolog the identical instant: the slot's
// instant TRUNCATED to the whole second (the second Sextile displays and the
// timeline's -qb natal already sends), one float above its exact Julian Day.
func TestEveryCallSendsOneInstant(t *testing.T) {
	sec := func(y int, mo time.Month, d, h, mi, s int) time.Time {
		return time.Date(y, mo, d, h, mi, s, 0, time.UTC)
	}
	hairBelow := func(w time.Time) float64 { return math.Nextafter(GregorianToJD(w), math.Inf(-1)) }
	cases := []struct {
		name string
		jd   float64
		want time.Time // the second that must be sent
	}{
		{"review 00:00:16", GregorianToJD(reviewInstant), reviewInstant},
		{"00:00:16.4", GregorianToJD(reviewInstant.Add(400 * time.Millisecond)), reviewInstant},
		{"00:06:59.5 stays 59", GregorianToJD(time.Date(2026, 10, 5, 0, 6, 59, 500_000_000, time.UTC)), sec(2026, 10, 5, 0, 6, 59)},
		{"x.999 stays x", GregorianToJD(time.Date(2026, 10, 5, 3, 4, 5, 999_000_000, time.UTC)), sec(2026, 10, 5, 3, 4, 5)},
		{"23:59:59.6 stays the same day", GregorianToJD(time.Date(2026, 10, 5, 23, 59, 59, 600_000_000, time.UTC)), sec(2026, 10, 5, 23, 59, 59)},
		{"23:59:59.9995 through SlotJD stays 59 the same day", SlotJD(time.Date(2026, 10, 5, 23, 59, 59, 999_500_000, time.UTC)), sec(2026, 10, 5, 23, 59, 59)},
		{"pre-1970 with a fraction", GregorianToJD(time.Date(1955, 3, 4, 10, 20, 30, 700_000_000, time.UTC)), sec(1955, 3, 4, 10, 20, 30)},
		{"whole minute", GregorianToJD(sec(2026, 10, 5, 0, 31, 0)), sec(2026, 10, 5, 0, 31, 0)},
		{"a whole second a hair below", hairBelow(sec(2026, 10, 5, 0, 31, 0)), sec(2026, 10, 5, 0, 31, 0)},
		{"a whole second a hair below, pre-1970", hairBelow(sec(1955, 3, 4, 10, 20, 30)), sec(1955, 3, 4, 10, 20, 30)},
	}
	// The slot's ORIGINAL time.Time, for the cases that have one: handed to the
	// timeline as is, never rebuilt from the already-truncated Julian Day.
	originals := map[string]time.Time{
		"23:59:59.9995 through SlotJD stays 59 the same day": time.Date(2026, 10, 5, 23, 59, 59, 999_500_000, time.UTC),
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			want := strconv.FormatFloat(math.Nextafter(GregorianToJD(c.want), math.Inf(1)), 'f', -1, 64)
			zonedSpec := fixtureSpec(t, zoned(c.jd, "Europe/Amsterdam"), recipe.Placidus, recipe.Tropical)
			plainSpec := fixtureSpec(t, recipe.MomentAtPlace{JD: c.jd, Lon: 4.9, Lat: 52.3667}, recipe.Placidus, recipe.Tropical)
			got := map[string]string{}
			for name, spec := range map[string]recipe.ChartSpec{"zoned": zonedSpec, "plain": plainSpec} {
				for _, p := range []recipe.Projection{recipe.Positions{}, recipe.Aspects{}, recipe.ChartRawReport(), recipe.AspectsRawReport(), recipe.WheelImage{Render: recipe.DefaultWheelRender()}} {
					got[name+"/"+p.Label()] = qjWord(t, name+"/"+p.Label(), compileOne(t, spec, p).Argv)
				}
				bundle, err := CompileBundle(spec, recipe.Positions{}, recipe.WheelImage{Render: recipe.DefaultWheelRender()})
				if err != nil {
					t.Fatal(err)
				}
				for i, plan := range bundle.Plans {
					got[name+"/bundle"+strconv.Itoa(i)] = qjWord(t, name+" bundle", plan.Invocations[0].Argv)
				}
			}
			for k, v := range got {
				if v != want {
					t.Errorf("%s sends -qj %s, want the truncated second %s (%s)", k, v, want, c.want)
				}
			}

			// The staged files (local pair files, UT comparison files) name the same second.
			f, err := localMomentFields(recipe.LocalMomentAtPlace{JD: c.jd, Lon: 4.9, Lat: 52.37, TZ: "UTC"})
			if err != nil {
				t.Fatal(err)
			}
			if wantClock := fmt.Sprintf("%d:%02d:%02d", c.want.Hour(), c.want.Minute(), c.want.Second()); f[3] != wantClock {
				t.Errorf("localMomentFields clock %q, want the second of %s", f[3], c.want)
			}
			ut, err := FormatChartFile(recipe.MomentAtPlace{JD: c.jd, Lon: 4.9, Lat: 52.37}, recipe.Placidus, recipe.Tropical)
			if err != nil {
				t.Fatal(err)
			}
			// the specification C1: the staged file names the second as the very
			// -qj every other call sends, never as calendar fields.
			if !strings.Contains(ut, "\n-qj "+want+"\n") {
				t.Errorf("FormatChartFile does not name -qj %s:\n%s", want, ut)
			}

			// The timeline's -qj natal for the same slot names the same second.
			natalWhen := julianDayToTimeForTest(c.jd).Round(100 * time.Microsecond)
			if orig, ok := originals[c.name]; ok {
				natalWhen = orig // the original, not a rebuild from the truncated Julian Day
			}
			call, err := TimelineCall(TimelineTransit, TimelineSegment{Offset: 0}, TimelineRequest{
				Year: 2026, Month: time.October, Zone: time.UTC,
				Natal: &TimelineNatal{When: natalWhen, Lon: 4.9, Lat: 52.37},
			})
			if err != nil {
				t.Fatal(err)
			}
			if got := qjWord(t, "timeline natal", call.Argv()); got != want {
				t.Errorf("timeline natal sends -qj %s, want the truncated second %s (%s)", got, want, c.want)
			}
		})
	}
}

// julianDayToTimeForTest is the slot's instant as the UI holds it: the exact
// time, with its sub-second part, that the Julian Day stands for.
func julianDayToTimeForTest(jd float64) time.Time {
	ns := math.Round((jd - jdUnixEpoch) * 86400e9)
	return time.Unix(0, int64(ns)).UTC()
}
