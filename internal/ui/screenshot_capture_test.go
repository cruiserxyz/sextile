package ui

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/store"
)

// the README screenshot.
//
// docs/screenshot.svg is one real frame of the program, rendered by the
// model's own View() through the same capture lane as the demo frames
// (demo_capture_test.go) and converted to SVG by the test-only converter
// below. It is regenerated, never edited:
//
//	go test ./internal/ui -run '^TestReadmeScreenshotScene$' -demo
//
// Every fact in the scene derives from the solar eclipse fixture
// (internal/astrolog/testdata/charts/Eclipse-2026-08-12.as): an event chart
// with no person and no birth data, so the picture shows nobody's record.

const (
	screenshotPath = "../../docs/screenshot.svg"
	screenshotCols = 144
	screenshotRows = 40

	// The alt text is the SVG's <title> and, word for word, the README's
	// image description.
	screenshotAlt = "Sextile showing the solar eclipse of 12 August 2026 beside the sky as Venus moves back into Libra: bodies, aspects and the month's timeline"
)

// screenshotAnchor is Sun Oct 25, 2026 09:00 in Hamburg (CET), the frozen
// clock of the scene.
var screenshotAnchor = time.Date(2026, 10, 25, 8, 0, 0, 0, time.UTC)

var screenshotHamburg = location.PlaceInfo{Name: "Hamburg", Country: "DE", Lat: 53.5507, Lon: 9.993, TZ: "Europe/Berlin"}

// screenshotScene plays the README's walk through the real model with real
// Astrolog: the eclipse fixture opened from a charts folder, the cursor on the
// floating chart, Details on its Timeline tab and Enter on the row where Venus
// moves back into Libra. It returns the coloured frame.
//
// The 144-column layout is the narrowest that shows Details beside the three
// panels (132 stacks them and hides Details), and docs/REFERENCE.md names it
// as the size of the screen the README describes.
func screenshotScene(t *testing.T) string {
	t.Helper()
	needLiveAstrolog(t)
	forceColor(t)
	const file = "Eclipse-2026-08-12.as"
	titleLibrary(t, []string{file}, nil)

	m := NewModelWithClock(Config{Color: ColorAlways}, FrozenClock(screenshotAnchor))
	m.startup = startupReady
	m.place = screenshotHamburg
	slots, err := store.NewSlots(screenshotAnchor, screenshotHamburg)
	if err != nil {
		t.Fatalf("NewSlots: %v", err)
	}
	m.slots = slots
	m.targetFloatingTime = slots.Floating().Time
	m.width, m.height = screenshotCols, screenshotRows

	m = pickLive(t, m, file)
	// `0` cycles Details to its Timeline tab (the floating chart is in the
	// selected group, so the tab lists the month), the first enter attaches it
	// with the cursor on the row nearest the floating moment (Venus's return
	// to Libra, 69 minutes after the frozen clock), and the second enter presses
	// that row, which moves the floating chart to 10:09.
	for _, k := range []tea.KeyMsg{
		{Type: tea.KeyRunes, Runes: []rune{'0'}},
		{Type: tea.KeyEnter},
		{Type: tea.KeyEnter},
	} {
		u, cmd := m.Update(k)
		m = pumpPatient(t, u.(Model), cmd)
	}
	return m.View()
}

// TestReadmeScreenshotScene checks the scene says what the README says it does
// on every run, so the capture cannot rot between regenerations, and writes
// docs/screenshot.svg only under -demo.
func TestReadmeScreenshotScene(t *testing.T) {
	frame := screenshotScene(t)
	assertFrameGeometry(t, frame, screenshotCols, screenshotRows)
	plain := ansi.Strip(frame)
	for _, want := range []string{
		"① Eclipse-2026-08-12  Wed Aug 12, 2026 19:37 CEST",
		"② ~                   Sun Oct 25, 2026 10:09 CET",
		"[0] Chart - Timeline",
		"Sun 10/25/2026   10:09         Venus [Sco] <-- Libra",
	} {
		if !strings.Contains(plain, want) {
			t.Errorf("the frame lacks %q:\n%s", want, plain)
		}
	}
	// Crawl safety: nothing of the machine that made the frame (the release
	// crawl checks the committed picture for every other banned word).
	for _, banned := range []string{"/tmp", "/home", os.TempDir()} {
		if strings.Contains(plain, banned) {
			t.Errorf("the frame carries %q:\n%s", banned, plain)
		}
	}
	assertAstrologColour(t, []demoFrame{{"screenshot", frame}})

	if !*demoCapture {
		t.Skip("frame checked; pass -demo to write docs/screenshot.svg")
	}
	svg := ansiToSVG(frame, screenshotAlt)
	if err := os.WriteFile(screenshotPath, svg, 0o644); err != nil {
		t.Fatalf("write %s: %v", screenshotPath, err)
	}
	t.Logf("wrote %s (%d bytes)", filepath.Clean(screenshotPath), len(svg))
}

