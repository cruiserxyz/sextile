// All chart values reachable from this file are constructed synthetic test inputs.
package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"time"

	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/store"
)

// saveBase is a ready model with the FLOATING slot focused (cursorIdx 1 over
// slots.All() = [anchored, floating]) and an isolated charts directory, so a
// save in these tests can never reach the operator's library.
func saveBase(t *testing.T) Model {
	t.Helper()
	t.Setenv("SEXTILE_CHARTS_DIR", t.TempDir())
	m := arrowBase(t)
	m.cursorIdx = 1
	return m
}

// chartsDirOf returns the isolated directory saveBase pointed the model at.
func chartsDirOf(t *testing.T) string {
	t.Helper()
	return os.Getenv("SEXTILE_CHARTS_DIR")
}

// runSave drives one full save: open, type name, Enter, run the returned
// command, and fold its message back in. It returns the settled model.
func runSave(t *testing.T, m Model, name string) Model {
	t.Helper()
	m = openSave(t, m)
	m = typeInto(m, name)
	tm, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = tm.(Model)
	if cmd == nil {
		t.Fatal("Enter on a filled save prompt returned no command")
	}
	msg := cmd()
	done, ok := msg.(SaveChartDoneMsg)
	if !ok {
		t.Fatalf("the save command returned %T, want SaveChartDoneMsg", msg)
	}
	tm2, _ := m.Update(done)
	return tm2.(Model)
}

func openSave(t *testing.T, m Model) Model {
	t.Helper()
	tm, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	return tm.(Model)
}

func typeInto(m Model, s string) Model {
	for _, r := range s {
		var tm tea.Model = m
		if r == ' ' {
			tm, _ = tm.Update(tea.KeyMsg{Type: tea.KeySpace})
		} else {
			tm, _ = tm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		}
		m = tm.(Model)
	}
	return m
}

// C1: while the floating slot is focused, ⌃s or the leader sequence `s` opens
// the save prompt with an empty name.
//
// BOTH bindings, in one test, because the leader exists precisely for the
// terminal that eats ⌃s for flow control — a slice that wired only one of them
// would leave that operator with no way in at all.
func TestSavePromptOpensOnBothBindings(t *testing.T) {
	t.Run("ctrl+s", func(t *testing.T) {
		m := openSave(t, saveBase(t))
		if m.startup != startupSaveOpen {
			t.Fatalf("startup = %q, want %q", m.startup, startupSaveOpen)
		}
		if m.save.typedName() != "" {
			t.Errorf("the prompt opened with name %q, want it empty", m.save.typedName())
		}
	})
	t.Run("leader s", func(t *testing.T) {
		m := saveBase(t)
		m, _ = pressKey(t, m, ',')
		m, _ = pressKey(t, m, 's')
		if m.startup != startupSaveOpen {
			t.Fatalf("startup = %q, want %q", m.startup, startupSaveOpen)
		}
		if m.save.typedName() != "" {
			t.Errorf("the prompt opened with name %q, want it empty", m.save.typedName())
		}
	})
}

// C1a: a confirmed name that is written closes the prompt and logs one line
// naming the saved chart.
func TestSavePromptClosesOnASuccessfulSave(t *testing.T) {
	m := saveBase(t)
	before := len(m.logLines)
	m = runSave(t, m, "Test Person")

	if m.startup == startupSaveOpen {
		t.Fatalf("the prompt is still open after a successful save")
	}
	if m.save.typedName() != "" {
		t.Errorf("save state survived the close: name = %q", m.save.typedName())
	}
	added := m.logLines[before:]
	if len(added) != 1 {
		t.Fatalf("a successful save logged %d lines, want exactly 1: %+v", len(added), added)
	}
	if added[0].lane != laneNote {
		t.Errorf("the confirmation is on lane %v, want the note lane: nothing failed", added[0].lane)
	}
	if !strings.Contains(added[0].text, "Test Person") {
		t.Errorf("the confirmation %q does not name the saved chart", added[0].text)
	}
	// ...and the chart really is there.
	if _, err := os.Stat(filepath.Join(chartsDirOf(t), "test-person.as")); err != nil {
		t.Errorf("the save reported success but wrote no file: %v", err)
	}
}

