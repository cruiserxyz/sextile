package astrolog

// the specification: a chart before the Gregorian reform shows and finds the
// date Astrolog gives it. C1 TestNoGoCalendarFieldsReachAstrolog, C2
// TestSlotJDIsExactForEveryYear, C3 TestPreReformChartIsCastForItsMoment, C4
// TestLabelMatchesAstrolog, C5 TestParseMatchesAstrolog, C6
// TestReformDateIsPinned, C8 TestPreReformTimelineMatchesAstrolog, N7
// TestOldTimelineRowRefusesAnImpossibleDate.

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/big"
	"math/rand"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/cruiserxyz/sextile/internal/recipe"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// ---- C2 --------------------------------------------------------------------

// The Unix formula, written out in the test: the code under test is not asked
// to agree with itself.
func unixFormulaJD(t time.Time) float64 { return float64(t.Unix())/86400.0 + 2440587.5 }

func TestSlotJDIsExactForEveryYear(t *testing.T) {
	clocks := [][3]int{{0, 0, 0}, {0, 0, 1}, {12, 0, 0}, {23, 59, 59}}
	check := func(year int, month time.Month, day int) {
		for _, c := range clocks {
			when := time.Date(year, month, day, c[0], c[1], c[2], 0, time.UTC)
			got, want := SlotJD(when), unixFormulaJD(when)
			if math.Abs(got-want) > 1e-9 {
				t.Errorf("SlotJD(%d-%02d-%02d %02d:%02d:%02d) = %.9f, want %.9f (off by %.3f days)",
					year, int(month), day, c[0], c[1], c[2], got, want, got-want)
			}
		}
	}
	for year := -4700; year <= 3000; year++ {
		check(year, time.March, 1)
		check(year, time.December, 31)
	}
	// The years the truncating formula was a day late in, by name.
	for _, year := range []int{-100, -200, -300, -400, -500} {
		check(year, time.March, 1)
	}
}

// momentJD is what a call sends for a slot's Julian Day, for a Julian Day known
// to be sendable.
func momentJD(jd float64) string {
	s, err := momentArg(jd)
	if err != nil {
		panic(err)
	}
	return s
}

// ---- helpers ---------------------------------------------------------------

// astrologOut runs the real binary on a bare argv and returns its output.
func astrologOut(t *testing.T, argv ...string) string {
	t.Helper()
	out, _, err := invoke(context.Background(), argv)
	if err != nil {
		t.Fatalf("astrolog %q: %v\n%.300s", argv, err, out)
	}
	return out
}

// julianNoon is the Julian Day number of a Julian-calendar date, by Meeus's
// formula (AA 2e ch. 7, with B = 0): written here and not in the code under
// test.
func julianJD(year, month, day int, hour float64) float64 {
	y, m := float64(year), float64(month)
	if month <= 2 {
		y, m = y-1, m+12
	}
	return math.Floor(365.25*(y+4716)) + math.Floor(30.6001*(m+1)) + float64(day) - 1524.5 + hour/24
}

var reHeader = regexp.MustCompile(`chart for ([A-Z][a-z]{2}) ([A-Z][a-z]{2}) +([0-9]+), (-?[0-9]+) +([0-9]+):([0-9]{2}) \(`)

var monthNames = []string{"", "Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"}

// headerFields reads the header date and clock of a `chart for` line.
func headerFields(t *testing.T, line string) (wd string, d CalendarDate) {
	t.Helper()
	m := reHeader.FindStringSubmatch(line)
	if m == nil {
		t.Fatalf("no 24-hour chart header in %q", line)
	}
	d.Month = slices.Index(monthNames, m[2])
	d.Day, _ = strconv.Atoi(m[3])
	d.Year, _ = strconv.Atoi(m[4])
	d.Hour, _ = strconv.Atoi(m[5])
	d.Minute, _ = strconv.Atoi(m[6])
	return m[1], d
}

func sameStamp(a CalendarDate, wd string, b CalendarDate) bool {
	return a.Year == b.Year && a.Month == b.Month && a.Day == b.Day && a.Hour == b.Hour && a.Minute == b.Minute &&
		a.Weekday.String()[:3] == wd
}

func describe(d CalendarDate) string {
	return fmt.Sprintf("%s %s %d, %d %d:%02d", d.Weekday.String()[:3], monthNames[d.Month], d.Day, d.Year, d.Hour, d.Minute)
}

// ---- C4 --------------------------------------------------------------------

// The offsets C4 names, in seconds east: 0, +5, -8, +5:45, +14, -12.
var labelOffsets = []int{0, 5 * 3600, -8 * 3600, 5*3600 + 45*60, 14 * 3600, -12 * 3600}

