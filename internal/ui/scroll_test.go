package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// C1 — a viewport overflows only when content is hidden AND the track has
// more than one row, so a one-row body never claims a thumb.
func TestViewportOverflows(t *testing.T) {
	for _, tc := range []struct {
		name string
		v    viewport
		want bool
	}{
		{"fits exactly", viewport{Total: 30, Visible: 30}, false},
		{"fits with room", viewport{Total: 5, Visible: 30}, false},
		{"one line hidden", viewport{Total: 31, Visible: 30}, true},
		{"many hidden", viewport{Total: 300, Visible: 30}, true},
		{"one-row track", viewport{Total: 40, Visible: 1}, false},
		{"zero-row track", viewport{Total: 40, Visible: 0}, false},
		{"empty content", viewport{Total: 0, Visible: 30}, false},
		{"two-row track", viewport{Total: 40, Visible: 2}, true},
	} {
		if got := tc.v.overflows(); got != tc.want {
			t.Errorf("%s: %+v.overflows() = %v, want %v", tc.name, tc.v, got, tc.want)
		}
	}
}

// C3 — the thumb's share of the track is the content's visible fraction, so a
// shorter report gets a longer thumb. The rows are the spec's worked examples.
func TestThumbSize(t *testing.T) {
	for _, tc := range thumbFixture {
		_, size := tc.v.thumb()
		if size != tc.size {
			t.Errorf("%s: %+v thumb size = %d, want %d", tc.reads, tc.v, size, tc.size)
		}
		// C3: at least one track row always stays empty.
		if size >= tc.v.Visible {
			t.Errorf("%s: thumb of %d fills a %d-row track", tc.reads, size, tc.v.Visible)
		}
	}
}

// C4 — the thumb top is the offset's share of the slack, rounded half away
// from zero.
func TestThumbTop(t *testing.T) {
	for _, tc := range thumbFixture {
		top, size := tc.v.thumb()
		if top != tc.top {
			t.Errorf("%s: %+v thumb top = %d, want %d", tc.reads, tc.v, top, tc.top)
		}
		if top+size > tc.v.Visible {
			t.Errorf("%s: thumb [%d,%d) runs past a %d-row track", tc.reads, top, top+size, tc.v.Visible)
		}
	}
}

// thumbFixture is the spec's C3/C4 table: the worked examples of "thumb
// length tells you how much is left". A Details panel at 132x40 has
// Visible = 30; the last row is a Bodies planet window.
var thumbFixture = []struct {
	v         viewport
	size, top int
	reads     string
}{
	{viewport{Visible: 30, Total: 31}, 29, 0, "one line hidden — thumb fills all but one row"},
	{viewport{Visible: 30, Total: 35}, 26, 0, "a few lines hidden"},
	{viewport{Visible: 30, Total: 52}, 17, 0, "the earlier change influence chart, top of report"},
	{viewport{Visible: 30, Total: 52, Offset: 11}, 17, 7, "same report, middle"},
	{viewport{Visible: 30, Total: 52, Offset: 22}, 17, 13, "same report, last page — thumb touches bottom"},
	{viewport{Visible: 30, Total: 60, Offset: 30}, 15, 15, "exactly half hidden — thumb is half the track"},
	{viewport{Visible: 30, Total: 120}, 8, 0, "four screens of content"},
	{viewport{Visible: 30, Total: 300}, 3, 0, "ten screens"},
	{viewport{Visible: 30, Total: 1000}, 1, 0, "floor: never smaller than one row"},
	{viewport{Visible: 9, Total: 14, Offset: 5}, 6, 3, "Bodies: 9 planet rows visible of 14"},
}

// C5 — the thumb touches the first track row at the top of the content and
// the last track row at the bottom, at every shape of report.
func TestThumbTouchesBothEnds(t *testing.T) {
	for _, total := range []int{31, 35, 52, 60, 120, 300, 1000} {
		for _, visible := range []int{2, 5, 9, 30} {
			if total <= visible {
				continue
			}
			top, size := viewport{Total: total, Visible: visible}.thumb()
			if top != 0 {
				t.Errorf("total=%d visible=%d offset=0: top = %d, want 0", total, visible, top)
			}
			top, size = viewport{Total: total, Visible: visible, Offset: total - visible}.thumb()
			if top+size != visible {
				t.Errorf("total=%d visible=%d at the end: thumb ends at %d, want the last track row %d",
					total, visible, top+size, visible)
			}
		}
	}
}

