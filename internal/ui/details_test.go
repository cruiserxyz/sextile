package ui

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/internal/store"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

type rawReportCall struct {
	when    time.Time
	request lookup.RawReportRequest
}

func stubRawReports(t *testing.T) *[]rawReportCall {
	t.Helper()
	singleCalls := []rawReportCall{}
	oldSingle := computeRawReport
	computeRawReport = func(_ context.Context, when time.Time, _ location.PlaceInfo, _ lookup.Lens, request lookup.RawReportRequest) (string, string, error) {
		singleCalls = append(singleCalls, rawReportCall{when: when, request: request})
		return "single raw", "", nil
	}
	t.Cleanup(func() {
		computeRawReport = oldSingle
	})
	return &singleCalls
}

type crossBodyCall struct {
	natal store.AnchorChart
	when  time.Time
	place location.PlaceInfo
	body  aspect.Body
}

// stubCrossBodyReports records calls to the comparison body report
// so a test can prove which of the two report paths ran.
func stubCrossBodyReports(t *testing.T) *[]crossBodyCall {
	t.Helper()
	calls := []crossBodyCall{}
	old := computeCrossBodyReport
	computeCrossBodyReport = func(_ context.Context, _ lookup.Lens, natal store.AnchorChart, when time.Time, place location.PlaceInfo, body aspect.Body) (string, string, error) {
		calls = append(calls, crossBodyCall{natal: natal, when: when, place: place, body: body})
		return "cross raw", "", nil
	}
	t.Cleanup(func() { computeCrossBodyReport = old })
	return &calls
}

// with a comparison pair selected, a body's Details is the
// transit→natal cross list for that body — one spawn, over BOTH charts, at
// the committed transit moment — and the title says so. Deselecting the pair
// returns the single-chart report unchanged.
func TestBodyDetailsFollowsComparisonPair(t *testing.T) {
	// Arrival at now lands on `Aspects`, which does not compare: the body
	// report is the floating chart's own (the specification C1).
	arrival := tabsPairModel(t, Config{Accessible: true})
	arrival.width, arrival.height = 144, 40
	arrival.focused = PanelBodies
	arrival.chart.positions = []aspect.Position{{Body: aspect.Moon}}
	arrival.bodyCursorIdx = 0
	if r, ok := arrival.detailsRequestFor(); !ok || r.cross {
		t.Fatalf("arrival at now: ok=%v cross=%v, want a single-chart request", ok, r.cross)
	}

	// The pair tab, reached with a real `]`, is the comparison.
	m := tabsOnPair(t, Config{Accessible: true})
	m.width, m.height = 144, 40
	m.focused = PanelBodies
	m.chart.positions = []aspect.Position{{Body: aspect.Moon}}
	m.bodyCursorIdx = 0

	req, ok := m.detailsRequestFor()
	if !ok {
		t.Fatal("no Details request with a pair selected")
	}
	if !req.cross {
		t.Error("body report did not follow the comparison")
	}
	// Changed by selected-charts-compare C4: the suffix is the pair's tab title.
	if req.title != "[0] Moon 2 in 1" {
		t.Errorf("title = %q", req.title)
	}
	if req.natalChart != anchorHandle(m) {
		t.Errorf("natal end = %+v, want the anchored chart %+v", req.natalChart, anchorHandle(m))
	}
	if !req.when.Equal(m.slots.Floating().Time) {
		t.Errorf("transit end = %v, want the committed floating time %v", req.when, m.slots.Floating().Time)
	}

	single := stubRawReports(t)
	cross := stubCrossBodyReports(t)
	msg := runDetailsCmd(t, m)
	if len(*cross) != 1 || len(*single) != 0 {
		t.Fatalf("spawns: cross=%d single=%d, want exactly one cross report", len(*cross), len(*single))
	}
	if (*cross)[0].body != aspect.Moon || !(*cross)[0].when.Equal(m.slots.Floating().Time) {
		t.Errorf("cross call = %+v", (*cross)[0])
	}
	if msg.payload.(detailsPayload).body != "cross raw" {
		t.Errorf("payload = %+v", msg.payload)
	}

	// Deselecting the pair restores the focused slot's own report.
	m.slots.SetComparisonSelected(false)
	req, ok = m.detailsRequestFor()
	if !ok || req.cross {
		t.Fatalf("after deselect: ok=%v cross=%v, want a single-chart request", ok, req.cross)
	}
	if req.title != "[0] Moon" {
		t.Errorf("after deselect: title = %q", req.title)
	}
	msg = runDetailsCmd(t, m)
	if len(*cross) != 1 || len(*single) != 1 {
		t.Fatalf("after deselect spawns: cross=%d single=%d, want one single-chart report", len(*cross), len(*single))
	}
	if msg.payload.(detailsPayload).body != "single raw" {
		t.Errorf("after deselect payload = %+v", msg.payload)
	}
}

