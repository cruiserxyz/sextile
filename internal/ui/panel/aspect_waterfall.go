package panel

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/internal/astrologstyle"
	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/internal/ui/term"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

const (
	AspectColWidth = 15
	// Columns abut directly: each entry is right-padded to aspectColWidth, so a
	// zero gutter still leaves the short entries' trailing pad as visual
	// separation while packing one more column into the same width.
	aspectGutter = 0
)

func RenderAspectWaterfallStyled(aspects []aspect.Aspect, styles astrologstyle.Aspects, innerWidth, innerHeight int, cfg term.Style) string {
	return renderAspectWaterfallMarked(aspects, styles, innerWidth, innerHeight, cfg, EnteredMark{}, false)
}

// renderAspectWaterfallMarked is the waterfall with the entered row's cell, if
// the list holds it, drawn bold.
func renderAspectWaterfallMarked(aspects []aspect.Aspect, styles astrologstyle.Aspects, innerWidth, innerHeight int, cfg term.Style, mark EnteredMark, dot bool) string {
	if len(aspects) == 0 {
		return ""
	}
	sorted := append([]aspect.Aspect(nil), aspects...)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].Power > sorted[j].Power
	})
	cells, room := 1, 0
	for _, a := range sorted {
		cells = max(cells, sideCells(a.AObject), sideCells(a.BObject))
		room = max(room, orbRoom(a.BObject, a.Orb))
	}
	entries := make([]string, len(sorted))
	keep := -1
	for i, a := range sorted {
		var style astrologstyle.AspectStyle
		if a.AObject != (aspect.Object{}) && a.BObject != (aspect.Object{}) {
			// A side the registry could not resolve (the zero Object) is not a
			// style key. Earth, object 0, is a resolved side with its own name.
			style = styles[astrologstyle.AspectIdentity{First: a.AObject, Second: a.BObject, Kind: a.Kind}]
		}
		if mark.ownCell(a) {
			entries[i] = BoldCell(formatAspectEntryCells(a, style, cfg, cells, room, mark.Colour, dot))
			keep = i
		} else {
			entries[i] = formatAspectEntryCells(a, style, cfg, cells, room, "", dot)
		}
	}
	return layoutWaterfallGridKeeping(entries, keep, entryWidth(cells)+room, innerWidth, innerHeight)
}

func RenderCrossAspectWaterfallStyled(aspects []lookup.OverlayAspect, styles astrologstyle.Aspects, innerWidth, innerHeight int, cfg term.Style) string {
	return renderCrossAspectWaterfallPair(aspects, styles, innerWidth, innerHeight, cfg, true)
}

// renderCrossAspectWaterfallPair is the cross waterfall with the phase switch:
// phase=true is today's transit pair (each cell ends A or S, Astrolog's verdict
// with the natal chart held still); phase=false is a pair of two anchored
// charts, whose Astrolog verdict is a synthetic one and is never displayed.
func renderCrossAspectWaterfallPair(aspects []lookup.OverlayAspect, styles astrologstyle.Aspects, innerWidth, innerHeight int, cfg term.Style, phase bool) string {
	return renderCrossAspectWaterfallMarked(aspects, styles, innerWidth, innerHeight, cfg, phase, EnteredMark{}, false)
}

