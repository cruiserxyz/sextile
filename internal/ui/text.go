package ui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// text.go is the one place text is shortened to fit
// (the specification C11, C12). Every cut leaves a mark where
// the text was removed — `…`, or `...` under --accessible — so the operator can
// tell a shortened name from a whole one. kept by convention (the invariant was retired 2026-10-03): no
// other file in this package calls an ansi cut function.
//
// A cut that removes only trailing spaces removes nothing anyone can read, so it
// carries no mark: a padded cell trimmed to its column is not a shortened name.

// cutMark is the mark a cut leaves.
func cutMark(cfg Config) string {
	if cfg.useASCII() {
		return "..."
	}
	return "…"
}

// markIn is the mark fitted into w cells, for a cut with no room for text.
func markIn(w int, cfg Config) string {
	mark := cutMark(cfg)
	if w >= ansi.StringWidth(mark) {
		return mark
	}
	return strings.Repeat(".", max(w, 0))
}

// onlySpacesBeyond reports whether everything past the first w cells of s is
// blank, so cutting it there loses nothing visible.
func onlySpacesBeyond(s string, w int) bool {
	return ansi.StringWidth(strings.TrimRight(ansi.Strip(s), " ")) <= w
}

// cutRight keeps the start of s in at most w cells, ending it in the mark when
// visible text had to go. ANSI styling inside s is kept.
func cutRight(s string, w int, cfg Config) string {
	if w <= 0 {
		return ""
	}
	if ansi.StringWidth(s) <= w {
		return s
	}
	if onlySpacesBeyond(s, w) {
		return ansi.Truncate(s, w, "")
	}
	mark := cutMark(cfg)
	if w <= ansi.StringWidth(mark) {
		return markIn(w, cfg)
	}
	return ansi.Truncate(s, w, mark)
}

// cutLeft keeps the end of s in at most w cells, starting it with the mark: a
// typed value keeps what was typed last, and a path keeps its file name.
func cutLeft(s string, w int, cfg Config) string {
	if w <= 0 {
		return ""
	}
	sw := ansi.StringWidth(s)
	if sw <= w {
		return s
	}
	mark := cutMark(cfg)
	mw := ansi.StringWidth(mark)
	if w <= mw {
		return markIn(w, cfg)
	}
	return ansi.TruncateLeft(s, sw-w+mw, mark)
}

// shortenRight keeps the start of s in EXACTLY w cells, ending it in the mark.
// A wide character that would straddle the cut is left out whole and the cell
// it leaves is a space before the mark, so the width is exact for any script.
// s is plain text: it is a list row's cell, before the row is styled.
func shortenRight(s string, w int, cfg Config) string {
	if ansi.StringWidth(s) <= w {
		return s
	}
	if w < 1 {
		return ""
	}
	mark := cutMark(cfg)
	mw := ansi.StringWidth(mark)
	if w <= mw {
		return markIn(w, cfg)
	}
	var b strings.Builder
	used := 0
	for _, r := range s {
		rw := ansi.StringWidth(string(r))
		if used+rw > w-mw {
			break
		}
		b.WriteRune(r)
		used += rw
	}
	return b.String() + strings.Repeat(" ", w-mw-used) + mark
}

// shortenLeft keeps the end of s in EXACTLY w cells, starting it with the
// mark: a place loses its city before its country, and a date its day before
// its year. As in shortenRight, a straddling wide character becomes a space.
func shortenLeft(s string, w int, cfg Config) string {
	if ansi.StringWidth(s) <= w {
		return s
	}
	if w < 1 {
		return ""
	}
	mark := cutMark(cfg)
	mw := ansi.StringWidth(mark)
	if w <= mw {
		return markIn(w, cfg)
	}
	runes := []rune(s)
	used, from := 0, len(runes)
	for from > 0 {
		rw := ansi.StringWidth(string(runes[from-1]))
		if used+rw > w-mw {
			break
		}
		used += rw
		from--
	}
	return mark + strings.Repeat(" ", w-mw-used) + string(runes[from:])
}

// fitLine is cutRight for a line that must come out exactly w cells wide:
// shorter text is padded with spaces.
func fitLine(s string, w int, cfg Config) string {
	s = cutRight(s, w, cfg)
	if sw := ansi.StringWidth(s); sw < w {
		s += strings.Repeat(" ", w-sw)
	}
	return s
}
