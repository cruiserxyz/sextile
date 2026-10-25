package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/internal/ingest"
	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/store"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// Picking a new city via the atlas modal must clear stale chart positions
// so the modal-close → recompute gap shows empty panels rather than the
// previous city's chart.
func TestAtlasPicked_ClearsStaleChart(t *testing.T) {
	m := NewModel(Config{Accessible: true})
	m.startup = startupReady
	m.chart.positions = []aspect.Position{
		{Body: aspect.Sun, Longitude: 35.5},
	}
	m.chart.aspects = []aspect.Aspect{
		aspect.NewAspect(aspect.Sun, aspect.Moon, aspect.Square, aspect.Orb(0.5)),
	}

	picked := AtlasPickedMsg{Place: location.PlaceInfo{
		Name: "Munich", Country: "DE", Lat: 48.1374, Lon: 11.5761, TZ: "Europe/Berlin",
	}}
	got, cmd := applyAtlasPicked(m, picked)

	if got.startup != startupComputing {
		t.Errorf("startup = %q, want %q", got.startup, startupComputing)
	}
	if got.place.Name != "Munich" {
		t.Errorf("place.Name = %q, want %q", got.place.Name, "Munich")
	}
	if len(got.chart.positions) != 0 {
		t.Errorf("chart.positions = %d, want 0 (must clear stale chart)", len(got.chart.positions))
	}
	if len(got.chart.aspects) != 0 {
		t.Errorf("chart.aspects = %d, want 0 (must clear stale aspects)", len(got.chart.aspects))
	}
	if cmd == nil {
		t.Error("applyAtlasPicked returned nil cmd; expected the compute")
	}
}

// Switching cities (Dubai → Berlin → Tokyo) must always:
//   - update m.place to the new pick (the slot line derives its location from it)
//   - return a non-nil Cmd (the compute)
//   - flip startup back to startupComputing
//
// Context: user reported "Dubai → Berlin moon position stays the same".
// For the same UTC instant, geocentric moon longitude legitimately won't
// shift much across cities — but the lat/lon MUST propagate end-to-end so
// houses, topocentric corrections, and the active-city display all react.
// This test guards the UI's part of that contract; the lookup-level
// propagation lives in internal/lookup/first_call_test.go.
func TestApplyAtlasPicked_FiresComputeOnEachCityChange(t *testing.T) {
	cities := []location.PlaceInfo{
		{Name: "Dubai", Country: "AE", Lat: 25.276, Lon: 55.296, TZ: "Asia/Dubai"},
		{Name: "Berlin", Country: "DE", Lat: 52.5244, Lon: 13.4105, TZ: "Europe/Berlin"},
		{Name: "Tokyo", Country: "JP", Lat: 35.6762, Lon: 139.6503, TZ: "Asia/Tokyo"},
	}

	m := NewModel(Config{Accessible: true})
	m.startup = startupReady

	for _, p := range cities {
		got, cmd := applyAtlasPicked(m, AtlasPickedMsg{Place: p})
		if cmd == nil {
			t.Fatalf("pick %q: applyAtlasPicked returned nil cmd; recompute did not fire", p.Display())
		}
		if got.place != p {
			t.Errorf("pick %q: m.place = %+v, want %+v (slot line derives its location from place)", p.Display(), got.place, p)
		}
		if got.startup != startupComputing {
			t.Errorf("pick %q: startup = %q, want %q", p.Display(), got.startup, startupComputing)
		}
		m = got
	}
}

// Cold-start through the atlas modal (location resolution failed,
// user picked a city) must construct the slot workspace. Without this,
// m.slots stays nil forever, ↑/↓ silently no-op, and downstream slot
// surfaces stay dark for any first-run user who needed to disambiguate.
func TestApplyAtlasPicked_ColdStart_BuildsSlots(t *testing.T) {
	m := NewModel(Config{Accessible: true})
	m.startup = startupAtlasOpen
	// No prior place, no prior slots — the cold-start-via-atlas shape.
	if m.slots != nil {
		t.Fatalf("precondition: m.slots = %v, want nil before atlas pick", m.slots)
	}

	picked := AtlasPickedMsg{Place: location.PlaceInfo{
		Name: "Berlin", Country: "DE",
		Lat: 52.5244, Lon: 13.4105, TZ: "Europe/Berlin",
	}}
	got, _ := applyAtlasPicked(m, picked)

	if got.slots == nil {
		t.Fatal("m.slots = nil after atlas pick on cold start; workspace was never constructed")
	}
	all := got.slots.All()
	if len(all) != 1 {
		t.Errorf("len(slots.All()) = %d, want 1 (floating only — no anchor yet)", len(all))
	}
	if !all[0].Floating {
		t.Error("slots.All()[0] should be the floating slot")
	}
	if all[0].Location.Name != "Berlin" {
		t.Errorf("floating slot location = %q, want %q", all[0].Location.Name, "Berlin")
	}
	if got.cursorIdx != 0 {
		t.Errorf("cursorIdx = %d, want 0 (floating is at index 0 with no anchor)", got.cursorIdx)
	}
	if firstAnchor(got.slots) != nil {
		t.Error("firstAnchor(slots) should be nil on cold start without natal")
	}
}

