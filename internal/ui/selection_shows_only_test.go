package ui

import (
	"context"
	"fmt"
	"os/exec"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/internal/recipe"
	"github.com/cruiserxyz/sextile/internal/store"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// selection_shows_only_test.go: selected bodies show only themselves on the
// chart's own wheel, Aspects list and Details report (I-012 part 1). The Bodies
// table stays complete and every comparison surface ignores the selection.
//
// The seams below stand in for Astrolog the way Astrolog behaves: a lens with
// Only set restricts BOTH the positions and the aspect list of the call it
// rides on, so a UI that handed the restricted lens to the wrong call (the
// positions, the pair, the timeline) shows it in the frame, and one that never
// handed it on shows nothing restricted at all.

var selFixtureBodies = []aspect.Body{aspect.Sun, aspect.Moon, aspect.Mercury, aspect.Venus, aspect.Mars, aspect.Jupiter}

var selFixtureAspects = []aspect.Aspect{
	{A: aspect.Sun, B: aspect.Moon, Kind: aspect.Trine, Power: 9},
	{A: aspect.Sun, B: aspect.Mars, Kind: aspect.Square, Power: 8},
	{A: aspect.Moon, B: aspect.Venus, Kind: aspect.Sextile, Power: 7},
	{A: aspect.Mars, B: aspect.Jupiter, Kind: aspect.Opposition, Power: 6},
	{A: aspect.Mercury, B: aspect.Venus, Kind: aspect.Conjunction, Power: 5},
}

// selCalls records the lens every seam was handed.
type selCalls struct {
	mu       sync.Mutex
	natal    []lookup.Lens
	transit  []lookup.Lens
	raw      []lookup.Lens
	wheel    []lookup.Lens
	overlay  []lookup.Lens
	rings    []lookup.Lens
	cross    []lookup.Lens
	pairBody []lookup.Lens
	timeline []lookup.Lens
	probe    []lookup.Lens
}

func (c *selCalls) add(dst *[]lookup.Lens, l lookup.Lens) {
	c.mu.Lock()
	defer c.mu.Unlock()
	*dst = append(*dst, l)
}

func (c *selCalls) snapshot(src *[]lookup.Lens) []lookup.Lens {
	c.mu.Lock()
	defer c.mu.Unlock()
	return slices.Clone(*src)
}

// reset forgets what was recorded so far.
func (c *selCalls) reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.natal, c.transit, c.raw, c.wheel, c.overlay, c.rings = nil, nil, nil, nil, nil, nil
	c.cross, c.pairBody, c.timeline, c.probe = nil, nil, nil, nil
}

// onlyOf is the names in a lens's Only, ascending, or nil.
func onlyOf(l lookup.Lens) []aspect.Body {
	if len(l.Objects.Only) == 0 {
		return nil
	}
	return l.Objects.Only.Members()
}

// visibleIn is the fixture bodies a lens leaves drawn: all of them, or those in Only.
func visibleIn(l lookup.Lens) []aspect.Body {
	if len(l.Objects.Only) == 0 {
		return slices.Clone(selFixtureBodies)
	}
	var out []aspect.Body
	for _, b := range selFixtureBodies {
		if l.Objects.Only.Has(b) {
			out = append(out, b)
		}
	}
	return out
}

func selBodyNames(bs []aspect.Body) string {
	var out []string
	for _, b := range bs {
		out = append(out, bodyName(b))
	}
	return strings.Join(out, " ")
}

// selResult is the lookup result Astrolog would give under the lens.
func selResult(l lookup.Lens) lookup.Result {
	vis := visibleIn(l)
	var r lookup.Result
	for i, b := range vis {
		r.Positions = append(r.Positions, aspect.Position{Body: b, Longitude: float64(20 + 40*i), Speed: 1, House: 1})
	}
	for _, a := range selFixtureAspects {
		if slices.Contains(vis, a.A) && slices.Contains(vis, a.B) {
			r.Aspects = append(r.Aspects, a)
		}
	}
	r.AspectsReport = "ASPECTS " + selBodyNames(vis)
	if len(l.Objects.Only) > 0 {
		r.Commands = []string{"astrolog -R0 " + selBodyNames(vis)}
	} else {
		r.Commands = []string{"astrolog full"}
	}
	return r
}

