package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/internal/store"
	"github.com/cruiserxyz/sextile/internal/ui/panel"
)

// What the wheel draws, derived from the cursor and the selected set
// (the specification C6-C9). Nothing here is stored but the key
// of the last render asked for. The first-cast chart (chart 1 of the bi-wheel)
// is the active pair's left chart, the base, which is the scene's frame;
// Astrolog draws it on the outer ring with its houses and the cursor's chart
// inside.

// wheelMode is which render the wheel takes.
type wheelMode int

const (
	wheelSingle wheelMode = iota // the cursor's chart alone (C8)
	wheelPair                    // today's -r0 bi-wheel: the base passed first, drawn on the outer ring; the cursor's chart inside (C6)
	wheelRings                   // the selected set as 3 to 6 rings (C7): the compiler keeps it, the UI no longer asks
)

// wheelPlan is one wheel render's inputs, read from the model.
type wheelPlan struct {
	mode wheelMode
	// rings are the drawn charts in numeral order (pair and rings only).
	rings []lookup.WheelRing
	// labels name each ring for a failure line: its numeral and chart name.
	labels []string
	// inner is the bi-wheel's first chart (pair only): the pair's left chart,
	// the frame, which Astrolog draws on the outer ring with its houses.
	inner store.AnchorChart
	// when and place are the single render's chart; highlight its body tiers.
	when      time.Time
	place     location.PlaceInfo
	record    string
	highlight lookup.WheelHighlight
	// floating says which of the plan's moments is the floating slot's: the
	// single render's when, and each ring by index. They are the only moments a
	// held time step moves (stillKey).
	floating     bool
	ringFloating []bool
	// label names the single render's chart as the Log does (numeral and name).
	label string
}

// slotLabel names a selected slot the way a failure line must: its numeral and
// its chart's name, or "now" for the floating slot.
func slotLabel(n int, slot *store.Slot, cfg Config) string {
	name := slot.Chart.Name
	switch {
	case slot.Floating:
		name = "now"
	case name == "":
		name = "chart"
	}
	return panel.NumeralForColour(n, cfg) + " " + name
}

// wheelPlan reads what the wheel would draw now. ok is false before a slot
// exists.
func (m Model) wheelPlan() (wheelPlan, bool) { return m.wheelPlanFor(m.deriveScene()) }

// wheelPlanFor is the plan for a given scene, so a test can hand it one whose
// frame is another slot.
func (m Model) wheelPlanFor(sc scene) (wheelPlan, bool) {
	slot := sc.cursor
	if slot == nil {
		return wheelPlan{}, false
	}
	// The wheel draws exactly the active tab (S4a D2): a pair is a bi-wheel,
	// the pair's left chart (the frame) passed first, which Astrolog draws on
	// the outer ring with its houses, and the cursor's chart inside; the chart's own
	// tab draws that chart alone, with its highlight.
	if sc.mode == sceneComparing && sc.pair != nil {
		return m.pairWheelPlan(sc, sc.pair.left, sc.pair.right), true
	}
	when := slot.Time
	if slot.Floating {
		when = m.floatingImageTime()
	}
	return wheelPlan{mode: wheelSingle, when: when, place: slot.Location, record: slot.Chart.Record, highlight: m.wheelHighlight(), floating: slot.Floating, label: slotLabel(sc.numeral(slot), slot, m.cfg)}, true
}

// pairWheelPlan is the bi-wheel of two slots: the frame first (Astrolog's
// outer ring, with its houses), the cursor's chart second (inside).
func (m Model) pairWheelPlan(sc scene, inner, outer *store.Slot) wheelPlan {
	p := wheelPlan{mode: wheelPair, inner: anchorOf(inner)}
	for _, s := range []*store.Slot{inner, outer} {
		when := s.Time
		if s.Floating {
			when = m.floatingImageTime()
		}
		p.rings = append(p.rings, lookup.WheelRing{When: when, Place: s.Location, Record: s.Chart.Record})
		p.ringFloating = append(p.ringFloating, s.Floating)
		p.labels = append(p.labels, slotLabel(sc.numeral(s), s, m.cfg))
	}
	return p
}

// shows is what the Log says the wheel draws: a pair as the chart read in the
// base (`① Kepler in ② Goethe`, the cursor's chart first), a single chart by
// its own label, a set of rings in numeral order.
func (p wheelPlan) shows() string {
	switch {
	case p.mode == wheelSingle:
		return p.label
	case p.mode == wheelPair && len(p.labels) == 2:
		return p.labels[1] + " in " + p.labels[0]
	}
	return strings.Join(p.labels, ", ")
}

