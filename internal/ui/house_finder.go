package ui

import (
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/internal/recipe"
)

// house_finder.go is the house finder (⌃h, or the leader sequence `, h`):
// Astrolog's 22 house systems in the shared picker chassis, and Enter casts the
// focused chart in the one under the cursor (the specification).
//
// It adds no write path. Enter hands the choice to chooseHouseSystem, the one
// door a house choice comes through (the specification), which resolves,
// records, saves the session once and recasts. Moving the cursor, typing and
// Esc touch nothing outside this state, and there is no live preview: the
// finder is a chooser, not a try-on.

// houseFinderState is the finder's state. The list machinery is the chassis's.
type houseFinderState struct {
	fuzzyList[recipe.HouseSystem]

	// slotIdx is the chart Enter changes: the focused chart, selected or not.
	slotIdx int
	// current is that chart's own resolved system when it opened (marked ●);
	// the cursor starts on it.
	current recipe.HouseSystem
	// name is the title's chart: the one Enter changes.
	name string
}

// housePinned are the seven systems that lead an empty search, in this order
// (operator, 2026-10-08, the specification).
var housePinned = []recipe.HouseSystem{
	recipe.Placidus, recipe.Koch, recipe.WholeSign, recipe.Equal,
	recipe.Porphyry, recipe.Regiomontanus, recipe.Campanus,
}

func isHousePinned(h recipe.HouseSystem) bool {
	for _, p := range housePinned {
		if p == h {
			return true
		}
	}
	return false
}

// refilter recomputes the list. An empty search is the pinned seven and then the
// other fifteen alphabetically, each system once. Any text makes one list of
// all 22 ranked by location.MatchTier (exact, starts-with, contains, then the
// typed letters in order), ties in Astrolog's own order.
func (f *houseFinderState) refilter() {
	q := strings.ToLower(strings.TrimSpace(f.filterText))
	if q == "" {
		out := append([]recipe.HouseSystem(nil), housePinned...)
		var rest []recipe.HouseSystem
		for h := recipe.HouseSystem(0); h < recipe.HouseSystemCount; h++ {
			if !isHousePinned(h) {
				rest = append(rest, h)
			}
		}
		sort.SliceStable(rest, func(i, j int) bool {
			return strings.ToLower(lookup.HouseName(rest[i])) < strings.ToLower(lookup.HouseName(rest[j]))
		})
		f.setMatches(append(out, rest...), false)
		return
	}
	var hits []recipe.HouseSystem
	var tiers []int
	for h := recipe.HouseSystem(0); h < recipe.HouseSystemCount; h++ {
		if t := location.MatchTier(strings.ToLower(lookup.HouseName(h)), q); t > 0 {
			hits = append(hits, h)
			tiers = append(tiers, t)
		}
	}
	f.setMatches(rankByTier(hits, tiers), false)
}

// rankByTier orders items best tier first, keeping the list's own order within
// a tier. tiers[i] is items[i]'s location.MatchTier.
func rankByTier[T any](items []T, tiers []int) []T {
	idx := make([]int, len(items))
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, func(a, b int) bool { return tiers[idx[a]] > tiers[idx[b]] })
	out := make([]T, len(items))
	for i, k := range idx {
		out[i] = items[k]
	}
	return out
}

// houseFinderChartName is how the title names a chart.
func houseFinderChartName(name string, floating bool) string {
	if floating || strings.TrimSpace(name) == "" {
		return "Now"
	}
	return name
}

// openHouseFinder is the ⌃h / `, h` handler: it opens the finder for the
// focused chart, from the main screen only and once the first frame is in.
// Enter changes the focused chart's own system and no other chart's, in a
// group or not (operator, 2026-10-24: "only the focused chart should have
// it"); the title names that chart.
func openHouseFinder(m Model) (tea.Model, tea.Cmd) {
	slot := m.focusedSlot()
	if m.startup != startupReady || slot == nil {
		return m, nil
	}
	st := houseFinderState{
		slotIdx: m.slotIndexOf(slot),
		current: m.ownHouses(slot).hs,
		name:    houseFinderChartName(slot.Chart.Name, slot.Floating),
	}
	st.refilter()
	for i, h := range st.matches {
		if h == st.current {
			st.cursor = i
		}
	}
	st.scrollTo(st.cursor, modalListHeight(m.height))
	m.houseFinder = st
	m.startup = startupHouseOpen
	return m, nil
}

// updateHouseFinderKey handles one keystroke while the finder has the keyboard.
func updateHouseFinderKey(m Model, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	f := m.houseFinder
	height := modalListHeight(m.height)
	switch matchBinding(msg, housesBindings()) {
	case BindingClose:
		m.startup = startupReady
		return m, nil
	case BindingConfirm:
		// Nothing under the cursor, nothing to apply: the finder stays open.
		if f.cursor < 0 || f.cursor >= len(f.matches) {
			return m, nil
		}
		hs := f.matches[f.cursor]
		m.startup = startupReady
		return m.chooseHouseSystem(f.slotIdx, hs)
	case BindingListUp:
		f.moveUp()
		f.scrollTo(f.cursor, height)
	case BindingListDown:
		f.moveDown()
		f.scrollTo(f.cursor, height)
	case BindingPageUp:
		f.pageHouses(pageUp, height)
	case BindingPageDown:
		f.pageHouses(pageDown, height)
	case BindingPageHome:
		f.pageHouses(pageHome, height)
	case BindingPageEnd:
		f.pageHouses(pageEnd, height)
	default:
		if f.typeKey(msg) {
			f.refilter()
			f.cursor = 0
			f.scrollTo(f.cursor, height)
		}
	}
	m.houseFinder = f
	return m, nil
}