func detailsTestModel(t *testing.T) Model {
	t.Helper()
	m := modelWithSlot(t)
	m.width = 132
	m.height = 40
	m.startup = startupReady
	m.place = testPlaceBerlin()
	m.chart.positions = []aspect.Position{{Body: aspect.Sun}, {Body: aspect.Moon}}
	return m
}

func runDetailsCmd(t *testing.T, m Model) refreshResultMsg {
	t.Helper()
	cmd := buildDetailsCmd(m, context.Background(), 7, 9)
	if cmd == nil {
		t.Fatal("buildDetailsCmd returned nil")
	}
	rawMsg := cmd()
	msg, ok := rawMsg.(refreshResultMsg)
	if !ok {
		t.Fatalf("details command returned %T, want refreshResultMsg", rawMsg)
	}
	return msg
}

func TestDetailsRoutesFocusedContext(t *testing.T) {
	singleCalls := stubRawReports(t)
	m := detailsTestModel(t)

	m.focused = PanelChart
	msg := runDetailsCmd(t, m)
	if got := msg.payload.(detailsPayload).title; got != "[0] Chart" {
		t.Errorf("chart title = %q", got)
	}
	if got := (*singleCalls)[0].request.Kind; got != lookup.RawReportChart {
		t.Errorf("chart request kind = %v", got)
	}

	m.focused = PanelBodies
	m.bodyCursorIdx = 1
	msg = runDetailsCmd(t, m)
	if got := msg.payload.(detailsPayload).title; got != "[0] Moon" {
		t.Errorf("body title = %q", got)
	}
	bodyCall := (*singleCalls)[1].request
	if bodyCall.Kind != lookup.RawReportBody || bodyCall.Body != aspect.Moon {
		t.Errorf("body request = %+v, want Moon body report", bodyCall)
	}

	// Aspects spawns nothing: its Details is the committed frame's own
	// report, so buildDetailsCmd has no request to run.
	m.focused = PanelAspects
	if cmd := buildDetailsCmd(m, context.Background(), 7, 9); cmd != nil {
		t.Error("Aspects focus produced a Details spawn; it must read the frame instead")
	}
	if len(*singleCalls) != 2 {
		t.Errorf("Aspects focus leaked into the single report path: %d calls", len(*singleCalls))
	}
}

func TestDetailsDoesNoHiddenWorkAtNarrowWidth(t *testing.T) {
	singleCalls := stubRawReports(t)
	m := detailsTestModel(t)
	m.width = 80

	if cmd := buildDetailsCmd(m, context.Background(), 1, 1); cmd != nil {
		t.Fatal("narrow layout built a hidden Details command")
	}
	m, teaCmd := m.requestDetails()
	if teaCmd != nil || len(*singleCalls) != 0 || m.refresh[surfaceDetails].cancel != nil {
		t.Fatalf("narrow request started Details work: cmd=%v calls=%d cancel=%v", teaCmd != nil, len(*singleCalls), m.refresh[surfaceDetails].cancel != nil)
	}

	m.width = 132
	msg := runDetailsCmd(t, m)
	if msg.err != nil || len(*singleCalls) != 1 {
		t.Fatalf("wide layout did not run Details: err=%v calls=%d", msg.err, len(*singleCalls))
	}
}

