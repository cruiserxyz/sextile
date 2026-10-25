package store

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// TestNewSlots_StartsWithFloatingOnly asserts the first-run workspace
// invariant: exactly one slot, that slot is the floating slot, no anchor.
// Cursor state lives on ui.Model — store stays passive.
func TestNewSlots_StartsWithFloatingOnly(t *testing.T) {
	place := location.PlaceInfo{
		Name:    "Berlin",
		Country: "DE",
		Lat:     52.5244,
		Lon:     13.4105,
		TZ:      "Europe/Berlin",
	}

	slots, err := NewSlots(time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC), place)
	if err != nil {
		t.Fatalf("NewSlots: %v", err)
	}

	all := slots.All()
	if len(all) != 1 {
		t.Errorf("All() length = %d, want 1", len(all))
	}
	if !all[0].Floating {
		t.Error("All()[0] should be the floating slot at first run")
	}
	if slots.AnchoredCount() != 0 {
		t.Error("AnchoredCount() should be 0 at first run")
	}
}

// TestSlots_AddAnchored_AddsNatalAhead asserts that adding a natal anchor
// places it ahead of the floating slot in slot order, leaves the floating
// slot intact, and does not move the cursor off floating.
func TestSlots_AddAnchored_AddsNatalAhead(t *testing.T) {
	floatingPlace := location.PlaceInfo{
		Name: "Berlin", Country: "DE",
		Lat: 52.5244, Lon: 13.4105, TZ: "Europe/Berlin",
	}
	natalPlace := location.PlaceInfo{
		Name: "Amsterdam", Country: "NL",
		Lat: 52.3676, Lon: 4.9041, TZ: "Europe/Amsterdam",
	}

	slots, err := NewSlots(time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC), floatingPlace)
	if err != nil {
		t.Fatalf("NewSlots: %v", err)
	}
	if err := slots.AddAnchored(SessionChart{}, time.Date(1990, 1, 1, 14, 0, 0, 0, time.UTC), natalPlace); err != nil {
		t.Fatalf("AddAnchored: %v", err)
	}

	if slots.AnchoredCount() == 0 {
		t.Fatal("AnchoredCount() should be 1")
	}
	if slots.All()[0].Floating {
		t.Error("Anchored slot should have Floating=false")
	}

	all := slots.All()
	if len(all) != 2 {
		t.Fatalf("All() length = %d, want 2", len(all))
	}
	if all[0].Floating {
		t.Error("All()[0] should be the anchored slot, not the floating one")
	}
	if !all[1].Floating {
		t.Error("All()[1] should be the floating slot")
	}
	if !slots.ComparisonSelected() {
		t.Error("new anchored workspace should derive its default comparison from selected pair")
	}
}

func TestSlots_SelectionIsStoreOwnedAndPerSlot(t *testing.T) {
	slots, err := NewSlots(time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC), location.PlaceInfo{
		Name: "Berlin", Country: "DE", TZ: "Europe/Berlin",
	})
	if err != nil {
		t.Fatalf("NewSlots: %v", err)
	}
	if err := slots.AddAnchored(SessionChart{}, time.Date(1990, 1, 1, 14, 0, 0, 0, time.UTC), location.PlaceInfo{
		Name: "Amsterdam", Country: "NL", TZ: "Europe/Amsterdam",
	}); err != nil {
		t.Fatalf("AddAnchored: %v", err)
	}

	if selected, ok := slots.ToggleSlotSelection(0); !ok || selected {
		t.Fatalf("toggle anchored selection = (%v, %v), want (false, true)", selected, ok)
	}
	if slots.ComparisonSelected() {
		t.Error("comparison remained active after one selected slot was removed")
	}
	if !slots.SetComparisonSelected(true) || !slots.ComparisonSelected() {
		t.Error("SetComparisonSelected(true) did not restore the selected pair")
	}

	if selected, ok := slots.ToggleBodySelection(0, aspect.Mars); !ok || !selected {
		t.Fatalf("toggle natal Mars = (%v, %v), want (true, true)", selected, ok)
	}
	if !slots.HasBodySelections(0) {
		t.Error("natal slot did not report its body selection")
	}
	if slots.HasBodySelections(1) {
		t.Error("body-selection presence leaked into the floating slot")
	}
	if !slots.BodySelected(0, aspect.Mars) {
		t.Error("natal Mars selection was not retained")
	}
	if slots.BodySelected(1, aspect.Mars) {
		t.Error("natal Mars selection leaked into the floating slot")
	}
	if _, ok := slots.ToggleBodySelection(99, aspect.Mars); ok {
		t.Error("invalid slot index was accepted")
	}
	if _, ok := slots.ToggleBodySelection(0, aspect.BodyUnknown); ok {
		t.Error("unknown body was accepted")
	}
	if selected, ok := slots.ToggleBodySelection(0, aspect.Mars); !ok || selected || slots.HasBodySelections(0) {
		t.Fatalf("clearing last natal body selection = (%v, %v, has=%v), want (false, true, false)", selected, ok, slots.HasBodySelections(0))
	}
}