// Re-picking a city after startup must rebuild the workspace at the
// new place AND preserve the natal anchor — moving cities does not move
// the user's birthplace. Without this, slot navigation keeps showing
// the old floating place even though m.place has changed.
func TestApplyAtlasPicked_RePick_RefreshesPlace_PreservesAnchor(t *testing.T) {
	natalPlace := location.PlaceInfo{
		Name: "Amsterdam", Country: "NL",
		Lat: 52.3676, Lon: 4.9041, TZ: "Europe/Amsterdam",
	}
	originalPlace := location.PlaceInfo{
		Name: "Berlin", Country: "DE",
		Lat: 52.5244, Lon: 13.4105, TZ: "Europe/Berlin",
	}

	m := NewModel(Config{Accessible: true})
	m.startup = startupReady
	m.place = originalPlace
	slots, err := store.NewSlots(m.clock.Now(), originalPlace)
	if err != nil {
		t.Fatalf("NewSlots: %v", err)
	}
	if err := addAnchor(slots, time.Date(1990, 1, 1, 14, 0, 0, 0, time.UTC), natalPlace); err != nil {
		t.Fatalf("addAnchor: %v", err)
	}
	m.slots = slots
	m.cursorIdx = 1
	m.slots.ToggleSlotSelection(1) // preserve a deliberate comparison-off state
	m.slots.ToggleBodySelection(0, aspect.Sun)
	m.slots.ToggleBodySelection(1, aspect.Moon)

	newPlace := location.PlaceInfo{
		Name: "Tokyo", Country: "JP",
		Lat: 35.6762, Lon: 139.6503, TZ: "Asia/Tokyo",
	}
	got, _ := applyAtlasPicked(m, AtlasPickedMsg{Place: newPlace})

	if got.slots == nil {
		t.Fatal("m.slots = nil after re-pick; workspace lost")
	}
	all := got.slots.All()
	if len(all) != 2 {
		t.Fatalf("len(slots.All()) = %d, want 2 (anchor preserved + floating)", len(all))
	}
	if all[0].Floating {
		t.Error("slots.All()[0] should be the anchored slot")
	}
	if all[0].Location.Name != "Amsterdam" {
		t.Errorf("anchor location = %q, want %q (anchor must NOT move with city re-pick)",
			all[0].Location.Name, "Amsterdam")
	}
	if !all[1].Floating {
		t.Error("slots.All()[1] should be the floating slot")
	}
	if all[1].Location.Name != "Tokyo" {
		t.Errorf("floating location = %q, want %q (floating must follow the new city)",
			all[1].Location.Name, "Tokyo")
	}
	if got.cursorIdx != 1 {
		t.Errorf("cursorIdx = %d, want 1 (floating index 1 when anchor present)", got.cursorIdx)
	}
}