// stubSelectionSeams puts the lookup seams behind recorders that answer as
// Astrolog does under the lens they are handed.
func stubSelectionSeams(t *testing.T) *selCalls {
	t.Helper()
	c := &selCalls{}
	oldN, oldT, oldO, oldS := firstCallNatalPositions, firstCallTransits, overlayCrossAspects, synastryCrossAspects
	oldRaw, oldCB, oldSB := computeRawReport, computeCrossBodyReport, computeSynastryBodyReport
	oldW, oldOW, oldMW, oldShow := renderWheelImage, renderOverlayWheelImage, renderMultiWheelImage, showWheel
	oldTL, oldProbe := computeTimeline, probeBody
	t.Cleanup(func() {
		firstCallNatalPositions, firstCallTransits, overlayCrossAspects, synastryCrossAspects = oldN, oldT, oldO, oldS
		computeRawReport, computeCrossBodyReport, computeSynastryBodyReport = oldRaw, oldCB, oldSB
		renderWheelImage, renderOverlayWheelImage, renderMultiWheelImage, showWheel = oldW, oldOW, oldMW, oldShow
		computeTimeline, probeBody = oldTL, oldProbe
	})
	firstCallNatalPositions = func(_ context.Context, _ *lookup.Probers, _ store.AnchorChart, l lookup.Lens) (lookup.Result, error) {
		c.add(&c.natal, l)
		return selResult(l), nil
	}
	firstCallTransits = func(_ context.Context, _ *lookup.Probers, _ time.Time, _ location.PlaceInfo, l lookup.Lens) (lookup.Result, error) {
		c.add(&c.transit, l)
		return selResult(l), nil
	}
	cross := func(l lookup.Lens) (lookup.OverlayAspects, error) {
		c.add(&c.cross, l)
		return lookup.OverlayAspects{
			Aspects:  []lookup.OverlayAspect{{Transit: aspect.Sun, Natal: aspect.Moon, Kind: aspect.Trine, Power: 5}, {Transit: aspect.Mars, Natal: aspect.Venus, Kind: aspect.Square, Power: 4}},
			Report:   "CROSS ASPECTS",
			Commands: []string{"astrolog -r0 cross"},
		}, nil
	}
	overlayCrossAspects = func(_ context.Context, l lookup.Lens, _ store.AnchorChart, _ time.Time, _ location.PlaceInfo) (lookup.OverlayAspects, error) {
		return cross(l)
	}
	synastryCrossAspects = func(_ context.Context, l lookup.Lens, _ store.AnchorChart, _ time.Time, _ location.PlaceInfo, _ store.ReportLabel) (lookup.OverlayAspects, error) {
		return cross(l)
	}
	computeRawReport = func(_ context.Context, _ time.Time, _ location.PlaceInfo, l lookup.Lens, req lookup.RawReportRequest) (string, string, error) {
		c.add(&c.raw, l)
		return fmt.Sprintf("REPORT kind=%d body=%s lists %s", int(req.Kind), bodyName(req.Body), selBodyNames(visibleIn(l))), "astrolog report", nil
	}
	computeCrossBodyReport = func(_ context.Context, l lookup.Lens, _ store.AnchorChart, _ time.Time, _ location.PlaceInfo, b aspect.Body) (string, string, error) {
		c.add(&c.pairBody, l)
		return "PAIR REPORT " + bodyName(b), "astrolog -rt report", nil
	}
	computeSynastryBodyReport = func(_ context.Context, l lookup.Lens, _ store.AnchorChart, _ time.Time, _ location.PlaceInfo, _ store.ReportLabel, b aspect.Body) (string, string, error) {
		c.add(&c.pairBody, l)
		return "PAIR REPORT " + bodyName(b), "astrolog -r0 report", nil
	}
	renderWheelImage = func(_ context.Context, _ time.Time, _ location.PlaceInfo, l lookup.Lens, _ lookup.WheelHighlight) ([]byte, error) {
		c.add(&c.wheel, l)
		return []byte("wheel " + selBodyNames(visibleIn(l))), nil
	}
	renderOverlayWheelImage = func(_ context.Context, _ store.AnchorChart, _ time.Time, _ location.PlaceInfo, l lookup.Lens) ([]byte, error) {
		c.add(&c.overlay, l)
		return []byte("overlay"), nil
	}
	renderMultiWheelImage = func(_ context.Context, _ []lookup.WheelRing, l lookup.Lens) ([]byte, []string, error) {
		c.add(&c.rings, l)
		return []byte("rings"), []string{"astrolog rings"}, nil
	}
	showWheel = func([]byte) (string, error) { return "/tmp/stub.png", nil }
	computeTimeline = func(_ context.Context, req lookup.TimelineRequest) (lookup.TimelineResult, error) {
		c.add(&c.timeline, req.Lens)
		return lookup.TimelineResult{}, nil
	}
	probeBody = func(_ context.Context, _ *lookup.Probers, _ time.Time, _ location.PlaceInfo, l lookup.Lens, _ int) (lookup.BodyProbe, error) {
		c.add(&c.probe, l)
		return lookup.BodyProbe{Drawable: true}, nil
	}
	return c
}

