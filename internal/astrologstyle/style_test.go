package astrologstyle

import (
	"strings"
	"testing"
)

func TestParseBodiesPreservesIndependentAstrologSpans(t *testing.T) {
	const body = "\x1b[31mSun :\x1b[0m  \x1b[32m15Ari20\x1b[0m   - 0:00' (X) \x1b[34m[ 8th house]\x1b[0m [-] +0.993"

	rows, err := ParseBodies(body)
	if err != nil {
		t.Fatalf("ParseBodies: %v", err)
	}
	row, ok := rows["Sun"]
	if !ok {
		t.Fatalf("ParseBodies did not return Sun row: %#v", rows)
	}
	if row.Body != "\x1b[31m" || row.Position != "\x1b[32m" || row.House != "\x1b[34m" {
		t.Fatalf("independent spans = %#v, want body/position/house ANSI codes", row)
	}
}

func TestParseBodiesRejectsPartiallyUncoloredRow(t *testing.T) {
	const body = "\x1b[31mSun :\x1b[0m  15Ari20   - 0:00' (X) \x1b[34m[ 8th house]\x1b[0m [-] +0.993"

	_, err := ParseBodies(body)
	if err == nil || !strings.Contains(err.Error(), "uncolored body row") {
		t.Fatalf("ParseBodies error = %v, want uncolored body row", err)
	}
}

func TestParseBodiesRejectsOutputWithoutBodyRows(t *testing.T) {
	_, err := ParseBodies("Astrolog diagnostic output\n")
	if err == nil || !strings.Contains(err.Error(), "no colored body rows") {
		t.Fatalf("ParseBodies error = %v, want no colored body rows", err)
	}
}

func TestParseAspectRowsPreservesIndependentAstrologSpans(t *testing.T) {
	const report = "  1: \x1b[1;31m    Sun\x1b[1;31m (Ari) \x1b[1;33mCon \x1b[1;31m(Ari) \x1b[1;34mMoon      \x1b[0;37m - orb: +1:08'\x1b[0;32m - power: 23.99"

	rows, err := ParseAspectRows(report)
	if err != nil {
		t.Fatalf("ParseAspectRows: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("ParseAspectRows returned %d rows, want 1", len(rows))
	}
	row := rows[0]
	if row.FirstName != "Sun" || row.KindName != "Con" || row.SecondName != "Moon" {
		t.Fatalf("identity tokens = %#v", row)
	}
	if row.Style.First != "\x1b[1;31m" || row.Style.Kind != "\x1b[1;33m" || row.Style.Second != "\x1b[1;34m" {
		t.Fatalf("aspect spans = %#v", row.Style)
	}
}

func TestParseCrossAspectRowsPreservesRolesAndSpans(t *testing.T) {
	const report = "  1: \x1b[1;34m   Moon\x1b[1;33m (Tau) \x1b[1;36mSex with \x1b[1;34m(Can) \x1b[1;31mSun       \x1b[1;37m- orb: -1:10'\x1b[0;32m - power: 14.09"

	rows, err := ParseCrossAspectRows(report)
	if err != nil {
		t.Fatalf("ParseCrossAspectRows: %v", err)
	}
	row := rows[0]
	if row.FirstName != "Moon" || row.KindName != "Sex" || row.SecondName != "Sun" {
		t.Fatalf("cross identity tokens = %#v", row)
	}
	if row.Style.First != "\x1b[1;34m" || row.Style.Kind != "\x1b[1;36m" || row.Style.Second != "\x1b[1;31m" {
		t.Fatalf("cross spans = %#v", row.Style)
	}
}

func TestParseAspectRowsRejectsPartiallyUncoloredRow(t *testing.T) {
	const report = "  1: \x1b[1;31mSun\x1b[0m (Ari) Con (Ari) \x1b[1;34mMoon\x1b[0m - orb: +1:08' - power: 23.99"

	_, err := ParseAspectRows(report)
	if err == nil || !strings.Contains(err.Error(), "partially uncolored aspect row") {
		t.Fatalf("ParseAspectRows error = %v, want partially uncolored aspect row", err)
	}
}

func TestParseAspectRowsAllowsAReportWithoutAspects(t *testing.T) {
	rows, err := ParseAspectRows("Sum power: 0.00 - Average power: 0.00\n")
	if err != nil || len(rows) != 0 {
		t.Fatalf("ParseAspectRows = %#v, %v; want empty valid result", rows, err)
	}
}
