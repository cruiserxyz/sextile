// All chart values reachable from this file are constructed synthetic test inputs.
package ui

import (
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/internal/store"
)

// Verifiers for the specification item 4: Ctrl+R in the chart picker
// renames the highlighted chart through the item-3 write path (C4.1–C4.6).
// Every chart file is a COPY in a temp library; live astrolog writes.

const titledChart = "@AI780  ; Astrolog chart info.\n-qb Sep 23 1995 6:00pm DT 1E 11:34E 48:08N\n-zi \"Constructed Titled\" \"Munich, Germany\"\n"

// renameLibrary is a library of a titled plain chart, an untitled one, a
// rich one, and one more titled chart sorting first, so a cursor landing on
// row 0 by accident cannot pass for one landing on the renamed chart.
func renameLibrary(t *testing.T) string {
	t.Helper()
	return titleLibrary(t, []string{"Equinox-2026-09.as", "FullMoon-2026-02.as"}, map[string]string{
		"Titled.as": titledChart,
		"Aaa.as":    strings.Replace(titledChart, "Constructed Titled", "Aaa Fixture", 1),
	})
}

// cursorTo puts the picker's cursor on file's row.
func cursorTo(t *testing.T, m Model, file string) Model {
	t.Helper()
	for i, c := range m.picker.matches {
		if c.Source.File == file {
			m.picker.cursor = i
			return m
		}
	}
	t.Fatalf("no row for %s in %v", file, pickerNames(m.picker.matches))
	return m
}

func ctrlR(t *testing.T, m Model) (Model, tea.Cmd) {
	t.Helper()
	return escSend(t, m, escKey(tea.KeyCtrlR))
}

// eraseField backspaces the focused field empty.
func eraseField(t *testing.T, m Model) Model {
	t.Helper()
	for range []rune(m.title.form.fields[m.title.form.focus].value) {
		m, _ = escSend(t, m, escKey(tea.KeyBackspace))
	}
	return m
}

// renameTo renames file to title through the keys, runs the write and the
// library reload it dispatches, and returns the model after both land.
func renameTo(t *testing.T, m Model, file, title string) Model {
	t.Helper()
	return renameWith(t, m, file, title, "")
}

// renameWith is renameTo for either field: an empty title or place leaves
// that field as Ctrl+R filled it in. It is strict: the write must be followed
// by the library reload and by nothing else.
func renameWith(t *testing.T, m Model, file, title, place string) Model {
	t.Helper()
	m, recompute := renameDriving(t, m, file, title, place, false)
	if recompute != nil {
		t.Fatal("the reload dispatched a command; a rename starts no work once the library is back")
	}
	return m
}

// renameRecomputing is renameWith for a chart a slot holds under a changed
// report name: the write may be followed by the reload AND one recompute of
// that slot's reports (the specification N3). The recompute is
// returned, not run.
func renameRecomputing(t *testing.T, m Model, file, title, place string) (Model, tea.Cmd) {
	t.Helper()
	return renameDriving(t, m, file, title, place, true)
}

func renameDriving(t *testing.T, m Model, file, title, place string, allowRecompute bool) (Model, tea.Cmd) {
	t.Helper()
	m = cursorTo(t, m, file)
	m, _ = ctrlR(t, m)
	if m.startup != startupTitleOpen || !m.title.rename {
		t.Fatalf("Ctrl+R did not open the rename form (startup %s)", m.startup)
	}
	if title != "" {
		m = eraseField(t, m)
		m, _ = escSend(t, m, escRunes(title))
	}
	if place != "" {
		m, _ = escSend(t, m, escKey(tea.KeyTab))
		m = eraseField(t, m)
		m, _ = escSend(t, m, escRunes(place))
	}
	m, cmd := escSend(t, m, escKey(tea.KeyEnter))
	if cmd == nil || !m.title.saving {
		t.Fatal("the submit dispatched no write")
	}
	m, cmd = runCmd(t, m, cmd)
	if m.title.refusal != "" {
		t.Fatalf("the write was refused: %s", m.title.refusal)
	}
	if cmd == nil {
		t.Fatal("the write was not followed by a library reload")
	}
	msg := cmd()
	var recompute tea.Cmd
	if batch, ok := msg.(tea.BatchMsg); ok && allowRecompute {
		var load tea.Msg
		for _, c := range batch {
			if c == nil {
				continue
			}
			if got := c(); got != nil {
				if _, isLoad := got.(ChartsLoadedMsg); isLoad && load == nil {
					load = got
					continue
				}
				got := got
				recompute = tea.Batch(recompute, func() tea.Msg { return got })
			}
		}
		msg = load
	}
	if _, ok := msg.(ChartsLoadedMsg); !ok {
		t.Fatalf("the write was followed by %T, want only the library reload", msg)
	}
	m, next := escSend(t, m, msg)
	if next != nil {
		t.Fatal("the reload dispatched a command; a rename starts no work once the library is back")
	}
	return m, recompute
}

