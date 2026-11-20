package ui

import (
	tea "github.com/charmbracelet/bubbletea"

	"fmt"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/internal/ui/dialog"
	"github.com/cruiserxyz/sextile/internal/ui/keys"
	"github.com/cruiserxyz/sextile/internal/ui/popup"
)

// popup_rules_test.go holds two rules every popup keeps: no empty row at the
// top or the bottom inside the frame, and a bold green frame and title while
// it is open. The list of popups is fitCases (held to the program's own calls
// by TestEveryPopupIsInTheFitGuard) and the three button popups.

// ruleSizes are the two terminal sizes the rules are walked at.
var ruleSizes = []struct{ w, h int }{{80, 24}, {132, 40}}

// ruleBox is one popup as it opens: its name and a function that draws it.
type ruleBox struct {
	name string
	draw func(t *testing.T, cfg Config, w, h int) string
}

// rulePopups is every popup the app can open.
func rulePopups() []ruleBox {
	var out []ruleBox
	for _, c := range fitCases() {
		out = append(out, ruleBox{name: c.popup + "/" + c.state, draw: c.open})
	}
	choice := func(name string, choice func(t *testing.T) (Model, dialog.Choice)) {
		out = append(out, ruleBox{name: name, draw: func(t *testing.T, cfg Config, w, h int) string {
			_, c := choice(t)
			return dialog.RenderChoice(c, cfg, w, h, popupChrome(cfg))
		}})
	}
	choice("choice/first write", func(t *testing.T) (Model, dialog.Choice) {
		m := questionAt(t, "first write")
		return m, firstWriteChoice(m.save)
	})
	choice("choice/change", func(t *testing.T) (Model, dialog.Choice) {
		m := questionAt(t, "change")
		return m, changeChoice(m.title, m.cfg)
	})
	choice("choice/new session", func(t *testing.T) (Model, dialog.Choice) {
		m := askingModel(t)
		return m, newSessionChoice(m)
	})
	return out
}

// TestNoPopupHasAnEmptyRowAtItsTopOrBottom is row 26: inside the frame the
// first and the last row of every popup carry something, the popups that
// keep room for a message included. An empty row between two groups of one
// popup is not at its edge and stays.
func TestNoPopupHasAnEmptyRowAtItsTopOrBottom(t *testing.T) {
	for _, p := range rulePopups() {
		for _, size := range ruleSizes {
			t.Run(fmt.Sprintf("%s/%dx%d", p.name, size.w, size.h), func(t *testing.T) {
				// The popup with every row of its content on show: a list
				// shorter than its window keeps blank rows to hold its size,
				// and a scrolled list is cut wherever the window ends (the
				// Keybindings popup is walked at every scroll position below).
				restore := fitProbe(0, true)
				box := p.draw(t, Config{Accessible: true, Color: ColorNever}, size.w, size.h)
				restore()
				rows, _ := fitBoxRows(t, box)
				if strings.TrimSpace(rows[0]) == "" {
					t.Errorf("%s: the first row inside the frame is empty:\n%s", p.name, strings.Join(rows, "\n"))
				}
				if strings.TrimSpace(rows[len(rows)-1]) == "" {
					t.Errorf("%s: the last row inside the frame is empty:\n%s", p.name, strings.Join(rows, "\n"))
				}
			})
		}
	}
}

var sgrRun = regexp.MustCompile(`\x1b\[([0-9;]*)m`)

// styleAt is whether the text at rune index idx of a raw line is drawn bold
// and in the green of the frame, by reading the escapes before it.
func styleAt(raw string, idx int) (bold bool, fg string) {
	pos, cells := 0, 0
	for pos < len(raw) && cells < idx {
		if loc := sgrRun.FindStringIndex(raw[pos:]); loc != nil && loc[0] == 0 {
			bold, fg = applySGR(raw[pos:pos+loc[1]], bold, fg)
			pos += loc[1]
			continue
		}
		_, size := firstRune(raw[pos:])
		pos += size
		cells++
	}
	for pos < len(raw) {
		loc := sgrRun.FindStringIndex(raw[pos:])
		if loc == nil || loc[0] != 0 {
			break
		}
		bold, fg = applySGR(raw[pos:pos+loc[1]], bold, fg)
		pos += loc[1]
	}
	return bold, fg
}

func firstRune(s string) (rune, int) {
	for i, r := range s {
		if i == 0 {
			return r, len(string(r))
		}
	}
	return 0, 1
}