// A3: a cold start whose location does not resolve, carrying a session that
// names a chart. Startup opens the atlas with nothing anchored, and the
// session's anchor comes back once the operator picks a city — driven through
// the real unresolved branch, not a hand-built stash.
func TestUnresolvedStartup_SessionSurvivesCityPick(t *testing.T) {
	snap := snapshotChart()
	p := withSession(ingest.StartupPerimeterReadMsg{
		LocationCandidate: location.Candidate{
			Source: "stored-location",
			Place:  location.RawPlace{Name: "Atlantis", Country: "??"},
		},
	}, v1(&snap, true, true, nil, nil))

	var tm tea.Model = NewModelWithClock(Config{Accessible: true}, FrozenClock(sessionFrozen))
	tm, _ = tm.Update(AtlasLoadedMsg{Index: location.NewIndex([]location.PlaceInfo{berlinP, fixtureOneP})})
	tm, _ = tm.Update(p)
	m := tm.(Model)
	if m.startup != startupAtlasOpen {
		t.Fatalf("startup = %q, want the atlas modal open", m.startup)
	}
	if m.slots != nil && firstAnchor(m.slots) != nil {
		t.Fatalf("anchored before the city was picked: %+v", firstAnchor(m.slots))
	}
	if m.pendingPerimeter == nil {
		t.Fatal("the unresolved branch did not stash the perimeter")
	}

	got, cmd := applyAtlasPicked(m, AtlasPickedMsg{Place: berlinP})
	if cmd == nil {
		t.Fatal("nil cmd after the city pick")
	}
	if got.pendingPerimeter != nil {
		t.Error("pendingPerimeter should be consumed by the pick")
	}
	a := anchoredSlot(t, got)
	if !a.Time.Equal(snap.When) || a.Location != snap.Place {
		t.Errorf("anchored slot = %v / %+v, want the session's %v / %+v", a.Time, a.Location, snap.When, snap.Place)
	}
	if !sameRecord(anchorRecord(got), snap) {
		t.Errorf("anchorChart = %+v, want the session's %+v", anchorRecord(got), snap)
	}
	if mode := selectComputeMode(got); mode != computeModeOverlay {
		t.Errorf("compute mode = %v, want the overlay route", mode)
	}
}

// Re-picking a city while a natal anchor is loaded must keep the chart
// state's lastChart, store selection, and comparison route. Without this, slots
// keep claiming an anchor (preserved by refreshSlotsForPlace) while the
// chart panels silently degrade to transit-only after every city change.
func TestApplyAtlasPicked_RePick_PreservesNatalOverlayRoute(t *testing.T) {
	natalPlace := location.PlaceInfo{
		Name: "Amsterdam", Country: "NL",
		Lat: 52.3676, Lon: 4.9041, TZ: "Europe/Amsterdam",
	}
	originalPlace := location.PlaceInfo{
		Name: "Berlin", Country: "DE",
		Lat: 52.5244, Lon: 13.4105, TZ: "Europe/Berlin",
	}
	natalWhen := time.Date(1990, 1, 1, 14, 0, 0, 0, time.UTC)

	m := NewModel(Config{Accessible: true})
	m.startup = startupReady
	m.place = originalPlace
	slots, err := store.NewSlots(m.clock.Now(), originalPlace)
	if err != nil {
		t.Fatalf("NewSlots: %v", err)
	}
	if err := addAnchor(slots, natalWhen, natalPlace); err != nil {
		t.Fatalf("addAnchor: %v", err)
	}
	m.slots = slots
	m.cursorIdx = 1
	m.slots.ToggleSlotSelection(1) // preserve a deliberate comparison-off state
	m.slots.ToggleBodySelection(0, aspect.Sun)
	m.slots.ToggleBodySelection(1, aspect.Moon)

	newPlace := location.PlaceInfo{
		Name: "Tokyo", Country: "JP",
		Lat: 35.6762, Lon: 139.6503, TZ: "Asia/Tokyo",
	}
	got, cmd := applyAtlasPicked(m, AtlasPickedMsg{Place: newPlace})

	if cmd == nil {
		t.Fatal("applyAtlasPicked returned nil cmd after natal-loaded re-pick")
	}
	if anchorHandle(got).IsZero() {
		t.Error("lastChart was cleared during natal-loaded re-pick; overlay route lost")
	}
	if anchorHandle(got).Place.Name != "Amsterdam" {
		t.Errorf("lastChart place = %q, want %q (anchor must not move with city re-pick)",
			anchorHandle(got).Place.Name, "Amsterdam")
	}
	if got.slots.ComparisonSelected() || !got.slots.SlotSelected(0) || got.slots.SlotSelected(1) {
		t.Errorf("slot selection changed across city re-pick: anchor=%v floating=%v comparison=%v",
			got.slots.SlotSelected(0), got.slots.SlotSelected(1), got.slots.ComparisonSelected())
	}
	if !got.slots.BodySelected(0, aspect.Sun) || !got.slots.BodySelected(1, aspect.Moon) {
		t.Error("per-slot body selection changed across city re-pick")
	}
}