func TestSlots_CopySelectionsFromPreservesWorkspaceRoles(t *testing.T) {
	place := location.PlaceInfo{Name: "Berlin", Country: "DE", TZ: "Europe/Berlin"}
	anchorPlace := location.PlaceInfo{Name: "Amsterdam", Country: "NL", TZ: "Europe/Amsterdam"}
	previous, err := NewSlots(time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC), place)
	if err != nil {
		t.Fatalf("NewSlots(previous): %v", err)
	}
	if err := previous.AddAnchored(SessionChart{}, time.Date(1990, 1, 1, 14, 0, 0, 0, time.UTC), anchorPlace); err != nil {
		t.Fatalf("AddAnchored(previous): %v", err)
	}
	previous.ToggleSlotSelection(1) // floating off; anchor remains on
	previous.ToggleBodySelection(0, aspect.Sun)
	previous.ToggleBodySelection(1, aspect.Moon)

	next, err := NewSlots(time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC), place)
	if err != nil {
		t.Fatalf("NewSlots(next): %v", err)
	}
	if err := next.AddAnchored(SessionChart{}, time.Date(1990, 1, 1, 14, 0, 0, 0, time.UTC), anchorPlace); err != nil {
		t.Fatalf("AddAnchored(next): %v", err)
	}
	next.CopySelectionsFrom(previous)

	if !next.SlotSelected(0) || next.SlotSelected(1) || next.ComparisonSelected() {
		t.Errorf("slot selections not copied by role: anchor=%v floating=%v comparison=%v",
			next.SlotSelected(0), next.SlotSelected(1), next.ComparisonSelected())
	}
	if !next.BodySelected(0, aspect.Sun) || !next.BodySelected(1, aspect.Moon) {
		t.Error("per-slot body selections were not copied")
	}
}

// TestSlots_AdvanceFloating_AppliesCalendarAndDuration asserts that the
// floating slot's Time advances by AddDate semantics (calendar-aware
// year/month/day) followed by Add (wall-clock duration). Anchored slot
// is untouched — only the floating slot's Time is mutable per the
// canonical slot vocabulary contract.
func TestSlots_AdvanceFloating_AppliesCalendarAndDuration(t *testing.T) {
	floatingPlace := location.PlaceInfo{
		Name: "Berlin", Country: "DE",
		Lat: 52.5244, Lon: 13.4105, TZ: "Europe/Berlin",
	}
	natalPlace := location.PlaceInfo{
		Name: "Amsterdam", Country: "NL",
		Lat: 52.3676, Lon: 4.9041, TZ: "Europe/Amsterdam",
	}
	start := time.Date(2026, 1, 31, 12, 0, 0, 0, time.UTC)
	natalWhen := time.Date(1990, 6, 15, 14, 30, 0, 0, time.UTC)

	slots, err := NewSlots(start, floatingPlace)
	if err != nil {
		t.Fatalf("NewSlots: %v", err)
	}
	if err := slots.AddAnchored(SessionChart{}, natalWhen, natalPlace); err != nil {
		t.Fatalf("AddAnchored: %v", err)
	}

	// +1 month from Jan 31 normalizes to Mar 3 in 2026 (Feb has 28 days);
	// this is the documented AddDate behavior and the operator should see
	// it consistently. +3 hours on top.
	slots.AdvanceFloating(0, 1, 0, 3*time.Hour)

	floating := slots.All()[1] // anchored at [0], floating at [1] with anchor present
	wantTime := time.Date(2026, 3, 3, 15, 0, 0, 0, time.UTC)
	if !floating.Time.Equal(wantTime) {
		t.Errorf("floating.Time = %v, want %v (AddDate(0,1,0)+3h from Jan 31)",
			floating.Time.UTC(), wantTime)
	}

	// Anchor must NOT have moved.
	if !slots.All()[0].Time.Equal(natalWhen) {
		t.Errorf("anchored.Time mutated to %v, want unchanged %v",
			slots.All()[0].Time.UTC(), natalWhen)
	}
}