// chartPanel is the Chart panel's text in a frame: the rows from its title
// to its bottom border, cut at the left column's width so nothing beside it,
// the Log included, is read as the panel's.
func chartPanel(t *testing.T, view string, width int) string {
	t.Helper()
	var out []string
	in := false
	for _, l := range strings.Split(ansi.Strip(view), "\n") {
		r := []rune(l)
		if len(r) > width {
			r = r[:width]
		}
		row := string(r)
		if strings.Contains(row, "[1] Chart") {
			in = true
			continue
		}
		if in && (strings.HasPrefix(row, "+") || strings.HasPrefix(row, "╰")) {
			return strings.Join(out, "\n")
		}
		if in {
			out = append(out, row)
		}
	}
	t.Fatalf("no Chart panel in the frame:\n%s", view)
	return ""
}

// C4.1: Ctrl+R on a plain chart opens the title form holding what the file
// states now, as values to edit.
func TestCtrlROpensRenameForm(t *testing.T) {
	renameLibrary(t)
	for _, tc := range []struct {
		file, title, titleHint, place string
	}{
		{"Titled.as", "Constructed Titled", "Constructed Titled", "Munich, Germany"},
		{"Equinox-2026-09.as", "", "Equinox-2026-09", "Bremen, Germany"},
	} {
		t.Run(tc.file, func(t *testing.T) {
			m := cursorTo(t, openedPicker(t), tc.file)
			m, cmd := ctrlR(t, m)
			if m.startup != startupTitleOpen || !m.title.rename || cmd != nil {
				t.Fatalf("Ctrl+R: startup %s, rename %v, cmd %v; want the rename form and nothing dispatched", m.startup, m.title.rename, cmd != nil)
			}
			f := m.title.form
			if f.fields[0].value != tc.title || f.fields[0].placeholder != tc.titleHint {
				t.Errorf("Title field = %+v, want value %q hint %q", f.fields[0], tc.title, tc.titleHint)
			}
			if f.fields[1].value != tc.place {
				t.Errorf("Place field = %+v, want value %q", f.fields[1], tc.place)
			}
			if m.title.chart.Source.File != tc.file {
				t.Errorf("the form renames %s, want %s", m.title.chart.Source.File, tc.file)
			}
		})
	}
}

// the specification C1: Ctrl+R opens the rename form on every .as,
// whatever else the file holds. FullMoon-2026-02.as carries a house-system
// line Astrolog does not write back; it is renamed like any other chart.
func TestCtrlROpensOnEveryChartFile(t *testing.T) {
	dir := renameLibrary(t)
	path := filepath.Join(dir, "FullMoon-2026-02.as")
	before := fileText(t, path)
	m := openedPicker(t)
	m, _ = escSend(t, m, escRunes("Full"))
	m = cursorTo(t, m, "FullMoon-2026-02.as")
	logged := len(m.logLines)

	m, cmd := ctrlR(t, m)
	if cmd != nil {
		t.Error("opening the rename form dispatched a command")
	}
	if m.startup != startupTitleOpen || !m.title.rename || m.title.chart.Source.File != "FullMoon-2026-02.as" {
		t.Fatalf("Ctrl+R on a full chart file did not open its rename form (startup %s, rename %v)", m.startup, m.title.rename)
	}
	if got := m.logLines[logged:]; len(got) != 0 {
		t.Errorf("opening the form wrote to the Log: %+v", got)
	}
	if fileText(t, path) != before {
		t.Error("opening the form changed the file")
	}
}

