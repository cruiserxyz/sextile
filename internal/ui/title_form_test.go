// All chart values reachable from this file are constructed synthetic test inputs.
package ui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/internal/recipe"
	"github.com/cruiserxyz/sextile/internal/store"
)

// Verifiers for the specification item 3 in the perimeter: the pick of an
// untitled chart opens the title form (C3.4–C3.6b), its refusals (C3.12), the
// anchor and Log line after a write (C3.13, C3.14), and restore never asking
// (C3.16). Every chart file is a COPY in a temp library; live astrolog writes.

const noPlaceChart = "@AI780  ; Astrolog chart info.\n-qb Sep 23 1995 6:00pm DT 1E 11:34E 48:08N\n-zi \"\" \"\"\n"

// titleLibrary makes a temp library holding copies of the named astrolog
// fixtures plus the given extra files, and returns its directory.
func titleLibrary(t *testing.T, fixtures []string, extra map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("SEXTILE_CHARTS_DIR", dir)
	t.Setenv("SEXTILE_DEV_CHARTS_DIR", "")
	for _, f := range fixtures {
		b, err := os.ReadFile(filepath.Join("..", "astrolog", "testdata", "charts", f))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, f), b, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for f, b := range extra {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, f)), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, f), []byte(b), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// pickRow presses Enter on the row for file in an opened picker.
func pickRow(t *testing.T, m Model, file string) (Model, tea.Cmd) {
	t.Helper()
	idx := -1
	for i, c := range m.picker.matches {
		if c.Source.File == file {
			idx = i
		}
	}
	if idx < 0 {
		t.Fatalf("no row for %s in %v", file, pickerNames(m.picker.matches))
	}
	m.picker.cursor = idx
	u, cmd := updateChartModalKey(m, tea.KeyMsg{Type: tea.KeyEnter})
	return u.(Model), cmd
}

func sendKey(t *testing.T, m Model, k tea.KeyMsg) (Model, tea.Cmd) {
	t.Helper()
	u, cmd := m.Update(k)
	return u.(Model), cmd
}

func typeText(t *testing.T, m Model, s string) Model {
	t.Helper()
	m, _ = sendKey(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)})
	return m
}

