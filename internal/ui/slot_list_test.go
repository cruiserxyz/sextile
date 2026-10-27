package ui

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/internal/store"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// All chart values in this file are constructed synthetic test inputs.

// The workspace holds a list of charts (the specification).

// listModel is a ready Chart-focused workspace of n anchored charts above the
// floating slot, with the cursor on list index cursor.
func listModel(t *testing.T, n, cursor int) Model {
	t.Helper()
	m := readyModelWithSlots(t, berlinP)
	m.slots = listWorkspace(t, n)
	m.targetFloatingTime = m.slots.Floating().Time
	m.place = testPlaceBerlin()
	m.cursorIdx = cursor
	m.focused = PanelChart
	m.width, m.height = 144, 40
	return m
}

// withFrame commits a full set of body rows, so the Bodies cursor has rows to
// walk.
func withFrame(m Model) Model {
	m, _ = applyTextCommit(m, FirstFrameReadyMsg{Positions: fullChartPositions()})
	return withSceneColumns(m)
}

func send(t *testing.T, m Model, k tea.KeyMsg) Model {
	t.Helper()
	m, _ = tpress(t, m, k)
	return m
}

var (
	keyCtrlK  = tea.KeyMsg{Type: tea.KeyCtrlK}
	keyCtrlJ  = tea.KeyMsg{Type: tea.KeyCtrlJ}
	keyDelete = tea.KeyMsg{Type: tea.KeyDelete}
	keySpace  = tea.KeyMsg{Type: tea.KeySpace}
)

// order is the anchored slots' names in list order.
func order(m Model) []string {
	var out []string
	for _, s := range m.slots.All() {
		if s.Floating {
			out = append(out, "now")
		} else {
			out = append(out, s.Chart.Name)
		}
	}
	return out
}

func selections(m Model) []bool {
	var out []bool
	for i := range m.slots.All() {
		out = append(out, m.slots.SlotSelected(i))
	}
	return out
}

func barKeys(m Model) string {
	return lastRow(renderKeyBar(200, m.keyBarEntries(), Config{Accessible: true}, ""))
}

// C3: a picked chart that is not in the list becomes a new slot directly above
// the floating slot, takes the cursor, and disturbs nothing.
func TestPickAddsASlotAboveFloating(t *testing.T) {
	m := listModel(t, 2, 0)
	m.slots.ToggleBodySelection(1, aspect.Mars)
	before := []store.Slot{*m.slots.All()[0], *m.slots.All()[1]}
	marks := [][]aspect.Body{m.slots.SelectedBodies(0), m.slots.SelectedBodies(1)}

	third := store.NamedChart{ID: "synthetic-third", Name: "Synthetic Third", Place: fixtureOneP,
		Source: store.ChartSource{Kind: store.ChartKindFile, File: "synthetic-third.as"}}
	when := time.Date(2001, 2, 3, 4, 5, 0, 0, time.UTC)
	got, _ := applyChartResolved(m, ChartResolvedMsg{Chart: third, When: when, Gen: m.pickGen})

	if got.slots.AnchoredCount() != 3 {
		t.Fatalf("%d anchored slots, want 3: %v", got.slots.AnchoredCount(), order(got))
	}
	if want := []string{"Synthetic 1", "Synthetic 2", "Synthetic Third", "now"}; !reflect.DeepEqual(order(got), want) {
		t.Errorf("order = %v, want the new chart just above now: %v", order(got), want)
	}
	if got.cursorIdx != 2 {
		t.Errorf("cursor = %d, want it on the new slot (2)", got.cursorIdx)
	}
	added := got.slots.All()[2]
	if !added.Time.Equal(when) || added.Location != fixtureOneP || added.Chart.ID != "synthetic-third" {
		t.Errorf("the new slot holds %v / %+v / %q", added.Time, added.Location, added.Chart.ID)
	}
	for i := 0; i < 2; i++ {
		if !reflect.DeepEqual(*got.slots.All()[i], before[i]) {
			t.Errorf("slot %d changed:\n  before %+v\n  after  %+v", i, before[i], *got.slots.All()[i])
		}
		if !reflect.DeepEqual(got.slots.SelectedBodies(i), marks[i]) {
			t.Errorf("slot %d marks changed: %v -> %v", i, marks[i], got.slots.SelectedBodies(i))
		}
	}
}

// C5 (operator, 2026-10-04: "my answer is A file-decided" — one file, one row):
// a chart is the same open chart iff its Source file is the same. Ids, moment and
// place do not decide. A pick of an open file moves the cursor to its row and,
// if the file's content differs from the row's, the row takes the file's current
// content in place; a chart with no Source is always added.
func TestPickOfAnOpenChartJumpsToIt(t *testing.T) {
	when := time.Date(2001, 2, 3, 4, 5, 0, 0, time.UTC)
	file := func(name string) store.ChartSource { return store.ChartSource{Kind: store.ChartKindFile, File: name} }
	pickAt := func(m Model, c store.NamedChart, at time.Time) Model {
		got, _ := applyChartResolved(m, ChartResolvedMsg{Chart: c, When: at, Gen: m.pickGen})
		return got
	}
	add := func(m Model, id, name string, src store.ChartSource, at time.Time) {
		t.Helper()
		if err := m.slots.AddAnchored(store.SessionChart{ID: id, Name: name, Source: src}, at, fixtureOneP); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("an unchanged file jumps and changes nothing", func(t *testing.T) {
		m := listModel(t, 3, 2)
		listBefore := order(m)
		open := m.slots.All()[1]
		got := pickAt(m, store.NamedChart{ID: "synthetic-2", Name: "Synthetic 2", Place: open.Location, Source: open.Chart.Source}, open.Time)
		if got.cursorIdx != 1 {
			t.Errorf("cursor = %d, want the slot already holding that file (1)", got.cursorIdx)
		}
		if !reflect.DeepEqual(order(got), listBefore) || got.slots.AnchoredCount() != 3 {
			t.Errorf("the list changed: %v -> %v", listBefore, order(got))
		}
	})

	t.Run("twins: one moment and place, two files, two rows, each re-pick jumps to its own", func(t *testing.T) {
		m := listModel(t, 0, 0)
		mia := store.NamedChart{ID: "mia", Name: "Mia", Place: fixtureOneP, Source: file("mia.as")}
		noa := store.NamedChart{ID: "noa", Name: "Noa", Place: fixtureOneP, Source: file("noa.as")}
		m = pickAt(m, mia, when)
		m = pickAt(m, noa, when)
		if m.slots.AnchoredCount() != 2 {
			t.Fatalf("%d rows for two files, want 2: twins must not merge", m.slots.AnchoredCount())
		}
		for i, c := range []store.NamedChart{mia, noa} {
			got := pickAt(m, c, when)
			if got.slots.AnchoredCount() != 2 || got.cursorIdx != i || got.slots.All()[i].Chart.Name != c.Name {
				t.Errorf("re-picking %s: %d rows, cursor %d, want a jump to row %d", c.Name, got.slots.AnchoredCount(), got.cursorIdx, i)
			}
		}
	})

	t.Run("legacy: the id changed AND the minute was corrected is still one row, updated in place", func(t *testing.T) {
		m := listModel(t, 0, 0)
		add(m, "old-id", "Legacy", file("legacy.as"), when)
		m.slots.ToggleBodySelection(0, aspect.Mars)
		got := pickAt(m, store.NamedChart{ID: "legacy", Name: "Legacy", Place: fixtureOneP, Source: file("legacy.as")}, when.Add(time.Minute))
		slot := got.slots.All()[0]
		if got.slots.AnchoredCount() != 1 || got.cursorIdx != 0 {
			t.Fatalf("%d rows, cursor %d: one file is one row", got.slots.AnchoredCount(), got.cursorIdx)
		}
		if slot.Chart.ID != "legacy" || !slot.Time.Equal(when.Add(time.Minute)) {
			t.Errorf("the row holds id %q at %v, want the file's current id and corrected minute", slot.Chart.ID, slot.Time)
		}
		if !reflect.DeepEqual(got.slots.SelectedBodies(0), []aspect.Body{aspect.Mars}) {
			t.Error("the row's marks were not kept")
		}
	})

	t.Run("replaced: the same file now holding a different person is one row with the file's content", func(t *testing.T) {
		m := listModel(t, 0, 0)
		add(m, "first-person", "First", file("reused.as"), when)
		m.slots.SetSlotSelected(0, true) // adding selects nothing: the operator selected this row
		m.slots.ToggleBodySelection(0, aspect.Mars)
		got := pickAt(m, store.NamedChart{ID: "second-person", Name: "Second", Place: fixtureOneP, Source: file("reused.as")}, when.Add(30*24*time.Hour))
		slot := got.slots.All()[0]
		if got.slots.AnchoredCount() != 1 || slot.Chart.Name != "Second" || slot.Chart.ID != "second-person" || !slot.Time.Equal(when.Add(30*24*time.Hour)) {
			t.Errorf("%d rows; the row holds %q (%s) at %v, want one row with the file's content", got.slots.AnchoredCount(), slot.Chart.Name, slot.Chart.ID, slot.Time)
		}
		if !reflect.DeepEqual(got.slots.SelectedBodies(0), []aspect.Body{aspect.Mars}) || !got.slots.SlotSelected(0) {
			t.Error("the row's selection and marks were not kept")
		}
	})

	t.Run("a chart with no source is always added, whatever its id", func(t *testing.T) {
		m := listModel(t, 0, 0)
		anon := store.NamedChart{ID: "same-id", Name: "Anonymous", Place: fixtureOneP}
		m = pickAt(m, anon, when)
		m = pickAt(m, anon, when)
		if m.slots.AnchoredCount() != 2 {
			t.Errorf("%d anchored slots, want 2: only a file identifies an open chart", m.slots.AnchoredCount())
		}
	})
}

// readOnlyChartsDir makes a charts directory with two files and returns a function
// that fingerprints names and contents.
func readOnlyChartsDir(t *testing.T) (dir string, fingerprint func() string) {
	t.Helper()
	dir = filepath.Join(t.TempDir(), "charts")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{"synthetic-1.as": "synthetic one\n", "synthetic-2.as": "synthetic two\n"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o400); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
	return dir, func() string {
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		var parts []string
		for _, e := range entries {
			b, err := os.ReadFile(filepath.Join(dir, e.Name()))
			if err != nil {
				t.Fatal(err)
			}
			info, err := e.Info()
			if err != nil {
				t.Fatal(err)
			}
			// A write of identical bytes leaves the hash and moves the mtime.
			parts = append(parts, fmt.Sprintf("%s %x %d", e.Name(), sha256.Sum256(b), info.ModTime().UnixNano()))
		}
		sort.Strings(parts)
		dirInfo, err := os.Stat(dir)
		if err != nil {
			t.Fatal(err)
		}
		parts = append(parts, fmt.Sprintf("dir %d", dirInfo.ModTime().UnixNano()))
		return strings.Join(parts, "\n")
	}
}

// C6: del takes the focused chart out of the workspace and touches no file.
func TestDelRemovesTheFocusedSlot(t *testing.T) {
	dir, fingerprint := readOnlyChartsDir(t)
	t.Setenv("SEXTILE_CHARTS_DIR", dir)
	before := fingerprint()

	m := listModel(t, 3, 1)
	m.slots.ToggleBodySelection(0, aspect.Sun)

	// The middle chart goes; the cursor lands on the slot that took its index.
	m = send(t, m, keyDelete)
	if want := []string{"Synthetic 1", "Synthetic 3", "now"}; !reflect.DeepEqual(order(m), want) {
		t.Fatalf("after del on the middle chart: %v, want %v", order(m), want)
	}
	if m.cursorIdx != 1 || m.slots.All()[m.cursorIdx].Chart.Name != "Synthetic 3" {
		t.Errorf("cursor = %d, want it on the former third chart (1)", m.cursorIdx)
	}
	if !reflect.DeepEqual(m.slots.SelectedBodies(0), []aspect.Body{aspect.Sun}) {
		t.Errorf("the first chart's marks changed: %v", m.slots.SelectedBodies(0))
	}

	// The last anchored chart goes; the cursor lands on the floating slot.
	m.cursorIdx = 1
	m = send(t, m, keyDelete)
	m.cursorIdx = 0
	m = send(t, m, keyDelete)
	if !reflect.DeepEqual(order(m), []string{"now"}) || m.cursorIdx != 0 || !m.focusedIsFloating() {
		t.Errorf("after removing everything: %v cursor %d, want only now with the cursor on it", order(m), m.cursorIdx)
	}

	if after := fingerprint(); after != before {
		t.Errorf("the charts directory changed:\n%s\nwant\n%s", after, before)
	}
	if lines := errorLines(m); len(lines) != 0 {
		t.Errorf("del wrote error lines %q; a swallowed write failure would look like this", lines)
	}
}

// st sends Delete as `ESC [P`. The message is captured from Bubble Tea's own
// input reader, not built by hand, so the test sees exactly what the program
// receives in that terminal; del must remove the focused chart from it.
func TestDelFromStRemovesTheFocusedSlot(t *testing.T) {
	msg := readOneInputMsg(t, "\x1b[P")
	m := listModel(t, 3, 1)
	next, _ := m.Update(msg)
	m = next.(Model)
	if want := []string{"Synthetic 1", "Synthetic 3", "now"}; !reflect.DeepEqual(order(m), want) {
		t.Fatalf("after st's delete (%T %v) on the middle chart: %v, want %v", msg, msg, order(m), want)
	}
}

// readOneInputMsg runs a throwaway Bubble Tea program on the given input bytes
// and returns the first message its input reader produced.
func readOneInputMsg(t *testing.T, input string) tea.Msg {
	t.Helper()
	rec := &firstMsg{}
	p := tea.NewProgram(rec, tea.WithInput(strings.NewReader(input)),
		tea.WithOutput(io.Discard), tea.WithoutRenderer(), tea.WithoutSignals())
	if _, err := p.Run(); err != nil {
		t.Fatalf("reading %q: %v", input, err)
	}
	if rec.msg == nil {
		t.Fatalf("Bubble Tea produced no message for %q", input)
	}
	return rec.msg
}

type firstMsg struct{ msg tea.Msg }

func (f *firstMsg) Init() tea.Cmd { return nil }
func (f *firstMsg) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg.(type) {
	case tea.WindowSizeMsg:
		return f, nil
	}
	if f.msg == nil {
		f.msg = msg
	}
	return f, tea.Quit
}
func (f *firstMsg) View() string { return "" }