// C4.3: a changed title is written through the item-3 path, and the picker
// comes back with its filter cleared, the library reloaded and the cursor on
// the renamed chart. Nothing anchors.
func TestRenameWritesAndReturnsToPicker(t *testing.T) {
	for _, tc := range []struct {
		name, filter, title, place string
	}{
		{"dat", "", "Constructed Renamed", ""},
		// The new title does not match the filter the rename started from,
		// so the cursor can only land on it if the filter was cleared.
		{"filter no longer matches", "Titl", "Zed Fixture", ""},
		// A changed Place alone is a change: it is written, title kept.
		{"place only", "", "", "Munich, Bavaria"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := renameLibrary(t)
			path := filepath.Join(dir, "Titled.as")
			original := fileText(t, path)
			m := openedPicker(t)
			if tc.filter != "" {
				m, _ = escSend(t, m, escRunes(tc.filter))
			}
			anchor := anchorRecord(m)
			m = renameWith(t, m, "Titled.as", tc.title, tc.place)
			title, place := tc.title, tc.place
			if title == "" {
				title = "Constructed Titled"
			}
			if place == "" {
				place = "Munich, Germany"
			}

			if m.startup != startupChartOpen || !m.picker.loaded || m.picker.filterText != "" {
				t.Fatalf("want the picker back, loaded, filter cleared: startup %s, loaded %v, filter %q", m.startup, m.picker.loaded, m.picker.filterText)
			}
			row := m.picker.matches[m.picker.cursor]
			if row.Source.File != "Titled.as" || row.Name != title || row.PlaceLabel != place {
				t.Errorf("cursor on %s %q %q, want Titled.as %q %q", row.Source.File, row.Name, row.PlaceLabel, title, place)
			}
			if hits, _ := store.FilterCharts([]store.NamedChart{row}, tc.filter, 1); tc.filter != "" && len(hits) != 0 {
				t.Fatalf("precondition: %q still matches the filter %q", title, tc.filter)
			}
			if anchorRecord(m) != anchor || m.picker.resolving {
				t.Errorf("the rename anchored: %+v, resolving %v", anchorRecord(m), m.picker.resolving)
			}
			if !strings.Contains(fileText(t, path), `-zi "`+title+`" "`+place+`"`) {
				t.Errorf("the file does not carry the new title and place:\n%s", fileText(t, path))
			}
			if fileText(t, backupFile(path)) != original {
				t.Error("the original was not kept under .sextile/backup")
			}
			assertOneChartLine(t, m, `chart: titled Titled.as as "`+title+`" (original kept as .sextile/backup/Titled.as)`)
		})
	}
}

// C4.4: unchanged fields, an empty Title, or Esc write nothing and go back to
// the picker as it was.
func TestRenameNoChangeWritesNothing(t *testing.T) {
	for _, tc := range []struct {
		name string
		do   func(t *testing.T, m Model) (Model, tea.Cmd)
	}{
		{"unchanged", func(t *testing.T, m Model) (Model, tea.Cmd) {
			return escSend(t, m, escKey(tea.KeyEnter))
		}},
		{"retyped the same", func(t *testing.T, m Model) (Model, tea.Cmd) {
			m = eraseField(t, m)
			return escSend(t, m, escRunes("Constructed Titled "), escKey(tea.KeyEnter))
		}},
		{"empty title", func(t *testing.T, m Model) (Model, tea.Cmd) {
			m = eraseField(t, m)
			return escSend(t, m, escKey(tea.KeyEnter))
		}},
		{"esc", func(t *testing.T, m Model) (Model, tea.Cmd) {
			m = eraseField(t, m)
			return escSend(t, m, escRunes("Constructed Other"), escKey(tea.KeyEsc))
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := renameLibrary(t)
			path := filepath.Join(dir, "Titled.as")
			before := fileText(t, path)
			m := openedPicker(t)
			m, _ = escSend(t, m, escRunes("Titled"))
			m = cursorTo(t, m, "Titled.as")
			picker, screen := m.picker, snapshotForEsc(m)

			m, _ = ctrlR(t, m)
			if m.startup != startupTitleOpen {
				t.Fatalf("Ctrl+R did not open the form (startup %s)", m.startup)
			}
			m, cmd := tc.do(t, m)
			if cmd != nil {
				t.Error("a command was dispatched")
			}
			requireSameScreen(t, tc.name, screen, snapshotForEsc(m))
			if m.picker.filterText != picker.filterText || m.picker.cursor != picker.cursor || m.picker.resolving {
				t.Errorf("the picker is not as it was: filter %q cursor %d resolving %v", m.picker.filterText, m.picker.cursor, m.picker.resolving)
			}
			if fileText(t, path) != before {
				t.Error("the file changed")
			}
			noBackup(t, path)
			for _, l := range m.logLines {
				if strings.HasPrefix(l.text, "chart: titled") {
					t.Errorf("logged a write: %q", l.text)
				}
			}
		})
	}
}

