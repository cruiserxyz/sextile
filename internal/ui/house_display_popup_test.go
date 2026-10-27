package ui

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// displayOpen is the main screen with the popup opened by `, a`.
func displayOpen(t *testing.T, cfg Config, w, h int) Model {
	t.Helper()
	m := escBase(t)
	m.cfg = cfg
	m.width, m.height = w, h
	m, _ = escSend(t, m, escRunes(","), escRunes("a"))
	if m.currentOwner() != ownerDisplay {
		t.Fatalf("`, a` left the keyboard with owner %d, want the House display popup", m.currentOwner())
	}
	return m
}

// displayLiveModel is a single chart on the main screen with the Bodies panel
// holding rows; with overlay the rows carry the chart's own house and the house
// in a compared chart's frame.
func displayLiveModel(t *testing.T, overlay bool, cfg Config) Model {
	t.Helper()
	m := bodiesPageModel(t, 16)
	m.cfg = cfg
	if overlay {
		m.chart.natalCusps, m.chart.natalPositions = wholeSignLeoRising, shiftedChart(40)
	}
	return m
}

// openOn opens the popup with `, a` on m.
func openOn(t *testing.T, m Model) Model {
	t.Helper()
	m, _ = escSend(t, m, escRunes(","), escRunes("a"))
	if m.currentOwner() != ownerDisplay {
		t.Fatalf("`, a` left the keyboard with owner %d, want the House display popup", m.currentOwner())
	}
	return m
}

// underneath is the Bodies panel as the program paints it now (paintedChart),
// one string per row, escapes stripped. It is what stands behind the popup,
// which at a small size covers the house column.
func underneath(m Model) []string {
	var out []string
	for _, l := range bodiesPageRows(m) {
		if strings.Contains(l, "°") {
			out = append(out, l)
		}
	}
	return out
}

// popupBody is the popup's interior without its frame, padding rows kept. The
// frame is found by its title, in the rounded or the ASCII border.
func popupBody(t *testing.T, m Model) []string {
	t.Helper()
	lines := strings.Split(ansi.Strip(m.View()), "\n")
	top, col, right := -1, -1, -1
	for i, l := range lines {
		if at := strings.Index(l, popupTitleDisplay); at >= 0 {
			r := []rune(l)
			title := len([]rune(l[:at]))
			for j := title; j >= 0; j-- {
				if r[j] == '╭' || r[j] == '+' {
					top, col = i, j
					break
				}
			}
			for j := title; j < len(r); j++ {
				if r[j] == '╮' || r[j] == '+' {
					right = j
					break
				}
			}
			break
		}
	}
	if top < 0 {
		t.Fatalf("no popup titled %q on screen:\n%s", popupTitleDisplay, ansi.Strip(m.View()))
	}
	var rows []string
	for _, l := range lines[top+1:] {
		r := []rune(l)
		if r[col] == '╰' || r[col] == '+' {
			return rows
		}
		rows = append(rows, string(r[col+1:right]))
	}
	t.Fatal("the popup has no bottom edge")
	return nil
}

var radioRe = regexp.MustCompile(`\((.)\)`)

// marks lists the radio mark of each of the first four rows.
func marks(t *testing.T, rows []string) []string {
	t.Helper()
	var out []string
	for _, r := range rows {
		if mm := radioRe.FindStringSubmatch(r); mm != nil {
			out = append(out, mm[1])
		}
	}
	if len(out) != len(houseForms) {
		t.Fatalf("found %d radio rows, want %d:\n%s", len(out), len(houseForms), strings.Join(rows, "\n"))
	}
	return out
}

func squash(s string) string { return strings.Join(strings.Fields(s), " ") }

