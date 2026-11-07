package popup

import (
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/internal/recipe"
	"github.com/cruiserxyz/sextile/internal/ui/dialog"
	"github.com/cruiserxyz/sextile/internal/ui/menu"
	"github.com/cruiserxyz/sextile/internal/ui/pane"
	"github.com/cruiserxyz/sextile/internal/ui/term"
)

// finder.go is the house finder (⌃h, `, h`) and the zodiac finder (⌃z, `, z`):
// small pickers in the shared list chassis, and Enter casts the focused chart in
// the one under the cursor. They share one shape, so they share one type: a
// FinderKind says what differs (the list, a row, the title's prefix, the pinned
// systems that lead an empty search). Neither adds a write path: Enter is
// reported as an outcome and ui hands the choice to its one door for it.

// FinderKind is what makes one Finder the house finder and another the zodiac
// finder.
type FinderKind[T comparable] struct {
	// Prefix leads the title ("Houses · "); the chart the choice changes follows.
	Prefix string
	// NoMatch is the note shown when the search matches nothing.
	NoMatch string
	// All is every choice, for the box's size.
	All []T
	// Pinned is how many choices lead an empty search, set apart from the rest by
	// a rule; zero for none.
	Pinned int
	// List is the choices for a search text (lower-cased and trimmed).
	List func(q string) []T
	// Row is one choice as the finder lists it.
	Row func(x T, current, cursor bool, cfg term.Style) menu.RowSpec
}

// Finder is a finder's state. The list machinery is the chassis's.
type Finder[T comparable] struct {
	menu.FuzzyList[T]
	Kind FinderKind[T]

	// SlotIdx is the chart Enter changes: the focused chart, selected or not.
	SlotIdx int
	// Current is that chart's own resolved choice when it opened (marked ●);
	// the cursor starts on it.
	Current T
	// Name is the title's chart: the one Enter changes.
	Name string
}

// FinderOutcome is what one key decided: Chosen is set when Enter fell on a row.
type FinderOutcome[T comparable] struct {
	Dismissed bool
	Confirmed bool
	Chosen    T
}

// Refilter recomputes the list.
func (f *Finder[T]) Refilter() {
	f.SetMatches(f.Kind.List(strings.ToLower(strings.TrimSpace(f.FilterText))), false)
}

// Open puts the cursor on the current choice and scrolls it into view.
func (f *Finder[T]) Open(listHeight int) {
	f.Refilter()
	for i, x := range f.Matches {
		if x == f.Current {
			f.Cursor = i
		}
	}
	f.ScrollTo(f.Cursor, listHeight)
}

// Apply runs one action. msg is read only by Type, for the filter text.
func (f *Finder[T]) Apply(a Action, msg tea.KeyMsg, height int) FinderOutcome[T] {
	var out FinderOutcome[T]
	switch a {
	case Dismiss:
		out.Dismissed = true
	case Confirm:
		// Nothing under the cursor, nothing to apply: the finder stays open.
		if f.Cursor < 0 || f.Cursor >= len(f.Matches) {
			return out
		}
		out.Confirmed, out.Chosen = true, f.Matches[f.Cursor]
	case Up:
		f.MoveUp()
		f.ScrollTo(f.Cursor, height)
	case Down:
		f.MoveDown()
		f.ScrollTo(f.Cursor, height)
	case PageUp:
		f.page(menu.PageUp, height)
	case PageDown:
		f.page(menu.PageDown, height)
	case PageHome:
		f.page(menu.PageHome, height)
	case PageEnd:
		f.page(menu.PageEnd, height)
	default:
		if f.TypeKey(msg) {
			f.Refilter()
			f.Cursor = 0
			f.ScrollTo(f.Cursor, height)
		}
	}
	return out
}

// Window is the rows to draw, the index of the first, and whether the rule
// between the pinned choices and the rest is drawn (it takes a row of the
// window, so the window is one row shorter then and still holds the cursor's row).
func (f Finder[T]) Window(height int) (rows []T, off int, rule bool) {
	rows, off = f.WindowAt(f.Cursor, height)
	if f.Kind.Pinned == 0 || strings.TrimSpace(f.FilterText) != "" || height < 2 {
		return rows, off, false
	}
	n := f.Kind.Pinned
	if off <= n-1 && off+len(rows) >= n+1 {
		rows, off = f.WindowAt(f.Cursor, height-1)
		rule = off <= n-1 && off+len(rows) >= n+1
	}
	return rows, off, rule
}