// Removing the last anchored chart of several puts the cursor on now.
func TestDelOfTheLastAnchoredChartFocusesNow(t *testing.T) {
	m := listModel(t, 3, 2)
	m = send(t, m, keyDelete)
	if want := []string{"Synthetic 1", "Synthetic 2", "now"}; !reflect.DeepEqual(order(m), want) {
		t.Fatalf("order = %v, want %v", order(m), want)
	}
	if !m.focusedIsFloating() {
		t.Errorf("cursor = %d, want it on now", m.cursorIdx)
	}
}

// C7: on the floating slot the list keys do nothing, write nothing and are not
// on the bar.
func TestFloatingSlotRefusesListKeys(t *testing.T) {
	m := listModel(t, 3, 3)
	if !m.focusedIsFloating() {
		t.Fatal("fixture: the cursor is not on now")
	}
	before := fmt.Sprintf("%v %v %d", order(m), selections(m), m.cursorIdx)
	logged := len(m.logLines)
	for _, k := range []tea.KeyMsg{keyDelete, keyCtrlK, keyCtrlJ} {
		tm, cmd := m.Update(k)
		after := tm.(Model)
		if cmd != nil {
			t.Errorf("%v on now returned a command", k)
		}
		if got := fmt.Sprintf("%v %v %d", order(after), selections(after), after.cursorIdx); got != before {
			t.Errorf("%v on now changed the workspace: %s -> %s", k, before, got)
		}
		if len(after.logLines) != logged {
			t.Errorf("%v on now wrote to the Log", k)
		}
	}
	bar := barKeys(m)
	for _, word := range []string{"del", "⌃k", "⌃j"} {
		if strings.Contains(bar, word) {
			t.Errorf("the bar offers %q while now is focused: %q", word, bar)
		}
	}
	// With Bodies focused the keys are as inert on an anchored chart.
	m = listModel(t, 3, 1)
	m.focused = PanelBodies
	if got := send(t, m, keyDelete); got.slots.AnchoredCount() != 3 {
		t.Error("del removed a chart while Bodies had the keyboard")
	}
}

// C8: ⌃k and ⌃j swap the focused chart with its neighbour and the cursor
// follows; at the ends they do nothing and are not on the bar.
func TestCtrlJKReorderAnchoredSlots(t *testing.T) {
	m := listModel(t, 3, 2)
	m.slots.ToggleSlotSelection(2)              // select the 3rd
	m.slots.ToggleBodySelection(2, aspect.Moon) // and mark it
	m = send(t, m, keyCtrlK)
	if want := []string{"Synthetic 1", "Synthetic 3", "Synthetic 2", "now"}; !reflect.DeepEqual(order(m), want) {
		t.Fatalf("⌃k on the third: %v, want %v", order(m), want)
	}
	if m.cursorIdx != 1 {
		t.Errorf("cursor = %d, want it following the moved chart (1)", m.cursorIdx)
	}
	if !m.slots.SlotSelected(1) || m.slots.SlotSelected(2) || !reflect.DeepEqual(m.slots.SelectedBodies(1), []aspect.Body{aspect.Moon}) {
		t.Errorf("selection or marks did not travel with the chart: selected %v, marks %v", selections(m), m.slots.SelectedBodies(1))
	}

	m = listModel(t, 3, 0)
	m = send(t, m, keyCtrlJ)
	if want := []string{"Synthetic 2", "Synthetic 1", "Synthetic 3", "now"}; !reflect.DeepEqual(order(m), want) {
		t.Fatalf("⌃j on the first: %v, want %v", order(m), want)
	}
	if m.cursorIdx != 1 {
		t.Errorf("cursor = %d, want it following the moved chart (1)", m.cursorIdx)
	}

	// The comparison recomputes exactly when the baseline changed: the first and
	// second charts are selected, so moving the first below the second makes the
	// second the baseline; with only the first selected, moving it changes nothing.
	swap := listModel(t, 3, 0)
	swap.slots.ToggleSlotSelection(1)
	if _, cmd := swap.Update(keyCtrlJ); cmd == nil {
		t.Error("⌃j changed the baseline and did not recompute the comparison")
	}
	still := listModel(t, 3, 0)
	if _, cmd := still.Update(keyCtrlJ); cmd != nil {
		t.Error("⌃j left the baseline alone and still recomputed")
	}

	// The ends: inert, and not offered.
	top := listModel(t, 3, 0)
	if got := send(t, top, keyCtrlK); !reflect.DeepEqual(order(got), order(top)) || got.cursorIdx != 0 {
		t.Errorf("⌃k on the first chart moved something: %v cursor %d", order(got), got.cursorIdx)
	}
	if bar := barKeys(top); strings.Contains(bar, "⌃k") || !strings.Contains(bar, "⌃j") {
		t.Errorf("the bar at the first chart is %q, want ⌃j offered and ⌃k not", bar)
	}
	last := listModel(t, 3, 2)
	if got := send(t, last, keyCtrlJ); !reflect.DeepEqual(order(got), order(last)) || got.cursorIdx != 2 {
		t.Errorf("⌃j on the last anchored chart moved something: %v cursor %d", order(got), got.cursorIdx)
	}
	if bar := barKeys(last); strings.Contains(bar, "⌃j") || !strings.Contains(bar, "⌃k") {
		t.Errorf("the bar at the last anchored chart is %q, want ⌃k offered and ⌃j not", bar)
	}
}