// H3: the House display popup. `, a` opens it; moving the focus changes only
// the description area; Space chooses the focused form, moves the radio mark
// and puts the form live on the Bodies panel while the popup stays open; Enter
// and Esc close keeping the choice; the area under the dotted line shows the
// focused form's description and example; accessible mode draws ASCII.
func TestHouseDisplayPopup(t *testing.T) {
	cfg := Config{Accessible: false, Color: ColorNever}

	t.Run("opens on the form in force, the first by default", func(t *testing.T) {
		m := displayOpen(t, cfg, 132, 40)
		rows := popupBody(t, m)
		got := strings.Join(rows, "\n")
		for _, info := range houseForms {
			if !strings.Contains(got, info.title) {
				t.Errorf("the popup does not list %q:\n%s", info.title, got)
			}
		}
		if mk := marks(t, rows); strings.Join(mk, "") != "•   " {
			t.Errorf("radio marks = %q, want only the first chosen", mk)
		}
		if m.houseDisplayPopup.list.cursor != 0 {
			t.Errorf("the cursor opens on row %d, want the chosen form's row 0", m.houseDisplayPopup.list.cursor)
		}

		// Opened again with another form in force, the cursor starts on it.
		m.houseDisplay = houseMarked
		m, _ = escSend(t, m, escKey(tea.KeyEsc), escRunes(","), escRunes("a"))
		if m.houseDisplayPopup.list.cursor != 3 || m.houseDisplayPopup.list.selected != 3 {
			t.Errorf("reopened with marked in force: cursor %d selected %d, want 3 and 3", m.houseDisplayPopup.list.cursor, m.houseDisplayPopup.list.selected)
		}
	})

	// It opens from attached Details too since 2026-10-24
	// (TestLeaderWorksFromDetails).
	t.Run("only once the first frame is in", func(t *testing.T) {
		m := escBase(t)
		m.startup = startupComputing
		m, _ = escSend(t, m, escRunes(","), escRunes("a"))
		if m.currentOwner() == ownerDisplay || m.startup == startupDisplayOpen {
			t.Error("`, a` opened the popup before the first frame")
		}
	})

	t.Run("moving the focus changes only the description area", func(t *testing.T) {
		m := openOn(t, displayLiveModel(t, true, cfg))
		beforeRows, beforeUnder := popupBody(t, m), underneath(m)
		moved, _ := escSend(t, m, escRunes("j"))
		afterRows := popupBody(t, moved)

		if got := underneath(moved); strings.Join(got, "\n") != strings.Join(beforeUnder, "\n") {
			t.Errorf("a focus move changed the Bodies panel:\n%s\nbecame\n%s", strings.Join(beforeUnder, "\n"), strings.Join(got, "\n"))
		}
		if moved.houseDisplay != m.houseDisplay {
			t.Errorf("a focus move changed the form in force: %v", moved.houseDisplay)
		}
		if a, b := strings.Join(marks(t, beforeRows), ""), strings.Join(marks(t, afterRows), ""); a != b {
			t.Errorf("a focus move moved the radio mark: %q -> %q", a, b)
		}
		// The four rows keep their words (only the cursor's marker moves) and
		// the dotted line stays; the area under it does not.
		strip := func(r string) string { return squash(strings.Trim(strings.TrimSpace(r), "▶>")) }
		for i := 1; i <= 5; i++ {
			if strip(beforeRows[i]) != strip(afterRows[i]) {
				t.Errorf("row %d changed on a focus move: %q -> %q", i, beforeRows[i], afterRows[i])
			}
		}
		if squash(strings.Join(beforeRows[6:], " ")) == squash(strings.Join(afterRows[6:], " ")) {
			t.Error("the description area did not change with the focus")
		}
	})

	t.Run("the description area shows the focused form's words and example", func(t *testing.T) {
		m := displayOpen(t, cfg, 132, 40)
		for i, info := range houseForms {
			area := squash(strings.Join(popupBody(t, m)[6:], " "))
			for _, want := range []string{squash(info.description), squash(houseComparingNote), squash(ansi.Strip(houseExampleRow(info.form, cfg)))} {
				if !strings.Contains(area, want) {
					t.Errorf("focus on %q: the area lacks %q:\n%s", info.title, want, area)
				}
			}
			for j, other := range houseForms {
				if j != i && strings.Contains(area, squash(other.description)) {
					t.Errorf("focus on %q: the area also carries the description of %q", info.title, other.title)
				}
			}
			m, _ = escSend(t, m, escKey(tea.KeyDown))
		}
	})

	t.Run("space makes the focused form live, moves the mark and keeps the popup open", func(t *testing.T) {
		m := openOn(t, displayLiveModel(t, true, cfg))
		first := underneath(m)
		for _, l := range first {
			if !strings.Contains(l, "|") {
				t.Fatalf("the default form draws no frame in %q", l)
			}
		}

		m, _ = escSend(t, m, escKey(tea.KeyDown), escKey(tea.KeyDown))
		if m.houseDisplay != houseAngularity {
			t.Fatal("moving the focus changed the form")
		}
		m, _ = escSend(t, m, escKey(tea.KeySpace))
		if m.houseDisplay != houseNumber {
			t.Fatalf("Space on Plain number left the form at %v", m.houseDisplay)
		}
		if m.currentOwner() != ownerDisplay {
			t.Error("Space closed the popup")
		}
		if mk := strings.Join(marks(t, popupBody(t, m)), ""); mk != "  • " {
			t.Errorf("radio marks = %q, want only Plain number chosen", mk)
		}
		live := underneath(m)
		if strings.Join(live, "\n") == strings.Join(first, "\n") {
			t.Fatal("Space changed nothing on the Bodies panel")
		}
		pair := regexp.MustCompile(`\d+  \d+ *$`)
		for _, l := range live {
			if strings.Contains(l, "|") {
				t.Errorf("plain number still draws a frame: %q", l)
			}
			if !pair.MatchString(l) {
				t.Errorf("the row does not end in the own and compared house numbers: %q", l)
			}
		}

		// And on to True quadrants: a frame again, one cell to the left.
		m, _ = escSend(t, m, escKey(tea.KeyUp), escKey(tea.KeySpace))
		if m.houseDisplay != houseQuadrants {
			t.Fatalf("Space on True quadrants left the form at %v", m.houseDisplay)
		}
		quad := underneath(m)
		for i := range quad {
			if a, q := strings.Index(first[i], "|"), strings.Index(quad[i], "|"); a < 0 || q < 0 || q != a-1 {
				t.Errorf("row %d: first separator at %d in quadrants, %d in angularity; want one cell left", i, q, a)
			}
		}

		// A chart on its own (no compared frame) shows one number.
		alone := openOn(t, displayLiveModel(t, false, cfg))
		alone, _ = escSend(t, alone, escKey(tea.KeyDown), escKey(tea.KeyDown), escKey(tea.KeySpace))
		one := regexp.MustCompile(`[^\d]\d{1,2} *$`)
		for _, l := range underneath(alone) {
			if !one.MatchString(l) || strings.Contains(l, "|") {
				t.Errorf("a chart alone in plain number should end in one house number: %q", l)
			}
		}
	})

	t.Run("enter and escape close keeping the choice", func(t *testing.T) {
		for _, closer := range []tea.KeyType{tea.KeyEnter, tea.KeyEsc} {
			m := displayOpen(t, cfg, 132, 40)
			m, _ = escSend(t, m, escKey(tea.KeyDown), escKey(tea.KeyDown), escKey(tea.KeyDown), escKey(tea.KeySpace))
			if m.houseDisplay != houseMarked {
				t.Fatalf("Space on Number + angularity left the form at %v", m.houseDisplay)
			}
			m, _ = escSend(t, m, escKey(closer))
			if m.currentOwner() != ownerMain || m.startup != startupReady {
				t.Errorf("%v left the keyboard with owner %d in %q", closer, m.currentOwner(), m.startup)
			}
			if m.houseDisplay != houseMarked {
				t.Errorf("%v did not keep the choice: %v", closer, m.houseDisplay)
			}
			if strings.Contains(ansi.Strip(m.View()), popupTitleDisplay) {
				t.Errorf("%v left the popup on screen", closer)
			}
		}
	})

	t.Run("a focus move without Space is not a choice", func(t *testing.T) {
		m := displayOpen(t, cfg, 132, 40)
		m, _ = escSend(t, m, escKey(tea.KeyDown), escKey(tea.KeyEnter))
		if m.houseDisplay != houseAngularity {
			t.Errorf("closing after a move only changed the form to %v", m.houseDisplay)
		}
	})

	t.Run("j k and the paging keys move the focus", func(t *testing.T) {
		m := displayOpen(t, cfg, 132, 40)
		at := func(m Model) int { return m.houseDisplayPopup.list.cursor }
		for _, c := range []struct {
			name string
			keys []tea.Msg
			want int
		}{
			{"j", []tea.Msg{escRunes("j")}, 1},
			{"k back", []tea.Msg{escRunes("j"), escRunes("k")}, 0},
			{"k at the top", []tea.Msg{escRunes("k")}, 0},
			{"down past the end", []tea.Msg{escKey(tea.KeyDown), escKey(tea.KeyDown), escKey(tea.KeyDown), escKey(tea.KeyDown), escKey(tea.KeyDown)}, 3},
			{"End", []tea.Msg{escKey(tea.KeyEnd)}, 3},
			{"G", []tea.Msg{escRunes("G")}, 3},
			{"PgDn", []tea.Msg{escKey(tea.KeyPgDown)}, 3},
			{"Home after End", []tea.Msg{escKey(tea.KeyEnd), escKey(tea.KeyHome)}, 0},
			{"g after End", []tea.Msg{escKey(tea.KeyEnd), escRunes("g")}, 0},
			{"PgUp from the end", []tea.Msg{escKey(tea.KeyEnd), escKey(tea.KeyPgUp)}, 0},
		} {
			got, _ := escSend(t, m, c.keys...)
			if at(got) != c.want {
				t.Errorf("%s: cursor %d, want %d", c.name, at(got), c.want)
			}
			if got.houseDisplay != houseAngularity || got.houseDisplayPopup.list.selected != 0 {
				t.Errorf("%s moved the choice", c.name)
			}
		}
	})

	t.Run("accessible mode draws ASCII marks and divider", func(t *testing.T) {
		acc := displayOpen(t, Config{Accessible: true}, 132, 40)
		rows := popupBody(t, acc)
		joined := strings.Join(rows, "\n")
		if !strings.Contains(joined, "(*) Angularity at a glance") || strings.Contains(joined, "•") || strings.Contains(joined, "┄") {
			t.Errorf("accessible popup is not ASCII:\n%s", joined)
		}
		if !strings.Contains(joined, "----------------") {
			t.Errorf("accessible popup has no ASCII divider:\n%s", joined)
		}
		forceColor(t)
		uni := displayOpen(t, Config{Color: ColorAlways}, 132, 40)
		ujoined := ansi.Strip(strings.Join(popupBody(t, uni), "\n"))
		if !strings.Contains(ujoined, "(•) Angularity at a glance") || !strings.Contains(ujoined, "┈┈┈┈") {
			t.Errorf("unicode popup lacks its mark or dotted line:\n%s", ujoined)
		}
	})

	t.Run("a small terminal keeps the example and the rows", func(t *testing.T) {
		m := displayOpen(t, cfg, 80, 24)
		for _, info := range houseForms {
			rows := popupBody(t, m)
			joined := strings.Join(rows, "\n")
			if !strings.Contains(joined, squash(ansi.Strip(houseExampleRow(info.form, cfg)))) && !strings.Contains(squash(joined), squash(ansi.Strip(houseExampleRow(info.form, cfg)))) {
				t.Errorf("80x24, %s: the example is missing:\n%s", info.title, joined)
			}
			for _, other := range houseForms {
				if !strings.Contains(joined, other.title) {
					t.Errorf("80x24, %s: the row %q is missing", info.title, other.title)
				}
			}
			w := ansi.StringWidth(rows[0])
			for _, r := range rows {
				if ansi.StringWidth(r) != w {
					t.Errorf("80x24, %s: ragged popup row %q", info.title, r)
				}
			}
			m, _ = escSend(t, m, escKey(tea.KeyDown))
		}
	})
}

