package panel

import (
	"fmt"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"github.com/cruiserxyz/sextile/internal/ui/term"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// The cursor lookups (ObjectAtCursor, AnyRowMarked) run many times per frame,
// on every key. Holding a cursor key in Bodies with every group ticked kept the
// cursor moving after release (operator's walk, 2026-11-06) because each lookup
// formatted the display text of every row to read only the row's object.

// rowTestStyle is a plain style for drawing the rows the lookups must agree with.
type rowTestStyle struct{}

func (rowTestStyle) UseColor() bool                  { return true }
func (rowTestStyle) UseASCII() bool                  { return false }
func (rowTestStyle) FocusColour() lipgloss.Color     { return term.DefaultFocusColour }
func (rowTestStyle) CursorRowColour() lipgloss.Color { return term.DefaultCursorRowColour }

// manyObjects is a chart with every group ticked: over a hundred objects above
// the rule, the four angles, and two plain cusp points (houses 11 and 12) whose
// two-digit numbers widen the glyph column.
func manyObjects() ([]aspect.Position, aspect.Cusps) {
	var ps []aspect.Position
	add := func(n int, name string) {
		ps = append(ps, aspect.Position{Object: aspect.Object{Number: n, Name: name}, Longitude: float64((n * 37) % 360)})
	}
	for n := 1; n <= 21; n++ {
		add(n, fmt.Sprintf("Body %d", n))
	}
	for _, c := range []struct {
		n    int
		name string
	}{{22, "Ascendant"}, {25, "Nadir"}, {28, "Descendant"}, {31, "Midheaven"}, {32, "11th Cusp"}, {33, "12th Cusp"}} {
		add(c.n, c.name)
	}
	for n := 34; n <= 120; n++ {
		add(n, fmt.Sprintf("Star %d", n))
	}
	var cusps aspect.Cusps
	for i := range cusps {
		cusps[i] = float64(i * 30)
	}
	return ps, cusps
}

// Every cursor index names the object of the row drawn at that index, the
// rows under the rule included; past the last row nothing resolves.
func TestObjectAtCursorIsTheDrawnRow(t *testing.T) {
	ps, cusps := manyObjects()
	planets, under := bodyRowsFor(ps, cusps, nil, rowTestStyle{}, HouseOverlay{})
	drawn := append(planets, under...)
	if len(drawn) < 100 || len(under) < 6 {
		t.Fatalf("fixture draws %d rows, %d under the rule; want over 100 and the angles with two cusps", len(drawn), len(under))
	}
	for i, r := range drawn {
		o, ok := ObjectAtCursor(ps, cusps, i)
		if o != r.Object || ok != (r.Object != aspect.Object{}) {
			t.Errorf("row %d: ObjectAtCursor = (%v, %v), the drawn row holds %v", i, o, ok, r.Object)
		}
		if r.Object != (aspect.Object{}) && !AnyRowMarked(ps, cusps, map[int]bool{r.Object.Number: true}) {
			t.Errorf("row %d: AnyRowMarked misses the marked %v", i, r.Object)
		}
	}
	if _, ok := ObjectAtCursor(ps, cusps, len(drawn)); ok {
		t.Errorf("a cursor past the last row resolved to an object")
	}
	if AnyRowMarked(ps, cusps, map[int]bool{999: true}) {
		t.Errorf("AnyRowMarked reports a mark whose object has no row")
	}
}

// The lookups read which object a row is and format no text: their work stays
// a handful of allocations however many objects are ticked. Formatting one row
// alone costs several allocations, so a lookup that formats again fails here
// by a wide margin.
func TestCursorLookupsFormatNoText(t *testing.T) {
	ps, cusps := manyObjects()
	const most = 60
	a := testing.AllocsPerRun(20, func() { ObjectAtCursor(ps, cusps, 50) })
	t.Logf("ObjectAtCursor: %.0f allocations for %d objects", a, len(ps))
	if a > most {
		t.Errorf("ObjectAtCursor allocates %.0f times for %d objects, want at most %d: it formats row text", a, len(ps), most)
	}
	if a := testing.AllocsPerRun(20, func() { AnyRowMarked(ps, cusps, map[int]bool{120: true}) }); a > most {
		t.Errorf("AnyRowMarked allocates %.0f times for %d objects, want at most %d: it formats row text", a, len(ps), most)
	}
}
