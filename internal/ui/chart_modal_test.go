package ui

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/store"
)

var (
	berlinP     = location.PlaceInfo{Name: "Berlin", Country: "DE", Lat: 52.5244, Lon: 13.4105, TZ: "Europe/Berlin"}
	fixtureOneP = location.PlaceInfo{Name: "Test Bay", Country: "TS", Lat: 12.34, Lon: 56.78, TZ: "Etc/UTC"}
)

func testNamedChart(name string, place location.PlaceInfo) store.NamedChart {
	return store.NamedChart{
		ID:    name,
		Name:  name,
		Year:  1980,
		Place: place,
		// A labelled place keeps a .as row from asking for a title on pick.
		PlaceLabel: place.Name,
		Source:     store.ChartSource{Kind: store.ChartKindFile, File: name + ".as"},
	}
}

// runCmd executes a command and feeds its message back into the model, the way
// the bubbletea loop would. It is how a test walks the pick -> resolve -> anchor
// sequence that a lazy library turns every pick into.
func runCmd(t *testing.T, m Model, cmd tea.Cmd) (Model, tea.Cmd) {
	t.Helper()
	if cmd == nil {
		t.Fatal("no command to run")
	}
	u, next := m.Update(cmd())
	return u.(Model), next
}

// resolved delivers a successful resolve for the chart a pick is waiting on.
func resolved(t *testing.T, m Model, when time.Time) (Model, tea.Cmd) {
	t.Helper()
	return applyChartResolved(m, ChartResolvedMsg{Chart: m.picker.pending, When: when, Gen: m.pickGen})
}

func pickerNames(cs []store.NamedChart) []string {
	out := make([]string, len(cs))
	for i, c := range cs {
		out[i] = c.Name
	}
	return out
}

// readyModelWithSlots builds a post-startup Model with a floating workspace at
// `place`, so the anchor-set paths have slots to attach to.
func readyModelWithSlots(t *testing.T, place location.PlaceInfo) Model {
	t.Helper()
	m := NewModelWithClock(Config{Accessible: true}, FrozenClock(time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)))
	m.startup = startupReady
	m.place = place
	slots, err := store.NewSlots(m.clock.Now(), place)
	if err != nil {
		t.Fatalf("NewSlots: %v", err)
	}
	m.slots = slots
	m.targetFloatingTime = slots.Floating().Time
	return m
}