// C2: while any other slot is focused, ⌃s and the leader sequence do not open
// the prompt.
func TestSaveRefusesWhenFloatingNotFocused(t *testing.T) {
	m := saveBase(t)
	m.cursorIdx = 0 // the ANCHORED slot
	if m.slots.All()[0].Floating {
		t.Fatal("precondition: slot 0 is the floating one; this test needs the anchored slot")
	}

	got := openSave(t, m)
	if got.startup == startupSaveOpen {
		t.Error("⌃s opened the save prompt with the anchored slot focused")
	}
	m2 := m
	m2, _ = pressKey(t, m2, ',')
	m2, _ = pressKey(t, m2, 's')
	if m2.startup == startupSaveOpen {
		t.Error("the leader sequence opened the save prompt with the anchored slot focused")
	}
}

// C2a, revised by the specification Part B: on the anchored slot the
// key does nothing and says nothing. Saving acts on the floating slot, and a
// key that cannot act is not explained in the Log.
func TestSaveRefusalIsQuiet(t *testing.T) {
	m := saveBase(t)
	m.cursorIdx = 0
	before := len(m.logLines)

	m = openSave(t, m)
	if added := m.logLines[before:]; len(added) != 0 {
		t.Errorf("the refused save wrote %+v", added)
	}
}

// C6a: on a refusal the prompt stays open with the typed name and shows what
// it collided with.
func TestSavePromptStaysOpenOnRefusal(t *testing.T) {
	m := saveBase(t)
	// Occupy the slug first.
	m = runSave(t, m, "Test Person")
	if m.startup == startupSaveOpen {
		t.Fatal("precondition: the first save did not close the prompt")
	}

	m = runSave(t, m, "Test  Person") // same slug, different text
	if m.startup != startupSaveOpen {
		t.Fatalf("startup = %q after a refused save, want the prompt still open", m.startup)
	}
	if m.save.typedName() != "Test  Person" {
		t.Errorf("the typed name became %q, want it kept as typed", m.save.typedName())
	}
	if m.save.refusal == "" {
		t.Fatal("the prompt shows no refusal")
	}
	if !strings.Contains(m.save.refusal, "test-person") {
		t.Errorf("the refusal %q does not name what it collided with", m.save.refusal)
	}
	// And the frame actually shows it.
	if !strings.Contains(m.View(), "is taken") {
		t.Error("the rendered prompt does not show the refusal")
	}
}

// C10: a successful save appends no argv line, and this test asserts IN THE
// SAME RUN that the save succeeded — otherwise it would pass against a save
// that did nothing at all, which is the shape the spec's question 2 is about.
func TestSaveCommandStaysOutOfTheConsole(t *testing.T) {
	m := saveBase(t)
	argvBefore := countArgvLines(m)

	m = runSave(t, m, "Test Person")

	// The save SUCCEEDED...
	if _, err := os.Stat(filepath.Join(chartsDirOf(t), "test-person.as")); err != nil {
		t.Fatalf("the save did not happen, so the argv assertion below proves nothing: %v", err)
	}
	if m.startup == startupSaveOpen {
		t.Fatal("the save did not complete, so the argv assertion below proves nothing")
	}
	// ...and it carried no argv into the lane the console copies.
	if got := countArgvLines(m); got != argvBefore {
		t.Errorf("argv lines went %d -> %d; a save is not a spawn that produced the frame", argvBefore, got)
	}
}

func countArgvLines(m Model) int {
	n := 0
	for _, l := range m.logLines {
		if l.lane == laneArgv {
			n++
		}
	}
	return n
}

