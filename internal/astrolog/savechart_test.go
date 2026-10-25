// All chart values in this file are constructed synthetic test inputs.
package astrolog

import (
	"context"
	"errors"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// qbFields returns the -qb line of a written record as whitespace-separated
// fields. astrolog pads that line with runs of spaces whose width is not part
// of the contract, so every assertion here is on the FIELDS. Comparing the
// literal line would fail on a padding change that broke nothing.
func qbFields(t *testing.T, record []byte) []string {
	t.Helper()
	for _, line := range strings.Split(string(record), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "-qb") {
			return strings.Fields(line)
		}
	}
	t.Fatalf("no -qb line in the record astrolog wrote:\n%s", record)
	return nil
}

func ziLine(t *testing.T, record []byte) string {
	t.Helper()
	for _, line := range strings.Split(string(record), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "-zi") {
			return strings.TrimSpace(line)
		}
	}
	t.Fatalf("no -zi line in the record astrolog wrote:\n%s", record)
	return ""
}

// amsterdam is a constructed +2 zone. A fixed zone rather than a tzdata lookup
// so the test states the offset it is asserting about instead of depending on
// which tzdata the machine carries.
func amsterdam() *time.Location { return time.FixedZone("CEST", 2*3600) }

// C3: the file is astrolog's own record, carrying the moment as civil time
// with the offset in effect, the name astrolog recorded, and the place label.
func TestSaveChartWritesAstrologsOwnRecord(t *testing.T) {
	record, err := ChartRecord(context.Background(), ChartToSave{
		Name:  "Test Person",
		Place: "Amsterdam",
		When:  time.Date(1987, 3, 14, 16, 30, 0, 0, amsterdam()),
		Lon:   4.9,
		Lat:   52.3667,
	})
	if err != nil {
		t.Fatalf("ChartRecord: %v", err)
	}
	if !strings.HasPrefix(string(record), "@AI") {
		t.Errorf("record does not open with astrolog's @AI header:\n%s", record)
	}
	got := qbFields(t, record)
	want := []string{"-qb", "Mar", "14", "1987", "4:30pm", "ST", "2E", "4:54E", "52:22N"}
	if len(got) != len(want) {
		t.Fatalf("-qb fields = %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("-qb field %d = %q, want %q (whole line: %q)", i, got[i], want[i], got)
		}
	}
	// The labels are astrolog's, read back from the file it wrote.
	if zi := ziLine(t, record); zi != `-zi "Test Person" "Amsterdam"` {
		t.Errorf("-zi line = %q, want -zi \"Test Person\" \"Amsterdam\"", zi)
	}
}

// C3a: a zone whose offset is not a whole number of hours is written so that
// it reads back to the same minute.
//
// This is the row that would have caught the spec's `=b0`: with that switch
// astrolog writes seconds precision instead, and the fields below become
// "4:30:00pm" and "77:13'00E".
func TestSaveChartWritesAHalfHourZone(t *testing.T) {
	when := time.Date(1987, 3, 14, 16, 30, 0, 0, time.FixedZone("IST", 5*3600+1800))
	record, err := ChartRecord(context.Background(), ChartToSave{
		Name: "Half Hour", Place: "New Delhi", When: when, Lon: 77.2167, Lat: 28.6,
	})
	if err != nil {
		t.Fatalf("ChartRecord: %v", err)
	}
	got := qbFields(t, record)
	if len(got) < 7 || got[4] != "4:30pm" || got[6] != "5:30E" {
		t.Fatalf("-qb fields = %q, want the moment 4:30pm in zone 5:30E", got)
	}
	wantRecord := "@AI800  ; Astrolog chart info.\n" +
		"-qb Mar 14 1987  4:30pm ST 5:30E  77:13E 28:36N\n" +
		"-zi \"Half Hour\" \"New Delhi\"\n"
	if string(record) != wantRecord {
		t.Errorf("half-hour record changed:\n got %q\nwant %q", record, wantRecord)
	}
	// And it is a real instant to astrolog, not just well-shaped text.
	assertReadsBackTo(t, record, when)
}