// C5: a listing row carries no birth moment, so a pick resolves BEFORE it
// anchors. The pick itself changes no anchor and starts no compute; the resolve
// landing is what sets the overlay handle and the anchored slot, bumps the
// workspace identity on the text surface, and routes the recompute to the
// overlay branch.
func TestApplyChartPicked_ResolvesBeforeAnchoring(t *testing.T) {
	m := readyModelWithSlots(t, berlinP)
	startRunID := m.refresh[surfaceText].runningRunID
	fixtureOne := testNamedChart("Constructed One", fixtureOneP)
	when := time.Date(2001, 2, 3, 4, 5, 0, 0, time.UTC)

	picked, cmd := applyChartPicked(m, ChartPickedMsg{Chart: fixtureOne})
	if cmd == nil {
		t.Fatal("a pick must dispatch a resolve")
	}
	if picked.startup != startupChartOpen && picked.startup != m.startup {
		t.Errorf("the picker should stay put while resolving; startup = %q", picked.startup)
	}
	if !picked.picker.resolving {
		t.Error("the picker must show that it is reading the chart")
	}
	if anchorHandle(picked) != (store.AnchorChart{}) {
		t.Errorf("nothing may be anchored before the resolve lands, got %+v", anchorHandle(picked))
	}
	if firstAnchor(picked.slots) != nil {
		t.Error("no anchored slot may exist before the resolve lands")
	}
	if picked.refresh[surfaceText].runningRunID != startRunID {
		t.Error("a pick must not start a compute before it knows the moment")
	}

	got, recompute := resolved(t, picked, when)
	if recompute == nil {
		t.Fatal("the resolve landing did not fire the recompute")
	}
	if got.picker.resolving {
		t.Error("resolving must clear once the resolve lands")
	}
	if got.startup != startupComputing {
		t.Errorf("startup = %q, want %q", got.startup, startupComputing)
	}
	if !anchorHandle(got).When.Equal(when) || anchorHandle(got).Place != fixtureOne.Place {
		t.Errorf("lastChart = %+v, want the resolved moment at %+v", anchorHandle(got), fixtureOne.Place)
	}
	anchor := firstAnchor(got.slots)
	if anchor == nil {
		t.Fatal("firstAnchor(slots) = nil; the picked chart did not become an anchored slot")
	}
	if !anchor.Time.Equal(when) || anchor.Location != fixtureOne.Place {
		t.Errorf("anchored slot = {%v, %+v}, want {%v, %+v}", anchor.Time, anchor.Location, when, fixtureOne.Place)
	}
	if got.cursorIdx != 0 {
		t.Errorf("cursorIdx = %d, want 0 (the newly anchored slot)", got.cursorIdx)
	}
	st := got.refresh[surfaceText]
	if st.runningRunID != startRunID+1 {
		t.Errorf("runningRunID = %d, want %d (identity bump starts the overlay run; prior in-flight superseded)", st.runningRunID, startRunID+1)
	}
	if st.commitFloor != st.wantID || st.commitFloor == 0 {
		t.Errorf("commitFloor = %d, wantID = %d — identity change must floor queued results at the new want", st.commitFloor, st.wantID)
	}
	// Opening a chart selects nothing, so no comparison is engaged: the
	// focused anchored chart computes alone until the operator selects a pair.
	if mode := selectComputeMode(got); mode != computeModeNatalSingle {
		t.Errorf("selectComputeMode = %v, want the single natal chart after a chart pick", mode)
	}
}

func TestChartPicker_CtrlO_OpensAndDispatchesLoad(t *testing.T) {
	m := readyModelWithSlots(t, berlinP)
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlO})
	got := updated.(Model)
	if got.startup != startupChartOpen {
		t.Errorf("startup = %q, want %q after Ctrl+O", got.startup, startupChartOpen)
	}
	if got.picker.prior != startupReady {
		t.Errorf("picker.prior = %q, want %q (the view to restore on Esc)", got.picker.prior, startupReady)
	}
	if cmd == nil {
		t.Error("Ctrl+O should dispatch loadChartsCmd")
	}
}

func TestApplyChartsLoaded_PopulatesAndFilters(t *testing.T) {
	m := readyModelWithSlots(t, berlinP)
	m.startup = startupChartOpen
	m.picker = chartPickerState{prior: startupReady}
	m.picker.filterText = "one"
	charts := []store.NamedChart{
		testNamedChart("Constructed One", fixtureOneP),
		testNamedChart("Example Three", berlinP),
	}
	got, _ := applyChartsLoaded(m, ChartsLoadedMsg{Charts: charts})

	if !got.picker.loaded {
		t.Error("picker.loaded should be true after ChartsLoadedMsg")
	}
	if len(got.picker.charts) != 2 {
		t.Errorf("charts = %d, want 2", len(got.picker.charts))
	}
	if len(got.picker.matches) != 1 || got.picker.matches[0].Name != "Constructed One" {
		t.Errorf("matches for %q = %v, want [Constructed One]", "one", pickerNames(got.picker.matches))
	}
}

func TestApplyChartsLoaded_Error(t *testing.T) {
	m := readyModelWithSlots(t, berlinP)
	m.picker = chartPickerState{prior: startupReady}
	got, _ := applyChartsLoaded(m, ChartsLoadedMsg{Err: errors.New("boom")})
	if got.picker.loadErr == nil {
		t.Error("loadErr should be set on a failed library read")
	}
	if !got.picker.loaded {
		t.Error("loaded should be true even on error (distinguishes from still-loading)")
	}
}