// C4.5: Ctrl+R is the open-chart popup's entry, in its key bar, and no other
// owner's; pressed anywhere else it changes nothing.
func TestCtrlRIsDeclared(t *testing.T) {
	m := NewModel(Config{})
	ctrl := tea.KeyMsg{Type: tea.KeyCtrlR}
	for o := ownerMain; o <= ownerDetails; o++ {
		id := matchBinding(ctrl, m.ownerBindings(o))
		switch {
		case o == ownerOpenChart && id != BindingRename:
			t.Errorf("the open-chart table matches Ctrl+R as %d, want BindingRename", id)
		case o != ownerOpenChart && id != BindingNone:
			t.Errorf("owner %d's table matches Ctrl+R (%d)", o, id)
		}
	}
	inBar := false
	for _, e := range openChartBindings() {
		if e.ID == BindingRename && e.Bar && e.Action == "Rename" {
			inBar = true
		}
	}
	if !inBar {
		t.Error("Ctrl+R is not in the open-chart popup's key bar")
	}

	// Every owner but the picker, the chart title form included.
	for _, c := range keyOwnerCases() {
		if c.owner == ownerOpenChart {
			continue
		}
		t.Run(c.name, func(t *testing.T) {
			m := reachOwner(t, c, Config{Accessible: true}, 144, 40)
			screen, view, logged := snapshotForEsc(m), m.View(), len(m.logLines)
			m, cmd := ctrlR(t, m)
			if cmd != nil {
				t.Error("Ctrl+R dispatched a command")
			}
			requireSameScreen(t, "Ctrl+R in "+c.name, screen, snapshotForEsc(m))
			if m.currentOwner() != c.owner || m.View() != view || len(m.logLines) != logged {
				t.Errorf("Ctrl+R in %s changed the screen or the Log", c.name)
			}
		})
	}
}

// C4.6: renaming the anchored chart changes the name it is shown and
// remembered under, and does not re-anchor it. A changed REPORT name (the title)
// recomputes that slot's reports exactly once, because Astrolog's own header
// names the chart (the specification N3); a place label is not
// part of the anchor and not part of the header: nothing recomputes.
func TestRenameAnchoredUpdatesNameOnly(t *testing.T) {
	for _, tc := range []struct {
		name, title, place, want string
		recompute                bool
	}{
		{"title", "Zed One", "", "Zed One", true},
		// A place label is not part of the anchor: nothing changes at all.
		{"place only", "", "Munich, Bavaria", "Constructed Titled", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			renameLibrary(t)
			m := openedPicker(t)
			m, cmd := pickRow(t, m, "Titled.as")
			m = runToAnchor(t, m, cmd)
			m.startup = startupReady // the recompute the anchor dispatched is not this test's
			m.width, m.height = 132, 40
			if anchorRecord(m).Source.File != "Titled.as" {
				t.Fatalf("precondition: anchored %+v", anchorRecord(m))
			}
			before := anchorRecord(m)
			slots, handle, want := snapshotForEsc(m).Slots, anchorHandle(m), m.refresh[surfaceText].wantID

			m, load := escSend(t, m, escKey(tea.KeyCtrlO))
			m, _ = runCmd(t, m, load)
			var recompute tea.Cmd
			if tc.recompute {
				m, recompute = renameRecomputing(t, m, "Titled.as", tc.title, tc.place)
			} else {
				m = renameWith(t, m, "Titled.as", tc.title, tc.place)
			}

			after := before
			after.Name = tc.want
			if tc.recompute {
				after.Label.Name = tc.want
			}
			if anchorRecord(m) != after {
				t.Errorf("anchorChart = %+v, want %+v (the name only)", anchorRecord(m), after)
			}
			wantHandle := handle
			wantHandle.Label = after.Label
			if snapshotForEsc(m).Slots != slots || m.picker.resolving {
				t.Error("the rename re-anchored")
			}
			if tc.recompute {
				if recompute == nil || m.refresh[surfaceText].wantID != want+1 {
					t.Errorf("a changed report name recomputed %d times (command %v), want once", m.refresh[surfaceText].wantID-want, recompute != nil)
				}
			} else if m.refresh[surfaceText].wantID != want || recompute != nil {
				t.Error("the rename recomputed")
			}
			if anchorHandle(m) != wantHandle {
				t.Errorf("the chart payload's anchor moved: %+v, want %+v", anchorHandle(m), wantHandle)
			}
			m, _ = escSend(t, m, escKey(tea.KeyEsc))
			panel := chartPanel(t, m.View(), 66)
			if !strings.Contains(panel, tc.want) {
				t.Errorf("the Chart panel does not show %q:\n%s", tc.want, panel)
			}
			if tc.want != before.Name && strings.Contains(panel, "Constructed") {
				t.Errorf("the Chart panel still shows the old name:\n%s", panel)
			}
		})
	}
}
