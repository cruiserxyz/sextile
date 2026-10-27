package ui

import (
	"strings"

	"github.com/mattn/go-runewidth"
)

// glyph_room.go keeps a wide-drawn glyph from spilling onto the next letter.
//
// Some glyphs are counted as one cell and drawn as two: their East Asian Width
// is Ambiguous or Wide, and a terminal set for East Asian text, or a font that
// has the glyph only in a wide fallback face, draws them wider than the cell
// they are counted in (operator, 2026-10-27: the circled numerals "experience a
// coloring issue, currently one third of the glyph receives the default
// inactive color in WSL; Linux st similar ... the next character starts inside
// this glyph"). Whatever is drawn in the next cell then lands on the glyph.
//
// The rule, by design: every such glyph is immediately followed by a space
// that is drawn in the glyph's own style, so the overflow lands on a cell
// painted in the glyph's colour and never on the next character. The glyphs
// Sextile draws are spaced where they are made (a tab label ends with its
// glyph and a space, a group mark is "[● ]"); paintGlyphRoom is the net under
// them, for the one thing the makers cannot see: the style of the space that
// follows, which the row around them chose. TestEveryWideGlyphHasRoom reads
// the frames and holds the rule, so a glyph added without room fails there.

// roomyGlyph reports whether a terminal may draw r wider than the one cell it
// is counted as: its East Asian Width is Ambiguous or Wide.
func roomyGlyph(r rune) bool {
	if r < 0x80 {
		return false
	}
	c := runewidth.NewCondition()
	c.EastAsianWidth = false
	return runewidth.IsAmbiguousWidth(r) || c.RuneWidth(r) == 2
}

// flushByNature are the ambiguous-width runes that cannot take a space after
// them, or need none, and why. Everything else Sextile draws that is roomy is
// followed by a space of its own style. Widening this list is the deliberate
// act that lets a glyph through, so each line says what keeps it out of the
// rule.
func flushByNature(r rune) bool {
	switch {
	// Frames, rules and bars. The terminal draws these itself, edge to edge
	// (st and Windows Terminal both do), whatever the font; a space after a
	// border or inside a bar would break the line they make.
	case r >= 0x2500 && r < 0x25A0:
		return true
	// Letters and punctuation of running text: Latin-1 and Latin Extended
	// (the degree sign sits flush on its number, "16°43'"; a chart's name has
	// its own accents), Greek and Cyrillic letters of a name. Every monospace
	// face has them in its primary font, so none is drawn from a wide
	// fallback face.
	case r >= 0x80 && r < 0x0530:
		return true
	// General punctuation: the dash, the bullet and the cut mark "…", the
	// minute mark. Also in every monospace face's primary font.
	case r >= 0x2010 && r < 0x2028, r >= 0x2030 && r < 0x2040:
		return true
	// The four arrows of the key bar: in the primary font of every monospace
	// face (WGL4), and they sit in pairs ("↑↓").
	case r >= 0x2190 && r <= 0x2193:
		return true
	}
	return false
}

// needsRoom reports whether r must be followed by a space of its own style.
func needsRoom(r rune) bool { return roomyGlyph(r) && !flushByNature(r) }

// endsWithRoomyGlyph reports whether the last visible rune of s needs room: a
// title or a label that ends with one is given its space before whatever
// follows it (a border, a column).
func endsWithRoomyGlyph(s string) bool {
	var last rune
	for i := 0; i < len(s); {
		if s[i] == escapeByte && i+1 < len(s) && s[i+1] == '[' {
			j := i + 2
			for j < len(s) && (s[j] == ';' || (s[j] >= '0' && s[j] <= '9')) {
				j++
			}
			i = j + 1
			continue
		}
		r := []rune(s[i:min(i+4, len(s))])[0]
		last = r
		i += len(string(r))
	}
	return needsRoom(last)
}

// paintGlyphRoom draws the space after each glyph that needs room in the
// glyph's style. A row styles its glyph, closes the style and writes its own
// space; here that space is moved in front of the closing, so the glyph and
// the cell it may spill onto are one run. Nothing changes width, no cell is
// added: a glyph that is followed by anything but a space is for its maker to
// fix, because only the maker can make the room.
func paintGlyphRoom(frame string) string {
	if !strings.ContainsRune(frame, escapeByte) {
		return frame
	}
	var b strings.Builder
	b.Grow(len(frame))
	for i := 0; i < len(frame); {
		c := frame[i]
		if c == escapeByte {
			j := i + 1
			if j < len(frame) && frame[j] == '[' {
				j++
				for j < len(frame) && (frame[j] == ';' || (frame[j] >= '0' && frame[j] <= '9')) {
					j++
				}
				j = min(j+1, len(frame))
			}
			b.WriteString(frame[i:j])
			i = j
			continue
		}
		if c < 0x80 {
			b.WriteByte(c)
			i++
			continue
		}
		r := []rune(frame[i:min(i+4, len(frame))])[0]
		size := len(string(r))
		b.WriteString(frame[i : i+size])
		i += size
		if !needsRoom(r) {
			continue
		}
		// The style changes between the glyph and what follows (a reset, a
		// new colour) and then a space comes: write the space first.
		j := i
		for j < len(frame) && frame[j] == escapeByte && j+1 < len(frame) && frame[j+1] == '[' {
			k := j + 2
			for k < len(frame) && (frame[k] == ';' || (frame[k] >= '0' && frame[k] <= '9')) {
				k++
			}
			if k >= len(frame) || frame[k] != 'm' {
				break
			}
			j = k + 1
		}
		if j > i && j < len(frame) && frame[j] == ' ' {
			b.WriteByte(' ')
			b.WriteString(frame[i:j])
			i = j + 1
		}
	}
	return b.String()
}

// blankGlyphAtEnd replaces the last visible rune of s with a space if it needs
// room: the cell to its right is about to belong to something else (a popup's
// border laid over a row), and the glyph's spill would land on it. The popup
// takes the cell; the glyph under its edge is not missed.
func blankGlyphAtEnd(s string) string {
	last, size := -1, 0
	for i := 0; i < len(s); {
		if s[i] == escapeByte && i+1 < len(s) && s[i+1] == '[' {
			j := i + 2
			for j < len(s) && (s[j] == ';' || (s[j] >= '0' && s[j] <= '9')) {
				j++
			}
			i = min(j+1, len(s))
			continue
		}
		r := []rune(s[i:min(i+4, len(s))])[0]
		last, size = i, len(string(r))
		i += size
	}
	if last < 0 || size < 2 || !needsRoom([]rune(s[last : last+size])[0]) {
		return s
	}
	return s[:last] + " " + s[last+size:]
}