func TestDetailsFocusChangeAndResizeDropStaleResults(t *testing.T) {
	m := detailsTestModel(t)
	m.focused = PanelChart
	m.details = detailsViewState{title: "[0] Chart", body: "last chart report"}
	m, _ = m.requestDetails()
	chartRun := m.refresh[surfaceDetails]
	lastFrame := renderDetailsPanel(64, 18, (detailsFrameOf(m.details)).state.attached, Config{Accessible: true}, detailsFrameOf(m.details))

	m.focused = PanelBodies
	m.bodyCursorIdx = 1
	m, _ = m.deferDetails()
	if m.details.title != "[0] Chart" || m.details.body != "last chart report" || !m.details.pending {
		t.Fatalf("focus change did not retain the last complete frame: %+v", m.details)
	}
	if got := renderDetailsPanel(64, 18, (detailsFrameOf(m.details)).state.attached, Config{Accessible: true}, detailsFrameOf(m.details)); got != lastFrame {
		t.Fatalf("focus change visibly repainted Details:\nBEFORE:\n%s\nAFTER:\n%s", lastFrame, got)
	}
	// A stale completion from the canceled Chart request must not replace the
	// retained frame.
	m, _ = m.onRefreshResult(refreshResultMsg{
		surface: surfaceDetails, runID: chartRun.runningRunID, wantID: chartRun.wantID,
		payload: detailsPayload{title: "stale", body: "stale chart"},
	})
	if m.details.title != "[0] Chart" || m.details.body != "last chart report" {
		t.Fatalf("stale chart report painted over retained body context: %+v", m.details)
	}

	// Only the latest trailing tick starts the final body report.
	latestTimer := m.refresh[surfaceDetails].timerID
	m, cmd := m.onRefreshTick(refreshTickMsg{surface: surfaceDetails, timerID: latestTimer})
	if cmd == nil {
		t.Fatal("settled Details tick did not start the final report")
	}
	bodyRun := m.refresh[surfaceDetails]
	if bodyRun.runningRunID == chartRun.runningRunID {
		t.Fatal("focus change did not replace the Details run")
	}
	m, _ = m.onRefreshResult(refreshResultMsg{
		surface: surfaceDetails, runID: bodyRun.runningRunID, wantID: bodyRun.wantID,
		payload: detailsPayload{title: "[0] Moon", body: "latest body"},
	})
	if m.details.body != "latest body" {
		t.Fatalf("latest Details result did not commit: %+v", m.details)
	}

	m, _ = m.requestDetails()
	wideRun := m.refresh[surfaceDetails]
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)
	if m.refresh[surfaceDetails].cancel != nil || !reflect.DeepEqual(m.details, detailsViewState{}) {
		t.Fatalf("narrow resize did not cancel and clear Details: state=%+v", m.details)
	}
	m, _ = m.onRefreshResult(refreshResultMsg{
		surface: surfaceDetails, runID: wideRun.runningRunID, wantID: wideRun.wantID,
		payload: detailsPayload{title: "stale", body: "stale wide report"},
	})
	if !reflect.DeepEqual(m.details, detailsViewState{}) {
		t.Fatalf("pre-resize report painted while hidden: %+v", m.details)
	}
}

func TestDetailsHeldNavigationStartsOnlyFinalReport(t *testing.T) {
	m := detailsTestModel(t)
	m.focused = PanelBodies
	m.bodyCursorIdx = 0
	m.details = detailsViewState{title: "[0] Sun", body: "last complete raw report"}
	lastFrame := renderDetailsPanel(64, 18, (detailsFrameOf(m.details)).state.attached, Config{Accessible: true}, detailsFrameOf(m.details))

	m, firstTimer := m.deferDetails()
	if firstTimer == nil || m.refresh[surfaceDetails].cancel != nil {
		t.Fatal("first navigation event must arm a timer without starting a report")
	}
	firstID := m.refresh[surfaceDetails].timerID
	retained := m.details
	if got := renderDetailsPanel(64, 18, (detailsFrameOf(m.details)).state.attached, Config{Accessible: true}, detailsFrameOf(m.details)); got != lastFrame {
		t.Fatalf("first navigation event visibly changed Details:\nBEFORE:\n%s\nAFTER:\n%s", lastFrame, got)
	}

	// Simulate repeated cursor movement before the idle window expires.
	m.bodyCursorIdx = 1
	m, repeatedCmd := m.deferDetails()
	if repeatedCmd != nil || m.refresh[surfaceDetails].cancel != nil {
		t.Fatal("repeated navigation must reuse the one armed timer without starting work")
	}
	if !reflect.DeepEqual(m.details, retained) {
		t.Fatalf("held navigation changed retained Details state: before=%+v after=%+v", retained, m.details)
	}
	if got := renderDetailsPanel(64, 18, (detailsFrameOf(m.details)).state.attached, Config{Accessible: true}, detailsFrameOf(m.details)); got != lastFrame {
		t.Fatalf("held navigation visibly changed Details:\nBEFORE:\n%s\nAFTER:\n%s", lastFrame, got)
	}
	if m.refresh[surfaceDetails].timerID != firstID {
		t.Fatal("repeated navigation armed a second timer instead of reusing the first")
	}

	// The sole timer observes intervening input and rearms one checkpoint; it
	// still starts no report.
	var cmd tea.Cmd
	m, cmd = m.onRefreshTick(refreshTickMsg{surface: surfaceDetails, timerID: firstID})
	if cmd == nil || m.refresh[surfaceDetails].cancel != nil {
		t.Fatal("active Details timer did not rearm cleanly after repeated input")
	}
	m, cmd = m.onRefreshTick(refreshTickMsg{surface: surfaceDetails, timerID: m.refresh[surfaceDetails].timerID})
	if cmd == nil || m.refresh[surfaceDetails].cancel == nil {
		t.Fatal("final Details timer did not start exactly one report")
	}
}