// TestReadmeScreenshotIsCommitted holds the committed picture to what the
// README promises: it exists, it is well-formed XML, it carries the alt text
// as its title and the chart's title, and nothing of the machine that made it.
func TestReadmeScreenshotIsCommitted(t *testing.T) {
	b, err := os.ReadFile(screenshotPath)
	if err != nil {
		t.Fatalf("docs/screenshot.svg is missing (regenerate with -demo, see screenshot_capture_test.go): %v", err)
	}
	var title string
	dec := xml.NewDecoder(bytes.NewReader(b))
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("docs/screenshot.svg is not well-formed XML: %v", err)
		}
		if se, ok := tok.(xml.StartElement); ok && se.Name.Local == "title" {
			var s string
			if err := dec.DecodeElement(&s, &se); err != nil {
				t.Fatalf("reading <title>: %v", err)
			}
			title = s
		}
	}
	if title != screenshotAlt {
		t.Errorf("<title> = %q, want the alt text %q", title, screenshotAlt)
	}
	if !strings.Contains(string(b), "Eclipse-2026-08-12") {
		t.Error("docs/screenshot.svg does not show the eclipse chart")
	}
	for _, banned := range []string{"/tmp", "/home"} {
		if strings.Contains(string(b), banned) {
			t.Errorf("docs/screenshot.svg carries %q", banned)
		}
	}
}

// TestAnsiToSVGSpans pins the converter on a hand-made frame: positions come
// from cells (a wide glyph takes two), colours from all three SGR forms, and a
// box-drawing run lands where its column says.
func TestAnsiToSVGSpans(t *testing.T) {
	const esc = "\x1b["
	frame := strings.Join([]string{
		"╭─" + esc + "1;31mA" + esc + "0m" + esc + "38;5;75mB" + esc + "0m" + esc + "38;2;1;2;3mC" + esc + "0m",
		"世x" + esc + "48;5;196m  " + esc + "0mz",
		"│x──",
		"█▌",
	}, "\n")
	svg := string(ansiToSVG(frame, "alt"))
	for _, r := range "│─╭█▌" {
		if strings.ContainsRune(svg, r) {
			t.Errorf("the SVG draws %q as text:\n%s", r, svg)
		}
	}
	if strings.Contains(svg, "textLength") {
		t.Errorf("the SVG still stretches runs with textLength:\n%s", svg)
	}
	for _, want := range []string{
		`<title>alt</title>`,
		`<line x1="4.2" y1="36.0" x2="4.2" y2="54.0" stroke-width="1.2" stroke="#cccccc"/>`,               // │ fills its cell's height, centred
		`<line x1="16.8" y1="45.0" x2="33.6" y2="45.0" stroke-width="1.2" stroke="#cccccc"/>`,             // ── is one line across both cells
		`<rect x="0.0" y="54.0" width="12.6" height="18.0" fill="#cccccc" shape-rendering="crispEdges"/>`, // █▌ merge into one rect: 8.4 + 4.2
		`x="8.4"`,            // the x after │ is drawn as text
		`x="16.8"`,           // A: two cells in
		`x="0.0 16.8"`,       // 世 takes two cells, so x jumps two cells
		`fill="#cd3131"`,     // 1;31 is red, bold
		`font-weight="bold"`, // from the 1
		`fill="#5fafff"`,     // 256-colour 75
		`fill="#010203"`,     // true colour
		`<rect x="25.2" y="18.0" width="16.8" height="18.0" fill="#ff0000"`, // 世 is two cells, so the red pad starts at column 3
	} {
		if !strings.Contains(svg, want) {
			t.Errorf("the SVG lacks %s:\n%s", want, svg)
		}
	}
}

