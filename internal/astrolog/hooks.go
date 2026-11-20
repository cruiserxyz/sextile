package astrolog

import (
	"errors"
	"regexp"
	"slices"
	"strings"
)

// hooks.go is how a call keeps the host's astrolog.as out of the answer
// (the specification). Astrolog lets its data file define
// AstroExpression hooks that rewrite orbs, positions, cusps and filters, and
// print to stderr while doing it. Sextile defines no hooks of its own except one: the void search's Moon
// event filter (`-~d`, void.go), set after the clear band, whose output
// ReadVoids refuses when the filter did not apply. Every call clears every hook
// by name, right after `-YQ 0`, and a chart call refuses on any stderr line
// that is not a wheel progress line.

// hookNames are the 54 hooks of Astrolog 8.00, in the order the band clears
// them: the 52 `astrolog -Y` advertises (bracket forms expanded) plus XQ and
// WQ, which only astrolog.h lists. TestHookClearBandCoversAstrologHelp holds
// this list equal to the installed help.
//
// Neither `-0~` nor `-~0` stands in for it: `-0~` leaves a defined filter hook
// evaluating to 0, which rejects every row (0 transits instead of 38,
// measured), and `-~0` is a toggle a host file can pre-flip.
var hookNames = []string{
	"g", "a", "a0", "m", "ma", "j", "j0", "7", "L", "E",
	"Zd", "d", "dv", "t", "P", "O", "C", "A", "p", "p0",
	"kO", "kA", "kv", "F", "FC", "FO", "FA", "FN", "FT",
	"v", "v3", "v30", "sd", "U", "U0", "Ux", "Iv", "IV", "Ia", "IA",
	"Xv", "XL", "Xt", "XQ", "WQ", "q1", "q2", "Q1", "Q2", "Q3",
	"5s", "5f", "5Y", "5i",
}

// HookClearBand is the flat argv that clears every calculation hook: `-~<hook>`
// then `""`, for each hook. It is a fresh slice on every call, so no caller can
// change what a later call launches with.
func HookClearBand() []string {
	out := make([]string, 0, 2*len(hookNames))
	for _, name := range hookNames {
		out = append(out, "-~"+name, "")
	}
	return out
}

// ErrAstrologWarned means Astrolog wrote to stderr during a chart call (a hook
// error, a missing ephemeris file, an expression the host file evaluated), so
// the chart on stdout cannot be trusted. Match it with errors.Is.
var ErrAstrologWarned = errors.New("astrolog warned")

// WarnedError is the error a refused chart call returns. It carries stderr
// verbatim and nothing from stdout.
type WarnedError struct{ Stderr string }

func (e *WarnedError) Error() string {
	return ErrAstrologWarned.Error() + ": " + strings.TrimSpace(e.Stderr)
}

// Is makes errors.Is(err, ErrAstrologWarned) true.
func (e *WarnedError) Is(target error) bool { return target == ErrAstrologWarned }

// Warnings are the non-empty stderr lines that are not wheel progress.
func (e *WarnedError) Warnings() []string { return warningLines(e.Stderr) }

// MissingEphemeris reports whether any warning of the refused call is
// Astrolog's missing-ephemeris line (an object it placed at 0 Aries): the one
// refusal that makes a composite result untrustworthy as a whole.
func (e *WarnedError) MissingEphemeris() bool {
	for _, l := range e.Warnings() {
		if missingEphemeris.MatchString(l) {
			return true
		}
	}
	return false
}

// The two lines a wheel image writes to stderr on success (measured on 8.00).
var progressLines = map[string]bool{
	"Creating graphics chart in memory.": true,
	"Writing chart bitmap to file.":      true,
}

// missingEphemeris is Astrolog's line for an object whose Swiss Ephemeris file
// is absent, anchored over the whole line. Only a caller that collects it on
// purpose (the availability probe) treats it as expected; for every other call
// it is a warning and refuses the result. A line that merely contains
// "SwissEph" is not this line.
var missingEphemeris = regexp.MustCompile(`^SwissEph file '([^']+)' not found in PATH '.*'$`)

// warningLines is stderr minus empty lines and the two progress lines, each
// with trailing whitespace trimmed. A missing-ephemeris line IS a warning: the
// object it names was placed at 0 Aries, and a result that carries it cannot
// be trusted.
func warningLines(stderr string) []string { return warningLinesFor(stderr, false) }

// warningLinesFor is warningLines for a caller that collects the
// missing-ephemeris lines itself (the availability probe, which casts an object
// on purpose to learn its file): they are not warnings to it.
func warningLinesFor(stderr string, collecting bool) []string {
	var out []string
	for _, line := range strings.Split(stderr, "\n") {
		line = strings.TrimRight(line, " \t\r")
		if line == "" || progressLines[line] || (collecting && missingEphemeris.MatchString(line)) {
			continue
		}
		out = append(out, line)
	}
	return out
}

// missingEphemerisFiles is the file names of the missing-ephemeris lines in
// stderr, in the order Astrolog wrote them, each once. These are the lines
// warningLines drops; the probe in the body popup reads them from the result.
func missingEphemerisFiles(stderr string) []string {
	var out []string
	for _, line := range strings.Split(stderr, "\n") {
		m := missingEphemeris.FindStringSubmatch(strings.TrimRight(line, " \t\r"))
		if m == nil || slices.Contains(out, m[1]) {
			continue
		}
		out = append(out, m[1])
	}
	return out
}

// firstSaidLine is the line to keep of what Astrolog wrote on stderr: the
// first missing-ephemeris line when there is one, else the first line that is
// not wheel progress, with escape sequences and other control bytes removed
// and the ends trimmed. "" when Astrolog wrote nothing worth keeping.
func firstSaidLine(stderr string) string {
	first := ""
	for _, line := range strings.Split(stderr, "\n") {
		line = cleanSaid(line)
		if line == "" || progressLines[line] {
			continue
		}
		if missingEphemeris.MatchString(line) {
			return line
		}
		if first == "" {
			first = line
		}
	}
	return first
}

// cleanSaid removes escape sequences and control bytes from one line.
func cleanSaid(line string) string {
	line = ansiSequence.ReplaceAllString(line, "")
	line = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f || (r >= 0x80 && r < 0xa0) {
			return -1
		}
		return r
	}, line)
	return strings.TrimSpace(line)
}

// ansiSequence matches CSI and OSC escape sequences.
var ansiSequence = regexp.MustCompile(`\x1b\[[0-?]*[ -/]*[@-~]|\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)|\x1b[@-Z\\-_]`)
