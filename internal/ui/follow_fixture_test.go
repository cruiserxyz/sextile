package ui

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/internal/store"
	"github.com/cruiserxyz/sextile/internal/ui/panel"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// The fixture of specs/details-follows-its-tab.md (the Taste lane): the taste
// fixtures' frozen clock (Sat Apr 25, 2026 14:00 CEST, Berlin, the floating
// `Now`) and one anchored synthetic chart (`Synthetic 1`, Testford), the
// default lens and no selection; the Chart panel focused with its cursor on
// `Now`, Details on its report tab, the Bodies cursor on the Sun. A stand-in
// Astrolog answers every cast and report at once; a test that wants a result
// held does not run the command that fetches it.

// influenceHead is the first line of the Chart's influence table.
const influenceHead = "Planet:    Position      Aspects    Total Rank  Percent"

// followStub is the stand-in Astrolog of the fixture.
type followStub struct {
	// failBodies makes every body report fail.
	failBodies bool
	// reports counts the report spawns, casts the frame casts.
	reports, casts int
}

// followFixture builds the fixture and commits Now's text frame and Now's
// Chart report. It is the state every V row starts from. The stand-in marks
// everything it computes with its subject (`SUBJECT <kind> <object> <who>` in
// a report, and the sky and the natal chart at different longitudes), so a
// test can read from a drawn frame which subject a body belongs to.
func followFixture(t *testing.T, cfg Config, w, h int) (Model, *followStub) {
	t.Helper()
	stub := &followStub{}
	sky := []aspect.Position{
		{Object: objSun, Longitude: 35.5},
		{Object: objMoon, Longitude: 120.25},
		{Object: objMercury, Longitude: 50.1},
		{Object: objVenus, Longitude: 75.9},
		{Object: objMars, Longitude: 300.4},
	}
	natal := []aspect.Position{
		{Object: objSun, Longitude: 100.5},
		{Object: objMoon, Longitude: 200.25},
		{Object: objMercury, Longitude: 110.1},
		{Object: objVenus, Longitude: 150.9},
		{Object: objMars, Longitude: 20.4},
	}
	stubSpawns(t, func(s *rowSpawns) {
		s.cast.transit = func(_ context.Context, _ *lookup.Probers, _ time.Time, _ location.PlaceInfo, _ lookup.Lens) (lookup.Result, error) {
			stub.casts++
			return lookup.Result{Positions: sky, AspectsReport: "SUBJECT aspects floating\n"}, nil
		}
		s.cast.natal = func(context.Context, *lookup.Probers, store.AnchorChart, lookup.Lens) (lookup.Result, error) {
			stub.casts++
			return lookup.Result{Positions: natal, AspectsReport: "SUBJECT aspects natal\n"}, nil
		}
		s.cast.overlay = func(context.Context, lookup.Lens, store.AnchorChart, time.Time, location.PlaceInfo) (lookup.OverlayAspects, error) {
			stub.casts++
			return lookup.OverlayAspects{Report: "SUBJECT cross\n"}, nil
		}
		s.cast.synastry = func(context.Context, lookup.Lens, store.AnchorChart, store.AnchorChart) (lookup.OverlayAspects, error) {
			stub.casts++
			return lookup.OverlayAspects{Report: "SUBJECT cross\n"}, nil
		}
		s.reports.raw = func(_ context.Context, when time.Time, _ location.PlaceInfo, _ lookup.Lens, req lookup.RawReportRequest) (string, string, error) {
			stub.reports++
			who := "floating"
			if when.Year() < 2000 {
				who = "natal"
			}
			switch req.Kind {
			case lookup.RawReportChart:
				return influenceHead + "\nSun        1 Tau 35'     3          12    1        8.1%\nSUBJECT chart - " + who + "\n", "", nil
			default:
				if stub.failBodies {
					return "", "", errors.New("astrolog stopped")
				}
				return fmt.Sprintf("SUBJECT body %s %s\nreport at %s\n", panel.BodyName(req.Object), who, when.Format("15:04 Jan 2")), "", nil
			}
		}
	})
	m := newTestModel(t, cfg, FrozenClock(testFrozenTime()))
	slots, err := store.NewSlots(testFrozenTime(), testPlaceBerlin())
	if err != nil {
		t.Fatalf("NewSlots: %v", err)
	}
	m.slots = slots
	m.place = testPlaceBerlin()
	m.targetFloatingTime = slots.Floating().Time
	m.startup = startupReady
	m.width, m.height = w, h
	openChart(t, m, 1)
	m.cursorIdx = len(m.slots.All()) - 1 // Now, the floating slot, last in the list
	m.focused = PanelChart
	m, cmd := m.bumpIdentity(surfaceText)
	m = pump(t, m, cmd)
	// Below the dual-column width Details is not in the layout and owes no report.
	if !m.shown.keyed || (m.detailsVisible() && (m.details.body == "" || m.details.key.kind != reportRaw)) {
		t.Fatalf("fixture: Now's frame and Chart report are not committed: body %q", m.details.body)
	}
	return m, stub
}

// pressOnly delivers k and returns the model and the command it started,
// which the caller runs, or holds by not running.
func pressOnly(t *testing.T, m Model, k tea.KeyMsg) (Model, tea.Cmd) {
	t.Helper()
	return tpress(t, m, k)
}
