package ui

import (
	"github.com/cruiserxyz/sextile/internal/ui/panel"
	"github.com/cruiserxyz/sextile/internal/ui/term"
	"strings"
	"testing"
)

// boldCell puts bold on the cell's text and leaves the column's padding
// outside the span, so the waterfall's right trim still finds its spaces.
func TestBoldCellKeepsPaddingOutsideTheSpan(t *testing.T) {
	cell := "\x1b[31mM\x1b[0m △ ♀ (0°00'A)   "
	got := panel.BoldCell(cell)
	if !strings.HasPrefix(got, term.SGRBold) || !strings.HasSuffix(got, term.SGRReset+"   ") {
		t.Errorf("boldCell = %q: want bold first and the padding after the closing reset", got)
	}
	if strings.TrimRight(got, " ") == got {
		t.Errorf("boldCell = %q: the padding is inside the span", got)
	}
	if !strings.Contains(got, "\x1b[0m"+term.SGRBold+" △") {
		t.Errorf("boldCell = %q: bold is not re-applied after Astrolog's reset", got)
	}
	if panel.BoldCell("   ") != "   " {
		t.Errorf("a blank cell was given a span")
	}
}
