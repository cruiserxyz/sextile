package lookup

import "github.com/cruiserxyz/sextile/internal/astrolog"

// The Details report's one list (the specification). astrolog owns
// the row grammar; ui may not import astrolog, so lookup hands the same names
// across without adding a second pattern or a second rule.
type (
	ReportGrammar = astrolog.ReportGrammar
	LineSpan      = astrolog.LineSpan
	ReportList    = astrolog.ReportList
)

const (
	GrammarNone         = astrolog.GrammarNone
	GrammarAspects      = astrolog.GrammarAspects
	GrammarCrossAspects = astrolog.GrammarCrossAspects
)

// ReportListSpans is astrolog.ReportListSpans: where a report's one list sits
// under its declared grammar, or ok false for "no list".
func ReportListSpans(text string, g ReportGrammar) (ReportList, bool) {
	return astrolog.ReportListSpans(text, g)
}