// Ctrl+O before the workspace is resolved (no slots / no timezone) must be a
// no-op — the chart picker anchors a transit-over-natal overlay and needs a
// transit place first (the reviewer an earlier change HIGH).
func TestChartPicker_CtrlO_NoopBeforeWorkspace(t *testing.T) {
	m := NewModel(Config{Accessible: true})
	m.startup = startupBootstrapping // location not resolved; m.slots nil, m.place zero
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlO})
	got := updated.(Model)
	if got.startup == startupChartOpen {
		t.Error("Ctrl+O must not open the picker before a workspace is resolved")
	}
	if cmd != nil {
		t.Error("Ctrl+O before resolution should dispatch nothing")
	}
}

// A late response from a superseded picker open must not clobber the current
// state (the reviewer an earlier change MED): the stale-gen message is dropped.
func TestApplyChartsLoaded_DropsStaleGen(t *testing.T) {
	m := readyModelWithSlots(t, berlinP)
	m.pickerGen = 2
	m.picker = chartPickerState{prior: startupReady, loaded: true, charts: []store.NamedChart{testNamedChart("Keep", berlinP)}}
	m.picker.refilter()

	got, _ := applyChartsLoaded(m, ChartsLoadedMsg{Err: errors.New("stale"), Gen: 1})
	if got.picker.loadErr != nil {
		t.Error("a stale-gen load response must be dropped, not latch loadErr")
	}
	if len(got.picker.charts) != 1 {
		t.Errorf("stale-gen drop must preserve current charts, got %d", len(got.picker.charts))
	}
}

// A successful load clears a prior error so a transient failure does not stick
// across opens (the reviewer an earlier change MED).
func TestApplyChartsLoaded_ClearsPriorError(t *testing.T) {
	m := readyModelWithSlots(t, berlinP)
	m.pickerGen = 1
	m.picker = chartPickerState{prior: startupReady, loaded: true, loadErr: errors.New("old")}
	got, _ := applyChartsLoaded(m, ChartsLoadedMsg{Charts: []store.NamedChart{testNamedChart("X", berlinP)}, Gen: 1})
	if got.picker.loadErr != nil {
		t.Error("a successful load must clear a prior loadErr")
	}
}

func TestChartPicker_Refilter_BrowseThenFilter(t *testing.T) {
	p := chartPickerState{loaded: true, charts: []store.NamedChart{
		testNamedChart("Example Three", berlinP),
		testNamedChart("Example Four", berlinP),
		testNamedChart("Constructed Two", fixtureOneP),
	}}
	p.refilter() // empty filter → browse all
	if len(p.matches) != 3 {
		t.Errorf("empty filter should browse all 3, got %d", len(p.matches))
	}
	p.filterText = "example"
	p.refilter()
	if len(p.matches) != 2 {
		t.Errorf("filter 'example' → 2, got %d: %v", len(p.matches), pickerNames(p.matches))
	}
}

func TestUpdateChartModalKey_EnterPicksFocused(t *testing.T) {
	m := readyModelWithSlots(t, berlinP)
	m.startup = startupChartOpen
	m.picker = chartPickerState{loaded: true, prior: startupReady, charts: []store.NamedChart{
		testNamedChart("Example Three", berlinP),
		testNamedChart("Constructed One", fixtureOneP),
	}}
	m.picker.refilter() // browse; cursor 0
	m.picker.cursor = 1 // focus the constructed chart

	updated, cmd := updateChartModalKey(m, tea.KeyMsg{Type: tea.KeyEnter})
	got := updated.(Model)
	if cmd == nil {
		t.Fatal("Enter should dispatch the focused row's resolve")
	}
	if got.picker.pending.Place != fixtureOneP {
		t.Errorf("resolving the wrong row: %+v, want Test Bay", got.picker.pending)
	}
	got, _ = resolved(t, got, time.Date(2001, 2, 3, 4, 5, 0, 0, time.UTC))
	if got.startup != startupComputing {
		t.Errorf("the resolved pick should compute → startup=%q, want computing", got.startup)
	}
	if anchorHandle(got).Place != fixtureOneP {
		t.Errorf("anchor set to the wrong chart: %+v, want Test Bay", anchorHandle(got))
	}
}