// selectionModel is a single chart on screen with a first frame committed: the
// floating slot alone, or one anchored chart above it with the cursor on the
// anchored one. Nothing is selected, Bodies has the keyboard, the wheel
// preview is open and Details is in the layout.
func selectionModel(t *testing.T, anchored bool) Model {
	t.Helper()
	n := 0
	if anchored {
		n = 1
	}
	m := listModel(t, n, 0)
	for i := range m.slots.All() {
		m.slots.SetSlotSelected(i, false)
	}
	m.focused = PanelBodies
	m.previewActive = true
	if !m.detailsVisible() {
		t.Fatalf("Details is not in the layout at %dx%d", m.width, m.height)
	}
	m, cmd := m.bumpIdentity(surfaceText)
	m = pump(t, m, cmd)
	m, cmd = m.requestImage()
	return pump(t, m, cmd)
}

// aspectsPanel is the Aspects list as the panel draws it.
func aspectsPanel(m Model) string {
	return ansi.Strip(renderAspectsBody(m.chart, TabAspects, 60, 12, Config{Accessible: true, Color: ColorNever}))
}

// bodiesPanel is the Bodies table as the panel draws it, unfocused.
func bodiesPanel(m Model) string {
	s, _, _ := renderBodiesBody(m.chart, m.slots, m.cursorIdx, m.cursorIdx, 60, 30, false, Config{Accessible: true, Color: ColorNever})
	return ansi.Strip(s)
}

func pressSpace(t *testing.T, m Model) (Model, tea.Cmd) {
	t.Helper()
	return tpress(t, m, tea.KeyMsg{Type: tea.KeySpace})
}

func aspectPairs(as []aspect.Aspect) string {
	var out []string
	for _, a := range as {
		out = append(out, bodyName(a.A)+"-"+bodyName(a.B))
	}
	return strings.Join(out, ",")
}

// B2: showLens is lensFor but for Only; a slot with nothing selected gets the
// identical lens.
func TestShowLensCarriesTheSelection(t *testing.T) {
	m := selectionModel(t, true)
	slot := m.focusedSlot()

	if got, want := m.showLens(slot), m.lensFor(slot); !reflect.DeepEqual(got, want) {
		t.Fatalf("nothing selected: showLens = %+v, want lensFor %+v", got, want)
	}
	if got := m.showLens(slot).Objects.Only; got != nil {
		t.Errorf("nothing selected: Only = %v, want nil (not an empty set)", got)
	}

	m.slots.ToggleBodySelection(m.cursorIdx, aspect.Moon)
	m.slots.ToggleBodySelection(m.cursorIdx, aspect.Sun)
	got := m.showLens(slot)
	want := m.lensFor(slot)
	want.Objects.Only = recipe.NewBodySet(aspect.Sun, aspect.Moon)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("selected Sun and Moon: showLens = %+v, want lensFor plus Only {Sun, Moon}: %+v", got, want)
	}
	// It is a copy: the model's own lens did not move.
	if len(m.lens.Objects.Only) != 0 || len(m.viewLens().Objects.Only) != 0 {
		t.Error("showLens wrote Only into the model's lens")
	}
	// The selection belongs to its slot: the other slot's lens is untouched.
	if other := m.slots.All()[len(m.slots.All())-1]; !reflect.DeepEqual(m.showLens(other), m.lensFor(other)) {
		t.Error("a body selected on one slot narrowed another slot's lens")
	}
}

