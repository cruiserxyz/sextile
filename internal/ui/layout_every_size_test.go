package ui

import (
	"fmt"
	"testing"
)

// leftOrder is the order of the panels down the left column, at every size.
var leftOrder = []PanelID{PanelStatus, PanelChart, PanelBodies, PanelAspects}

// checkLayout is every rule the screen's layout keeps, for one input. It
// returns the first broken rule, or "" when the layout keeps them all.
func checkLayout(in layoutInput, l *screenLayout) string {
	if l == nil {
		return "no layout"
	}
	mainH := in.height - KeyBarHeight
	// The panels cover the main area once: every cell in exactly one rectangle.
	cover := make([]int, in.width*mainH)
	all := map[PanelID]panelRect{}
	for id, r := range l.rects {
		all[id] = r
	}
	if l.blank.h > 0 {
		all["blank"] = l.blank
	}
	for id, r := range all {
		if r.w <= 0 || r.h <= 0 || r.x < 0 || r.y < 0 || r.x+r.w > in.width || r.y+r.h > mainH {
			return fmt.Sprintf("%s lies outside the screen: %+v", id, r)
		}
		for y := r.y; y < r.y+r.h; y++ {
			for x := r.x; x < r.x+r.w; x++ {
				cover[y*in.width+x]++
			}
		}
	}
	for i, n := range cover {
		if n != 1 {
			return fmt.Sprintf("cell (%d,%d) is covered %d times", i%in.width, i/in.width, n)
		}
	}
	for _, id := range leftOrder {
		if _, ok := l.rects[id]; !ok {
			return fmt.Sprintf("%s is not on screen", id)
		}
	}
	// A folded panel is its title line; an open one has at least one row.
	for _, id := range leftOrder {
		r := l.rects[id]
		if l.folded[id] && r.h != 1 {
			return fmt.Sprintf("folded %s is %d lines tall", id, r.h)
		}
		if !l.folded[id] && r.h < 3 {
			return fmt.Sprintf("open %s is %d lines tall", id, r.h)
		}
	}
	// The panel in focus shows its cursor row.
	if l.folded[in.focus] {
		return fmt.Sprintf("the panel in focus, %s, is folded", in.focus)
	}
	// The left column keeps its order, top to bottom.
	prev := -1
	for _, id := range leftOrder {
		if l.rects[id].y <= prev {
			return fmt.Sprintf("%s is out of order", id)
		}
		prev = l.rects[id].y
	}
	// Beside, as on a wide screen today: the width holds Details next to the
	// left column, and the height holds the Session panel, the Chart panel,
	// five lines of Bodies and the Aspects panel.
	chartH := chartPanelLines(in.height, in.charts) + 2
	beside := in.width >= dualColumnMinWidth(in.shown) && mainH >= statusHeight+chartH+5+AspectsHeight
	d, details := l.rects[PanelDetails]
	_, logShown := l.rects[PanelLog]
	switch {
	case l.beside:
		// Details and the Log beside the panels, as on a wide screen today, and nothing folds.
		if !details || !logShown || d.x == 0 {
			return "beside, but Details or the Log is not on the right"
		}
		for _, id := range leftOrder {
			if l.folded[id] {
				return fmt.Sprintf("beside, but %s is folded", id)
			}
		}
	case in.detailsOpen:
		// Details right below the panel in focus, the full width, no Log.
		f := l.rects[in.focus]
		if !details || d.x != 0 || d.w != in.width || d.y != f.y+f.h {
			return fmt.Sprintf("Details is not right below %s: %+v under %+v", in.focus, d, f)
		}
		if logShown {
			return "the Log is on screen with Details below a panel"
		}
		if d.h < 5 {
			return fmt.Sprintf("Details below is %d lines tall", d.h)
		}
		// The panel shows the cursor's row and one row each side; every other panel is its title line.
		if want := min(in.rowsOf(in.focus), 3) + 2; f.h != want {
			return fmt.Sprintf("%s is %d lines tall with Details open, want %d", in.focus, f.h, want)
		}
		for _, id := range leftOrder {
			if id != in.focus && !l.folded[id] {
				return fmt.Sprintf("%s is open beside Details below %s", id, in.focus)
			}
		}
	default:
		if details || logShown {
			return "Details or the Log is on screen while it is closed and not beside"
		}
		// At rest the panel in focus and its partner are open, at any height;
		// the others are title lines.
		pair := restPair(in.focus)
		for _, id := range leftOrder {
			if open := id == pair[0] || id == pair[1]; open == l.folded[id] {
				return fmt.Sprintf("at rest with %s in focus, %s open = %v", in.focus, id, !l.folded[id])
			}
		}
		// Empty rows only under a pair that holds every row it has: the
		// charts never scroll while rows stay empty.
		if c := l.rects[PanelChart]; !l.folded[PanelChart] && c.h-2 < in.charts && l.blank.h > 0 {
			return fmt.Sprintf("the Chart panel shows %d of %d charts above %d empty rows", c.h-2, in.charts, l.blank.h)
		}
		if l.blank.h > 0 && pair[0] != PanelStatus {
			return fmt.Sprintf("%d empty rows under %s and %s", l.blank.h, pair[0], pair[1])
		}
	}
	if beside != l.beside {
		return fmt.Sprintf("beside = %v, want %v", l.beside, beside)
	}
	return ""
}

