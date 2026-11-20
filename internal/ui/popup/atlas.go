package popup

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/ui/dialog"
	"github.com/cruiserxyz/sextile/internal/ui/menu"
	"github.com/cruiserxyz/sextile/internal/ui/pane"
	"github.com/cruiserxyz/sextile/internal/ui/term"
)

// atlas.go is the City picker (⌃l): a typed search over the atlas and its rows.
// The atlas load, the pick's effect on the chart and the phase Esc puts back
// stay in ui; the popup holds the index it was handed and reports the pick.

// Atlas carries the stateful place picker.
//
// Invariants:
//   - Matches is always derived from FilterText via Refilter(); never set
//     directly. Keeps cursor + matches in sync.
//   - Cursor is always in [0, len(Matches)] — clamped on every Refilter.
//   - Index is nil until the atlas has loaded; the popup renders a loading
//     line when nil.
//   - LoadErr non-nil → the popup renders an error line and Enter does nothing.
type Atlas struct {
	// The search buffer, cursor, matches and truncation flag live in the
	// shared chassis and are embedded. What stays here is what only the place
	// picker has.
	menu.FuzzyList[location.PlaceInfo]

	Index   []location.Indexed
	Preseed string
	LoadErr error
}

// MaxAtlasResults caps the matched-list size per render. 200 keeps render
// + alloc cheap on broad queries (single-char) and is well above what a
// modal of height ~24 can show. The truncation hint surfaces to the user.
const MaxAtlasResults = 200

// Refilter recomputes matches from FilterText against the loaded index.
// Cursor clamps into the new matches slice. No-op when the index has not
// been loaded.
func (a *Atlas) Refilter() {
	if a.Index == nil {
		a.Clear()
		return
	}
	a.SetMatches(location.Filter(a.Index, a.FilterText, MaxAtlasResults))
}

// AtlasOutcome is what one key decided: Picked with the place under the cursor
// for Enter, or Dismissed for Esc.
type AtlasOutcome struct {
	Picked    bool
	Place     location.PlaceInfo
	Dismissed bool
}

// Apply runs one action. msg is read only by Type, for the filter text;
// listHeight is the rows the list is drawn with. Every typed character goes
// into the filter, `q` included: the quit binding is the main screen's.
func (a *Atlas) Apply(act Action, msg tea.KeyMsg, listHeight int) AtlasOutcome {
	switch act {
	case Confirm:
		if a.LoadErr != nil || len(a.Matches) == 0 {
			return AtlasOutcome{}
		}
		return AtlasOutcome{Picked: true, Place: a.Matches[a.Cursor]}
	case Up:
		a.MoveUp()
		a.ScrollTo(a.Cursor, listHeight)
	case Down:
		a.MoveDown()
		a.ScrollTo(a.Cursor, listHeight)
	case PageUp:
		a.Page(menu.PageUp, listHeight)
	case PageDown:
		a.Page(menu.PageDown, listHeight)
	case PageHome:
		a.Page(menu.PageHome, listHeight)
	case PageEnd:
		a.Page(menu.PageEnd, listHeight)
	case Dismiss:
		return AtlasOutcome{Dismissed: true}
	default:
		if a.TypeKey(msg) {
			a.Refilter()
			a.Cursor = 0
			a.Offset = 0
		}
	}
	return AtlasOutcome{}
}

// Render draws the centered modal: search line + result list + footer. The
// chrome is the shared dialog.Box; only the list below is the picker's own.
// counter makes the position counter and hands back the string the box shows
// in its place (a cut name); noPlaces is the line shown when the atlas could
// not be read.
func (a Atlas) Render(cfg term.Style, termW, termH int, chrome dialog.Chrome, counter func(cursor, total int) (*dialog.Counter, *string), noPlaces string) string {
	total := 0
	if a.Index != nil && a.LoadErr == nil && a.FilterText != "" {
		total = len(a.Matches)
	}
	ctr, text := counter(a.Cursor, total)
	box := dialog.Box{
		Title:    dialog.TitleCity,
		Fit:      a.Fit(),
		Position: ctr,
		Scroll: func(height int) (pane.Viewport, int) {
			_, offset := a.WindowAt(a.Cursor, height)
			return pane.Viewport{Total: total, Visible: height, Offset: offset}, 0
		},
		FilterText: a.FilterText,
	}
	// A name the row had to cut is shown whole in the bottom edge, where the
	// counter is, for as long as the cursor is on it.
	if total > 0 && a.Cursor >= 0 && a.Cursor < len(a.Matches) {
		p := a.Matches[a.Cursor]
		row := menu.RowSpec{Left: p.Name, Right: []string{p.Country}}
		if w := dialog.ContentWidth(box, cfg, termW); menu.RowWidth(row, cfg) > w {
			*text = term.CutRight(p.Name, w+dialog.ChromeWidth-4, cfg)
		}
	}
	return dialog.Render(box, cfg, termW, termH, chrome, func(width, height int) string {
		return a.ResultList(cfg, width, height, noPlaces)
	})
}

// Fit is the place picker's size: as wide as its hint line and as tall as what
// it shows, up to eight rows (operator, 2026-11-14: no popup keeps an empty
// row, so the box grows with the matches and shrinks back). The atlas has
// 33,627 places and one of 57 cells, so no width follows its content: a longer
// name is cut with the cut mark, and while its row is under the cursor the
// box's bottom edge shows the name.
func (a Atlas) Fit() dialog.Fit {
	rows := 1
	if a.LoadErr == nil && a.Index != nil && a.FilterText != "" {
		rows = max(len(a.Matches), 1)
		if a.Truncated {
			rows++
		}
	}
	return dialog.Fit{Width: ansi.StringWidth(AtlasTypeHint), Rows: min(rows, atlasRows)}
}

// atlasRows is the most result rows the City list shows.
const atlasRows = 8

// AtlasTypeHint is the empty chooser's line: what to type, with a query that
// ranks its example first. No comma: the search key is `name country`.
const AtlasTypeHint = "Type a city, e.g. Amsterdam. Narrow it: Paris FR"

// ResultList draws the place picker's list: the city on the left and its
// country code pinned to the right edge (the specification C8).
func (a Atlas) ResultList(cfg term.Style, width, height int, noPlaces string) string {
	if a.LoadErr != nil {
		return menu.Message(noPlaces, width, height)
	}
	if a.Index == nil {
		return menu.Message("(loading atlas…)", width, height)
	}
	if a.FilterText == "" {
		return menu.Message(AtlasTypeHint, width, height)
	}
	if len(a.Matches) == 0 {
		return menu.Message("(no matches)", width, height)
	}

	// The window scrolls to follow the cursor; without it the cursor walks
	// past the bottom of the list and simply vanishes.
	visible, offset := a.WindowAt(a.Cursor, height)
	lines := make([]string, 0, height)
	for i, p := range visible {
		lines = append(lines, menu.Row(menu.RowSpec{
			Left:   p.Name,
			Right:  []string{p.Country},
			Cursor: offset+i == a.Cursor,
		}, cfg, width))
	}
	if a.Truncated && len(lines) < height {
		lines = append(lines, menu.Note(fmt.Sprintf("…%d more", len(a.Matches)-len(visible)), width, cfg))
	}
	return menu.Block(lines, width, height)
}