// A Gregorian time.Time is an instant, not permission for Go to choose the
// calendar label written into an Astrolog record. At ancient dates Astrolog's
// own label differs, while the instant must still round-trip unchanged.
func TestSaveChartRoundTripsYearZeroThroughAstrologCalendar(t *testing.T) {
	assertAncientChartRoundTrip(t, 0, "Jun", "17", "Jun 17, 0 12:00pm (ST Zone 2E)")
}

func TestSaveChartRoundTripsNegativeYearThroughAstrologCalendar(t *testing.T) {
	assertAncientChartRoundTrip(t, -5, "Jun", "17", "Jun 17, -5 12:00pm (ST Zone 2E)")
}

func assertAncientChartRoundTrip(t *testing.T, year int, month, day, label string) {
	t.Helper()
	when := time.Date(year, 6, 15, 12, 0, 0, 0, amsterdam())
	record, err := ChartRecord(context.Background(), ChartToSave{
		Name: "Ancient", Place: "Amsterdam", When: when, Lon: 4.9, Lat: 52.3667,
	})
	if err != nil {
		t.Fatalf("ChartRecord: %v", err)
	}
	got := qbFields(t, record)
	if len(got) < 5 || got[1] != month || got[2] != day || got[3] != strconv.Itoa(year) || got[4] != "12:00pm" {
		t.Fatalf("-qb fields = %q, want Astrolog's label %s %s %d 12:00pm", got, month, day, year)
	}

	path := filepath.Join(t.TempDir(), "ancient.as")
	if err := os.WriteFile(path, record, 0o600); err != nil {
		t.Fatalf("write record: %v", err)
	}
	out, _, err := invoke(context.Background(), []string{"-i", path, "-v"})
	if err != nil {
		t.Fatalf("read record through Astrolog: %v", err)
	}
	if !strings.Contains(out, label) {
		t.Fatalf("Astrolog readback has no %q label:\n%s", label, out)
	}
	header, err := ReadChartFileHeader(path)
	if err != nil {
		t.Fatalf("ReadChartFileHeader: %v", err)
	}
	if header.Year != year {
		t.Errorf("picker year = %d, want Astrolog's recorded year %d", header.Year, year)
	}
	assertReadsBackTo(t, record, when)
}

// C4: the saved file reads back through astrolog to the slot's moment rounded
// to the nearest minute, from a fixture moment whose seconds are NON-ZERO.
//
// The non-zero seconds are the point. astrolog records minutes, so a moment
// carrying 47 seconds must land on the nearest minute rather than the one it
// started in; a test built on a whole minute would pass against a writer that
// silently truncated.
func TestSavedChartRoundTripsToTheSameMinute(t *testing.T) {
	when := time.Date(1987, 3, 14, 16, 30, 47, 0, amsterdam())
	record, err := ChartRecord(context.Background(), ChartToSave{
		Name: "Test Person", Place: "Amsterdam", When: when, Lon: 4.9, Lat: 52.3667,
	})
	if err != nil {
		t.Fatalf("ChartRecord: %v", err)
	}
	if got := qbFields(t, record); len(got) < 5 || got[4] != "4:31pm" {
		t.Fatalf("-qb fields = %q, want 16:30:47 rounded to 4:31pm", got)
	}
	assertReadsBackTo(t, record, when.Round(time.Minute))
}