// Failure mode — thumb() never divides by zero, and reports no thumb where
// there is nothing to report.
func TestThumbNoDivideByZero(t *testing.T) {
	for _, v := range []viewport{
		{}, {Total: 30, Visible: 30}, {Total: 30, Visible: 30, Offset: 9},
		{Total: 0, Visible: 0}, {Total: 5, Visible: 0}, {Total: 5, Visible: 1},
		{Total: -3, Visible: 4}, {Total: 10, Visible: 4, Offset: -8},
	} {
		top, size := v.thumb()
		if v.overflows() {
			continue
		}
		if top != 0 || size != 0 {
			t.Errorf("%+v: thumb = (%d, %d), want no thumb", v, top, size)
		}
	}
}

// C2 — a panel with nothing hidden renders byte-identical to the same panel
// rendered without an indicator at all.
func TestNoIndicatorWhenContentFits(t *testing.T) {
	forceColor(t)
	body := strings.Join([]string{"alpha", "beta", "gamma"}, "\n")
	for _, cfg := range []Config{{Accessible: true}, {Color: ColorAlways}, {Color: ColorNever}} {
		for _, focused := range []bool{false, true} {
			for _, v := range []viewport{
				{}, {Total: 3, Visible: 8}, {Total: 8, Visible: 8}, {Total: 40, Visible: 1},
			} {
				box := renderTitledBox("[0] Details", 40, 10, focused, cfg, body)
				if got := withScrollIndicator(box, v, cfg, focused); got != box {
					t.Errorf("cfg=%+v focused=%v %+v: indicator drawn on a panel that fits:\n%s", cfg, focused, v, got)
				}
			}
		}
	}
}

// C6 — the indicator replaces the right border glyph on the thumb's rows and
// changes no other cell of the panel.
func TestIndicatorTouchesOnlyRightBorder(t *testing.T) {
	const w, h = 40, 12
	cfg := Config{Accessible: true}
	v := viewport{Total: 52, Visible: h - 2, Offset: 11}
	top, size := v.thumb()

	box := renderTitledBox("[0] Details", w, h, false, cfg, strings.Repeat("x\n", 9)+"x")
	got := withScrollIndicator(box, v, cfg, false)

	before := strings.Split(box, "\n")
	after := strings.Split(got, "\n")
	if len(before) != len(after) {
		t.Fatalf("indicator changed the panel height: %d -> %d", len(before), len(after))
	}
	for i := range before {
		wantRow := []rune(ansi.Strip(before[i]))
		gotRow := []rune(ansi.Strip(after[i]))
		if len(wantRow) != len(gotRow) {
			t.Fatalf("row %d changed width: %q -> %q", i, before[i], after[i])
		}
		onThumb := i >= 1+top && i < 1+top+size
		for col := range wantRow {
			expect := wantRow[col]
			if onThumb && col == w-1 {
				expect = []rune(scrollThumbASCII)[0]
			}
			if gotRow[col] != expect {
				t.Errorf("row %d col %d = %q, want %q (onThumb=%v)", i, col, gotRow[col], expect, onThumb)
			}
		}
	}
}

