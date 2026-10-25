package ui

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/internal/store"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// the specification rule 1: the entered row's cell is
// drawn and bold even when its power ranks below the cells the grid has room
// for — it takes the last visible cell, and the cells above keep their power
// order — for the cross list of a chart row and the own list of a sky row.
// Operator, 2026-10-13: "if the 0°00' row is further down, nothing is boldened
// at all .... fix that".
func TestEnteredAspectCellIsKeptInView(t *testing.T) {
	cfg := Config{Color: ColorNever}
	bodies := []aspect.Body{aspect.Moon, aspect.Mars, aspect.Jupiter, aspect.Saturn, aspect.Mercury, aspect.Uranus, aspect.Neptune, aspect.Pluto}
	// check reads the Aspects panel: the wanted cell is drawn, it is the only
	// bold cell, it sits in the last drawn line with nothing to its right, and
	// the first drawn cell is the strongest entry.
	check := func(step string, m Model, want, strongest string) {
		t.Helper()
		m.width, m.height = 132, 40
		_, aspects := leftPanels(t, m.View())
		drawn := []string{}
		for _, ln := range aspects {
			if strings.ContainsAny(ln, "☌⚹□△☍") {
				drawn = append(drawn, ln)
			}
		}
		if len(drawn) == 0 {
			t.Fatalf("%s: no aspect cells drawn:\n%s", step, strings.Join(aspects, "\n"))
		}
		joined := strings.Join(drawn, "\n")
		bold := boldSpanTexts(aspects)
		if !strings.Contains(joined, want) {
			t.Fatalf("%s: the entered cell is not drawn (want %q):\n%s", step, want, joined)
		}
		if len(bold) != 1 || !strings.HasPrefix(bold[0], want+" ") {
			t.Errorf("%s: bold cells = %q, want exactly the entered cell %q", step, bold, want)
		}
		last := drawn[len(drawn)-1]
		if i := strings.Index(last, want); i < 0 || strings.ContainsAny(last[i+len(want):], "☌⚹□△☍") {
			t.Errorf("%s: the entered cell is not the last visible cell:\n%s", step, joined)
		}
		if !strings.Contains(drawn[0], strongest) || strings.Index(drawn[0], strongest) > strings.Index(drawn[0], " ")+40 {
			t.Errorf("%s: the first drawn cell is not the strongest %q:\n%s", step, strongest, drawn[0])
		}
	}

	// A transit row of ② (Sun opposite natal Venus): 39 stronger squares, the
	// row's opposition at the lowest power.
	m, _ := markFixture(t, cfg)
	overlayCrossAspects = func(_ context.Context, _ lookup.Lens, _ store.AnchorChart, _ time.Time, _ location.PlaceInfo) (lookup.OverlayAspects, error) {
		var list []lookup.OverlayAspect
		for i := 0; i < 39; i++ {
			list = append(list, lookup.OverlayAspect{Transit: bodies[i%8], Natal: bodies[(i+3)%8], Kind: aspect.Square, Orb: aspect.Orb(1 + i%5), Power: 50 - float64(i)})
		}
		list = append(list, lookup.OverlayAspect{Transit: aspect.Sun, Natal: aspect.Venus, Kind: aspect.Opposition, Orb: 0, Power: 1})
		return lookup.OverlayAspects{Aspects: list}, nil
	}
	m = enter(t, m, timelineRowWith(t, m.timeline.rows, "Opp natal (Ari) Venus"))
	if got := activeAspectsTitle(m); got != "3 in 2" {
		t.Fatalf("fixture: the Aspects tab is %q, want 3 in 2", got)
	}
	check("cross list", m, bodyGlyph(aspect.Venus)+" "+aspectGlyph(aspect.Opposition)+" "+bodyGlyph(aspect.Sun), bodyGlyph(aspect.Saturn)+" "+aspectGlyph(aspect.Square)+" "+bodyGlyph(aspect.Moon))

	// A sky row (Moon trine Mars) on the Aspects tab: 39 stronger own aspects,
	// the row's trine at the lowest power.
	m, _ = markFixture(t, cfg)
	inner := firstCallTransits
	firstCallTransits = func(ctx context.Context, p *lookup.Probers, when time.Time, place location.PlaceInfo, lens lookup.Lens) (lookup.Result, error) {
		res, err := inner(ctx, p, when, place, lens)
		var list []aspect.Aspect
		for i := 0; i < 39; i++ {
			list = append(list, aspect.Aspect{A: bodies[i%8], B: bodies[(i+3)%8], Kind: aspect.Square, Orb: aspect.Orb(1 + i%5), Power: 50 - float64(i)})
		}
		list = append(list, aspect.Aspect{A: aspect.Moon, B: aspect.Mars, Kind: aspect.Trine, Orb: 0, Power: 1})
		res.Aspects = list
		return res, err
	}
	m = enter(t, m, timelineRowWith(t, m.timeline.rows, "Moon (Sag) Tri (Leo) Mars"))
	if got := activeAspectsTitle(m); got != "Aspects" {
		t.Fatalf("fixture: the Aspects tab is %q, want Aspects", got)
	}
	check("own list", m, glyphs(aspect.Moon)+" "+aspectGlyph(aspect.Trine)+" "+glyphs(aspect.Mars), glyphs(aspect.Moon)+" "+aspectGlyph(aspect.Square)+" "+glyphs(aspect.Saturn))
}
