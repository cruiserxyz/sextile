package panel

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/internal/ui/menu"
	"github.com/cruiserxyz/sextile/internal/ui/pane"
	"github.com/cruiserxyz/sextile/internal/ui/term"
)

// TimelineSearch is what the panel's last row needs: the query, whether the
// line is open, and how many rows the list holds before the filter.
type TimelineSearch struct {
	Show  bool
	Open  bool
	Query string
	Total int
}

// TimelineBag is everything RenderTimeline draws from, resolved by ui: the rows
// as shown, which of them are marked (Marked[i] for Rows[i]; AnyMarked says
// the list has any mark at all), the nearest event, whether the list has a row
// cursor (Attached) and whether its panel's border shows focus (Focused: the
// one fact that lights the cursor row), the row cursor and window, the search line, and whether the body
// dims while its tab waits.
type TimelineBag struct {
	Rows      []lookup.TimelineRow
	Marked    []bool
	AnyMarked bool
	Nearest   int
	Attached  bool
	Focused   bool
	Cursor    int
	RowOffset int
	Search    TimelineSearch
	Dim       bool
	InnerW    int
	InnerH    int
	Style     term.Style
}

// DisplayRow is a row as the panel shows it, copies it and filters it:
// AlignRow's layout, with a comparing transit or return row's numeral
// standing in its empty weekday column, where a weekday's second letter would
// sit (` ① ` against `Sat`), so every row keeps its width and the columns of
// numbered and sky rows line up by construction (C11, P4; operator,
// 2026-10-04). A transit row never carries a weekday (astrolog.TimelineRow);
// were one ever to, the numeral goes in front rather than be lost.
func DisplayRow(r lookup.TimelineRow, cfg term.Style) string {
	return DisplayRowPad(r, cfg, false)
}

// DisplayRowPad is DisplayRow with the Julian mark's four cells
// reserved on a row that has none, so a list holding both keeps its columns.
func DisplayRowPad(r lookup.TimelineRow, cfg term.Style, pad bool) string {
	if r.Numeral > 0 {
		glyph := NumeralForColour(r.Numeral, cfg)
		if r.Weekday != "" {
			return glyph + " " + AlignRowPad(r, pad)
		}
		r.Weekday = " " + glyph
	}
	return AlignRowPad(r, pad)
}

// AnyJulian says whether any of the rows is dated on the Julian branch.
func AnyJulian(rows []lookup.TimelineRow) bool {
	for _, r := range rows {
		if r.Julian {
			return true
		}
	}
	return false
}

// AlignRow lays one row out in columns: weekday (4), date (10, right),
// time (7, right), "trans" (5), first body (7, right, never cut), then the
// rest exactly as Astrolog wrote it. Only the spaces between Astrolog's words
// are Sextile's (D9). The columns come from the parser; Line is not re-read.
func AlignRow(r lookup.TimelineRow) string { return AlignRowPad(r, false) }

// AlignRowPad is AlignRow with the Julian mark: ` Jul` after the
// date of a row Astrolog's reader took on the Julian branch (the specification
// C7), or four spaces in its place when pad says another row of the list has it.
func AlignRowPad(r lookup.TimelineRow, pad bool) string {
	trans := "     "
	if r.Transit {
		trans = "trans"
	}
	mark := ""
	if r.Julian {
		mark = " Jul"
	} else if pad {
		mark = "    "
	}
	return fmt.Sprintf("%-3s %10s%s %7s %s %7s%s", r.Weekday, r.Date, mark, r.Time, trans, r.Body, r.Rest)
}

// NoRowMatches is the one plain line a filter that keeps nothing leaves.
const NoRowMatches = "no row matches"

// drawSearchLine is searchLine as the panel draws it: a closed line that keeps
// a query is faint with colours on, the open line is plain.
func drawSearchLine(open bool, query string, shown, total, innerW int, cfg term.Style) string {
	line := term.CutRight(searchLine(open, query, shown, total, innerW), innerW, cfg)
	if !open && query != "" && cfg.UseColor() {
		return term.SGRFaint + menu.ReapplyAfterSGR(line, term.SGRFaint) + term.SGRReset
	}
	return line
}

// searchLine draws the last row: `/<query>█` with `N of M` right-aligned while
// open, `/<query>  N of M` once closed. The slashes are the mode's: `/` exact,
// `//` fuzzy, `///` exact respecting case.
func searchLine(open bool, query string, shown, total, innerW int) string {
	count := fmt.Sprintf("%d of %d", shown, total)
	if !open {
		return menu.Filter{Query: query}.Shown() + "  " + count
	}
	left := menu.Filter{Query: query}.Shown() + "█"
	gap := innerW - lipgloss.Width(left) - lipgloss.Width(count)
	if gap < 1 {
		return left
	}
	return left + strings.Repeat(" ", gap) + count
}

// DimBody makes every line of a Details body faint with colours on; with
// colours off the tab's title carries the ` …` mark instead.
func DimBody(body string, cfg term.Style) string {
	if body == "" || !cfg.UseColor() {
		return body
	}
	lines := strings.Split(body, "\n")
	for i, ln := range lines {
		lines[i] = term.SGRFaint + menu.ReapplyAfterSGR(ln, term.SGRFaint) + term.SGRReset
	}
	return strings.Join(lines, "\n")
}

// RenderTimeline draws the window of timeline rows. A row is Astrolog's
// line, clipped at the panel edge — truncated, never wrapped, because one
// row is one event and Enter and the cursor both depend on that (V2) — and
// the cursor row carries the shared active-row treatment only while the
// panel's border shows focus, and is bold alone while Details is attached and
// something else has the keyboard.
func RenderTimeline(bag TimelineBag) (string, pane.Viewport) {
	rows, search, innerW, innerH, cfg := bag.Rows, bag.Search, bag.InnerW, bag.InnerH, bag.Style
	if innerW <= 0 || innerH <= 0 {
		return "", pane.Viewport{}
	}
	listH := innerH
	if search.Show {
		listH = innerH - 1
	}
	scroll := pane.Viewport{Total: len(rows), Visible: listH, Offset: pane.ClampOffset(bag.RowOffset, len(rows), listH)}
	end := min(scroll.Offset+listH, len(rows))
	pad := AnyJulian(rows)
	out := make([]string, 0, innerH)
	for i := scroll.Offset; i < end; i++ {
		cursorRow := bag.Attached && i == bag.Cursor
		text := menu.RowPrefix(bag.Focused, cursorRow, i < len(bag.Marked) && bag.Marked[i], bag.AnyMarked, cfg) + DisplayRowPad(rows[i], cfg, pad)
		// The cursor row is bold with the band while Details has the keyboard;
		// the nearest event is bold, and nothing more, always (rule 2).
		state := menu.CursorRowState(bag.Focused, cursorRow)
		if state == menu.RowPlain && i == bag.Nearest {
			state = menu.RowCursorBlurred
		}
		out = append(out, menu.RenderRow(term.CutRight(text, innerW, cfg), innerW, state, cfg))
	}
	if search.Show {
		if len(rows) == 0 && search.Query != "" && listH > 0 {
			out = append(out, term.CutRight(NoRowMatches, innerW, cfg))
		}
		for len(out) < listH {
			out = append(out, "")
		}
		out = append(out, drawSearchLine(search.Open, search.Query, len(rows), search.Total, innerW, cfg))
	}
	body := strings.Join(out, "\n")
	if bag.Dim && len(rows) > 0 {
		body = DimBody(body, cfg)
	}
	return body, scroll
}
