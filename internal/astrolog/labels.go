package astrolog

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/cruiserxyz/sextile/internal/recipe"
)

// maxLabelBytes is the longest name or place Sextile sends Astrolog 8.00 in a
// report header: one bound for both. Astrolog segfaults (exit 139) from a
// 104-byte name or an 88-byte place, measured 2026-10-09 across -v, -aa,
// -aa -k and -aa -RO (the specification C8). Measured 2026-10-24 against a gcc
// build with _FORTIFY_SOURCE, which turns the silent out-of-bounds writes into
// aborts:
//
//   - PrintHeader (charts1.cpp) sprintf's " <place> <location string>\n" into
//     a char sz[80]: a place of 63 bytes overflows it (62 is the largest safe
//     one), and 57 when the call also prints seconds (-b0, whose location
//     string is longer; 56 is safe);
//   - a pair report (-r0 with -aa) overflows on the SECOND chart's name of 55
//     bytes (54 is safe), whatever its place.
//
// 54 is inside every one of them.
const maxLabelBytes = 54

const labelEllipsis = "…"

// labelText is the one rule for text Sextile hands Astrolog as a chart label
// (the specification C2, C8, N27), applied to the argv form and
// the staged-file form alike:
//
//   - text that is not valid UTF-8 is sent empty (reportText would turn a 0xF8
//     byte inside it into a degree sign);
//   - every Unicode control rune (category Cc, U+0085 included) becomes one
//     space, before any cut: a newline would split the header;
//   - a label over maxLabelBytes is cut at a rune boundary to at most
//     maxLabelBytes bytes ending in an ellipsis.
//
// Everything else is Astrolog's to print exactly as given.
func labelText(s string) string {
	if !utf8.ValidString(s) {
		return ""
	}
	s = strings.Map(func(r rune) rune {
		if unicode.Is(unicode.Cc, r) {
			return ' '
		}
		return r
	}, s)
	return CutLabel(s)
}

// MaxLabelBytes is maxLabelBytes for the packages that refuse or cut a label
// before it reaches this one (the specification).
const MaxLabelBytes = maxLabelBytes

// CutLabel is the one cut rule (invalid UTF-8 bytes are dropped first): text over MaxLabelBytes is cut at a rune
// boundary to at most MaxLabelBytes bytes ending in an ellipsis. Reports use it
// through labelText; a save uses it for a slot place the operator never typed.
func CutLabel(s string) string {
	s = strings.ToValidUTF8(s, "") // the output is always valid UTF-8
	if len(s) <= maxLabelBytes {
		return s
	}
	cut := maxLabelBytes - len(labelEllipsis)
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + labelEllipsis
}

// LabelTooLongError is a typed name or place that Astrolog 8.00 could not read
// back: it segfaults on a -zi name of 104 bytes or a place of 88
// (the specification). Refused, never cut: a chart is not saved under a name
// the operator did not choose.
type LabelTooLongError struct {
	What  string // "name", "title" or "place"
	Bytes int
}

func (e *LabelTooLongError) Error() string {
	w := e.What
	if w != "" {
		w = strings.ToUpper(w[:1]) + w[1:]
	}
	return fmt.Sprintf("%s too long for Astrolog: %d of at most %d bytes (accented letters count as 2)", w, e.Bytes, maxLabelBytes)
}

// CheckLabelLength is nil for a label of at most MaxLabelBytes bytes.
func CheckLabelLength(what, s string) error {
	if len(s) > maxLabelBytes {
		return &LabelTooLongError{What: what, Bytes: len(s)}
	}
	return nil
}

// labelArgv is the `-zi <name> <place>` triple every single-chart call carries,
// so one prefix serves every projection and a host astrolog.as cannot supply a
// label of its own (N12). Only a call that prints the chart's header for the
// operator (labelsChart) carries the name and place; the wheel image and a
// bundle's shared prefix carry the empty pair, never a name (C5a).
func labelArgv(name, place string, labelled bool) []string {
	if !labelled {
		return []string{"-zi", "", ""}
	}
	return []string{"-zi", labelText(name), labelText(place)}
}

// stagedZI is the `-zi` line of a staged chart file. A staged value is wrapped
// in "…", or in '…' when it holds a double quote; a backslash escape does not
// work there (measured), so a label holding BOTH quote kinds loses its double
// quotes.
func stagedZI(name, place string) string {
	return "-zi " + stagedValue(labelText(name)) + " " + stagedValue(labelText(place))
}

func stagedValue(s string) string {
	if strings.Contains(s, `"`) && strings.Contains(s, `'`) {
		s = strings.ReplaceAll(s, `"`, "")
	}
	if strings.Contains(s, `"`) {
		return fmt.Sprintf("'%s'", s)
	}
	return `"` + s + `"`
}

// labelsChart says whether a projection prints a chart's header for the
// operator: the single-chart compute and every report. The wheel image never
// carries a name (C5a).
func labelsChart(p recipe.Projection) bool {
	switch p.(type) {
	case recipe.Positions, recipe.Aspects, recipe.RawReport:
		return true
	}
	return false
}
