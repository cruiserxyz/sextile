package ui

import (
	"regexp"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// An argv line is stored whole, as the spawn ran, and copied whole. What the
// console DISPLAYS starts with what names the chart: every call opens with the
// binary's path, `-YQ 0` and a block of AstroExpression-clearing pairs (`'-~g'
// ''` and fifty-odd more) that is the same on every line, so the display drops
// that opening behind `astrolog … ` and keeps the part after the last clearing
// pair (the hook-pair head of 7a7e5629, brought back by the operator,
// 2026-10-24). A line still too wide keeps its START and ends in the cut mark.
// A line with no clearing block shows from its first switch, its path shortened
// to `astrolog`; the whole line is one Enter away.

// hookClearPair is one clearing pair as the spawn quotes it: shellQuote wraps
// `-~<hook>` in single quotes and the empty expression is `”`.
var hookClearPair = regexp.MustCompile(`'-~[0-9A-Za-z]+' ''(?: |$)`)

// argvShortName is what the binary's path is shown as.
const argvShortName = "astrolog"

// argvDisplay is the display form of a stored argv line in w cells; w <= 0
// means no limit, and the line is shown whole with its path shortened.
func argvDisplay(line string, w int, cfg Config) string {
	rest := strings.TrimLeft(line[firstWordEnd(line):], " ")
	head := argvShortName + " "
	if pairs := hookClearPair.FindAllStringIndex(line, -1); len(pairs) > 0 {
		rest = strings.TrimSpace(line[pairs[len(pairs)-1][1]:])
		head += cutMark(cfg) + " "
	}
	whole := strings.TrimRight(head+rest, " ")
	if w <= 0 || ansi.StringWidth(whole) <= w {
		return whole
	}
	return cutRight(whole, w, cfg)
}

// firstWordEnd is where the line's first shell word ends: the binary's path,
// single-quoted when it holds a character the shell would read. A quote inside
// a quoted word is the closed-escaped-reopened idiom `'\”`.
func firstWordEnd(line string) int {
	if !strings.HasPrefix(line, "'") {
		if i := strings.IndexByte(line, ' '); i >= 0 {
			return i
		}
		return len(line)
	}
	for i := 1; i < len(line); i++ {
		if line[i] != '\'' {
			continue
		}
		if strings.HasPrefix(line[i:], `'\''`) {
			i += 3
			continue
		}
		return i + 1
	}
	return len(line)
}