// assertReadsBackTo writes the record where astrolog can read it and requires
// astrolog's own resolution of it to be the same instant as want, to the
// minute. This is the round trip that matters: astrolog wrote the file and
// astrolog is asked what it means, so nothing in Sextile re-derives the moment.
func assertReadsBackTo(t *testing.T, record []byte, want time.Time) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "read-back.as")
	if err := os.WriteFile(path, record, 0o600); err != nil {
		t.Fatalf("write record: %v", err)
	}
	cf, err := ResolveChartFile(context.Background(), path)
	if err != nil {
		t.Fatalf("ResolveChartFile on the record just written: %v\n%s", err, record)
	}
	if got := cf.WhenUTC.UTC().Truncate(time.Minute); !got.Equal(want.UTC().Truncate(time.Minute)) {
		t.Errorf("round trip: astrolog reads the saved chart as %s, want %s",
			got.Format(time.RFC3339), want.UTC().Truncate(time.Minute).Format(time.RFC3339))
	}
}

// C9: the save removes the temporary directory it wrote into, on success AND
// on failure, and leaves no file outside the charts directory.
//
// TMPDIR is redirected so the assertion is "that directory is empty", which is
// observable, rather than "no stray file exists anywhere", which is not.
func TestSaveLeavesNoTemporaryFiles(t *testing.T) {
	c := ChartToSave{
		Name: "Test Person", Place: "Amsterdam",
		When: time.Date(1987, 3, 14, 16, 30, 0, 0, amsterdam()), Lon: 4.9, Lat: 52.3667,
	}
	t.Run("on success", func(t *testing.T) {
		tmp := t.TempDir()
		t.Setenv("TMPDIR", tmp)
		if _, err := ChartRecord(context.Background(), c); err != nil {
			t.Fatalf("ChartRecord: %v", err)
		}
		assertEmpty(t, tmp)
	})
	t.Run("on failure", func(t *testing.T) {
		tmp := t.TempDir()
		t.Setenv("TMPDIR", tmp)
		old := binary
		binary = shim(t, "exit 3\n")
		t.Cleanup(func() { binary = old })
		if _, err := ChartRecord(context.Background(), c); err == nil {
			t.Fatal("ChartRecord succeeded against an astrolog that exits 3")
		}
		assertEmpty(t, tmp)
	})
}

func assertEmpty(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read %s: %v", dir, err)
	}
	if len(entries) != 0 {
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Errorf("the save left %d entr(y/ies) behind in the temp root: %q", len(names), names)
	}
}

// shim writes an executable stand-in for astrolog and returns its path.
func shim(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "astrolog-shim")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body), 0o700); err != nil {
		t.Fatalf("write shim: %v", err)
	}
	return path
}

// Failure mode: astrolog is not on PATH. The save fails with the cause the
// startup probe already uses, and writes nothing.
func TestSaveWithoutAstrolog(t *testing.T) {
	old := binary
	binary = filepath.Join(t.TempDir(), "no-such-astrolog")
	t.Cleanup(func() { binary = old })

	_, err := ChartRecord(context.Background(), ChartToSave{
		Name: "Test Person", Place: "Amsterdam",
		When: time.Date(1987, 3, 14, 16, 30, 0, 0, amsterdam()),
	})
	if err == nil {
		t.Fatal("ChartRecord succeeded with no astrolog on PATH")
	}
	if !strings.Contains(err.Error(), ErrNotInstalled.Error()) {
		t.Errorf("error = %v, want it to carry ErrNotInstalled so the perimeter can say the install sentence", err)
	}
}

// Failure mode: astrolog exits non-zero, or exits ZERO having written nothing.
//
// The second half is the one that matters and it is why this test exists
// separately from the exit-code half: -o is a toggle, so an argv carrying two
// of them writes no file and still exits 0. A writer that believed the exit
// code would report a successful save having produced no chart at all.
func TestSaveWhenAstrologFails(t *testing.T) {
	c := ChartToSave{
		Name: "Test Person", Place: "Amsterdam",
		When: time.Date(1987, 3, 14, 16, 30, 0, 0, amsterdam()), Lon: 4.9, Lat: 52.3667,
	}
	for _, tc := range []struct {
		name string
		body string
	}{
		{"exits non-zero", "echo 'Astrolog: bad switch' ; exit 3\n"},
		{"exits 0 having written no file", "echo 'Astrolog 8.00' ; exit 0\n"},
		{"exits 0 having written an empty file", "for a; do last=$a; done; : > \"$last\" ; exit 0\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			old := binary
			binary = shim(t, tc.body)
			t.Cleanup(func() { binary = old })

			record, err := ChartRecord(context.Background(), c)
			if err == nil {
				t.Fatalf("ChartRecord reported success; it returned %q", record)
			}
			if !strings.Contains(err.Error(), "Test Person") {
				t.Errorf("error = %v, want it to name the chart that was not saved", err)
			}
		})
	}
}