// TestSlots_AdvanceFloating_ZeroIsNoOp asserts the explicit no-op case
// — calling AdvanceFloating(0, 0, 0, 0) leaves Time exactly as it was.
func TestSlots_AdvanceFloating_ZeroIsNoOp(t *testing.T) {
	place := location.PlaceInfo{
		Name: "Berlin", Country: "DE",
		Lat: 52.5244, Lon: 13.4105, TZ: "Europe/Berlin",
	}
	start := time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)

	slots, err := NewSlots(start, place)
	if err != nil {
		t.Fatalf("NewSlots: %v", err)
	}

	slots.AdvanceFloating(0, 0, 0, 0)

	if !slots.All()[0].Time.Equal(start) {
		t.Errorf("zero-step mutated Time: got %v, want %v", slots.All()[0].Time.UTC(), start)
	}
}

// TestSlots_AdvanceFloating_NegativeStepsBack asserts the convention used
// by the UI key bindings (uppercase = backward): negative arguments step
// the floating slot's Time backward.
func TestSlots_AdvanceFloating_NegativeStepsBack(t *testing.T) {
	place := location.PlaceInfo{
		Name: "Berlin", Country: "DE",
		Lat: 52.5244, Lon: 13.4105, TZ: "Europe/Berlin",
	}
	start := time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)

	slots, err := NewSlots(start, place)
	if err != nil {
		t.Fatalf("NewSlots: %v", err)
	}

	slots.AdvanceFloating(-1, 0, 0, 0) // -1 year
	want := time.Date(2025, 4, 25, 12, 0, 0, 0, time.UTC)
	if !slots.All()[0].Time.Equal(want) {
		t.Errorf("after -1 year: Time = %v, want %v", slots.All()[0].Time.UTC(), want)
	}

	slots.AdvanceFloating(0, 0, 0, -2*time.Hour) // -2 hours
	want = time.Date(2025, 4, 25, 10, 0, 0, 0, time.UTC)
	if !slots.All()[0].Time.Equal(want) {
		t.Errorf("after -2 hours: Time = %v, want %v", slots.All()[0].Time.UTC(), want)
	}
}

// TestSlots_AdvanceFloating_DST_PreservesLocalClockTime is the reviewer
// a review finding stepping +1 month across the
// Europe/Berlin DST transition (CEST → CET on the last Sunday of
// October) must preserve the user-visible local clock time, not the
// underlying UTC offset. Pre-fix, AdvanceFloating did raw UTC
// arithmetic on slot.Time — a "+1 month" step from late September
// (CEST, UTC+2) to late October (CET, UTC+1) would have left the UTC
// hour unchanged but the local clock shifted by an hour, confusing
// the operator.
func TestSlots_AdvanceFloating_DST_PreservesLocalClockTime(t *testing.T) {
	berlin := location.PlaceInfo{
		Name: "Berlin", Country: "DE",
		Lat: 52.5244, Lon: 13.4105, TZ: "Europe/Berlin",
	}
	// Sep 25 12:00 UTC = Sep 25 14:00 CEST (Berlin DST on).
	start := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)
	slots, err := NewSlots(start, berlin)
	if err != nil {
		t.Fatalf("NewSlots: %v", err)
	}

	slots.AdvanceFloating(0, 1, 0, 0)

	// User mental model: same local clock time, one calendar month
	// forward. Oct 25 in Berlin is CET (DST ended Oct 25 2026), so
	// 14:00 local = 13:00 UTC.
	want := time.Date(2026, 10, 25, 13, 0, 0, 0, time.UTC)
	got := slots.All()[0].Time
	if !got.Equal(want) {
		t.Errorf("DST month-step regression:\n  got:  %v (%v)\n  want: %v (%v)\nLocal clock time must remain 14:00 across the DST boundary.",
			got.UTC(), got, want, want)
	}

	// Also verify by rendering the slot in Berlin TZ: the local clock
	// hour and minute should match the start's local clock hour and
	// minute (14:00).
	loc, _ := time.LoadLocation(berlin.TZ)
	gotLocal := got.In(loc)
	if gotLocal.Hour() != 14 || gotLocal.Minute() != 0 {
		t.Errorf("DST step local clock = %02d:%02d, want 14:00 (same as start)",
			gotLocal.Hour(), gotLocal.Minute())
	}
}

