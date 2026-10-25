package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/internal/recipe"
)

// zodiac_finder.go is the zodiac finder (⌃z, or the leader sequence `, z`),
// the house finder's sibling: Tropical and Astrolog's ten named ayanamsas in
// the shared picker chassis, and Enter casts the focused chart in the one under
// the cursor (the specification C8). The operator asked for it to be a
// popup of its own, not a second group in the house finder (2026-10-08).
//
// Like the house finder it adds no write path: Enter hands the choice to
// chooseZodiac, which records, saves the session once and recasts. Moving the
// cursor, typing and Esc touch nothing outside this state.

// zodiacFinderState is the finder's state. The list machinery is the chassis's.
type zodiacFinderState struct {
	fuzzyList[recipe.Zodiac]

	// slotIdx is the chart Enter changes: the focused chart, selected or not.
	slotIdx int
	// current is that chart's own resolved zodiac when it opened (marked ●);
	// the cursor starts on it.
	current recipe.Zodiac
	// name is the title's chart: the one Enter changes.
	name string
}

// refilter recomputes the list: all eleven in C2's order for an empty search,
// else the zodiacs the text matches, ranked by location.MatchTier as the house
// finder ranks, ties in C2's order.
func (f *zodiacFinderState) refilter() {
	q := strings.ToLower(strings.TrimSpace(f.filterText))
	var hits []recipe.Zodiac
	var tiers []int
	for z := recipe.Zodiac(0); int(z) < recipe.ZodiacCount; z++ {
		if q == "" {
			hits = append(hits, z)
			tiers = append(tiers, 0)
		} else if t := location.MatchTier(strings.ToLower(z.Name()), q); t > 0 {
			hits = append(hits, z)
			tiers = append(tiers, t)
		}
	}
	f.setMatches(rankByTier(hits, tiers), false)
}

// openZodiacFinder is the ⌃z / `, z` handler: it opens the finder for the
// focused chart, from the main screen only and once the first frame is in.
func openZodiacFinder(m Model) (tea.Model, tea.Cmd) {
	slot := m.focusedSlot()
	if m.startup != startupReady || slot == nil {
		return m, nil
	}
	st := zodiacFinderState{
		slotIdx: m.slotIndexOf(slot),
		current: m.ownZodiac(slot),
		name:    houseFinderChartName(slot.Chart.Name, slot.Floating),
	}
	st.refilter()
	for i, z := range st.matches {
		if z == st.current {
			st.cursor = i
		}
	}
	st.scrollTo(st.cursor, modalListHeight(m.height))
	m.zodiacFinder = st
	m.startup = startupZodiacOpen
	return m, nil
}

// updateZodiacFinderKey handles one keystroke while the finder has the keyboard.
func updateZodiacFinderKey(m Model, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	f := m.zodiacFinder
	height := modalListHeight(m.height)
	switch matchBinding(msg, zodiacBindings()) {
	case BindingClose:
		m.startup = startupReady
		return m, nil
	case BindingConfirm:
		// Nothing under the cursor, nothing to apply: the finder stays open.
		if f.cursor < 0 || f.cursor >= len(f.matches) {
			return m, nil
		}
		z := f.matches[f.cursor]
		m.startup = startupReady
		return m.chooseZodiac(f.slotIdx, z)
	case BindingListUp:
		f.moveUp()
		f.scrollTo(f.cursor, height)
	case BindingListDown:
		f.moveDown()
		f.scrollTo(f.cursor, height)
	case BindingPageUp:
		f.page(pageUp, height)
	case BindingPageDown:
		f.page(pageDown, height)
	case BindingPageHome:
		f.page(pageHome, height)
	case BindingPageEnd:
		f.page(pageEnd, height)
	default:
		if f.typeKey(msg) {
			f.refilter()
			f.cursor = 0
			f.scrollTo(f.cursor, height)
		}
	}
	m.zodiacFinder = f
	return m, nil
}

// zodiacFinderTitle is the house finder's title with the zodiac's prefix.
func zodiacFinderTitle(name, base string, cfg Config, termW int) string {
	return finderTitle("Zodiac · ", name, base, cfg, termW)
}

// renderZodiacFinder draws the finder.
func renderZodiacFinder(f zodiacFinderState, cfg Config, termW, termH int) string {
	pos := listPosition{Cursor: f.cursor, Total: len(f.matches)}
	return renderModalBox(modalBox{
		Title:    zodiacFinderTitle(f.name, "", cfg, termW),
		Position: &pos,
		Scroll: func(height int) (viewport, int) {
			rows, off := f.windowAt(f.cursor, height)
			return viewport{Total: len(f.matches), Visible: len(rows), Offset: off}, 0
		},
		FilterText: f.filterText,
	}, cfg, termW, termH, func(width, height int) string {
		return renderPinnedBody(pinnedBody{Rows: zodiacFinderRows(f, cfg, width, height)}, width, height)
	})
}

// zodiacNameColumn is the width a row gives the zodiac's name before the
// account of its offset begins (the specification F1).
const zodiacNameColumn = 17

// zodiacFinderRows renders the visible zodiacs: name and the account of its
// offset on the left, the tag code pinned right and, on the chart's current
// zodiac, a dot after it.
func zodiacFinderRows(f zodiacFinderState, cfg Config, width, height int) []string {
	if height < 1 {
		return nil
	}
	if len(f.matches) == 0 {
		return []string{listNote("(no matching zodiacs)", width, cfg)}
	}
	rows, off := f.windowAt(f.cursor, height)
	out := make([]string, 0, len(rows))
	for i, z := range rows {
		mark := " "
		if z == f.current {
			mark = "●"
		}
		left := z.Name()
		if gloss := lookup.ZodiacOffsetText(z); gloss != "" {
			left += strings.Repeat(" ", max(zodiacNameColumn-ansi.StringWidth(left), 1)) + gloss
		}
		out = append(out, listRow(listRowSpec{
			Left:   left,
			Right:  []string{lookup.ZodiacCode(z), mark},
			Cursor: off+i == f.cursor,
			Pinned: true,
		}, cfg, width))
	}
	return out
}
