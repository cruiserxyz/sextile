package ui

import (
	"context"
	"errors"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// C8 — the wheel highlight derives from the focused slot's selection only,
// and the strongest tier only from the Bodies cursor while Bodies is focused.
func TestWheelHighlightUsesFocusedSlotSelection(t *testing.T) {
	m := rowSelectionModel(t)
	m.slots.ToggleBodySelection(1, aspect.Mars)    // focused (floating) slot
	m.slots.ToggleBodySelection(0, aspect.Jupiter) // the other slot

	m.cursorIdx = 1
	m.focused = PanelBodies
	m.bodyCursorIdx = 1 // Moon (planets keep position order: Sun, Moon, …)

	hl := m.wheelHighlight()
	if len(hl.Selected) != 1 || hl.Selected[0] != aspect.Mars {
		t.Errorf("Selected = %v, want [Mars] (focused slot only)", hl.Selected)
	}
	if hl.Focused != aspect.Moon {
		t.Errorf("Focused = %v, want Moon (Bodies cursor)", hl.Focused)
	}

	// Moving the slot cursor swaps in the other slot's selection.
	m.cursorIdx = 0
	if hl := m.wheelHighlight(); len(hl.Selected) != 1 || hl.Selected[0] != aspect.Jupiter {
		t.Errorf("Selected after slot switch = %v, want [Jupiter]", hl.Selected)
	}

	// With Bodies unfocused the cursor body carries no strongest tier.
	m.focused = PanelChart
	if hl := m.wheelHighlight(); hl.Focused != aspect.BodyUnknown {
		t.Errorf("Focused with Bodies unfocused = %v, want BodyUnknown", hl.Focused)
	}
}

// C9 — while a pair tab is active the image surface requests the overlay
// render, which carries no highlight state at all, regardless of selection; on
// the chart's own tab the wheel is the cursor's chart alone and keeps its
// highlight (S4a A16).
func TestComparisonWheelEmitsNoHighlight(t *testing.T) {
	m := rowSelectionModel(t)
	wheelCalls := stubRenderWheelImage(t)
	overlayCalls := stubRenderOverlayWheelImage(t)
	m.slots.SetComparisonSelected(true)
	m.slots.ToggleBodySelection(1, aspect.Mars)
	m.focused = PanelBodies
	m.previewActive = true

	// The chart's own tab: the single wheel, with the highlight.
	m.buildSurfaceCmd(surfaceImage, context.Background(), 1, 1)()
	if len(*wheelCalls) != 1 || len(*overlayCalls) != 0 {
		t.Fatalf("own tab: want the single wheel (single=1 overlay=0), got single=%d overlay=%d", len(*wheelCalls), len(*overlayCalls))
	}
	*wheelCalls = nil
	m.base = m.slots.All()[0] // now read in ①

	cmd := m.buildSurfaceCmd(surfaceImage, context.Background(), 1, 1)
	if cmd == nil {
		t.Fatal("comparison image surface returned no Cmd")
	}
	cmd()
	if len(*overlayCalls) != 1 || len(*wheelCalls) != 0 {
		t.Errorf("want the highlight-free overlay render (overlay=1 single=0), got overlay=%d single=%d",
			len(*overlayCalls), len(*wheelCalls))
	}
}

// C10 — toggling a body selection while the preview is active requests an
// image refresh.
func TestBodyToggleRequestsImageRefresh(t *testing.T) {
	m := rowSelectionModel(t)
	m.focused = PanelBodies
	m.previewActive = true

	got, cmd := m.toggleFocusedSelection()
	if !got.slots.BodySelected(got.cursorIdx, aspect.Sun) {
		t.Fatal("Space on the Sun row must select it")
	}
	if imageState(got).wantID != 1 || cmd == nil {
		t.Errorf("body toggle must request an image refresh (wantID=%d, cmd nil=%v)",
			imageState(got).wantID, cmd == nil)
	}
}

// C11 — moving the Bodies cursor while the preview is active requests an
// image refresh (the strongest tier follows the cursor).
func TestBodyCursorMoveRequestsImageRefresh(t *testing.T) {
	m := rowSelectionModel(t)
	m.focused = PanelBodies
	m.previewActive = true

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	got := updated.(Model)
	if got.bodyCursorIdx != 1 {
		t.Fatalf("body cursor = %d, want 1", got.bodyCursorIdx)
	}
	if imageState(got).wantID != 1 {
		t.Errorf("cursor move must request an image refresh (wantID=%d)", imageState(got).wantID)
	}
}

// C12 — focusing into or out of the Bodies panel while the preview is active
// requests an image refresh; a change not crossing Bodies does not.
func TestBodiesFocusChangeRequestsImageRefresh(t *testing.T) {
	m := rowSelectionModel(t)
	// Single: the cursor's chart alone. A comparing wheel (C6/C7) draws no
	// highlight tiers, so a focus change there asks for the picture already
	// asked for (C9) — the Bodies-focus tier is the single render's.
	m.slots.SetComparisonSelected(false)
	m.focused = PanelChart
	m.previewActive = true

	got, _ := m.setFocusedPanel(PanelBodies)
	if imageState(got).wantID != 1 {
		t.Errorf("focus into Bodies must refresh (wantID=%d)", imageState(got).wantID)
	}
	got, _ = got.setFocusedPanel(PanelAspects)
	if imageState(got).wantID != 2 {
		t.Errorf("focus out of Bodies must refresh (wantID=%d)", imageState(got).wantID)
	}
	got, _ = got.setFocusedPanel(PanelDetails)
	if imageState(got).wantID != 2 {
		t.Errorf("a change not crossing Bodies must not refresh (wantID=%d)", imageState(got).wantID)
	}
}

// F1 — no focused slot, no render: the image surface stays silent even with
// the preview active.
func TestNilFocusedSlotSkipsHighlightRender(t *testing.T) {
	m := NewModelWithClock(Config{}, FrozenClock(testFrozenTime()))
	m.previewActive = true
	if cmd := m.buildSurfaceCmd(surfaceImage, context.Background(), 1, 1); cmd != nil {
		t.Error("image surface must return nil with no focused slot")
	}
}

// F5 — an out-of-range Bodies cursor yields no strongest body rather than
// indexing past the position list.
func TestOutOfRangeBodyCursorHasNoStrongest(t *testing.T) {
	m := rowSelectionModel(t)
	m.focused = PanelBodies
	m.bodyCursorIdx = 99
	if hl := m.wheelHighlight(); hl.Focused != aspect.BodyUnknown {
		t.Errorf("Focused = %v, want BodyUnknown for an out-of-range cursor", hl.Focused)
	}
}

// M13 — when the three-layer render succeeds, the model publishes exactly the
// bytes renderWheelImage returned (the composite), never a single layer: the
// composite happens below the seam, and the coordinator hands its result to
// the publisher unchanged.
func TestHighlightPublishesComposite(t *testing.T) {
	published := []byte{}
	prev := showWheel
	showWheel = func(b []byte) (string, error) {
		published = append([]byte{}, b...)
		return "/tmp/stub.png", nil
	}
	t.Cleanup(func() { showWheel = prev })
	prevRender := renderWheelImage
	renderWheelImage = func(_ context.Context, _ time.Time, _ location.PlaceInfo, _ lookup.Lens, _ lookup.WheelHighlight) ([]byte, error) {
		return []byte("COMPOSITE"), nil
	}
	t.Cleanup(func() { renderWheelImage = prevRender })

	m := rowSelectionModel(t)
	m.slots.SetComparisonSelected(false) // single-wheel path, not the overlay
	m.slots.ToggleBodySelection(1, aspect.Mars)
	m.previewActive = true
	m, _ = m.requestImageNow()
	st := imageState(m)
	cmd := m.buildSurfaceCmd(surfaceImage, context.Background(), st.runningRunID, st.runningWantID)
	if cmd == nil {
		t.Fatal("no render Cmd")
	}
	msg := cmd().(refreshResultMsg)
	m, pubCmd := m.onRefreshResult(msg)
	if pubCmd == nil {
		t.Fatal("commit must start a publish")
	}
	m, _ = m.onWheelPublished(pubCmd().(wheelPublishedMsg))
	if string(published) != "COMPOSITE" {
		t.Errorf("published %q, want the composite bytes", published)
	}
}

// M14 — a layer failure inside the composite render surfaces as one render
// error: nothing publishes and previewActive clears exactly as a single-layer
// failure does.
func TestLayerFailureClearsPreviewActive(t *testing.T) {
	calls := stubShowWheel(t)
	m := rowSelectionModel(t)
	m.slots.ToggleBodySelection(1, aspect.Mars)
	m.previewActive = true
	m, _ = m.requestImageNow()
	st := imageState(m)
	got, cmd := m.onRefreshResult(refreshResultMsg{
		surface: surfaceImage, runID: st.runningRunID, wantID: st.wantID,
		err: errors.New("astrolog: RenderImage: furniture layer failed"),
	})
	if got.previewActive {
		t.Error("a layer failure must clear previewActive")
	}
	if *calls != 0 || cmd != nil {
		t.Error("a layer failure must publish nothing")
	}
}

// M15 — a cancellation between layers surfaces as context.Canceled and the
// frame drops silently: no publish, no error surfacing, preview stays open.
func TestCancelBetweenLayersDropsFrame(t *testing.T) {
	calls := stubShowWheel(t)
	m := rowSelectionModel(t)
	m.slots.ToggleBodySelection(1, aspect.Mars)
	m.previewActive = true
	m, _ = m.requestImageNow()
	st := imageState(m)
	got, cmd := m.onRefreshResult(refreshResultMsg{
		surface: surfaceImage, runID: st.runningRunID, wantID: st.wantID,
		err: context.Canceled,
	})
	if !got.previewActive {
		t.Error("a canceled frame must not close the preview")
	}
	if *calls != 0 || cmd != nil {
		t.Error("a canceled frame must not publish or chain")
	}
}