// H5 (keys): `, a` is in the leader map and the main table, the Keys popup
// lists it, the key bar of the popup is its table, and the owner's entries act.
func TestHouseDisplayKeys(t *testing.T) {
	t.Run("the shipped leader map resolves a", func(t *testing.T) {
		if got, id := mustDefaultLeaderMap().resolve([]string{"a"}); got != resolveAction || id != BindingHouseDisplay {
			t.Errorf("`a` resolves to (%v, %v)", got, id)
		}
	})

	t.Run("the Keys popup lists , a with its words", func(t *testing.T) {
		m := escBase(t)
		m.cfg = Config{Color: ColorNever}
		found := false
		for _, l := range m.helpLines() {
			if l.binding && squash(l.text) == "House display ,a" {
				found = true
			}
		}
		if !found {
			var lines []string
			for _, l := range m.helpLines() {
				lines = append(lines, l.text)
			}
			t.Errorf("the Keys popup has no `House display ,a` row:\n%s", strings.Join(lines, "\n"))
		}
		count := 0
		for _, l := range m.helpLines() {
			if strings.Contains(l.text, "House display") {
				count++
			}
		}
		if count != 1 {
			t.Errorf("the Keys popup names House display %d times, want once (the leader section)", count)
		}
	})

	t.Run("the main table entry acts only after the leader", func(t *testing.T) {
		m := escBase(t)
		if got, _ := escSend(t, m, escRunes("a")); got.currentOwner() == ownerDisplay {
			t.Error("a bare `a` opened the popup")
		}
		armed, _ := escSend(t, m, escRunes(","))
		if got, _ := escSend(t, armed, escRunes("a")); got.currentOwner() != ownerDisplay {
			t.Error("`, a` did not open the popup")
		}
	})

	t.Run("the bar is the popup's table", func(t *testing.T) {
		m := displayOpen(t, Config{Accessible: true}, 132, 40)
		// On the chosen form Space would do nothing, so the bar does not offer it.
		row := lastRow(m.View())
		if want := " Move: ↑↓ | Close: enter/esc/q"; !strings.HasPrefix(row, want) {
			t.Errorf("on the chosen form the bar is %q, want it to open %q", row, want)
		}
		checkBarShape(t, "house display", row, m.keyBarEntries(), m.width)
		moved, _ := escSend(t, m, escKey(tea.KeyDown))
		row = lastRow(moved.View())
		if want := " Move: ↑↓ | Choose: space | Close: enter/esc/q"; !strings.HasPrefix(row, want) {
			t.Errorf("on another form the bar is %q, want it to open %q", row, want)
		}
		checkBarShape(t, "house display, moved", row, moved.keyBarEntries(), moved.width)
	})

	t.Run("every entry of the popup's table acts", func(t *testing.T) {
		base := displayOpen(t, Config{Accessible: true}, 132, 40)
		press := func(m Model, k string) Model { m, _ = escSend(t, m, keyMsgFor(t, k)); return m }
		states := []Model{base, press(press(base, "down"), "down"), press(base, "end")}
		for _, e := range displayBindings() {
			k := e.Binding.Keys()[0]
			acted := false
			for _, s := range states {
				tm, cmd := s.Update(keyMsgFor(t, k))
				if cmd != nil || s.View() != tm.(Model).View() || fmt.Sprintf("%+v", s) != fmt.Sprintf("%+v", tm.(Model)) {
					acted = true
					break
				}
			}
			if !acted {
				t.Errorf("entry %d (%q) did nothing in any state", e.ID, k)
			}
		}
	})
}