// B3: Space on a Bodies row toggles the selection and arms the three refreshes;
// the next frames list only the selected bodies; Space again on each returns
// every frame to the unselected one.
func TestSpaceOnABodyArmsAspectsDetailsAndWheel(t *testing.T) {
	for _, anchored := range []bool{true, false} {
		name := "floating chart"
		if anchored {
			name = "anchored chart"
		}
		t.Run(name, func(t *testing.T) {
			calls := stubSelectionSeams(t)
			m := selectionModel(t, anchored)
			if got := m.chart.aspects; len(got) != len(selFixtureAspects) {
				t.Fatalf("unselected: %d aspects, want %d", len(got), len(selFixtureAspects))
			}
			if !strings.Contains(m.details.body, "lists Sun Moon Mercury Venus Mars Jupiter") {
				t.Fatalf("unselected Details = %q, want the full report", m.details.body)
			}
			base := struct{ aspects, bodies, details, wheelKey string }{aspectsPanel(m), bodiesPanel(m), m.details.body, m.wheelKey}
			baseAspects := slices.Clone(m.chart.aspects)

			// Space on Sun arms the three surfaces: the text frame (Aspects), the
			// Details report, held until that frame commits, and the wheel.
			textWant, imageWant := m.refresh[surfaceText].wantID, m.refresh[surfaceImage].wantID
			m, cmd := pressSpace(t, m)
			if !m.slots.BodySelected(m.cursorIdx, aspect.Sun) {
				t.Fatal("Space did not select Sun")
			}
			if m.refresh[surfaceText].wantID == textWant {
				t.Error("Space armed no Aspects refresh (the text surface's want did not move)")
			}
			if !m.details.pending || !m.details.waitForText {
				t.Errorf("Space armed no Details refresh (pending=%v waitForText=%v)", m.details.pending, m.details.waitForText)
			}
			if m.refresh[surfaceImage].wantID == imageWant {
				t.Error("Space armed no wheel refresh (the image surface's want did not move)")
			}
			if cmd == nil {
				t.Fatal("Space returned no command")
			}
			calls.reset()
			m = pump(t, m, cmd)

			// Sun alone: its aspects need a second body, so the list is empty.
			if got := aspectPairs(m.chart.aspects); got != "" {
				t.Errorf("Sun alone: Aspects = %q, want none", got)
			}

			m, cmd = tpress(t, m, keyDown)
			m = pump(t, m, cmd)
			m, cmd = pressSpace(t, m)
			m = pump(t, m, cmd)
			if !m.slots.BodySelected(m.cursorIdx, aspect.Moon) {
				t.Fatalf("the cursor is on %v, not Moon", m.bodyCursorIdx)
			}

			// Sun and Moon: only the aspect between them, only them in the report,
			// and the wheel asked for them alone.
			if got := aspectPairs(m.chart.aspects); got != "Sun-Moon" {
				t.Errorf("Sun and Moon: Aspects = %q, want Sun-Moon", got)
			}
			if panel := aspectsPanel(m); panel == base.aspects || filledLines(panel) >= filledLines(base.aspects) {
				t.Errorf("the Aspects panel did not shrink:\n%s\nfrom\n%s", panel, base.aspects)
			}
			if !strings.Contains(m.details.body, "lists Sun Moon") || strings.Contains(m.details.body, "Mercury") {
				t.Errorf("Details = %q, want a report of Sun and Moon only", m.details.body)
			}
			wheels := calls.snapshot(&calls.wheel)
			if len(wheels) == 0 || !reflect.DeepEqual(onlyOf(wheels[len(wheels)-1]), []aspect.Body{aspect.Sun, aspect.Moon}) {
				t.Errorf("the wheel was last asked for Only %v, want Sun and Moon (calls: %d)", onlyOf(lastLens(wheels)), len(wheels))
			}
			if m.wheelKey == base.wheelKey {
				t.Error("the wheel key did not change with the selection")
			}

			// Space on Moon, then up to Sun and Space: every frame is the
			// unselected one again.
			m, cmd = pressSpace(t, m)
			m = pump(t, m, cmd)
			m, cmd = tpress(t, m, keyUp)
			m = pump(t, m, cmd)
			m, cmd = pressSpace(t, m)
			m = pump(t, m, cmd)
			if len(m.slots.SelectedBodies(m.cursorIdx)) != 0 {
				t.Fatalf("selection left over: %v", m.slots.SelectedBodies(m.cursorIdx))
			}
			if !reflect.DeepEqual(m.chart.aspects, baseAspects) {
				t.Errorf("after deselecting: Aspects = %q, want the unselected %q", aspectPairs(m.chart.aspects), aspectPairs(baseAspects))
			}
			if got := aspectsPanel(m); got != base.aspects {
				t.Errorf("after deselecting: the Aspects panel differs:\n%s\nwant\n%s", got, base.aspects)
			}
			if got := bodiesPanel(m); got != base.bodies {
				t.Errorf("after deselecting: the Bodies panel differs:\n%s\nwant\n%s", got, base.bodies)
			}
			if m.details.body != base.details {
				t.Errorf("after deselecting: Details = %q, want %q", m.details.body, base.details)
			}
			if m.wheelKey != base.wheelKey {
				t.Errorf("after deselecting: wheel key differs from the unselected one")
			}
			wheels = calls.snapshot(&calls.wheel)
			if got := lastLens(wheels); len(wheels) == 0 || len(got.Objects.Only) != 0 {
				t.Errorf("after deselecting the wheel was last asked for Only %v, want no narrowing", onlyOf(got))
			}
		})
	}
}

// filledLines counts the lines of a panel that hold something.
func filledLines(panel string) int {
	n := 0
	for _, l := range strings.Split(panel, "\n") {
		if strings.TrimSpace(l) != "" {
			n++
		}
	}
	return n
}

func lastLens(ls []lookup.Lens) lookup.Lens {
	if len(ls) == 0 {
		return lookup.Lens{}
	}
	return ls[len(ls)-1]
}