func TestUpdateChartModalKey_EscRestoresPrior(t *testing.T) {
	m := readyModelWithSlots(t, berlinP)
	m.startup = startupChartOpen
	m.picker = chartPickerState{loaded: true, prior: startupReady}
	updated, _ := updateChartModalKey(m, tea.KeyMsg{Type: tea.KeyEsc})
	if got := updated.(Model); got.startup != startupReady {
		t.Errorf("Esc → startup=%q, want restored prior startupReady", got.startup)
	}
}

func TestUpdateChartModalKey_EnterNoopWhenEmpty(t *testing.T) {
	m := readyModelWithSlots(t, berlinP)
	m.startup = startupChartOpen
	m.picker = chartPickerState{loaded: true, prior: startupReady} // loaded but no charts → no matches
	m.picker.refilter()
	updated, cmd := updateChartModalKey(m, tea.KeyMsg{Type: tea.KeyEnter})
	got := updated.(Model)
	if got.startup != startupChartOpen {
		t.Errorf("Enter with no matches must be a no-op; startup=%q", got.startup)
	}
	if cmd != nil {
		t.Error("Enter with no matches must not dispatch a compute")
	}
}

func TestUpdateChartModalKey_TypingFilters(t *testing.T) {
	m := readyModelWithSlots(t, berlinP)
	m.startup = startupChartOpen
	m.picker = chartPickerState{loaded: true, prior: startupReady, charts: []store.NamedChart{
		testNamedChart("Example Three", berlinP),
		testNamedChart("Constructed Two", fixtureOneP),
	}}
	m.picker.refilter()
	for _, r := range "two" {
		updated, _ := updateChartModalKey(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = updated.(Model)
	}
	if m.picker.filterText != "two" {
		t.Errorf("filterText = %q, want two", m.picker.filterText)
	}
	if len(m.picker.matches) != 1 || m.picker.matches[0].Name != "Constructed Two" {
		t.Errorf("matches = %v, want [Constructed Two]", pickerNames(m.picker.matches))
	}
}

// End-to-end through real files: author a chart on disk, open the picker
// (Ctrl+O), run the dispatched load, then Enter and run the dispatched resolve —
// and assert the authored chart became the natal anchor, with its moment read
// from the file at pick time rather than carried on the row.
func TestChartPicker_EndToEnd_RealDir(t *testing.T) {
	dir := t.TempDir()
	// Isolate via the top-precedence live tier: the dev tier is shadowed by
	// any existing operator library at the XDG default.
	t.Setenv("SEXTILE_CHARTS_DIR", dir)
	t.Setenv("SEXTILE_DEV_CHARTS_DIR", "")
	if err := os.WriteFile(filepath.Join(dir, "fixture-one.as"), []byte(shimChart), 0o644); err != nil {
		t.Fatal(err)
	}

	m := readyModelWithSlots(t, berlinP)

	// Ctrl+O opens the picker and dispatches the lazy load.
	u, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlO})
	m = u.(Model)
	if cmd == nil {
		t.Fatal("Ctrl+O dispatched no load cmd")
	}
	m, _ = runCmd(t, m, cmd)
	if len(m.picker.matches) != 1 || m.picker.matches[0].Name != "Dat Client" {
		t.Fatalf("picker did not load the authored chart: %v", pickerNames(m.picker.matches))
	}

	// Enter resolves it; the resolve landing sets the natal anchor.
	u, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = u.(Model)
	if anchorHandle(m) != (store.AnchorChart{}) {
		t.Error("Enter anchored before the resolve landed")
	}
	m, cmd = runCmd(t, m, cmd)
	want := time.Date(1995, 9, 23, 16, 0, 0, 0, time.UTC)
	wantPlace := location.PlaceInfo{Name: "Munich", Country: "DE", Lat: 48.13333333333333, Lon: 11.566666666666666, TZ: "Europe/Berlin"}
	if anchorHandle(m).Place != wantPlace || !anchorHandle(m).When.Equal(want) {
		t.Errorf("the resolved pick did not become the anchor: %+v", anchorHandle(m))
	}
	if firstAnchor(m.slots) == nil {
		t.Error("anchored slot not set after the end-to-end pick")
	}
	if cmd == nil {
		t.Error("the resolved pick should dispatch a recompute cmd")
	}
}

