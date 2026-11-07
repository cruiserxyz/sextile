package ui

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// details-key: a Details report carries the question it answered and lands only
// for the question the screen asks now (specs/details-key.md).

// stubNumberedReports makes every report distinct and every spawn a Log line:
// the nth call returns "report n" and a command, so a result that paints or
// logs when it should not is visible in the lines and in the Log.
func stubNumberedReports(t *testing.T) {
	t.Helper()
	n := 0
	stubSpawns(t, func(s *rowSpawns) {
		s.reports.raw = func(_ context.Context, _ time.Time, _ location.PlaceInfo, _ lookup.Lens, _ lookup.RawReportRequest) (string, string, error) {
			n++
			return "report " + string(rune('0'+n)), "astrolog -x", nil
		}
	})
}

// heldMercuryReport is Mercury's report command run and held, as a late or
// replayed message would be: the model is then settled on Mercury, and has
// not been told about it.
func heldMercuryReport(t *testing.T) (Model, refreshResultMsg) {
	t.Helper()
	freshModels(t)
	stubNumberedReports(t)
	m := detailsTestModel(t)
	m.cfg = Config{Color: ColorNever}
	m.focused = PanelBodies
	m.chart.positions = []aspect.Position{{Object: objMercury}, {Object: objVenus}, {Object: objMars}}
	m.chart.settled = true // the frame beside the report is a committed one
	m.bodyCursorIdx = 0
	m, cmd := m.requestDetails()
	m = pump(t, m, cmd)
	if m.details.title != "[0] Mercury" || m.details.body != "report 1" {
		t.Fatalf("fixture: title %q body %q", m.details.title, m.details.body)
	}
	return m, runDetailsCmd(t, m) // the held message: Mercury, "report 2"
}

// asCurrent sets the run id, the want id and the floor so the message passes
// every id gate, as a replayed message would.
func asCurrent(m Model, msg refreshResultMsg) Model {
	m.refresh[surfaceDetails].runningRunID = msg.runID
	m.refresh[surfaceDetails].wantID = msg.wantID
	m.refresh[surfaceDetails].commitFloor = msg.wantID
	return m
}

// keyNow is the key the Model asks for now, for a hand-built result that must
// pass the key gate (a real run carries it from buildDetailsCmd).
func keyNow(m Model) detailsKey {
	k, _ := m.detailsKeyFor()
	return k
}

type detailsScreen struct {
	title, body string
	lines       int
	log         int
}

func screenOf(m Model) detailsScreen {
	return detailsScreen{m.details.title, m.details.body, len(m.detailsView().Lines), len(m.logLines)}
}

// Acceptance 1.
func TestDetailsResultForOlderKeyNeverReplacesLines(t *testing.T) {
	m, held := heldMercuryReport(t)
	m, _ = tpress(t, m, keyDown) // Venus; its report is NOT pumped
	m = asCurrent(m, held)
	before := screenOf(m)
	m, _ = m.onRefreshResult(held)
	if after := screenOf(m); after != before {
		t.Errorf("a Mercury report painted under Venus: before %+v, after %+v", before, after)
	}
}

// Acceptance 2. The same body on another base (Moon in 3, then Moon in 2).
func TestDetailsResultForAnotherBaseNeverReplacesLines(t *testing.T) {
	stubNumberedReports(t)
	stubCrossBodyReports(t)
	stubSynastryBodyReports(t)
	m := selectedSet(t, 3, false, 0)
	m.cfg = Config{Color: ColorNever}
	m.width, m.height = dualColumnMinWidth(4), 40
	m.focused = PanelBodies
	m.chart.positions = []aspect.Position{{Object: objMoon}}
	m.bodyCursorIdx = 0
	m.base = m.slots.All()[2]
	m, cmd := m.startDetailsNow()
	m = pump(t, m, cmd)
	if m.details.title != "[0] Moon 1 in 3" {
		t.Fatalf("fixture: title %q", m.details.title)
	}
	held := runDetailsCmd(t, m)
	late := held.payload.(detailsPayload) // the stubs are constant: make the late body visible
	late.body = "late synastry raw"
	held.payload = late
	m.base = m.slots.All()[1] // the tab changed: Moon 1 in 2
	if req, ok := m.detailsRequestFor(); !ok || req.title != "[0] Moon 1 in 2" {
		t.Fatalf("fixture: request after the tab changed = %q ok=%v", req.title, ok)
	}
	m = asCurrent(m, held)
	before := screenOf(m)
	m, _ = m.onRefreshResult(held)
	if after := screenOf(m); after != before {
		t.Errorf("a report for the old base painted: before %+v, after %+v", before, after)
	}
}