// renderCrossAspectWaterfallMarked is the cross waterfall with the entered
// row's cell, if the list holds it, drawn bold.
func renderCrossAspectWaterfallMarked(aspects []lookup.OverlayAspect, styles astrologstyle.Aspects, innerWidth, innerHeight int, cfg term.Style, phase bool, mark EnteredMark, dot bool) string {
	if len(aspects) == 0 {
		return ""
	}
	sorted := append([]lookup.OverlayAspect(nil), aspects...)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].Power > sorted[j].Power
	})
	cells, room := 1, 0
	for _, a := range sorted {
		cells = max(cells, sideCells(a.NatalObject), sideCells(a.TransitObject))
		room = max(room, orbRoom(a.NatalObject, a.Orb))
	}
	entries := make([]string, len(sorted))
	keep := -1
	for i, a := range sorted {
		// Astrolog emits primary/natal first and secondary/transit second; the
		// style key keeps that order, the cell prints the transit side first.
		var style astrologstyle.AspectStyle
		if a.NatalObject != (aspect.Object{}) && a.TransitObject != (aspect.Object{}) {
			style = styles[astrologstyle.AspectIdentity{First: a.NatalObject, Second: a.TransitObject, Kind: a.Kind}]
		}
		if mark.crossCell(a) {
			entries[i] = BoldCell(formatCrossAspectEntryCells(a, style, cfg, phase, cells, room, mark.Colour, dot))
			keep = i
		} else {
			entries[i] = formatCrossAspectEntryCells(a, style, cfg, phase, cells, room, "", dot)
		}
	}
	return layoutWaterfallGridKeeping(entries, keep, entryWidth(cells)+room, innerWidth, innerHeight)
}

// layoutWaterfallGrid arranges pre-formatted, already-power-sorted entries into
// the column-major waterfall grid: cols = (innerWidth+gutter)/(colWidth+gutter)
// (min 1), filled column-major so the highest-power entries fill column 0
// top-down. On overflow the lowest-power entries fall off (the grid holds only
// cols×rows). Shared by the single-chart and cross-aspect renderers.
func layoutWaterfallGrid(entries []string, colWidth, innerWidth, innerHeight int) string {
	return layoutWaterfallGridKeeping(entries, -1, colWidth, innerWidth, innerHeight)
}

// layoutWaterfallGridKeeping is layoutWaterfallGrid with one entry, keep (an
// index into entries, -1 for none), that must stay in view: when it would fall
// off the grid it takes the last visible cell, and the cells above it keep
// their power order (the specification). The entered
// row's cell is that entry: a list that holds the row's aspect shows it, bold,
// however weak its power ranks.
func layoutWaterfallGridKeeping(entries []string, keep, colWidth, innerWidth, innerHeight int) string {
	if len(entries) == 0 || innerWidth <= 0 || innerHeight <= 0 {
		return ""
	}
	cols := (innerWidth + aspectGutter) / (colWidth + aspectGutter)
	if cols < 1 {
		cols = 1
	}
	rows := innerHeight
	if capacity := cols * rows; keep >= capacity && keep < len(entries) {
		kept := append([]string(nil), entries[:capacity-1]...)
		entries = append(kept, entries[keep])
	}
	gutter := strings.Repeat(" ", aspectGutter)
	blank := strings.Repeat(" ", colWidth)

	lines := make([]string, 0, rows)
	for r := 0; r < rows; r++ {
		cells := make([]string, 0, cols)
		for c := 0; c < cols; c++ {
			idx := c*rows + r // column-major: highest power fills column 0 first
			if idx < len(entries) {
				cells = append(cells, entries[idx])
			} else {
				cells = append(cells, blank)
			}
		}
		lines = append(lines, strings.TrimRight(strings.Join(cells, gutter), " "))
	}
	return strings.Join(lines, "\n")
}

func FormatAspectEntryStyled(a aspect.Aspect, style astrologstyle.AspectStyle, cfg term.Style) string {
	return formatAspectEntryCells(a, style, cfg, 1, orbRoom(a.BObject, a.Orb), "", false)
}

// FormatAspectEntryDot is FormatAspectEntryStyled with the experiment that draws
// an aspect without a glyph as a dot.
func FormatAspectEntryDot(a aspect.Aspect, style astrologstyle.AspectStyle, cfg term.Style) string {
	return formatAspectEntryCells(a, style, cfg, 1, orbRoom(a.BObject, a.Orb), "", true)
}

