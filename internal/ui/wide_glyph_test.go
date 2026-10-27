package ui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// sgrState is the style a cell is drawn in: the parts of an SGR sequence that
// show on a blank cell are the colours and the reverse video, but bold, faint,
// italic and underline are held too, so "the same style" means the same style.
type sgrState struct {
	fg, bg string
	attrs  map[int]bool
}

func (s sgrState) key() string {
	var as []int
	for a, on := range s.attrs {
		if on {
			as = append(as, a)
		}
	}
	sort.Ints(as)
	return fmt.Sprintf("fg=%s bg=%s attrs=%v", s.fg, s.bg, as)
}

// apply folds one SGR parameter list into the state.
func (s *sgrState) apply(params string) {
	if s.attrs == nil {
		s.attrs = map[int]bool{}
	}
	if params == "" {
		params = "0"
	}
	parts := strings.Split(params, ";")
	for i := 0; i < len(parts); i++ {
		var n int
		fmt.Sscanf(parts[i], "%d", &n)
		switch {
		case n == 0:
			s.fg, s.bg, s.attrs = "", "", map[int]bool{}
		case n == 1 || n == 2 || n == 3 || n == 4 || n == 7 || n == 9:
			s.attrs[n] = true
		case n == 22:
			delete(s.attrs, 1)
			delete(s.attrs, 2)
		case n == 23 || n == 24 || n == 27 || n == 29:
			delete(s.attrs, n-20)
		case n >= 30 && n <= 37, n >= 90 && n <= 97:
			s.fg = parts[i]
		case n == 39:
			s.fg = ""
		case n >= 40 && n <= 47, n >= 100 && n <= 107:
			s.bg = parts[i]
		case n == 49:
			s.bg = ""
		case n == 38 || n == 48:
			// 38;5;n or 38;2;r;g;b
			take := 2
			if i+1 < len(parts) && parts[i+1] == "2" {
				take = 4
			}
			end := min(i+1+take, len(parts))
			v := strings.Join(parts[i:end], ";")
			if n == 38 {
				s.fg = v
			} else {
				s.bg = v
			}
			i = end - 1
		}
	}
}

// styledCell is one cell of a frame: a rune and the style it is drawn in.
type styledCell struct {
	r     rune
	style string
}

// frameCells reads a line into cells, following its SGR sequences.
func frameCells(line string) []styledCell {
	var cells []styledCell
	var st sgrState
	for i := 0; i < len(line); {
		if line[i] == 0x1b && i+1 < len(line) && line[i+1] == '[' {
			j := i + 2
			for j < len(line) && (line[j] == ';' || (line[j] >= '0' && line[j] <= '9')) {
				j++
			}
			if j < len(line) && line[j] == 'm' {
				st.apply(line[i+2 : j])
			}
			i = j + 1
			continue
		}
		r, size := rune(line[i]), 1
		if r >= 0x80 {
			for _, rr := range line[i:] {
				r = rr
				break
			}
			size = len(string(r))
		}
		cells = append(cells, styledCell{r: r, style: st.key()})
		i += size
	}
	return cells
}

// wideGlyphFaults walks a frame, line by line and cell by cell, and returns
// every roomy glyph that is not followed by a space in its own style.
func wideGlyphFaults(frame string) []string {
	var out []string
	for n, line := range strings.Split(frame, "\n") {
		cells := frameCells(line)
		for i, c := range cells {
			if !needsRoom(c.r) {
				continue
			}
			if i+1 < len(cells) && cells[i+1].r == ' ' && cells[i+1].style == c.style {
				continue
			}
			from, to := max(i-6, 0), min(i+4, len(cells))
			var near strings.Builder
			for _, cc := range cells[from:to] {
				near.WriteRune(cc.r)
			}
			why := "followed by " + fmt.Sprintf("%q", func() string {
				if i+1 < len(cells) {
					return string(cells[i+1].r)
				}
				return "the end of the line"
			}())
			if i+1 < len(cells) && cells[i+1].r == ' ' {
				why = "followed by a space drawn in another style"
			}
			out = append(out, fmt.Sprintf("line %d col %d: %q %s (…%s…)", n+1, i+1, c.r, why, near.String()))
		}
	}
	return out
}