// C13: the binding table declares the save as exactly one help-visible entry
// claiming ctrl+s, and the rendered help modal names it.
func TestSaveChartBindingIsDeclaredAndInHelp(t *testing.T) {
	var claiming []BindingEntry
	for _, e := range defaultKeys().mainBindings() {
		for _, k := range e.Binding.Keys() {
			if k == "ctrl+s" {
				claiming = append(claiming, e)
			}
		}
	}
	if len(claiming) != 1 {
		t.Fatalf("%d entries claim ctrl+s, want exactly 1", len(claiming))
	}
	if claiming[0].ID != BindingSaveChart {
		t.Errorf("ctrl+s is claimed by binding %v, want BindingSaveChart", claiming[0].ID)
	}
	if !claiming[0].Help {
		t.Error("the save binding is not help-visible, so nothing tells the operator it exists")
	}

	// The rendered modal, at both supported widths — a row that exists in the
	// table but that the help list never scrolls to is not "named". Help
	// scrolls (design piece 3), so walk it to the end.
	for _, size := range []struct{ w, h int }{{80, 24}, {132, 40}} {
		m := saveBase(t)
		m.width, m.height = size.w, size.h
		m, _ = press(t, m, "?")
		seen := m.View()
		for i := 0; i < len(m.helpLines()); i++ {
			m, _ = press(t, m, "down")
			seen += m.View()
		}
		if !strings.Contains(seen, "Save chart") {
			t.Errorf("the help modal at %dx%d does not name the save binding", size.w, size.h)
		}
	}
}

// Failure mode: a name that slugifies to nothing is refused and the prompt
// stays open.
func TestSavePromptRefusesAnEmptySlug(t *testing.T) {
	m := saveBase(t)
	m = runSave(t, m, "!!!")

	if m.startup != startupSaveOpen {
		t.Fatalf("startup = %q, want the prompt still open", m.startup)
	}
	if m.save.refusal == "" {
		t.Error("the prompt shows no refusal for a name with nothing to slugify")
	}
	entries, err := os.ReadDir(chartsDirOf(t))
	if err != nil {
		t.Fatalf("read charts dir: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("a refused save wrote %d file(s)", len(entries))
	}
}

// Failure mode: a name containing a double quote. The confirmation shows the
// name ASTROLOG recorded — it rewrites `"` to `'` — not the text that was
// typed, because the recorded one is what the library will show from now on.
func TestSavePromptShowsTheRecordedName(t *testing.T) {
	m := saveBase(t)
	before := len(m.logLines)
	m = runSave(t, m, `Say "Hi" Now`)

	added := m.logLines[before:]
	if len(added) != 1 {
		t.Fatalf("logged %d lines, want 1: %+v", len(added), added)
	}
	if !strings.Contains(added[0].text, `Say 'Hi' Now`) {
		t.Errorf("the confirmation %q does not show astrolog's recorded name `Say 'Hi' Now`", added[0].text)
	}
}

// Failure mode: the slot's place matches no atlas city. The save succeeds and
// the operator is told the chart will not list until the atlas can place it.
func TestSavePromptWarnsWhenTheAtlasCannotPlaceIt(t *testing.T) {
	m := saveBase(t)
	// Move the floating slot to the middle of the South Pacific.
	floating := m.slots.All()[1]
	floating.Location.Name = "Nowhere"
	floating.Location.Lat, floating.Location.Lon = -30.0, -140.0

	before := len(m.logLines)
	m = runSave(t, m, "Test Person")

	added := m.logLines[before:]
	if len(added) != 1 {
		t.Fatalf("logged %d lines, want exactly 1: %+v", len(added), added)
	}
	if !strings.Contains(added[0].text, "will not list") {
		t.Errorf("the confirmation %q does not warn that the chart will not list", added[0].text)
	}
	// It still saved.
	if _, err := os.Stat(filepath.Join(chartsDirOf(t), "test-person.as")); err != nil {
		t.Errorf("the chart was not written: %v", err)
	}
}

// A save the operator backed out of with Esc must not apply itself when it
// lands, and must not disturb whatever they moved on to. It did write its
// file, so it leaves its one Log line (the specification A8).
//
// Found by an independent review round, 2026-09-20. The astrolog spawn is not
// interruptible, so an abandoned save still COMPLETES; without a generation
// token its message set m.startup to a zeroed `prior` — a state no branch of
// View() renders — and a modal the operator had since opened vanished with no
// input from them.
func TestStaleSaveDoneIsNotAppliedAfterEsc(t *testing.T) {
	m := saveBase(t)
	m = openSave(t, m)
	m = typeInto(m, "Test Person")

	// Enter: capture the command WITHOUT running it, so the save is still in
	// flight for the rest of this test.
	tm, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = tm.(Model)
	if cmd == nil {
		t.Fatal("Enter returned no command, so nothing is in flight and this test proves nothing")
	}

	// Esc, then the operator opens the chart picker and starts working in it.
	tm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = tm.(Model)
	tm, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlO})
	m = tm.(Model)
	if m.startup != startupChartOpen {
		t.Fatalf("precondition: startup = %q, want the chart picker open", m.startup)
	}

	// Now the abandoned save lands.
	msg := cmd()
	before := len(m.logLines)
	tm, _ = m.Update(msg)
	m = tm.(Model)

	if m.startup != startupChartOpen {
		t.Errorf("the stale save changed startup to %q; the modal the operator opened is gone", m.startup)
	}
	if added := m.logLines[before:]; len(added) != 1 || !strings.HasPrefix(added[0].text, `save: "Test Person" saved to `) {
		t.Errorf("the late save logged %+v, want its one save line", added)
	}
}