// The still-key of the wheel carries the selection, so a picture rendered for
// another selection is never published as the current one, and an unchanged
// selection asks for no new render.
func TestWheelStillKeyFollowsTheSelection(t *testing.T) {
	stubSelectionSeams(t)
	m := selectionModel(t, false)
	unselected, ok := m.wheelStillKey()
	if !ok {
		t.Fatal("no wheel plan")
	}

	// The key already moves with the selection through the plan's highlight
	// tiers; the lens leg is what survives the highlight staying parked, so it
	// is read alone, with the highlight taken out of the plan.
	lensLeg := func(m Model) string {
		p, _ := m.wheelPlan()
		p.highlight = lookup.WheelHighlight{}
		return p.stillKey(m.wheelLens(p))
	}
	lensBefore := lensLeg(m)

	m.slots.ToggleBodySelection(m.cursorIdx, aspect.Sun)
	if lensLeg(m) == lensBefore {
		t.Error("the wheel's key, without its highlight, did not change with the selection: the lens leg ignores Only")
	}
	selected, _ := m.wheelStillKey()
	if selected == unselected {
		t.Fatal("the still-key did not change with the selection")
	}
	if m.wheelUnchanged() {
		t.Error("a changed selection reads as the wheel already asked for")
	}

	// Bytes rendered before the selection wait behind another publish: they are
	// dropped, not shown.
	m.wheelPublishPending, m.wheelPublishKey = []byte("rendered for nothing selected"), unselected
	m, cmd := m.kickWheelPublish()
	if cmd != nil || m.wheelPublishing {
		t.Error("a picture of the unselected wheel was published after the selection changed")
	}

	// The same selection again asks for nothing.
	m = m.rememberWheel()
	if !m.wheelUnchanged() {
		t.Error("an unchanged selection reads as a changed wheel")
	}
	m.wheelPublishPending, m.wheelPublishKey = []byte("rendered for Sun"), selected
	if _, cmd := m.kickWheelPublish(); cmd == nil {
		t.Error("a picture of the current selection was dropped")
	}
}

// B4: the Bodies table keeps every body under a selection; the rows differ from
// the unselected table only by the mark column.
func TestBodiesTableStaysCompleteUnderSelection(t *testing.T) {
	stubSelectionSeams(t)
	m := selectionModel(t, true)
	before := bodiesPanel(m)
	positions := slices.Clone(m.chart.positions)

	for _, b := range []aspect.Body{aspect.Sun, aspect.Moon} {
		m.slots.ToggleBodySelection(m.cursorIdx, b)
	}
	m, cmd := m.refreshAfterSelectionForTest()
	m = pump(t, m, cmd)

	if !reflect.DeepEqual(m.chart.positions, positions) {
		t.Fatalf("Bodies positions under a selection = %v, want all %d of the unselected frame", m.chart.positions, len(positions))
	}
	after := bodiesPanel(m)
	mark := ansi.StringWidth(rowMarkerBlank)
	beforeRows, afterRows := strings.Split(before, "\n"), strings.Split(after, "\n")
	if len(beforeRows) != len(afterRows) {
		t.Fatalf("the table has %d rows, was %d", len(afterRows), len(beforeRows))
	}
	marked := 0
	for i := range afterRows {
		if len(afterRows[i]) < mark {
			t.Fatalf("row %d is shorter than the mark column: %q", i, afterRows[i])
		}
		if strings.TrimSpace(afterRows[i][:mark]) != "" {
			marked++
		}
		// The mark column is added on the left, so the row loses its last
		// cells to make room; compare what both show of the row.
		got, want := strings.TrimRight(afterRows[i][mark:], " "), strings.TrimRight(beforeRows[i], " ")
		if !strings.HasPrefix(want, got) {
			t.Errorf("row %d differs beyond its mark:\n  selected:   %q\n  unselected: %q", i, afterRows[i], beforeRows[i])
		}
	}
	if marked != 2 {
		t.Errorf("%d rows are marked, want the 2 selected", marked)
	}
}

// refreshAfterSelectionForTest is Space's refresh for a selection a test made
// directly in the store.
func (m Model) refreshAfterSelectionForTest() (Model, tea.Cmd) {
	m = m.waitDetailsForText()
	var textCmd, imageCmd tea.Cmd
	m, textCmd = m.noteWant(surfaceText, false)
	m, imageCmd = m.refreshWheelIfActive()
	return m, tea.Batch(textCmd, imageCmd)
}