// wideGlyphFrames are the frames the guard reads: every golden on disk (the
// screens and popups the suite pins, in colour and without, at both sizes) and
// the popups drawn fresh at both sizes, in colour.
func wideGlyphFrames(t *testing.T) map[string]string {
	t.Helper()
	frames := map[string]string{}
	paths, err := filepath.Glob("testdata/*.golden")
	if err != nil || len(paths) == 0 {
		t.Fatalf("no goldens to read: %v", err)
	}
	for _, p := range paths {
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		frames[filepath.Base(p)] = string(b)
	}
	forceColor(t)
	// The demo workspace as the operator sees it, with each popup open over
	// it: View() is the finished frame, so what the popup's edge does to the
	// glyph under it is read here too.
	if _, err := exec.LookPath("astrolog"); err == nil {
		cfg := Config{Color: ColorAlways}
		for _, size := range fitSizes {
			frames[fmt.Sprintf("demo %dx%d", size.w, size.h)] = designDemoModel(t, cfg, size.w, size.h).View()
			for _, popup := range []string{"city", "open-chart", "chart-title", "save-chart", "bodies", "aspects", "keys"} {
				frames[fmt.Sprintf("demo %s %dx%d", popup, size.w, size.h)] = designPopup(t, cfg, size.w, size.h, popup).View()
			}
		}
	}
	for _, c := range fitCases() {
		for _, size := range fitSizes {
			for _, mode := range fitModes {
				frames[fmt.Sprintf("popup %s/%s/%dx%d/%s", c.popup, c.state, size.w, size.h, mode.name)] = c.open(t, mode.cfg, size.w, size.h)
			}
		}
	}
	return frames
}

// TestEveryWideGlyphHasRoom: no glyph that a terminal may draw wider than it
// counts is followed by anything but a space of its own style, so the overflow
// lands on a cell painted in the glyph's colour and never on the next letter.
func TestEveryWideGlyphHasRoom(t *testing.T) {
	frames := wideGlyphFrames(t)
	var names []string
	for n := range frames {
		names = append(names, n)
	}
	sort.Strings(names)
	// One line per kind of fault: the glyph, what follows it, how many times,
	// and the first place it is.
	type kind struct {
		n     int
		first string
	}
	kinds := map[string]*kind{}
	for _, n := range names {
		for _, f := range wideGlyphFaults(frames[n]) {
			key := f[strings.Index(f, ": ")+2 : strings.Index(f, " (…")]
			if k := kinds[key]; k != nil {
				k.n++
				continue
			}
			kinds[key] = &kind{n: 1, first: n + " " + f}
		}
	}
	var keys []string
	for k := range kinds {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		t.Errorf("%s: %d times, first in %s", k, kinds[k].n, kinds[k].first)
	}
}

// The walker itself: what it passes and what it refuses, so a frame it reads
// as clean is clean. Each refusal is a case the program once had.
func TestWideGlyphWalkerSeesWhatItShould(t *testing.T) {
	for _, tc := range []struct {
		name  string
		frame string
		bad   bool
	}{
		{"glyph, space, same style", "\x1b[92m② \x1b[0mnext", false},
		{"glyph, plain space, glyph styled", "\x1b[92m②\x1b[0m next", true},
		{"glyph then a letter", "②next", true},
		{"glyph at the end of the line", "next ②", true},
		{"glyph before a border", "② in ①─────╮", true},
		{"tab label with room", "② in ① ─────╮", false},
		{"circled digit in a bracket", "[●] Planets", true},
		{"the same with room", "[● ] Planets", false},
		{"box drawing is flush", "╭──────╮", false},
		{"degree sign is flush", "Tau 16°43'", false},
		{"letters with accents are flush", "Tromsø", false},
		{"wide glyph", "日本", true},
		{"same colour, different attributes", "\x1b[1;92m②\x1b[0;92m x", true},
		{"same style written two ways", "\x1b[92m\x1b[1m②\x1b[0m\x1b[92m\x1b[1m x", false},
	} {
		got := wideGlyphFaults(tc.frame)
		if (len(got) > 0) != tc.bad {
			t.Errorf("%s: %q: faults %v, want bad=%v", tc.name, tc.frame, got, tc.bad)
		}
	}
}
