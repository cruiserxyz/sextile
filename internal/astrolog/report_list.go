package astrolog

import (
	"regexp"
	"strings"

	"github.com/cruiserxyz/sextile/internal/astrologstyle"
)

// ReportGrammar names the row grammar a report declares (specs/details-list-
// filter.md). A report is a list only by declaring one, and the grammar is the
// one this package already reads Astrolog's output with, so the UI never
// carries an Astrolog pattern of its own.
type ReportGrammar uint8

const (
	// GrammarNone declares no list: dashboards, the Chart report, templates.
	GrammarNone ReportGrammar = iota
	// GrammarAspects is the own-chart `-aa` row (aspectLinePat), also the
	// single-chart body report.
	GrammarAspects
	// GrammarCrossAspects is the `with` row of a comparison (crossAspectLinePat),
	// also the cross body reports.
	GrammarCrossAspects
)

// LineSpan is a half-open range [Start, End) of report line indexes, counted
// the way the Details panel splits a report: one final newline terminates the
// last line instead of starting an empty one.
type LineSpan struct{ Start, End int }

// ReportList is where a report's one list sits: Header is every line before
// it, List runs from the first record to the end of the last, Footer is
// everything after, and Records are the list cut at record boundaries (they
// tile List exactly).
type ReportList struct {
	Header, List, Footer LineSpan
	Records              []LineSpan
}

// ReportListSpans finds the one list of a report under its grammar. A record
// starts at a line the grammar recognises and runs over the indented,
// non-blank lines after it that the grammar does not recognise (continuations),
// up to the next record, a blank line or the end. ok is false, "no list", when
// the grammar is GrammarNone, the text has no record, or anything but a record
// lies between the first record's start and the last record's end.
func ReportListSpans(text string, g ReportGrammar) (ReportList, bool) {
	var isRow func(string) bool
	switch g {
	case GrammarAspects:
		isRow = aspectLinePat.MatchString
	case GrammarCrossAspects:
		isRow = crossAspectLinePat.MatchString
	default:
		return ReportList{}, false
	}
	text = strings.TrimSuffix(astrologstyle.StripSGR(text), "\n")
	if text == "" {
		return ReportList{}, false
	}
	lines := strings.Split(text, "\n")
	var recs []LineSpan
	for i := 0; i < len(lines); i++ {
		if !isRow(lines[i]) {
			continue
		}
		end := i + 1
		for end < len(lines) && isContinuation(lines[end]) && !isRow(lines[end]) {
			end++
		}
		recs = append(recs, LineSpan{i, end})
		i = end - 1
	}
	if len(recs) == 0 {
		return ReportList{}, false
	}
	// One list: the records must tile the stretch between the first and last.
	for k := 1; k < len(recs); k++ {
		if recs[k].Start != recs[k-1].End {
			return ReportList{}, false
		}
	}
	first, last := recs[0].Start, recs[len(recs)-1].End
	return ReportList{
		Header:  LineSpan{0, first},
		List:    LineSpan{first, last},
		Footer:  LineSpan{last, len(lines)},
		Records: recs,
	}, true
}

// isContinuation is a line that belongs to the record above it: indented, not
// blank, and not numbered. A numbered line the grammar rejects (the no-data
// `<…>` body) is a row of its own that Sextile does not read, so it breaks the
// list instead of hiding inside the record above it.
func isContinuation(line string) bool {
	return strings.TrimSpace(line) != "" && (line[0] == ' ' || line[0] == '\t') && !numberedRow.MatchString(line)
}

// numberedRow is Astrolog's row numbering, `  12:`, whatever follows it.
var numberedRow = regexp.MustCompile(`^\s*\d+:`)