func TestRenderChartModal_StatesRenderWithoutPanic(t *testing.T) {
	loaded := chartPickerState{loaded: true, charts: []store.NamedChart{testNamedChart("Constructed One", fixtureOneP)}}
	loaded.refilter()
	out := renderChartModal(loaded, Config{Accessible: true}, 132, 40)
	if !strings.Contains(out, "Constructed One") {
		t.Errorf("render missing the chart name; got:\n%s", out)
	}
	// loading + empty-library + error states must render without panicking.
	_ = renderChartModal(chartPickerState{}, Config{Accessible: true}, 132, 40)
	_ = renderChartModal(chartPickerState{loaded: true}, Config{Accessible: true}, 80, 24)
	_ = renderChartModal(chartPickerState{loaded: true, loadErr: errors.New("x")}, Config{Accessible: true}, 80, 24)
}

// the hard rule. A library holding only files that failed to parse must
// never render "(no saved charts yet)": for a tool whose premise is local client
// data, telling an astrologer they have no clients when their files failed is
// indistinguishable from data loss. Every dropped file is named, with a reason.
func TestRenderChartModal_UnreadableFilesAreNamedNotSilent(t *testing.T) {
	state := chartPickerState{loaded: true, drops: []store.ChartDrop{
		{File: "fixture-one.yaml", Reason: "no chart block in the file"},
		{File: "example-three.as", Reason: "not a chart astrolog can read"},
	}}
	state.refilter()

	out := renderChartModal(state, Config{Accessible: true}, 132, 40)
	if strings.Contains(out, "no saved charts yet") {
		t.Errorf("files were present and failed; the picker must not claim an empty library:\n%s", out)
	}
	for _, want := range []string{"fixture-one.yaml", "no chart block", "example-three.as", "not a chart astrolog can read"} {
		if !strings.Contains(out, want) {
			t.Errorf("render does not name %q; got:\n%s", want, out)
		}
	}
}

// An empty library is still allowed to say it is empty — the rule is about
// files that were present and failed, not about a library with nothing in it.
func TestRenderChartModal_TrulyEmptyLibraryStillSaysSo(t *testing.T) {
	state := chartPickerState{loaded: true}
	state.refilter()
	if out := renderChartModal(state, Config{Accessible: true}, 132, 40); !strings.Contains(out, "no saved charts yet") {
		t.Errorf("an empty library with no failures should say so; got:\n%s", out)
	}
}

