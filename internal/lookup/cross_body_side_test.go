package lookup

import (
	"context"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/store"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// The fixture is the real report of the cross body report for Venus,
// captured with the complete Astrolog on 2026-11-02: chart 1 (the base,
// "Sep 23 1995 6:00pm Munich") first, chart 2 (the cursor's chart, "Jun 15
// 1990 4:30pm Bassum") second, argv
//
//	astrolog ... -r0 -qb <chart 1> -r2 <chart 2> -aa -RO 4   (SynastryCrossBodyRawReport)
//
// There are no two blocks in the text: the rows are ONE list, ordered by power,
// and what tells the two roles apart is the row's own shape. Astrolog prints
// chart 1's body left of "with" and chart 2's body right of it. Venus left is
// the base's Venus (rows 2, 3, 4, 6, 7, 9, 10, 11); Venus right is the cursor
// chart's Venus (rows 1, 5, 8).
func fixtureCrossVenus(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("testdata/f3-venus-cross.txt")
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func keepText(report string, b aspect.Object) string {
	s, _, _ := KeepCursorSide(report, b)
	return s
}

func rowNumbers(report string) []string {
	var nums []string
	for _, ln := range strings.Split(report, "\n") {
		if aspectRowRE.MatchString(ln) {
			nums = append(nums, strings.TrimSuffix(strings.Fields(ln)[0], ":"))
		}
	}
	return nums
}

func TestKeepCursorSideKeepsOnlyTheCursorChartsBody(t *testing.T) {
	report := fixtureCrossVenus(t)
	got, kept, total := KeepCursorSide(report, objVenus)
	if kept != 3 || total != 11 {
		t.Errorf("kept %d of %d rows, want 3 of 11", kept, total)
	}
	if rows := strings.Join(rowNumbers(got), ","); rows != "1,2,3" {
		t.Errorf("rows kept = %s, want 1,2,3 (Astrolog's rows 1, 5 and 8, Venus on the right of \"with\", counted again)\n%s", rows, got)
	}
	for _, ln := range strings.Split(got, "\n") {
		if aspectRowRE.MatchString(ln) && !strings.Contains(ln, ") Venus") {
			t.Errorf("a row without the cursor chart's Venus survived: %q", ln)
		}
	}
	// Rows keep Astrolog's own text after the number, byte for byte, in order.
	var want []string
	for _, ln := range strings.Split(report, "\n") {
		if m := aspectWithRow.FindStringSubmatch(ln); m != nil && m[1] == "Venus" {
			want = append(want, aspectRowNumber.ReplaceAllString(ln, ""))
		}
	}
	var have []string
	for _, ln := range strings.Split(got, "\n") {
		if aspectRowRE.MatchString(ln) {
			have = append(have, aspectRowNumber.ReplaceAllString(ln, ""))
		}
	}
	if strings.Join(have, "|") != strings.Join(want, "|") {
		t.Errorf("row text altered:\n%q\nwant\n%q", have, want)
	}
	// The header (both chart lines) survives; the totals described the
	// unfiltered list and are dropped with it.
	if !strings.Contains(got, "Chart #2 comparison for") || !strings.HasPrefix(got, "Astrolog 8.00 chart for") {
		t.Errorf("header lost:\n%s", got)
	}
	if strings.Contains(got, "Sum power") || strings.Contains(got, "Ven: 11") {
		t.Errorf("totals of the unfiltered list survived:\n%s", got)
	}
}

func TestKeepCursorSideOtherBodyAndNamesWithSpaces(t *testing.T) {
	report := fixtureCrossVenus(t)
	if rows := strings.Join(rowNumbers(keepText(report, objMars)), ","); rows != "1" {
		t.Errorf("Mars rows = %q, want 1", rows)
	}
	if rows := strings.Join(rowNumbers(keepText(report, objNorthNode)), ","); rows != "1" {
		t.Errorf("North Node rows = %q, want 1 (a name with a space abutting the dash)", rows)
	}
}

func TestKeepCursorSideBodyWithNoRowReadsEmpty(t *testing.T) {
	got := keepText(fixtureCrossVenus(t), objSun)
	if rows := rowNumbers(got); len(rows) != 0 {
		t.Errorf("Sun has no right-side row, got %v", rows)
	}
	if !strings.Contains(got, "Chart #2 comparison for") {
		t.Errorf("header lost:\n%s", got)
	}
}

func TestKeepCursorSidePassesThroughWhatItDoesNotDrop(t *testing.T) {
	oneBlock := "Astrolog 8.00 chart for X\nChart #2 comparison for Y\n\n  1:    Moon (Vir) Tri with (Tau) Venus     - orb: a1°34' - power:  8.43\n\nSum power: 8.43 - Average power: 8.43\nCon:  0   Opp:  0\n"
	if got, k, n := KeepCursorSide(oneBlock, objVenus); got != oneBlock || k != 1 || n != 1 {
		t.Errorf("a report whose rows all belong was changed:\n%q", got)
	}
	for _, in := range []string{"", "no rows here\n"} {
		if got, _, _ := KeepCursorSide(in, objVenus); got != in {
			t.Errorf("%q changed to %q", in, got)
		}
	}
}

// rightOfWith is the body name Astrolog prints after "with (Sign) ".
var rightOfWith = regexp.MustCompile(`with\s+[(\[][A-Z][a-z]{2}[)\]]\s+(.+?)\s*-\s+orb:`)

// Live: the filter, on the real report of both cross body producers, keeps
// only the rows whose RIGHT end (chart 2, the cursor's chart) is the body.
func TestKeepCursorSideOnTheLiveCrossReports(t *testing.T) {
	requireAstrolog(t)
	munich := location.PlaceInfo{Name: "Munich", Country: "DE", Lon: 11.5667, Lat: 48.1333, TZ: "Europe/Berlin"}
	bassum := location.PlaceInfo{Name: "Bassum", Country: "DE", Lon: 8.7167, Lat: 52.85, TZ: "Europe/Berlin"}
	chart1 := store.AnchorChart{When: time.Date(1995, 9, 23, 16, 0, 0, 0, time.UTC), Place: munich}
	when2 := time.Date(1990, 6, 15, 14, 30, 0, 0, time.UTC)
	check := func(name, raw string) {
		t.Helper()
		rows := 0
		for _, ln := range strings.Split(raw, "\n") {
			if !aspectRowRE.MatchString(ln) {
				continue
			}
			rows++
			if m := rightOfWith.FindStringSubmatch(ln); m == nil || m[1] != "Venus" {
				t.Errorf("%s: a row whose right end is not Venus survived: %q", name, ln)
			}
		}
		if rows == 0 {
			t.Errorf("%s: no Venus row at all:\n%s", name, raw)
		}
	}
	syn, _, err := SynastryCrossBodyRawReport(context.Background(), Hellenistic, chart1, when2, bassum, store.ReportLabel{}, objVenus)
	if err != nil {
		t.Fatal(err)
	}
	check("synastry", keepText(syn, objVenus))
	cross, _, err := CrossBodyRawReport(context.Background(), Hellenistic, chart1, when2, bassum, objVenus)
	if err != nil {
		t.Fatal(err)
	}
	check("transit", keepText(cross, objVenus))
}

// TestKeepCursorSideByPrintedName: the cursor's side is matched by the name
// Astrolog prints right of "with" (ten cells), for any object, not only the
// ones Sextile has a glyph for.
func TestKeepCursorSideByPrintedName(t *testing.T) {
	report := "Astrolog 8.00 chart for X\nChart #2 comparison for Y\n\n" +
		"  1:     Sun (Ari) Tri with (Leo) Fortune    - orb: s0:37' - power:  9.41\n" +
		"  2: Ascenda (Sag) Tri with (Leo) Ascendant  - orb: a1:20' - power:  6.07\n" +
		"  3: Fortune (Sag) Con with (Sag) 11th Cusp  - orb: s2:19' - power:  4.68\n" +
		"  4:    Juno (Aqu) Squ with (Tau) Nadir      - orb: a0:03' - power:  3.96\n" +
		"\nSum power: 24.12 - Average power: 6.03\n"
	for _, c := range []struct {
		o    aspect.Object
		rows string
	}{
		{aspect.Object{Number: 19, Name: "Fortune"}, "1"},
		{aspect.Object{Number: 22, Name: "Ascendant"}, "1"},
		{aspect.Object{Number: 32, Name: "11th Cusp"}, "1"},
		{aspect.Object{Number: 25, Name: "Nadir"}, "1"},
		{aspect.Object{Number: 5, Name: "Mars"}, ""},
	} {
		got, _, _ := KeepCursorSide(report, c.o)
		if rows := strings.Join(rowNumbers(got), ","); rows != c.rows {
			t.Errorf("%s: rows kept = %q, want %q\n%s", c.o.Name, rows, c.rows, got)
		}
	}
	// The row that survives for Fortune is the one whose RIGHT end is Fortune,
	// not the one that has it on the left.
	got, _, _ := KeepCursorSide(report, aspect.Object{Number: 19, Name: "Fortune"})
	if !strings.Contains(got, "with (Leo) Fortune") || strings.Contains(got, "Fortune (Sag)") {
		t.Errorf("wrong side kept:\n%s", got)
	}
}