// houseFinderTitle names the chart Enter changes (base is "" now that the
// chart named is the one that decides; the clause stays for finderTitle's
// callers that pass one). It always fits the box's top edge.
func houseFinderTitle(name, base string, cfg Config, termW int) string {
	return finderTitle("Houses · ", name, base, cfg, termW)
}

// finderTitle is the title both finders wear: prefix, the chart the choice
// changes and, when that chart is selected but not the group's first, the chart
// that decides the comparison. The comparison clause gives way first.
func finderTitle(prefix, name, base string, cfg Config, termW int) string {
	const mid = " — comparison uses "
	// The box is clamp(termW/2, 60, 90) wide; its top edge keeps four cells
	// for the corners and one for the focus mark.
	budget := clamp(termW/2, 60, 90) - 5
	w := ansi.StringWidth
	if base == "" {
		return prefix + cutRight(name, budget-w(prefix), cfg)
	}
	if full := prefix + name + mid + base; w(full) <= budget {
		return full
	}
	avail := budget - w(prefix) - w(mid)
	if avail < 16 {
		return prefix + cutRight(name, budget-w(prefix), cfg)
	}
	baseW := min(w(base), avail/3)
	nameW := min(w(name), avail-baseW)
	baseW = min(w(base), avail-nameW)
	return prefix + cutRight(name, nameW, cfg) + mid + cutRight(base, baseW, cfg)
}

// houseWindow is the rows to draw, the index of the first, and whether the rule
// between the pinned seven and the rest is drawn (it takes a row of the window,
// so the window is one row shorter then and still holds the cursor's row).
func (f houseFinderState) houseWindow(height int) (rows []recipe.HouseSystem, off int, rule bool) {
	rows, off = f.windowAt(f.cursor, height)
	if strings.TrimSpace(f.filterText) != "" || height < 2 {
		return rows, off, false
	}
	n := len(housePinned)
	if off <= n-1 && off+len(rows) >= n+1 {
		rows, off = f.windowAt(f.cursor, height-1)
		rule = off <= n-1 && off+len(rows) >= n+1
	}
	return rows, off, rule
}

// housePageRows is the number of list rows a page can rely on: the rule between
// the pinned seven and the rest takes a row of the window whenever the window
// straddles it (houseWindow), so with no filter typed the smallest count the
// render shows is one row short.
func (f houseFinderState) housePageRows(height int) int {
	if strings.TrimSpace(f.filterText) == "" && height >= 2 {
		return height - 1
	}
	return height
}

// pageHouses applies a paging key by the one rule over that count, then holds
// the stored window where houseWindow draws it. The two differ by one row only
// at the end of the list, where the window past the rule shows a row more than
// the page counted on; the draw-time correction then finds nothing to correct.
func (f *houseFinderState) pageHouses(op pageOp, height int) {
	f.page(op, f.housePageRows(height))
	_, off, _ := f.houseWindow(height)
	f.offset = off
}

// renderHouseFinder draws the finder.
func renderHouseFinder(f houseFinderState, cfg Config, termW, termH int) string {
	pos := listPosition{Cursor: f.cursor, Total: len(f.matches)}
	return renderModalBox(modalBox{
		Title:    houseFinderTitle(f.name, "", cfg, termW),
		Position: &pos,
		Scroll: func(height int) (viewport, int) {
			rows, off, _ := f.houseWindow(height)
			return viewport{Total: len(f.matches), Visible: len(rows), Offset: off}, 0
		},
		FilterText: f.filterText,
	}, cfg, termW, termH, func(width, height int) string {
		return renderPinnedBody(pinnedBody{Rows: houseFinderRows(f, cfg, width, height)}, width, height)
	})
}

// houseFinderRows renders the visible systems: name on the left, the tag code
// pinned right and, on the chart's current system, a dot after it.
func houseFinderRows(f houseFinderState, cfg Config, width, height int) []string {
	if height < 1 {
		return nil
	}
	if len(f.matches) == 0 {
		return []string{listNote("(no matching house systems)", width, cfg)}
	}
	rows, off, rule := f.houseWindow(height)
	sepCh := "─"
	if cfg.useASCII() {
		sepCh = "-"
	}
	out := make([]string, 0, len(rows)+1)
	for i, h := range rows {
		if rule && off+i == len(housePinned) {
			out = append(out, listNote(strings.Repeat(sepCh, width), width, cfg))
		}
		mark := " "
		if h == f.current {
			mark = "●"
		}
		out = append(out, listRow(listRowSpec{
			Left:   lookup.HouseName(h),
			Right:  []string{lookup.HouseCode(h), mark},
			Cursor: off+i == f.cursor,
			Pinned: true,
		}, cfg, width))
	}
	return out
}