// Failure mode: a name containing a double quote. astrolog rewrites it to a
// single quote inside -zi, and the saved record carries astrolog's rewrite —
// so the name the operator is shown afterwards has to come from the file, not
// from what they typed.
func TestSaveNameWithAQuote(t *testing.T) {
	record, err := ChartRecord(context.Background(), ChartToSave{
		Name:  `Say "Hi" Now`,
		Place: "Amsterdam",
		When:  time.Date(1987, 3, 14, 16, 30, 0, 0, amsterdam()),
		Lon:   4.9, Lat: 52.3667,
	})
	if err != nil {
		t.Fatalf("ChartRecord: %v", err)
	}
	if zi := ziLine(t, record); zi != `-zi "Say 'Hi' Now" "Amsterdam"` {
		t.Errorf("-zi line = %q, want astrolog's own rewrite -zi \"Say 'Hi' Now\" \"Amsterdam\"", zi)
	}
}

// The zone parameter is astrolog's, and astrolog counts hours WEST as
// positive where Go counts seconds EAST. Getting this backwards is a silent
// wrong-answer bug of up to 2x the offset, so it is pinned directly.
func TestSaveZoneIsWestPositive(t *testing.T) {
	for _, tc := range []struct {
		name   string
		offset int
		want   string
	}{
		{"two hours east", 2 * 3600, "-2:00"},
		{"five hours west", -5 * 3600, "5:00"},
		{"five and a half east", 5*3600 + 1800, "-5:30"},
		{"three and a half west", -(3*3600 + 1800), "3:30"},
		{"greenwich", 0, "0:00"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			when := time.Date(1987, 3, 14, 16, 30, 0, 0, time.FixedZone("X", tc.offset))
			if got := formatSaveZone(when); got != tc.want {
				t.Errorf("formatSaveZone(%s) = %q, want %q", when.Format(time.RFC3339), got, tc.want)
			}
		})
	}
}

// A control character in a label is refused before anything is spawned.
//
// Found by an independent review round, 2026-09-20. It is not cosmetic: a
// newline splits the record's -zi line in two, and astrolog then refuses to
// read back the file it just wrote --
//
//	Astrolog: Too few parameters to switch -zi (1 given, 2 required)
//
// -- so the chart LISTS in the picker (the header parser uses a regex and is
// untroubled by the split) and cannot be anchored, ever. C8 says a saved chart
// resolves to the moment it was saved from; this is that row's missing input.
func TestSaveRefusesAControlCharacterInALabel(t *testing.T) {
	base := ChartToSave{
		Name: "Test Person", Place: "Amsterdam",
		When: time.Date(1987, 3, 14, 16, 30, 0, 0, amsterdam()), Lon: 4.9, Lat: 52.3667,
	}
	for _, tc := range []struct {
		what string
		c    ChartToSave
	}{
		{"a newline in the name", func() ChartToSave { c := base; c.Name = "Alice\nBob"; return c }()},
		{"a tab in the name", func() ChartToSave { c := base; c.Name = "Alice\tBob"; return c }()},
		{"an escape in the name", func() ChartToSave { c := base; c.Name = "Alice\x1b]0;x\x07Bob"; return c }()},
		{"a newline in the place", func() ChartToSave { c := base; c.Place = "Ams\nterdam"; return c }()},
	} {
		t.Run(tc.what, func(t *testing.T) {
			tmp := t.TempDir()
			t.Setenv("TMPDIR", tmp)
			record, err := ChartRecord(context.Background(), tc.c)
			if !errors.Is(err, ErrLabelHasControlChar) {
				t.Fatalf("err = %v, want ErrLabelHasControlChar; it returned %q", err, record)
			}
			// Refused BEFORE the spawn: nothing was written anywhere.
			entries, readErr := os.ReadDir(tmp)
			if readErr != nil {
				t.Fatalf("read temp root: %v", readErr)
			}
			if len(entries) != 0 {
				t.Errorf("the refusal left %d entr(y/ies) behind", len(entries))
			}
		})
	}
}