// TestSlots_AddAnchored_RefusesUntimedPlace asserts the IANA-timezone
// requirement is enforced for anchors the same way it is for NewSlots.
func TestSlots_AddAnchored_RefusesUntimedPlace(t *testing.T) {
	timed := location.PlaceInfo{
		Name: "Berlin", Country: "DE",
		Lat: 52.5244, Lon: 13.4105, TZ: "Europe/Berlin",
	}
	untimed := location.PlaceInfo{
		Name: "Nowhere", Country: "??",
		Lat: 0, Lon: 0,
	}

	slots, err := NewSlots(time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC), timed)
	if err != nil {
		t.Fatalf("NewSlots: %v", err)
	}
	if err := slots.AddAnchored(SessionChart{}, time.Date(1990, 1, 1, 14, 0, 0, 0, time.UTC), untimed); err == nil {
		t.Error("AddAnchored should refuse a place without an IANA timezone")
	}
	if slots.AnchoredCount() != 0 {
		t.Error("AnchoredCount() should still be 0 after a refused AddAnchored")
	}
}

func listSlots(t *testing.T, n int) *Slots {
	t.Helper()
	slots, err := NewSlots(time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC), location.PlaceInfo{
		Name: "Berlin", Country: "DE", Lat: 52.5244, Lon: 13.4105, TZ: "Europe/Berlin",
	})
	if err != nil {
		t.Fatalf("NewSlots: %v", err)
	}
	for i := 0; i < n; i++ {
		chart := SessionChart{ID: fmt.Sprintf("synthetic-%d", i+1), Name: fmt.Sprintf("Synthetic %d", i+1),
			Source: ChartSource{Kind: ChartKindFile, File: fmt.Sprintf("synthetic-%d.as", i+1)}}
		when := time.Date(1980+i, 1, 2, 3, 4, 0, 0, time.UTC)
		place := location.PlaceInfo{Name: fmt.Sprintf("Testford %d", i+1), Country: "TS", Lat: 10 + float64(i), Lon: 20 + float64(i), TZ: "Europe/Amsterdam"}
		if err := slots.AddAnchored(chart, when, place); err != nil {
			t.Fatalf("AddAnchored(%d): %v", i, err)
		}
	}
	return slots
}

func slotName(s *Slot) string { return s.Chart.Name }

// C1: the workspace holds any number of anchored slots, in list order, with the
// floating slot always last.
func TestSlotsHoldsAnyNumber(t *testing.T) {
	for _, n := range []int{0, 1, 2, 7} {
		slots := listSlots(t, n)
		all := slots.All()
		if len(all) != n+1 || slots.AnchoredCount() != n {
			t.Fatalf("%d anchored: All() has %d, AnchoredCount() = %d", n, len(all), slots.AnchoredCount())
		}
		for i := 0; i < n; i++ {
			if want := fmt.Sprintf("Synthetic %d", i+1); all[i].Floating || slotName(all[i]) != want {
				t.Errorf("%d anchored: All()[%d] = %q floating=%v, want %q anchored", n, i, slotName(all[i]), all[i].Floating, want)
			}
		}
		if !all[n].Floating {
			t.Errorf("%d anchored: the last slot is not the floating one", n)
		}
	}
}