func TestLabelMatchesAstrolog(t *testing.T) {
	needAstrolog(t)
	withClock(t, Clock{})
	type sample struct {
		jd  float64
		off int
	}
	var samples []sample
	// Every quarter day over the reform, at every offset.
	for jd := 2299150.0; jd <= 2299185.0; jd += 0.25 {
		for _, off := range labelOffsets {
			samples = append(samples, sample{jd, off})
		}
	}
	// 400 seeded random Julian Days from 0 to 2500000.
	rng := rand.New(rand.NewSource(20261011))
	for i := 0; i < 400; i++ {
		samples = append(samples, sample{math.Floor(rng.Float64()*2500000*1e5) / 1e5, labelOffsets[rng.Intn(len(labelOffsets))]})
	}
	// 00:30 and 23:30 UT on 28 Feb, 29 Feb and 1 Mar, Julian, of 1500 and -100.
	for _, year := range []int{1500, -100} {
		for _, day := range [][2]int{{2, 28}, {2, 29}, {3, 1}} {
			for _, hour := range []float64{0.5, 23.5} {
				for _, off := range labelOffsets {
					samples = append(samples, sample{julianJD(year, day[0], day[1], hour) + 0.5 - 0.5, off})
				}
			}
		}
	}
	// The instants around a pre-reform midnight, as Sextile sends them.
	midnight := int64(math.Round((2299000.5 - 2440587.5) * 86400))
	for _, d := range []int64{-1, 0, 1} {
		for _, off := range labelOffsets {
			samples = append(samples, sample{sentJD(midnight + d), off})
		}
	}
	for _, s := range samples {
		argv := []string{"-qj", strconv.FormatFloat(s.jd, 'f', -1, 64)}
		if s.off != 0 {
			argv = append(argv, "-zZ", formatSearchZone(s.off), "0")
		}
		argv = append(argv, "=Yt", "_Yd", "-v0")
		out := astrologOut(t, argv...)
		wd, want := headerFields(t, out)
		got := AstrologCalendar{}.Label(s.jd, s.off)
		if !sameStamp(got, wd, want) {
			t.Errorf("Label(%v, %+d) = %s, Astrolog prints %s %s %d, %d %d:%02d",
				s.jd, s.off, describe(got), wd, monthNames[want.Month], want.Day, want.Year, want.Hour, want.Minute)
		}
		if wantJulian := math.Floor(s.jd+0.5) < 2299171; got.Julian != wantJulian {
			t.Errorf("Label(%v, %+d).Julian = %v, want %v", s.jd, s.off, got.Julian, wantJulian)
		}
	}
	// N2: Label is not Parse's inverse. Inside 5-14 Oct 1582 the header names
	// the Julian day and the search names the Gregorian one.
	if got := (AstrologCalendar{}).Label(2299160.5, 0); got.Month != 10 || got.Day != 5 || got.Year != 1582 || !got.Julian {
		t.Errorf("Label(2299160.5, 0) = %s julian=%v, want Oct 5, 1582 on the Julian branch", describe(got), got.Julian)
	}
}

// ---- C5 --------------------------------------------------------------------

var reRow = regexp.MustCompile(`^ *(?:[A-Z][a-z]{2} +)?(?:(\d+)/ *(\d+)/ *(-?\d+)) +(\d+):(\d{2})`)

func rowFields(t *testing.T, line string) Fields {
	t.Helper()
	m := reRow.FindStringSubmatch(line)
	if m == nil {
		t.Fatalf("no month-first 24-hour date in row %q", line)
	}
	n := make([]int, 5)
	for i := range n {
		n[i], _ = strconv.Atoi(m[i+1])
	}
	return Fields{Month: n[0], Day: n[1], Year: n[2], Hour: n[3], Minute: n[4]}
}

func TestParseMatchesAstrolog(t *testing.T) {
	needAstrolog(t)
	withClock(t, Clock{})
	months := [][2]int{{10, 1582}, {2, 1500}, {3, -100}, {10, 2026}}
	for _, off := range []int{0, 5 * 3600, 1172 /* Amsterdam LMT 0:19:32E */} {
		for _, my := range months {
			mo, yr := strconv.Itoa(my[0]), strconv.Itoa(my[1])
			zone := formatSearchZone(off)
			searches := map[string][]string{
				"-dm": {"-z", zone, "-qm", mo, yr, "-dm", "=Yt", "_Yd"},
				"-t":  {"-z", zone, "-qj", "2440000.5", "-zl", "4:54E", "52:22N", "-t", mo, yr, "=Yt", "_Yd"},
			}
			for name, argv := range searches {
				out := astrologOut(t, argv...)
				rows := 0
				for _, line := range strings.Split(out, "\n") {
					if !reRow.MatchString(line) {
						continue
					}
					rows++
					f := rowFields(t, line)
					got, err := AstrologCalendar{}.Parse(f, off)
					if err != nil {
						t.Errorf("%s %s/%s off %d: Parse(%+v): %v", name, mo, yr, off, f, err)
						continue
					}
					// Astrolog reads the same fields back: -qa <fields> <zone>.
					back := astrologOut(t, "-qa", strconv.Itoa(f.Month), strconv.Itoa(f.Day), strconv.Itoa(f.Year),
						fmt.Sprintf("%d:%02d", f.Hour, f.Minute), zone, "0E", "0N", "-w", "-R0")
					m := reJulianDay.FindStringSubmatch(back)
					if m == nil {
						t.Fatalf("no Julian Day for %+v:\n%.200s", f, back)
					}
					want, _ := strconv.ParseFloat(m[1], 64)
					if gotJD := GregorianToJD(got.Instant); math.Abs(gotJD-want) > 6e-6 {
						t.Errorf("%s %s/%s off %d: Parse(%+v) is JD %.5f, Astrolog reads %.5f", name, mo, yr, off, f, gotJD, want)
					}
				}
				if rows == 0 {
					t.Errorf("%s %s/%s off %d: no rows to compare", name, mo, yr, off)
				}
			}
		}
	}
}

// ---- C1, N1, N9 -------------------------------------------------------------

var calendarFieldSwitches = []string{"-qa", "-qb", "-qc"}

// noCalendarFields fails when any token or line of an argv or a staged file is a
// chart-entry switch that carries calendar fields. C6's `-qag` is the reform pin,
// not a moment.
func noCalendarFields(t *testing.T, what string, tokens []string) {
	t.Helper()
	for _, tok := range tokens {
		if slices.Contains(calendarFieldSwitches, tok) {
			t.Errorf("%s carries %s: a moment built from Go's calendar fields", what, tok)
		}
	}
}

func stagedLines(content string) []string {
	var out []string
	for _, l := range strings.Split(content, "\n") {
		out = append(out, strings.Fields(l)...)
	}
	return out
}