// formatAspectEntryCells is the entry with each side's glyph in `cells`
// cells: 1 as always, 2 when a cusp 11 or 12 is in the list; room is the
// list's orbRoom. orb, when set and colours are on, draws the orb text
// `(d°mm'X)` in that SGR (the enter-colour experiment); "" draws it plain.
func formatAspectEntryCells(a aspect.Aspect, style astrologstyle.AspectStyle, cfg term.Style, cells, room int, orb astrologstyle.SGR, dot bool) string {
	deg := int(float64(a.Orb))
	min := int((float64(a.Orb) - float64(deg)) * 60)
	sep := " "
	if a.Orb >= 10 && orbRoom(a.BObject, a.Orb) == 0 {
		sep = ""
	}
	dir := "S"
	if a.Applying {
		dir = "A"
	}
	orbText := fmt.Sprintf("(%d°%02d'%s)", deg, min, dir)
	if cfg.UseColor() {
		orbText = RenderSGR(orb, orbText)
	}
	first, kind, second := aspectSideGlyph(a.AObject), aspectMark(a.Kind, dot), aspectSideGlyph(a.BObject)
	if cfg.UseColor() {
		first = RenderSGR(style.First, first)
		kind = RenderSGR(style.Kind, kind)
		second = RenderSGR(style.Second, second)
	}
	first, second = alignSides(first, second, cells)
	entry := aspectCell(first, kind, second, a.Kind, dot) + sep + orbText
	colWidth := entryWidth(cells) + room
	switch w := ansi.StringWidth(entry); {
	case w < colWidth:
		entry += strings.Repeat(" ", colWidth-w)
	case w > colWidth: // guard: never overflow the column / break alignment
		entry = term.CutRight(entry, colWidth, cfg)
	}
	return entry
}

func FormatCrossAspectEntryStyled(a lookup.OverlayAspect, style astrologstyle.AspectStyle, cfg term.Style) string {
	return formatCrossAspectEntryPhased(a, style, cfg, true)
}

// formatCrossAspectEntryPhased is the cell with its A/S suffix on (a transit
// pair) or off (a pair of two anchored charts: "(1°23')").
func formatCrossAspectEntryPhased(a lookup.OverlayAspect, style astrologstyle.AspectStyle, cfg term.Style, phase bool) string {
	return formatCrossAspectEntryCells(a, style, cfg, phase, 1, orbRoom(a.NatalObject, a.Orb), "", false)
}

// formatCrossAspectEntryCells is formatCrossAspectEntryPhased with each side's
// glyph in `cells` cells and the list's room (formatAspectEntryCells).
func formatCrossAspectEntryCells(a lookup.OverlayAspect, style astrologstyle.AspectStyle, cfg term.Style, phase bool, cells, room int, orb astrologstyle.SGR, dot bool) string {
	deg := int(float64(a.Orb))
	min := int((float64(a.Orb) - float64(deg)) * 60)
	sep := " "
	if a.Orb >= 10 && orbRoom(a.NatalObject, a.Orb) == 0 {
		sep = ""
	}
	dir := "S"
	if a.Applying {
		dir = "A"
	}
	if !phase {
		dir = ""
	}
	orbText := fmt.Sprintf("(%d°%02d'%s)", deg, min, dir)
	if cfg.UseColor() {
		orbText = RenderSGR(orb, orbText)
	}
	natal, kind, transit := aspectSideGlyph(a.NatalObject), aspectMark(a.Kind, dot), aspectSideGlyph(a.TransitObject)
	if cfg.UseColor() {
		// Style.First belongs to Astrolog's primary/natal body; Style.Second
		// to its secondary/transit body, wherever the cell prints them.
		natal = RenderSGR(style.First, natal)
		kind = RenderSGR(style.Kind, kind)
		transit = RenderSGR(style.Second, transit)
	}
	// The chart under the cursor (the transit side, the right of "② in ①")
	// reads first, as the tab names it and the Timeline's transit row prints
	// it, `trans Sun Tri natal Pollux` (operator, 2026-11-06: "make it
	// consistent"); each side keeps its own colour.
	transit, natal = alignSides(transit, natal, cells)
	entry := aspectCell(transit, kind, natal, a.Kind, dot) + sep + orbText
	colWidth := entryWidth(cells) + room
	switch w := ansi.StringWidth(entry); {
	case w < colWidth:
		entry += strings.Repeat(" ", colWidth-w)
	case w > colWidth:
		entry = term.CutRight(entry, colWidth, cfg)
	}
	return entry
}