// The round trip this protects, stated as the thing that used to break: a
// record whose -zi line is intact can be read back by astrolog itself.
func TestSavedRecordIsReadableByAstrologAgain(t *testing.T) {
	record, err := ChartRecord(context.Background(), ChartToSave{
		Name: "Test Person", Place: "Amsterdam",
		When: time.Date(1987, 3, 14, 16, 30, 0, 0, amsterdam()), Lon: 4.9, Lat: 52.3667,
	})
	if err != nil {
		t.Fatalf("ChartRecord: %v", err)
	}
	// The -zi line carries BOTH quoted labels on one physical line. A split
	// one leaves astrolog's own reader with a single parameter, which is the
	// failure this guards.
	if zi := ziLine(t, record); strings.Count(zi, `"`) != 4 {
		t.Fatalf("the -zi line does not carry two quoted labels: %q", zi)
	}
	path := filepath.Join(t.TempDir(), "readable.as")
	if err := os.WriteFile(path, record, 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := ResolveChartFile(context.Background(), path); err != nil {
		t.Fatalf("astrolog cannot read back the record it wrote: %v\n%s", err, record)
	}
}

// A saved chart's coordinates are astrolog's whole arc-minutes: it writes the
// -qb line itself and cuts anything finer, so seconds passed to -zl never reach
// the file. What Sextile owns is the rounding on the way in, which keeps a
// reopened chart within half an arc-minute of the point it was saved at, and
// the reader, which takes the older degree:minute form and the seconds forms
// alike.
func TestSaveChartCoordinatesStayWithinHalfAnArcMinute(t *testing.T) {
	const berlinLon, berlinLat = 13.4105, 52.5244 // the atlas's Berlin, DE
	record, err := ChartRecord(context.Background(), ChartToSave{
		Name: "Berlin Person", Place: "Berlin, DE",
		When: time.Date(1987, 3, 14, 16, 30, 0, 0, amsterdam()),
		Lon:  berlinLon, Lat: berlinLat,
	})
	if err != nil {
		t.Fatalf("ChartRecord: %v", err)
	}
	got := qbFields(t, record)
	if len(got) < 9 {
		t.Fatalf("-qb fields = %q, want the moment, zone, longitude and latitude", got)
	}
	lon, lat := parseDegMin(got[len(got)-2]), parseDegMin(got[len(got)-1])
	if d := math.Abs(lon-berlinLon) * 60; d > 0.5 {
		t.Errorf("longitude %q reads back %.2f arc-minutes from %v, want at most 0.5", got[len(got)-2], d, berlinLon)
	}
	if d := math.Abs(lat-berlinLat) * 60; d > 0.5 {
		t.Errorf("latitude %q reads back %.2f arc-minutes from %v, want at most 0.5", got[len(got)-1], d, berlinLat)
	}
	for _, c := range []struct {
		in   string
		want float64
	}{
		{"13:25E", 13 + 25.0/60},
		{"13:24:59E", 13 + 24.0/60 + 59.0/3600},
		{"52:31'30N", 52 + 31.0/60 + 30.0/3600},
		{"12:30W", -(12 + 30.0/60)},
	} {
		if g := parseDegMin(c.in); math.Abs(g-c.want) > 1e-9 {
			t.Errorf("parseDegMin(%q) = %v, want %v", c.in, g, c.want)
		}
	}
}