// The popup is as tall as its fullest form needs, decided when it opens, and
// keeps that size whichever form the cursor is on (operator, 2026-10-27: a
// popup's size never changes while it is open). A form with less to say leaves
// rows blank.
func TestHouseDisplayPopupHeightIsFixed(t *testing.T) {
	for _, size := range [][2]int{{132, 40}, {80, 24}} {
		heights, widths := map[int]bool{}, map[int]bool{}
		for i := range houseForms {
			list := newRadioList(len(houseForms), 0)
			list.cursor = i
			frame := renderHouseDisplay(houseDisplayState{list: list}, Config{}, size[0], size[1])
			lines := strings.Split(frame, "\n")
			heights[len(lines)], widths[ansi.StringWidth(lines[0])] = true, true
		}
		if len(heights) != 1 || len(widths) != 1 {
			t.Errorf("%dx%d: the popup took %d heights and %d widths across the four forms: %v %v", size[0], size[1], len(heights), len(widths), heights, widths)
		}
	}
	// At 132x40 nothing limits it but its content: the box is the content
	// (four forms, dotted line, fullest description) plus its own chrome.
	fit := houseDisplayFit(Config{})
	list := newRadioList(len(houseForms), 0)
	frame := renderHouseDisplay(houseDisplayState{list: list}, Config{}, 132, 40)
	want := fit.Rows + 1 + 2 + 1 // top edge, padding, bottom edge
	if got := len(strings.Split(frame, "\n")); got != want {
		t.Errorf("132x40: the popup is %d lines, want %d (its content and chrome)", got, want)
	}
}