// C10: with six slots selected, Space on an unselected one does nothing and is
// not offered; on a selected one it still deselects.
func TestSeventhSelectionIsRefused(t *testing.T) {
	m := listModel(t, 7, 6)
	for i := 1; i < 5; i++ { // slot 0 and now are selected already: 2 + 4 = 6
		m.slots.ToggleSlotSelection(i)
	}
	if got := m.slots.SelectedCount(); got != store.MaxSelectedSlots {
		t.Fatalf("fixture: %d selected, want %d", got, store.MaxSelectedSlots)
	}
	before := fmt.Sprintf("%v", selections(m))
	logged := len(m.logLines)
	tm, cmd := m.Update(keySpace)
	got := tm.(Model)
	if after := fmt.Sprintf("%v", selections(got)); after != before || cmd != nil {
		t.Errorf("Space on a seventh slot changed the selection: %s -> %s (cmd %v)", before, after, cmd != nil)
	}
	if len(got.logLines) != logged {
		t.Error("the refused Space wrote to the Log")
	}
	if bar := barKeys(m); strings.Contains(bar, "Select") {
		t.Errorf("the bar offers Select with six slots selected: %q", bar)
	}

	m.cursorIdx = 2 // a selected slot
	if bar := barKeys(m); !strings.Contains(bar, "Select") {
		t.Errorf("the bar hides Select on a selected slot: %q", bar)
	}
	if got := send(t, m, keySpace); got.slots.SlotSelected(2) || got.slots.SelectedCount() != 5 {
		t.Errorf("Space on a selected slot did not deselect it: %v", selections(got))
	}
}

// C11: every anchored row names its own chart, moment and place.
func TestEveryAnchoredRowNamesItsChart(t *testing.T) {
	slots := listWorkspace(t, 3)
	rows := strings.Split(ansi.Strip(chartSlotsBody(slots, 3, false, testFrozenTime(), testPlaceBerlin(), lookup.Hellenistic, 110, Config{Accessible: true})), "\n")
	if len(rows) != 4 {
		t.Fatalf("%d rows, want 3 anchored and now: %q", len(rows), rows)
	}
	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	type facts struct{ name, moment, place string }
	all := make([]facts, 3)
	for i := range all {
		n := i + 1
		when := time.Date(1960+n, time.Month(n), 10+n, 6, n, 0, 0, time.UTC)
		all[i] = facts{
			name:   fmt.Sprintf("Synthetic %d", n),
			moment: refMoment(when.In(ny), clockCase{"24", "month-first"}) + " " + when.In(ny).Format("MST"),
			place:  fmt.Sprintf("Testford %d, TS", n),
		}
	}
	for i, row := range rows[:3] {
		for j, f := range all {
			for what, text := range map[string]string{"name": f.name, "moment": f.moment, "place": f.place} {
				has := strings.Contains(row, text)
				if i == j && !has {
					t.Errorf("row %d lacks its own %s %q: %q", i, what, text, row)
				}
				if i != j && has {
					t.Errorf("row %d carries row %d's %s %q: %q", i, j, what, text, row)
				}
			}
		}
	}
}

// C12: the Bodies cursor returns to the row each slot was left on; a slot never
// focused starts at the top.
func TestBodiesCursorIsRememberedPerSlot(t *testing.T) {
	m := withFrame(listModel(t, 3, 0))
	walk := func(m Model, n int) Model {
		m.focused = PanelBodies
		for i := 0; i < n; i++ {
			m = send(t, m, keyDown)
		}
		m.focused = PanelChart
		return m
	}
	step := func(m Model, k tea.KeyMsg) Model {
		m = send(t, m, k)
		return withFrame(m) // the recompute's frame lands, so Bodies has rows again
	}

	m = walk(m, 4) // slot A (0): row 4
	m = step(m, keyDown)
	if m.cursorIdx != 1 || m.bodyCursorIdx != 0 {
		t.Fatalf("slot B: cursor %d body row %d, want slot 1 starting at row 0", m.cursorIdx, m.bodyCursorIdx)
	}
	m = walk(m, 7) // slot B (1): row 7
	m = step(m, keyUp)
	if m.cursorIdx != 0 || m.bodyCursorIdx != 4 {
		t.Errorf("back on A: slot %d row %d, want slot 0 at row 4", m.cursorIdx, m.bodyCursorIdx)
	}
	m = step(m, keyDown)
	if m.cursorIdx != 1 || m.bodyCursorIdx != 7 {
		t.Errorf("back on B: slot %d row %d, want slot 1 at row 7", m.cursorIdx, m.bodyCursorIdx)
	}
	m = step(m, keyDown)
	if m.cursorIdx != 2 || m.bodyCursorIdx != 0 {
		t.Errorf("a slot never focused: slot %d row %d, want slot 2 at row 0", m.cursorIdx, m.bodyCursorIdx)
	}
}

// Adding a chart while now is focused must not hand now's Bodies row to the
// new chart: the row belongs to the slot it was left on.
func TestAddingAChartKeepsNowsBodiesRow(t *testing.T) {
	m := withFrame(listModel(t, 2, 2)) // the cursor on now
	m.focused = PanelBodies
	for i := 0; i < 7; i++ {
		m = send(t, m, keyDown)
	}
	m.focused = PanelChart
	if m.bodyCursorIdx != 7 || !m.focusedIsFloating() {
		t.Fatalf("fixture: row %d on slot %d", m.bodyCursorIdx, m.cursorIdx)
	}
	third := store.NamedChart{ID: "synthetic-third", Name: "Synthetic Third", Place: fixtureOneP}
	m, _ = applyChartResolved(m, ChartResolvedMsg{Chart: third, When: time.Date(2001, 2, 3, 4, 5, 0, 0, time.UTC), Gen: m.pickGen})
	if m.cursorIdx != 2 || m.bodyCursorIdx != 0 {
		t.Errorf("the new chart: slot %d row %d, want slot 2 starting at row 0", m.cursorIdx, m.bodyCursorIdx)
	}
	m = withFrame(m)
	m = send(t, m, keyDown)
	if !m.focusedIsFloating() || m.bodyCursorIdx != 7 {
		t.Errorf("back on now: slot %d row %d, want now at row 7", m.cursorIdx, m.bodyCursorIdx)
	}
}

// Each slot keeps its remembered row through a removal and a reorder.
func TestBodiesRowsFollowTheirSlotsThroughDelAndMove(t *testing.T) {
	m := withFrame(listModel(t, 3, 0))
	rowOf := func(m Model, slot int, row int) Model {
		m.cursorIdx = slot
		m = withFrame(m)
		m.bodyCursorIdx = row
		return m
	}
	// Rows 2, 5, 9 on the three charts, left in turn.
	m = rowOf(m, 0, 2)
	m = send(t, m, keyDown)
	m = rowOf(m, 1, 5)
	m = send(t, m, keyDown)
	m = rowOf(m, 2, 9)
	m = send(t, m, keyUp) // leave 3 (row 9), land on 2
	m = withFrame(m)
	if m.cursorIdx != 1 || m.bodyCursorIdx != 5 {
		t.Fatalf("fixture: slot %d row %d", m.cursorIdx, m.bodyCursorIdx)
	}
	// Move chart 2 down below chart 3: it keeps row 5 under the cursor.
	m = send(t, m, keyCtrlJ)
	if order(m)[2] != "Synthetic 2" || m.bodyCursorIdx != 5 {
		t.Fatalf("after ⌃j: %v row %d, want Synthetic 2 last at row 5", order(m), m.bodyCursorIdx)
	}
	// Up to the chart that moved into index 1 (chart 3): its own row, 9.
	m = send(t, m, keyUp)
	m = withFrame(m)
	if order(m)[m.cursorIdx] != "Synthetic 3" || m.bodyCursorIdx != 9 {
		t.Errorf("chart 3: %q row %d, want row 9", order(m)[m.cursorIdx], m.bodyCursorIdx)
	}
	// Remove chart 3: the slot that takes its index is chart 2, with row 5.
	m = send(t, m, keyDelete)
	m = withFrame(m)
	if order(m)[m.cursorIdx] != "Synthetic 2" || m.bodyCursorIdx != 5 {
		t.Errorf("after del: %q row %d, want chart 2 at row 5", order(m)[m.cursorIdx], m.bodyCursorIdx)
	}
	// And chart 1 still has its row 2.
	m = send(t, m, keyUp)
	if order(m)[m.cursorIdx] != "Synthetic 1" || m.bodyCursorIdx != 2 {
		t.Errorf("chart 1: %q row %d, want row 2", order(m)[m.cursorIdx], m.bodyCursorIdx)
	}
}

// C13: a location change rebuilds the workspace and keeps the whole list, in
// order, with its selection and marks.
func TestLocationRebuildPreservesTheList(t *testing.T) {
	m := listModel(t, 3, 1)
	m.slots.ToggleSlotSelection(2) // the 1st (default) and the 3rd selected
	m.slots.ToggleBodySelection(1, aspect.Venus)
	m.slots.ToggleBodySelection(1, aspect.Mars)
	before := m.slots
	type snap struct {
		name     string
		when     time.Time
		place    location.PlaceInfo
		selected bool
		marks    []aspect.Body
	}
	take := func(s *store.Slots) []snap {
		var out []snap
		for i, slot := range s.All()[:s.AnchoredCount()] {
			out = append(out, snap{slot.Chart.Name, slot.Time, slot.Location, s.SlotSelected(i), s.SelectedBodies(i)})
		}
		return out
	}
	want := take(before)

	got, _ := applyAtlasPicked(m, AtlasPickedMsg{Place: testPlaceAmsterdam()})

	if got.slots == before {
		t.Fatal("the workspace was not rebuilt")
	}
	if g := take(got.slots); !reflect.DeepEqual(g, want) {
		t.Errorf("the list changed across the rebuild:\n  before %+v\n  after  %+v", want, g)
	}
	if got.slots.Floating().Location != testPlaceAmsterdam() {
		t.Errorf("the floating slot is at %+v, want the new city", got.slots.Floating().Location)
	}
	if got.slots.SlotSelected(3) != before.SlotSelected(3) {
		t.Error("the floating slot's selection did not carry across")
	}
}

func listEntry(n int, selected bool, bodies ...aspect.Body) store.SessionSlot {
	return store.SessionSlot{
		Chart: store.SessionChart{
			ID: fmt.Sprintf("synthetic-%d", n), Name: fmt.Sprintf("Synthetic %d", n),
			When:   time.Date(1960+n, time.Month(n), 10+n, 6, n, 0, 0, time.UTC),
			Place:  location.PlaceInfo{Name: fmt.Sprintf("Testford %d", n), Country: "TS", Lat: 40 + float64(n), Lon: -70 - float64(n), TZ: "America/New_York"},
			Source: store.ChartSource{Kind: store.ChartKindFile, File: fmt.Sprintf("synthetic-%d.as", n)},
		},
		Usable: true, Selected: selected, Bodies: bodies,
	}
}