// Two consecutive picks of the SAME city should still re-fire compute.
// A user may legitimately want to "refresh" the chart by re-picking; we
// must not optimise that away.
func TestApplyAtlasPicked_SameCityStillFiresCompute(t *testing.T) {
	berlin := location.PlaceInfo{
		Name: "Berlin", Country: "DE",
		Lat: 52.5244, Lon: 13.4105, TZ: "Europe/Berlin",
	}

	m := NewModel(Config{Accessible: true})
	m.startup = startupReady

	m1, cmd1 := applyAtlasPicked(m, AtlasPickedMsg{Place: berlin})
	if cmd1 == nil {
		t.Fatal("first Berlin pick returned nil cmd")
	}
	_, cmd2 := applyAtlasPicked(m1, AtlasPickedMsg{Place: berlin})
	if cmd2 == nil {
		t.Fatal("re-pick of same city returned nil cmd; refresh must always recompute")
	}
}

var munichDE = location.PlaceInfo{Name: "Munich", Country: "DE", Lat: 48.1374, Lon: 11.5761, TZ: "Europe/Berlin"}

// feedBack runs a command and feeds each message it produced back into the model
// once, as the loop would.
func feedBack(m Model, cmd tea.Cmd) Model {
	for _, msg := range runAll(cmd) {
		tm, next := m.Update(msg)
		m = tm.(Model)
		runAll(next)
	}
	return m
}

// Acceptance 5: a city picked in the atlas changes the Chart row and ends the
// guess, lasts for the session and writes nothing: no preferences file, no
// session file, nothing under the charts directory.
func TestAtlasPickWritesNothing(t *testing.T) {
	m, start := guessedStart(t)
	m = feedBack(m, start)
	if !m.placeGuessed {
		t.Fatal("fixture: the start is not a guess")
	}
	before := profileSnapshot(t)

	m, cmd := applyAtlasPicked(m, AtlasPickedMsg{Place: munichDE})
	if m.placeGuessed {
		t.Error("a pick is a choice: the guess mark stays")
	}
	m = feedBack(m, cmd)

	view := ansi.Strip(m.View())
	if !strings.Contains(view, "Munich, DE") || strings.Contains(view, "(guessed)") {
		t.Errorf("the Chart row after the pick:\n%s", view)
	}
	sameProfile(t, before, profileSnapshot(t), "an atlas pick")
	if _, err := os.Stat(store.PreferencesPath()); !os.IsNotExist(err) {
		t.Errorf("preferences.yaml exists after a pick: %v", err)
	}

	// With a preferences file already there it is left byte-identical.
	path := store.PreferencesPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("clock:\n  mode: held\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	before = profileSnapshot(t)
	m, cmd = applyAtlasPicked(m, AtlasPickedMsg{Place: testPlaceBerlin()})
	m = feedBack(m, cmd)
	sameProfile(t, before, profileSnapshot(t), "an atlas pick over an existing file")
}

// Acceptance 12: a start where no city can be guessed asks for one. A pick
// answers it for the session only, so the next start asks again, until `,c d`
// keeps the city.
func TestColdStartPickIsNotRemembered(t *testing.T) {
	sessionSandbox(t)
	asks := func() Model {
		t.Helper()
		m, _ := startFromDisk(t, munichDE) // no Amsterdam in this atlas: nothing resolves
		if m.startup != startupAtlasOpen {
			t.Fatalf("the start did not ask for a city: %q", m.startup)
		}
		return m
	}

	m := asks()
	m, cmd := applyAtlasPicked(m, AtlasPickedMsg{Place: munichDE})
	m = feedBack(m, cmd)
	if _, ok := store.Location(); ok {
		t.Fatal("a pick was remembered")
	}
	m = asks() // restart after the pick: asks again

	m, cmd = applyAtlasPicked(m, AtlasPickedMsg{Place: munichDE})
	m = feedBack(m, cmd)
	m, cmd = press(t, m, ",", "c", "d")
	m = deliver(m, cmd)
	if saved, ok := store.Location(); !ok || saved.Name != "Munich" {
		t.Fatalf("`,c d` did not keep the city: %+v %v", saved, ok)
	}

	r, _ := startFromDisk(t, munichDE) // restart after `,c d`: no question
	if r.startup == startupAtlasOpen || r.place.Name != "Munich" || r.placeGuessed {
		t.Errorf("after `,c d` the start is %q at %q (guessed %v), want it settled on Munich", r.startup, r.place.Name, r.placeGuessed)
	}
}