// B5: two charts selected for comparison, bodies selected on one: the cross
// aspect list, the pair report and the ring wheels carry the full lens; the
// bi-wheel's argv is the one it had before.
func TestSelectionDoesNotReachComparisonSurfaces(t *testing.T) {
	calls := stubSelectionSeams(t)
	m := selectedSet(t, 1, true, 1)
	m.focused = PanelBodies
	m.previewActive = true
	m.base = m.slots.All()[0] // the pair tab: ② in ①, now read in ①
	if !m.comparing() {
		t.Fatal("the set is not comparing")
	}
	if !m.detailsVisible() {
		t.Fatalf("Details is not in the layout at %dx%d", m.width, m.height)
	}
	m, cmd := m.bumpIdentity(surfaceText)
	m = pump(t, m, cmd)
	m, cmd = m.requestImage()
	m = pump(t, m, cmd)
	crossBefore := slices.Clone(m.chart.crossAspects)
	reportBefore := m.chart.crossAspectsReport
	if len(crossBefore) == 0 || len(calls.snapshot(&calls.overlay)) == 0 {
		t.Fatalf("the comparison drew no cross aspects or no bi-wheel: %d, %d", len(crossBefore), len(calls.snapshot(&calls.overlay)))
	}
	calls.reset()

	for _, b := range []aspect.Body{aspect.Sun, aspect.Moon} {
		m.slots.ToggleBodySelection(m.cursorIdx, b)
	}
	m, cmd = m.refreshAfterSelectionForTest()
	m = pump(t, m, cmd)
	// A Details request for the cursor body, as the pair has it, and a wheel
	// asked for again (the key did not move, so the selection alone asked for none).
	m, cmd = m.startDetailsNow()
	m = pump(t, m, cmd)
	m.refresh[surfaceImage].cancel = nil
	m, cmd = m.requestImageNow()
	m = pump(t, m, cmd)

	if !reflect.DeepEqual(m.chart.crossAspects, crossBefore) || m.chart.crossAspectsReport != reportBefore {
		t.Errorf("the cross aspects moved with the selection: %v", m.chart.crossAspects)
	}
	for name, ls := range map[string][]lookup.Lens{
		"cross aspects": calls.snapshot(&calls.cross),
		"pair report":   calls.snapshot(&calls.pairBody),
		"bi-wheel":      calls.snapshot(&calls.overlay),
	} {
		if len(ls) == 0 {
			t.Errorf("%s: nothing was asked for after the selection", name)
		}
		for _, l := range ls {
			if len(l.Objects.Only) != 0 {
				t.Errorf("%s was asked for under Only %v; a comparison ignores the body selection", name, onlyOf(l))
			}
		}
	}

	// The ring wheel is not reachable from the UI today (a pair is always a
	// bi-wheel), so its plan is built by hand: it is a comparison and takes the
	// screen's lens whatever is selected.
	rings := wheelPlan{mode: wheelRings}
	if got := m.wheelLens(rings); len(got.Objects.Only) != 0 || !reflect.DeepEqual(got, m.viewLens()) {
		t.Errorf("the multi-ring wheel's lens carries Only %v", onlyOf(got))
	}
	pair := wheelPlan{mode: wheelPair}
	if got := m.wheelLens(pair); len(got.Objects.Only) != 0 {
		t.Errorf("the bi-wheel's lens carries Only %v", onlyOf(got))
	}
	// The chart's own wheel still follows the selection, so the narrowing is
	// there to be refused.
	if got := m.wheelLens(wheelPlan{mode: wheelSingle}); len(got.Objects.Only) != 2 {
		t.Errorf("the single wheel's lens has Only %v, want Sun and Moon", onlyOf(got))
	}
}

// B6: a session that selected bodies restores to restricted surfaces: the
// first frame, the wheel and the Details report come up as saved.
func TestRestoredSelectionRestrictsAtStart(t *testing.T) {
	calls := stubSelectionSeams(t)
	m := listModel(t, 0, 0)
	for i := range m.slots.All() {
		m.slots.SetSlotSelected(i, false)
	}
	// What applySessionSelections does with the file's marks.
	selectBodies(m.slots, m.cursorIdx, []aspect.Body{aspect.Sun, aspect.Moon})
	m.focused = PanelBodies
	m.previewActive = true

	// The very first compute is the cold start's bumpIdentity.
	m, cmd := m.bumpIdentity(surfaceText)
	m = pump(t, m, cmd)
	m, cmd = m.requestImage()
	m = pump(t, m, cmd)

	if got := aspectPairs(m.chart.aspects); got != "Sun-Moon" {
		t.Errorf("restored: Aspects = %q, want Sun-Moon", got)
	}
	if len(m.chart.positions) != len(selFixtureBodies) {
		t.Errorf("restored: Bodies has %d rows, want all %d", len(m.chart.positions), len(selFixtureBodies))
	}
	if !strings.Contains(m.details.body, "lists Sun Moon") || strings.Contains(m.details.body, "Mercury") {
		t.Errorf("restored: Details = %q, want a report of Sun and Moon only", m.details.body)
	}
	if got := lastLens(calls.snapshot(&calls.wheel)); !reflect.DeepEqual(onlyOf(got), []aspect.Body{aspect.Sun, aspect.Moon}) {
		t.Errorf("restored: the wheel was asked for Only %v", onlyOf(got))
	}
}