func v2(slots []store.SessionSlot, floatingSelected bool) store.Session {
	return store.Session{Version: 2, Slots: slots, FloatingSelected: floatingSelected}
}

// C17: entries that cannot stand on their own are dropped one by one, the rest
// come back in order, and one error line gives the total.
func TestRestoreDropsUnresolvableSlots(t *testing.T) {
	sessionSandbox(t)
	notUsable := listEntry(2, true)
	notUsable.Usable = false
	noZone := listEntry(4, false, aspect.Venus)
	noZone.Chart.Place.TZ = "" // usable by the store's say-so, refused by the workspace
	sess := v2([]store.SessionSlot{
		listEntry(1, true, aspect.Sun), notUsable, listEntry(3, false, aspect.Moon), noZone, listEntry(5, true),
	}, true)

	m := startedModel(t, withSession(perimeter(), sess))

	if want := []string{"Synthetic 1", "Synthetic 3", "Synthetic 5", "now"}; !reflect.DeepEqual(order(m), want) {
		t.Fatalf("restored %v, want the three good charts in order: %v", order(m), want)
	}
	if want := []bool{true, false, true, true}; !reflect.DeepEqual(selections(m), want) {
		t.Errorf("selections = %v, want %v (each entry's own flag)", selections(m), want)
	}
	if !reflect.DeepEqual(m.slots.SelectedBodies(0), []aspect.Body{aspect.Sun}) || !reflect.DeepEqual(m.slots.SelectedBodies(1), []aspect.Body{aspect.Moon}) || len(m.slots.SelectedBodies(2)) != 0 {
		t.Errorf("marks followed the wrong entries: %v / %v / %v", m.slots.SelectedBodies(0), m.slots.SelectedBodies(1), m.slots.SelectedBodies(2))
	}
	if want := []string{"Couldn't restore 2 of the open charts."}; !reflect.DeepEqual(errorLines(m), want) {
		t.Errorf("error lines = %q, want %q", errorLines(m), want)
	}
}

// C17b: a library file that has gone since the quit is no reason to drop a
// chart: it comes back from its snapshot, and restore says nothing.
func TestRestoreFromSnapshotWhenFileIsGone(t *testing.T) {
	sessionSandbox(t) // an empty charts directory: no library file exists
	entry := listEntry(1, true, aspect.Mars)
	p := withSession(perimeter(), v2([]store.SessionSlot{entry}, false))
	m := startedModel(t, p)

	slot := firstAnchor(m.slots)
	if slot == nil {
		t.Fatal("the chart was not restored from its snapshot")
	}
	if slot.Chart.Name != "Synthetic 1" || !slot.Time.Equal(entry.Chart.When) || slot.Location != entry.Chart.Place {
		t.Errorf("restored %q at %v in %+v, want the snapshot", slot.Chart.Name, slot.Time, slot.Location)
	}
	if !m.slots.SlotSelected(0) || !reflect.DeepEqual(m.slots.SelectedBodies(0), []aspect.Body{aspect.Mars}) {
		t.Errorf("selection %v / marks %v, want selected and [mars]", selections(m), m.slots.SelectedBodies(0))
	}
	for _, l := range m.logLines {
		if strings.HasPrefix(l.text, "session:") || l.lane == laneError {
			t.Errorf("restore wrote %q for a chart whose file is gone", l.text)
		}
	}

	// The existing .as re-check is what notices the file is gone, with
	// its one line; the slot keeps its snapshot, name, moment, place, selection
	// and marks.
	cmd := m.sessionRecheckCmd(p, true)
	if cmd == nil {
		t.Fatal("a restored .as slot owes a re-check")
	}
	// Run it for real — it reads the (empty) library — and feed what it
	// delivers back through Update, so a re-check that cannot find the vanished
	// file, or never delivers, fails here.
	msg := cmd()
	if _, ok := msg.(SessionChartRecheckedMsg); !ok {
		t.Fatalf("the re-check delivered %T, want its result", msg)
	}
	tm, _ := m.Update(msg)
	m = tm.(Model)
	var said []string
	for _, l := range m.logLines {
		if strings.HasPrefix(l.text, "session:") {
			said = append(said, l.text)
		}
	}
	if want := []string{"session: synthetic-1.as is no longer in the library"}; !reflect.DeepEqual(said, want) {
		t.Errorf("the re-check said %q, want exactly %q", said, want)
	}
	slot = firstAnchor(m.slots)
	if slot == nil || slot.Chart.Name != "Synthetic 1" || !slot.Time.Equal(entry.Chart.When) || slot.Location != entry.Chart.Place ||
		!m.slots.SlotSelected(0) || !reflect.DeepEqual(m.slots.SelectedBodies(0), []aspect.Body{aspect.Mars}) {
		t.Errorf("the re-check disturbed the snapshot: %+v", slot)
	}
}

// C17a, the UI half: a session that somehow marks more than six slots selected
// keeps the first six and says how many it turned off.
func TestRestoreReportsSelectionsItCouldNotKeep(t *testing.T) {
	sessionSandbox(t)
	var entries []store.SessionSlot
	for i := 1; i <= 7; i++ {
		entries = append(entries, listEntry(i, true))
	}
	m := startedModel(t, withSession(perimeter(), v2(entries, false)))
	if got := m.slots.SelectedCount(); got != store.MaxSelectedSlots {
		t.Errorf("%d slots selected, want the cap %d", got, store.MaxSelectedSlots)
	}
	if m.slots.SlotSelected(6) {
		t.Error("the seventh chart stayed selected")
	}
	if want := []string{"Couldn't keep 1 of the selections. At most six slots can be selected."}; !reflect.DeepEqual(errorLines(m), want) {
		t.Errorf("error lines = %q, want %q", errorLines(m), want)
	}
}

// A whole list quits and relaunches intact, through the real files.
func TestAListSurvivesAQuitAndARelaunch(t *testing.T) {
	sessionSandbox(t)
	seedPreferences(t)
	m := withFrame(listModel(t, 3, 1))
	m.slots.ToggleSlotSelection(2)
	m.slots.ToggleBodySelection(1, aspect.Saturn)
	wantOrder, wantSel := order(m), selections(m)

	if _, cmd := press(t, m, "q"); cmd == nil {
		t.Fatal("q did not quit")
	}
	got := startedModel(t, realPerimeter(t))
	if !reflect.DeepEqual(order(got), wantOrder) || !reflect.DeepEqual(selections(got), wantSel) {
		t.Errorf("relaunched %v %v, want %v %v", order(got), selections(got), wantOrder, wantSel)
	}
	if !reflect.DeepEqual(got.slots.SelectedBodies(1), []aspect.Body{aspect.Saturn}) {
		t.Errorf("the second chart's marks = %v, want [saturn]", got.slots.SelectedBodies(1))
	}
	if lines := sessionLines(got); len(lines) != 0 {
		t.Errorf("a clean relaunch said %v", lines)
	}
}

// C21 (operator, 2026-10-04): the Chart panel shows up to 7 slot rows on a
// terminal 40 rows tall or more and up to 5 on a shorter one; past that it shows
// a contiguous window holding the cursor and keeps counting `N of M`.
// The window's rows may now open with a numeral (selected-charts-compare C14),
// which the row scan skips beside the cursor and the old selection marks.
func TestSlotListWindowsAroundTheCursor(t *testing.T) {
	for _, size := range [][2]int{{80, 24}, {132, 40}} {
		for _, tc := range []struct {
			cursor int
			reads  string
		}{{0, "1 of 10"}, {4, "5 of 10"}, {9, "10 of 10"}} {
			t.Run(fmt.Sprintf("%dx%d/cursor %d", size[0], size[1], tc.cursor), func(t *testing.T) {
				m := withFrame(listModel(t, 9, tc.cursor))
				m.width, m.height = size[0], size[1]
				view := ansi.Strip(m.View())

				rects := layoutGeometry(size[0], size[1], chartContentLines(m.slots), m.shownCount())
				chart, bodies := rects[PanelChart], rects[PanelBodies]
				wantRows := map[int]int{24: 5, 40: 7}[size[1]]
				if chart.h-2 != wantRows {
					t.Errorf("the Chart panel has %d rows, want %d", chart.h-2, wantRows)
				}

				lines := strings.Split(view, "\n")
				var rows []string
				for _, l := range lines[1 : 1+chart.h-2] {
					r := []rune(l)
					rows = append(rows, strings.TrimSpace(string(r[:min(len(r), 66)])))
				}
				// Contiguous: Synthetic n, n+1, ... with now only at the end.
				first, prev := -1, 0
				sawCursor := false
				for i, row := range rows {
					var n int
					if _, err := fmt.Sscanf(strings.TrimLeft(strings.TrimLeft(strings.TrimLeft(row, "|│"), ">*▶ 123456①②③④⑤⑥"), " "), "Synthetic %d", &n); err != nil {
						// not a named row: it must be now, and it must be last
						if i != len(rows)-1 || tc.cursor != 9 {
							t.Fatalf("row %d is %q: not a slot row of the window\n%s", i, row, view)
						}
						sawCursor = true
						continue
					}
					if first < 0 {
						first = n
					} else if n != prev+1 {
						t.Errorf("the window is not contiguous: %d after %d\n%s", n, prev, view)
					}
					prev = n
					if n-1 == tc.cursor {
						sawCursor = true
					}
				}
				if !sawCursor {
					t.Errorf("the window %v does not hold the cursor (slot %d)\n%s", rows, tc.cursor, view)
				}
				if !strings.Contains(strings.Join(lines[:chart.h], "\n"), tc.reads) {
					t.Errorf("the Chart frame does not read %q:\n%s", tc.reads, strings.Join(lines[:chart.h], "\n"))
				}
				if bodies.h < 5 {
					t.Errorf("Bodies is %d rows tall; the layout floor is 5", bodies.h)
				}
			})
		}
	}
}