func applySGR(seq string, bold bool, fg string) (bool, string) {
	body := strings.TrimSuffix(strings.TrimPrefix(seq, "\x1b["), "m")
	parts := strings.Split(body, ";")
	if body == "" {
		return false, ""
	}
	for i := 0; i < len(parts); i++ {
		n, _ := strconv.Atoi(parts[i])
		switch {
		case n == 0:
			bold, fg = false, ""
		case n == 1:
			bold = true
		case n == 22:
			bold = false
		case n == 39:
			fg = ""
		case n == 38 && i+2 < len(parts) && parts[i+1] == "5":
			fg = parts[i+2]
			i += 2
		case n == 38 && i+4 < len(parts) && parts[i+1] == "2":
			fg = strings.Join(parts[i+2:i+5], ",")
			i += 4
		case n >= 30 && n <= 37, n >= 90 && n <= 97:
			fg = strconv.Itoa(n)
		}
	}
	return bold, fg
}

// isGreen is whether a foreground is the green of the focus frame (colour 10,
// or the plain greens).
func isGreen(fg string) bool {
	switch fg {
	case "10", "2", "32", "92", "0,255,0", "0,205,0":
		return true
	}
	return false
}

// TestEveryPopupFrameAndTitleAreBoldGreen is row 30: while a popup is open
// its top-left corner, its title, its side edge and its bottom-left corner are
// bold and green with colour on.
func TestEveryPopupFrameAndTitleAreBoldGreen(t *testing.T) {
	forceColor(t)
	for _, p := range rulePopups() {
		for _, size := range ruleSizes {
			t.Run(fmt.Sprintf("%s/%dx%d", p.name, size.w, size.h), func(t *testing.T) {
				lines := strings.Split(p.draw(t, Config{Color: ColorAlways}, size.w, size.h), "\n")
				check := func(what string, line string, idx int) {
					if bold, fg := styleAt(line, idx); !bold || !isGreen(fg) {
						t.Errorf("%s: %s is bold=%v fg=%q, want bold green:\n%q", p.name, what, bold, fg, line)
					}
				}
				check("the top-left corner", lines[0], 0)
				check("the title", lines[0], 2)
				mid := lines[len(lines)/2]
				check("the left edge", mid, 0)
				check("the bottom-left corner", lines[len(lines)-1], 0)
			})
		}
	}
}

// kbPopupLines is the Keybindings popup as it opens at a size, colour as
// given, stripped of escapes when plain is set.
func kbPopupLines(t *testing.T, cfg Config, w, h int, plain bool) []string {
	t.Helper()
	m := kbModel(t)
	m.cfg = cfg
	m.width, m.height = w, h
	m = openedKeybindings(m)
	out := renderKeybindings(m.keybindings, m.kbOff, cfg, w, h)
	if plain {
		out = ansi.Strip(out)
	}
	return strings.Split(out, "\n")
}

// TestKeybindingsTopEdgeSaysTypeToFilter is row 22: the top frame ends in
// "(Type to filter)" while nothing is typed and while a filter is.
func TestKeybindingsTopEdgeSaysTypeToFilter(t *testing.T) {
	for _, size := range ruleSizes {
		top := kbPopupLines(t, Config{Color: ColorNever}, size.w, size.h, true)[0]
		if !strings.Contains(top, "(Type to filter)─╮") && !strings.Contains(top, "(Type to filter)╮") {
			t.Errorf("%dx%d: the top edge is %q, want it to end in (Type to filter)", size.w, size.h, top)
		}
	}
	m := kbType(t, openedKeybindings(kbModel(t)), "zod")
	top := strings.Split(ansi.Strip(renderKeybindings(m.keybindings, m.kbOff, m.cfg, 132, 40)), "\n")[0]
	if !strings.Contains(top, "(Type to filter)") {
		t.Errorf("with a filter typed the top edge is %q, want the words to stay", top)
	}
	// In the frame's style: bold green with colour on.
	forceColor(t)
	raw := kbPopupLines(t, Config{Color: ColorAlways}, 132, 40, false)[0]
	idx := strings.Index(ansi.Strip(raw), "(Type")
	if bold, fg := styleAt(raw, len([]rune(ansi.Strip(raw)[:idx]))); !bold || !isGreen(fg) {
		t.Errorf("(Type to filter) is bold=%v fg=%q, want bold green", bold, fg)
	}
}

