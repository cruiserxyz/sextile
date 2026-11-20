package astrolog

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/cruiserxyz/sextile/internal/recipe"
)

// the specification. All chart values in this file are constructed
// synthetic test inputs.

// ---- helpers ---------------------------------------------------------------

// boxBorder is Astrolog's centre box inner border, the line the reader counts
// from. boxRow is an ordinary row of the box.
const boxBorder = "<2> 0Tau00'00\"-----|-------------------------------------|----- 0Vir00'00\"<6>"

func boxCell(s string) string {
	const w = 37
	if len(s) >= w {
		return s
	}
	pad := w - len(s)
	return strings.Repeat(" ", pad/2) + s + strings.Repeat(" ", pad-pad/2)
}

func boxRow(s string) string {
	return "|                  |" + boxCell(s) + "|                  |"
}

// boxText is a centre box as `astrolog ... -w -b0` prints it. The five lines
// above the border are the ones the reader counts: JD, zodiac, houses, UT and
// zone. top holds the lines above them (title, name, date, place).
func boxText(top []string, zone, ut, houses, zodiac, jd string) string {
	lines := []string{
		"+-------<11> 8Sag31'11\"----<10> 8Sco31'11\"-----<9> 8Lib31'11\"---------------+",
		"<12> 8Cap31'11\"----|-------------------------------------|----- 8Vir31'11\"<8>",
	}
	for _, s := range top {
		lines = append(lines, boxRow(s))
	}
	lines = append(lines,
		"<1> 8Aqu31'11\"-----|"+boxCell(zone)+"|----- 8Leo31'11\"<7>",
		boxRow(ut), boxRow(houses), boxRow(zodiac), boxRow(jd),
		boxBorder,
		boxRow(""),
		"+--------------- 8Ari31'11\"<3>----- 8Tau31'11\"<4>----- 8Gem31'11\"<5>--------+",
	)
	return strings.Join(lines, "\n") + "\n"
}

