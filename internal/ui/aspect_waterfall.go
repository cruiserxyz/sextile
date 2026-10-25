package ui

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/internal/astrologstyle"
	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

const (
	aspectColWidth = 15
	// Columns abut directly: each entry is right-padded to aspectColWidth, so a
	// zero gutter still leaves the short entries' trailing pad as visual
	// separation while packing one more column into the same width.
	aspectGutter = 0
)

// renderAspectWaterfall lays out aspects as a dense, power-sorted, multi-column
// waterfall (ported from legacy gui/views/aspects_renderer.go): each entry is
// aspectColWidth cells; the column count is (innerWidth+gutter)/(colWidth+gutter)
// (min 1); cells fill column-major so the highest-power aspects fill the first
// column top-down. Narrow widths reduce the column count; on overflow the
// lowest-power aspects fall off (the grid only holds cols×rows entries).
func renderAspectWaterfall(aspects []aspect.Aspect, innerWidth, innerHeight int) string {
	return renderAspectWaterfallStyled(aspects, nil, innerWidth, innerHeight, Config{Color: ColorNever})
}

func renderAspectWaterfallStyled(aspects []aspect.Aspect, styles astrologstyle.Aspects, innerWidth, innerHeight int, cfg Config) string {
	return renderAspectWaterfallMarked(aspects, styles, innerWidth, innerHeight, cfg, enteredMark{})
}

// renderAspectWaterfallMarked is the waterfall with the entered row's cell, if
// the list holds it, drawn bold.
func renderAspectWaterfallMarked(aspects []aspect.Aspect, styles astrologstyle.Aspects, innerWidth, innerHeight int, cfg Config, mark enteredMark) string {
	if len(aspects) == 0 {
		return ""
	}
	sorted := append([]aspect.Aspect(nil), aspects...)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].Power > sorted[j].Power
	})
	cells := 1
	for _, a := range sorted {
		cells = max(cells, sideCells(a.A, a.ALabel), sideCells(a.B, a.BLabel))
	}
	entries := make([]string, len(sorted))
	keep := -1
	for i, a := range sorted {
		var style astrologstyle.AspectStyle
		if a.A != aspect.BodyUnknown && a.B != aspect.BodyUnknown {
			// A side with no typed identity is not a style key: every such
			// side would collide on BodyUnknown.
			style = styles[astrologstyle.AspectIdentity{First: a.A, Second: a.B, Kind: a.Kind}]
		}
		entries[i] = formatAspectEntryCells(a, style, cfg, cells)
		if mark.ownCell(a) {
			entries[i] = boldCell(entries[i])
			keep = i
		}
	}
	return layoutWaterfallGridKeeping(entries, keep, entryWidth(cells), innerWidth, innerHeight)
}

// renderCrossAspectWaterfall is the overlay analogue: transit→natal cross-aspects
// in the same power-sorted grid, each entry reading "<natal> <asp> <transit>" —
// Astrolog's own -r0/-rt list order (chart 1 / natal on the left, "with" the
// chart 2 / transit body on the right), so the cell reads like the raw report
// the Details panel shows. The Transit→Natal tab names the relationship.
func renderCrossAspectWaterfall(aspects []lookup.OverlayAspect, innerWidth, innerHeight int) string {
	return renderCrossAspectWaterfallStyled(aspects, nil, innerWidth, innerHeight, Config{Color: ColorNever})
}

func renderCrossAspectWaterfallStyled(aspects []lookup.OverlayAspect, styles astrologstyle.Aspects, innerWidth, innerHeight int, cfg Config) string {
	return renderCrossAspectWaterfallPair(aspects, styles, innerWidth, innerHeight, cfg, true)
}