// TestKeybindingsTableSitsAtTheLeftEdge is row 23: the key column starts one
// cell inside the frame, so the widest key has one cell of margin, and the
// keys are right-aligned in their column.
func TestKeybindingsTableSitsAtTheLeftEdge(t *testing.T) {
	for _, size := range ruleSizes {
		m := kbModel(t)
		m.cfg = Config{Color: ColorAlways}
		m = openedKeybindings(m)
		lines := kbPopupLines(t, m.cfg, size.w, size.h, true)
		ends := map[int]bool{}
		for _, l := range lines[1 : len(lines)-1] {
			inner := string([]rune(l)[1 : len([]rune(l))-1])
			for _, d := range []string{" Open chart", " Save chart", " Choose city", " Choose houses", " Previous tab", " Next tab"} {
				i := strings.Index(inner, d)
				if i < 0 {
					continue
				}
				key := strings.TrimSpace(inner[:i])
				lead := len([]rune(inner[:i])) - len([]rune(strings.TrimLeft(inner[:i], " ")))
				if lead != 1+m.keybindings.KeyW-len([]rune(key)) {
					t.Errorf("%dx%d: %q starts %d cells inside the frame, want the key column at 1 (key width %d)", size.w, size.h, key, lead, m.keybindings.KeyW)
				}
				ends[len([]rune(inner[:i]))] = true
			}
		}
		if len(ends) != 1 {
			t.Errorf("%dx%d: the keys end in columns %v, want one", size.w, size.h, ends)
		}
	}
}

// TestKeybindingsHeaderStartsWhereDescriptionsStart is row 24: a group header
// starts in the column of the descriptions, bold and green.
func TestKeybindingsHeaderStartsWhereDescriptionsStart(t *testing.T) {
	forceColor(t)
	for _, size := range ruleSizes {
		raw := kbPopupLines(t, Config{Color: ColorAlways}, size.w, size.h, false)
		plain := kbPopupLines(t, Config{Color: ColorAlways}, size.w, size.h, true)
		for i, l := range plain {
			if !strings.Contains(l, "─── Local") {
				continue
			}
			hCol := len([]rune(l[:strings.Index(l, "─── Local")]))
			var descCol int
			for _, r := range plain[i+1:] {
				if j := strings.Index(r, "Details"); j >= 0 {
					descCol = len([]rune(r[:j]))
					break
				}
			}
			if hCol != descCol {
				t.Errorf("%dx%d: the header starts at cell %d, the descriptions at %d", size.w, size.h, hCol, descCol)
			}
			if bold, fg := styleAt(raw[i], hCol); !bold || !isGreen(fg) {
				t.Errorf("%dx%d: the header is bold=%v fg=%q, want bold green", size.w, size.h, bold, fg)
			}
		}
	}
}

// TestCursorRowIsABlueBarWithBrightKey is row 28, checked on a render with
// colour on against lazygit's capture: the cursor row has the blue background
// across the whole inner width, with its key in bright cyan on it, and the
// bottom frame carries the counter.
func TestCursorRowIsABlueBarWithBrightKey(t *testing.T) {
	forceColor(t)
	for _, size := range ruleSizes {
		raw := kbPopupLines(t, Config{Color: ColorAlways}, size.w, size.h, false)
		plain := kbPopupLines(t, Config{Color: ColorAlways}, size.w, size.h, true)
		if !regexp.MustCompile(`\d+ of \d+`).MatchString(plain[len(plain)-1]) {
			t.Errorf("%dx%d: the bottom edge has no counter: %q", size.w, size.h, plain[len(plain)-1])
		}
		found := false
		for i, l := range raw {
			if !strings.Contains(l, "44m") {
				continue
			}
			found = true
			w := ansi.StringWidth(plain[i])
			// The band runs from the left edge to the right one: every cell
			// between the frame's sides is drawn on the blue background.
			bandCells := 0
			bg := false
			for pos := 0; pos < len(l); {
				if loc := sgrRun.FindStringIndex(l[pos:]); loc != nil && loc[0] == 0 {
					seq := l[pos : pos+loc[1]]
					switch {
					case strings.Contains(seq, "44"):
						bg = true
					case seq == "\x1b[0m" || strings.HasSuffix(seq, "[49m"):
						bg = false
					}
					pos += loc[1]
					continue
				}
				_, size := firstRune(l[pos:])
				if bg {
					bandCells++
				}
				pos += size
			}
			if bandCells < w-3 {
				t.Errorf("%dx%d: the band covers %d cells, want the whole width inside the frame (%d)", size.w, size.h, bandCells, w-2)
			}
			if !strings.Contains(l, "38;5;14m") && !strings.Contains(l, "\x1b[96m") {
				t.Errorf("the key on the cursor row is not bright cyan: %q", l)
			}
		}
		if !found {
			t.Errorf("%dx%d: no row has a blue background", size.w, size.h)
		}
	}
}

