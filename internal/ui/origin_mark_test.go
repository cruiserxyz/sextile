package ui

import (
	"fmt"
	"github.com/charmbracelet/lipgloss"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/internal/ui/pane"
	"github.com/cruiserxyz/sextile/internal/ui/term"
)

// markedPanels counts the panels whose top edge carries the origin mark, `*`
// two cells left of the right corner.
func markedPanels(view string) int {
	n := 0
	for _, l := range strings.Split(ansi.Strip(view), "\n") {
		n += strings.Count(l, "-*-+") + strings.Count(l, "─✻─╮") + strings.Count(l, "-✻-+")
	}
	return n
}

// While Details owns the keyboard, the panel it was entered from shows `*` in
// the corner of its border and no other panel does; leaving Details, or never
// entering it, shows none.
func TestTheOriginPanelCarriesAMarkWhileDetailsIsEntered(t *testing.T) {
	for _, size := range []struct{ w, h int }{{144, 40}, {80, 24}} {
		for _, from := range []PanelID{PanelChart, PanelBodies, PanelAspects} {
			t.Run(fmt.Sprintf("%s %dx%d", from, size.w, size.h), func(t *testing.T) {
				m := tabsPairModel(t, Config{Accessible: true, Color: ColorNever})
				m.width, m.height = size.w, size.h
				m.focused = from
				if n := markedPanels(m.View()); n != 0 {
					t.Fatalf("%dx%d before Enter: %d marks, want none", size.w, size.h, n)
				}
				m, _ = tpress(t, m, keyEnter)
				if !m.details.attached {
					t.Fatalf("%dx%d: Enter from %s did not enter Details", size.w, size.h, from)
				}
				if n := markedPanels(m.View()); n != 1 {
					t.Errorf("%dx%d entered from %s: %d marks, want 1:\n%s", size.w, size.h, from, n, ansi.Strip(m.View()))
				}
				m, _ = tpress(t, m, keyEsc)
				if m.details.attached {
					t.Fatalf("Esc did not leave Details")
				}
				if n := markedPanels(m.View()); n != 0 {
					t.Errorf("%dx%d after Esc: %d marks, want none", size.w, size.h, n)
				}
				// Beside the panels, 0 shows Details without entering it. Below them
				// (the narrow screen) it is entered, and the mark follows.
				m, _ = tpress(t, m, runes("0"))
				if m.details.attached != (size.w < 100) || (!m.details.attached && markedPanels(m.View()) != 0) {
					t.Errorf("%dx%d Details opened with 0: attached=%v, %d marks, want a plain open", size.w, size.h, m.details.attached, markedPanels(m.View()))
				}
			})
		}
	}
}

// The mark stands on the dash two cells left of the corner and leaves the
// rest of the edge as it was; where that cell is not a dash it is left out.
func TestOriginMarkPlacement(t *testing.T) {
	cfg := Config{Accessible: false, Color: ColorNever}
	box := "╭─ Bodies ───╮\n│           │\n╰───────────╯"
	if got, want := pane.WithOriginMark(box, cfg), "╭─ Bodies ─✻─╮\n│           │\n╰───────────╯"; got != want {
		t.Errorf("unicode: %q, want %q", got, want)
	}
	ascii := Config{Accessible: true, Color: ColorNever}
	if got, want := pane.WithOriginMark("+- Bodies ---+\n|           |", ascii), "+- Bodies -*-+\n|           |"; got != want {
		t.Errorf("ascii: %q, want %q", got, want)
	}
	if got := pane.WithOriginMark("╭─ Bodies abc╮\n│", cfg); got != "╭─ Bodies abc╮\n│" {
		t.Errorf("text under the mark was covered: %q", got)
	}
}

// The glyph is one cell wide where the panels measure width, and needs no room.
func TestOriginMarkIsOneCellWide(t *testing.T) {
	if w := ansi.StringWidth(pane.OriginMark); w != 1 {
		t.Errorf("width %d, want 1", w)
	}
	if term.NeedsRoom([]rune(pane.OriginMark)[0]) {
		t.Error("the mark is a glyph that needs a space after it")
	}
}

// titleSGR is the colour code a title in colour c opens with.
func titleSGR(c lipgloss.Color, text string) string {
	out := lipgloss.NewStyle().Foreground(c).Render(text)
	return out[:strings.Index(out, text)]
}

// The panel that carries the origin mark draws its title in the focus colour
// too, while Details is entered; before that, and with colours off, it does not.
func TestTheOriginPanelTitleIsInTheFocusColour(t *testing.T) {
	forceColor(t)
	m := tabsPairModel(t, Config{Color: ColorAlways})
	m.width, m.height = 144, 40
	m.focused = PanelChart
	focus := titleSGR(m.cfg.FocusColour(), "[2] ")
	plain := titleSGR(term.TitleColour, "[2] ")
	if focus == plain {
		t.Fatal("fixture: the focus colour is the title colour")
	}
	m, _ = tpress(t, m, keyEnter)
	if !m.details.attached {
		t.Fatal("Enter did not enter Details")
	}
	v := m.View()
	if !strings.Contains(v, focus+"[2] Chart") {
		t.Errorf("the origin panel's title is not in the focus colour:\n%q", strings.Split(v, "\n")[0])
	}
	if strings.Contains(v, plain+"[2] Chart") {
		t.Error("the origin panel's title is still in the title colour")
	}
	if !strings.Contains(v, plain+"[3] Bodies") {
		t.Error("another panel's title left the title colour")
	}
	off := tabsPairModel(t, Config{Color: ColorNever})
	off.width, off.height = 144, 40
	off.focused = PanelChart
	off, _ = tpress(t, off, keyEnter)
	if v := off.View(); strings.Contains(v, focus) {
		t.Error("with colours off the frame carries the focus colour")
	}
}