// B7: the timeline and the chooser's probes keep the full lens; so do the
// template tab's inputs.
func TestTimelineAndProbingKeepTheFullLens(t *testing.T) {
	calls := stubSelectionSeams(t)
	m := selectionModel(t, false)
	for _, b := range []aspect.Body{aspect.Sun, aspect.Moon} {
		m.slots.ToggleBodySelection(m.cursorIdx, b)
	}
	if len(m.showLens(m.focusedSlot()).Objects.Only) != 2 {
		t.Fatal("setup: the selection did not reach showLens")
	}
	calls.reset()

	// The timeline's request.
	req, _, ok := m.timelineInputs()
	if !ok {
		t.Fatal("no timeline request for the floating chart")
	}
	if len(req.Lens.Objects.Only) != 0 {
		t.Errorf("the timeline request carries Only %v", onlyOf(req.Lens))
	}
	// And the search the model actually runs.
	m, cmd := m.syncTimeline()
	m = pump(t, m, cmd)
	for _, l := range calls.snapshot(&calls.timeline) {
		if len(l.Objects.Only) != 0 {
			t.Errorf("a timeline search ran under Only %v", onlyOf(l))
		}
	}

	// The body chooser's probes: opened on a selection, the popup probes the
	// body under its cursor in the chart's own frame, with the lens in force.
	m.objectRegistry = probeRegistry()
	opened, probeCmd := openProbePopup(t, m)
	opened = pump(t, opened, probeCmd)
	probes := calls.snapshot(&calls.probe)
	if len(probes) == 0 {
		t.Fatal("the chooser ran no probe, so there is nothing to observe")
	}
	for _, l := range probes {
		if len(l.Objects.Only) != 0 {
			t.Errorf("the chooser probed under Only %v", onlyOf(l))
		}
	}
	// The chooser seeds from the lens in force, not the narrowed one.
	if len(opened.chooserEntry.lens.Objects.Only) != 0 || len(opened.lens.Objects.Only) != 0 {
		t.Error("the chooser's lens carries Only")
	}
}

// The restricted call rides beside the full one: the frame's Bodies come from
// the lens without Only and its Aspects from the lens with it, and both
// commands are on the argv lane. Run against the fixture binary so the argv is
// the compiler's own.
func TestSelectionArgvThroughTheCompiler(t *testing.T) {
	spawns := chartFileCapturingAstrolog(t)
	m := listModel(t, 1, 0)
	for i := range m.slots.All() {
		m.slots.SetSlotSelected(i, false)
	}
	m.focused = PanelBodies
	stubShowWheel(t)
	m.previewActive = true
	m, cmd := m.bumpIdentity(surfaceText)
	m = pumpPatient(t, m, cmd)
	m, cmd = m.requestImage()
	m = pumpPatient(t, m, cmd)
	plain := spawns()
	for _, s := range plain {
		if slices.Contains(s.argv, "-R0") {
			t.Fatalf("an unselected frame spawned with a show-list: %v", s.argv)
		}
	}
	plainCount := len(plain)

	for _, b := range []aspect.Body{aspect.Sun, aspect.Moon} {
		m.slots.ToggleBodySelection(m.cursorIdx, b)
	}
	m, cmd = m.refreshAfterSelectionForTest()
	m = pumpPatient(t, m, cmd)

	// The frame's two Astrolog calls are told apart by their positions
	// invocation (`=v0`); the Details report and the wheel are not counted.
	var narrowed, full, report, wheel int
	for _, s := range spawns()[plainCount:] {
		if !slices.Contains(s.argv, "=v0") {
			// The Details report (`-RO <body>`) and the wheel image (`-XV`/`-Xbp`)
			// are the other two surfaces that ride the narrowed lens.
			if containsRun(s.argv, []string{"-R0", "1", "2"}) {
				switch {
				case slices.Contains(s.argv, "-RO"):
					report++
				case slices.Contains(s.argv, "-XV") || slices.Contains(s.argv, "-Xbp"):
					wheel++
				}
			}
			continue
		}
		switch {
		case containsRun(s.argv, []string{"-R0", "1", "2"}):
			narrowed++
		case !slices.Contains(s.argv, "-R0"):
			full++
		default:
			t.Errorf("a frame call with a show-list that is not Sun and Moon: %v", s.argv)
		}
	}
	if narrowed == 0 {
		t.Error("no frame call carried `-R0 1 2`: the Aspects never asked Astrolog for the selection")
	}
	if full == 0 {
		t.Error("every frame call was narrowed: the Bodies table is computed under the selection")
	}
	if report == 0 {
		t.Error("the Details report did not ask Astrolog for the selection (`-R0 1 2` with `-RO`)")
	}
	if wheel == 0 {
		t.Error("the wheel image did not ask Astrolog for the selection (`-R0 1 2` with `-XV`)")
	}
}

func containsRun(argv, run []string) bool {
	for i := 0; i+len(run) <= len(argv); i++ {
		if slices.Equal(argv[i:i+len(run)], run) {
			return true
		}
	}
	return false
}