// --- the converter -------------------------------------------------------

const (
	svgCellW    = 8.4 // 0.6 em of the 14 px font: a monospace cell
	svgCellH    = 18.0
	svgFontSize = 14
	svgBaseline = 14.0
	svgDefaultF = "#cccccc"
	svgDefaultB = "#1e1e1e"
	svgFontFam  = "ui-monospace, SFMono-Regular, Menlo, Consolas, 'DejaVu Sans Mono', monospace"
)

var svgBasic = [16]string{
	"#4e4e4e", "#cd3131", "#0dbc79", "#e5e510", "#2472c8", "#bc3fbc", "#11a8cd", "#e5e5e5",
	"#666666", "#f14c4c", "#23d18b", "#f5f543", "#3b8eea", "#d670d6", "#29b8db", "#ffffff",
}

// svgStyle is the SGR state that survives into a span. fg and bg are "" for
// the terminal's defaults.
type svgStyle struct {
	fg, bg                                 string
	bold, faint, italic, underline, invert bool
}

// svgSpan is a run of cells sharing one style. at holds the starting column
// of each rune of text (a zero-width mark repeats its base's).
type svgSpan struct {
	row, col, cells int
	text            string
	at              []int
	st              svgStyle
}

func svg256(n int) string {
	switch {
	case n < 16:
		return svgBasic[n]
	case n < 232:
		n -= 16
		lv := [6]int{0, 95, 135, 175, 215, 255}
		return fmt.Sprintf("#%02x%02x%02x", lv[n/36], lv[n/6%6], lv[n%6])
	default:
		g := 8 + 10*(n-232)
		return fmt.Sprintf("#%02x%02x%02x", g, g, g)
	}
}

// apply folds one SGR parameter list into the style.
func (s svgStyle) apply(params string) svgStyle {
	if params == "" {
		return svgStyle{}
	}
	p := strings.Split(params, ";")
	num := func(i int) int {
		if i >= len(p) {
			return 0
		}
		n, _ := strconv.Atoi(p[i])
		return n
	}
	for i := 0; i < len(p); i++ {
		switch n := num(i); {
		case n == 0:
			s = svgStyle{}
		case n == 1:
			s.bold = true
		case n == 2:
			s.faint = true
		case n == 3:
			s.italic = true
		case n == 4:
			s.underline = true
		case n == 7:
			s.invert = true
		case n == 22:
			s.bold, s.faint = false, false
		case n == 23:
			s.italic = false
		case n == 24:
			s.underline = false
		case n == 27:
			s.invert = false
		case n >= 30 && n <= 37:
			s.fg = svgBasic[n-30]
		case n >= 90 && n <= 97:
			s.fg = svgBasic[n-90+8]
		case n >= 40 && n <= 47:
			s.bg = svgBasic[n-40]
		case n >= 100 && n <= 107:
			s.bg = svgBasic[n-100+8]
		case n == 39:
			s.fg = ""
		case n == 49:
			s.bg = ""
		case n == 38 || n == 48:
			var c string
			switch num(i + 1) {
			case 5:
				c, i = svg256(num(i+2)), i+2
			case 2:
				c, i = fmt.Sprintf("#%02x%02x%02x", num(i+2), num(i+3), num(i+4)), i+4
			}
			if n == 38 {
				s.fg = c
			} else {
				s.bg = c
			}
		}
	}
	return s
}

// colours resolves the style to the fill of its text and of its cell.
func (s svgStyle) colours() (fg, bg string) {
	fg, bg = s.fg, s.bg
	if fg == "" {
		fg = svgDefaultF
	}
	if bg == "" {
		bg = svgDefaultB
	}
	if s.invert {
		fg, bg = bg, fg
	}
	return fg, bg
}

