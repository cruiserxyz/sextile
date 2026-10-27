package ui

import (
	"fmt"
	"testing"

	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/store"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// pageOwner is one owner of a cursor, described in the paging rule's own frame:
// where its cursor is among its rows, where its window starts, how many rows it
// has and how many cursor rows its render shows (the SMALLEST count, rule 1b).
// The table below lists every owner there is, and TestEveryCursorOwnerPagesCursorFirst
// holds each of them to the same two presses: that is what proves one rule, since
// a surface that pages by hand cannot pass it by name.
type pageOwner struct {
	name string
	// open returns a model with the owner open and holding many rows.
	open func(t *testing.T) Model
	// read is the owner's state in the rule's frame. For a list with pinned rows
	// above it the cursor is the list's own index.
	read func(m Model) (cursor, offset, total, visible int)
	// put places the cursor and window, in the same frame.
	put func(m Model, cursor, offset int) Model
	// drawn is the offset the owner's render draws from for its stored state: what
	// its draw-time correction (windowAt, chartWindow, bodyWindowStart) settles on.
	drawn func(m Model) int
	// popup says the owner is one of the six filtered popups.
	popup bool
	// pinned is how many always-visible rows sit above the list in a combined
	// cursor (the aspect families, the body groups): Home goes to the first of
	// them, which the list frame calls -pinned.
	pinned int
	// noHomeEnd says Home and End are not the owner's keys: the `/` line takes
	// text, and they are no-ops there.
	noHomeEnd bool
	// filter, set for the popups, is the text typed into the owner's filter box.
	filter func(m Model) string
	// endRows, if set, is how many rows the render draws at the very end of the
	// list when that is more than the page counted on (visible+1): the window
	// past the rule, or with no status row. The stored window is settled to the
	// drawn one, one row short of a page.
	endRows func(m Model) int
}

func syntheticPlaces(n int) []location.PlaceInfo {
	out := make([]location.PlaceInfo, n)
	for i := range out {
		out[i] = location.PlaceInfo{Name: fmt.Sprintf("Testville %02d", i), Country: "TS", Lat: float64(i % 80), Lon: float64(i % 170), TZ: "Europe/Berlin"}
	}
	return out
}

func syntheticAspects(n int) []aspect.RegistryAspect {
	out := make([]aspect.RegistryAspect, n)
	for i := range out {
		out[i] = aspect.RegistryAspect{Index: i + 1, Name: fmt.Sprintf("Aspect %02d", i+1), Abbrev: fmt.Sprintf("A%02d", i+1), Angle: float64(i%90) * 2, Orb: 3, InForce: true}
	}
	return out
}

func syntheticObjects(n int) []aspect.RegistryObject {
	out := make([]aspect.RegistryObject, n)
	for i := range out {
		out[i] = aspect.RegistryObject{Number: i, Name: fmt.Sprintf("Object %02d", i)}
	}
	return out
}

func syntheticCharts(n int) []store.NamedChart {
	out := make([]store.NamedChart, n)
	for i := range out {
		out[i] = testNamedChart(fmt.Sprintf("Client %02d", i), berlinP)
	}
	return out
}

// pageOwners is every cursor owner in the program: the Chart and Bodies panels,
// the attached timeline and its `/` line, the Log console and the six filtered
// popups. A new owner of a cursor is added here or it is not held to the rule.
func pageOwners() []pageOwner {
	var owners []pageOwner

	owners = append(owners, pageOwner{
		name: "Chart panel",
		open: func(t *testing.T) Model { return chartPageModel(t) },
		read: func(m Model) (int, int, int, int) {
			return m.cursorIdx, m.chartTop, len(m.slots.All()), m.geometry()[PanelChart].h - 2
		},
		put: func(m Model, c, o int) Model { m.cursorIdx, m.chartTop = c, o; return m },
		drawn: func(m Model) int {
			start, _ := chartWindow(len(m.slots.All()), m.cursorIdx, m.geometry()[PanelChart].h-2, m.chartTop)
			return start
		},
	})

	bodiesVisible := func(m Model) int {
		p, u, h := m.bodiesRows()
		if bodyRuled(p, u, h) {
			return h - 1
		}
		return h
	}
	owners = append(owners, pageOwner{
		name: "Bodies panel",
		open: func(t *testing.T) Model { return bodiesPageModel(t, 16) },
		read: func(m Model) (int, int, int, int) {
			p, u, h := m.bodiesRows()
			return m.bodyCursorIdx, bodyPageOffset(p, u, h, m.bodyTop), p + u, bodiesVisible(m)
		},
		put: func(m Model, c, o int) Model {
			p, u, h := m.bodiesRows()
			m.bodyCursorIdx, m.bodyTop = c, bodyPageTop(p, u, h, o)
			return m
		},
		drawn: func(m Model) int {
			p, u, h := m.bodiesRows()
			return bodyPageOffset(p, u, h, bodyWindowStart(p, u, m.bodyCursorIdx, m.bodyTop, h))
		},
	})

	timeline := func(m Model) (int, int, int, int) {
		return m.details.cursor, m.details.rowOffset, len(m.timelineShown()), m.timelineHeight()
	}
	putTimeline := func(m Model, c, o int) Model {
		m.details.cursor, m.details.rowOffset = c, o
		return m.recordTimelineCursor()
	}
	drawnTimeline := func(m Model) int {
		return clampOffset(m.details.rowOffset, len(m.timelineShown()), m.timelineHeight())
	}
	owners = append(owners, pageOwner{
		name:  "Timeline, attached",
		open:  func(t *testing.T) Model { return pagedTimeline(t, 100) },
		read:  timeline,
		put:   putTimeline,
		drawn: drawnTimeline,
	}, pageOwner{
		name:      "Timeline, search line",
		noHomeEnd: true,
		open: func(t *testing.T) Model {
			m := pagedTimeline(t, 100)
			m, _ = tpress(t, m, runes("/"))
			if !m.timeline.searching {
				t.Fatal("the search line did not open")
			}
			return m
		},
		read:  timeline,
		put:   putTimeline,
		drawn: drawnTimeline,
	})

	owners = append(owners, pageOwner{
		name: "Log console",
		open: func(t *testing.T) Model { return openedConsole(t, 120) },
		read: func(m Model) (int, int, int, int) {
			return m.console.cursor, m.console.offset, len(m.console.snapshot), m.consoleInnerHeight()
		},
		put: func(m Model, c, o int) Model { m.console.cursor, m.console.offset = c, o; return m },
		drawn: func(m Model) int {
			start, _ := consoleWindow(len(m.console.snapshot), m.console.offset, m.consoleInnerHeight())
			return start
		},
	})

	popupBase := func(t *testing.T, termH int) Model {
		m := escBase(t)
		m.height = termH
		return m
	}
	owners = append(owners, pageOwner{
		name:   "City popup",
		filter: func(m Model) string { return m.atlas.filterText },
		popup:  true,
		open: func(t *testing.T) Model {
			m := popupBase(t, 30)
			m.startup = startupAtlasOpen
			m.atlas.filterText = "t"
			m.atlas.setMatches(syntheticPlaces(60), false)
			return m
		},
		read: func(m Model) (int, int, int, int) {
			return m.atlas.cursor, m.atlas.offset, len(m.atlas.matches), modalListHeight(m.height)
		},
		put: func(m Model, c, o int) Model { m.atlas.cursor, m.atlas.offset = c, o; return m },
		drawn: func(m Model) int {
			_, off := m.atlas.windowAt(m.atlas.cursor, modalListHeight(m.height))
			return off
		},
	}, pageOwner{
		name:   "Open chart popup",
		filter: func(m Model) string { return m.picker.filterText },
		popup:  true,
		open: func(t *testing.T) Model {
			m := popupBase(t, 30)
			m.picker = chartPickerState{loaded: true, prior: startupReady, charts: syntheticCharts(60),
				drops: []store.ChartDrop{{File: "a.as", Reason: "x"}, {File: "b.as", Reason: "y"}}}
			m.picker.refilter()
			m.startup = startupChartOpen
			return m
		},
		read: func(m Model) (int, int, int, int) {
			return m.picker.cursor, m.picker.offset, len(m.picker.matches), chartBlockHeight(m.picker, modalListHeight(m.height))
		},
		put: func(m Model, c, o int) Model { m.picker.cursor, m.picker.offset = c, o; return m },
		drawn: func(m Model) int {
			_, off := m.picker.windowAt(m.picker.cursor, chartBlockHeight(m.picker, modalListHeight(m.height)))
			return off
		},
	}, pageOwner{
		name:    "Houses popup",
		filter:  func(m Model) string { return m.houseFinder.filterText },
		popup:   true,
		endRows: func(m Model) int { return modalListHeight(m.height) },
		open: func(t *testing.T) Model {
			m := popupBase(t, 24)
			next, _ := openHouseFinder(m)
			return next.(Model)
		},
		read: func(m Model) (int, int, int, int) {
			f := m.houseFinder
			return f.cursor, f.offset, len(f.matches), f.housePageRows(modalListHeight(m.height))
		},
		put: func(m Model, c, o int) Model { m.houseFinder.cursor, m.houseFinder.offset = c, o; return m },
		drawn: func(m Model) int {
			_, off, _ := m.houseFinder.houseWindow(modalListHeight(m.height))
			return off
		},
	}, pageOwner{
		name:   "Zodiac popup",
		filter: func(m Model) string { return m.zodiacFinder.filterText },
		popup:  true,
		open: func(t *testing.T) Model {
			m := popupBase(t, 24)
			next, _ := openZodiacFinder(m)
			return next.(Model)
		},
		read: func(m Model) (int, int, int, int) {
			f := m.zodiacFinder
			return f.cursor, f.offset, len(f.matches), modalListHeight(m.height)
		},
		put: func(m Model, c, o int) Model { m.zodiacFinder.cursor, m.zodiacFinder.offset = c, o; return m },
		drawn: func(m Model) int {
			_, off := m.zodiacFinder.windowAt(m.zodiacFinder.cursor, modalListHeight(m.height))
			return off
		},
	}, pageOwner{
		name:   "Aspects popup",
		filter: func(m Model) string { return m.aspectModal.filterText },
		popup:  true,
		pinned: familyRows(),
		open: func(t *testing.T) Model {
			m := popupBase(t, 30)
			m.aspectRegistry = syntheticAspects(60)
			next, _ := openAspectChooser(m)
			return next.(Model)
		},
		read: func(m Model) (int, int, int, int) {
			a := m.aspectModal
			return a.cursor - familyRows(), a.offset, len(a.matches), aspectListHeight(m.height)
		},
		put: func(m Model, c, o int) Model {
			m.aspectModal.cursor, m.aspectModal.offset = familyRows()+c, o
			return m
		},
		drawn: func(m Model) int {
			_, off := m.aspectModal.windowAt(m.aspectModal.cursor-familyRows(), aspectListHeight(m.height))
			return off
		},
	}, pageOwner{
		name:    "Bodies popup",
		filter:  func(m Model) string { return m.bodyModal.filterText },
		popup:   true,
		pinned:  groupRows(),
		endRows: func(m Model) int { return bodyListHeight(m.height) },
		open: func(t *testing.T) Model {
			m := popupBase(t, 30)
			m.objectRegistry = syntheticObjects(80)
			next, _ := openBodySelector(m)
			return next.(Model)
		},
		read: func(m Model) (int, int, int, int) {
			b := m.bodyModal
			visible := bodyListHeight(m.height)
			return b.cursor - groupRows(), b.offset, len(b.matches), visible
		},
		put: func(m Model, c, o int) Model {
			m.bodyModal.cursor, m.bodyModal.offset = groupRows()+c, o
			return m
		},
		drawn: func(m Model) int {
			h := bodyListHeight(m.height)
			_, off := m.bodyModal.windowAt(m.bodyModal.cursor-groupRows(), h)
			return off
		},
	})
	return owners
}

// A1b: one table over ALL owners. With the cursor in the middle of a scrolled
// window, PgDn moves the cursor to the window's last row and leaves the window; a
// second PgDn moves the window by a page less a row, the cursor to its new last
// row; PgUp mirrors it. The expected numbers are written out here from the
// rule, not read from pageCursor, so an owner that pages by hand (or by a page,
// or with its own margin) fails by its name; and after every press the owner's
// draw-time correction finds nothing to correct.
func TestEveryCursorOwnerPagesCursorFirst(t *testing.T) {
	for _, o := range pageOwners() {
		o := o
		t.Run(o.name, func(t *testing.T) {
			m := o.open(t)
			const offset = 2
			m = o.put(m, offset+2, offset)
			c, off, total, v := o.read(m)
			if v < 4 || total <= off+v || c != off+2 || off != offset {
				t.Fatalf("fixture: cursor %d offset %d over %d rows with %d visible; want a cursor mid-window and room for a second page", c, off, total, v)
			}
			if got := o.drawn(m); got != off {
				t.Fatalf("fixture: the render draws from %d, the state says %d", got, off)
			}
			check := func(step, key string, wantCursor, wantOffset int) {
				t.Helper()
				m, _ = press(t, m, key)
				c, off, _, _ := o.read(m)
				if c != wantCursor || off != wantOffset {
					t.Fatalf("%s: cursor %d offset %d, want %d / %d (%d rows, %d visible)", step, c, off, wantCursor, wantOffset, total, v)
				}
				if got := o.drawn(m); got != off {
					t.Errorf("%s: the render draws from %d but the state stored %d: it corrects what paging set", step, got, off)
				}
			}
			check("first PgDn goes to the window's last row", "pgdown", off+v-1, off)
			page := min(off+v-1, total-v)
			check("second PgDn pages the window", "pgdown", page+v-1, page)
			check("PgUp goes to the window's first row", "pgup", page, page)
			back := max(page-(v-1), 0)
			check("second PgUp pages the window back", "pgup", back, back)
			if o.noHomeEnd {
				c0, o0, _, _ := o.read(m)
				for _, k := range []string{"end", "home"} {
					m, _ = press(t, m, k)
					if c1, o1, _, _ := o.read(m); c1 != c0 || o1 != o0 {
						t.Errorf("%s moved the cursor (%d/%d to %d/%d) on a line that takes text", k, c0, o0, c1, o1)
					}
				}
				return
			}
			endRows := v
			if o.endRows != nil {
				endRows = o.endRows(m)
			}
			check("End", "end", total-1, total-endRows)
			check("Home", "home", -o.pinned, 0)
		})
	}
}

// bodyPageOffset and bodyPageTop translate the stored window between display
// rows (the rule counts as one) and the cursor-row index the owner table works in.
func bodyPageOffset(planets, under, height, top int) int {
	if bodyRuled(planets, under, height) && top > planets {
		return top - 1
	}
	return top
}

func bodyPageTop(planets, under, height, offset int) int {
	if bodyRuled(planets, under, height) && offset >= planets {
		return offset + 1
	}
	return offset
}
