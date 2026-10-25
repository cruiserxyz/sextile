package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// markedRows are the rows of a rendered popup that carry the cursor mark.
func markedRows(view string) []string {
	var out []string
	for _, line := range strings.Split(ansi.Strip(view), "\n") {
		if i := strings.Index(line, "> "); i >= 0 {
			out = append(out, strings.TrimSpace(line[i+2:]))
		}
	}
	return out
}

func popupOwners() []pageOwner {
	var out []pageOwner
	for _, o := range pageOwners() {
		if o.popup {
			out = append(out, o)
		}
	}
	return out
}

// A5: all six filtered popups page by the one rule, at the ends as well as in
// the middle; the render's own correction finds nothing to correct after any
// page op; a letter is filter text and jumps nowhere.
func TestPopupsPageTheCursor(t *testing.T) {
	if n := len(popupOwners()); n != 6 {
		t.Fatalf("%d popups in the owner table, want the six", n)
	}
	for _, o := range popupOwners() {
		o := o
		t.Run(o.name, func(t *testing.T) {
			m := o.open(t)
			m = o.put(m, 0, 0)
			_, _, total, v := o.read(m)
			settled := func(step string) {
				t.Helper()
				if _, off, _, _ := o.read(m); o.drawn(m) != off {
					t.Errorf("%s: the render draws from %d but %d is stored", step, o.drawn(m), off)
				}
			}
			top := -o.pinned
			if o.pinned > 0 {
				// The combined cursors start on the first pinned row.
				if c, _, _, _ := o.read(m); c != -o.pinned && c != 0 {
					t.Fatalf("fixture: cursor %d", c)
				}
				m = o.put(m, 0, 0)
			}
			// Page up at the top: the first press may land on a pinned row, the rest stay.
			m, _ = press(t, m, "pgup")
			m, _ = press(t, m, "pgup")
			if c, off, _, _ := o.read(m); off != 0 || c != top {
				t.Errorf("PgUp at the top: cursor %d offset %d, want %d / 0", c, off, top)
			}
			settled("PgUp at the top")

			// Down the whole list one page at a time: the cursor never leaves its
			// window, ends on the last row and stays there.
			lastCursor := -1
			for i := 0; i < total+2; i++ {
				m, _ = press(t, m, "pgdown")
				c, off, _, _ := o.read(m)
				if c < off && c >= 0 || c > off+v+1 && c < total {
					t.Fatalf("PgDn %d: cursor %d outside its window (offset %d, %d visible)", i, c, off, v)
				}
				settled("PgDn")
				lastCursor = c
			}
			if lastCursor != total-1 {
				t.Errorf("PgDn held: cursor %d, want the last row %d", lastCursor, total-1)
			}
			for i := 0; i < total+2; i++ {
				m, _ = press(t, m, "pgup")
				settled("PgUp")
			}
			if c, off, _, _ := o.read(m); off != 0 || c != top {
				t.Errorf("PgUp held: cursor %d offset %d, want %d / 0", c, off, top)
			}
			m, _ = press(t, m, "end")
			if c, _, _, _ := o.read(m); c != total-1 {
				t.Errorf("End: cursor %d, want %d", c, total-1)
			}
			settled("End")

			// g is filter text in a popup with a filter box: it filters and jumps nowhere.
			m = o.put(m, 3, 0)
			before := o.filter(m)
			m, _ = press(t, m, "g")
			if got := o.filter(m); got != before+"g" {
				t.Errorf("g: filter text %q, want %q", got, before+"g")
			}
			m, _ = press(t, m, "G")
			if got := o.filter(m); got != before+"gG" {
				t.Errorf("G: filter text %q, want %q", got, before+"gG")
			}
		})
	}
}

// A5: the Aspects and Bodies choosers walk pinned rows (families, groups) above
// the list with one cursor. PgUp from the list's first row lands on pinned row 0,
// PgDn from a pinned row lands on the last visible list row, Home is pinned row 0
// and End the last list row.
func TestChoosersLandOnPinnedRows(t *testing.T) {
	for _, o := range popupOwners() {
		if o.pinned == 0 {
			continue
		}
		o := o
		t.Run(o.name, func(t *testing.T) {
			m := o.open(t)
			_, _, total, v := o.read(m)

			m = o.put(m, 0, 0)
			m, _ = press(t, m, "pgup")
			if c, _, _, _ := o.read(m); c != -o.pinned {
				t.Errorf("PgUp from the list's first row: cursor %d, want pinned row 0 (%d)", c, -o.pinned)
			}
			// Page down from a pinned row, window scrolled: the last visible list row.
			m = o.put(m, -2, 4)
			m, _ = press(t, m, "pgdown")
			if c, off, _, _ := o.read(m); off != 4 || c != 4+v-1 {
				t.Errorf("PgDn from a pinned row: cursor %d offset %d, want the window's last row %d with the window unmoved", c, off, 4+v-1)
			}
			// PgUp from a pinned row: pinned row 0.
			m = o.put(m, -1, 4)
			m, _ = press(t, m, "pgup")
			if c, _, _, _ := o.read(m); c != -o.pinned {
				t.Errorf("PgUp from a pinned row: cursor %d, want %d", c, -o.pinned)
			}
			// A list scrolled away from its top still pages as a list: the first
			// press goes to the window's first row, not to the pinned rows.
			m = o.put(m, 9, 9)
			m, _ = press(t, m, "pgup")
			if c, off, _, _ := o.read(m); c < 0 || off >= 9 {
				t.Errorf("PgUp from a scrolled window's first row: cursor %d offset %d, want the window paged up", c, off)
			}
			m = o.put(m, 7, 5)
			m, _ = press(t, m, "home")
			if c, off, _, _ := o.read(m); c != -o.pinned || off != 0 {
				t.Errorf("Home: cursor %d offset %d, want pinned row 0 and the list from its top", c, off)
			}
			m, _ = press(t, m, "end")
			if c, _, _, _ := o.read(m); c != total-1 {
				t.Errorf("End: cursor %d, want the last list row %d", c, total-1)
			}
		})
	}
}