func TestDetailsWaitsForFinalTextCommit(t *testing.T) {
	m := detailsTestModel(t)
	m.focused = PanelChart
	m.details = detailsViewState{title: "[0] Chart", body: "old raw"}
	m = m.waitDetailsForText()
	m.refresh[surfaceText].wantID = 2
	detailsRunID := m.refresh[surfaceDetails].runningRunID

	// The leading/intermediate chart frame may paint, but must not spawn a raw
	// report that would immediately be canceled by the settled frame.
	m, _ = applyTextCommit(m, FirstFrameReadyMsg{WantID: 1, TransitTime: testFrozenTime()})
	if m.refresh[surfaceDetails].runningRunID != detailsRunID || m.refresh[surfaceDetails].cancel != nil {
		t.Fatal("intermediate text commit started a Details report")
	}
	if !m.details.pending || !m.details.waitForText {
		t.Fatalf("intermediate text commit lost pending state: %+v", m.details)
	}
	if m.details.title != "[0] Chart" || m.details.body != "old raw" {
		t.Fatalf("intermediate text commit changed the last complete frame: %+v", m.details)
	}

	// Only the current/final chart frame starts the one matching raw report.
	m, cmd := applyTextCommit(m, FirstFrameReadyMsg{WantID: 2, TransitTime: testFrozenTime().Add(time.Hour)})
	if cmd == nil || m.refresh[surfaceDetails].cancel == nil {
		t.Fatal("final text commit did not start the settled Details report")
	}
}

func TestDetailsFailureRetainsLastCompleteFrame(t *testing.T) {
	m := detailsTestModel(t)
	m.details = detailsViewState{
		title:       "[0] Moon",
		body:        "last complete raw report",
		pending:     true,
		waitForText: true,
		timerArmed:  true,
	}
	want := renderDetailsPanel(64, 18, (detailsFrameOf(m.details)).state.attached, Config{Accessible: true}, detailsFrameOf(m.details))

	m, _ = m.commitRefresh(surfaceDetails, refreshResultMsg{
		surface: surfaceDetails,
		payload: detailsPayload{title: "[0] Mars"},
		err:     errors.New("report failed"),
	})

	if m.details.pending || m.details.waitForText || m.details.timerArmed {
		t.Fatalf("failed replacement left Details pending: %+v", m.details)
	}
	if got := renderDetailsPanel(64, 18, (detailsFrameOf(m.details)).state.attached, Config{Accessible: true}, detailsFrameOf(m.details)); got != want {
		t.Fatalf("failed replacement visibly changed Details:\nBEFORE:\n%s\nAFTER:\n%s", want, got)
	}
}

func TestRenderDetailsPanelStartsAtReportTop(t *testing.T) {
	got := renderDetailsPanel(30, 5, (detailsFrameOf(detailsViewState{
		title: "[0] Chart",
		body:  "first\nsecond\nthird\nfourth\nfifth",
	})).state.attached, Config{Accessible: true}, detailsFrameOf(detailsViewState{
		title: "[0] Chart",
		body:  "first\nsecond\nthird\nfourth\nfifth",
	}))
	if !strings.Contains(got, "first") || !strings.Contains(got, "third") {
		t.Fatalf("Details did not retain first visible report lines:\n%s", got)
	}
	if strings.Contains(got, "fourth") || strings.Contains(got, "fifth") {
		t.Fatalf("Details autoscrolled to report tail:\n%s", got)
	}
}

// detailsFrameOf renders a bare view state the way the Model would: the
// strip derived from that state, no timeline rows.
func detailsFrameOf(state detailsViewState) detailsFrame {
	// A workspace with the cursor on the floating slot: the Timeline tab is
	// offered for that cursor, which these strips are drawn for.
	slots, err := store.NewSlots(tlFloatingTime, testPlaceAmsterdam())
	if err != nil {
		panic(err)
	}
	m := Model{details: state, focused: PanelChart, slots: slots}
	return detailsFrame{state: state, strip: m.tabStrips()[PanelDetails]}
}

// the specification rule 6: a slot whose zone does not load gets one
// Log error on the report lane and no Astrolog run — the real report path is
// used, not a stub, so the refusal is the compiler's and lookup's own, and no
// command (argv line) is carried.
func TestReportRefusesAnUnknownZone(t *testing.T) {
	m := detailsTestModel(t)
	m.focused = PanelChart
	m.focusedSlot().Location.TZ = "Mars/Olympus_Mons"
	msg := runDetailsCmd(t, m)
	if msg.err == nil {
		t.Fatal("a report for an unloadable zone succeeded; it must fail before Astrolog runs")
	}
	if p, _ := msg.payload.(detailsPayload); len(p.commands) != 0 {
		t.Fatalf("Astrolog ran (%v) for an unloadable zone", p.commands)
	}
	before := len(m.logLines)
	m, _ = m.commitRefresh(surfaceDetails, msg)
	got := m.logLines[before:]
	if len(got) != 1 || got[0].lane != laneError || !strings.HasPrefix(got[0].text, "Couldn't read the full report.") {
		t.Errorf("Log gained %+v, want exactly one report error", got)
	}
}