func TestNoGoCalendarFieldsReachAstrolog(t *testing.T) {
	instants := map[string]time.Time{
		"1500":  time.Date(1500, 6, 15, 12, 30, 45, 0, time.UTC),
		"0":     time.Date(0, 3, 1, 0, 0, 1, 0, time.UTC),
		"-100":  time.Date(-100, 3, 1, 23, 59, 59, 0, time.UTC),
		"2026":  time.Date(2026, 10, 11, 9, 5, 0, 0, time.UTC),
		"-4000": time.Date(-4000, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	for name, when := range instants {
		jd := SlotJD(when)
		exact := exactJD(when)
		want := strconv.FormatFloat(exact, 'f', -1, 64)
		local := recipe.LocalMomentAtPlace{JD: jd, Lon: 4.9, Lat: 52.37, TZ: "Etc/GMT-5"}
		utc := recipe.MomentAtPlace{JD: jd, Lon: 4.9, Lat: 52.37}
		other := recipe.MomentAtPlace{JD: SlotJD(time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)), Lon: 10, Lat: 50}

		t.Run(name+"/raw-report", func(t *testing.T) {
			spec := fixtureSpec(t, local, recipe.Placidus, recipe.Tropical)
			for _, p := range []recipe.Projection{recipe.ChartRawReport(), recipe.AspectsRawReport()} {
				inv := compileOne(t, spec, p)
				noCalendarFields(t, "raw report "+p.Label(), inv.Argv)
				if got := qjWord(t, "raw report "+p.Label(), inv.Argv); !jdClose(got, exact) {
					t.Errorf("raw report %s sends -qj %s, want %s", p.Label(), got, want)
				}
			}
		})
		t.Run(name+"/staged-files", func(t *testing.T) {
			for label, src := range map[string]recipe.Source{
				"local pair": recipe.Comparison{Primary: local, Secondary: utc},
				"ut pair":    recipe.Comparison{Primary: utc, Secondary: other},
				"ut ring":    recipe.MultiWheel{Rings: []recipe.Source{utc, other, local}},
			} {
				spec := fixtureSpec(t, src, recipe.Placidus, recipe.Tropical)
				p := recipe.Projection(recipe.AspectsRawReport())
				if label == "ut ring" {
					p = recipe.WheelImage{Render: recipe.DefaultWheelRender()}
				}
				inv := compileOne(t, spec, p)
				if len(inv.TempInputs) < 2 {
					t.Fatalf("%s: %d staged files", label, len(inv.TempInputs))
				}
				found := false
				for i, temp := range inv.TempInputs {
					noCalendarFields(t, fmt.Sprintf("%s staged file %d", label, i), stagedLines(temp.Content))
					for _, w := range stagedQJ(temp.Content) {
						if jdClose(w, exact) {
							found = true
						}
					}
				}
				if !found {
					t.Errorf("%s: no staged file names -qj %s:\n%s", label, want, inv.TempInputs[0].Content)
				}
			}
			// N1: the staged local file gets its local header from -zZ in the file.
			for _, form := range []func() (string, error){
				func() (string, error) { return FormatLocalChartFile(local, recipe.Placidus, recipe.Tropical) },
			} {
				content, err := form()
				if err != nil {
					t.Fatal(err)
				}
				noCalendarFields(t, "FormatLocalChartFile", stagedLines(content))
				if !strings.Contains(content, "\n-zZ ") {
					t.Errorf("the staged local file has no -zZ line, so its header would be UT:\n%s", content)
				}
			}
			content, err := FormatChartFile(utc, recipe.Placidus, recipe.Tropical)
			if err != nil {
				t.Fatal(err)
			}
			noCalendarFields(t, "FormatChartFile", stagedLines(content))
			if w := stagedQJ(content); len(w) != 1 || !jdClose(w[0], exact) {
				t.Errorf("FormatChartFile does not name -qj %s:\n%s", want, content)
			}
		})
		t.Run(name+"/timeline-natal", func(t *testing.T) {
			if when.Before(time.Date(-4700, 1, 1, 0, 0, 0, 0, time.UTC)) {
				t.Skip("the month itself is before Julian Day 0 for the request below")
			}
			req := TimelineRequest{Year: 2026, Month: time.October, Zone: time.UTC, Natal: &TimelineNatal{When: when, Lon: 4.9, Lat: 52.37}}
			for _, src := range []TimelineSource{TimelineTransit, TimelineReturn} {
				call, err := TimelineCall(src, TimelineSegment{Offset: 0}, req)
				if err != nil {
					t.Fatal(err)
				}
				noCalendarFields(t, "timeline natal "+src.String(), call.Argv())
				if got := qjWord(t, "timeline natal", call.Argv()); !jdClose(got, exact) {
					t.Errorf("timeline natal sends -qj %s, want %s", got, want)
				}
			}
		})
	}
}

// exactJD is the Julian Day of an instant by exact arithmetic in the test:
// floor(sec/86400) + 2440587.5 + (sec mod 86400)/86400, as a rational rounded
// once to the nearest float. It shares nothing with the code under test.
func exactJD(when time.Time) float64 {
	sec := when.Truncate(time.Second).Unix()
	days := sec / 86400
	if sec%86400 < 0 {
		days--
	}
	rem := sec - days*86400
	r := new(big.Rat).Add(big.NewRat(days, 1), big.NewRat(4881175, 2))
	r.Add(r, big.NewRat(rem, 86400))
	f, _ := r.Float64()
	return f
}

// jdClose says whether a sent -qj word is the exact Julian Day, or at most two
// floats above it (production sends one float up, to keep the printed minute).
func jdClose(word string, exact float64) bool {
	got, err := strconv.ParseFloat(word, 64)
	if err != nil {
		return false
	}
	hi := math.Nextafter(math.Nextafter(exact, math.Inf(1)), math.Inf(1))
	lo := math.Nextafter(exact, math.Inf(-1))
	return got >= lo && got <= hi
}

// stagedQJ are the operands of a staged file's -qj lines.
func stagedQJ(content string) []string {
	var out []string
	for _, l := range strings.Split(content, "\n") {
		if w, ok := strings.CutPrefix(l, "-qj "); ok {
			out = append(out, w)
		}
	}
	return out
}

// ---- C3 --------------------------------------------------------------------

// runInvocation runs a compiled invocation with its staged files and extra
// switches appended, and returns the text.
func runStaged(t *testing.T, inv Invocation, extra ...string) string {
	t.Helper()
	argv := slices.Clone(inv.Argv)
	dir := t.TempDir()
	for i, temp := range inv.TempInputs {
		p := filepath.Join(dir, fmt.Sprintf("chart%d.as", i))
		if err := os.WriteFile(p, []byte(temp.Content), 0o600); err != nil {
			t.Fatal(err)
		}
		argv[temp.ArgIndex] = p
	}
	return astrologOut(t, append(argv, extra...)...)
}

// headerLines are the `chart for` / `comparison for` lines of a text, in order.
func headerLines(out string) []string {
	var hs []string
	for _, l := range strings.Split(out, "\n") {
		if strings.Contains(l, "chart for ") || strings.Contains(l, "comparison for ") {
			if i := strings.Index(l, " for "); i >= 0 {
				hs = append(hs, "chart for "+l[i+5:])
			}
		}
	}
	return hs
}

func jdPrinted(t *testing.T, out string) float64 {
	t.Helper()
	m := reJulianDay.FindStringSubmatch(out)
	if m == nil {
		t.Fatalf("no Julian Day in:\n%.300s", out)
	}
	v, _ := strconv.ParseFloat(m[1], 64)
	return v
}

func TestPreReformChartIsCastForItsMoment(t *testing.T) {
	needAstrolog(t)
	withClock(t, Clock{})
	type moment struct {
		name string
		when time.Time
		tz   string
	}
	var ms []moment
	for _, y := range []int{1500, 0, -5} {
		for _, tz := range []string{"Etc/GMT-5", "Etc/GMT+8", "Europe/Amsterdam", "Asia/Kolkata", "UTC"} {
			ms = append(ms, moment{fmt.Sprintf("%d/%s", y, tz), time.Date(y, 6, 15, 21, 40, 17, 0, time.UTC), tz})
		}
	}
	// The window: 2299170.4 at +5 and +14 east, and the western controls.
	window := time.Unix(int64(math.Round((2299170.4-2440587.5)*86400)), 0).UTC()
	for _, tz := range []string{"Etc/GMT-5", "Etc/GMT-14", "Etc/GMT+5", "Etc/GMT+12"} {
		ms = append(ms, moment{"window/" + tz, window, tz})
	}
	for _, m := range ms {
		t.Run(m.name, func(t *testing.T) {
			jd := SlotJD(m.when)
			loc, err := time.LoadLocation(m.tz)
			if err != nil {
				t.Fatal(err)
			}
			stamp := SlotLabel(m.when, loc)
			if strings.HasPrefix(m.name, "window/") {
				_, off := m.when.In(loc).Zone()
				if wantUT := off > 0; stamp.UT != wantUT {
					t.Errorf("SlotLabel UT = %v at offset %d, want %v", stamp.UT, off, wantUT)
				}
			}
			src := recipe.MomentAtPlace{JD: jd, Lon: 4.9, Lat: 52.37, TZ: m.tz}
			local := recipe.LocalMomentAtPlace{JD: jd, Lon: 4.9, Lat: 52.37, TZ: m.tz}
			utc := recipe.MomentAtPlace{JD: jd, Lon: 4.9, Lat: 52.37}
			sibling := recipe.MomentAtPlace{JD: SlotJD(time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)), Lon: 10, Lat: 50}

			// Every call path: the compiled call, its Julian Day and its header.
			type path struct {
				name   string
				inv    Invocation
				header int // which header line is the slot's: 0 first, 1 second
				jdAll  bool
				ut     bool // the path never sends a zone: its header is UT
			}
			paths := []path{
				{"single chart", compileOne(t, fixtureSpec(t, src, recipe.Placidus, recipe.Tropical), recipe.Positions{}), 0, true, false},
				{"single aspects", compileOne(t, fixtureSpec(t, src, recipe.Placidus, recipe.Tropical), recipe.Aspects{}), 0, true, false},
				{"raw report", compileOne(t, fixtureSpec(t, local, recipe.Placidus, recipe.Tropical), recipe.ChartRawReport()), 0, true, false},
				{"aspects raw report", compileOne(t, fixtureSpec(t, local, recipe.Placidus, recipe.Tropical), recipe.AspectsRawReport()), 0, true, false},
				{"staged local pair", compileOne(t, fixtureSpec(t, recipe.Comparison{Primary: local, Secondary: sibling}, recipe.Placidus, recipe.Tropical), recipe.AspectsRawReport()), 0, false, false},
				{"staged local pair second", compileOne(t, fixtureSpec(t, recipe.Comparison{Primary: sibling, Secondary: local}, recipe.Placidus, recipe.Tropical), recipe.AspectsRawReport()), 1, true, false},
				{"staged ut pair", compileOne(t, fixtureSpec(t, recipe.Comparison{Primary: utc, Secondary: sibling}, recipe.Placidus, recipe.Tropical), recipe.AspectsRawReport()), 0, false, true},
			}
			for _, p := range paths {
				text := runStaged(t, p.inv)
				hs := headerLines(text)
				if len(hs) <= p.header {
					t.Fatalf("%s: no header %d in:\n%.400s", p.name, p.header, text)
				}
				wd, got := headerFields(t, hs[p.header])
				want := stamp.CalendarDate
				if p.ut {
					want = AstrologCalendar{}.Label(sentJD(m.when.Unix()), 0)
				}
				if !sameStamp(want, wd, got) {
					t.Errorf("%s: header %s %s %d, %d %d:%02d, want %s", p.name, wd, monthNames[got.Month], got.Day, got.Year, got.Hour, got.Minute, describe(want))
				}
				if p.jdAll {
					if gotJD := jdPrinted(t, runStaged(t, p.inv, "-w")); math.Abs(gotJD-jd) > 6e-6 {
						t.Errorf("%s: Julian Day %.5f, want %.5f", p.name, gotJD, jd)
					}
				}
			}
			// The timeline's natal line: -qj, in UT, its header the UT label.
			call, err := TimelineCall(TimelineTransit, TimelineSegment{Offset: 0}, TimelineRequest{
				Year: 2026, Month: time.October, Zone: time.UTC, Natal: &TimelineNatal{When: m.when, Lon: 4.9, Lat: 52.37}})
			if err != nil {
				t.Fatal(err)
			}
			text := astrologOut(t, call.Argv()...)
			hs := headerLines(text)
			if len(hs) == 0 {
				t.Fatalf("timeline natal: no header in:\n%.300s", text)
			}
			wd, got := headerFields(t, hs[0])
			if want := (AstrologCalendar{}).Label(sentJD(m.when.Unix()), 0); !sameStamp(want, wd, got) {
				t.Errorf("timeline natal header %s %s %d, %d %d:%02d, want %s", wd, monthNames[got.Month], got.Day, got.Year, got.Hour, got.Minute, describe(want))
			}
		})
	}
}