// The unreadable block keeps its reserved rows however long the chart list is,
// and however small the terminal: a full library must not push the failures off
// the bottom, which would restore exactly the silence an earlier change removes.
func TestRenderChartModal_UnreadableFilesSurviveALongList(t *testing.T) {
	var charts []store.NamedChart
	for _, n := range []string{"A", "B", "C", "D", "E", "F", "G", "H", "I", "J", "K", "L"} {
		charts = append(charts, testNamedChart("Client "+n, berlinP))
	}
	state := chartPickerState{loaded: true, charts: charts, drops: []store.ChartDrop{
		{File: "broken.yaml", Reason: "no chart block in the file"},
	}}
	state.refilter()

	for _, size := range []struct{ w, h int }{{132, 40}, {80, 24}} {
		out := renderChartModal(state, Config{Accessible: true}, size.w, size.h)
		if !strings.Contains(out, "broken.yaml") {
			t.Errorf("at %dx%d a long chart list hid the unreadable file; got:\n%s", size.w, size.h, out)
		}
		if !strings.Contains(out, "Client A") {
			t.Errorf("at %dx%d the charts themselves were pushed out; got:\n%s", size.w, size.h, out)
		}
	}
}

// With more failures than reserved rows the block spends its last row on the
// count rather than silently truncating: the operator still learns the total.
func TestRenderChartModal_ManyUnreadableFilesReportTheCount(t *testing.T) {
	var drops []store.ChartDrop
	for i := 0; i < 40; i++ {
		drops = append(drops, store.ChartDrop{File: fmt.Sprintf("client-%02d.yaml", i), Reason: "could not be opened"})
	}
	state := chartPickerState{loaded: true, charts: []store.NamedChart{testNamedChart("Readable", berlinP)}, drops: drops}
	state.refilter()

	out := renderChartModal(state, Config{Accessible: true}, 80, 24)
	if !strings.Contains(out, "more could not be read") {
		t.Errorf("an overflowing unreadable block must report what it hid; got:\n%s", out)
	}
	if !strings.Contains(out, "Readable") {
		t.Errorf("the readable chart must still show; got:\n%s", out)
	}
}

// Drop rows are inert. With nothing readable in the library, Enter must not
// anchor anything — a named failure is not a pickable chart.
func TestUpdateChartModalKey_EnterIgnoresUnreadableRows(t *testing.T) {
	m := readyModelWithSlots(t, berlinP)
	m.startup = startupChartOpen
	m.picker = chartPickerState{loaded: true, prior: startupReady, drops: []store.ChartDrop{
		{File: "broken.yaml", Reason: "no chart block in the file"},
	}}
	m.picker.refilter()

	updated, cmd := updateChartModalKey(m, tea.KeyMsg{Type: tea.KeyEnter})
	got := updated.(Model)
	if got.startup != startupChartOpen {
		t.Errorf("Enter over an unreadable row must be a no-op; startup=%q", got.startup)
	}
	if cmd != nil {
		t.Error("Enter over an unreadable row must not dispatch a compute")
	}
}

// The missing atlas is the one library failure that is not about a single file:
// it drops every .as at once and is fixed outside Sextile, so it goes on the
// error lane (visible in both the 6-row Log and the console) as well as into
// the picker's named rows.
func TestApplyChartsLoaded_MissingAtlasWritesTheErrorLane(t *testing.T) {
	m := readyModelWithSlots(t, berlinP)
	m.pickerGen = 1
	m.picker = chartPickerState{prior: startupReady}
	drops := []store.ChartDrop{{File: "jung.as", Reason: "no place list, so no city or timezone"}}

	got, _ := applyChartsLoaded(m, ChartsLoadedMsg{
		Drops:    drops,
		AtlasErr: errors.New("atlas.as not found in any of: /nowhere"),
		Gen:      1,
	})

	if len(got.picker.drops) != 1 {
		t.Fatalf("picker.drops = %d, want the one named file", len(got.picker.drops))
	}
	// an earlier change put this on the error lane; an earlier change made it a sentence. What the
	// operator has to be told is the same either way: the place list is the
	// thing that is missing, and it is named.
	logged := strings.Join(got.logPanelLines(), "\n")
	if !strings.Contains(logged, "atlas.as") {
		t.Errorf("the missing atlas is not on the error lane; Log reads:\n%s", logged)
	}
	onConsole := false
	for _, l := range got.consoleLines() {
		if l.lane == laneError && strings.Contains(l.text, "atlas.as") {
			onConsole = true
		}
	}
	if !onConsole {
		t.Error("an error-lane line must reach the console too")
	}
}