// C2: time stepping touches the floating slot's Time and nothing else.
func TestOnlyFloatingTimeMoves(t *testing.T) {
	steps := map[string]func(*Slots){
		"AdvanceFloating": func(s *Slots) { s.AdvanceFloating(0, 0, 3, 90*time.Minute) },
		"SetFloatingTime": func(s *Slots) { s.SetFloatingTime(time.Date(2031, 8, 9, 10, 11, 0, 0, time.UTC)) },
	}
	for name, step := range steps {
		t.Run(name, func(t *testing.T) {
			slots := listSlots(t, 7)
			slots.ToggleBodySelection(2, aspect.Mars)
			beforeAnchored := make([]Slot, 0, 7)
			for _, s := range slots.All()[:7] {
				beforeAnchored = append(beforeAnchored, *s)
			}
			beforeFloating := slots.Floating()
			step(slots)

			after := slots.Floating()
			if after.Time.Equal(beforeFloating.Time) {
				t.Fatalf("the floating Time did not move")
			}
			if name == "AdvanceFloating" {
				want := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC).Add(90 * time.Minute)
				if !after.Time.Equal(want) {
					t.Errorf("floating Time = %v, want %v (exactly the step)", after.Time, want)
				}
			}
			after.Time = beforeFloating.Time
			if !reflect.DeepEqual(after, beforeFloating) {
				t.Errorf("a floating field other than Time changed: %+v -> %+v", beforeFloating, after)
			}
			for i, s := range slots.All()[:7] {
				if !reflect.DeepEqual(*s, beforeAnchored[i]) {
					t.Errorf("anchored slot %d changed: %+v -> %+v", i, beforeAnchored[i], *s)
				}
			}
		})
	}
}

// C4: the first chart added selects itself and the floating slot; later ones
// arrive unselected and disturb nothing.
func TestAddSelectionDefaults(t *testing.T) {
	slots := listSlots(t, 0)
	if err := slots.AddAnchored(SessionChart{ID: "a"}, time.Unix(0, 0), location.PlaceInfo{Name: "p", TZ: "UTC"}); err != nil {
		t.Fatal(err)
	}
	if !slots.SlotSelected(0) || !slots.SlotSelected(1) || !slots.ComparisonSelected() {
		t.Errorf("first add: added=%v floating=%v, want both selected", slots.SlotSelected(0), slots.SlotSelected(1))
	}

	// One anchored selected: the next arrives unselected and nothing else moves.
	slots.ToggleSlotSelection(1) // floating off
	if err := slots.AddAnchored(SessionChart{ID: "b"}, time.Unix(0, 0), location.PlaceInfo{Name: "p", TZ: "UTC"}); err != nil {
		t.Fatal(err)
	}
	if !slots.SlotSelected(0) || slots.SlotSelected(1) || slots.SlotSelected(2) {
		t.Errorf("second add: slots 0,1,2 selected = %v,%v,%v, want true,false,false",
			slots.SlotSelected(0), slots.SlotSelected(1), slots.SlotSelected(2))
	}
}

// C9: the baseline is the first selected anchored slot in list order. Changed
// by selected-charts-compare C1-C3: the baseline is no longer only the left side
// of a baseline-and-floating comparison, it is numeral 1 of the selected set
// (SelectedInOrder, Numeral) and the left side of every pair, so the numerals
// are asserted here beside it.
func TestBaselineIsFirstSelectedInListOrder(t *testing.T) {
	slots := listSlots(t, 3)
	// listSlots selected the 1st and the floating slot; select the 3rd.
	slots.ToggleSlotSelection(2)
	if b := slots.Baseline(); b == nil || slotName(b) != "Synthetic 1" {
		t.Fatalf("baseline = %v, want Synthetic 1", b)
	}
	all := slots.All()
	for i, want := range []int{1, 0, 2, 3} {
		if got := slots.Numeral(all[i]); got != want {
			t.Fatalf("numeral of slot %d = %d, want %d (set = 1st, 3rd, floating)", i, got, want)
		}
	}
	if got := slots.SelectedInOrder(); len(got) != 3 || got[0] != slots.Baseline() || got[2] != all[3] {
		t.Fatalf("SelectedInOrder = %d slots, baseline first and floating last expected", len(got))
	}
	if !slots.ComparisonSelected() {
		t.Fatal("no comparison with the baseline and the floating slot selected")
	}
	// Drop the 1st: the 3rd is now the first selected.
	slots.ToggleSlotSelection(0)
	if b := slots.Baseline(); b == nil || slotName(b) != "Synthetic 3" {
		t.Fatalf("after deselecting the 1st, baseline = %v, want Synthetic 3", b)
	}
	// Reselect the 1st, then move the 3rd above it with the list reorder.
	slots.ToggleSlotSelection(0)
	if !slots.MoveAnchored(2, -1) || !slots.MoveAnchored(1, -1) {
		t.Fatal("could not move the 3rd slot to the top")
	}
	if b := slots.Baseline(); b == nil || slotName(b) != "Synthetic 3" {
		t.Fatalf("after moving the 3rd to the top, baseline = %v, want Synthetic 3", b)
	}
	if slots.Numeral(slots.All()[0]) != 1 || slots.Numeral(slots.All()[1]) != 2 || slots.Numeral(slots.All()[3]) != 3 {
		t.Error("the numerals did not follow the list reorder")
	}
	if !slots.ComparisonSelected() || slots.Floating().Floating != true {
		t.Fatal("comparison lost its operands")
	}
	// Deselect the floating slot: no comparison.
	slots.ToggleSlotSelection(3)
	if slots.ComparisonSelected() {
		t.Error("comparison active with the floating slot deselected")
	}
	// Floating back on, every anchored slot off: no comparison either.
	slots.ToggleSlotSelection(3)
	for i := 0; i < 3; i++ {
		if slots.SlotSelected(i) {
			slots.ToggleSlotSelection(i)
		}
	}
	if slots.Baseline() != nil || slots.ComparisonSelected() {
		t.Error("comparison active with no anchored slot selected")
	}
}