// PageRows is the number of list rows a page can rely on: the rule takes a row
// of the window whenever the window straddles it (Window), so with no filter
// typed the smallest count the render shows is one row short.
func (f Finder[T]) PageRows(height int) int {
	if f.Kind.Pinned > 0 && strings.TrimSpace(f.FilterText) == "" && height >= 2 {
		return height - 1
	}
	return height
}

// page applies a paging key. With pinned choices it is the one rule over
// PageRows, then the stored window is held where Window draws it (the two
// differ by one row only at the end of the list, where the window past the rule
// shows a row more than the page counted on). Without, it is the chassis's.
func (f *Finder[T]) page(op menu.PageOp, height int) {
	if f.Kind.Pinned == 0 {
		f.Page(op, height)
		return
	}
	f.Page(op, f.PageRows(height))
	_, off, _ := f.Window(height)
	f.Offset = off
}

// Render draws the finder with the chrome and the position counter the popups
// share.
func (f Finder[T]) Render(cfg term.Style, termW, termH int, chrome dialog.Chrome, counter func(cursor, total int) *dialog.Counter) string {
	return dialog.Render(dialog.Box{
		Title:    FinderTitle(f.Kind.Prefix, f.Name, "", cfg, termW),
		Fit:      f.fit(cfg),
		Position: counter(f.Cursor, len(f.Matches)),
		Scroll: func(height int) (pane.Viewport, int) {
			rows, off, _ := f.Window(height)
			return pane.Viewport{Total: len(f.Matches), Visible: len(rows), Offset: off}, 0
		},
		FilterText: f.FilterText,
	}, cfg, termW, termH, chrome, func(width, height int) string {
		return menu.RenderPinnedBody(menu.PinnedBody{Rows: f.rows(cfg, width, height)}, width, height)
	})
}

// fit is the finder's size: wide enough for the longest choice and as tall as
// all of them (and the rule, while no search is typed). A search leaves rows blank.
func (f Finder[T]) fit(cfg term.Style) dialog.Fit {
	fit := dialog.Fit{Rows: len(f.Kind.All)}
	if f.Kind.Pinned > 0 {
		fit.Rows++
	}
	for _, x := range f.Kind.All {
		fit.Width = max(fit.Width, menu.RowWidth(f.Kind.Row(x, false, false, cfg), cfg))
	}
	return fit
}

// rows renders the visible choices.
func (f Finder[T]) rows(cfg term.Style, width, height int) []string {
	if height < 1 {
		return nil
	}
	if len(f.Matches) == 0 {
		return []string{menu.Note(f.Kind.NoMatch, width, cfg)}
	}
	rows, off, rule := f.Window(height)
	out := make([]string, 0, len(rows)+1)
	for i, x := range rows {
		if rule && off+i == f.Kind.Pinned {
			out = append(out, menu.Rule(cfg, width))
		}
		out = append(out, menu.Row(f.Kind.Row(x, x == f.Current, off+i == f.Cursor, cfg), cfg, width))
	}
	return out
}

// FinderTitle is the title both finders wear: prefix, the chart the choice
// changes and, when that chart is selected but not the group's first, the chart
// that decides the comparison. The comparison clause gives way first.
func FinderTitle(prefix, name, base string, cfg term.Style, termW int) string {
	const mid = " — comparison uses "
	// The box is as wide as its content, and at least as wide as the title up
	// to dialog.TitleBudget; beyond that the title is cut.
	budget := dialog.TitleBudget(termW)
	w := ansi.StringWidth
	if base == "" {
		return prefix + term.CutRight(name, budget-w(prefix), cfg)
	}
	if full := prefix + name + mid + base; w(full) <= budget {
		return full
	}
	avail := budget - w(prefix) - w(mid)
	if avail < 16 {
		return prefix + term.CutRight(name, budget-w(prefix), cfg)
	}
	baseW := min(w(base), avail/3)
	nameW := min(w(name), avail-baseW)
	baseW = min(w(base), avail-nameW)
	return prefix + term.CutRight(name, nameW, cfg) + mid + term.CutRight(base, baseW, cfg)
}

