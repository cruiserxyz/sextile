package ui

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/internal/store"
	"github.com/cruiserxyz/sextile/internal/ui/panel"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// cursorOnObject puts the Bodies cursor on the row of Astrolog object n.
func cursorOnObject(t *testing.T, m Model, n int) Model {
	t.Helper()
	ch := m.shownChart()
	for i := 0; i < len(ch.positions)+12; i++ {
		if o, ok := panel.ObjectAtCursor(ch.positions, ch.cusps, i); ok && o.Number == n {
			m.bodyCursorIdx = i
			return m
		}
	}
	t.Fatalf("no Bodies row for object %d", n)
	return m
}

// The per-body Details report works for every object, not only the ones
// Sextile has glyphs for: the cursor on Fortune asks for Fortune's report.
func TestDetailsReportForFortune(t *testing.T) {
	calls := stubRawReports(t)
	m := detailsTestModel(t)
	m.chart.positions = append(m.chart.positions, aspect.Position{Object: fixtureObject(t, 19), Longitude: 300})
	m.focused = PanelBodies
	m = cursorOnObject(t, m, 19)

	req, ok := m.detailsRequestFor()
	if !ok {
		t.Fatal("no Details request with the cursor on Fortune")
	}
	if !strings.HasSuffix(req.title, "Fortune") || strings.HasSuffix(req.title, "Fortune ") {
		t.Errorf("title = %q, want it to end in Fortune", req.title)
	}
	if got := m.detailsWanted().caption; got != "Fortune" {
		t.Errorf("caption = %q, want Fortune", got)
	}
	msg := runDetailsCmd(t, m)
	if len(*calls) != 1 {
		t.Fatalf("spawns = %d, want one body report", len(*calls))
	}
	call := (*calls)[0]
	if call.request.Kind != lookup.RawReportBody || call.request.Object.Number != 19 {
		t.Fatalf("request = %+v, want a body report for object 19", call.request)
	}
	if got := msg.payload.(detailsPayload).title; !strings.HasSuffix(got, "Fortune") {
		t.Errorf("payload title = %q", got)
	}

	// The argv Astrolog is given carries the number.
	if _, err := exec.LookPath("astrolog"); err != nil {
		t.Skip("astrolog is not on PATH")
	}
	_, command, err := lookup.ComputeRawReport(context.Background(), time.Date(2000, 1, 1, 12, 0, 0, 0, time.UTC),
		location.PlaceInfo{Name: "Berlin", Country: "DE", Lat: 52.52, Lon: 13.405, TZ: "Europe/Berlin"}, lookup.Hellenistic, call.request)
	if err != nil {
		t.Fatalf("ComputeRawReport: %v", err)
	}
	if !strings.Contains(command, "-RO 19") {
		t.Errorf("command = %q, want -RO 19", command)
	}
}

// On a pair tab the report is the cross report; a ticked cusp point is an
// object like any other, so the request carries its number and the filter keeps
// the rows whose right end prints its name.
func TestDetailsPairReportForATickedCusp(t *testing.T) {
	var got aspect.Object
	stubSpawns(t, func(s *rowSpawns) {
		s.reports.cross = func(_ context.Context, _ lookup.Lens, _ store.AnchorChart, _ time.Time, _ location.PlaceInfo, o aspect.Object) (string, string, error) {
			got = o
			return "Astrolog 8.00 chart for X\nChart #2 comparison for Y\n\n" +
				"  1:     Sun (Ari) Tri with (Leo) 2nd Cusp   - orb: s0:37' - power:  9.41\n" +
				"  2: 2nd Cus (Sag) Con with (Sag) Moon       - orb: s2:19' - power:  6.68\n\n" +
				"Sum power: 16.09 - Average power: 8.04\n", "astrolog -rt", nil
		}
	})
	m := tabsOnPair(t, Config{Accessible: true})
	m.width, m.height = 144, 40
	m.focused = PanelBodies
	m.chart.positions = []aspect.Position{{Object: objSun}, {Object: fixtureObject(t, 23), Longitude: 40}}
	m.chart.cusps = fullChartCusps()
	m = cursorOnObject(t, m, 23)

	req, ok := m.detailsRequestFor()
	if !ok || !req.cross {
		t.Fatalf("ok=%v cross=%v, want a cross request for the ticked 2nd Cusp", ok, req.cross)
	}
	if !strings.Contains(req.title, "2nd Cusp") {
		t.Errorf("title = %q, want it to name the 2nd Cusp", req.title)
	}
	msg := runDetailsCmd(t, m)
	if got.Number != 23 {
		t.Fatalf("cross report asked for %+v, want object 23", got)
	}
	body := msg.payload.(detailsPayload).body
	if !strings.Contains(body, "with (Leo) 2nd Cusp") || strings.Contains(body, "2nd Cus (Sag)") {
		t.Errorf("the cursor side was not kept:\n%s", body)
	}
}
