package lookup

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/cruiserxyz/sextile/internal/astrolog"
	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/store"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// Acceptance 9 of the specification: against the live binary, for
// both list kinds, the list the grammar finds holds exactly Astrolog's rows,
// numbered 1..n with no gap, and n is the count of numbered rows in the whole
// text (no row outside the list, none inside it that is not a row).
func TestReportListHoldsExactlyAstrologsRowsLive(t *testing.T) {
	if _, err := exec.LookPath("astrolog"); err != nil {
		t.Skip("astrolog not on PATH; skipping live report-list test")
	}
	place := location.PlaceInfo{Name: "Berlin", Country: "DE", Lat: 52.5244, Lon: 13.4105, TZ: "Europe/Berlin"}
	natal := store.AnchorChart{When: time.Date(1990, 5, 15, 12, 0, 0, 0, time.UTC), Place: place}
	when := time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)
	ctx := context.Background()

	type report struct {
		name    string
		grammar astrolog.ReportGrammar
		text    string
	}
	var reports []report
	own, _, err := ComputeRawReport(ctx, when, place, Hellenistic, RawReportRequest{Kind: RawReportAspects})
	if err != nil {
		t.Fatal(err)
	}
	reports = append(reports, report{"own-chart list", astrolog.GrammarAspects, own})
	body, _, err := ComputeRawReport(ctx, when, place, Hellenistic, RawReportRequest{Kind: RawReportBody, Body: aspect.Moon})
	if err != nil {
		t.Fatal(err)
	}
	reports = append(reports, report{"single body report", astrolog.GrammarAspects, body})
	full, err := OverlayCrossAspects(ctx, Hellenistic, natal, when, place)
	if err != nil {
		t.Fatal(err)
	}
	reports = append(reports, report{"comparison list", astrolog.GrammarCrossAspects, full.Report})
	cross, _, err := CrossBodyRawReport(ctx, Hellenistic, natal, when, place, aspect.Moon)
	if err != nil {
		t.Fatal(err)
	}
	reports = append(reports, report{"cross body report", astrolog.GrammarCrossAspects, cross})
	syn, _, err := SynastryCrossBodyRawReport(ctx, Hellenistic, natal, when, place, store.ReportLabel{}, aspect.Moon)
	if err != nil {
		t.Fatal(err)
	}
	reports = append(reports, report{"synastry body report", astrolog.GrammarCrossAspects, syn})

	for _, r := range reports {
		spans, ok := astrolog.ReportListSpans(r.text, r.grammar)
		if !ok {
			t.Errorf("%s: no list found:\n%s", r.name, r.text)
			continue
		}
		lines := strings.Split(strings.TrimSuffix(r.text, "\n"), "\n")
		rows := 0
		for _, ln := range lines {
			if aspectRowRE.MatchString(ln) {
				rows++
			}
		}
		if len(spans.Records) != rows {
			t.Errorf("%s: %d records, %d numbered rows in the text", r.name, len(spans.Records), rows)
		}
		for i, rec := range spans.Records {
			if rec.End-rec.Start != 1 {
				t.Errorf("%s: record %d spans %d lines, want 1", r.name, i+1, rec.End-rec.Start)
			}
			if got := strings.TrimSpace(lines[rec.Start]); !strings.HasPrefix(got, fmt.Sprintf("%d:", i+1)) {
				t.Errorf("%s: record %d is %q, want it numbered %d", r.name, i+1, got, i+1)
			}
		}
		for i, ln := range lines {
			inList := i >= spans.List.Start && i < spans.List.End
			if !inList && aspectRowRE.MatchString(ln) {
				t.Errorf("%s: numbered row outside the list: %q", r.name, ln)
			}
		}
	}
}