// wheelLens is the lens plan p is drawn under: the single chart's own wheel
// shows only the focused slot's selected bodies in its own lens (showLens); a
// pair or a ring set is a comparison, ignores body selection and is drawn in
// the frame's lens (comparisonLens).
func (m Model) wheelLens(p wheelPlan) lookup.Lens {
	if p.mode == wheelSingle {
		return m.showLens(m.focusedSlot())
	}
	return m.comparisonLens()
}

// key is the render's inputs in comparable form (C9): the mode, the ordered ring
// moments and places, the lens, and — for a single chart, which is the only
// render that takes them — the highlight tiers. Two equal keys draw the same
// picture, so the second is not asked for.
func (p wheelPlan) key(lens lookup.Lens) string {
	if p.mode == wheelSingle {
		return fmt.Sprintf("%d|%d|%+v|%+v|%+v", p.mode, p.when.UnixNano(), p.place, lens, p.highlight)
	}
	return fmt.Sprintf("%d|%+v|%+v", p.mode, p.rings, lens)
}

// stillKey is key with the floating slot's moment left out: two plans with
// equal stillKeys differ in the floating time alone, which is all a held time
// step changes. A render for one is an intermediate frame of the animation;
// a render for any other plan is a picture of something the screen has left
// (S4b B4, B5).
func (p wheelPlan) stillKey(lens lookup.Lens) string {
	if p.floating {
		p.when = time.Time{}
	}
	if len(p.ringFloating) == len(p.rings) {
		rings := append([]lookup.WheelRing(nil), p.rings...)
		for i, f := range p.ringFloating {
			if f {
				rings[i].When = time.Time{}
			}
		}
		p.rings = rings
	}
	return p.key(lens)
}

// rememberWheel records the render being asked for, so refreshWheelIfActive can
// tell a request for the same picture from a request for a new one.
func (m Model) rememberWheel() Model {
	if p, ok := m.wheelPlan(); ok {
		m.wheelKey, m.wheelKeyed = p.key(m.wheelLens(p)), true
	}
	return m
}

// wheelUnchanged reports whether the wheel last asked for is the one it would
// be asked for now.
func (m Model) wheelUnchanged() bool {
	if !m.wheelKeyed {
		return false
	}
	p, ok := m.wheelPlan()
	return ok && p.key(m.wheelLens(p)) == m.wheelKey
}

// renderMultiWheelCmd renders the selected set as rings (astrolog -r3..-r6),
// tagged with the image surface's runID/wantID; labels name each ring for a
// failure line. Same coordinator contract as renderWheelCmd.
func renderMultiWheelCmd(ctx context.Context, runID, wantID uint64, render func(context.Context, []lookup.WheelRing, lookup.Lens) ([]byte, []string, error), rings []lookup.WheelRing, labels []string, lens lookup.Lens, shows string) tea.Cmd {
	return func() tea.Msg {
		img, commands, err := render(ctx, rings, lens)
		return refreshResultMsg{
			surface: surfaceImage,
			runID:   runID,
			wantID:  wantID,
			payload: imagePayload{bytes: img, rings: labels, commands: commands, shows: shows},
			err:     err,
		}
	}
}

// buildImageCmd is the Image row's build (specs/refresh-descriptor-image.md):
// the render of the wheel the screen would ask for now, nil when there is none.
func buildImageCmd(m Model, ctx context.Context, runID, wantID uint64) tea.Cmd {
	// Gate the render-START on previewActive (not just the request
	// triggers): this makes it impossible for a queued debounce tick or a
	// stale-want follow-up to revive rendering after a render error cleared
	// previewActive. previewActive is set true by Ctrl+X
	// (dispatchRenderWheel) before its immediate request, and stays true
	// across auto-refreshes until a render error turns it off.
	if !m.previewActive {
		return nil
	}
	plan, ok := m.wheelPlan()
	if !ok {
		return nil
	}
	if m.unresolvedFor(m.focusedSlot()) != nil {
		return nil
	}
	lens := m.wheelLens(plan)
	wheel := m.rows[surfaceImage].spawn.wheel
	switch plan.mode {
	case wheelPair:
		// The active tab's bi-wheel: the pair's left chart, the base, is the
		// first ring, which Astrolog draws on the outer ring with its houses;
		// the cursor's chart (the floating slot at the operator's target, or an
		// anchored slot at its own moment) is the second, inside. Two rings
		// compile to the -r0 bi-wheel and stage both chart records, so a chart
		// opened from a file, which has no IANA zone, draws like any other.
		return renderMultiWheelCmd(ctx, runID, wantID, wheel.renderRings, plan.rings, plan.labels, lens, plan.shows())
	case wheelRings:
		return renderMultiWheelCmd(ctx, runID, wantID, wheel.renderRings, plan.rings, plan.labels, lens, plan.shows())
	}
	return renderWheelCmd(ctx, runID, wantID, wheel.render, store.AnchorChart{When: plan.when, Place: plan.place, Record: plan.record}, lens, plan.highlight, plan.shows())
}