// A paste carrying a newline cannot enter the name buffer, so the single input
// line stays one line and the save can never build a record astrolog would
// refuse to read back.
func TestSavePromptSanitisesAPastedNewline(t *testing.T) {
	m := saveBase(t)
	m = openSave(t, m)
	tm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Paste: true, Runes: []rune("Jane Doe\nb. 1987")})
	m = tm.(Model)

	if strings.ContainsAny(m.save.typedName(), "\n\r\t") {
		t.Fatalf("the name buffer holds a control character: %q", m.save.typedName())
	}
	if m.save.typedName() != "Jane Doe b. 1987" {
		t.Errorf("name = %q, want the break turned into a space", m.save.typedName())
	}
	// The prompt's input line is still ONE line.
	for _, line := range strings.Split(m.View(), "\n") {
		if strings.Contains(line, "Save as:") && strings.Count(line, "Save as:") != 1 {
			t.Errorf("the input line is malformed: %q", line)
		}
	}
	// And it saves, rather than being refused for something the prompt let in.
	m = runSaveFrom(t, m)
	if m.startup == startupSaveOpen {
		t.Errorf("the sanitised name was refused: %q", m.save.refusal)
	}
}

// runSaveFrom confirms an already-typed prompt and settles the result.
func runSaveFrom(t *testing.T, m Model) Model {
	t.Helper()
	tm, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = tm.(Model)
	if cmd == nil {
		t.Fatal("Enter returned no command")
	}
	tm2, _ := m.Update(cmd())
	return tm2.(Model)
}

// C8, through the REAL anchor path: save a chart, open the picker, press
// Enter on it, and require it to anchor.
//
// The lookup-side C8 test calls lookup.ResolvePicked directly, which is a
// weaker claim than the row makes. Anchoring in the app runs applyChartResolved
// as well, and that applies store.SessionChart.UnusableReason -- a stricter bar
// than the one the picker LISTS on. So a chart could list, resolve, and still
// refuse to anchor, and the lookup test would stay green.
//
// That is not hypothetical: on 2026-09-20 the operator saved a chart, saw it
// listed, pressed Enter and got "Couldn't anchor that chart. It has an unknown
// timezone" -- because the atlas parser had put a city name in the timezone
// field (fixed in internal/location/atlas.go, controlled in its own test).
// This test is the one that would have caught it here.
func TestSavedChartAnchorsThroughThePicker(t *testing.T) {
	m := saveBase(t)
	m = runSave(t, m, "Anchor Me")

	// Open the picker and let it load the library.
	tm, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlO})
	m = tm.(Model)
	if m.startup != startupChartOpen {
		t.Fatalf("startup = %q, want the chart picker open", m.startup)
	}
	if cmd == nil {
		t.Fatal("opening the picker dispatched no library load")
	}
	tm, _ = m.Update(cmd())
	m = tm.(Model)
	if m.picker.loadErr != nil {
		t.Fatalf("library load failed: %v", m.picker.loadErr)
	}
	if len(m.picker.matches) != 1 {
		t.Fatalf("picker shows %d rows, want exactly the 1 just saved (drops: %+v)",
			len(m.picker.matches), m.picker.drops)
	}
	if got := m.picker.matches[0].ID; got != "anchor-me" {
		t.Fatalf("the listed row is %q, want %q", got, "anchor-me")
	}

	// Enter: pick it, then run the resolve and deliver its result.
	tm, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = tm.(Model)
	if cmd == nil {
		t.Fatal("picking the row dispatched no resolve")
	}
	msg := cmd()
	resolved, ok := msg.(ChartResolvedMsg)
	if !ok {
		t.Fatalf("the resolve returned %T, want ChartResolvedMsg", msg)
	}
	if resolved.Err != nil {
		t.Fatalf("resolving the saved chart failed: %v", resolved.Err)
	}
	tm, _ = m.Update(resolved)
	m = tm.(Model)

	// It anchored: no refusal, and the anchor is the chart just saved.
	if m.picker.refusal != "" {
		t.Fatalf("the saved chart would not anchor: %s", m.picker.refusal)
	}
	if focusedRecord(m).ID != "anchor-me" {
		t.Errorf("focused chart ID = %q, want %q", focusedRecord(m).ID, "anchor-me")
	}
	if !focusedRecord(m).Place.HasTimezone() {
		t.Errorf("the anchored chart carries no timezone")
	}
	if _, err := time.LoadLocation(focusedRecord(m).Place.TZ); err != nil {
		t.Errorf("the anchored chart's timezone %q does not load: %v", focusedRecord(m).Place.TZ, err)
	}
}