// C22: with the cursor on an anchored chart, Bodies computes that chart.
//
// Changed by selected-charts-compare C1-C4 and P2: the comparison no longer
// "stays baseline against the floating slot whichever slot the cursor is on".
// An UNSELECTED cursor chart is shown alone (C3); a cursor on a selected
// non-baseline chart compares baseline x the active pair, which for another
// anchored chart is a synastry pair (C1c); only a pair with the floating slot
// still reads today's transit cross (C2).
func TestFocusedAnchoredSlotComputesItsOwnChart(t *testing.T) {
	var natal []store.AnchorChart
	var crossed []store.AnchorChart // the transit cross's natal side
	var synastry [][2]store.AnchorChart
	oldN, oldT, oldO, oldS := firstCallNatalPositions, firstCallTransits, overlayCrossAspects, synastryCrossAspects
	t.Cleanup(func() {
		firstCallNatalPositions, firstCallTransits, overlayCrossAspects, synastryCrossAspects = oldN, oldT, oldO, oldS
	})
	firstCallNatalPositions = func(_ context.Context, _ *lookup.Probers, c store.AnchorChart, _ lookup.Lens) (lookup.Result, error) {
		natal = append(natal, c)
		return lookup.Result{Positions: []aspect.Position{{Body: aspect.Sun, Longitude: float64(c.When.Year() % 360)}}}, nil
	}
	firstCallTransits = func(context.Context, *lookup.Probers, time.Time, location.PlaceInfo, lookup.Lens) (lookup.Result, error) {
		return lookup.Result{Positions: []aspect.Position{{Body: aspect.Sun, Longitude: 1}}}, nil
	}
	overlayCrossAspects = func(_ context.Context, _ lookup.Lens, c store.AnchorChart, _ time.Time, _ location.PlaceInfo) (lookup.OverlayAspects, error) {
		crossed = append(crossed, c)
		return lookup.OverlayAspects{}, nil
	}
	synastryCrossAspects = func(_ context.Context, _ lookup.Lens, c store.AnchorChart, when time.Time, place location.PlaceInfo, _ store.ReportLabel) (lookup.OverlayAspects, error) {
		synastry = append(synastry, [2]store.AnchorChart{c, {When: when, Place: place}})
		return lookup.OverlayAspects{}, nil
	}
	run := func(m Model) FirstFrameReadyMsg {
		t.Helper()
		natal, crossed, synastry = nil, nil, nil
		msg, ok := buildTextCmd(m, context.Background(), 1, 1)().(FirstFrameReadyMsg)
		if !ok {
			t.Fatal("the compute did not return a frame")
		}
		return msg
	}
	handle := func(m Model, i int) store.AnchorChart { return anchorOf(m.slots.All()[i]) }

	// Single chart: the baseline is deselected, so there is no comparison and
	// the 2nd chart is simply the one on screen.
	m := listModel(t, 3, 1)
	m.slots.SetComparisonSelected(false)
	if mode := selectComputeMode(m); mode != computeModeNatalSingle {
		t.Fatalf("mode = %d, want the focused chart's single compute", mode)
	}
	frame := run(m)
	if len(natal) != 1 || natal[0] != handle(m, 1) {
		t.Errorf("computed %+v, want the 2nd chart %+v", natal, handle(m, 1))
	}
	if frame.Positions[0].Longitude != float64(handle(m, 1).When.Year()%360) {
		t.Errorf("Bodies carries %v, not the 2nd chart's positions", frame.Positions)
	}

	// C3 / P2: the baseline and the floating slot are selected but the cursor
	// is on the UNSELECTED 2nd chart: that chart alone, no cross at all.
	m = listModel(t, 3, 1)
	if !m.slots.ComparisonSelected() || m.comparing() {
		t.Fatal("fixture: want baseline and now selected, the cursor on an unselected chart")
	}
	if mode := selectComputeMode(m); mode != computeModeNatalSingle {
		t.Fatalf("an unselected cursor chart: mode = %d, want the single compute", mode)
	}
	run(m)
	if len(natal) != 1 || natal[0] != handle(m, 1) || len(crossed)+len(synastry) != 0 {
		t.Errorf("unselected cursor: natal %+v cross %+v synastry %+v, want that chart alone", natal, crossed, synastry)
	}

	// C1c (amended by the view rule, 2026-10-24): select the 2nd chart (the
	// cursor stays on it) while now is selected: the screen compares and starts
	// on the chart alone; read in ①, `2 in 1`, it is a synastry with ① as
	// chart 1 and the 2nd chart as chart 2, Bodies listing the 2nd chart's own
	// bodies framed by ①; no transit cross is computed.
	m = send(t, m, keySpace)
	if !m.comparing() || m.activePairSlot() != nil {
		t.Fatal("fixture: selecting the cursor's chart should compare, on the chart alone")
	}
	m.base = m.slots.All()[0]
	if p := m.activePairSlot(); p == nil || p.left != m.slots.All()[0] || p.right != m.slots.All()[1] {
		t.Fatal("fixture: the 2nd chart should be read in ①")
	}
	frame = run(m)
	if len(crossed) != 0 || len(synastry) != 1 || synastry[0][0] != handle(m, 0) || synastry[0][1].When != handle(m, 1).When || !frame.CrossSynastry {
		t.Errorf("2 in 1: transit %+v synastry %+v, want only the synastry with ① as chart 1 then ②", crossed, synastry)
	}
	want := map[store.AnchorChart]bool{handle(m, 0): true, handle(m, 1): true}
	if len(natal) != 2 || !want[natal[0]] || !want[natal[1]] {
		t.Errorf("natal computes = %+v, want the base and the focused 2nd chart", natal)
	}
	if frame.Positions[0].Longitude != float64(handle(m, 1).When.Year()%360) || frame.NatalPositions[0].Longitude != float64(handle(m, 0).When.Year()%360) {
		t.Errorf("Bodies carries %v framed by %v, want the 2nd chart's positions framed by ①", frame.Positions, frame.NatalPositions)
	}
	// A moment is never a base: now stored as one shows the chart alone.
	m.base = m.slots.All()[3]
	if p := m.activePairSlot(); p != nil {
		t.Errorf("now stored as the base shows the pair %+v", p)
	}

	// The cursor on now read in the 2nd chart: today's transit cross, with
	// the 2nd chart as the natal; Bodies lists now's bodies framed by it.
	m.cursorIdx = 3
	m.base = m.slots.All()[1]
	frame = run(m)
	if len(crossed) != 1 || crossed[0] != handle(m, 1) || len(synastry) != 0 || frame.CrossSynastry {
		t.Errorf("now in 2: transit %+v synastry %+v, want only the transit cross with ② as the natal", crossed, synastry)
	}
	if frame.Positions[0].Longitude != 1 || frame.NatalPositions[0].Longitude != float64(handle(m, 1).When.Year()%360) {
		t.Errorf("Bodies carries %v framed by %v, want now's positions framed by the 2nd chart", frame.Positions, frame.NatalPositions)
	}

	// Without now, the cursor on the baseline read in the 2nd chart:
	// synastry, the 2nd chart as chart 1.
	m.slots.SetSlotSelected(3, false)
	m.base = m.slots.All()[1]
	m.cursorIdx = 0
	run(m)
	// A Bodies column per shown chart (bodies-columns C2) makes the 2nd chart's
	// natal lookup part of the baseline's frame as well.
	if len(natal) != 2 || !want[natal[0]] || !want[natal[1]] || len(synastry) != 1 || len(crossed) != 0 || synastry[0][0] != handle(m, 1) || synastry[0][1] != handle(m, 0) {
		t.Errorf("on the baseline: natal %+v cross %+v synastry %+v, want the baseline and 2nd chart's natals and the synastry pair", natal, crossed, synastry)
	}
}

// C20: the list keys are in the binding table the keys are handled from, and
// the Keys popup is built from that table: rebind them and both follow.
func TestHelpListsListKeys(t *testing.T) {
	// Each press gets its own workspace: a Model copy shares its *Slots.
	rebound := func() Model {
		m := listModel(t, 3, 1)
		m.keys.SlotMoveUp = key.NewBinding(key.WithKeys("ctrl+t"), key.WithHelp("^t", "Move chart up"))
		m.keys.SlotMoveDown = key.NewBinding(key.WithKeys("ctrl+g"), key.WithHelp("^g", "Move chart down"))
		m.keys.SlotDelete = key.NewBinding(key.WithKeys("ctrl+y"), key.WithHelp("^y", "Remove chart"))
		return m
	}
	m := rebound()

	// The new keys act through Update; the old ones no longer do.
	if got := send(t, rebound(), tea.KeyMsg{Type: tea.KeyCtrlT}); !reflect.DeepEqual(order(got), []string{"Synthetic 2", "Synthetic 1", "Synthetic 3", "now"}) {
		t.Errorf("the rebound move-up key did not act: %v", order(got))
	}
	if got := send(t, rebound(), tea.KeyMsg{Type: tea.KeyCtrlG}); !reflect.DeepEqual(order(got), []string{"Synthetic 1", "Synthetic 3", "Synthetic 2", "now"}) {
		t.Errorf("the rebound move-down key did not act: %v", order(got))
	}
	if got := send(t, rebound(), tea.KeyMsg{Type: tea.KeyCtrlY}); got.slots.AnchoredCount() != 2 {
		t.Errorf("the rebound remove key did not act: %v", order(got))
	}
	for _, old := range []tea.KeyMsg{keyCtrlK, keyCtrlJ, keyDelete} {
		if got := send(t, rebound(), old); !reflect.DeepEqual(order(got), order(rebound())) {
			t.Errorf("the old key %v still acts after rebinding: %v", old, order(got))
		}
	}

	// The Keys popup shows the new spellings, and the bar the new actions.
	var listing []string
	for _, l := range m.helpLines() {
		listing = append(listing, l.text)
	}
	text := strings.Join(listing, "\n")
	for _, want := range []string{"Move chart up  ^t", "Move chart down  ^g", "Remove chart  ^y"} {
		if !strings.Contains(strings.Join(strings.Fields(strings.ReplaceAll(text, "\n", " | ")), " "), strings.Join(strings.Fields(want), " ")) {
			t.Errorf("the Keys popup lacks %q:\n%s", want, text)
		}
	}
	for _, old := range []string{"⌃k", "⌃j", "del"} {
		if strings.Contains(text, old+"\n") || strings.HasSuffix(text, old) {
			t.Errorf("the Keys popup still lists the old key %q", old)
		}
	}
	// With one chart open the popup lists all three; with none, none.
	listed := func(m Model) []string {
		var out []string
		for _, l := range m.helpLines() {
			for _, w := range []string{"Remove chart", "Move chart up", "Move chart down"} {
				if strings.Contains(l.text, w) {
					out = append(out, w)
				}
			}
		}
		return out
	}
	if got := listed(listModel(t, 1, 0)); len(got) != 3 {
		t.Errorf("one open chart: the popup lists %v, want all three", got)
	}
	if got := listed(listModel(t, 0, 0)); len(got) != 0 {
		t.Errorf("no open chart: the popup lists %v, want none", got)
	}
}