// A5: Houses. A page that crosses the rule between the pinned seven and the rest
// keeps the cursor's row on screen with the rule drawn, the window one row
// shorter than the box.
func TestHousesPageCrossesTheRule(t *testing.T) {
	var o pageOwner
	for _, c := range popupOwners() {
		if c.name == "Houses popup" {
			o = c
		}
	}
	m := o.open(t)
	m = o.put(m, 0, 0)
	_, _, _, v := o.read(m)
	view := func() string {
		return ansi.Strip(renderHouseFinder(m.houseFinder, Config{Accessible: true, Color: ColorNever}, m.width, m.height))
	}
	// The rule is the dashes-only row under Campanus, the last of the pinned seven.
	ruleBelowCampanus := func(v string) (campanus, rule bool) {
		lines := strings.Split(v, "\n")
		for i, l := range lines {
			if strings.Contains(l, "Campanus") {
				if i+1 < len(lines) {
					inner := strings.Trim(strings.TrimSpace(lines[i+1]), "|│ ")
					return true, inner != "" && strings.Trim(inner, "-─") == ""
				}
				return true, false
			}
		}
		return false, false
	}
	sawRule := false
	for i, key := range []string{"pgdown", "pgdown", "pgdown", "pgup", "pgup", "end", "home", "pgdown", "pgdown"} {
		m, _ = press(t, m, key)
		if got := markedRows(view()); len(got) != 1 {
			t.Fatalf("step %d (%s): cursor %d offset %d, marked rows on screen: %q", i, key, m.houseFinder.cursor, m.houseFinder.offset, got)
		}
		_, off, _, _ := o.read(m)
		crosses := off <= len(housePinned)-1 && off+v >= len(housePinned)+1
		if campanus, rule := ruleBelowCampanus(view()); campanus && rule != crosses {
			t.Errorf("step %d (%s): window from %d: the rule drawn = %v, want %v", i, key, off, rule, crosses)
		} else if campanus && rule {
			sawRule = true
		}
	}
	if !sawRule {
		t.Error("fixture: no page put the rule in the window")
	}
}

// A5: Open chart. With unreadable files listed below, the pickable charts get a
// shorter block and the page counts that block, so the row Enter picks is always
// the one on screen with the mark.
func TestOpenChartPagesWithUnreadableBlock(t *testing.T) {
	var o pageOwner
	for _, c := range popupOwners() {
		if c.name == "Open chart popup" {
			o = c
		}
	}
	m := o.open(t)
	if len(m.picker.drops) == 0 {
		t.Fatal("fixture: no unreadable files")
	}
	for i, key := range []string{"pgdown", "pgdown", "pgdown", "pgup", "end", "pgup", "home"} {
		m, _ = press(t, m, key)
		want := m.picker.matches[m.picker.cursor].Name
		marked := markedRows(renderChartModal(m.picker, Config{Accessible: true, Color: ColorNever}, m.width, m.height))
		if len(marked) != 1 || !strings.HasPrefix(marked[0], want+" ") {
			t.Fatalf("step %d (%s): Enter would pick %q, the marked rows on screen are %q", i, key, want, marked)
		}
	}
}

// A5: Bodies chooser. The probe's status line takes a list row while the cursor
// is on a body Astrolog cannot draw; a page whose destination shows it keeps the
// cursor on screen.
func TestBodiesChooserPagesOntoTheProbeStatusRow(t *testing.T) {
	var o pageOwner
	for _, c := range popupOwners() {
		if c.name == "Bodies popup" {
			o = c
		}
	}
	m := o.open(t)
	for _, n := range []int{0, 5, 10, 15, 20, 25, 30, 35, 40, 79} {
		m.bodyModal.recordProbe(n, false, []string{"se00010.se1"})
	}
	_, _, total, v := o.read(m)
	m = o.put(m, 0, 0)
	cfg := Config{Accessible: true, Color: ColorNever}
	sawStatus := false
	for i, key := range []string{"pgdown", "pgdown", "pgdown", "pgdown", "pgup", "end", "pgup", "home", "pgdown"} {
		m, _ = press(t, m, key)
		c, off, _, _ := o.read(m)
		view := renderBodyModal(m.bodyModal, cfg, m.width, m.height)
		if c >= 0 && len(markedRows(view)) != 1 {
			t.Fatalf("step %d (%s): cursor %d offset %d of %d (%d visible): marked rows on screen %q", i, key, c, off, total, v, markedRows(view))
		}
		if strings.Contains(ansi.Strip(view), "can't be drawn") {
			sawStatus = true
		}
		if got := o.drawn(m); got != off {
			t.Errorf("step %d (%s): the render draws from %d, %d stored", i, key, got, off)
		}
	}
	if !sawStatus {
		t.Error("fixture: no step put the cursor on a body with a status row")
	}
}