// The same path, at a city the atlas parser used to poison.
//
// TestSavedChartAnchorsThroughThePicker saves at the fixture's own place,
// which was never affected, so it exercises the anchor path without being
// able to catch the defect that motivated it. Chur is one of the ~831 entries
// that carried "Biel/Bienne" as their timezone before
// internal/location/atlas.go was fixed: this case is red against that parser
// and green against this one, which is what makes it a control rather than
// another green line.
func TestSavedChartAnchorsAtAFormerlyPoisonedCity(t *testing.T) {
	m := saveBase(t)
	// Chur, Switzerland -- atlas row "-9.5333 46.8500 CH Chur", inheriting
	// Europe/Zurich from Aarau.
	floating := m.slots.All()[1]
	floating.Location = location.PlaceInfo{
		Name: "Chur", Country: "CH", Lat: 46.85, Lon: 9.5333, TZ: "Europe/Zurich",
	}

	m = runSave(t, m, "Chur Chart")

	tm, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlO})
	m = tm.(Model)
	if cmd == nil {
		t.Fatal("opening the picker dispatched no library load")
	}
	tm, _ = m.Update(cmd())
	m = tm.(Model)
	if len(m.picker.matches) != 1 {
		t.Fatalf("picker shows %d rows, want the 1 just saved (drops: %+v)",
			len(m.picker.matches), m.picker.drops)
	}

	tm, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = tm.(Model)
	if cmd == nil {
		t.Fatal("picking the row dispatched no resolve")
	}
	resolved, ok := cmd().(ChartResolvedMsg)
	if !ok {
		t.Fatal("the resolve did not return ChartResolvedMsg")
	}
	if resolved.Err != nil {
		t.Fatalf("resolving failed: %v", resolved.Err)
	}
	tm, _ = m.Update(resolved)
	m = tm.(Model)

	if m.picker.refusal != "" {
		t.Fatalf("a chart saved at Chur would not anchor: %s", m.picker.refusal)
	}
	if got := focusedRecord(m).Place.TZ; got != "Europe/Zurich" {
		t.Errorf("anchored timezone = %q, want %q", got, "Europe/Zurich")
	}
}