// The comparison's own spawns keep their argv under a selection: the bi-wheel
// and the cross list are byte-identical to the ones made with nothing selected.
func TestSelectionLeavesComparisonArgvAlone(t *testing.T) {
	spawns := chartFileCapturingAstrolog(t)
	stubShowWheel(t)
	pairArgv := func(m Model) [][]string {
		var out [][]string
		for _, s := range spawns() {
			if slices.Contains(s.argv, "-r0") {
				out = append(out, normaliseStage(s.argv))
			}
		}
		return out
	}

	m := selectedSet(t, 1, true, 1)
	m.focused = PanelBodies
	m.base = m.slots.All()[0]
	m, cmd := m.bumpIdentity(surfaceText)
	m = pumpPatient(t, m, cmd)
	m = openWheel(t, m)
	before := pairArgv(m)
	if len(before) == 0 {
		t.Fatal("no -r0 spawn before the selection")
	}

	for _, b := range []aspect.Body{aspect.Sun, aspect.Moon} {
		m.slots.ToggleBodySelection(m.cursorIdx, b)
	}
	m, cmd = m.refreshAfterSelectionForTest()
	m = pumpPatient(t, m, cmd)
	m, cmd = m.requestImage()
	m = pumpPatient(t, m, cmd)

	after := pairArgv(m)
	for _, a := range after {
		if slices.Contains(a, "-R0") {
			t.Errorf("a comparison spawn carries a show-list: %v", a)
		}
	}
	// Every distinct pair argv made after the selection is one made before it.
	for _, a := range after {
		if !slices.ContainsFunc(before, func(b []string) bool { return slices.Equal(a, b) }) {
			t.Errorf("a comparison argv changed under the selection:\n  %v", a)
		}
	}
}

// Against the real binary: the numbers are Astrolog's, the restriction is its
// own, and the Aspects list that comes back is a subset of the full one.
func TestSelectionShowsOnlyAgainstLiveAstrolog(t *testing.T) {
	if _, err := exec.LookPath("astrolog"); err != nil {
		t.Skip("astrolog not on PATH; skipping live-binary test")
	}
	m := listModel(t, 0, 0)
	for i := range m.slots.All() {
		m.slots.SetSlotSelected(i, false)
	}
	m.focused = PanelBodies
	m.clock = FrozenClock(time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC))
	m, cmd := m.bumpIdentity(surfaceText)
	m = pumpPatient(t, m, cmd)
	full, fullReport := slices.Clone(m.chart.aspects), m.chart.aspectsReport
	rows := len(m.chart.positions)
	if len(full) < 3 || rows < 10 {
		t.Fatalf("the unselected frame is too thin to test against: %d aspects, %d rows", len(full), rows)
	}

	// Select the two bodies of the strongest aspect of the full list, so the
	// restricted list is known to hold one.
	strongest := full[0]
	for _, a := range full {
		if a.Power > strongest.Power {
			strongest = a
		}
	}
	for _, b := range []aspect.Body{strongest.A, strongest.B} {
		m.slots.ToggleBodySelection(m.cursorIdx, b)
	}
	m, cmd = m.refreshAfterSelectionForTest()
	m = pumpPatient(t, m, cmd)

	if len(m.chart.positions) != rows {
		t.Errorf("Bodies has %d rows under the selection, want all %d", len(m.chart.positions), rows)
	}
	if len(m.chart.aspects) != 1 || m.chart.aspects[0].A != strongest.A || m.chart.aspects[0].B != strongest.B {
		t.Errorf("Aspects = %s, want only %s-%s", aspectPairs(m.chart.aspects), bodyName(strongest.A), bodyName(strongest.B))
	}
	if !strings.Contains(m.chart.aspectsReport, bodyName(strongest.A)) {
		t.Errorf("the Aspects report does not name %s: %q", bodyName(strongest.A), m.chart.aspectsReport)
	}
	if len(m.chart.aspectsReport) >= len(fullReport) {
		t.Errorf("the restricted report (%d bytes) is not shorter than the full one (%d)", len(m.chart.aspectsReport), len(fullReport))
	}

	// One body alone makes no aspect: Astrolog answers "No aspects in list.",
	// which is an empty list and not a failed frame.
	m.slots.ToggleBodySelection(m.cursorIdx, strongest.B)
	m, cmd = m.refreshAfterSelectionForTest()
	m = pumpPatient(t, m, cmd)
	if len(m.chart.aspects) != 0 || !m.chart.settled {
		t.Errorf("one body selected: %d aspects, settled=%v, want an empty settled frame", len(m.chart.aspects), m.chart.settled)
	}
	if len(m.chart.positions) != rows {
		t.Errorf("one body selected: Bodies has %d rows, want all %d", len(m.chart.positions), rows)
	}
}
