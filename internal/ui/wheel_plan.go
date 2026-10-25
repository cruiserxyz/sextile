package ui

import (
	"context"
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/internal/store"
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
	highlight lookup.WheelHighlight
	// floating says which of the plan's moments is the floating slot's: the
	// single render's when, and each ring by index. They are the only moments a
	// held time step moves (stillKey).
	floating     bool
	ringFloating []bool
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
	return numeralForColour(n, cfg) + " " + name
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
	return wheelPlan{mode: wheelSingle, when: when, place: slot.Location, highlight: m.wheelHighlight(), floating: slot.Floating}, true
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
		p.rings = append(p.rings, lookup.WheelRing{When: when, Place: s.Location})
		p.ringFloating = append(p.ringFloating, s.Floating)
		p.labels = append(p.labels, slotLabel(sc.numeral(s), s, m.cfg))
	}
	return p
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
func renderMultiWheelCmd(ctx context.Context, runID, wantID uint64, rings []lookup.WheelRing, labels []string, lens lookup.Lens) tea.Cmd {
	return func() tea.Msg {
		img, commands, err := renderMultiWheelImage(ctx, rings, lens)
		return refreshResultMsg{
			surface: surfaceImage,
			runID:   runID,
			wantID:  wantID,
			payload: imagePayload{bytes: img, rings: labels, commands: commands},
			err:     err,
		}
	}
}

var renderMultiWheelImage = lookup.RenderMultiWheelImageCommands
