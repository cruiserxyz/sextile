package ui

// Verifiers for the pick contract: a listing row carries no birth moment,
// so picking one resolves it — once, for that chart only — and everything the
// operator was looking at survives a resolve that fails.

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/internal/store"
)

// All chart values in this file are constructed synthetic test inputs.

// astrologShim points PATH at a fake `astrolog` that records every invocation.
// See the twin in internal/lookup for why this is a PATH shim and not a seam
// in production code, and what invariants.TestAstrologSpawnChokePoint (C22)
// has to hold for it to be able to fail for the right reason.
func astrologShim(t *testing.T) (count func() int, clear func()) {
	t.Helper()
	dir := t.TempDir()
	log := filepath.Join(dir, "invocations")
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> " + strconv.Quote(log) + "\nexit 1\n"
	if err := os.WriteFile(filepath.Join(dir, "astrolog"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	return func() int {
			b, err := os.ReadFile(log)
			if err != nil {
				return 0
			}
			return len(strings.Split(strings.TrimSuffix(string(b), "\n"), "\n"))
		}, func() {
			_ = os.Remove(log)
		}
}

const shimChart = "@AI780\n-qb Sep 23 1995 6:00pm DT 1E 11:34E 48:08N\n-zi \"Dat Client\" \"Munich, Germany\"\n"

// fixtureLibrary points ChartsDir at a temp copy of the earlier change fixture set:
// every .as under internal/astrolog/testdata/charts that resolves today. It is
// the set C6 and C17 compare against their pre-lazy baselines.
func fixtureLibrary(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	copyFixtures(t, dir, "../astrolog/testdata/charts", ".as")
	t.Setenv("SEXTILE_CHARTS_DIR", dir)
	t.Setenv("SEXTILE_DEV_CHARTS_DIR", "")
	return dir
}

func copyFixtures(t *testing.T, dst, src, ext string) {
	t.Helper()
	entries, err := os.ReadDir(src)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if filepath.Ext(e.Name()) != ext {
			continue
		}
		b, err := os.ReadFile(filepath.Join(src, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dst, e.Name()), b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// smallLibrary writes one .as chart into an isolated ChartsDir.
func smallLibrary(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("SEXTILE_CHARTS_DIR", dir)
	t.Setenv("SEXTILE_DEV_CHARTS_DIR", "")
	if err := os.WriteFile(filepath.Join(dir, "dat-client.as"), []byte(shimChart), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// openedPicker returns a ready model with the library loaded into an open
// picker, the way Ctrl+O leaves it.
func openedPicker(t *testing.T) Model {
	t.Helper()
	m := readyModelWithSlots(t, berlinP)
	u, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlO})
	m = u.(Model)
	if cmd == nil {
		t.Fatal("Ctrl+O dispatched no load")
	}
	m, _ = runCmd(t, m, cmd)
	if !m.picker.loaded {
		t.Fatal("the library did not load")
	}
	return m
}

// pick presses Enter on the row named `name` and runs the resolve it dispatches.
func pick(t *testing.T, m Model, name string) (Model, tea.Cmd) {
	t.Helper()
	idx := -1
	for i, c := range m.picker.matches {
		if c.Name == name {
			idx = i
		}
	}
	if idx < 0 {
		t.Fatalf("no row named %q in %v", name, pickerNames(m.picker.matches))
	}
	m.picker.cursor = idx
	u, cmd := updateChartModalKey(m, tea.KeyMsg{Type: tea.KeyEnter})
	m = u.(Model)
	if cmd == nil && m.startup == startupTitleOpen {
		// An untitled chart asks for a title first (the specification
		// C3.4). Enter with the title left empty skips it, which is the pick
		// exactly as it was (C3.6).
		u, cmd = updateTitleFormKey(m, tea.KeyMsg{Type: tea.KeyEnter})
		m = u.(Model)
	}
	if cmd == nil {
		t.Fatalf("picking %q dispatched no resolve", name)
	}
	return runCmd(t, m, cmd)
}

// C6, the anti-regression requirement and the reason this slice is safe.
// Anchoring a library chart must produce the same complete store.AnchorChart —
// When and Place alike — as it did before the lazy path existed, compared
// against a baseline captured from the OLD implementation and checked in.
func TestApplyChartPicked_AnchorChartMatchesPreLazyBaseline(t *testing.T) {
	type baseline struct {
		ID    string `json:"id"`
		Name  string `json:"name"`
		When  string `json:"when"`
		Place struct {
			Name    string  `json:"name"`
			Country string  `json:"country"`
			Lat     float64 `json:"lat"`
			Lon     float64 `json:"lon"`
			TZ      string  `json:"tz"`
		} `json:"place"`
	}
	raw, err := os.ReadFile("testdata/chart-anchor-prelazy.json")
	if err != nil {
		t.Fatal(err)
	}
	var want []baseline
	if err := json.Unmarshal(raw, &want); err != nil {
		t.Fatal(err)
	}
	if len(want) == 0 {
		t.Fatal("the pre-lazy baseline is empty; this test would pass vacuously")
	}

	fixtureLibrary(t)
	for _, b := range want {
		t.Run(b.ID, func(t *testing.T) {
			m := openedPicker(t)
			m, _ = pick(t, m, b.Name)

			wantWhen, err := time.Parse(time.RFC3339, b.When)
			if err != nil {
				t.Fatal(err)
			}
			got := anchorHandle(m)
			if !got.When.Equal(wantWhen) {
				t.Errorf("When = %v, want the pre-lazy %v", got.When.UTC(), wantWhen)
			}
			wantPlace := location.PlaceInfo{
				Name: b.Place.Name, Country: b.Place.Country,
				Lat: b.Place.Lat, Lon: b.Place.Lon, TZ: b.Place.TZ,
			}
			if got.Place != wantPlace {
				t.Errorf("Place = %+v, want the pre-lazy %+v", got.Place, wantPlace)
			}
			anchor := firstAnchor(m.slots)
			if anchor == nil {
				t.Fatal("the pick set no anchored slot")
			}
			if !anchor.Time.Equal(wantWhen) || anchor.Location != wantPlace {
				t.Errorf("anchored slot = {%v %+v}, want {%v %+v}", anchor.Time.UTC(), anchor.Location, wantWhen, wantPlace)
			}
		})
	}
}

// C16: drawing the loaded listing spawns nothing. Nothing on that screen needs
// astrolog — the name, the place and the year are already in hand.
func TestChartPicker_RenderSpawnsNothing(t *testing.T) {
	smallLibrary(t)
	m := openedPicker(t)
	count, clear := astrologShim(t)

	// Positive control: a path that DOES spawn, under this very shim.
	_, _ = lookup.ResolvePicked(t.Context(), store.NamedChart{
		Source: store.ChartSource{Kind: store.ChartKindFile, File: "dat-client.as"},
	})
	if got := count(); got != 1 {
		t.Fatalf("positive control recorded %d spawns, want 1 — the shim is not intercepting", got)
	}
	clear()

	out := renderChartModal(m.picker, Config{Accessible: true}, 132, 40)
	if !strings.Contains(out, "Dat Client") {
		t.Fatalf("the render did not draw the listing:\n%s", out)
	}
	if got := count(); got != 0 {
		t.Errorf("rendering the listing spawned astrolog %d time(s), want 0", got)
	}
}

// C18: nor does drawing it open any chart file for writing.
func TestChartPicker_RenderNeverWritesChartDir(t *testing.T) {
	dir := smallLibrary(t)
	m := openedPicker(t)
	readOnlyDir(t, dir)

	out := renderChartModal(m.picker, Config{Accessible: true}, 132, 40)
	if !strings.Contains(out, "Dat Client") {
		t.Errorf("a read-only library must still render its row:\n%s", out)
	}
}

// C21: picking a .as spawns astrolog exactly once — for that one chart, at the
// moment it is actually needed.
func TestApplyChartPicked_ChartFileSpawnsExactlyOne(t *testing.T) {
	smallLibrary(t)
	m := openedPicker(t)
	count, clear := astrologShim(t)

	// Positive control, as every PATH-shim verifier here carries: exercise a
	// known-spawning path first, so a "1" below cannot be a shim that stopped
	// intercepting while something else spawned.
	_, _ = lookup.ResolvePicked(t.Context(), store.NamedChart{
		Source: store.ChartSource{Kind: store.ChartKindFile, File: "dat-client.as"},
	})
	if got := count(); got != 1 {
		t.Fatalf("positive control recorded %d spawns, want 1 — the shim is not intercepting", got)
	}
	clear()

	m, _ = pick(t, m, "Dat Client")
	if got := count(); got != 1 {
		t.Errorf("picking one .as spawned astrolog %d time(s), want exactly 1", got)
	}
	// Under the shim the resolve fails, so C7's contract applies: the anchor
	// is untouched and the failure is reported.
	if anchorHandle(m) != (store.AnchorChart{}) {
		t.Errorf("a failed resolve must anchor nothing, got %+v", anchorHandle(m))
	}
}

// C7: a pick whose resolve fails changes nothing. The chart the astrologer was
// looking at stays exactly as it was, and the reason goes on the error lane.
func TestApplyChartPicked_ResolveFailureKeepsAnchor(t *testing.T) {
	m := readyModelWithSlots(t, berlinP)
	kept := time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := addAnchor(m.slots, kept, fixtureOneP); err != nil {
		t.Fatal(err)
	}

	picked, _ := applyChartPicked(m, ChartPickedMsg{Chart: testNamedChart("Broken", berlinP)})
	got, cmd := applyChartResolved(picked, ChartResolvedMsg{
		Chart: picked.picker.pending,
		Err:   errors.New("the chart file has no readable birth moment"),
		Gen:   picked.pickGen,
	})

	if cmd != nil {
		t.Error("a failed resolve must dispatch no recompute")
	}
	if !anchorHandle(got).When.Equal(kept) || anchorHandle(got).Place != fixtureOneP {
		t.Errorf("the anchor changed on a failed resolve: %+v", anchorHandle(got))
	}
	if anchor := firstAnchor(got.slots); anchor == nil || !anchor.Time.Equal(kept) {
		t.Errorf("the anchored slot changed on a failed resolve: %+v", anchor)
	}
	if got.picker.resolveErr == nil {
		t.Error("the picker must remember that the pick failed")
	}
	logged := strings.Join(got.logPanelLines(), "\n")
	if !strings.Contains(logged, "Couldn't open that chart.") {
		t.Errorf("the failure is not on the error lane; Log reads:\n%s", logged)
	}
	onConsole := false
	for _, l := range got.consoleLines() {
		if l.lane == laneError && strings.Contains(l.text, "Couldn't open that chart.") {
			onConsole = true
		}
	}
	if !onConsole {
		t.Error("an error-lane line must reach the console too")
	}
}

// The library lists on a weaker bar than anchoring needs, and that is
// deliberate: a listing row carries no birth moment at all, so a row
// can resolve to a record that cannot be anchored — here, one whose place is
// empty.
//
// Picking it used to build the anchored slot anyway. The header then showed two
// slots, selectComputeMode saw no usable anchor, the comparison never computed
// and `t` did nothing — with nothing said anywhere. An explicit pick that
// silently does nothing is indistinguishable from a broken key.
func TestApplyChartPicked_UnanchorableRowRefusesAndSaysWhy(t *testing.T) {
	m := readyModelWithSlots(t, berlinP)
	row := testNamedChart("Placeless Client", location.PlaceInfo{})
	row.PlaceLabel = "Nowhere"

	m, _ = applyChartPicked(m, ChartPickedMsg{Chart: row})
	m, cmd := resolved(t, m, time.Date(2001, 2, 3, 4, 5, 0, 0, time.UTC))

	if cmd != nil {
		t.Error("a refused pick must dispatch no recompute")
	}
	if m.picker.refusal == "" {
		t.Error("a refused pick must say why")
	}
	if got := firstAnchor(m.slots); got != nil {
		t.Errorf("a chart that cannot anchor anchored a slot anyway: %+v", got)
	}
	if n := len(m.slots.All()); n != 1 {
		t.Errorf("the workspace holds %d slots; the second one is the comparison that never computes", n)
	}
	if anchorHandle(m) != (store.AnchorChart{}) {
		t.Errorf("a refused pick moved the anchor handle: %+v", anchorHandle(m))
	}
}

// C12: library resolution never enters the recorded-command lane. an earlier change's
// console copies a command straight to a system clipboard, so that lane stays
// reserved for the spawns that produced the frame the operator is looking at.
func TestApplyChartPicked_ResolveStaysOutOfArgvLane(t *testing.T) {
	smallLibrary(t)
	m := openedPicker(t)
	m, _ = pick(t, m, "Dat Client")

	for _, l := range m.logLines {
		if l.lane == laneArgv {
			t.Errorf("a library resolution reached the recorded-command lane: %q", l.text)
		}
	}
}

// If astrolog cannot be spawned at all, only the anchor fails: the picker stays
// open and usable, with its listing intact.
func TestApplyChartPicked_AstrologMissingKeepsPicker(t *testing.T) {
	smallLibrary(t)
	m := openedPicker(t)
	listed := len(m.picker.matches)
	t.Setenv("PATH", t.TempDir()) // no astrolog anywhere

	m, _ = pick(t, m, "Dat Client")

	if m.startup != startupChartOpen {
		t.Errorf("the picker must stay open when a resolve fails, got %q", m.startup)
	}
	if len(m.picker.matches) != listed {
		t.Errorf("the listing lost rows on a failed resolve: %d, want %d", len(m.picker.matches), listed)
	}
	if m.picker.resolveErr == nil {
		t.Error("the picker must say the pick failed")
	}
	if anchorHandle(m) != (store.AnchorChart{}) {
		t.Errorf("nothing may be anchored when astrolog is missing, got %+v", anchorHandle(m))
	}
	// And the picker still works: the operator can type and move.
	u, _ := updateChartModalKey(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	if got := u.(Model); got.picker.filterText != "y" {
		t.Error("the picker stopped accepting keys after a failed resolve")
	}
}

// A row names a file; the file can be gone by the time it is picked. That must
// fail safely under C7, not anchor half a chart.
func TestApplyChartPicked_VanishedFileFailsSafely(t *testing.T) {
	dir := smallLibrary(t)
	m := openedPicker(t)
	if err := os.Remove(filepath.Join(dir, "dat-client.as")); err != nil {
		t.Fatal(err)
	}

	m, _ = pick(t, m, "Dat Client")
	if anchorHandle(m) != (store.AnchorChart{}) {
		t.Errorf("a vanished file anchored something: %+v", anchorHandle(m))
	}
	if m.picker.resolveErr == nil {
		t.Error("a vanished file must report a failed pick")
	}
	if !strings.Contains(strings.Join(m.logPanelLines(), "\n"), "Couldn't open that chart.") {
		t.Error("a vanished file must be reported on the error lane")
	}
}

// readOnlyDir makes dir and its contents unwritable for the test, restoring the
// permissions before t.TempDir's own cleanup needs them.
func readOnlyDir(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if err := os.Chmod(filepath.Join(dir, e.Name()), 0o400); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Chmod(dir, 0o700)
		for _, e := range entries {
			_ = os.Chmod(filepath.Join(dir, e.Name()), 0o600)
		}
	})
}