// svgSpans splits the frame into same-style runs by cell position. A rune's
// width is the one the layout counted (ansi.StringWidth), so a wide glyph
// advances two columns and a zero-width mark stays on its base.
func svgSpans(frame string) (spans []svgSpan, rows, cols int) {
	for row, line := range strings.Split(frame, "\n") {
		rows = row + 1
		var st svgStyle
		var cur *svgSpan
		col := 0
		rs := []rune(line)
		for i := 0; i < len(rs); i++ {
			if rs[i] == 0x1b && i+1 < len(rs) && rs[i+1] == '[' {
				j := i + 2
				for j < len(rs) && (rs[j] < 0x40 || rs[j] > 0x7e) {
					j++
				}
				if j < len(rs) && rs[j] == 'm' {
					st = st.apply(string(rs[i+2 : j]))
					cur = nil
				}
				i = j
				continue
			}
			w := ansi.StringWidth(string(rs[i]))
			if w == 0 && cur != nil {
				cur.text += string(rs[i])
				cur.at = append(cur.at, col-1)
				continue
			}
			if cur == nil {
				spans = append(spans, svgSpan{row: row, col: col, st: st})
				cur = &spans[len(spans)-1]
			}
			cur.text += string(rs[i])
			cur.at = append(cur.at, col)
			cur.cells += w
			col += w
		}
		if col > cols {
			cols = col
		}
	}
	return spans, rows, cols
}

// ansiToSVG draws the frame as a dark SVG: a rectangle per span with its own
// background, and a text run per span with one absolute x per glyph at
// column*cell width, so a glyph whose fallback font has another advance
// cannot push its neighbours. Box-drawing runes are not text at all: a font's
// glyph stops short of the 18 px row and its advance is not the cell, so they
// are line segments that fill their cell exactly, and block elements (the
// Bodies speed bars) are rectangles. The only dependency is the
// standard library.
func ansiToSVG(frame, alt string) []byte {
	spans, rows, cols := svgSpans(strings.TrimRight(frame, "\n"))
	var b bytes.Buffer
	esc := func(s string) string {
		var e bytes.Buffer
		_ = xml.EscapeText(&e, []byte(s))
		return e.String()
	}
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" width="%.0f" height="%.0f" viewBox="0 0 %.1f %.1f" role="img" xml:space="preserve">`+"\n",
		float64(cols)*svgCellW, float64(rows)*svgCellH, float64(cols)*svgCellW, float64(rows)*svgCellH)
	fmt.Fprintf(&b, "<title>%s</title>\n", esc(alt))
	fmt.Fprintf(&b, `<rect width="100%%" height="100%%" fill="%s"/>`+"\n", svgDefaultB)
	fmt.Fprintf(&b, `<g shape-rendering="crispEdges">`+"\n")
	for _, sp := range spans {
		if _, bg := sp.st.colours(); bg != svgDefaultB {
			fmt.Fprintf(&b, `<rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" fill="%s"/>`+"\n",
				float64(sp.col)*svgCellW, float64(sp.row)*svgCellH, float64(sp.cells)*svgCellW, svgCellH, bg)
		}
	}
	fmt.Fprintf(&b, "</g>\n")
	fmt.Fprintf(&b, `<g font-family="%s" font-size="%d">`+"\n", svgFontFam, svgFontSize)
	for _, sp := range spans {
		// Spaces draw nothing; dropping them keeps the file small, and the
		// x list skips their cells.
		fg, _ := sp.st.colours()
		var text strings.Builder
		var xs []string
		var box svgBoxDrawing
		var blocks svgBlocks
		for k, r := range []rune(sp.text) {
			if r == ' ' {
				continue
			}
			if g, ok := svgBlockGlyph(r); ok {
				blocks.add(g, float64(sp.at[k])*svgCellW, float64(sp.row)*svgCellH)
				continue
			}
			if g, ok := svgBoxGlyphs[r]; ok {
				box.add(g, float64(sp.at[k])*svgCellW, float64(sp.row)*svgCellH)
				continue
			}
			text.WriteRune(r)
			xs = append(xs, fmt.Sprintf("%.1f", float64(sp.at[k])*svgCellW))
		}
		opacity := ""
		if sp.st.faint {
			opacity = ` opacity="0.6"`
		}
		blocks.write(&b, fg, sp.st.faint)
		box.write(&b, fg, opacity)
		if text.Len() == 0 {
			continue
		}
		var attrs strings.Builder
		fmt.Fprintf(&attrs, `fill="%s"`, fg)
		if sp.st.bold {
			attrs.WriteString(` font-weight="bold"`)
		}
		if sp.st.italic {
			attrs.WriteString(` font-style="italic"`)
		}
		attrs.WriteString(opacity)
		if sp.st.underline {
			attrs.WriteString(` text-decoration="underline"`)
		}
		fmt.Fprintf(&b, `<text x="%s" y="%.1f" %s>%s</text>`+"\n",
			strings.Join(xs, " "), float64(sp.row)*svgCellH+svgBaseline, attrs.String(), esc(text.String()))
	}
	fmt.Fprintf(&b, "</g>\n</svg>\n")
	return b.Bytes()
}