// C7 — the thumb inherits the panel's border colour, so it brightens with
// focus exactly as the border does and never introduces a colour of its own.
func TestIndicatorUsesBorderColor(t *testing.T) {
	forceColor(t)
	cfg := Config{Color: ColorAlways}
	v := viewport{Total: 52, Visible: 10, Offset: 0}
	for _, focused := range []bool{false, true} {
		box := renderTitledBox("[0] Details", 40, 12, focused, cfg, strings.Repeat("x\n", 9)+"x")
		got := withScrollIndicator(box, v, cfg, focused)
		want := lipgloss.NewStyle().Foreground(panelBorderColor(focused, Config{})).Render(scrollThumbGlyph)
		if !strings.Contains(got, want) {
			t.Errorf("focused=%v: thumb %q not rendered in the border colour", focused, want)
		}
		// The colour must be the one the side border already uses: the SGR
		// introducing the thumb also introduces untouched border rows.
		sgr := strings.SplitN(want, scrollThumbGlyph, 2)[0]
		if sgr == "" {
			t.Fatal("no SGR emitted for the thumb; colour profile not forced?")
		}
		if !strings.Contains(box, sgr) {
			t.Errorf("focused=%v: thumb colour %q is not the panel's border colour", focused, sgr)
		}
	}
	// The two focus states must actually differ, or C7 proves nothing.
	unfocused := withScrollIndicator(renderTitledBox("[0] Details", 40, 12, false, cfg, "x"), v, cfg, false)
	focusedBox := withScrollIndicator(renderTitledBox("[0] Details", 40, 12, true, cfg, "x"), v, cfg, true)
	if strings.Contains(unfocused, strings.SplitN(lipgloss.NewStyle().Foreground(panelBorderColor(true, Config{})).Render(scrollThumbGlyph), scrollThumbGlyph, 2)[0]) {
		t.Error("the unfocused thumb carries the focused border colour")
	}
	if unfocused == focusedBox {
		t.Error("focus did not change the indicator at all")
	}
}

// C8 — an ASCII frame keeps an ASCII indicator: thumb `#`, rule `-`, tees
// `+`, and no box-drawing glyph anywhere. Where colour alone is unavailable
// the frame stays Unicode, and the heavy thumb still reads against the light
// side border with no colour at all.
func TestIndicatorAccessible(t *testing.T) {
	body := strings.Repeat("x\n", 9) + "x"
	v := viewport{Total: 52, Visible: 10, Offset: 4}

	ascii := withPanelRule(withScrollIndicator(
		renderTitledBox("[2] Bodies", 40, 12, false, Config{Accessible: true}, body),
		v, Config{Accessible: true}, false), 6, Config{Accessible: true}, false)
	if !strings.Contains(ascii, scrollThumbASCII) {
		t.Errorf("accessible thumb missing:\n%s", ascii)
	}
	if !strings.Contains(ascii, panelRuleTeeASCII+strings.Repeat(panelRuleASCII, 38)+panelRuleTeeASCII) {
		t.Errorf("accessible rule missing:\n%s", ascii)
	}
	for _, r := range ascii {
		if r >= 0x2500 && r <= 0x257F {
			t.Errorf("accessible frame emitted box-drawing glyph %q:\n%s", r, ascii)
			break
		}
	}
	if strings.ContainsRune(ascii, '\x1b') {
		t.Errorf("accessible frame emitted ANSI:\n%s", ascii)
	}

	// --color=never / NO_COLOR: no SGR, but the frame keeps its Unicode
	// borders, so the thumb stays the heavy bar that contrasts with `│`.
	plain := withScrollIndicator(renderTitledBox("[0] Details", 40, 12, false, Config{Color: ColorNever}, body), v, Config{Color: ColorNever}, false)
	if strings.ContainsRune(plain, '\x1b') {
		t.Errorf("--color=never emitted ANSI:\n%s", plain)
	}
	if !strings.Contains(plain, scrollThumbGlyph) {
		t.Errorf("--color=never lost the thumb:\n%s", plain)
	}
	if scrollThumbGlyph == "│" {
		t.Error("the thumb must not be the side border glyph: with no colour it would be invisible")
	}
}

// C25 (rule half) — the rule joins both side borders on exactly one row and
// leaves every other row alone.
func TestPanelRuleSpansBorders(t *testing.T) {
	const w, h, row = 40, 12, 5
	cfg := Config{Accessible: true}
	box := renderTitledBox("[2] Bodies", w, h, false, cfg, strings.Repeat("x\n", 9)+"x")
	got := withPanelRule(box, row, cfg, false)

	before, after := strings.Split(box, "\n"), strings.Split(got, "\n")
	for i := range before {
		if i == 1+row {
			want := panelRuleTeeASCII + strings.Repeat(panelRuleASCII, w-2) + panelRuleTeeASCII
			if after[i] != want {
				t.Errorf("rule row = %q, want %q", after[i], want)
			}
			continue
		}
		if after[i] != before[i] {
			t.Errorf("row %d changed: %q -> %q", i, before[i], after[i])
		}
	}
	if got := withPanelRule(box, -1, cfg, false); got != box {
		t.Error("row -1 must leave the box untouched")
	}
}