// rowsOf is how many rows the panel holds, for the input's charts.
func (in layoutInput) rowsOf(id PanelID) int {
	switch id {
	case PanelStatus:
		return 1
	case PanelChart:
		return in.charts
	}
	return 3 // Bodies and Aspects hold more than three rows in every chart
}

// Every size from 66x15 to 200x60, each panel in focus, Details open and
// closed, one, two and eight charts, one and three shown side by side: the panels
// never overlap, they cover the screen, the panel in focus shows its cursor
// row, and Details is on screen exactly when it fits beside the panels or is
// open.
func TestLayoutAtEverySize(t *testing.T) {
	n := 0
	for w := MinPanelWidth; w <= 200; w++ {
		for h := MinTerminalHeight; h <= 60; h++ {
			for _, focus := range leftOrder {
				for _, open := range []bool{false, true} {
					for _, charts := range []int{1, 2, 8} {
						for _, shown := range []int{1, 3} {
							in := layoutInput{width: w, height: h, focus: focus, detailsOpen: open, charts: charts, shown: shown}
							if why := checkLayout(in, layoutFor(in)); why != "" {
								t.Fatalf("%dx%d, %s in focus, Details open %v, %d charts, %d shown: %s", w, h, focus, open, charts, shown, why)
							}
							n++
						}
					}
				}
			}
		}
	}
	t.Logf("%d layouts checked", n)
}

// At rest two panels are open: Session with Chart, Chart with Bodies, and Bodies
// with Aspects for either of those two in focus. A short pair keeps its height
// and the rows under it stay empty; the Chart panel grows with its charts.
func TestRestOpensThePanelInFocusAndItsPartner(t *testing.T) {
	l := layoutFor(layoutInput{width: 80, height: 24, focus: PanelChart, charts: 2, shown: 1})
	if l == nil || l.rects[PanelChart].h != 4 || l.folded[PanelBodies] || !l.folded[PanelStatus] || !l.folded[PanelAspects] {
		t.Fatalf("Chart in focus at 80x24: %+v", l)
	}
	l = layoutFor(layoutInput{width: 80, height: 24, focus: PanelStatus, charts: 2, shown: 1})
	if l == nil || l.rects[PanelStatus].h != 3 || l.rects[PanelChart].h != 4 || !l.folded[PanelBodies] || l.blank.h != 23-3-4-2 {
		t.Fatalf("Session in focus at 80x24: %+v", l)
	}
	for _, f := range []PanelID{PanelBodies, PanelAspects} {
		l = layoutFor(layoutInput{width: 80, height: 40, focus: f, charts: 2, shown: 1})
		if l == nil || l.folded[PanelBodies] || l.folded[PanelAspects] || !l.folded[PanelStatus] || !l.folded[PanelChart] {
			t.Fatalf("%s in focus at 80x40: %+v", f, l)
		}
	}
	l = layoutFor(layoutInput{width: 80, height: 40, focus: PanelChart, charts: 20, shown: 1})
	if l == nil || l.rects[PanelChart].h != 22 {
		t.Fatalf("twenty charts at 80x40: the Chart panel is %+v, want all twenty rows", l.rects[PanelChart])
	}
}

// The control for the walk: a layout whose Details overlaps the panel above it
// is named, with its size.
func TestLayoutCheckNamesAnOverlap(t *testing.T) {
	in := layoutInput{width: 80, height: 24, focus: PanelBodies, detailsOpen: true, charts: 2, shown: 1}
	l := layoutFor(in)
	if l == nil {
		t.Fatal("no layout at 80x24")
	}
	bad := &screenLayout{rects: map[PanelID]panelRect{}, folded: l.folded, beside: l.beside}
	for id, r := range l.rects {
		bad.rects[id] = r
	}
	d := bad.rects[PanelDetails]
	d.y--
	bad.rects[PanelDetails] = d
	if why := checkLayout(in, bad); why == "" {
		t.Fatal("an overlap passed the check")
	}
}
