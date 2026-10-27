package store

import (
	"sort"

	"errors"
	"time"

	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// Slot is a single workspace slot. v1 carries primitives directly (no
// Chart-record indirection). Floating discriminates the always-present
// quasi-chart slot (cursor's startup target, time-stepping target) from
// anchored slots (chart-bearing).
//
// An anchored slot also carries its own Chart record: which library chart it
// is and what it is called (the specification). Time and Location stay the
// authoritative moment and place — a city re-pick or an edited chart file moves
// them — and Chart.When / Chart.Place are the snapshot taken when the slot was
// added. The floating slot carries a zero Chart.
type Slot struct {
	Time     time.Time
	Location location.PlaceInfo
	Floating bool
	Chart    SessionChart

	selected       bool
	selectedBodies map[aspect.Body]struct{}
}

// MaxSelectedSlots is how many slots may be selected at once (M7).
const MaxSelectedSlots = 6

// Slots is the workspace. The floating slot is always present and always last;
// anchored slots are an ordered list of any length above it. List order is
// meaning: the topmost selected anchored slot is the baseline. Persistent
// slot/body selection lives here; row cursors remain UI state.
//
// The anchored slots are pointers so a *Slot held across an add stays valid.
type Slots struct {
	floating Slot
	anchored []*Slot
}

// NewSlots initializes the workspace with one floating slot at first run.
// Refuses if place lacks an IANA timezone — astrolog rejects untimed places.
func NewSlots(when time.Time, place location.PlaceInfo) (*Slots, error) {
	if !place.HasTimezone() {
		return nil, errors.New("store: NewSlots: place.TZ required")
	}
	return &Slots{
		floating: Slot{
			Time:     when,
			Location: place,
			Floating: true,
		},
	}, nil
}

// AddAnchored appends an anchored slot directly above the floating slot.
// Refuses, leaving the list unchanged, if place lacks an IANA timezone.
//
// The new slot arrives unselected and no selection changes: selection is only
// ever made by the user.
func (s *Slots) AddAnchored(chart SessionChart, when time.Time, place location.PlaceInfo) error {
	if !place.HasTimezone() {
		return errors.New("store: Slots.AddAnchored: place.TZ required")
	}
	chart.When = when
	chart.Place = place
	slot := &Slot{Time: when, Location: place, Chart: chart}
	s.anchored = append(s.anchored, slot)
	return nil
}

// RemoveAnchored takes the anchored slot at list index i out of the workspace.
// It reports false, changing nothing, for an index that is not an anchored
// slot — the floating slot cannot be removed.
func (s *Slots) RemoveAnchored(i int) bool {
	if s == nil || i < 0 || i >= len(s.anchored) {
		return false
	}
	s.anchored = append(s.anchored[:i:i], s.anchored[i+1:]...)
	return true
}

// MoveAnchored swaps the anchored slot at i with its neighbour: delta -1 is up,
// +1 is down. It reports false, changing nothing, at either end of the anchored
// list or for any other delta; nothing moves below the floating slot.
func (s *Slots) MoveAnchored(i, delta int) bool {
	if s == nil || (delta != -1 && delta != 1) {
		return false
	}
	j := i + delta
	if i < 0 || i >= len(s.anchored) || j < 0 || j >= len(s.anchored) {
		return false
	}
	s.anchored[i], s.anchored[j] = s.anchored[j], s.anchored[i]
	return true
}

// ReplaceAnchored gives the anchored slot at i a new chart, moment and place,
// keeping its selection and body marks. It is for the same chart arriving
// corrected (an edited chart file), where a pick would rightly start fresh.
func (s *Slots) ReplaceAnchored(i int, chart SessionChart, when time.Time, place location.PlaceInfo) error {
	if s == nil || i < 0 || i >= len(s.anchored) {
		return errors.New("store: Slots.ReplaceAnchored: no such slot")
	}
	if !place.HasTimezone() {
		return errors.New("store: Slots.ReplaceAnchored: place.TZ required")
	}
	chart.When = when
	chart.Place = place
	slot := s.anchored[i]
	slot.Time, slot.Location, slot.Chart = when, place, chart
	return nil
}

// AnchoredCount is how many anchored slots the workspace holds.
func (s *Slots) AnchoredCount() int {
	if s == nil {
		return 0
	}
	return len(s.anchored)
}

// FindAnchored returns the list index of the anchored slot holding the same
// chart as c, or -1. Identity is the Source file (SameChartAs); a chart with no
// Source is never found, so it is always added.
func (s *Slots) FindAnchored(c SessionChart) int {
	if s == nil {
		return -1
	}
	for i, slot := range s.anchored {
		if slot.Chart.SameChartAs(c) {
			return i
		}
	}
	return -1
}

// SameChartAs reports whether two records name the same open chart: both have a
// non-empty Source and the Sources are equal. One file, one row (operator,
// 2026-10-04, "file-decided"). Ids, moment and place do not decide — an id can
// change under astrolog's own rewrite and a file can come to hold somebody else,
// but either way the file is still the one chart the astrologer opened — and a
// chart with no Source never matches anything, so it is always added.
func (c SessionChart) SameChartAs(o SessionChart) bool {
	return c.Source.File != "" && c.Source == o.Source
}

// FindAnchored's identity is SameChartAs.

// Baseline is the first selected anchored slot in list order, or nil. It is the
// house frame and the left side of the comparison (M1).
func (s *Slots) Baseline() *Slot {
	if s == nil {
		return nil
	}
	for _, slot := range s.anchored {
		if slot.selected {
			return slot
		}
	}
	return nil
}

// SelectedInOrder is the selected set: the selected anchored slots in list
// order, then the floating slot when it is selected (it is pinned last). It
// holds at most MaxSelectedSlots slots. The baseline is its first anchored
// member, and a slot's numeral is its 1-based position in it.
func (s *Slots) SelectedInOrder() []*Slot {
	if s == nil {
		return nil
	}
	var out []*Slot
	for _, slot := range s.All() {
		if slot.selected {
			out = append(out, slot)
		}
	}
	return out
}

// Numeral is the 1-based position of slot in the selected set, or 0 when it is
// not selected (or is not one of this workspace's slots).
func (s *Slots) Numeral(slot *Slot) int {
	for i, member := range s.SelectedInOrder() {
		if member == slot {
			return i + 1
		}
	}
	return 0
}

// All returns the loaded slots in slot order: anchored slots in list order,
// then the floating slot last. Length is 1 with no anchored slot.
func (s *Slots) All() []*Slot {
	out := make([]*Slot, 0, len(s.anchored)+1)
	out = append(out, s.anchored...)
	return append(out, &s.floating)
}

// Floating returns a copy of the floating slot. Callers that need to
// mutate the floating slot's Time must go through AdvanceFloating so the
// store stays the single owner of the mutation surface.
func (s *Slots) Floating() Slot {
	return s.floating
}

// SlotSelected reports persistent selection for a slot index in All order.
func (s *Slots) SlotSelected(index int) bool {
	slot := s.slotAt(index)
	return slot != nil && slot.selected
}

// SelectedCount is how many slots, floating included, are selected.
func (s *Slots) SelectedCount() int {
	if s == nil {
		return 0
	}
	n := 0
	for _, slot := range s.All() {
		if slot.selected {
			n++
		}
	}
	return n
}

// CanSelect reports whether the slot at index could be toggled now: it is
// valid, and either already selected (deselecting always works) or below the
// MaxSelectedSlots cap.
func (s *Slots) CanSelect(index int) bool {
	slot := s.slotAt(index)
	return slot != nil && (slot.selected || s.SelectedCount() < MaxSelectedSlots)
}

// ToggleSlotSelection flips persistent selection for a slot index in All
// order. The returned bool is the new state; ok is false for an invalid index,
// and for selecting a seventh slot, which does nothing (M7). Deselecting always
// works.
func (s *Slots) ToggleSlotSelection(index int) (selected, ok bool) {
	slot := s.slotAt(index)
	if slot == nil {
		return false, false
	}
	if !slot.selected && s.SelectedCount() >= MaxSelectedSlots {
		return false, false
	}
	slot.selected = !slot.selected
	return slot.selected, true
}

// SetSlotSelected sets persistent selection for a slot index in All order,
// subject to the same MaxSelectedSlots cap. It reports whether the slot now
// holds the requested state.
func (s *Slots) SetSlotSelected(index int, selected bool) bool {
	slot := s.slotAt(index)
	if slot == nil {
		return false
	}
	if slot.selected == selected {
		return true
	}
	_, ok := s.ToggleSlotSelection(index)
	return ok
}

// ComparisonSelected reports whether the baseline anchored slot and the
// floating slot are both selected: the sole authority for the
// transit-over-natal comparison.
func (s *Slots) ComparisonSelected() bool {
	return s != nil && s.Baseline() != nil && s.floating.selected
}

// SetComparisonSelected turns the baseline-and-floating comparison on or off.
// On selects the floating slot and, when no anchored slot is selected, the
// first anchored slot; off deselects the baseline and the floating slot. It
// returns false when there is no anchored slot, or when switching on would
// need a seventh selection.
func (s *Slots) SetComparisonSelected(selected bool) bool {
	if s == nil || len(s.anchored) == 0 {
		return false
	}
	if !selected {
		if b := s.Baseline(); b != nil {
			b.selected = false
		}
		s.floating.selected = false
		return true
	}
	need := 0
	if s.Baseline() == nil {
		need++
	}
	if !s.floating.selected {
		need++
	}
	if s.SelectedCount()+need > MaxSelectedSlots {
		return false
	}
	if s.Baseline() == nil {
		s.anchored[0].selected = true
	}
	s.floating.selected = true
	return true
}

// BodySelected reports whether body is selected within one slot. Body
// selection is deliberately per-slot: selecting natal Mars must not silently
// select transit Mars.
func (s *Slots) BodySelected(slotIndex int, body aspect.Body) bool {
	slot := s.slotAt(slotIndex)
	if slot == nil || body == aspect.BodyUnknown {
		return false
	}
	_, ok := slot.selectedBodies[body]
	return ok
}

// HasBodySelections reports whether a slot has any selected body. Renderers use
// this to reserve one marker column for the entire Bodies table only while the
// table contains a selection.
func (s *Slots) HasBodySelections(slotIndex int) bool {
	slot := s.slotAt(slotIndex)
	return slot != nil && len(slot.selectedBodies) > 0
}

// SelectedBodies returns one slot's selected bodies in ascending body order —
// a stable order so consumers (the earlier change wheel highlight) derive deterministic
// output from it. Nil for an invalid slot or an empty selection.
func (s *Slots) SelectedBodies(slotIndex int) []aspect.Body {
	slot := s.slotAt(slotIndex)
	if slot == nil || len(slot.selectedBodies) == 0 {
		return nil
	}
	out := make([]aspect.Body, 0, len(slot.selectedBodies))
	for b := range slot.selectedBodies {
		out = append(out, b)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// ToggleBodySelection flips a body's selection within one slot. The returned
// bool is the new state; ok is false for an invalid slot or unknown body.
func (s *Slots) ToggleBodySelection(slotIndex int, body aspect.Body) (selected, ok bool) {
	slot := s.slotAt(slotIndex)
	if slot == nil || body == aspect.BodyUnknown {
		return false, false
	}
	if slot.selectedBodies == nil {
		slot.selectedBodies = make(map[aspect.Body]struct{})
	}
	if _, exists := slot.selectedBodies[body]; exists {
		delete(slot.selectedBodies, body)
		return false, true
	}
	slot.selectedBodies[body] = struct{}{}
	return true, true
}

// CopySelectionsFrom transfers selection by list position (the i-th anchored
// slot to the i-th anchored slot) and floating to floating. It is used when a
// location change rebuilds Slots but preserves the logical workspace, and that
// rebuild preserves the list.
func (s *Slots) CopySelectionsFrom(previous *Slots) {
	if s == nil || previous == nil {
		return
	}
	copySlotSelection(&s.floating, &previous.floating)
	for i := 0; i < len(s.anchored) && i < len(previous.anchored); i++ {
		copySlotSelection(s.anchored[i], previous.anchored[i])
	}
}

func (s *Slots) slotAt(index int) *Slot {
	if s == nil {
		return nil
	}
	all := s.All()
	if index < 0 || index >= len(all) {
		return nil
	}
	return all[index]
}

func copySlotSelection(dst, src *Slot) {
	dst.selected = src.selected
	if len(src.selectedBodies) == 0 {
		dst.selectedBodies = nil
		return
	}
	dst.selectedBodies = make(map[aspect.Body]struct{}, len(src.selectedBodies))
	for body := range src.selectedBodies {
		dst.selectedBodies[body] = struct{}{}
	}
}

// SetFloatingTime overwrites the floating slot's Time wholesale. Used
// by the UI's text commit (applyTextCommit) to store the computed time once
// the matching FirstFrameReadyMsg arrives — separating the operator's
// step target (m.targetFloatingTime in ui) from the displayed/computed
// slot Time keeps the chart panel header in lockstep with bodies and
// aspects through held-key bursts.
//
// Unlike AdvanceFloating, this does NOT do calendar arithmetic — the
// caller has already computed the target moment; this just stores it.
// Does nothing if t is zero (defensive — the text commit should
// always carry a real TransitTime in v1).
func (s *Slots) SetFloatingTime(t time.Time) {
	if t.IsZero() {
		return
	}
	s.floating.Time = t
}

// AdvanceFloating mutates the floating slot's Time by the given calendar
// delta + duration. Per canonical slot vocabulary the floating slot is
// the only mutable-Time slot (anchored is the immutable natal moment).
// Caller is responsible for triggering chart recompute against the new
// time — store stays passive about compute.
//
// Calendar stepping (years/months/days) runs in the floating slot's
// local timezone so "+1 month at the same local clock time" is
// preserved across DST boundaries. Without this normalization, a
// step across the DST transition would silently shift the displayed
// local time by an hour (the host/UTC clock advances cleanly while
// the user's wall clock has jumped). `dur` is added as a fixed-offset
// wall-clock duration (correct semantics for hour/minute steps:
// "1 hour later" means 1 hour of elapsed real time, not local-clock
// minute arithmetic).
//
// Negative values step backward. Passing all zeros is a no-op. Falls
// back to UTC arithmetic if the place's TZ fails to load (defensive;
// NewSlots/AddAnchored gate on HasTimezone so this branch shouldn't fire
// in practice).
func (s *Slots) AdvanceFloating(years, months, days int, dur time.Duration) {
	if years == 0 && months == 0 && days == 0 && dur == 0 {
		return
	}
	t := s.floating.Time
	if years != 0 || months != 0 || days != 0 {
		if loc, err := time.LoadLocation(s.floating.Location.TZ); err == nil {
			t = t.In(loc).AddDate(years, months, days)
		} else {
			t = t.AddDate(years, months, days)
		}
	}
	if dur != 0 {
		t = t.Add(dur)
	}
	s.floating.Time = t
}