// stampWheelRunKey is the Image row's started column: the key of the wheel the
// run was asked for, which the accept and the publish compare with the screen.
func stampWheelRunKey(m Model) Model {
	m.wheelRunKey, _ = m.wheelStillKey()
	return m
}

// acceptImageFrame is the Image row's accept. A stale ERROR is dropped (a
// mid-burst render failure must not close a working preview when a newer
// target already exists), and so is a stale success whose plan differs from
// the screen's in more than the floating time: a picture of another chart, tab
// or lens is the cursor's scan leaving pictures behind. Anything else lands.
func acceptImageFrame(m Model, msg refreshResultMsg) bool {
	if msg.wantID == m.refresh[surfaceImage].wantID {
		return true
	}
	if msg.err != nil {
		return false
	}
	now, ok := m.wheelStillKey()
	return ok && now == m.wheelRunKey
}

// commitImage is the Image row's commit: the ONLY place that queues bytes for
// preview.Show. A render error clears previewActive so a failed render stops
// the auto-refresh. A stale success that landed (a frame of a held time step)
// chains the run for the newest want, batched with its publish.
func commitImage(m Model, msg refreshResultMsg) (Model, tea.Cmd) {
	stale := msg.wantID != m.refresh[surfaceImage].wantID
	m, cmd := commitImageResult(m, msg)
	if stale && m.previewActive {
		var follow tea.Cmd
		m, follow = m.startRun(surfaceImage)
		return m, tea.Batch(cmd, follow)
	}
	return m, cmd
}

func commitImageResult(m Model, msg refreshResultMsg) (Model, tea.Cmd) {
	// Route on surface (always set), not payload type: an error result may
	// carry a nil payload, and the error handling must not depend on it.
	if p, ok := msg.payload.(imagePayload); ok {
		// A ring whose chart file could not be written failed before any
		// spawn: no command of this result is the one that failed.
		var failed []int
		if _, staged := lookup.StagedRing(msg.err); !staged {
			failed = failedLast(msg.err, p.commands)
		}
		m = m.logCommandsFor(surfaceImage, msg.wantID, p.commands, failed...)
	}
	if msg.err != nil {
		m.previewActive = false
		m.wheelPublishPending = nil
		// Error lane: the render the operator asked for failed.
		m = m.noteEphemeris(msg.err)
		reason := m.failureReason(msg.err, wheelRenderFallback)
		if ring, staged := lookup.StagedRing(msg.err); staged {
			// A ring's chart file could not be written: nothing was drawn,
			// the picture on screen is still the last one, and the line
			// names the slot (C16).
			if p, _ := msg.payload.(imagePayload); ring >= 1 && ring <= len(p.rings) {
				reason = "the chart file for " + p.rings[ring-1] + " could not be written"
			}
		}
		return m.logError("wheel", "wheel not drawn, %s", reason).logWarnings(msg.err), nil
	}
	// Symmetric to the build's render-start gate: drop a SUCCESSFUL render
	// that was already in flight when the preview was disabled (a render or
	// publish error cleared previewActive). Without this, an in-flight render
	// committing after a publish failure would queue a new publish and revive
	// the broken preview.
	if !m.previewActive {
		return m, nil
	}
	// Hand the bytes to the serialized off-thread publisher rather than
	// calling preview.Show synchronously in Update: keeps the HTTP publish +
	// browser launch off the render loop while preserving order (one publish
	// at a time) and latest-wins.
	img, _ := msg.payload.(imagePayload)
	m.wheelPublishPending, m.wheelPublishKey, m.wheelPublishShows = img.bytes, m.wheelRunKey, img.shows
	return m.kickWheelPublish()
}