// A load that succeeds with no failures clears the previous open's drop rows,
// the same way it clears loadErr — a fixed file must not stay accused.
func TestApplyChartsLoaded_CleanLoadClearsPriorDrops(t *testing.T) {
	m := readyModelWithSlots(t, berlinP)
	m.pickerGen = 1
	m.picker = chartPickerState{prior: startupReady, loaded: true, drops: []store.ChartDrop{{File: "was-broken.yaml", Reason: "could not be opened"}}}

	got, _ := applyChartsLoaded(m, ChartsLoadedMsg{Charts: []store.NamedChart{testNamedChart("Fixed", berlinP)}, Gen: 1})
	if len(got.picker.drops) != 0 {
		t.Errorf("a clean load must clear prior drops, got %v", got.picker.drops)
	}
}

func TestChartRowShowsAgeAfterName(t *testing.T) {
	c := testNamedChart("Good Fella", fixtureOneP)
	c.Year, c.Month, c.Day = 1980, 6, 15
	today := time.Date(2026, 6, 15, 9, 0, 0, 0, time.UTC)
	colour := Config{Color: ColorAlways}

	t.Run("colours off is plain", func(t *testing.T) {
		got := listRow(chartRow(c, false, today), plainCfg, 60)
		if !strings.HasPrefix(got, "  Good Fella (46) ") {
			t.Errorf("row = %q, want `Good Fella (46)` after the cursor column", got)
		}
	})
	t.Run("colours on fades only the age", func(t *testing.T) {
		got := listRow(chartRow(c, false, today), colour, 60)
		if !strings.Contains(got, "Good Fella"+sgrFaint+" (46)"+sgrReset) {
			t.Errorf("row = %q, want the age faint straight after the name", got)
		}
		if w := ansi.StringWidth(got); w != 60 {
			t.Errorf("row is %d cells wide, want 60", w)
		}
		if !strings.HasSuffix(ansi.Strip(got), chartDate(c)) {
			t.Errorf("the date must stay at the right edge: %q", ansi.Strip(got))
		}
	})
	t.Run("a long name shortens, the age stays whole", func(t *testing.T) {
		long := c
		long.Name = strings.Repeat("Longname ", 8)
		got := ansi.Strip(listRow(chartRow(long, false, today), colour, 40))
		if !strings.Contains(got, " (46)") || ansi.StringWidth(got) != 40 {
			t.Errorf("row = %q (%d cells), want the age kept at width 40", got, ansi.StringWidth(got))
		}
	})
	t.Run("the day before the birthday is a year less", func(t *testing.T) {
		got := listRow(chartRow(c, false, today.AddDate(0, 0, -1)), plainCfg, 60)
		if !strings.Contains(got, "Good Fella (45)") {
			t.Errorf("row = %q, want (45) the day before the birthday", got)
		}
	})
	t.Run("under a year is zero", func(t *testing.T) {
		b := c
		b.Year, b.Month, b.Day = 2026, 1, 1
		if got := listRow(chartRow(b, false, today), plainCfg, 60); !strings.Contains(got, "Good Fella (0)") {
			t.Errorf("row = %q, want (0)", got)
		}
	})
	t.Run("no date, a future date or no clock shows no age", func(t *testing.T) {
		nd := c
		nd.Month, nd.Day = 0, 0
		fut := c
		fut.Year = 2030
		for name, got := range map[string]string{
			"no date":   listRow(chartRow(nd, false, today), plainCfg, 60),
			"future":    listRow(chartRow(fut, false, today), plainCfg, 60),
			"zero time": listRow(chartRow(c, false, time.Time{}), plainCfg, 60),
		} {
			if strings.Contains(got, "(") {
				t.Errorf("%s: row = %q, want no parentheses", name, got)
			}
		}
	})
}