// TestCommandsThatOpenAChooserEndInAnEllipsis is row 29: every command is run
// from the main screen, and its `Opens` field equals whether a popup or a
// prompt has the keyboard afterwards. The popup draws the `…` from the field.
func TestCommandsThatOpenAChooserEndInAnEllipsis(t *testing.T) {
	m0 := kbModel(t)
	ran := 0
	for _, e := range append(mainBindings(m0.keys), leaderOnlyBindings()...) {
		if !e.Palette && e.ID != keys.BindingActions {
			continue
		}
		t.Run(e.Title, func(t *testing.T) {
			m := kbModel(t)
			for i := 0; i < 6 && m.blocked(e.ID) != ""; i++ {
				m.cursorIdx = i
			}
			if why := m.blocked(e.ID); why != "" {
				t.Fatalf("%q cannot run in any state of the fixture: %s", e.Title, why)
			}
			tm, _, handled := m.runAction(e.ID)
			if !handled {
				t.Fatalf("%q is a command and runAction does not run it", e.Title)
			}
			after, ok := tm.(Model)
			if !ok {
				t.Fatalf("%q returned %T", e.Title, tm)
			}
			opened := after.keybindingsOpen || after.startup != startupReady || after.console.open
			if opened != e.Opens {
				t.Errorf("%q: Opens is %v but running it leaves a popup or prompt with the keyboard = %v", e.Title, e.Opens, opened)
			}
		})
		ran++
	}
	if ran < 15 {
		t.Fatalf("only %d commands were run", ran)
	}
	// The list is longer than the popup: read it at the top and at the end.
	m := openedKeybindings(m0)
	view := screen(m)
	for i := 0; i < 20; i++ {
		next, _ := m.Update(tea.KeyMsg{Type: tea.KeyPgDown})
		m = next.(Model)
		view += screen(m)
	}
	for _, want := range []string{"Open chart…", "Choose city…", "Keybindings…", "Copy", "Quit"} {
		if !strings.Contains(view, want) {
			t.Errorf("the popup lacks %q:\n%s", want, view)
		}
	}
	for _, bad := range []string{"Copy…", "Quit…", "Quit from anywhere…"} {
		if strings.Contains(view, bad) {
			t.Errorf("the popup has %q", bad)
		}
	}
}

// TestKeybindingsWindowNeverStartsOrEndsOnAnEmptyRow is row 26 for a popup
// that scrolls: at every scroll position and size the first and the last row
// inside the frame carry something, filtered or not.
func TestKeybindingsWindowNeverStartsOrEndsOnAnEmptyRow(t *testing.T) {
	for _, size := range ruleSizes {
		for _, filter := range []string{"", "o"} {
			m := kbModel(t)
			m.width, m.height = size.w, size.h
			m = openedKeybindings(m)
			if filter != "" {
				m = kbType(t, m, filter)
			}
			for i := 0; i < len(m.keybindings.Matches); i++ {
				rows, _ := fitBoxRows(t, renderKeybindings(m.keybindings, m.kbOff, m.cfg, size.w, size.h))
				if strings.TrimSpace(rows[0]) == "" || strings.TrimSpace(rows[len(rows)-1]) == "" {
					t.Fatalf("%dx%d filter %q cursor %d: the window starts or ends on an empty row:\n%s", size.w, size.h, filter, i, strings.Join(rows, "\n"))
				}
				m.keybindings.MoveBy(1, m.kbListHeight(m.keybindings))
			}
		}
	}
}

// TestAFormWithARefusalGrowsByItsLinesAndShrinksBack is the message zone's
// rule: a form keeps no row for a refusal. With one it is taller by a blank row
// and the refusal's lines, and when it goes the box is the height it opened at.
func TestAFormWithARefusalGrowsByItsLinesAndShrinksBack(t *testing.T) {
	for _, size := range ruleSizes {
		m := fitBase(t, Config{Accessible: true, Color: ColorNever}, size.w, size.h)
		m.startup = startupSessionNameOpen
		m.sessions = sessionsPopupState{Sessions: popup.Sessions{Form: dialog.NewForm(dialog.Field{Label: "Save as: "})}, prior: startupReady}
		height := func() int {
			rows, _ := fitBoxRows(t, renderSessionName(m, size.w, size.h))
			return len(rows)
		}
		quiet := height()
		m.sessions.Refusal = "That name is taken."
		one := height()
		m.sessions.Refusal = "That name is taken. " + strings.Repeat("Try another one, please. ", 6)
		many := height()
		lines := len(dialog.MessageLines(m.sessions.Refusal, dialog.ProseWidth))
		m.sessions.Refusal = ""
		back := height()
		if one != quiet+2 {
			t.Errorf("%dx%d: a one-line refusal makes the form %d rows, want %d (a blank row and the line)", size.w, size.h, one, quiet+2)
		}
		if many != quiet+1+lines || lines < 2 {
			t.Errorf("%dx%d: a refusal of %d lines makes the form %d rows, want %d", size.w, size.h, lines, many, quiet+1+lines)
		}
		if back != quiet {
			t.Errorf("%dx%d: after the refusal the form is %d rows, it opened at %d", size.w, size.h, back, quiet)
		}
	}
}