// HousePinned are the seven systems that lead an empty search, in this order
// (operator, 2026-10-08, the specification).
var HousePinned = []recipe.HouseSystem{
	recipe.Placidus, recipe.Koch, recipe.WholeSign, recipe.Equal,
	recipe.Porphyry, recipe.Regiomontanus, recipe.Campanus,
}

func isHousePinned(h recipe.HouseSystem) bool {
	for _, p := range HousePinned {
		if p == h {
			return true
		}
	}
	return false
}

// HouseKind is the house finder: Astrolog's 22 house systems. An empty search
// is the pinned seven and then the other fifteen alphabetically, each system
// once. Any text makes one list of all 22 ranked by location.MatchTier (exact,
// starts-with, contains, then the typed letters in order), ties in Astrolog's
// own order.
func HouseKind() FinderKind[recipe.HouseSystem] {
	var all []recipe.HouseSystem
	for h := recipe.HouseSystem(0); h < recipe.HouseSystemCount; h++ {
		all = append(all, h)
	}
	return FinderKind[recipe.HouseSystem]{
		Prefix:  dialog.TitleHouses + " · ",
		NoMatch: "(no matching house systems)",
		All:     all,
		Pinned:  len(HousePinned),
		List: func(q string) []recipe.HouseSystem {
			if q == "" {
				out := append([]recipe.HouseSystem(nil), HousePinned...)
				var rest []recipe.HouseSystem
				for h := recipe.HouseSystem(0); h < recipe.HouseSystemCount; h++ {
					if !isHousePinned(h) {
						rest = append(rest, h)
					}
				}
				sort.SliceStable(rest, func(i, j int) bool {
					return strings.ToLower(lookup.HouseName(rest[i])) < strings.ToLower(lookup.HouseName(rest[j]))
				})
				return append(out, rest...)
			}
			var hits []recipe.HouseSystem
			var tiers []int
			for h := recipe.HouseSystem(0); h < recipe.HouseSystemCount; h++ {
				if t := location.MatchTier(strings.ToLower(lookup.HouseName(h)), q); t > 0 {
					hits = append(hits, h)
					tiers = append(tiers, t)
				}
			}
			return menu.RankByTier(hits, tiers)
		},
		Row: func(h recipe.HouseSystem, current, cursor bool, _ term.Style) menu.RowSpec {
			// The mark is two cells: the dot and the room a wide-drawn dot spills into.
			mark := "  "
			if current {
				mark = "● "
			}
			return menu.RowSpec{
				Left:   lookup.HouseName(h),
				Right:  []string{lookup.HouseCode(h), mark},
				Cursor: cursor,
				Pinned: true,
			}
		},
	}
}

// zodiacNameColumn is the width a row gives the zodiac's name before the
// account of its offset begins (the specification F1).
const zodiacNameColumn = 17

// ZodiacKind is the zodiac finder: Tropical and Astrolog's ten named ayanamsas.
// All eleven in order for an empty search, else the zodiacs the text matches,
// ranked by location.MatchTier as the house finder ranks, ties in order.
func ZodiacKind() FinderKind[recipe.Zodiac] {
	var all []recipe.Zodiac
	for z := recipe.Zodiac(0); int(z) < recipe.ZodiacCount; z++ {
		all = append(all, z)
	}
	return FinderKind[recipe.Zodiac]{
		Prefix:  dialog.TitleZodiac + " · ",
		NoMatch: "(no matching zodiacs)",
		All:     all,
		List: func(q string) []recipe.Zodiac {
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
			return menu.RankByTier(hits, tiers)
		},
		Row: func(z recipe.Zodiac, current, cursor bool, _ term.Style) menu.RowSpec {
			mark := "  "
			if current {
				mark = "● "
			}
			left := z.Name()
			if gloss := lookup.ZodiacOffsetText(z); gloss != "" {
				left += strings.Repeat(" ", max(zodiacNameColumn-ansi.StringWidth(left), 1)) + gloss
			}
			return menu.RowSpec{
				Left:   left,
				Right:  []string{lookup.ZodiacCode(z), mark},
				Cursor: cursor,
				Pinned: true,
			}
		},
	}
}