// renderCrossAspectWaterfallPair is the cross waterfall with the phase switch:
// phase=true is today's transit pair (each cell ends A or S, Astrolog's verdict
// with the natal chart held still); phase=false is a pair of two anchored
// charts, whose Astrolog verdict is a synthetic one and is never displayed.
func renderCrossAspectWaterfallPair(aspects []lookup.OverlayAspect, styles astrologstyle.Aspects, innerWidth, innerHeight int, cfg Config, phase bool) string {
	return renderCrossAspectWaterfallMarked(aspects, styles, innerWidth, innerHeight, cfg, phase, enteredMark{})
}

// renderCrossAspectWaterfallMarked is the cross waterfall with the entered
// row's cell, if the list holds it, drawn bold.
func renderCrossAspectWaterfallMarked(aspects []lookup.OverlayAspect, styles astrologstyle.Aspects, innerWidth, innerHeight int, cfg Config, phase bool, mark enteredMark) string {
	if len(aspects) == 0 {
		return ""
	}
	sorted := append([]lookup.OverlayAspect(nil), aspects...)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].Power > sorted[j].Power
	})
	cells := 1
	for _, a := range sorted {
		cells = max(cells, sideCells(a.Natal, a.NatalLabel), sideCells(a.Transit, a.TransitLabel))
	}
	entries := make([]string, len(sorted))
	keep := -1
	for i, a := range sorted {
		// Astrolog emits primary/natal first and secondary/transit second; the
		// style key and the cell both keep that order.
		var style astrologstyle.AspectStyle
		if a.Natal != aspect.BodyUnknown && a.Transit != aspect.BodyUnknown {
			style = styles[astrologstyle.AspectIdentity{First: a.Natal, Second: a.Transit, Kind: a.Kind}]
		}
		entries[i] = formatCrossAspectEntryCells(a, style, cfg, phase, cells)
		if mark.crossCell(a) {
			entries[i] = boldCell(entries[i])
			keep = i
		}
	}
	return layoutWaterfallGridKeeping(entries, keep, entryWidth(cells), innerWidth, innerHeight)
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

// formatAspectEntry renders one aspectColWidth-cell entry:
//
//	"<P1> <asp> <P2> (<D°MM'><A|S>)"
//
// The A/S suffix is the applying/separating flag (D2). For orb >= 10° the space
// before "(" is dropped to keep the entry within the column width (legacy parity).
func formatAspectEntry(a aspect.Aspect) string {
	return formatAspectEntryStyled(a, astrologstyle.AspectStyle{}, Config{Color: ColorNever})
}

func formatAspectEntryStyled(a aspect.Aspect, style astrologstyle.AspectStyle, cfg Config) string {
	return formatAspectEntryCells(a, style, cfg, 1)
}

// formatAspectEntryCells is the entry with each side's glyph in `cells`
// cells: 1 as always, 2 when a cusp 11 or 12 is in the list.
func formatAspectEntryCells(a aspect.Aspect, style astrologstyle.AspectStyle, cfg Config, cells int) string {
	deg := int(float64(a.Orb))
	min := int((float64(a.Orb) - float64(deg)) * 60)
	sep := " "
	if a.Orb >= 10 {
		sep = ""
	}
	dir := "S"
	if a.Applying {
		dir = "A"
	}
	first, kind, second := aspectSideGlyph(a.A, a.ALabel), aspectGlyph(a.Kind), aspectSideGlyph(a.B, a.BLabel)
	if cfg.useColor() {
		first = renderSGR(style.First, first)
		kind = renderSGR(style.Kind, kind)
		second = renderSGR(style.Second, second)
	}
	first, second = alignSides(first, second, cells)
	entry := fmt.Sprintf("%s %s %s%s(%d°%02d'%s)", first, kind, second, sep, deg, min, dir)
	colWidth := entryWidth(cells)
	switch w := ansi.StringWidth(entry); {
	case w < colWidth:
		entry += strings.Repeat(" ", colWidth-w)
	case w > colWidth: // guard: never overflow the column / break alignment
		entry = cutRight(entry, colWidth, cfg)
	}
	return entry
}