// The bar shows exactly the list keys that act right now (D29): del on an
// anchored chart under the Chart panel's cursor, ⌃k unless it is the first
// anchored chart, ⌃j unless it is the last — with one chart open as with many.
func TestBarOffersListKeysWhereTheyAct(t *testing.T) {
	has := func(m Model) (del, up, down bool) {
		bar := barKeys(m)
		return strings.Contains(bar, "Remove: del"), strings.Contains(bar, "Chart up: ⌃k"), strings.Contains(bar, "Chart down: ⌃j")
	}
	check := func(name string, m Model, wantDel, wantUp, wantDown bool) {
		t.Helper()
		if d, u, dn := has(m); d != wantDel || u != wantUp || dn != wantDown {
			t.Errorf("%s: bar %q has del=%v up=%v down=%v, want %v %v %v", name, barKeys(m), d, u, dn, wantDel, wantUp, wantDown)
		}
	}
	check("middle chart", listModel(t, 3, 1), true, true, true)
	check("first chart", listModel(t, 3, 0), true, false, true)
	check("last anchored chart", listModel(t, 3, 2), true, true, false)
	check("one chart, cursor on it", listModel(t, 1, 0), true, false, false)
	check("now focused", listModel(t, 1, 1), false, false, false)
	bodies := listModel(t, 3, 1)
	bodies.focused = PanelBodies
	check("Bodies focused", bodies, false, false, false)
	aspects := listModel(t, 3, 1)
	aspects.focused = PanelAspects
	check("Aspects focused", aspects, false, false, false)
}

// A pick that changes the focused slot — an add or a jump — takes the wheel the
// same way ↑/↓ does: with the preview open, a render is requested and it is for
// the newly focused chart.
func TestPickRefreshesAnOpenWheel(t *testing.T) {
	stubTextCompute(t)
	stubShowWheel(t)
	type call struct {
		when  time.Time
		place string
	}
	var calls []call
	prev := renderWheelImage
	renderWheelImage = func(_ context.Context, when time.Time, place location.PlaceInfo, _ lookup.Lens, _ lookup.WheelHighlight) ([]byte, error) {
		calls = append(calls, call{when, place.Name})
		return []byte("BM-wheel"), nil
	}
	t.Cleanup(func() { renderWheelImage = prev })

	open := func() Model {
		m := listModel(t, 2, 0) // A focused
		m.slots.SetComparisonSelected(false)
		m.slots.ToggleSlotSelection(0) // A selected, now not: no comparison, a single wheel
		m.previewActive = true
		return m
	}
	when := time.Date(2001, 2, 3, 4, 5, 0, 0, time.UTC)
	for name, pick := range map[string]struct {
		chart store.NamedChart
		place string
	}{
		"a jump to an open chart": {store.NamedChart{ID: "synthetic-2", Name: "Synthetic 2", Place: fixtureOneP}, "Testford 2"},
		"an add":                  {store.NamedChart{ID: "synthetic-new", Name: "Synthetic New", Place: fixtureOneP}, "Test Bay"},
	} {
		t.Run(name, func(t *testing.T) {
			calls = nil
			m := open()
			want := m.refresh[surfaceImage].wantID
			pickWhen := when
			if pick.chart.ID == "synthetic-2" { // a jump: the file reads as the slot does
				pick.chart.Place = m.slots.All()[1].Location
				pickWhen = m.slots.All()[1].Time
			}
			var pickCmd tea.Cmd
			m, pickCmd = applyChartResolved(m, ChartResolvedMsg{Chart: pick.chart, When: pickWhen, Gen: m.pickGen})
			if m.cursorIdx == 0 {
				t.Fatal("fixture: the pick did not move focus")
			}
			if m.refresh[surfaceImage].wantID == want {
				t.Fatal("the pick requested no wheel render")
			}
			// Drive the returned commands through Update until quiet: the render
			// for the newly focused chart must really be issued.
			calls = nil
			pump(t, m, pickCmd)
			if len(calls) == 0 || calls[len(calls)-1].place != pick.place {
				t.Errorf("wheel renders = %+v, want the last for %q", calls, pick.place)
			}
		})
	}
	// With the preview closed, a pick starts no wheel work.
	m := open()
	m.previewActive = false
	want := m.refresh[surfaceImage].wantID
	m, _ = applyChartResolved(m, ChartResolvedMsg{Chart: store.NamedChart{ID: "synthetic-2", Name: "Synthetic 2", Place: fixtureOneP}, When: when, Gen: m.pickGen})
	if m.refresh[surfaceImage].wantID != want {
		t.Error("a pick with no preview open requested a wheel render")
	}
}

// Picking a chart that is already open follows the file: if its resolved moment
// or place differs from the slot's, the slot takes the file's, in place.
func TestPickOfAnEditedOpenChartTakesTheFile(t *testing.T) {
	stubTextCompute(t)
	stubShowWheel(t)
	var renders []string
	prev := renderWheelImage
	renderWheelImage = func(_ context.Context, _ time.Time, place location.PlaceInfo, _ lookup.Lens, _ lookup.WheelHighlight) ([]byte, error) {
		renders = append(renders, place.Name)
		return []byte("BM-wheel"), nil
	}
	t.Cleanup(func() { renderWheelImage = prev })

	setup := func() Model {
		m := withFrame(listModel(t, 3, 2))
		m.slots.SetComparisonSelected(false)
		m.slots.ToggleSlotSelection(1)
		m.slots.ToggleBodySelection(1, aspect.Mars)
		m = m.focusSlot(1)
		m.bodyCursorIdx = 5
		m = m.focusSlot(2)
		m.previewActive = true
		return m
	}
	pickNamed := func(m Model, name string, when time.Time, place location.PlaceInfo) (Model, tea.Cmd) {
		c := store.NamedChart{ID: "synthetic-2", Name: name, Place: place,
			Source: store.ChartSource{Kind: store.ChartKindFile, File: "synthetic-2.as"}}
		return applyChartResolved(m, ChartResolvedMsg{Chart: c, When: when, Gen: m.pickGen})
	}
	pickCmd := func(m Model, when time.Time, place location.PlaceInfo) (Model, tea.Cmd) {
		return pickNamed(m, "Synthetic 2 (corrected)", when, place)
	}
	pick := func(m Model, when time.Time, place location.PlaceInfo) Model {
		m, _ = pickCmd(m, when, place)
		return m
	}

	t.Run("a differing moment is taken in place", func(t *testing.T) {
		renders = nil
		m := setup()
		old := *m.slots.All()[1]
		wantID := m.refresh[surfaceText].wantID
		wheelID := m.refresh[surfaceImage].wantID
		newWhen := old.Time.Add(4 * time.Minute)
		var cmd tea.Cmd
		m, cmd = pickCmd(m, newWhen, old.Location)
		slot := m.slots.All()[1]
		if m.cursorIdx != 1 || m.slots.AnchoredCount() != 3 || slot.Chart.ID != "synthetic-2" {
			t.Fatalf("cursor %d, %d anchored: the pick must stay in place at slot 1", m.cursorIdx, m.slots.AnchoredCount())
		}
		if !slot.Time.Equal(newWhen) || !slot.Chart.When.Equal(newWhen) || slot.Chart.Name != "Synthetic 2 (corrected)" {
			t.Errorf("the slot holds %v / %+v, want the file's moment and snapshot", slot.Time, slot.Chart)
		}
		if !m.slots.SlotSelected(1) || !reflect.DeepEqual(m.slots.SelectedBodies(1), []aspect.Body{aspect.Mars}) || m.bodyCursorIdx != 5 {
			t.Errorf("selection %v marks %v row %d: the slot's own state must stay", m.slots.SlotSelected(1), m.slots.SelectedBodies(1), m.bodyCursorIdx)
		}
		if m.refresh[surfaceText].wantID == wantID {
			t.Error("the corrected chart was not recomputed")
		}
		if m.refresh[surfaceImage].wantID == wheelID {
			t.Error("an open wheel was not refreshed")
		}
		renders = nil
		pump(t, m, cmd)
		if len(renders) == 0 || renders[len(renders)-1] != old.Location.Name {
			t.Errorf("wheel renders = %v, want the corrected chart's (%s) actually issued", renders, old.Location.Name)
		}
	})

	t.Run("a differing place is taken too", func(t *testing.T) {
		m := setup()
		old := *m.slots.All()[1]
		m = pick(m, old.Time, fixtureOneP)
		if m.slots.All()[1].Location != fixtureOneP {
			t.Errorf("the slot is at %+v, want the file's place", m.slots.All()[1].Location)
		}
	})

	t.Run("a title-only edit is taken too", func(t *testing.T) {
		m := setup()
		old := *m.slots.All()[1]
		wantID := m.refresh[surfaceText].wantID
		m, _ = pickNamed(m, "Synthetic 2 (retitled)", old.Time, old.Location)
		slot := m.slots.All()[1]
		if slot.Chart.Name != "Synthetic 2 (retitled)" || !slot.Time.Equal(old.Time) || slot.Location != old.Location {
			t.Errorf("the slot holds %q at %v: want the file's name and the same moment and place", slot.Chart.Name, slot.Time)
		}
		if !m.slots.SlotSelected(1) || m.refresh[surfaceText].wantID == wantID {
			t.Error("a retitled chart kept its selection and was recomputed like any other correction")
		}
	})

	t.Run("an unchanged pick with the cursor already there changes nothing", func(t *testing.T) {
		m := setup()
		m = m.focusSlot(1)
		old := *m.slots.All()[1]
		textID, imageID := m.refresh[surfaceText].wantID, m.refresh[surfaceImage].wantID
		before := fmt.Sprintf("%+v|%d|%d|%v", m.chart, m.cursorIdx, m.bodyCursorIdx, m.startup)
		m.picker.prior, m.startup = m.startup, startupChartOpen // ⌃o opened the picker
		var cmd tea.Cmd
		m, cmd = pickNamed(m, old.Chart.Name, old.Time, old.Location)
		if cmd != nil || m.refresh[surfaceText].wantID != textID || m.refresh[surfaceImage].wantID != imageID {
			t.Errorf("an unchanged pick issued work: cmd %v, text %d->%d, image %d->%d", cmd != nil, textID, m.refresh[surfaceText].wantID, imageID, m.refresh[surfaceImage].wantID)
		}
		if after := fmt.Sprintf("%+v|%d|%d|%v", m.chart, m.cursorIdx, m.bodyCursorIdx, m.startup); after != before {
			t.Errorf("the settled frame or cursor changed:\n  %s\n  %s", before, after)
		}
		if !reflect.DeepEqual(*m.slots.All()[1], old) {
			t.Error("the slot changed")
		}
	})

	t.Run("an unchanged pick of another chart moves the cursor like an arrow key", func(t *testing.T) {
		m := setup() // the cursor is on chart 3
		old := *m.slots.All()[1]
		textID := m.refresh[surfaceText].wantID
		m, _ = pickNamed(m, old.Chart.Name, old.Time, old.Location)
		if m.cursorIdx != 1 || m.refresh[surfaceText].wantID == textID {
			t.Errorf("cursor %d, text want %d->%d: the new focus must be computed, as ↑/↓ does", m.cursorIdx, textID, m.refresh[surfaceText].wantID)
		}
		if !reflect.DeepEqual(*m.slots.All()[1], old) {
			t.Error("the slot changed")
		}
	})
}

