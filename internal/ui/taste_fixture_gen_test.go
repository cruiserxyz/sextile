package ui

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/lookup"
)

// TestGenScrollFixtures regenerates the earlier change taste fixtures from the real
// Astrolog binary, so the frames the operator approved are Astrolog's own
// output and not a hand-written stand-in (gate 12: inspectable provenance).
// Skipped by default — it needs `astrolog` on PATH.
//
//	GEN_FIXTURE=1 go test ./internal/ui -run TestGenScrollFixtures -v
//
// testdata/influence-chart.report is written directly; the positions it logs
// are the source of fullChartPositions() in bodies_scroll_test.go.
func TestGenScrollFixtures(t *testing.T) {
	if os.Getenv("GEN_FIXTURE") == "" {
		t.Skip("set GEN_FIXTURE=1 to regenerate from the real astrolog binary")
	}
	place := location.PlaceInfo{Name: "Berlin", Country: "DE", Lat: 52.5244, Lon: 13.4105, TZ: "Europe/Berlin"}
	when := time.Date(1990, 6, 15, 14, 30, 0, 0, time.UTC)

	raw, _, err := lookup.ComputeRawReport(context.Background(), when, place, lookup.Hellenistic, lookup.RawReportRequest{Kind: lookup.RawReportChart})
	if err != nil {
		t.Fatalf("influence chart: %v", err)
	}
	if err := os.WriteFile("testdata/influence-chart.report", []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Logf("testdata/influence-chart.report: %d lines", len(detailsReportLines(raw)))

	out, err := lookup.FirstCallTransits(context.Background(), nil, when, place, lookup.Hellenistic)
	if err != nil {
		t.Fatalf("chart: %v", err)
	}
	var sb strings.Builder
	for _, p := range out.Positions {
		sb.WriteString(fmt.Sprintf("\t\t{Body: aspect.%v, Longitude: %.4f, Speed: %.4f, House: %d},\n", p.Body, p.Longitude, p.Speed, p.House))
	}
	t.Logf("fullChartPositions() — %d rows:\n%s", len(out.Positions), sb.String())
}