// Acceptance 4. A dropped result writes no Log line: no argv line, no note.
func TestDetailsKeyDropWritesNoLogLine(t *testing.T) {
	for _, withErr := range []bool{false, true} {
		m, held := heldMercuryReport(t)
		if withErr {
			held.err = errors.New("astrolog failed")
		}
		m, _ = tpress(t, m, keyDown)
		m = asCurrent(m, held)
		before := len(m.logLines)
		m, _ = m.onRefreshResult(held)
		if got := len(m.logLines); got != before {
			t.Errorf("error=%v: a dropped result wrote %d Log line(s)", withErr, got-before)
		}
	}
}

// Acceptance 6. The ids still drop by themselves, with the key equal.
func TestDetailsOtherGatesStillDropWithKeyEqual(t *testing.T) {
	cases := map[string]func(Model, refreshResultMsg) Model{
		"below the floor": func(m Model, msg refreshResultMsg) Model {
			m = asCurrent(m, msg)
			m.refresh[surfaceDetails].commitFloor = msg.wantID + 1
			m.refresh[surfaceDetails].wantID = msg.wantID + 1
			return m
		},
		"superseded want": func(m Model, msg refreshResultMsg) Model {
			m = asCurrent(m, msg)
			m.refresh[surfaceDetails].wantID = msg.wantID + 1
			return m
		},
		"superseded run": func(m Model, msg refreshResultMsg) Model {
			m = asCurrent(m, msg)
			m.refresh[surfaceDetails].runningRunID = msg.runID + 1
			return m
		},
	}
	for name, arrange := range cases {
		m, held := heldMercuryReport(t)
		m = arrange(m, held)
		before := screenOf(m)
		m, _ = m.onRefreshResult(held)
		if after := screenOf(m); after != before {
			t.Errorf("%s: the result painted: before %+v, after %+v", name, before, after)
		}
	}
}

// Acceptance 3. Typing in the report's search or scrolling is presentation:
// the key is unchanged and the report in flight still lands.
func TestDetailsKeyIgnoresSearchAndScroll(t *testing.T) {
	m, _ := heldMercuryReport(t)
	m, cmd := tpress(t, m, keyDown) // Venus; the report is in flight
	before, ok := m.detailsKeyFor()
	if !ok {
		t.Fatal("no key for the Venus request")
	}
	m.details.query, m.details.searching = "sun", true
	m = m.scrollDetails(2)
	if after, _ := m.detailsKeyFor(); after != before {
		t.Errorf("typing and scrolling changed the key: %+v -> %+v", before, after)
	}
	m = pump(t, m, cmd)
	if m.details.title != "[0] Venus" || m.details.pending {
		t.Errorf("the report did not land: title %q pending %v", m.details.title, m.details.pending)
	}
}

// Acceptance 5. The run carries the key the Model builds at dispatch, on the
// success and the error result; a result for the current key lands as before.
func TestDetailsRunCarriesTheKeyOfItsDispatch(t *testing.T) {
	m, held := heldMercuryReport(t)
	want, ok := m.detailsKeyFor()
	if !ok {
		t.Fatal("no key at dispatch")
	}
	if got := held.payload.(detailsPayload).key; got != want {
		t.Errorf("success result key = %+v, want %+v", got, want)
	}
	failing := withSpawns(m, func(s *rowSpawns) {
		s.reports.raw = func(context.Context, time.Time, location.PlaceInfo, lookup.Lens, lookup.RawReportRequest) (string, string, error) {
			return "", "astrolog -x", errors.New("astrolog failed")
		}
	})
	if got := runDetailsCmd(t, failing).payload.(detailsPayload).key; got != want {
		t.Errorf("error result key = %+v, want %+v", got, want)
	}

	// The current key lands exactly as before: same title keeps the reader's
	// place and the query; the new text replaces the lines.
	m.details.query, m.details.scrollOffset = "sun", 0
	m = asCurrent(m, held)
	m, _ = m.onRefreshResult(held)
	if m.details.title != "[0] Mercury" || m.details.body != "report 2" || m.details.query != "sun" {
		t.Errorf("current-key result: title %q body %q query %q", m.details.title, m.details.body, m.details.query)
	}
}
