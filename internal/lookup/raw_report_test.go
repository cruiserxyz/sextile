package lookup

import (
	"context"
	"os/exec"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/store"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

func TestComputeRawReportRejectsInvalidRequestBeforeSpawn(t *testing.T) {
	place := location.PlaceInfo{Name: "Berlin", Country: "DE", Lat: 52.52, Lon: 13.405, TZ: "Europe/Berlin"}
	when := time.Date(2000, 1, 1, 12, 0, 0, 0, time.UTC)
	for _, request := range []RawReportRequest{
		{},
		{Kind: RawReportBody, Body: aspect.BodyUnknown},
		{Kind: RawReportChart, Body: aspect.Moon},
		{Kind: RawReportAspects, Body: aspect.Moon},
	} {
		if _, _, err := ComputeRawReport(context.Background(), when, place, Hellenistic, request); err == nil {
			t.Errorf("invalid request %+v was accepted", request)
		}
	}
}

func TestComputeRawReportRejectsUntimedPlace(t *testing.T) {
	_, _, err := ComputeRawReport(
		context.Background(), time.Now(), location.PlaceInfo{Name: "Nowhere"}, Hellenistic,
		RawReportRequest{Kind: RawReportChart},
	)
	if err == nil {
		t.Fatal("raw report must reject a place without an IANA timezone")
	}
}

func TestComputeRawReportsAgainstLiveAstrolog(t *testing.T) {
	if _, err := exec.LookPath("astrolog"); err != nil {
		t.Skip("astrolog not on PATH; skipping live raw-report test")
	}
	place := location.PlaceInfo{Name: "London", Country: "GB", Lat: 51.5074, Lon: -0.1278, TZ: "Europe/London"}
	when := time.Date(2000, 1, 1, 12, 0, 0, 0, time.UTC)
	cases := []RawReportRequest{
		{Kind: RawReportChart},
		{Kind: RawReportBody, Body: aspect.Moon},
		{Kind: RawReportAspects},
	}
	for _, request := range cases {
		raw, _, err := ComputeRawReport(context.Background(), when, place, Hellenistic, request)
		if err != nil {
			t.Fatalf("ComputeRawReport(%v): %v", request.Kind, err)
		}
		if strings.TrimSpace(raw) == "" {
			t.Errorf("ComputeRawReport(%v) returned empty output", request.Kind)
		}
		if strings.Contains(raw, "\x1b[") {
			t.Errorf("ComputeRawReport(%v) returned ANSI-colored output", request.Kind)
		}
	}
}

// The chart report is Astrolog's influence chart, with the sign and house
// sections present. This is the end-to-end proof that the forced
// `=j0` beats whatever the host's astrolog.as sets: a bare `-j0` would
// toggle those sections OFF on a default installation.
func TestChartRawReportIsInfluenceChart(t *testing.T) {
	if _, err := exec.LookPath("astrolog"); err != nil {
		t.Skip("astrolog not on PATH; skipping live raw-report test")
	}
	place := location.PlaceInfo{Name: "London", Country: "GB", Lat: 51.5074, Lon: -0.1278, TZ: "Europe/London"}
	when := time.Date(2000, 1, 1, 12, 0, 0, 0, time.UTC)
	raw, _, err := ComputeRawReport(context.Background(), when, place, Hellenistic, RawReportRequest{Kind: RawReportChart})
	if err != nil {
		t.Fatalf("ComputeRawReport: %v", err)
	}
	for _, want := range []string{"Planet:", "Total:", "Sign:", "Element", "House:", "Angular"} {
		if !strings.Contains(raw, want) {
			t.Errorf("influence chart missing %q:\n%s", want, raw)
		}
	}
	// The -v listing it replaced is gone.
	if strings.Contains(raw, "House cusp") {
		t.Errorf("chart report is still the -v position listing:\n%s", raw)
	}
}

// aspectRowRE matches one numbered row of an Astrolog aspect list.
var aspectRowRE = regexp.MustCompile(`^\s*\d+:`)

// The comparison body report is the relationship list restricted to one
// object: every row it prints has that body at one of its two ends,
// and it is a strict subset of the unrestricted cross list.
func TestCrossBodyRawReportAgainstLiveAstrolog(t *testing.T) {
	if _, err := exec.LookPath("astrolog"); err != nil {
		t.Skip("astrolog not on PATH; skipping live raw-report test")
	}
	place := location.PlaceInfo{Name: "Berlin", Country: "DE", Lat: 52.5244, Lon: 13.4105, TZ: "Europe/Berlin"}
	natal := store.AnchorChart{When: time.Date(1990, 5, 15, 12, 0, 0, 0, time.UTC), Place: place}
	transitWhen := time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)

	raw, _, err := CrossBodyRawReport(context.Background(), Hellenistic, natal, transitWhen, place, aspect.Moon)
	if err != nil {
		t.Fatalf("CrossBodyRawReport: %v", err)
	}
	if strings.Contains(raw, "\x1b[") {
		t.Errorf("CrossBodyRawReport returned ANSI-colored output:\n%s", raw)
	}
	if !strings.Contains(raw, "comparison") {
		t.Errorf("report is not a two-chart run:\n%s", raw)
	}
	rows := 0
	for _, line := range strings.Split(raw, "\n") {
		if !aspectRowRE.MatchString(line) {
			continue
		}
		rows++
		if !strings.Contains(line, "Moon") {
			t.Errorf("row without the restricted body: %q", line)
		}
	}
	if rows == 0 {
		t.Fatalf("no aspect rows in the restricted report:\n%s", raw)
	}
	full, err := OverlayCrossAspects(context.Background(), Hellenistic, natal, transitWhen, place)
	if err != nil {
		t.Fatalf("OverlayCrossAspects: %v", err)
	}
	fullRows := 0
	for _, line := range strings.Split(full.Report, "\n") {
		if aspectRowRE.MatchString(line) {
			fullRows++
		}
	}
	if rows >= fullRows {
		t.Errorf("restricted report has %d rows, unrestricted %d — -RO did not restrict", rows, fullRows)
	}
}

func TestCrossBodyRawReportRejectsUntimedPlace(t *testing.T) {
	natal := store.AnchorChart{When: time.Now(), Place: location.PlaceInfo{Name: "Nowhere"}}
	if _, _, err := CrossBodyRawReport(context.Background(), Hellenistic, natal, time.Now(), location.PlaceInfo{Name: "Nowhere"}, aspect.Moon); err == nil {
		t.Fatal("cross body report must reject a place without an IANA timezone")
	}
}