// svgBoxGlyph names the arms a box-drawing rune draws from its cell centre to
// the cell edge: l, r, u, d. Rounded corners are drawn square.
type svgBoxGlyph struct {
	arms          string
	heavy, dashed bool
}

var svgBoxGlyphs = func() map[rune]svgBoxGlyph {
	m := map[rune]svgBoxGlyph{}
	for arms, runes := range map[string]string{
		"lr": "─", "ud": "│", "rd": "┌╭", "ld": "┐╮", "ru": "└╰", "lu": "┘╯",
		"rud": "├", "lud": "┤", "lrd": "┬", "lru": "┴", "lrud": "┼",
	} {
		for _, r := range runes {
			m[r] = svgBoxGlyph{arms: arms}
		}
	}
	for arms, runes := range map[string]string{
		"lr": "━", "ud": "┃", "rd": "┏", "ld": "┓", "ru": "┗", "lu": "┛",
		"rud": "┣", "lud": "┫", "lrd": "┳", "lru": "┻", "lrud": "╋",
	} {
		for _, r := range runes {
			m[r] = svgBoxGlyph{arms: arms, heavy: true}
		}
	}
	for arms, runes := range map[string]string{"lr": "┄┈╌", "ud": "┆┊╎"} {
		for _, r := range runes {
			m[r] = svgBoxGlyph{arms: arms, dashed: true}
		}
	}
	for arms, runes := range map[string]string{"lr": "┅┉╍", "ud": "┇┋╏"} {
		for _, r := range runes {
			m[r] = svgBoxGlyph{arms: arms, heavy: true, dashed: true}
		}
	}
	return m
}()

// svgBoxDrawing collects the segments of one span's box-drawing runes, so a
// run of ─ becomes one line.
type svgBoxDrawing struct {
	elems []string // finished vertical lines and corner paths
	hs    []svgHSeg
}

type svgHSeg struct {
	x0, x1, y     float64
	heavy, dashed bool
}

func svgStrokeWidth(heavy bool) float64 {
	if heavy {
		return 2.4
	}
	return 1.2
}

// add draws g's arms in the cell whose top-left corner is (x, y).
func (d *svgBoxDrawing) add(g svgBoxGlyph, x, y float64) {
	cx, cy := x+svgCellW/2, y+svgCellH/2
	l, r := strings.Contains(g.arms, "l"), strings.Contains(g.arms, "r")
	u, dn := strings.Contains(g.arms, "u"), strings.Contains(g.arms, "d")
	if (l != r) && (u != dn) {
		// A corner is one polyline, so its outer corner has no notch.
		ex, ey := x, y
		if r {
			ex = x + svgCellW
		}
		if dn {
			ey = y + svgCellH
		}
		d.elems = append(d.elems, fmt.Sprintf(`<path d="M%.1f %.1f L%.1f %.1f L%.1f %.1f" fill="none" stroke-width="%.1f"/>`,
			ex, cy, cx, cy, cx, ey, svgStrokeWidth(g.heavy)))
		return
	}
	if l || r {
		x0, x1 := x, x+svgCellW
		if !l {
			x0 = cx
		}
		if !r {
			x1 = cx
		}
		if n := len(d.hs); n > 0 && d.hs[n-1].y == cy && d.hs[n-1].x1 >= x0-0.05 &&
			d.hs[n-1].heavy == g.heavy && d.hs[n-1].dashed == g.dashed {
			d.hs[n-1].x1 = x1
		} else {
			d.hs = append(d.hs, svgHSeg{x0, x1, cy, g.heavy, g.dashed})
		}
	}
	if u || dn {
		y0, y1 := y, y+svgCellH
		if !u {
			y0 = cy
		}
		if !dn {
			y1 = cy
		}
		dash := ""
		if g.dashed {
			dash = ` stroke-dasharray="4.5 4.5"`
		}
		d.elems = append(d.elems, fmt.Sprintf(`<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke-width="%.1f"%s/>`,
			cx, y0, cx, y1, svgStrokeWidth(g.heavy), dash))
	}
}