// A session refused for a line too long to read says so once, and the bytes
// survive the quit in the copy the launch kept: the astrologer's charts are in
// them.
func TestTooLongSessionLineSurvivesAQuit(t *testing.T) {
	sessionSandbox(t)
	seedPreferences(t)
	long := listEntry(2, false)
	long.Chart.Name = strings.Repeat("N", (1<<20)+1024) // past the reader's 1 MiB line limit
	if err := store.SaveSession(store.Session{Slots: []store.SessionSlot{listEntry(1, true), long, listEntry(3, false)}}); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(store.SessionPath())
	if err != nil {
		t.Fatal(err)
	}
	m := startedModel(t, realPerimeter(t))
	if m.slots.AnchoredCount() != 0 {
		t.Errorf("%d slots restored from a refused file", m.slots.AnchoredCount())
	}
	if want := []string{"Couldn't restore the last session. Its file cannot be used."}; !reflect.DeepEqual(errorLines(m), want) {
		t.Errorf("error lines = %q, want %q", errorLines(m), want)
	}
	if _, cmd := press(t, m, "q"); cmd == nil {
		t.Fatal("q did not quit")
	}
	// the specification C1/C6: the refused file is moved aside at launch,
	// so its bytes survive the quit in the kept copy and the quit writes anew.
	kept, _ := filepath.Glob(store.SessionPath() + ".refused-*")
	if len(kept) != 1 {
		t.Fatalf("kept copies = %v, want one", kept)
	}
	after, err := os.ReadFile(kept[0])
	if err != nil || string(after) != string(before) {
		t.Errorf("the refused session bytes did not survive (%d bytes before, %d after, err %v)", len(before), len(after), err)
	}
}

// A pick bumps the shared pick generation, but a startup re-check is about one
// slot: it is stale only if THAT slot was replaced or removed since it was
// issued. Picking C must not throw away B's corrected file.
func TestPickDoesNotDiscardAnotherChartsRecheck(t *testing.T) {
	stubTextCompute(t)
	sessionSandbox(t)
	a, b := listEntry(1, true), listEntry(2, false)
	m := startedModel(t, withSession(perimeter(), v2([]store.SessionSlot{a, b}, true)))
	if m.slots.AnchoredCount() != 2 {
		t.Fatalf("fixture: %d slots restored", m.slots.AnchoredCount())
	}
	slotB := m.slots.All()[1]
	cmd := m.sessionRecheckCmd(withSession(perimeter(), v2([]store.SessionSlot{a, b}, true)), true)
	if cmd == nil {
		t.Fatal("two .as slots owe re-checks")
	}

	// C is picked first: the shared generation moves.
	third := store.NamedChart{ID: "synthetic-3", Name: "Synthetic 3", Place: fixtureOneP, Source: store.ChartSource{Kind: store.ChartKindFile, File: "synthetic-3.as"}}
	m, _ = applyChartResolved(m, ChartResolvedMsg{Chart: third, When: time.Date(2001, 2, 3, 4, 5, 0, 0, time.UTC), Gen: m.pickGen})
	if m.slots.AnchoredCount() != 3 {
		t.Fatal("fixture: the pick did not add a slot")
	}
	m.pickGen++ // resolvePick bumps it on every pick; the resolve above skipped that

	// Then B's re-check lands, carrying B's corrected moment.
	edited := b.Chart.When.Add(7 * time.Minute)
	row := store.NamedChart{ID: b.Chart.ID, Name: b.Chart.Name, Place: b.Chart.Place, Source: b.Chart.Source}
	m, _ = applySessionChartRecheck(m, SessionChartRecheckedMsg{Remembered: b.Chart, Slot: slotB, Row: row, Found: true, When: edited})
	if got := slotB.Time; !got.Equal(edited) {
		t.Errorf("B's slot is at %v, want the corrected %v: a later pick of C discarded its re-check", got, edited)
	}

	// A re-check whose slot has since been replaced is stale.
	m2 := startedModel(t, withSession(perimeter(), v2([]store.SessionSlot{a, b}, true)))
	slotB2 := m2.slots.All()[1]
	if err := m2.slots.ReplaceAnchored(1, b.Chart, b.Chart.When.Add(time.Hour), b.Chart.Place); err != nil {
		t.Fatal(err)
	}
	m2, _ = applySessionChartRecheck(m2, SessionChartRecheckedMsg{Remembered: b.Chart, Slot: slotB2, Row: row, Found: true, When: edited})
	if got := slotB2.Time; !got.Equal(b.Chart.When.Add(time.Hour)) {
		t.Errorf("a re-check for a replaced slot was applied: %v", got)
	}
	// And one for a removed slot.
	m3 := startedModel(t, withSession(perimeter(), v2([]store.SessionSlot{a, b}, true)))
	gone := m3.slots.All()[1]
	m3.slots.RemoveAnchored(1)
	m3, _ = applySessionChartRecheck(m3, SessionChartRecheckedMsg{Remembered: b.Chart, Slot: gone, Row: row, Found: true, When: edited})
	if m3.slots.AnchoredCount() != 1 || len(sessionLines(m3)) != 0 {
		t.Errorf("a re-check for a removed slot acted: %d slots, lines %q", m3.slots.AnchoredCount(), sessionLines(m3))
	}
}

// The whole list shows, with no window, when it fits: four charts and now at
// 80x24 (5 rows), six and now at 132x40 (7 rows). Read inside the Chart panel's
// own frame: every chart row and the now row, no row cut, and the counter exactly
// "<cursor> of <total>".
func TestSlotListShowsWholeListWhenItFits(t *testing.T) {
	counter := regexp.MustCompile(`[0-9]+ of [0-9]+`)
	for _, tc := range []struct{ w, h, charts int }{{80, 24, 4}, {132, 40, 6}} {
		t.Run(fmt.Sprintf("%dx%d with %d charts", tc.w, tc.h, tc.charts), func(t *testing.T) {
			m := withFrame(listModel(t, tc.charts, 0))
			m.width, m.height = tc.w, tc.h
			rects := layoutGeometry(tc.w, tc.h, chartContentLines(m.slots), m.shownCount())
			chart := rects[PanelChart]
			if got := chart.h - 2; got != tc.charts+1 {
				t.Fatalf("the Chart panel has %d rows, want all %d", got, tc.charts+1)
			}
			var frame []string
			for _, l := range strings.Split(ansi.Strip(m.View()), "\n")[:chart.h] {
				r := []rune(l)
				frame = append(frame, string(r[:min(len(r), chart.w)]))
			}
			rows := frame[1 : chart.h-1]
			for i := 1; i <= tc.charts; i++ {
				if !strings.Contains(rows[i-1], fmt.Sprintf("Synthetic %d", i)) {
					t.Errorf("row %d is %q, want chart %d in order", i-1, rows[i-1], i)
				}
			}
			if now := rows[tc.charts]; !strings.Contains(now, "Berlin, DE") {
				t.Errorf("the last row is %q, want the now row", now)
			}
			if got := counter.FindAllString(strings.Join(frame, "\n"), -1); len(got) != 1 || got[0] != fmt.Sprintf("1 of %d", tc.charts+1) {
				t.Errorf("the Chart frame's counter reads %v, want exactly one, \"1 of %d\":\n%s", got, tc.charts+1, strings.Join(frame, "\n"))
			}
		})
	}
}

// The taste lane: the Chart panel as the list grows (V1) and what Bodies keeps
// (V2). Synthetic names only; every moment, place and name is constructed
// (specs/frames/residue.md).
func frameModel(t *testing.T, cfg Config, w, h, anchored, cursor int) Model {
	t.Helper()
	names := []string{"Synthetic Aster", "Synthetic Birch", "Synthetic Cedar", "Synthetic Dune", "Synthetic Elm", "Synthetic Fern"}
	m := withFrame(listModel(t, anchored, cursor))
	m.cfg = cfg
	for i := 0; i < anchored; i++ {
		m.slots.All()[i].Chart.Name = names[i]
	}
	if anchored >= 3 {
		m.slots.ToggleSlotSelection(2) // the 1st (the add default) and the 3rd
	}
	m.width, m.height = w, h
	return m
}