// C18: an untimed place is refused and the list stays as it was.
func TestAddRefusesPlaceWithoutTimezone(t *testing.T) {
	slots := listSlots(t, 2)
	if err := slots.AddAnchored(SessionChart{ID: "x"}, time.Unix(0, 0), location.PlaceInfo{Name: "Nowhere"}); err == nil {
		t.Fatal("AddAnchored accepted a place with no timezone")
	}
	if slots.AnchoredCount() != 2 {
		t.Errorf("the list changed on a refused add: %d anchored", slots.AnchoredCount())
	}
}

// M7: a seventh selection is refused, and deselecting still works.
func TestSlotsRefuseASeventhSelection(t *testing.T) {
	slots := listSlots(t, 7)
	for i := 0; i < 7; i++ {
		if !slots.SlotSelected(i) {
			slots.ToggleSlotSelection(i)
		}
	}
	if got := slots.SelectedCount(); got != MaxSelectedSlots {
		t.Fatalf("selected %d slots, want the cap %d", got, MaxSelectedSlots)
	}
	if slots.CanSelect(6) {
		t.Error("CanSelect offered a seventh selection")
	}
	if _, ok := slots.ToggleSlotSelection(6); ok || slots.SlotSelected(6) {
		t.Error("a seventh slot was selected")
	}
	if !slots.CanSelect(0) {
		t.Error("a selected slot cannot be deselected at the cap")
	}
}

// Remove, move and find, with the pointer stability the interface promises.
func TestSlotsRemoveMoveFind(t *testing.T) {
	slots := listSlots(t, 3)
	held := slots.All()[1]
	if err := slots.AddAnchored(SessionChart{ID: "synthetic-4", Name: "Synthetic 4"}, time.Unix(0, 0), location.PlaceInfo{Name: "p", TZ: "UTC"}); err != nil {
		t.Fatal(err)
	}
	if slots.All()[1] != held {
		t.Error("a *Slot held across an add moved")
	}
	// One file, one row: the Source decides, whatever the id says.
	if i := slots.FindAnchored(SessionChart{ID: "another-id", Source: ChartSource{Kind: ChartKindFile, File: "synthetic-3.as"}}); i != 2 {
		t.Errorf("FindAnchored(synthetic-3.as) = %d, want 2", i)
	}
	if i := slots.FindAnchored(SessionChart{ID: "synthetic-3"}); i != -1 {
		t.Errorf("a chart with an id but no Source was found at %d", i)
	}
	if i := slots.FindAnchored(SessionChart{}); i != -1 {
		t.Errorf("a chart with neither ID nor Source was found at %d", i)
	}
	if slots.MoveAnchored(0, -1) || slots.MoveAnchored(3, 1) || slots.MoveAnchored(1, 2) {
		t.Error("an out-of-range move reported success")
	}
	if !slots.RemoveAnchored(1) || slots.AnchoredCount() != 3 || slots.All()[1] == held {
		t.Error("RemoveAnchored(1) did not take the second slot out")
	}
	if slots.RemoveAnchored(3) || slots.RemoveAnchored(-1) {
		t.Error("the floating slot, or a negative index, was removable")
	}
}