// formatCrossAspectEntry renders one overlay cross-aspect cell:
//
//	"<natal> <asp> <transit> (<D°MM'><A|S>)"
//
// natal body first, transit second — Astrolog's -r0/-rt output order, so the
// cell matches the raw comparison report line for line. Same column
// width + A/S suffix + wide-orb spacing rule as formatAspectEntry.
func formatCrossAspectEntry(a lookup.OverlayAspect) string {
	return formatCrossAspectEntryStyled(a, astrologstyle.AspectStyle{}, Config{Color: ColorNever})
}

func formatCrossAspectEntryStyled(a lookup.OverlayAspect, style astrologstyle.AspectStyle, cfg Config) string {
	return formatCrossAspectEntryPhased(a, style, cfg, true)
}

// formatCrossAspectEntryPhased is the cell with its A/S suffix on (a transit
// pair) or off (a pair of two anchored charts: "(1°23')").
func formatCrossAspectEntryPhased(a lookup.OverlayAspect, style astrologstyle.AspectStyle, cfg Config, phase bool) string {
	return formatCrossAspectEntryCells(a, style, cfg, phase, 1)
}

// formatCrossAspectEntryCells is formatCrossAspectEntryPhased with each side's
// glyph in `cells` cells (formatAspectEntryCells).
func formatCrossAspectEntryCells(a lookup.OverlayAspect, style astrologstyle.AspectStyle, cfg Config, phase bool, cells int) string {
	deg := int(float64(a.Orb))
	min := int((float64(a.Orb) - float64(deg)) * 60)
	sep := " "
	if a.Orb >= 10 {
		sep = ""
	}
	dir := "S"
	if a.Applying {
		dir = "A"
	}
	if !phase {
		dir = ""
	}
	natal, kind, transit := aspectSideGlyph(a.Natal, a.NatalLabel), aspectGlyph(a.Kind), aspectSideGlyph(a.Transit, a.TransitLabel)
	if cfg.useColor() {
		// Style.First belongs to Astrolog's primary/natal body; Style.Second
		// to its secondary/transit body — the same order the cell prints.
		natal = renderSGR(style.First, natal)
		kind = renderSGR(style.Kind, kind)
		transit = renderSGR(style.Second, transit)
	}
	natal, transit = alignSides(natal, transit, cells)
	entry := fmt.Sprintf("%s %s %s%s(%d°%02d'%s)", natal, kind, transit, sep, deg, min, dir)
	colWidth := entryWidth(cells)
	switch w := ansi.StringWidth(entry); {
	case w < colWidth:
		entry += strings.Repeat(" ", colWidth-w)
	case w > colWidth:
		entry = cutRight(entry, colWidth, cfg)
	}
	return entry
}

// reCuspLabel reads a house cusp's number from the name Astrolog prints for it
// in an aspect list: "2nd Cusp", "12th Cusp", or the left column's
// seven-character cut "12th Cu".
var reCuspLabel = regexp.MustCompile(`^(1[0-2]|[1-9])(?:st|nd|rd|th) Cu`)

// aspectSideGlyph is one side of an aspect cell. A cusp Astrolog names, which
// the Body enum cannot, shows its house number (the specification C2,
// operator 2026-10-11); any other unnamed body keeps the placeholder.
func aspectSideGlyph(b aspect.Body, label string) string {
	if b == aspect.BodyUnknown {
		if m := reCuspLabel.FindStringSubmatch(label); m != nil {
			return m[1]
		}
	}
	return bodyGlyph(b)
}

// sideCells is how many cells a side's glyph needs: two for cusp 11 or 12.
func sideCells(b aspect.Body, label string) int {
	if n, err := strconv.Atoi(aspectSideGlyph(b, label)); err == nil && n >= 10 {
		return 2
	}
	return 1
}

// entryWidth is the width of one cell of the waterfall: each side's glyph
// grows by one cell when the list holds a two-digit cusp.
func entryWidth(cells int) int { return aspectColWidth + 2*(cells-1) }

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