// write emits the collected segments in the span's colour.
func (d *svgBoxDrawing) write(b *bytes.Buffer, fg, opacity string) {
	for _, h := range d.hs {
		dash := ""
		if h.dashed {
			dash = ` stroke-dasharray="2.1 2.1"`
		}
		d.elems = append(d.elems, fmt.Sprintf(`<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke-width="%.1f"%s/>`,
			h.x0, h.y, h.x1, h.y, svgStrokeWidth(h.heavy), dash))
	}
	for _, e := range d.elems {
		fmt.Fprintf(b, "%s stroke=\"%s\"%s/>\n", strings.TrimSuffix(e, "/>"), fg, opacity)
	}
}

// svgBlock is the part of a cell a block element fills, as fractions of the
// cell, at an opacity for the shades.
type svgBlock struct {
	x, y, w, h float64
	opacity    float64
}

// svgBlockGlyph resolves the left-block ramp (▏ one eighth to █ the whole
// cell), the right, upper and lower halves and the three shades.
func svgBlockGlyph(r rune) (svgBlock, bool) {
	if r >= 0x2588 && r <= 0x258f {
		return svgBlock{0, 0, float64(0x2590-r) / 8, 1, 1}, true
	}
	switch r {
	case 0x2580:
		return svgBlock{0, 0, 1, 0.5, 1}, true
	case 0x2584:
		return svgBlock{0, 0.5, 1, 0.5, 1}, true
	case 0x2590:
		return svgBlock{0.5, 0, 0.5, 1, 1}, true
	case 0x2591:
		return svgBlock{0, 0, 1, 1, 0.25}, true
	case 0x2592:
		return svgBlock{0, 0, 1, 1, 0.5}, true
	case 0x2593:
		return svgBlock{0, 0, 1, 1, 0.75}, true
	}
	return svgBlock{}, false
}

type svgBlockRect struct {
	x, y, w, h, opacity float64
}

// svgBlocks collects one span's block rectangles; neighbours that touch and
// match are one rectangle, so a bar is a single rect however many cells it
// covers.
type svgBlocks struct{ rects []svgBlockRect }

func (d *svgBlocks) add(g svgBlock, x, y float64) {
	r := svgBlockRect{x + g.x*svgCellW, y + g.y*svgCellH, g.w * svgCellW, g.h * svgCellH, g.opacity}
	if n := len(d.rects); n > 0 {
		p := &d.rects[n-1]
		if p.y == r.y && p.h == r.h && p.opacity == r.opacity && math.Abs(p.x+p.w-r.x) < 0.05 {
			p.w = r.x + r.w - p.x
			return
		}
	}
	d.rects = append(d.rects, r)
}

func (d *svgBlocks) write(b *bytes.Buffer, fg string, faint bool) {
	for _, r := range d.rects {
		op := r.opacity
		if faint {
			op *= 0.6
		}
		attr := ""
		if op != 1 {
			attr = fmt.Sprintf(` opacity="%.2f"`, op)
		}
		fmt.Fprintf(b, `<rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" fill="%s"%s shape-rendering="crispEdges"/>`+"\n",
			r.x, r.y, r.w, r.h, fg, attr)
	}
}