// U3 1: on an anchored chart ⌃s and `,s` change nothing, write no Log line,
// and the key bar says why.
func TestSaveOnAnAnchoredChartSaysWhy(t *testing.T) {
	const want = "Only Now can be saved as a new chart. Focus Now first."
	for _, name := range []string{"ctrl+s", "leader s"} {
		t.Run(name, func(t *testing.T) {
			m := saveBase(t)
			m.cursorIdx = 0
			startup, logged := m.startup, len(m.logLines)
			if name == "ctrl+s" {
				m = openSave(t, m)
			} else {
				m, _ = pressKey(t, m, ',')
				m, _ = pressKey(t, m, 's')
			}
			if m.startup != startup {
				t.Errorf("startup = %q, want it unchanged (%q)", m.startup, startup)
			}
			if len(m.logLines) != logged {
				t.Errorf("the refused save wrote %+v", m.logLines[logged:])
			}
			if got := m.footerIndicator(); got != want {
				t.Errorf("footerIndicator() = %q, want %q", got, want)
			}
		})
	}
}

// U3 2: with Now under the cursor the prompt opens and no notice is set.
func TestSaveOnNowSetsNoNotice(t *testing.T) {
	m := openSave(t, saveBase(t))
	if m.startup != startupSaveOpen {
		t.Fatalf("startup = %q, want the prompt open", m.startup)
	}
	if m.yank.notice != "" || m.footerIndicator() == noticeOnlyNow {
		t.Errorf("a notice was set: %q", m.yank.notice)
	}
}

// D2. Sextile writes Astrolog's own `.as`: the prompt line shows the file the
// typed name will become, the file is written under that name, and the log
// line carries its full path.
func TestSaveWritesDotAs(t *testing.T) {
	m := saveBase(t)
	m = openSave(t, m)
	m = typeInto(m, "Test Person")
	prompt := strings.Join(strings.Fields(renderSaveModal(m.save, m.cfg, 132, 40)), " ")
	if !strings.Contains(prompt, "/test-person.as") {
		t.Errorf("the save prompt does not show test-person.as:\n%s", prompt)
	}

	m = saveBase(t)
	before := len(m.logLines)
	m = runSave(t, m, "Test Person")
	want := filepath.Join(chartsDirOf(t), "test-person.as")
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("the saved file is not %s: %v", want, err)
	}
	added := m.logLines[before:]
	if len(added) != 1 || !strings.Contains(added[0].text, want) {
		t.Errorf("the log line %+v does not name %s", added, want)
	}
}

// The operator's rule (2026-10-26): a save opens the new chart right away, the
// way Ctrl+O would -- an anchored slot with the cursor on it, the Charts panel
// focused, nothing selected, the floating chart where it was, and the slot IS
// the saved file.
func TestSaveOpensTheNewChartFocused(t *testing.T) {
	m := saveBase(t)
	floatTime := m.slots.Floating().Time
	anchoredBefore := m.slots.AnchoredCount()
	selectedBefore := m.slots.SelectedCount()

	m = openSave(t, m)
	m = typeInto(m, "Open Me")
	tm, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = tm.(Model)
	tm, cmd = m.Update(cmd())
	m = tm.(Model)
	if cmd == nil {
		t.Fatal("a finished save dispatched no open of the new chart")
	}
	resolved, ok := cmd().(ChartResolvedMsg)
	if !ok {
		t.Fatalf("the open returned %T, want ChartResolvedMsg", cmd())
	}
	tm, _ = m.Update(resolved)
	m = tm.(Model)

	if got := m.slots.AnchoredCount(); got != anchoredBefore+1 {
		t.Fatalf("anchored count = %d, want %d", got, anchoredBefore+1)
	}
	if m.cursorIdx != m.slots.AnchoredCount()-1 {
		t.Errorf("cursor = %d, want the new slot (%d)", m.cursorIdx, m.slots.AnchoredCount()-1)
	}
	if m.focused != PanelChart {
		t.Errorf("focused panel = %v, want the Chart panel", m.focused)
	}
	if got := m.slots.SelectedCount(); got != selectedBefore {
		t.Errorf("selected count = %d, want %d: opening never selects", got, selectedBefore)
	}
	rec := focusedRecord(m)
	if rec.ID != "open-me" || rec.Source.Kind != store.ChartKindFile || rec.Source.File != "open-me.as" {
		t.Errorf("the slot is %+v, want the saved file open-me.as", rec)
	}
	if !m.slots.Floating().Time.Equal(floatTime) {
		t.Errorf("the floating chart moved: %v -> %v", floatTime, m.slots.Floating().Time)
	}
}