// ---- C6, N4, N6 -------------------------------------------------------------

func hasRun(argv, run []string) bool { return containsRun(argv, run) }

// recordingAstrolog puts an `astrolog` on PATH that records each argv on a line
// and prints nothing; the returned func reads the calls so far.
func recordingAstrolog(t *testing.T) func() [][]string {
	t.Helper()
	dir := t.TempDir()
	log := filepath.Join(dir, "calls.log")
	script := "#!/bin/sh\nfor a in \"$@\"; do printf '%s\\n' \"$a\"; done >> '" + log + "'\nprintf '\\n--END--\\n' >> '" + log + "'\nexit 0\n"
	if err := os.WriteFile(filepath.Join(dir, "astrolog"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return func() [][]string {
		b, _ := os.ReadFile(log)
		var calls [][]string
		var cur []string
		for _, l := range strings.Split(string(b), "\n") {
			if l == "--END--" {
				calls = append(calls, cur)
				cur = nil
				continue
			}
			if l != "" {
				cur = append(cur, l)
			}
		}
		return calls
	}
}

func TestReformDateIsPinned(t *testing.T) {
	// N4 and the argv half: every chart and search call carries the pin, under
	// both hour pins.
	for _, c := range allClocks {
		t.Run("argv/"+clockName(c), func(t *testing.T) {
			withClock(t, c)
			utc := recipe.MomentAtPlace{JD: 2450000.5, Lon: 0, Lat: 51.5, TZ: "Europe/London"}
			local := recipe.LocalMomentAtPlace{JD: 2450000.5, Lon: 0, Lat: 51.5, TZ: "Europe/London"}
			srcs := map[string][]recipe.Projection{
				"single": {recipe.Positions{}, recipe.Aspects{}, recipe.ChartRawReport(), recipe.AspectsRawReport(), recipe.BodyRawReport(aspect.Sun), recipe.WheelImage{Render: recipe.DefaultWheelRender()}},
				"local":  {recipe.ChartRawReport(), recipe.AspectsRawReport()},
				"pair":   {recipe.Aspects{}, recipe.AspectsRawReport(), recipe.WheelImage{Render: recipe.DefaultWheelRender()}},
				"ring":   {recipe.WheelImage{Render: recipe.DefaultWheelRender()}},
			}
			sources := map[string]recipe.Source{
				"single": utc, "local": local,
				"pair": recipe.Comparison{Primary: fixLondon, Secondary: fixTromso},
				"ring": recipe.MultiWheel{Rings: []recipe.Source{fixLondon, fixTromso, fixOslo}},
			}
			for name, projs := range srcs {
				spec := fixtureSpec(t, sources[name], recipe.Placidus, recipe.Tropical)
				for _, p := range projs {
					plan, err := Compile(spec, p)
					if err != nil {
						t.Fatalf("%s/%s: %v", name, p.Label(), err)
					}
					for _, inv := range plan.Invocations {
						if !hasRun(inv.Argv, reformPin) || !hasRun(inv.Call().Argv(), reformPin) {
							t.Errorf("%s/%s lacks the reform pin: %q", name, p.Label(), inv.Argv)
						}
					}
				}
				bundle, err := CompileBundle(spec, recipe.Positions{}, recipe.WheelImage{Render: recipe.DefaultWheelRender()})
				if name == "single" {
					if err != nil {
						t.Fatal(err)
					}
					for _, plan := range bundle.Plans {
						for _, inv := range plan.Invocations {
							if !hasRun(inv.Argv, reformPin) {
								t.Errorf("bundle lacks the reform pin: %q", inv.Argv)
							}
						}
					}
				}
			}
			for _, src := range []TimelineSource{TimelineTransit, TimelineSky, TimelineReturn} {
				call, err := TimelineCall(src, TimelineSegment{Offset: 3600}, TimelineRequest{
					Year: 2026, Month: time.October, Zone: time.UTC,
					Natal: &TimelineNatal{When: time.Date(1990, 5, 1, 12, 0, 0, 0, time.UTC), Lon: 4.9, Lat: 52.37}})
				if err != nil {
					t.Fatal(err)
				}
				if !hasRun(call.Argv(), reformPin) {
					t.Errorf("timeline %s lacks the reform pin: %q", src, call.Argv())
				}
			}
		})
	}

	// N6: the pin reaches the chart-file read and no other exempt call.
	t.Run("exempt-calls", func(t *testing.T) {
		calls := recordingAstrolog(t)
		dat := filepath.Join(t.TempDir(), "c.as")
		if err := os.WriteFile(dat, []byte("@AI780\n-qb 1 1 2000 12:00 ST 0W 0:00E 0:00N\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		ctx := context.Background()
		_, _ = ResolveChartFile(ctx, dat)
		resolved := len(calls())
		_, _ = ReadDefaultSets(ctx)
		_, _ = ObjectRegistry(ctx)
		_, _ = AspectRegistry(ctx)
		_, _, _ = Version()
		all := calls()
		if resolved != 1 || len(all) < 5 {
			t.Fatalf("recorded %d calls (%d before the metadata reads), want the chart-file read then at least four reads", len(all), resolved)
		}
		if read := all[0]; !hasRun(read, reformPin) || slices.Index(read, "-qag") > slices.Index(read, "-i") {
			t.Errorf("the chart-file read must carry the pin before -i: %q", read)
		}
		if read := all[0]; slices.Index(read, "-R0") < slices.Index(read, "-i") || slices.Index(read, "-w") < 0 {
			t.Errorf("the chart-file read lost its body restriction (-R0 after -i, then -w): %q", read)
		}
		for _, c := range all[resolved:] {
			if slices.Contains(c, "-qag") {
				t.Errorf("an exempt metadata call carries the reform pin: %q", c)
			}
		}
	})

	needAstrolog(t)
	// N4, live: the pin's time token parses under both hour pins and both date
	// orders, and the commands that ran carry it.
	for _, c := range allClocks {
		t.Run("live/"+clockName(c), func(t *testing.T) {
			withClock(t, c)
			pin := strings.Join(reformPin, " ")
			rows, commands, err := ComputeTimeline(context.Background(), TimelineRequest{Year: 1582, Month: time.October, Zone: time.UTC}, []TimelineSource{TimelineSky}, nil)
			if err != nil || len(rows) == 0 {
				t.Fatalf("%d rows, err %v: the pin did not parse under %s", len(rows), err, clockName(c))
			}
			if !strings.Contains(strings.Join(commands, "\n"), pin) {
				t.Errorf("the timeline search did not run with the pin: %q", commands)
			}
			local := recipe.LocalMomentAtPlace{JD: 2299000.5, Lon: 4.9, Lat: 52.37, TZ: "Europe/Amsterdam"}
			_, command, err := ComputeRawReport(context.Background(), fixtureSpec(t, local, recipe.Placidus, recipe.Tropical), recipe.ChartRawReport())
			if err != nil || !strings.Contains(command, pin) {
				t.Errorf("the raw report did not run with the pin (err %v): %q", err, command)
			}
		})
	}

	// The host half: a host astrolog.as with a 25 Oct 1582 reform date.
	host := "-qag 10 25 1582 12:00am 0 0:00E 0:00N"
	t.Run("host", func(t *testing.T) {
		withClock(t, Clock{})
		rowsOf := func(t *testing.T) (instants []time.Time, jd float64) {
			t.Helper()
			req := TimelineRequest{Year: 1582, Month: time.October, Zone: time.UTC}
			rows, _, err := ComputeTimeline(context.Background(), req, []TimelineSource{TimelineSky}, nil)
			if err != nil {
				t.Fatal(err)
			}
			for _, r := range rows {
				instants = append(instants, r.Instant)
			}
			// A library file read at pick time: a 20 Oct 1582 local chart.
			dat := filepath.Join(t.TempDir(), "lib.as")
			if err := os.WriteFile(dat, []byte("@AI780\n-qb 10 20 1582 12:00am ST 0W 0:00E 0:00N\n-zi \"X\" \"Y\"\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			cf, err := ResolveChartFile(context.Background(), dat)
			if err != nil {
				t.Fatal(err)
			}
			return instants, cf.JD
		}
		// The control: the hostile host moves an unpinned search.
		useDataDir(t, dataDirWith(t))
		clean := astrologOut(t, "-z", "0", "-qm", "10", "1582", "-dm")
		useDataDir(t, dataDirWith(t, host))
		hostile := astrologOut(t, "-z", "0", "-qm", "10", "1582", "-dm")
		if clean == hostile {
			t.Fatal("the hostile host did not move an unpinned search: the fixture is not the file Astrolog loaded")
		}
		useDataDir(t, dataDirWith(t))
		cleanRows, cleanJD := rowsOf(t)
		useDataDir(t, dataDirWith(t, host))
		hostRows, hostJD := rowsOf(t)
		if len(cleanRows) == 0 || !slices.EqualFunc(cleanRows, hostRows, time.Time.Equal) {
			t.Errorf("the October 1582 rows moved under a 25 Oct reform date: %d rows against %d", len(cleanRows), len(hostRows))
		}
		if cleanJD != hostJD {
			t.Errorf("a library chart resolves to JD %.5f under the default host and %.5f under a 25 Oct reform date", cleanJD, hostJD)
		}
	})
}

// ---- C8 --------------------------------------------------------------------

// rawEventLines counts a raw search's event lines with a loose pattern that
// does not depend on the grammar under test.
func rawEventLines(out string) int {
	n := 0
	re := regexp.MustCompile(`^ *(?:[A-Z][a-z]{2} +)?\d+[/-] *\d+[/-] *-?\d+ +\d+:\d{2}`)
	for _, l := range strings.Split(out, "\n") {
		if re.MatchString(l) {
			n++
		}
	}
	return n
}

func TestPreReformTimelineMatchesAstrolog(t *testing.T) {
	needAstrolog(t)
	amsterdam, err := time.LoadLocation("Europe/Amsterdam")
	if err != nil {
		t.Fatal(err)
	}
	natalWhen := time.Date(1979, 5, 15, 9, 30, 0, 0, time.UTC)
	months := []struct {
		year  int
		month time.Month
	}{{1500, time.February}, {1582, time.October}, {-100, time.March}}
	for _, zone := range []*time.Location{time.UTC, amsterdam} {
		for _, my := range months {
			for _, withNatal := range []bool{false, true} {
				name := fmt.Sprintf("%s/%d-%02d/natal=%v", zone, my.year, int(my.month), withNatal)
				t.Run(name, func(t *testing.T) {
					withClock(t, Clock{})
					req := TimelineRequest{Year: my.year, Month: my.month, Zone: zone}
					srcs := []TimelineSource{TimelineSky}
					if withNatal {
						req.Natal = &TimelineNatal{When: natalWhen, Lon: 4.9, Lat: 52.37}
						srcs = []TimelineSource{TimelineTransit, TimelineSky, TimelineReturn}
					}
					rows, _, err := ComputeTimeline(context.Background(), req, srcs, nil)
					if err != nil {
						t.Fatalf("no row may be refused: %v", err)
					}
					start, end, err := AstrologCalendar{}.MonthBounds(my.year, my.month, zone)
					if err != nil {
						t.Fatal(err)
					}
					segs := TimelineSegments(my.year, my.month, zone)
					if !segs[0].Start.Equal(start) || !segs[len(segs)-1].End.Equal(end) {
						t.Errorf("segments run %v to %v, want the month's bounds %v to %v", segs[0].Start, segs[len(segs)-1].End, start, end)
					}
					// The bounds again, independently: live Astrolog reads the first
					// of this month and of the next as local 0:00 fields, and prints
					// the Julian Day it makes of them.
					_, offStart := start.In(zone).Zone()
					_, offEnd := end.In(zone).Zone()
					indStart := astrologFirstOfMonth(t, my.year, int(my.month), offStart)
					ny, nm := my.year, int(my.month)+1
					if nm > 12 {
						ny, nm = ny+1, 1
					}
					indEnd := astrologFirstOfMonth(t, ny, nm, offEnd)
					for _, c := range []struct {
						what      string
						got, want time.Time
					}{{"start", start, indStart}, {"end", end, indEnd}} {
						if d := c.got.Sub(c.want); d < -time.Second || d > time.Second {
							t.Errorf("MonthBounds %s = %v, Astrolog reads the first of the month at %v (%v apart)", c.what, c.got, c.want, d)
						}
					}
					mo, yr := strconv.Itoa(int(my.month)), strconv.Itoa(my.year)
					var want []time.Time
					for _, seg := range segs {
						zoneArg := formatSearchZone(seg.Offset)
						natal := []string{"-qj", momentJD(SlotJD(natalWhen)), "-zl", formatLon(4.9), formatLat(52.37)}
						var raws []string
						raws = append(raws, astrologOut(t, "-z", zoneArg, "-qm", mo, yr, "-dm", "=Yt", "_Yd"))
						if withNatal {
							raws = append(raws,
								astrologOut(t, append(append([]string{"-z", zoneArg}, natal...), "-t", mo, yr, "=Yt", "_Yd")...),
								astrologOut(t, append(append([]string{"-z", zoneArg}, natal...), "-tr", mo, yr, "=Yt", "_Yd")...))
						}
						for _, raw := range raws {
							got, err := ParseTimeline(raw, time.FixedZone("", seg.Offset))
							if err != nil {
								t.Fatalf("raw rows are not readable through Parse: %v", err)
							}
							if len(got) != rawEventLines(raw) {
								t.Errorf("Parse read %d of %d raw event lines", len(got), rawEventLines(raw))
							}
							for _, r := range got {
								if !r.Instant.Before(indStart) && r.Instant.Before(indEnd) {
									want = append(want, r.Instant)
								}
							}
						}
					}
					// Sextile's rows: the same instants, all of them, as a multiset
					// (the -t and -tr searches repeat a return).
					var got []time.Time
					for _, r := range rows {
						got = append(got, r.Instant)
					}
					slices.SortFunc(got, time.Time.Compare)
					slices.SortFunc(want, time.Time.Compare)
					if !slices.EqualFunc(got, want, time.Time.Equal) {
						t.Errorf("Sextile has %d rows, raw Astrolog read through Parse has %d (first differing: %v)", len(got), len(want), firstDiff(got, want))
					}
					if len(got) == 0 {
						t.Error("no rows at all")
					}
					// The mark follows the printed date: before 15 Oct 1582 is Julian.
					for _, r := range rows {
						f := rowFields(t, r.Line)
						wantJulian := f.Year < 1582 || (f.Year == 1582 && (f.Month < 10 || (f.Month == 10 && f.Day < 15)))
						if r.Julian != wantJulian {
							t.Errorf("row %q: Julian = %v, want %v", r.Line, r.Julian, wantJulian)
							break
						}
					}
				})
			}
		}
	}

	// The year forms under all four clocks: 1-, 3-, 4-digit and negative years.
	for _, c := range allClocks {
		for _, year := range []int{5, 500, 1500, -5, -100} {
			t.Run(fmt.Sprintf("years/%s/%d", clockName(c), year), func(t *testing.T) {
				withClock(t, c)
				req := TimelineRequest{Year: year, Month: time.March, Zone: time.UTC}
				rows, _, err := ComputeTimeline(context.Background(), req, []TimelineSource{TimelineSky}, nil)
				if err != nil {
					t.Fatalf("year %d under %s: %v", year, clockName(c), err)
				}
				if len(rows) == 0 {
					t.Fatalf("year %d: no rows", year)
				}
				start, end, _ := AstrologCalendar{}.MonthBounds(year, time.March, time.UTC)
				for _, r := range rows {
					if r.Instant.Before(start) || !r.Instant.Before(end) {
						t.Errorf("row %q at %v lies outside the month %v to %v", r.Line, r.Instant, start, end)
					}
				}
			})
		}
	}

	// The month asked for at the edge of each sampled month is Parse's inverse's.
	for _, zone := range []*time.Location{time.UTC, amsterdam} {
		for _, my := range append(months, struct {
			year  int
			month time.Month
		}{2026, time.October}) {
			start, _, err := AstrologCalendar{}.MonthBounds(my.year, my.month, zone)
			if err != nil {
				t.Fatal(err)
			}
			for _, d := range []struct {
				at   time.Time
				prev bool
			}{{start.Add(-time.Hour), true}, {start.Add(time.Hour), false}} {
				_, off := d.at.In(zone).Zone()
				y, mo := AstrologCalendar{}.MonthOf(d.at, off)
				wy, wm := my.year, my.month
				if d.prev {
					wy, wm = my.year, my.month-1
					if wm < time.January {
						wy, wm = my.year-1, time.December
					}
				}
				if y != wy || mo != wm {
					t.Errorf("%s: an hour %s the start of %d-%02d lies in %d-%02d, want %d-%02d", zone, map[bool]string{true: "before", false: "after"}[d.prev], my.year, int(my.month), y, int(mo), wy, int(wm))
				}
			}
		}
	}
}

// astrologFirstOfMonth is the instant live Astrolog makes of 0:00 on the first of
// a month, read as calendar fields at a zone offset (seconds east): its printed
// Julian Day, to its five decimals.
func astrologFirstOfMonth(t *testing.T, year, month, offset int) time.Time {
	t.Helper()
	out := astrologOut(t, "-qa", strconv.Itoa(month), "1", strconv.Itoa(year), "0:00", formatSearchZone(offset), "0E", "0N", "-w", "-R0")
	return time.Unix(int64(math.Round((jdPrinted(t, out)-2440587.5)*86400)), 0).UTC()
}

func firstDiff(a, b []time.Time) string {
	for i := 0; i < len(a) && i < len(b); i++ {
		if !a[i].Equal(b[i]) {
			return fmt.Sprintf("#%d %v against %v", i, a[i], b[i])
		}
	}
	return "one list is a prefix of the other"
}

// ---- N7 --------------------------------------------------------------------

func TestOldTimelineRowRefusesAnImpossibleDate(t *testing.T) {
	withClock(t, Clock{})
	transit := func(date string) string { return " " + date + "  1:21 trans    Moon (Pis) Con natal (Ari) Venus" }
	valid := []string{
		transit(" 2/29/1500"), // Julian: 1500 is a leap year
		transit(" 2/29/ 500"),
		transit("10/ 4/1582"),
		transit("10/15/1582"),
		transit(" 2/29/2024"),
	}
	for _, line := range valid {
		rows, err := ParseTimeline(line, time.UTC)
		if err != nil || len(rows) != 1 {
			t.Errorf("%q: %d rows, err %v, want one row", line, len(rows), err)
		}
	}
	invalid := map[string]string{
		"month 13":                               transit("13/ 1/1500"),
		"month 0":                                transit(" 0/ 1/1500"),
		"day 30 of February":                     transit(" 2/30/1500"),
		"day 31 of April":                        transit(" 4/31/1500"),
		"day 0":                                  transit(" 3/ 0/1500"),
		"29 February of a Gregorian common year": transit(" 2/29/1700"),
		"hour 24":                                " 3/ 1/1500 24:00 trans    Moon (Pis) Con natal (Ari) Venus",
		"minute 60":                              " 3/ 1/1500  1:60 trans    Moon (Pis) Con natal (Ari) Venus",
	}
	for name, line := range invalid {
		rows, err := ParseTimeline(line, time.UTC)
		var lineErr *TimelineLineError
		if !errors.As(err, &lineErr) || lineErr.Line != line || rows != nil {
			t.Errorf("%s: %q gave %d rows and err %v, want a TimelineLineError naming the row and no row", name, line, len(rows), err)
		}
	}
}

// ---- C10 -------------------------------------------------------------------

// Every chart and search call constructor refuses a moment before Julian Day 0
// with an error and builds no argv or file.
func TestEveryCallConstructorRefusesAMomentBeforeJulianDayZero(t *testing.T) {
	before := time.Date(-4713, 11, 24, 11, 59, 0, 0, time.UTC) // a minute before JD 0
	if got := before.Unix(); got != jdZeroUnix-60 {
		t.Fatalf("fixture: the moment is Unix %d, want a minute before JD 0 (%d)", got, jdZeroUnix-60)
	}
	jd := unixFormulaJD(before)
	if jd >= 0 {
		t.Fatalf("fixture: JD %v is not before 0", jd)
	}
	for _, c := range []struct {
		name string
		call func() error
	}{
		{"FormatChartFile", func() error {
			_, err := FormatChartFile(recipe.MomentAtPlace{JD: jd, Lon: 4.9, Lat: 52.37}, recipe.Placidus, recipe.Tropical)
			return err
		}},
		{"FormatLocalChartFile", func() error {
			_, err := FormatLocalChartFile(recipe.LocalMomentAtPlace{JD: jd, Lon: 4.9, Lat: 52.37, TZ: "UTC"}, recipe.Placidus, recipe.Tropical)
			return err
		}},
		{"momentArg", func() error { _, err := momentArg(jd); return err }},
		{"TimelineCall natal", func() error {
			_, err := TimelineCall(TimelineTransit, TimelineSegment{}, TimelineRequest{Year: 2026, Month: time.October, Zone: time.UTC, Natal: &TimelineNatal{When: before, Lon: 4.9, Lat: 52.37}})
			return err
		}},
		{"TimelineCall month", func() error {
			_, err := TimelineCall(TimelineSky, TimelineSegment{}, TimelineRequest{Year: -4714, Month: time.June, Zone: time.UTC})
			return err
		}},
		{"ComputeTimeline month", func() error {
			_, _, err := ComputeTimeline(context.Background(), TimelineRequest{Year: -4714, Month: time.June, Zone: time.UTC}, []TimelineSource{TimelineSky}, func(context.Context, BodyCall) (string, string, error) {
				t.Error("a search ran for a month before Julian Day 0")
				return "", "", nil
			})
			return err
		}},
	} {
		if err := c.call(); !errors.Is(err, ErrBeforeJulianDayZero) {
			t.Errorf("%s: err = %v, want ErrBeforeJulianDayZero", c.name, err)
		}
	}
	// Compile, for every source and every BodyCall kind: an error, no plan.
	local := recipe.LocalMomentAtPlace{JD: jd, Lon: 4.9, Lat: 52.37, TZ: "UTC"}
	utc := recipe.MomentAtPlace{JD: jd, Lon: 4.9, Lat: 52.37}
	for name, src := range map[string]recipe.Source{
		"single": utc, "local": local,
		"pair": recipe.Comparison{Primary: utc, Secondary: fixLondon},
		"ring": recipe.MultiWheel{Rings: []recipe.Source{utc, fixLondon, fixTromso}},
	} {
		spec, err := recipe.NewChartSpec(src, recipe.Placidus, recipe.Tropical, recipe.ObjectPolicy{Baseline: recipe.AstrologDefault})
		if err != nil {
			continue // the recipe itself refused it: no argv either
		}
		for _, p := range []recipe.Projection{recipe.Positions{}, recipe.Aspects{}, recipe.ChartRawReport(), recipe.AspectsRawReport(), recipe.BodyRawReport(aspect.Sun), recipe.WheelImage{Render: recipe.DefaultWheelRender()}} {
			if plan, err := Compile(spec, p); err == nil {
				t.Errorf("%s/%s compiled a plan for a moment before Julian Day 0: %v", name, p.Label(), plan.Invocations[0].Argv)
			}
		}
	}
}