// stubAstrolog puts an `astrolog` on PATH that prints out whatever the call.
func stubAstrolog(t *testing.T, out string) {
	t.Helper()
	dir := t.TempDir()
	body := filepath.Join(dir, "out.txt")
	if err := os.WriteFile(body, []byte(out), 0o600); err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/sh\ncat '" + body + "'\n"
	if err := os.WriteFile(filepath.Join(dir, "astrolog"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func writeChartFile(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "c.as")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

const readerQB = "-qb Oct 6 2026 12:30:15am DT 1E 4:53E 52:22N\n"

// readerZI and stubTop agree: a stand-in box prints the name and the place rows
// only for a file that names them, as Astrolog does.
const readerZI = "-zi \"N\" \"P\"\n"

var stubTop = []string{"Astrolog 8.00 chart", "N", "Tue Oct  6 2026  0:30:15", "P"}

func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

// ---- C1, C2, C4: values come from fixed lines of the box -------------------

func TestChartFileValuesComeFromFixedBoxLines(t *testing.T) {
	// The row-form subtests run FIRST: each stubs Astrolog in its own scope,
	// and their live halves need the real binary, which the stub below
	// would otherwise shadow for every subtest after it.
	t.Run("offset zone row (-Yz1)", offsetZoneRows)
	t.Run("whole-minute coordinates (-b2)", wholeMinuteRows)
	// The name, the place and the title area carry decoys of every value the
	// reader takes: a Julian Day, a zone line with coordinates, a houses line.
	top := []string{
		"Astrolog 8.00 chart",
		"Julian Day: 2440000.00000",
		"Tue Oct  6 2026  0:30:15",
		"DT Zone 9:00E, 1:00'00E 2:00'00N",
	}
	stubAstrolog(t, boxText(top,
		"DT Zone 1:00E,   4:53'41E 52:22'07N",
		"UT:  1:29:45, Sid.T: 23:48:16",
		"Koch houses",
		"Tropical, Geocentric",
		"Julian Day: 2461319.4376736"))
	p := writeChartFile(t, "@AI780\n"+readerQB+"-zi \"Julian Day: 2440000.00000\" \"Whole houses\"\n")
	cf, err := ResolveChartFile(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	wantJD := 2461319.4376736
	if !near(cf.JD, wantJD) || !cf.WhenUTC.Equal(julianDayToUTC(wantJD)) {
		t.Errorf("moment = JD %v / %v, want the bottom line's JD %v", cf.JD, cf.WhenUTC, wantJD)
	}
	if !near(cf.Lon, 4+53.0/60+41.0/3600) || !near(cf.Lat, 52+22.0/60+7.0/3600) {
		t.Errorf("place = %v, %v, want the zone line's 4:53'41E 52:22'07N", cf.Lon, cf.Lat)
	}
	if !cf.HasHouses || cf.Houses != recipe.Koch {
		t.Errorf("houses = %v (has %v), want Koch from the houses line", cf.Houses, cf.HasHouses)
	}
	want := BoxClock{Weekday: "Tue", Month: "Oct", Day: 6, Year: 2026, Hour: 0, Minute: 30, Second: 15, DST: "DT", Zone: "1:00E", UT: "1:29:45"}
	if cf.Clock != want {
		t.Errorf("clock = %+v, want the date, zone and UT rows %+v", cf.Clock, want)
	}
}

// wholeMinuteRows is a subtest of TestChartFileValuesComeFromFixedBoxLines.
// `-b2` in a chart file hides zero seconds; the box pins come after the file,
// so the file reads exactly as without it.
func wholeMinuteRows(t *testing.T) {
	sameAsWithout(t, "-b2\n")
}

// sameAsWithout reads the chart readerQB states with and without extra lines
// of display switches, and wants the same record from both.
func sameAsWithout(t *testing.T, extra string) {
	t.Helper()
	needAstrolog(t)
	plain, err := ResolveChartFile(context.Background(), writeChartFile(t, "@AI780\n"+readerQB+readerZI))
	if err != nil {
		t.Fatal(err)
	}
	with, err := ResolveChartFile(context.Background(), writeChartFile(t, "@AI780\n"+readerQB+readerZI+extra))
	if err != nil {
		t.Fatalf("with %q: %v", extra, err)
	}
	if !reflect.DeepEqual(with, plain) {
		t.Errorf("with %q: %+v\nwithout: %+v", extra, with, plain)
	}
	if plain.Clock.Second != 15 || plain.Clock.Hour != 0 || plain.Clock.Minute != 30 {
		t.Errorf("the clock is %+v, want 0:30:15 as the file states", plain.Clock)
	}
}

// offsetZoneRows is a subtest of TestChartFileValuesComeFromFixedBoxLines.
// `-Yz1` (and the clock and date forms) in a chart file change only how the box
// prints; the box pins come after the file, so it reads exactly as without them.
func offsetZoneRows(t *testing.T) {
	for _, extra := range []string{"-Yz1\n", "_Yt\n", "=Yd\n", "-b0\n", "=b1\n"} {
		t.Run(strings.TrimSpace(extra), func(t *testing.T) { sameAsWithout(t, extra) })
	}
}

func TestALabelCannotChooseTheMoment(t *testing.T) {
	needAstrolog(t)
	plain, err := ResolveChartFile(context.Background(), writeChartFile(t, "@AI780\n"+readerQB+"-zi \"Plain\" \"Place\"\n"))
	if err != nil {
		t.Fatal(err)
	}
	for _, label := range []string{"Julian Day: 2440000.00000", "x Julian Day: 1.5"} {
		p := writeChartFile(t, "@AI780\n"+readerQB+fmt.Sprintf("-zi %q %q\n", label, label))
		cf, err := ResolveChartFile(context.Background(), p)
		if err != nil {
			t.Fatalf("%q: %v", label, err)
		}
		if !cf.WhenUTC.Equal(plain.WhenUTC) || !near(cf.JD, plain.JD) {
			t.Errorf("a chart named %q opens at JD %v (%v), the file's own moment is %v (%v)", label, cf.JD, cf.WhenUTC, plain.JD, plain.WhenUTC)
		}
	}
}

func TestALabelCannotChooseTheHouses(t *testing.T) {
	needAstrolog(t)
	for _, tc := range []struct {
		c    string
		want recipe.HouseSystem
		has  bool
	}{{"-c Placidus\n", recipe.Placidus, true}, {"", 0, false}, {"-c Koch\n", recipe.Koch, true}} {
		for _, label := range []string{"Whole houses", "x | Equal houses | y", "Veloc. Campanus Houses"} {
			p := writeChartFile(t, "@AI780\n"+readerQB+fmt.Sprintf("-zi %q %q\n", label, label)+tc.c)
			cf, err := ResolveChartFile(context.Background(), p)
			if err != nil {
				t.Fatalf("%q: %v", label, err)
			}
			if cf.HasHouses != tc.has || (tc.has && cf.Houses != tc.want) {
				t.Errorf("%q with %q: houses %v (has %v), want %v (has %v)", label, tc.c, cf.Houses, cf.HasHouses, tc.want, tc.has)
			}
		}
	}
}

// ---- C3, C9: the last -qb, and the listing agrees ---------------------------

var reCusp = regexp.MustCompile(`<[0-9]+>\s*[0-9]+[A-Z][a-z]{2}[0-9]+|[A-Z][a-z]{2} +[0-9]+[A-Z][a-z]{2}[0-9]+`)

func TestRepeatedBirthLinesFollowAstrolog(t *testing.T) {
	needAstrolog(t)
	p := writeChartFile(t, "@AI780\n-qb Jun 1 1990 4:30pm DT 1E 4:53E 52:22N\n-qb Jun 15 1991 9:10am DT 1E 13:24E 52:31N\n-zi \"Two\" \"Lines\"\n-c Placidus\n")
	cf, err := ResolveChartFile(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	if !near(cf.Lon, 13+24.0/60) || !near(cf.Lat, 52+31.0/60) {
		t.Errorf("place = %v, %v, want the last line's 13:24E 52:31N", cf.Lon, cf.Lat)
	}
	// Astrolog's own cast of the file, then a cast of what was read: the house
	// cusps depend on both the moment and the place.
	want := reCusp.FindAllString(astrologOut(t, "-c", "0", "-i", p, "-w"), -1)
	got := reCusp.FindAllString(astrologOut(t, "-c", "0", "-qj", fmt.Sprintf("%.7f", cf.JD),
		"-zl", formatLon(cf.Lon), formatLat(cf.Lat), "-w"), -1)
	if len(want) < 12 {
		t.Fatalf("control: found %d cusps and positions in Astrolog's own cast", len(want))
	}
	sort.Strings(got)
	sort.Strings(want)
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("cusps of what was read\n%v\nAstrolog's cast of the file\n%v", got, want)
	}
}

func TestListingAndOpeningAgree(t *testing.T) {
	needAstrolog(t)
	p := writeChartFile(t, "@AI780\n-qb Jun 1 1990 4:30pm DT 1E 4:53E 52:22N\n-zi \"A\" \"P1\" -zi \"B\" \"P2\"\n-qb Jun 15 1991 9:10am DT 1E 13:24E 52:31N\n")
	h, err := ReadChartFileHeader(p)
	if err != nil {
		t.Fatal(err)
	}
	cf, err := ResolveChartFile(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	if h.Name != "B" || h.PlaceLabel != "P2" || cf.Name != h.Name || cf.PlaceLabel != h.PlaceLabel {
		t.Errorf("labels: listing %q/%q, opening %q/%q, want B/P2 both", h.Name, h.PlaceLabel, cf.Name, cf.PlaceLabel)
	}
	if !near(h.Lon, 13+24.0/60) || !near(h.Lat, 52+31.0/60) || !near(cf.Lon, h.Lon) || !near(cf.Lat, h.Lat) {
		t.Errorf("place: listing %v,%v opening %v,%v, want 13:24E 52:31N both", h.Lon, h.Lat, cf.Lon, cf.Lat)
	}
	if h.Year != 1991 || h.Month != 6 || h.Day != 15 ||
		cf.WhenUTC.Year() != h.Year || int(cf.WhenUTC.Month()) != h.Month || cf.WhenUTC.Day() != h.Day {
		t.Errorf("moment: listing %d-%d-%d, opening %v", h.Year, h.Month, h.Day, cf.WhenUTC)
	}
}

// TestTextArgumentsAreNotSwitches: Astrolog hands each switch its own
// arguments before it looks for the next switch (FProcessSwitches,
// astrolog.cpp), and the quotes are gone by then (NParseCommandLine), so a
// name or a city that spells `-qb` or `-zi` is still only an argument.
func TestTextArgumentsAreNotSwitches(t *testing.T) {
	needAstrolog(t)
	for _, tc := range []struct{ name, body string }{
		{"-zj carries -qb", "@AI780\n-qb Jun 1 1990 4:30pm DT 1E 4:53E 52:22N\n-zi \"Real\" \"Home\"\n-zj \"-qb\" \"Place\"\n"},
		{"-zj carries -zi", "@AI780\n-qb Jun 1 1990 4:30pm DT 1E 4:53E 52:22N\n-zi \"Real\" \"Home\"\n-zj \"-zi\" \"Fake\" -zj \"X\" \"Y\"\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := writeChartFile(t, tc.body)
			h, err := ReadChartFileHeader(p)
			if err != nil {
				t.Fatal(err)
			}
			cf, err := ResolveChartFile(context.Background(), p)
			if err != nil {
				t.Fatal(err)
			}
			if h.Name != "Real" || h.PlaceLabel != "Home" || cf.Name != h.Name || cf.PlaceLabel != h.PlaceLabel {
				t.Errorf("labels: listing %q/%q, opening %q/%q, want Real/Home both", h.Name, h.PlaceLabel, cf.Name, cf.PlaceLabel)
			}
			if h.Year != 1990 || h.Month != 6 || h.Day != 1 || !near(h.Lon, 4+53.0/60) || !near(h.Lat, 52+22.0/60) {
				t.Errorf("listing %d-%d-%d at %v,%v, want 1990-6-1 at 4:53E 52:22N", h.Year, h.Month, h.Day, h.Lon, h.Lat)
			}
			if !near(cf.Lon, h.Lon) || !near(cf.Lat, h.Lat) || cf.WhenUTC.Year() != h.Year {
				t.Errorf("opening %v at %v,%v disagrees with the listing", cf.WhenUTC, cf.Lon, cf.Lat)
			}
		})
	}
}

// ---- C5: Astrolog's argument splitter ---------------------------------------

func TestSingleQuotedLabelsAreRead(t *testing.T) {
	needAstrolog(t)
	for _, tc := range []struct {
		name, zi, wantName, wantPlace string
	}{
		{"single quotes keep a double quote", "-zi 'A\"B' 'Paris'", "A\"B", "Paris"},
		{"several switches on one line", "-zi \"A\" \"Paris\" -zi \"B\" \"Rome\"", "B", "Rome"},
		{"mixed quotes", "-zi 'Solo' \"Mixed Place\"", "Solo", "Mixed Place"},
		{"a double quote inside double quotes", "-zi \"A\"B\" \"C\"", "A\"B", "C"},
		{"a later line still wins", "-zi \"A\" \"Paris\"\n-zi 'B' 'Rome'", "B", "Rome"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := writeChartFile(t, "@AI780\n"+readerQB+tc.zi+"\n")
			// Astrolog's own reading of the file (control).
			out := astrologOut(t, "-i", p, "-R0", "-w")
			if !strings.Contains(out, tc.wantName) || !strings.Contains(out, tc.wantPlace) {
				t.Fatalf("control: Astrolog does not print %q / %q:\n%.600s", tc.wantName, tc.wantPlace, out)
			}
			h, err := ReadChartFileHeader(p)
			if err != nil {
				t.Fatal(err)
			}
			if h.Name != tc.wantName || h.PlaceLabel != tc.wantPlace {
				t.Errorf("listing read %q/%q, want %q/%q", h.Name, h.PlaceLabel, tc.wantName, tc.wantPlace)
			}
			cf, err := ResolveChartFile(context.Background(), p)
			if err != nil {
				t.Fatal(err)
			}
			if cf.Name != tc.wantName || cf.PlaceLabel != tc.wantPlace {
				t.Errorf("opening read %q/%q, want %q/%q", cf.Name, cf.PlaceLabel, tc.wantName, tc.wantPlace)
			}
		})
	}
}

// ---- C6: seconds ------------------------------------------------------------

func TestChartFileKeepsSeconds(t *testing.T) {
	needAstrolog(t)
	for _, coords := range []string{"4:53'41E 52:22'07N", "4:53:41E 52:22:07N"} {
		p := writeChartFile(t, "@AI780\n-qb Oct 6 2026 12:30:15am DT 1E "+coords+"\n-zi \"\" \"\"\n")
		wantLon, wantLat := 4+53.0/60+41.0/3600, 52+22.0/60+7.0/3600
		cf, err := ResolveChartFile(context.Background(), p)
		if err != nil {
			t.Fatalf("%s: %v", coords, err)
		}
		if !near(cf.Lon, wantLon) || !near(cf.Lat, wantLat) {
			t.Errorf("%s: opened at %v, %v, want %v, %v", coords, cf.Lon, cf.Lat, wantLon, wantLat)
		}
		h, err := ReadChartFileHeader(p)
		if err != nil {
			t.Fatalf("%s: listing refused the file: %v", coords, err)
		}
		if !near(h.Lon, wantLon) || !near(h.Lat, wantLat) {
			t.Errorf("%s: listed at %v, %v, want %v, %v", coords, h.Lon, h.Lat, wantLon, wantLat)
		}
	}
}

// ---- C7: a box of the wrong shape is refused --------------------------------

func TestUnexpectedBoxRefuses(t *testing.T) {
	good := func() []string {
		return []string{"DT Zone 1:00E,   4:53'41E 52:22'07N", "UT:  1:29:45, Sid.T: 23:48:16", "Null houses", "Tropical, Geocentric", "Julian Day: 2461319.4376736"}
	}
	box := func(cells []string) string {
		return boxText(stubTop, cells[0], cells[1], cells[2], cells[3], cells[4])
	}
	if cells := good(); cells[2] == "" {
		t.Fatal("control")
	}
	bad := func(i int, v string) string { c := good(); c[i] = v; return box(c) }
	for name, out := range map[string]string{
		"no border at all":               "Julian Day: 2461319.4376736\nNull houses\n",
		"the bottom line is not a JD":    bad(4, "Tropical, Geocentric"),
		"the houses line names none":     bad(2, "Tropical"),
		"the zone line has no coords":    bad(0, "DT Zone 1:00E"),
		"a Julian Day that is no number": bad(4, "Julian Day: soon"),
		"too few lines above":            boxBorder + "\n",
		"zodiac and UT lines swapped":    box([]string{good()[0], good()[3], good()[2], good()[1], good()[4]}),
		"a progressed chart's line":      bad(1, "Prog: Jun  1 1990  4:30pm"),
		"a UT line that is no time":      bad(1, "UT: banana, Sid.T: potato"),
		"a zodiac line naming no centre": bad(3, "Tropical, 4:53E"),
		"a zone with no daylight word":   bad(0, "Zone h1e,   4:53'41E 52:22'07N"),
		"a place to the minute":          bad(0, "DT Zone 1:00E,   4:53E 52:22N"),
		"a UT to the minute":             bad(1, "UT:  1:29, Sid.T: 23:48:16"),
		"a 12-hour date":                 boxText([]string{"Astrolog 8.00 chart", "N", "Tue Oct  6 2026 12:30:15am", "P"}, good()[0], good()[1], good()[2], good()[3], good()[4]),
		"no date row":                    boxText([]string{"Astrolog 8.00 chart", "N", "P"}, good()[0], good()[1], good()[2], good()[3], good()[4]),
	} {
		t.Run(name, func(t *testing.T) {
			stubAstrolog(t, out)
			cf, err := ResolveChartFile(context.Background(), writeChartFile(t, "@AI780\n"+readerQB+readerZI))
			if err == nil {
				t.Fatalf("opened as %+v", cf)
			}
			if !strings.Contains(err.Error(), "chart box") {
				t.Errorf("message %q does not say the box was not understood", err)
			}
		})
	}
	// The control: the same stub with the good cells opens.
	t.Run("the well-shaped box opens", func(t *testing.T) {
		stubAstrolog(t, box(good()))
		if _, err := ResolveChartFile(context.Background(), writeChartFile(t, "@AI780\n"+readerQB+readerZI)); err != nil {
			t.Fatal(err)
		}
	})
}

// ---- C8: the marked fixture library reads as before -------------------------

func TestFixtureLibraryReadsUnchanged(t *testing.T) {
	needAstrolog(t)
	utc := func(y int, mo time.Month, d, h, mi int) time.Time { return time.Date(y, mo, d, h, mi, 0, 0, time.UTC) }
	type fixtureWant struct {
		file      string
		when      time.Time
		jd        float64
		lon, lat  float64
		name, plc string
		houses    recipe.HouseSystem
		hasHouses bool
	}
	tcs := []fixtureWant{
		{"Eclipse-2026-08-12.as", utc(2026, 8, 12, 17, 37), 2461265.23403, 8.716666666666667, 52.85, "", "Bassum, Germany", 0, false},
		{"Equinox-2026-03.as", utc(2026, 3, 20, 14, 46), 2461120.11528, 11.566666666666666, 48.13333333333333, "", "Munich, Germany", 0, true},
		{"Equinox-2026-09.as", utc(2026, 9, 23, 0, 6), 2461306.50417, 8.8, 53.06666666666667, "", "Bremen, Germany", 0, false},
		{"FullMoon-2026-02.as", utc(2026, 2, 1, 22, 9), 2461073.42292, 9.016666666666667, 53, "", "", 14, true},
		{"GreatConjunction-2020-12-21.as", utc(2020, 12, 21, 18, 19), 2459205.26319, 8.666666666666666, 50.1, "Great Conjunction", "Frankfurt, Germany", 2, true},
		{"Solstice-2025-12.as", utc(2025, 12, 21, 15, 3), 2461031.12708, 11.566666666666666, 48.13333333333333, "", "Munich, Germany", 0, true},
	}
	for _, tc := range tcs {
		cf, err := ResolveChartFile(context.Background(), fixture(tc.file))
		if err != nil {
			t.Errorf("%s: %v", tc.file, err)
			continue
		}
		if !cf.WhenUTC.Equal(tc.when) || math.Abs(cf.JD-tc.jd) > 1e-5 {
			t.Errorf("%s: moment %v JD %v, want %v JD %v", tc.file, cf.WhenUTC, cf.JD, tc.when, tc.jd)
		}
		if cf.Lon != tc.lon || cf.Lat != tc.lat {
			t.Errorf("%s: place %v, %v, want %v, %v", tc.file, cf.Lon, cf.Lat, tc.lon, tc.lat)
		}
		if cf.Name != tc.name || cf.PlaceLabel != tc.plc || cf.Houses != tc.houses || cf.HasHouses != tc.hasHouses {
			t.Errorf("%s: %q/%q houses %v(%v), want %q/%q %v(%v)", tc.file, cf.Name, cf.PlaceLabel, cf.Houses, cf.HasHouses, tc.name, tc.plc, tc.houses, tc.hasHouses)
		}
		h, err := ReadChartFileHeader(fixture(tc.file))
		if err != nil {
			t.Errorf("%s: listing: %v", tc.file, err)
			continue
		}
		if h.Lon != tc.lon || h.Lat != tc.lat || h.Name != tc.name || h.PlaceLabel != tc.plc || h.Year != tc.when.Year() {
			t.Errorf("%s: listing read %+v", tc.file, h)
		}
	}
	// The person fixtures: their moments are kept in their files only, so the
	// data can be corrected in one place; here the place, name and year are read.
	persons := []struct {
		file      string
		year      int
		lon, lat  float64
		name, plc string
	}{
		{"Bailey-1880-06-16.as", 1880, -(2 + 14.0/60), 53 + 29.0/60, "Alice A. Bailey", "Manchester, United Kingdom"},
		{"Gauquelin-1928-11-13.as", 1928, 2 + 20.0/60, 48 + 50.0/60, "Michel Gauquelin", "Paris 14e, France"},
		{"Kepler-1571-12-27.as", 1571, 8 + 52.0/60, 48 + 45.0/60, "Johannes Kepler", "Weil der Stadt, Germany"},
		{"Rudhyar-1895-03-23.as", 1895, 2 + 21.0/60, 48 + 51.0/60, "Dane Rudhyar", "Paris, France"},
	}
	for _, p := range persons {
		cf, err := ResolveChartFile(context.Background(), fixture(p.file))
		if err != nil {
			t.Errorf("%s: %v", p.file, err)
			continue
		}
		if math.Abs(cf.Lon-p.lon) > 1e-9 || math.Abs(cf.Lat-p.lat) > 1e-9 || cf.Name != p.name || cf.PlaceLabel != p.plc {
			t.Errorf("%s: %v, %v %q/%q, want %v, %v %q/%q", p.file, cf.Lon, cf.Lat, cf.Name, cf.PlaceLabel, p.lon, p.lat, p.name, p.plc)
		}
		h, err := ReadChartFileHeader(fixture(p.file))
		if err != nil {
			t.Errorf("%s: listing: %v", p.file, err)
			continue
		}
		if h.Name != p.name || h.PlaceLabel != p.plc || h.Year != p.year {
			t.Errorf("%s: listing read %+v", p.file, h)
		}
	}
	// Every file in the library is either in a table or the refused one.
	seen := map[string]bool{"InvalidFile.as": true}
	for _, tc := range tcs {
		seen[tc.file] = true
	}
	for _, p := range persons {
		seen[p.file] = true
	}
	ents, err := os.ReadDir(filepath.Join("testdata", "charts"))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range ents {
		if !seen[e.Name()] {
			t.Errorf("%s is in the fixture library but not in this table", e.Name())
		}
	}
	for _, f := range []string{"InvalidFile.as"} {
		if _, err := ResolveChartFile(context.Background(), fixture(f)); err == nil {
			t.Errorf("%s opened", f)
		}
	}
}
