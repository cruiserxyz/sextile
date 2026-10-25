package ui

import (
	"strings"
	"testing"

	"github.com/cruiserxyz/sextile/internal/astrologstyle"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// TestPositionRows_AnglesPinnedAndOrdered: planets render first (emit order),
// then the angles pinned to the bottom in ASC/MC/DSC/IC order even when astrolog
// emits them out of that order.
func TestPositionRows_AnglesPinnedAndOrdered(t *testing.T) {
	ps := []aspect.Position{
		{Body: aspect.Midheaven, Longitude: 100}, // angle, emitted before ASC
		{Body: aspect.Sun, Longitude: 35},        // planet
		{Body: aspect.Ascendant, Longitude: 10},  // angle
	}
	rows := positionRows(ps, false)
	if len(rows) != 3 {
		t.Fatalf("got %d rows, want 3", len(rows))
	}
	if !strings.Contains(rows[0], "Sun") {
		t.Errorf("row 0 = %q, want the planet (Sun) first", rows[0])
	}
	if !strings.Contains(rows[1], "Asc") {
		t.Errorf("row 1 = %q, want Asc pinned first among angles", rows[1])
	}
	if !strings.Contains(rows[2], "MC") {
		t.Errorf("row 2 = %q, want MC after Asc", rows[2])
	}
}

func TestRenderBodiesBody_FocusedSlotOnly(t *testing.T) {
	state := chartViewState{
		positions: []aspect.Position{{Body: aspect.Sun, Longitude: 35}},
		overlay:   true,
	}

	got, _, _ := renderBodiesBody(state, nil, 0, 0, 80, 10, false, Config{})
	if !strings.Contains(got, "Sun") {
		t.Fatalf("focused slot body missing from render: %q", got)
	}
	for _, heading := range []string{"Natal", "Transit", "Transit over Natal"} {
		if strings.Contains(got, heading) {
			t.Errorf("Bodies rendered forbidden inline heading %q: %q", heading, got)
		}
	}
}

func TestFormatPositionRowStyled_UsesDistinctAstrologSpans(t *testing.T) {
	p := aspect.Position{Body: aspect.Moon, Longitude: 136.18, House: 8}
	row := formatPositionRowStyled(p, astrologstyle.BodyRow{Body: "\x1b[1;31m", Position: "\x1b[1;33m", House: "\x1b[1;34m"}, true, houseOverlay{})
	for _, want := range []string{"\x1b[91m", "\x1b[93m", "\x1b[94m"} {
		if !strings.Contains(row, want) {
			t.Errorf("styled row %q missing span %q", row, want)
		}
	}
	if strings.Contains(row, "\x1b[1;") {
		t.Errorf("unselected body row retained bold SGR: %q", row)
	}
}

func TestNormalWeightSGRPreservesColorSemantics(t *testing.T) {
	tests := []struct {
		in, want astrologstyle.SGR
	}{
		{"\x1b[1;31m", "\x1b[91m"},
		{"\x1b[1;38;5;123m", "\x1b[38;5;123m"},
		{"\x1b[36m", "\x1b[36m"},
		{"\x1b[1m", ""},
	}
	for _, tt := range tests {
		if got := normalWeightSGR(tt.in); got != tt.want {
			t.Errorf("normalWeightSGR(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// TestNewBodyNamesAndGlyphs: the B1 bodies all have names and glyphs (no "?").
func TestNewBodyNamesAndGlyphs(t *testing.T) {
	for _, b := range []aspect.Body{
		aspect.Lilith, aspect.Ascendant, aspect.Midheaven, aspect.Descendant, aspect.ImumCoeli,
	} {
		if got := bodyName(b); got == "?" || got == "" {
			t.Errorf("bodyName(%v) unset (%q)", b, got)
		}
		if got := bodyGlyph(b); got == "?" || got == "" {
			t.Errorf("bodyGlyph(%v) unset (%q)", b, got)
		}
	}
}