// aspectDot stands in an aspect cell for an aspect with no glyph, when the
// experiment that shows a dot is on.
const aspectDot = "·"

// aspectMark is what the cell draws between the two sides: the aspect's glyph;
// for an aspect without one, its three letters, or the dot with the experiment on.
func aspectMark(k aspect.AspectKind, dot bool) string {
	if dot && k.Known() && !HasGlyph(k) {
		return aspectDot
	}
	return AspectGlyph(k)
}

// aspectCell joins the two sides and the mark. A glyph's cell is `a ⚹ b`. An
// aspect without a glyph, drawn by its letters, is `aSepb`: the sides and the
// three letters with no space, the same five cells, so no column depends on
// which aspects are shown (term.IsAspectLettersCell is the one form the
// glyph-room rule lets stand without a space after the first glyph).
func aspectCell(first, mark, second string, k aspect.AspectKind, dot bool) string {
	if k.Known() && !HasGlyph(k) && !dot {
		return first + mark + second
	}
	return first + " " + mark + " " + second
}

// aspectSideGlyph is one side of an aspect cell. A plain cusp point Astrolog
// names (not one of the four angles) shows its house number (the specification
// C2, operator 2026-10-11); any other object Sextile has no glyph for keeps
// the placeholder.
func aspectSideGlyph(o aspect.Object) string {
	if h, ok := aspect.CuspHouse(o); ok && !isAngleHouse(h) {
		return strconv.Itoa(h)
	}
	return BodyGlyph(o)
}

// orbRoom is 1 when an orb of 10° or more would sit flush on a right-hand glyph
// a terminal may draw wide (term.EndsWithRoomyGlyph, ♀ and ♂ among them): that
// entry keeps its space before "(", the glyph's room (glyph_room.go), and its
// list's columns are one cell wider. A 10° orb sits flush on any other glyph.
func orbRoom(right aspect.Object, orb aspect.Orb) int {
	if orb >= 10 && term.EndsWithRoomyGlyph(aspectSideGlyph(right)) {
		return 1
	}
	return 0
}

// sideCells is how many cells a side's glyph needs: two for cusp 11 or 12.
func sideCells(o aspect.Object) int {
	if n, err := strconv.Atoi(aspectSideGlyph(o)); err == nil && n >= 10 {
		return 2
	}
	return 1
}

// entryWidth is the width of one cell of the waterfall: each side's glyph
// grows by one cell when the list holds a two-digit cusp.
func entryWidth(cells int) int { return AspectColWidth + 2*(cells-1) }

// alignSides puts each side's glyph in `cells` cells: the left one
// right-aligned and the right one left-aligned, so the aspect symbol between
// them stays in one column ("the one-char glyphs stay centered with the
// aspects glyph", operator 2026-10-11).
func alignSides(left, right string, cells int) (string, string) {
	if pad := cells - ansi.StringWidth(left); pad > 0 {
		left = strings.Repeat(" ", pad) + left
	}
	if pad := cells - ansi.StringWidth(right); pad > 0 {
		right += strings.Repeat(" ", pad)
	}
	return left, right
}

// AspectRowsHeld is how many rows the waterfall needs to show every entry of
// the bag's own list (cross false) or its pair list (cross true) at innerWidth:
// the panel's scroll bar compares it with the rows the panel shows.
func AspectRowsHeld(bag AspectsBag, cross bool, innerWidth int) int {
	cells, room, n := 1, 0, 0
	if cross {
		n = len(bag.Cross)
		for _, a := range bag.Cross {
			cells = max(cells, sideCells(a.NatalObject), sideCells(a.TransitObject))
			room = max(room, orbRoom(a.NatalObject, a.Orb))
		}
	} else {
		n = len(bag.Rows)
		for _, a := range bag.Rows {
			cells = max(cells, sideCells(a.AObject), sideCells(a.BObject))
			room = max(room, orbRoom(a.BObject, a.Orb))
		}
	}
	if n == 0 || innerWidth <= 0 {
		return 0
	}
	cols := max((innerWidth+aspectGutter)/(entryWidth(cells)+room+aspectGutter), 1)
	return (n + cols - 1) / cols
}
