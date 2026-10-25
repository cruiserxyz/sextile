package astrolog

import (
	"reflect"
	"strings"
	"testing"
)

const ownListText = "Astrolog 8.00 chart for Tue Aug 25, 2026 14:00 (DT Zone 1E)  13°25E 52°31N\n" +
	"\n" +
	"  1:     Sun (Vir) Con (Vir) Mercury    - orb: a2°14' - power: 14.40\n" +
	"  2:    Moon (Aqu) Squ [Tau] Chiron     - orb: s0°50' - power: 11.09\n" +
	"  3:    Moon (Aqu) Sex [Ari] Neptune    - orb: a2°18' - power:  6.63\n" +
	"\n" +
	"Sum power: 32.12 - Average power: 10.71\n" +
	"Con:  1   Opp:  0   Squ:  1   Tri:  0   Sex:  1\n" +
	"Sun:  1   Moo:  2   Mer:  1   Ven:  0   Mar:  0   Jup:  0   Sat:  0   Ura:  0\n" +
	"Nep:  1   Plu:  0   Chi:  1   Nor:  0   Sou:  0   Lil:  0   Asc:  0   Nad:  0\n"

const crossListText = "Astrolog 8.00 chart for Tue May 15, 1990 14:00 (DT Zone 1E)  13°25E 52°31N\n" +
	"Chart #2 comparison for Tue Aug 25, 2026 14:00 (DT Zone 1E)  13°25E 52°31N\n" +
	"\n" +
	"  1:    Moon (Cap) Con with (Aqu) Moon      - orb: s4°20' - power: 12.93\n" +
	"  2:    Moon (Cap) Squ with [Tau] Chiron    - orb: a3°29' - power:  6.98\n" +
	"\n" +
	"Sum power: 19.91 - Average power: 9.96\n" +
	"Con:  1   Opp:  0   Squ:  1   Tri:  0   Sex:  0\n"

// Acceptance 1, 2: the own-chart list is found with its 2 header lines, its
// rows and the 5 footer lines, and the spans tile the text.
func TestReportListSpansOwnChart(t *testing.T) {
	got, ok := ReportListSpans(ownListText, GrammarAspects)
	if !ok {
		t.Fatal("own-chart report has no list")
	}
	want := ReportList{
		Header:  LineSpan{0, 2},
		List:    LineSpan{2, 5},
		Footer:  LineSpan{5, 10},
		Records: []LineSpan{{2, 3}, {3, 4}, {4, 5}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("spans = %+v, want %+v", got, want)
	}
}

// Acceptance 3: the comparison list has 3 header lines under its own grammar,
// and the own-chart grammar does not read a `with` row.
func TestReportListSpansCross(t *testing.T) {
	got, ok := ReportListSpans(crossListText, GrammarCrossAspects)
	if !ok {
		t.Fatal("comparison report has no list")
	}
	if got.Header != (LineSpan{0, 3}) || got.List != (LineSpan{3, 5}) || got.Footer != (LineSpan{5, 8}) || len(got.Records) != 2 {
		t.Fatalf("spans = %+v", got)
	}
	if _, ok := ReportListSpans(crossListText, GrammarAspects); ok {
		t.Error("the own-chart grammar read a comparison row")
	}
	if _, ok := ReportListSpans(ownListText, GrammarCrossAspects); ok {
		t.Error("the cross grammar read an own-chart row")
	}
}

// Acceptance 4: indented, non-blank lines the grammar does not recognise
// belong to the record above them (a `-ma`-shaped continuation).
func TestReportListSpansMultiLineRecord(t *testing.T) {
	text := "Astrolog 8.00 chart for X\n" +
		"\n" +
		"  1:     Sun (Vir) Con (Vir) Mercury    - orb: a2°14' - power: 14.40\n" +
		"        Midpoint Sun makes Con with Mercury\n" +
		"        and a second note\n" +
		"  2:    Moon (Aqu) Squ [Tau] Chiron     - orb: s0°50' - power: 11.09\n" +
		"\n" +
		"Sum power: 25.49\n"
	got, ok := ReportListSpans(text, GrammarAspects)
	if !ok {
		t.Fatal("no list")
	}
	if !reflect.DeepEqual(got.Records, []LineSpan{{2, 5}, {5, 6}}) {
		t.Fatalf("records = %+v", got.Records)
	}
	if got.Footer != (LineSpan{6, 8}) {
		t.Fatalf("footer = %+v", got.Footer)
	}
}

// Acceptance 5: a foreign line (or a blank one) between two records, no record
// at all, and a grammar-less tab each mean "no list".
func TestReportListSpansNoList(t *testing.T) {
	row1 := "  1:     Sun (Vir) Con (Vir) Mercury    - orb: a2°14' - power: 14.40\n"
	row2 := "  2:    Moon (Aqu) Squ [Tau] Chiron     - orb: s0°50' - power: 11.09\n"
	cases := map[string]string{
		"foreign line between": "head\n\n" + row1 + "Interlude\n" + row2,
		// A numbered row the grammar rejects (Astrolog's no-data <…> body) is
		// indented like a continuation but is a row of its own: foreign.
		"unread numbered row": "head\n\n" + row1 + "  2:    Moon <Aqu> Squ [Tau] Chiron     - orb: s0°50' - power: 11.09\n" + row2,
		"blank between":       "head\n\n" + row1 + "\n" + row2,
		"no record":           "Astrolog 8.00 chart for X\n\nPlanet: 1\n",
		"empty":               "",
	}
	for name, text := range cases {
		if _, ok := ReportListSpans(text, GrammarAspects); ok {
			t.Errorf("%s: found a list", name)
		}
	}
	if _, ok := ReportListSpans(ownListText, GrammarNone); ok {
		t.Error("GrammarNone found a list")
	}
	if strings.Count(ownListText, "\n") == 0 {
		t.Fatal("fixture broke")
	}
}