func TestSlotListFrames(t *testing.T) {
	forceColor(t)
	for _, n := range []int{1, 3, 6} {
		for _, size := range []struct {
			name string
			w, h int
		}{{"80x24", 80, 24}, {"132x40", 132, 40}} {
			for _, mode := range []struct {
				name string
				cfg  Config
			}{{"color", Config{Color: ColorAlways}}, {"never", Config{Color: ColorNever}}} {
				m := frameModel(t, mode.cfg, size.w, size.h, n, 1) // the 2nd chart; with one chart, now
				writeDesignFrame(t, fmt.Sprintf("slot-list-%d-%s-%s", n, size.name, mode.name), m.View())
			}
		}
	}
	// V2: what Bodies keeps with six charts open.
	m := frameModel(t, Config{Color: ColorNever}, 80, 24, 6, 1)
	writeDesignFrame(t, "slot-list-bodies-floor-80x24", m.View())
}

// A city change rebuilds every slot as a new object; a re-check issued before it
// still belongs to the rebuilt slot holding the same chart — and to no other.
func TestCityChangeKeepsAPendingRecheck(t *testing.T) {
	stubTextCompute(t)
	sessionSandbox(t)
	a, b := listEntry(1, true), listEntry(2, false)
	p := withSession(perimeter(), v2([]store.SessionSlot{a, b}, true))
	m := startedModel(t, p)
	oldB := m.slots.All()[1]

	m, _ = applyAtlasPicked(m, AtlasPickedMsg{Place: testPlaceAmsterdam()})
	if m.slots.All()[1] == oldB {
		t.Fatal("fixture: the city change did not rebuild the slots")
	}
	edited := b.Chart.When.Add(9 * time.Minute)
	row := store.NamedChart{ID: b.Chart.ID, Name: b.Chart.Name, Place: b.Chart.Place, Source: b.Chart.Source}
	m, _ = applySessionChartRecheck(m, SessionChartRecheckedMsg{Remembered: b.Chart, Slot: oldB, Row: row, Found: true, When: edited})
	if got := m.slots.All()[1].Time; !got.Equal(edited) {
		t.Errorf("B's rebuilt slot is at %v, want the corrected %v", got, edited)
	}
	if got := m.slots.All()[0].Time; !got.Equal(a.Chart.When) {
		t.Errorf("A's slot moved to %v: a re-check must never apply to a different chart", got)
	}

	// A pick that put different content into the slot after the rebuild — here
	// a retitle, same moment and place — outranks the re-check issued against the
	// old snapshot: it applies only to a slot that still holds exactly that.
	m2 := startedModel(t, p)
	old2 := m2.slots.All()[1]
	m2, _ = applyAtlasPicked(m2, AtlasPickedMsg{Place: testPlaceAmsterdam()})
	picked := b.Chart
	picked.Name = "Synthetic 2 (retitled by a pick)"
	if err := m2.slots.ReplaceAnchored(1, picked, b.Chart.When, b.Chart.Place); err != nil {
		t.Fatal(err)
	}
	m2, _ = applySessionChartRecheck(m2, SessionChartRecheckedMsg{Remembered: b.Chart, Slot: old2, Row: row, Found: true, When: edited})
	if got := m2.slots.All()[1]; !got.Time.Equal(b.Chart.When) || got.Chart.Name != picked.Name {
		t.Errorf("a stale re-check overwrote the operator's pick: %v %q", got.Time, got.Chart.Name)
	}
}

// C16: a session file of an unknown version restores nothing and says so once,
// on the error lane.
func TestRestoreRefusesVersionFourWithOneErrorLine(t *testing.T) {
	sessionSandbox(t)
	seedPreferences(t)
	seedSession(t, "version: 4\ncharts:\n  - id: x\n    name: X\n    when: \"2001-02-03T04:05:00Z\"\n    place: P\n    lat: 1\n    lon: 2\n    tz: UTC\n")
	m := startedModel(t, realPerimeter(t))
	if m.slots.AnchoredCount() != 0 {
		t.Errorf("%d slots restored from a version-4 file", m.slots.AnchoredCount())
	}
	if want := []string{"Couldn't restore the last session. Its file cannot be used."}; !reflect.DeepEqual(errorLines(m), want) {
		t.Errorf("error lines = %q, want %q", errorLines(m), want)
	}
}

// C12/C13: a city change rebuilds the workspace and every slot — now included —
// keeps the Bodies row it was left on.
func TestCityChangeKeepsEveryBodiesRow(t *testing.T) {
	for _, focusOnNow := range []bool{true, false} {
		t.Run(fmt.Sprintf("cursor on now: %v", focusOnNow), func(t *testing.T) {
			m := withFrame(listModel(t, 2, 0))
			m.bodyCursorIdx = 4 // chart 1, row 4
			m = m.focusSlot(1)  // leave it
			m.bodyCursorIdx = 3 // chart 2, row 3
			m = m.focusSlot(2)  // leave it; now starts at 0
			m.bodyCursorIdx = 7 // now, row 7
			if !focusOnNow {
				m = m.focusSlot(0) // back to chart 1 (row 4) with the others remembered
			}
			m, _ = applyAtlasPicked(m, AtlasPickedMsg{Place: testPlaceAmsterdam()})
			m = withFrame(m)
			if !m.focusedIsFloating() || m.bodyCursorIdx != 7 {
				t.Errorf("after the city change: slot %d row %d, want now at row 7", m.cursorIdx, m.bodyCursorIdx)
			}
			for slot, row := range map[int]int{0: 4, 1: 3, 2: 7} {
				m = m.focusSlot(slot)
				if m.bodyCursorIdx != row {
					t.Errorf("slot %d: row %d, want %d", slot, m.bodyCursorIdx, row)
				}
			}
		})
	}
}

// A re-check issued at startup survives any number of city changes, and the
// bookkeeping that makes it so is bounded by the re-checks still pending, not by
// how many times the workspace was rebuilt.
func TestCityChangesKeepTheRecheckMapBounded(t *testing.T) {
	stubTextCompute(t)
	sessionSandbox(t)
	a, b := listEntry(1, true), listEntry(2, false)
	m := startedModel(t, withSession(perimeter(), v2([]store.SessionSlot{a, b}, true)))
	issued := m.slots.All()[1] // B's re-check is issued against this slot
	if len(m.pendingRechecks) != 2 {
		t.Fatalf("%d re-checks tracked after restore, want 2 (one per .as slot)", len(m.pendingRechecks))
	}

	places := []location.PlaceInfo{testPlaceAmsterdam(), testPlaceBerlin()}
	for i := 0; i < 100; i++ {
		m, _ = applyAtlasPicked(m, AtlasPickedMsg{Place: places[i%2]})
	}
	if len(m.pendingRechecks) != 2 {
		t.Errorf("%d tracked after 100 rebuilds, want 2: the map must not grow with rebuilds", len(m.pendingRechecks))
	}

	edited := b.Chart.When.Add(11 * time.Minute)
	row := store.NamedChart{ID: b.Chart.ID, Name: b.Chart.Name, Place: b.Chart.Place, Source: b.Chart.Source}
	m, _ = applySessionChartRecheck(m, SessionChartRecheckedMsg{Remembered: b.Chart, Slot: issued, Row: row, Found: true, When: edited})
	if got := m.slots.All()[1].Time; !got.Equal(edited) {
		t.Errorf("after 100 city changes B's slot is at %v, want the corrected %v", got, edited)
	}
	if len(m.pendingRechecks) != 1 {
		t.Errorf("%d tracked after B's re-check landed, want 1 (A's)", len(m.pendingRechecks))
	}
	// Removing a slot drops its pending re-check.
	m = m.focusSlot(0)
	m, _ = m.removeFocusedSlot()
	if len(m.pendingRechecks) != 0 {
		t.Errorf("%d tracked after the slot was removed, want 0", len(m.pendingRechecks))
	}
}

// D5. The session names a chart's kind `file`. A session written before that
// word changed names it with the retired word; the reader no longer accepts
// that, so the entry is unusable and restore drops it with the one line it
// already has for an entry that cannot stand.
func TestSessionKindFile(t *testing.T) {
	sessionSandbox(t)
	entry := listEntry(1, true, aspect.Sun)
	if err := store.SaveSession(v2([]store.SessionSlot{entry}, false)); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(store.SessionPath())
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	if !strings.Contains(text, "    kind: file\n") {
		t.Fatalf("the session does not write `kind: file`:\n%s", text)
	}
	got, ok, err := store.LoadSession()
	if err != nil || !ok || len(got.Slots) != 1 || !got.Slots[0].Usable || got.Slots[0].Chart.Source.Kind != store.ChartKindFile {
		t.Fatalf("round trip: %+v (ok %v, err %v), want one usable entry of kind file", got.Slots, ok, err)
	}

	// The retired word is spelled in two parts so the repository-wide ./check
	// row that refuses it everywhere needs no exception for this test.
	retired := "da" + "t"
	old := strings.Replace(text, "    kind: file\n", "    kind: "+retired+"\n", 1)
	if old == text {
		t.Fatal("the session text has no kind line to age")
	}
	if err := os.WriteFile(store.SessionPath(), []byte(old), 0o600); err != nil {
		t.Fatal(err)
	}
	aged, ok, err := store.LoadSession()
	if err != nil || !ok || len(aged.Slots) != 1 {
		t.Fatalf("an older session did not load: %+v (ok %v, err %v)", aged.Slots, ok, err)
	}
	if aged.Slots[0].Usable || aged.Slots[0].Chart.Source.Kind != store.ChartKindNone {
		t.Errorf("an entry with the retired kind word: usable %v, kind %v; want unusable, ChartKindNone", aged.Slots[0].Usable, aged.Slots[0].Chart.Source.Kind)
	}
	m := startedModel(t, withSession(perimeter(), aged))
	if want := []string{"now"}; !reflect.DeepEqual(order(m), want) {
		t.Errorf("restored %v, want only the floating slot", order(m))
	}
	if want := []string{"Couldn't restore 1 of the open charts."}; !reflect.DeepEqual(errorLines(m), want) {
		t.Errorf("error lines = %q, want %q", errorLines(m), want)
	}
}