func fileText(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// backupFile is where the backup of the chart at path is kept.
func backupFile(path string) string {
	return filepath.Join(filepath.Dir(path), store.ChartBackupRel(filepath.Base(path)))
}

func noBackup(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(backupFile(path)); !os.IsNotExist(err) {
		t.Errorf("%s has a backup; nothing should have been written", filepath.Base(path))
	}
}

// runToAnchor runs a pick's resolve and returns the model after it lands.
func runToAnchor(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()
	m, _ = runCmd(t, m, cmd)
	if m.picker.resolveErr != nil || m.picker.refusal != "" {
		t.Fatalf("the pick did not anchor: err %v, refusal %q", m.picker.resolveErr, m.picker.refusal)
	}
	return m
}

// C3.4: picking a non-rich untitled chart, or a non-rich .as with no place
// label, opens the title form instead of resolving.
func TestPickUntitledOpensTitleForm(t *testing.T) {
	titleLibrary(t, []string{"Equinox-2026-09.as"}, map[string]string{
		"NoPlace.as":       noPlaceChart,
		"TitledNoPlace.as": strings.Replace(noPlaceChart, `-zi "" ""`, `-zi "Constructed Titled" ""`, 1),
	})
	for _, tc := range []struct {
		file, titleHint, placeValue, placeHint, plainInfo, colorInfo string
	}{
		{"Equinox-2026-09.as", "Equinox-2026-09", "Bremen, Germany", "", "Please enter a chart title for Equinox-2026-09.as.", "Please enter a chart title."},
		{"NoPlace.as", "NoPlace", "", "Munich, DE", "Please enter a chart title for NoPlace.as.", "Please enter a chart title."},
		{"TitledNoPlace.as", "Constructed Titled", "", "Munich, DE", "Please enter a place for TitledNoPlace.as.", "Please enter a place."},
	} {
		t.Run(tc.file, func(t *testing.T) {
			m := openedPicker(t)
			startGen := m.pickGen
			m, cmd := pickRow(t, m, tc.file)
			if m.startup != startupTitleOpen {
				t.Fatalf("startup = %s, want the title form", m.startup)
			}
			if cmd != nil || m.picker.resolving {
				t.Error("the pick resolved instead of asking for a title")
			}
			if m.pickGen == startGen {
				t.Error("opening the form did not bump the pick generation; an earlier resolve could land behind it")
			}
			f := m.title.form
			if f.fields[0].value != "" || f.fields[0].placeholder != tc.titleHint {
				t.Errorf("Title field = %+v, want empty with hint %q", f.fields[0], tc.titleHint)
			}
			if f.fields[1].value != tc.placeValue || f.fields[1].placeholder != tc.placeHint {
				t.Errorf("Place field = %+v, want value %q hint %q", f.fields[1], tc.placeValue, tc.placeHint)
			}
			// The info line: general with colour, where the placeholder shows
			// the name; naming the file without, where no placeholder is drawn.
			// Nothing else about files is shown (operator, 2026-10-02).
			plain := titleFrame(m)
			if !strings.Contains(plain, tc.plainInfo) || strings.Contains(plain, "backup") {
				t.Errorf("colourless form lacks %q, or mentions a backup:\n%s", tc.plainInfo, plain)
			}
			colored := renderTitleForm(m.title, Config{Color: ColorAlways}, 132, 40)
			if !strings.Contains(colored, tc.colorInfo) || strings.Contains(colored, tc.file) {
				t.Errorf("colour form lacks %q, or names the file:\n%s", tc.colorInfo, colored)
			}
		})
	}
}

// the specification C2: an untitled .as asks for a title when it is
// picked, whatever else its file holds. FullMoon-2026-02.as states no title
// and carries a house-system line.
func TestPickUntitledAlwaysAsksForTitle(t *testing.T) {
	dir := titleLibrary(t, []string{"FullMoon-2026-02.as"}, nil)
	path := filepath.Join(dir, "FullMoon-2026-02.as")
	before := fileText(t, path)
	m := openedPicker(t)
	m, cmd := pickRow(t, m, "FullMoon-2026-02.as")
	if m.startup != startupTitleOpen || cmd != nil {
		t.Fatalf("picking an untitled full chart file did not ask for a title (startup %s, cmd %v)", m.startup, cmd != nil)
	}
	if m.title.chart.Source.File != "FullMoon-2026-02.as" {
		t.Errorf("the form is for %q", m.title.chart.Source.File)
	}
	if fileText(t, path) != before {
		t.Error("the file changed before anything was typed")
	}
}

// C3.6: Enter with an empty Title writes nothing and anchors as an untouched
// pick does; the next pick asks again. Esc is not a dismissal any more: it
// goes back to the list (the specification C9,
// TestTitleFormEscGoesBack).
func TestTitleFormDismissAnchorsAsToday(t *testing.T) {
	for _, dismiss := range []string{"empty enter"} {
		t.Run(dismiss, func(t *testing.T) {
			dir := titleLibrary(t, []string{"Equinox-2026-09.as"}, nil)
			path := filepath.Join(dir, "Equinox-2026-09.as")
			before := fileText(t, path)
			m := openedPicker(t)
			m, _ = pickRow(t, m, "Equinox-2026-09.as")
			m = typeText(t, m, "   ") // spaces only is empty
			m, cmd := sendKey(t, m, tea.KeyMsg{Type: tea.KeyEnter})
			if m.startup != startupChartOpen || cmd == nil || !m.picker.resolving {
				t.Fatalf("dismissing did not resume the pick: startup %s", m.startup)
			}
			m = runToAnchor(t, m, cmd)
			if anchorRecord(m).Name != "Equinox-2026-09" || anchorRecord(m).Source.File != "Equinox-2026-09.as" {
				t.Errorf("anchored %+v, want Equinox-2026-09 as listed", anchorRecord(m))
			}
			if fileText(t, path) != before {
				t.Error("the file changed")
			}
			noBackup(t, path)

			m = openedPicker(t)
			if m, _ = pickRow(t, m, "Equinox-2026-09.as"); m.startup != startupTitleOpen {
				t.Error("the next pick of the same chart did not ask again")
			}
		})
	}
}

// C3.6b, as the specification C9 rewrites it: a title with no
// place, on a chart stating no place, is a required-field error. The popup
// stays open with the cursor on Place: and a line that names no key, and
// nothing is written.
func TestTitleFormRefusesAnEmptyPlace(t *testing.T) {
	dir := titleLibrary(t, nil, map[string]string{"NoPlace.as": noPlaceChart})
	path := filepath.Join(dir, "NoPlace.as")
	m := openedPicker(t)
	m, _ = pickRow(t, m, "NoPlace.as")
	m = typeText(t, m, "Constructed One")
	m, cmd := sendKey(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil || m.startup != startupTitleOpen {
		t.Fatalf("an empty place was submitted (startup %s)", m.startup)
	}
	frame := titleFrame(m)
	if !strings.Contains(frame, "Add a place to save this title.") {
		t.Errorf("the required-field line is not drawn:\n%s", frame)
	}
	if !strings.Contains(frame, "Place: █") {
		t.Errorf("the cursor is not on Place:\n%s", frame)
	}
	if strings.Contains(frame, "Title: Constructed One█") {
		t.Errorf("the cursor stayed on Title:\n%s", frame)
	}
	if fileText(t, path) != noPlaceChart {
		t.Error("the file changed")
	}
	noBackup(t, path)
}

// titleEquinox picks Equinox-2026-09.as, types title, submits, and runs the write and the
// resolve that follows it.
func titleEquinox(t *testing.T, title string) Model {
	t.Helper()
	m := openedPicker(t)
	m, _ = pickRow(t, m, "Equinox-2026-09.as")
	m = typeText(t, m, title)
	m, _ = sendKey(t, m, tea.KeyMsg{Type: tea.KeyEnter}) // the second step
	m, cmd := sendKey(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil || !m.title.saving {
		t.Fatal("the submit dispatched no write")
	}
	m, cmd = runCmd(t, m, cmd)
	if m.title.refusal != "" {
		t.Fatalf("the write was refused: %s", m.title.refusal)
	}
	if cmd == nil {
		t.Fatal("the write was not followed by a resolve")
	}
	return runToAnchor(t, m, cmd)
}

// C3.13: after the write the chart anchors under its new title; the library
// row, anchorChart.Name and the quit's session file all carry it, and the id
// is unchanged.
func TestTitledChartAnchorsUnderItsNewTitle(t *testing.T) {
	sessionSandbox(t)
	titleLibrary(t, []string{"Equinox-2026-09.as"}, nil)
	m := titleEquinox(t, "Constructed One")
	if anchorRecord(m).Name != "Constructed One" || anchorRecord(m).ID != "equinox-2026-09" {
		t.Errorf("anchorChart = %+v, want Name \"Constructed One\" and the id unchanged (equinox-2026-09)", anchorRecord(m))
	}
	lib, err := lookup.LoadChartLibrary()
	if err != nil || len(lib.Charts) != 1 || lib.Charts[0].Name != "Constructed One" || lib.Charts[0].ID != "equinox-2026-09" {
		t.Errorf("library = %+v (%v)", lib.Charts, err)
	}
	if _, cmd := press(t, m, "q"); cmd == nil {
		t.Fatal("q did not quit")
	}
	s, ok, err := store.LoadSession()
	if !ok || err != nil || len(s.Slots) != 1 || s.Slots[0].Chart.Name != "Constructed One" || s.Slots[0].Chart.ID != "equinox-2026-09" {
		t.Errorf("session chart = %+v (ok %v, err %v)", s.Slots, ok, err)
	}
}

// C3.14: one Log line after a write, naming the backup only when this write made
// it.
func TestTitleWriteLogLine(t *testing.T) {
	t.Run("first write", func(t *testing.T) {
		titleLibrary(t, []string{"Equinox-2026-09.as"}, nil)
		m := titleEquinox(t, "Constructed One")
		assertOneChartLine(t, m, `chart: titled Equinox-2026-09.as as "Constructed One" (original kept as .sextile/backup/Equinox-2026-09.as)`)
	})
	t.Run("backup already there", func(t *testing.T) {
		titleLibrary(t, []string{"Equinox-2026-09.as"}, map[string]string{".sextile/backup/Equinox-2026-09.as": "an older original\n"})
		m := titleEquinox(t, "Constructed One")
		assertOneChartLine(t, m, `chart: titled Equinox-2026-09.as as "Constructed One"`)
	})
}

func assertOneChartLine(t *testing.T, m Model, want string) {
	t.Helper()
	var got []string
	for _, l := range m.logLines {
		if strings.HasPrefix(l.text, "chart: titled") {
			got = append(got, l.text)
			if l.lane != laneNote {
				t.Errorf("%q is on lane %d, want the note lane", l.text, l.lane)
			}
		}
	}
	if len(got) != 1 || got[0] != want {
		t.Errorf("log = %q, want exactly [%q]", got, want)
	}
}

// C3.16: neither session restore nor the .as re-check opens the form or
// writes, even for an untitled, non-rich chart.
func TestRestoreNeverTitles(t *testing.T) {
	sessionSandbox(t)
	dir := store.ChartsDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(filepath.Join("..", "astrolog", "testdata", "charts", "Equinox-2026-09.as"))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "Equinox-2026-09.as")
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatal(err)
	}
	lib, err := lookup.LoadChartLibrary()
	if err != nil || len(lib.Charts) != 1 || !needsTitle(lib.Charts[0]) {
		t.Fatalf("precondition: Equinox-2026-09.as must list as needing a title: %+v %v", lib.Charts, err)
	}
	row := lib.Charts[0]
	when, err := lookup.ResolvePicked(context.Background(), row)
	if err != nil {
		t.Fatal(err)
	}
	rec := store.SessionChart{ID: row.ID, Name: row.Name, When: when, Place: row.Place, Source: row.Source}

	m := startedModel(t, chartFileSession(rec))
	if m.startup == startupTitleOpen || m.title.form.fields != nil {
		t.Error("restore opened the title form")
	}
	cmd := m.sessionRecheckCmd(chartFileSession(rec), true)
	if cmd == nil {
		t.Fatal("the restored .as owes a re-check")
	}
	u, _ := m.Update(cmd())
	m = u.(Model)
	if m.startup == startupTitleOpen || m.title.form.fields != nil {
		t.Error("the re-check opened the title form")
	}
	if fileText(t, path) != string(original) {
		t.Error("the chart file changed")
	}
	noBackup(t, path)
}

// titleFrame is the form as drawn at 132x40. These models have no terminal
// size, so View() would draw nothing at all.
func titleFrame(m Model) string {
	return renderTitleForm(m.title, m.cfg, 132, 40)
}

// TestTitleFormTasteFrames is V1, approved by the operator on 2026-10-02:
// the title form for an untitled .as with a place label, one with none, a
// typed title, and the refusal (the empty-place one, C3.6b, since the convert
// refusal went with YAML charts), at both supported sizes, with colour and
// without.
func TestTitleFormTasteFrames(t *testing.T) {
	forceColor(t)
	equinox := store.NamedChart{ID: "equinox-2026-09", Name: "Equinox-2026-09", Untitled: true, PlaceLabel: "Bremen, Germany",
		Place:  location.PlaceInfo{Name: "Bremen", Country: "DE", Lat: 53.07, Lon: 8.8, TZ: "Europe/Berlin"},
		Source: store.ChartSource{Kind: store.ChartKindFile, File: "Equinox-2026-09.as"}}
	noplace := store.NamedChart{ID: "noplace", Name: "NoPlace", Untitled: true,
		Place:  location.PlaceInfo{Name: "Munich", Country: "DE", Lat: 48.13, Lon: 11.57, TZ: "Europe/Berlin"},
		Source: store.ChartSource{Kind: store.ChartKindFile, File: "NoPlace.as"}}
	typed := titleFormState{form: newTitleForm(equinox), chart: equinox}
	typed.form.fields[0].value = "Constructed One"
	refused := titleFormState{form: newTitleForm(noplace), chart: noplace, refusal: emptyPlaceRefusal}
	refused.form.fields[0].value = "Constructed One"
	confirming := titleFormState{form: newTitleForm(equinox), chart: equinox, confirming: true}
	confirming.form.fields[0].value = "Constructed One"
	held := confirming
	held.backupHeld = true

	for _, st := range []struct {
		name string
		s    titleFormState
	}{
		{"place", titleFormState{form: newTitleForm(equinox), chart: equinox}},
		{"noplace", titleFormState{form: newTitleForm(noplace), chart: noplace}},
		{"typed", typed},
		{"refused", refused},
		{"confirm", confirming},
		{"confirm-held", held},
	} {
		for _, size := range [][2]int{{132, 40}, {80, 24}} {
			for _, mode := range []struct {
				name string
				cfg  Config
			}{{"color", Config{Color: ColorAlways}}, {"accessible", Config{Accessible: true, Color: ColorNever}}} {
				name := fmt.Sprintf("title-form-%s-%dx%d-%s", st.name, size[0], size[1], mode.name)
				t.Run(name, func(t *testing.T) {
					checkGolden(t, name, renderTitleForm(st.s, mode.cfg, size[0], size[1]))
				})
			}
		}
	}
}

// § Failure modes in the perimeter: a chart changed after the picker listed
// it is refused with the sentence the spec names, and nothing is written.
func TestTitleFormRefusesAChangedFile(t *testing.T) {
	dir := titleLibrary(t, []string{"Equinox-2026-09.as"}, nil)
	path := filepath.Join(dir, "Equinox-2026-09.as")
	m := openedPicker(t)
	m, _ = pickRow(t, m, "Equinox-2026-09.as")
	edited := strings.Replace(fileText(t, path), "2026 2:06 DT", "2026 2:36 DT", 1)
	if err := os.WriteFile(path, []byte(edited), 0o600); err != nil {
		t.Fatal(err)
	}
	m = typeText(t, m, "Constructed One")
	m, _ = sendKey(t, m, tea.KeyMsg{Type: tea.KeyEnter}) // the second step
	m, cmd := sendKey(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	m, _ = runCmd(t, m, cmd)
	if want := "Equinox-2026-09.as changed since the list was read — reopen the picker"; m.title.refusal != want {
		t.Errorf("refusal = %q, want %q", m.title.refusal, want)
	}
	if fileText(t, path) != edited {
		t.Error("the edited chart was rewritten")
	}
	noBackup(t, path)
}

// The info line wraps before a file name too long for the box, rather than
// cutting the name's end off (operator, 2026-10-02).
func TestTitleInfoWrapsALongFileName(t *testing.T) {
	c := store.NamedChart{ID: "x", Name: "x", Untitled: true, PlaceLabel: "Munich",
		Source: store.ChartSource{Kind: store.ChartKindFile, File: "Constructed Client With A Long Name.as"}}
	frame := renderTitleForm(titleFormState{form: newTitleForm(c), chart: c}, Config{Accessible: true, Color: ColorNever}, 80, 24)
	// Read the box's text back as one run of words: wrapping may split the
	// sentence across lines, but must not drop or cut any of it.
	var words []string
	for _, l := range strings.Split(frame, "\n") {
		words = append(words, strings.Fields(strings.Trim(l, "|"))...)
	}
	if !strings.Contains(strings.Join(words, " "), "Please enter a chart title for Constructed Client With A Long Name.as.") {
		t.Errorf("the sentence was cut at 80 columns:\n%s", frame)
	}
}

// After a write the chart anchors only from the row the library lists NOW.
// When the library cannot list it -- here, the atlas went missing -- nothing
// is anchored from the old pick: the write is logged and the picker reloads.
func TestTitleWriteWithoutAFreshRowAnchorsNothing(t *testing.T) {
	titleLibrary(t, []string{"Equinox-2026-09.as", "GreatConjunction-2020-12-21.as"}, nil)
	// An anchor already in place must survive the path untouched.
	m := openedPicker(t)
	m, cmd := pickRow(t, m, "GreatConjunction-2020-12-21.as")
	m = runToAnchor(t, m, cmd)
	if anchorRecord(m).Source.File != "GreatConjunction-2020-12-21.as" {
		t.Fatalf("precondition: anchored %+v", anchorRecord(m))
	}
	anchoredAt := firstAnchor(m.slots).Time
	u, load := m.Update(tea.KeyMsg{Type: tea.KeyCtrlO})
	m, _ = runCmd(t, u.(Model), load)
	before := anchorRecord(m)
	m, _ = pickRow(t, m, "Equinox-2026-09.as")
	m = typeText(t, m, "Constructed One")
	m, _ = sendKey(t, m, tea.KeyMsg{Type: tea.KeyEnter}) // the second step
	m, cmd = sendKey(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	t.Setenv("SEXTILE_ATLAS_PATH", filepath.Join(t.TempDir(), "no-atlas"))
	m, next := runCmd(t, m, cmd)
	if anchorRecord(m) != before || m.picker.resolving || !firstAnchor(m.slots).Time.Equal(anchoredAt) {
		t.Errorf("the anchor changed, or a pick is resolving, without a fresh row: %+v", anchorRecord(m))
	}
	if m.startup != startupChartOpen || m.picker.loaded || next == nil {
		t.Errorf("want the picker reloading: startup %s, loaded %v, cmd %v", m.startup, m.picker.loaded, next != nil)
	}
	assertOneChartLine(t, m, `chart: titled Equinox-2026-09.as as "Constructed One" (original kept as .sextile/backup/Equinox-2026-09.as)`)
}

// ---- S2: the title is confirmed before any file is touched ----

// diskSnapshot is every byte, mtime, mode and name under the roots, so a test
// cannot pass while a write -- a new file, a touched one, a made directory --
// slipped through. A root that does not exist is recorded as absent.
func diskSnapshot(t *testing.T, roots ...string) string {
	t.Helper()
	var b strings.Builder
	for _, root := range roots {
		if _, err := os.Lstat(root); err != nil {
			fmt.Fprintf(&b, "%s: absent\n", root)
			continue
		}
		err := filepath.Walk(root, func(p string, fi os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			fmt.Fprintf(&b, "%s mode=%v mtime=%d size=%d", p, fi.Mode(), fi.ModTime().UnixNano(), fi.Size())
			if fi.Mode().IsRegular() {
				data, rerr := os.ReadFile(p)
				if rerr != nil {
					return rerr
				}
				fmt.Fprintf(&b, " bytes=%q", data)
			}
			b.WriteString("\n")
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	return b.String()
}

// confirmWorld is a sandboxed session and preferences home plus a library of
// a copy of Equinox-2026-09.as, and the snapshot roots covering all three.
func confirmWorld(t *testing.T, extra map[string]string) (dir string, roots []string) {
	t.Helper()
	sessionSandbox(t)
	dir = titleLibrary(t, []string{"Equinox-2026-09.as"}, extra)
	return dir, []string{dir, store.SessionDir(), store.PreferencesPath()}
}

// flat reads a frame back as one run of words, so a sentence wrapped across
// lines still reads as itself.
func flat(frame string) string {
	var words []string
	for _, l := range strings.Split(frame, "\n") {
		words = append(words, strings.Fields(strings.Trim(l, "|"))...)
	}
	return strings.Join(words, " ")
}

// toConfirm picks Equinox-2026-09.as, types title and presses Enter once.
func toConfirm(t *testing.T, title string) (Model, tea.Cmd) {
	t.Helper()
	m := openedPicker(t)
	m, _ = pickRow(t, m, "Equinox-2026-09.as")
	m = typeText(t, m, title)
	return sendKey(t, m, tea.KeyMsg{Type: tea.KeyEnter})
}

func TestConfirmNamesTheFileAndBackup(t *testing.T) {
	confirmWorld(t, nil)
	m, _ := toConfirm(t, "Constructed One")
	if m.startup != startupTitleOpen || !m.title.confirming {
		t.Fatalf("startup %s, confirming %v; want the title popup in its second step", m.startup, m.title.confirming)
	}
	frame := flat(titleFrame(m))
	for _, want := range []string{"Equinox-2026-09.as", "keep the previous file in .sextile/backup.", "Press Enter to save the title, or Esc to go back."} {
		if !strings.Contains(frame, want) {
			t.Errorf("the second step does not say %q:\n%s", want, titleFrame(m))
		}
	}
}

func TestNothingIsWrittenBeforeConfirm(t *testing.T) {
	_, roots := confirmWorld(t, nil)
	m := openedPicker(t)
	m, _ = pickRow(t, m, "Equinox-2026-09.as")
	m = typeText(t, m, "Constructed One")
	before := diskSnapshot(t, roots...)
	m, cmd := sendKey(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		t.Error("Enter on the form returned a command; nothing may be started before the confirm")
	}
	if !m.title.confirming || m.title.saving {
		t.Errorf("confirming %v, saving %v; want the second step and no write in flight", m.title.confirming, m.title.saving)
	}
	if after := diskSnapshot(t, roots...); after != before {
		t.Errorf("disk changed before the confirm:\n--- before\n%s\n--- after\n%s", before, after)
	}
}

func TestConfirmCancelLeavesEverything(t *testing.T) {
	_, roots := confirmWorld(t, nil)
	m := openedPicker(t)
	m, _ = pickRow(t, m, "Equinox-2026-09.as")
	m = typeText(t, m, "Constructed One")
	before := diskSnapshot(t, roots...)
	m, _ = sendKey(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if !m.title.confirming {
		t.Fatal("precondition: the second step is not open")
	}
	m, cmd := sendKey(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if cmd != nil || m.title.confirming || m.title.saving || m.startup != startupTitleOpen {
		t.Fatalf("Cancel: cmd %v, confirming %v, saving %v, startup %s; want the form again", cmd != nil, m.title.confirming, m.title.saving, m.startup)
	}
	if got, want := m.title.form.value(0), "Constructed One"; got != want {
		t.Errorf("Title = %q, want %q", got, want)
	}
	if got, want := m.title.form.value(1), "Bremen, Germany"; got != want {
		t.Errorf("Place = %q, want %q", got, want)
	}
	if after := diskSnapshot(t, roots...); after != before {
		t.Errorf("Cancel changed the disk:\n--- before\n%s\n--- after\n%s", before, after)
	}
	m, _ = sendKey(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.startup != startupChartOpen {
		t.Errorf("Esc in the form: startup %s, want the picker", m.startup)
	}
	if after := diskSnapshot(t, roots...); after != before {
		t.Error("Esc to the picker changed the disk")
	}
}

func TestConfirmSaveKeepsTheOriginal(t *testing.T) {
	dir, _ := confirmWorld(t, nil)
	path := filepath.Join(dir, "Equinox-2026-09.as")
	original := fileText(t, path)
	m := titleEquinox(t, "Constructed One")
	if got := fileText(t, path); got == original || !strings.Contains(got, "Constructed One") {
		t.Errorf("Equinox-2026-09.as was not retitled:\n%s", got)
	}
	if got := fileText(t, backupFile(path)); got != original {
		t.Errorf("the backup of Equinox-2026-09.as is not the original:\n%s", got)
	}
	assertOneChartLine(t, m, `chart: titled Equinox-2026-09.as as "Constructed One" (original kept as .sextile/backup/Equinox-2026-09.as)`)
}

func TestConfirmWhenBackupHeld(t *testing.T) {
	dir, _ := confirmWorld(t, map[string]string{".sextile/backup/Equinox-2026-09.as": "an older original\n"})
	path := filepath.Join(dir, "Equinox-2026-09.as")
	original := fileText(t, path)
	bakBefore, err := os.Stat(backupFile(path))
	if err != nil {
		t.Fatal(err)
	}
	m, _ := toConfirm(t, "Constructed One")
	if !m.title.confirming {
		t.Fatal("the second step is not open")
	}
	if frame := flat(titleFrame(m)); !strings.Contains(frame, "Its first original is already in .sextile/backup and stays there.") {
		t.Errorf("the popup does not say the backup stays:\n%s", titleFrame(m))
	}
	m, cmd := sendKey(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	m, next := runCmd(t, m, cmd)
	runToAnchor(t, m, next)
	if got := fileText(t, backupFile(path)); got != "an older original\n" {
		t.Errorf("the held backup changed: %q", got)
	}
	bakAfter, err := os.Stat(backupFile(path))
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(bakBefore, bakAfter) || bakBefore.Mode() != bakAfter.Mode() || !bakBefore.ModTime().Equal(bakAfter.ModTime()) {
		t.Errorf("the held backup was replaced or touched: before %v %v, after %v %v",
			bakBefore.Mode(), bakBefore.ModTime(), bakAfter.Mode(), bakAfter.ModTime())
	}
	if got := fileText(t, path); got == original || !strings.Contains(got, "Constructed One") {
		t.Errorf("Equinox-2026-09.as was not retitled:\n%s", got)
	}
}

func TestConfirmRefusesAnUnusableBackup(t *testing.T) {
	dir, roots := confirmWorld(t, nil)
	if err := os.MkdirAll(filepath.Join(dir, store.ChartBackupRel("Equinox-2026-09.as")), 0o700); err != nil {
		t.Fatal(err)
	}
	m := openedPicker(t)
	m, _ = pickRow(t, m, "Equinox-2026-09.as")
	m = typeText(t, m, "Constructed One")
	before := diskSnapshot(t, roots...)
	m, cmd := sendKey(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil || m.title.confirming || m.title.saving {
		t.Fatalf("cmd %v, confirming %v, saving %v; want a refusal in the form", cmd != nil, m.title.confirming, m.title.saving)
	}
	want := ".sextile/backup/Equinox-2026-09.as is not a file, so nothing can be saved"
	if m.title.refusal != want || !strings.Contains(flat(titleFrame(m)), want) {
		t.Errorf("refusal = %q, want %q drawn in the form", m.title.refusal, want)
	}
	if after := diskSnapshot(t, roots...); after != before {
		t.Error("the refusal changed the disk")
	}
}

// The directory shown at the confirm step is the directory written: a charts
// location that resolves differently by Save time must not redirect the write.
func TestConfirmWritesWhereItShowed(t *testing.T) {
	dir, _ := confirmWorld(t, nil)
	// A second library the resolver will point at, holding its own Equinox-2026-09.as, so
	// a write that followed the resolver would succeed there and be visible.
	other := t.TempDir()
	b, err := os.ReadFile(filepath.Join(dir, "Equinox-2026-09.as"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(other, "Equinox-2026-09.as"), b, 0o600); err != nil {
		t.Fatal(err)
	}
	m, _ := toConfirm(t, "Constructed One")
	if !m.title.confirming {
		t.Fatal("the second step is not open")
	}
	t.Setenv("SEXTILE_CHARTS_DIR", other)
	otherBefore := diskSnapshot(t, other)
	m, cmd := sendKey(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("Save title dispatched no write")
	}
	msg, ok := cmd().(TitleWrittenMsg)
	if !ok || msg.Err != nil {
		t.Fatalf("the write: %#v", msg)
	}
	if got := fileText(t, filepath.Join(dir, "Equinox-2026-09.as")); !strings.Contains(got, "Constructed One") {
		t.Error("the directory shown at the confirm step was not written")
	}
	if _, err := os.Stat(filepath.Join(dir, store.ChartBackupRel("Equinox-2026-09.as"))); err != nil {
		t.Errorf("no backup where the popup said: %v", err)
	}
	if after := diskSnapshot(t, other); after != otherBefore {
		t.Error("the write followed the resolver into another directory")
	}
}

func TestConfirmFramesAndBar(t *testing.T) {
	long := store.NamedChart{ID: "x", Name: "x", Untitled: true, PlaceLabel: "Munich",
		Source: store.ChartSource{Kind: store.ChartKindFile, File: "Constructed Client With A Long Name.as"}}
	for _, held := range []bool{false, true} {
		for _, size := range [][2]int{{80, 24}, {66, 15}} {
			st := titleFormState{form: newTitleForm(long), chart: long, confirming: true, backupHeld: held}
			st.form.fields[0].value = "Constructed One"
			cfg := Config{Accessible: true, Color: ColorNever}
			frame := renderTitleForm(st, cfg, size[0], size[1])
			lines := strings.Split(frame, "\n")
			if len(lines) > size[1] {
				t.Errorf("held %v %dx%d: %d lines", held, size[0], size[1], len(lines))
			}
			for _, l := range lines {
				if w := ansi.StringWidth(l); w > size[0] {
					t.Errorf("held %v %dx%d: a line is %d wide: %q", held, size[0], size[1], w, l)
				}
			}
			words := flat(frame)
			backupWords := "keep the previous file in .sextile/backup."
			if held {
				backupWords = "is already in .sextile/backup and stays there."
			}
			for _, want := range []string{"Constructed Client With A Long Name.as", backupWords, "Press Enter to save the title, or Esc to go back."} {
				if !strings.Contains(words, want) {
					t.Errorf("held %v %dx%d: %q is cut:\n%s", held, size[0], size[1], want, frame)
				}
			}
			m := NewModelWithClock(cfg, FrozenClock(time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)))
			m.startup, m.title = startupTitleOpen, st
			if got, want := keyBarLine(size[0], m.keyBarEntries(), cfg), "Save title: enter | Cancel: esc"; got != want {
				t.Errorf("%dx%d bar = %q, want %q", size[0], size[1], got, want)
			}
		}
	}
}

// A confirmed write never asks the charts-directory resolver again: the house
// record, the rewrite, the row it anchors and the moment it opens all come from
// the directory the popup named, even when the resolver answers with a library
// holding a different file of the same name.
func TestConfirmedWriteNeverReResolvesTheDir(t *testing.T) {
	sessionSandbox(t)
	a := titleLibrary(t, nil, map[string]string{"Equinox-2026-09.as": kochChart("", true)})
	b := t.TempDir()
	bText := "@AI780  ; Astrolog chart info.\n-qb Jun 5 1980 3:00pm DT 1E 11:34E 48:08N\n-zi \"\" \"Munich, Germany\"\n-c Equal\n"
	if err := os.WriteFile(filepath.Join(b, "Equinox-2026-09.as"), []byte(bText), 0o600); err != nil {
		t.Fatal(err)
	}
	stubHouseLookups(t)

	aRead, err := lookup.ReadChartFileIn(context.Background(), a, "Equinox-2026-09.as")
	if err != nil || !aRead.HasHouses || aRead.Houses != recipe.Koch {
		t.Fatalf("premise: library A reads as %+v (%v), want Koch", aRead, err)
	}

	m := openedPicker(t)
	m, _ = pickRow(t, m, "Equinox-2026-09.as")
	m = typeText(t, m, "Constructed One")
	m, _ = sendKey(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if !m.title.confirming {
		t.Fatal("the confirm step is not open")
	}
	t.Setenv("SEXTILE_CHARTS_DIR", b)
	bBefore := diskSnapshot(t, b)
	m, cmd := sendKey(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	var written TitleWrittenMsg
	for i := 0; i < 6 && cmd != nil && m.slots.AnchoredCount() == 0; i++ {
		msg := cmd()
		if tw, ok := msg.(TitleWrittenMsg); ok {
			written = tw
		}
		u, next := m.Update(msg)
		m, cmd = u.(Model), next
		if m.title.refusal != "" {
			t.Fatalf("the write was refused: %s", m.title.refusal)
		}
	}

	rec, ok := m.frames.find("Equinox-2026-09.as", false)
	if !ok || rec.Houses != recipe.Koch || !rec.When.Equal(aRead.When) {
		t.Errorf("session house record = %+v (found %v), want library A's moment %v and Koch", rec, ok, aRead.When)
	}
	if m.slots.AnchoredCount() != 1 {
		t.Fatalf("anchored slots = %d, want the written chart anchored", m.slots.AnchoredCount())
	}
	// The row the write hands back is library A's, listed after the rewrite.
	if written.Err != nil || written.Result.Row.Name != "Constructed One" || written.Result.Row.Place.Name != "Bremen" {
		t.Errorf("the write's row = %q at %q (err %v), want library A's \"Constructed One\" at Bremen",
			written.Result.Row.Name, written.Result.Row.Place.Name, written.Err)
	}
	libA, err := lookup.LoadChartLibraryIn(a)
	if err != nil || len(libA.Charts) != 1 {
		t.Fatalf("library A after the write: %+v (%v)", libA.Charts, err)
	}
	for _, sl := range m.slots.All() {
		if sl.Floating {
			continue
		}
		if sl.Chart.ID != libA.Charts[0].ID {
			t.Errorf("anchored chart id = %q, want library A's row %q", sl.Chart.ID, libA.Charts[0].ID)
		}
		if sl.Chart.Source.File != "Equinox-2026-09.as" || sl.Chart.Name != "Constructed One" || !sl.Time.Equal(aRead.When) {
			t.Errorf("anchored chart = %q file %q at %v; want library A's Equinox-2026-09.as, titled, at %v",
				sl.Chart.Name, sl.Chart.Source.File, sl.Time, aRead.When)
		}
	}
	if got := fileText(t, filepath.Join(a, "Equinox-2026-09.as")); !strings.Contains(got, "Constructed One") {
		t.Errorf("library A's Equinox-2026-09.as was not retitled:\n%s", got)
	}
	if after := diskSnapshot(t, b); after != bBefore {
		t.Errorf("library B changed:\n--- before\n%s\n--- after\n%s", bBefore, after)
	}
}

// D4. A retitle keeps the previous file under <charts>/.sextile/backup, where
// the library never looks: the folder listing shows no ghost chart, and the
// confirm step names the backup folder rather than a sibling file.
func TestRetitleBackupLivesUnderSextileBackup(t *testing.T) {
	dir, _ := confirmWorld(t, nil)
	path := filepath.Join(dir, "Equinox-2026-09.as")
	original := fileText(t, path)

	m, _ := toConfirm(t, "Constructed One")
	if !m.title.confirming {
		t.Fatal("the second step is not open")
	}
	frame := flat(titleFrame(m))
	if !strings.Contains(frame, "keep the previous file in .sextile/backup.") || strings.Contains(frame, ".bak") {
		t.Errorf("the confirm step must name the backup folder and no .bak file:\n%s", titleFrame(m))
	}

	m, cmd := sendKey(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	m, next := runCmd(t, m, cmd)
	runToAnchor(t, m, next)

	if got := fileText(t, filepath.Join(dir, ".sextile", "backup", "Equinox-2026-09.as")); got != original {
		t.Errorf("the backup does not hold the old bytes:\n%s", got)
	}
	if got := fileText(t, path); got == original || !strings.Contains(got, "Constructed One") {
		t.Errorf("Equinox-2026-09.as was not retitled:\n%s", got)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	if got := strings.Join(names, ","); got != ".sextile,Equinox-2026-09.as" {
		t.Errorf("the charts folder holds %q, want only the chart and the hidden .sextile", got)
	}
	lib, err := lookup.LoadChartLibraryIn(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(lib.Charts) != 1 || lib.Charts[0].Source.File != "Equinox-2026-09.as" || len(lib.Drops) != 0 {
		t.Errorf("the listing after a retitle: charts %v, drops %v; want Equinox-2026-09.as alone, no ghost", lib.Charts, lib.Drops)
	}
}
