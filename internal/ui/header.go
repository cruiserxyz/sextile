package ui

import (
	"strings"
	"time"

	// Embed the IANA tz database so the zoned slot line (and the first-frame
	// golden invariant that depends on it) renders identically regardless of
	// host tzdata. Sextile is timezone-load-bearing; determinism here is cheap.
	_ "time/tzdata"

	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/internal/store"
)

// chartSlotsBody renders the [1] Chart panel body — one line per slot in the
// workspace. The cursor row carries the shared cursor-row treatment while
// Chart owns the keyboard (renderRowState); with colours off it also gets the
// cursor column. Persistent slot selection remains visible independently. Pre-resolution (slots == nil)
// falls back to the clock+place line so the body is never empty.
//
// Each anchored row opens with its OWN chart's name (Slot.Chart.Name), in a
// column as wide as the widest shown name (capped at nameColumnMax), so every
// row's moment — and, by padding the zone, every row's place — starts in the
// same cell. The floating row shows no name, only the column's blank width.
// With no name to show, rows render exactly as they did before names existed
// (the specification item B).
//
// It returns one row per slot, however many there are. When the workspace
// holds more slots than the panel has rows, compose draws the window of them
// that holds the cursor (chartWindow); the column widths are read from every
// slot here, so the columns do not shift as the window moves.
func chartSlotsBody(slots *store.Slots, cursorIdx int, panelFocused bool, now time.Time, place location.PlaceInfo, lens lookup.Lens, width int, cfg Config) string {
	return chartSlotsBodyTagged(slots, cursorIdx, panelFocused, now, place, lens, width, cfg, nil)
}

// chartSlotsBodyTagged is chartSlotsBody with a per-row tag: each row carries
// the house system and zodiac ITS chart is cast in (the specification C5,
// C7), not one shared lens's. A nil tag gives every row the lens's own.
func chartSlotsBodyTagged(slots *store.Slots, cursorIdx int, panelFocused bool, now time.Time, place location.PlaceInfo, lens lookup.Lens, width int, cfg Config, tag func(*store.Slot) string) string {
	return chartSlotsBodyLayout(slots, cursorIdx, panelFocused, now, place, lens, width, cfg, tag, "", false)
}

// chartSlotsBodyLayout is chartSlotsBodyTagged told which layout the screen is
// in: oneChart is the one-chart layout, where Bodies shows one column.
// floatTitle is the floating row's title (`~ Now`, `~`), set in the name column
// the anchored rows use; empty gives the floating row no name, as before.
func chartSlotsBodyLayout(slots *store.Slots, cursorIdx int, panelFocused bool, now time.Time, place location.PlaceInfo, lens lookup.Lens, width int, cfg Config, tag func(*store.Slot) string, floatTitle string, oneChart bool) string {
	if width <= 0 {
		return ""
	}
	if slots == nil {
		prefix := rowPrefix(panelFocused, true, false, false, cfg)
		line := tagSlotLine(now, place, lens.HouseAbbrev()+lens.ZodiacAbbrev(), width-ansi.StringWidth(prefix), cfg)
		if line == "" {
			return ""
		}
		return renderRow(prefix+line, width, cursorRowState(panelFocused, true), cfg)
	}
	all := slots.All()
	if len(all) == 0 {
		prefix := rowPrefix(panelFocused, true, false, false, cfg)
		line := tagSlotLine(now, place, lens.HouseAbbrev()+lens.ZodiacAbbrev(), width-ansi.StringWidth(prefix), cfg)
		if line == "" {
			return ""
		}
		return renderRow(prefix+line, width, cursorRowState(panelFocused, true), cfg)
	}
	set := selectionOf(slots)
	selectionColumn := len(set) > 0
	names := make([]string, len(all))
	nameW, zoneW := 0, 0
	// The one-chart layout fits each name into what is left of the row after
	// the moment, a space and the tag (the specification rule 2),
	// measured from what is rendered, never assumed.
	nameCap := nameColumnMax
	if oneChart {
		momentW, tagW, rowW := 0, 0, width
		for i, s := range all {
			if !s.Location.HasTimezone() {
				continue
			}
			momentW = max(momentW, ansi.StringWidth(slotMoment(s.Time, s.Location)))
			tagW = max(tagW, ansi.StringWidth(rowTagOf(s, lens, tag)))
			rowW = min(rowW, width-ansi.StringWidth(chartRowPrefix(panelFocused, i == cursorIdx, numeralIn(set, s), selectionColumn, cfg)))
		}
		nameCap = min(nameColumnMax, rowW-nameGutter-momentW-1-tagW)
	}
	for i, s := range all {
		if !s.Location.HasTimezone() {
			continue
		}
		if s.Floating {
			if floatTitle != "" {
				names[i] = abbreviateNameTo(floatTitle, max(nameCap, 0), cfg)
				nameW = max(nameW, ansi.StringWidth(names[i]))
			}
			continue
		}
		names[i] = abbreviateNameTo(s.Chart.Name, max(nameCap, 0), cfg)
		nameW = max(nameW, ansi.StringWidth(names[i]))
	}
	if oneChart {
		nameW = fitFloatTitle(all, names, nameW, width, lens, tag, panelFocused, cursorIdx, set, selectionColumn, cfg)
	}
	nameW = min(nameW, max(nameCap, 0))
	if nameW > 0 || oneChart {
		for _, s := range all {
			if s.Location.HasTimezone() {
				zoneW = max(zoneW, ansi.StringWidth(slotZone(s.Time, s.Location)))
			}
		}
	}
	lines := make([]string, len(all))
	for i, s := range all {
		// Honest time semantics: every slot displays its own Time field.
		// For the anchored slot this is the natal moment; for the floating
		// slot it is the time the chart payload was computed for (set at
		// NewSlots/refreshSlotsForPlace construction). Reading the live
		// clock here would advertise a wall-clock time the bodies/aspects
		// panels have not been recomputed against — a trust-breaking lie
		// during consults. Time-stepping advances slot.Time and
		// triggers recompute so the two stay in step.
		prefix := chartRowPrefix(panelFocused, i == cursorIdx, numeralIn(set, s), selectionColumn, cfg)
		rowLens := lens
		var line string
		if oneChart {
			line = oneChartLine(names[i], nameW, zoneW, s.Time, s.Location, rowTagOf(s, lens, tag), width-ansi.StringWidth(prefix), cfg)
		} else if tag != nil {
			t := tag(s)
			if nameW == 0 {
				line = tagSlotLine(s.Time, s.Location, t, width-ansi.StringWidth(prefix), cfg)
			} else {
				line = tagNamedSlotLine(names[i], nameW, zoneW, s.Time, s.Location, t, width-ansi.StringWidth(prefix), cfg)
			}
		} else if nameW == 0 {
			line = tagSlotLine(s.Time, s.Location, rowLens.HouseAbbrev()+rowLens.ZodiacAbbrev(), width-ansi.StringWidth(prefix), cfg)
		} else {
			line = tagNamedSlotLine(names[i], nameW, zoneW, s.Time, s.Location, rowLens.HouseAbbrev()+rowLens.ZodiacAbbrev(), width-ansi.StringWidth(prefix), cfg)
		}
		lines[i] = renderRow(prefix+line, width, cursorRowState(panelFocused, i == cursorIdx), cfg)
	}
	return strings.Join(lines, "\n")
}

// floatTitleShorts are the attached floating row's shorter titles, tried in
// order when `~ Now` would cost the row its place: the place keeps its column
// and the title gives way (operator 2026-10-24). Never `~` alone, which says
// the chart has been moved off the clock; when none fits the title stays whole.
var floatTitleShorts = []string{"~Now", "Now"}

// fitFloatTitle shortens the floating row's title to floatTitleShort when the
// title alone is what would push the floating row's place out of the one-chart
// layout, and returns the name column's width after it. When an anchored name
// already takes that room the title stays: shortening it would save nothing.
func fitFloatTitle(all []*store.Slot, names []string, nameW, width int, lens lookup.Lens, tag func(*store.Slot) string, panelFocused bool, cursorIdx int, set []*store.Slot, selectionColumn bool, cfg Config) int {
	fi, otherW := -1, 0
	for i, s := range all {
		if s.Floating {
			fi = i
		} else {
			otherW = max(otherW, ansi.StringWidth(names[i]))
		}
	}
	if fi < 0 || names[fi] != "~ Now" || !all[fi].Location.HasTimezone() {
		return nameW
	}
	room := floatTitleRoom(all[fi], width, lens, tag, panelFocused, fi == cursorIdx, numeralIn(set, all[fi]), selectionColumn, cfg)
	if ansi.StringWidth(names[fi]) <= room || otherW > room {
		return nameW
	}
	for _, short := range floatTitleShorts {
		if ansi.StringWidth(short) <= room {
			names[fi] = short
			return max(otherW, ansi.StringWidth(short))
		}
	}
	return nameW
}

// floatTitleRoom is how many cells the floating row's title may take in the
// one-chart layout and still leave the row its moment, zone, place and tag
// whole (oneChartLine drops the place first when room runs out).
func floatTitleRoom(s *store.Slot, width int, lens lookup.Lens, tag func(*store.Slot) string, panelFocused, cursor bool, numeral int, selectionColumn bool, cfg Config) int {
	rowW := width - ansi.StringWidth(chartRowPrefix(panelFocused, cursor, numeral, selectionColumn, cfg))
	used := ansi.StringWidth(slotMoment(s.Time, s.Location)) + 1 + ansi.StringWidth(slotZone(s.Time, s.Location)) +
		1 + ansi.StringWidth(s.Location.Display()) + 1 + ansi.StringWidth(rowTagOf(s, lens, tag))
	return rowW - nameGutter - used
}

// chartWindow is the contiguous run of slot rows the Chart panel draws when
// it has room for rows of them: all of them while they fit, otherwise the
// window of that height that holds the cursor. top is where the window stood
// before: it stays put while the cursor is inside it and slides only as far as
// the cursor needs, in either direction. It returns the first row and how many
// rows show, and never a window without the cursor.
func chartWindow(total, cursor, rows, top int) (start, visible int) {
	visible = min(total, max(rows, 1))
	top = min(max(top, 0), max(total-visible, 0))
	if cursor < top {
		top = cursor
	}
	if cursor >= top+visible {
		top = cursor - visible + 1
	}
	return max(top, 0), visible
}

// chartSlotLine renders one slot's chart-context line, fitted to exactly width
// cells:
//
//	"<moment> <TZ> <location…>      <WhTr>"
//
// where <moment> is the held clock form (clockMoment), e.g. "Mon Oct  5, 2026  9:05".
//
// The condensed house·zodiac tag (e.g. "WhTr") is pinned to the far right; the
// location is truncated with an ellipsis to fill the gap between the timezone
// and the tag. Returns "" when the place carries no timezone (pre-location
// startup) so the Chart panel body stays empty.
//
// Placement (Slice A/A.1, D15): the slot line is the Chart panel BODY — which
// exists at every width — not the Details panel (absent at 80×24). The panel
// title stays title-only ("[1] Chart"); the location lives here in the line.
func chartSlotLine(now time.Time, place location.PlaceInfo, lens lookup.Lens, width int, cfg Config) string {
	return tagSlotLine(now, place, lens.HouseAbbrev()+lens.ZodiacAbbrev(), width, cfg)
}

// tagSlotLine is chartSlotLine with the row's tag given.
func tagSlotLine(now time.Time, place location.PlaceInfo, right string, width int, cfg Config) string {
	if !place.HasTimezone() || width <= 0 {
		return ""
	}
	left := clockMoment(zonedIn(now, place)) + " " + slotZone(now, place)
	return layoutSlotLine(left, place.Display(), right, width, cfg)
}

// namedSlotLine is chartSlotLine behind a name column: the name padded to
// nameW, a two-space gutter, then the moment with its zone right-padded to
// zoneW so the places of all rows line up. A row whose place has no timezone
// renders "" exactly as chartSlotLine does, name included.
func namedSlotLine(name string, nameW, zoneW int, now time.Time, place location.PlaceInfo, lens lookup.Lens, width int, cfg Config) string {
	return tagNamedSlotLine(name, nameW, zoneW, now, place, lens.HouseAbbrev()+lens.ZodiacAbbrev(), width, cfg)
}

// tagNamedSlotLine is namedSlotLine with the row's tag given.
func tagNamedSlotLine(name string, nameW, zoneW int, now time.Time, place location.PlaceInfo, right string, width int, cfg Config) string {
	if !place.HasTimezone() || width <= 0 {
		return ""
	}
	cell := name + strings.Repeat(" ", nameW-ansi.StringWidth(name)) + "  "
	if ansi.StringWidth(cell) >= width {
		// No room for anything past the name: the row still fits its width.
		return cutRight(cell, width, cfg)
	}
	zone := slotZone(now, place)
	left := slotMoment(now, place) + " " + zone + strings.Repeat(" ", max(0, zoneW-ansi.StringWidth(zone)))
	return cell + layoutSlotLine(left, place.Display(), right, width-ansi.StringWidth(cell), cfg)
}

// nameGutter is the two spaces between the name column and the moment.
const nameGutter = 2

// rowTagOf is the row's tag: the given function's, else the lens's own.
func rowTagOf(s *store.Slot, lens lookup.Lens, tag func(*store.Slot) string) string {
	if tag != nil {
		return tag(s)
	}
	return lens.HouseAbbrev() + lens.ZodiacAbbrev()
}

// oneChartLine is a chart row in the one-chart layout: the name column (blank
// when no chart carries a name) and its gutter, the moment whole, then the
// zone and the place only while each fits whole, then the tag pinned right.
// Nothing after the name is ever cut: what does not fit is dropped, and the
// moment is the one thing that always stays (the specification
// rules 1-3). A row whose place has no timezone renders "".
func oneChartLine(name string, nameW, zoneW int, now time.Time, place location.PlaceInfo, right string, width int, cfg Config) string {
	if !place.HasTimezone() || width <= 0 {
		return ""
	}
	cell := ""
	if nameW > 0 {
		cell = name + strings.Repeat(" ", nameW-ansi.StringWidth(name)) + strings.Repeat(" ", nameGutter)
	}
	moment := slotMoment(now, place)
	rw := ansi.StringWidth(right)
	room := width - ansi.StringWidth(cell) - ansi.StringWidth(moment) - 1 - rw
	if room < 0 {
		// Not even the moment and the tag: the existing clamp, which keeps the
		// tag and clips the left (only at widths the layout never offers).
		return clampLine(cell+moment, right, width, cfg)
	}
	left := cell + moment
	zone := slotZone(now, place)
	if zw := max(zoneW, ansi.StringWidth(zone)); room >= 1+zw {
		left += " " + zone + strings.Repeat(" ", zw-ansi.StringWidth(zone))
		room -= 1 + zw
		if loc := place.Display(); room >= 1+ansi.StringWidth(loc) {
			left += " " + loc
		}
	}
	return left + strings.Repeat(" ", width-ansi.StringWidth(left)-rw) + right
}

// slotMoment and slotZone split chartSlotLine's left field so the zone can be
// padded on its own; together they are exactly its `<moment> <zone>` (clockMoment).
func slotMoment(now time.Time, place location.PlaceInfo) string {
	return clockMoment(zonedIn(now, place))
}

func slotZone(now time.Time, place location.PlaceInfo) string {
	local := zonedIn(now, place)
	// Inside the 1582 window the call carries no zone, and the row says so.
	if lookup.StampMoment(local, local.Location()).UT {
		return "UT"
	}
	return local.Format("MST")
}

func zonedIn(now time.Time, place location.PlaceInfo) time.Time {
	if loc, err := time.LoadLocation(place.TZ); err == nil {
		return now.In(loc)
	}
	return now
}

// nameColumnMax caps the name column, in cells, at every panel width.
const nameColumnMax = 20

// nameParticles are the lowercase forms of the surname particles abbreviation
// step (a) turns into initials. Matched case-insensitively.
var nameParticles = map[string]bool{
	"van": true, "von": true, "der": true, "den": true, "de": true, "del": true,
	"della": true, "di": true, "da": true, "du": true, "la": true, "le": true,
	"ter": true, "te": true, "ten": true, "zu": true, "dos": true, "das": true,
	"do": true,
}

// abbreviateName fits a chart name into nameColumnMax cells. Whitespace runs
// collapse to single spaces first. A name that fits is returned as is. A
// one-token name is cut to the cap ending in "…". Otherwise, stopping at the
// first result that fits: (a) the particles directly before the surname (the
// last token), with at least one token before them, become their initials —
// "van der" → "v.d.", case kept; (b) the surname becomes its initial; (c) the
// result of (b) is cut to the cap ending in "…". Widths are display cells, and
// a cut never splits a rune.
func abbreviateName(name string, cfg Config) string {
	return abbreviateNameTo(name, nameColumnMax, cfg)
}

// abbreviateNameTo is abbreviateName fitted into w cells instead of the cap:
// the same steps, only the width they fit into changes.
func abbreviateNameTo(name string, w int, cfg Config) string {
	tokens := strings.Fields(name)
	joined := strings.Join(tokens, " ")
	fits := func(s string) bool { return ansi.StringWidth(s) <= w }
	cut := func(s string) string { return cutRight(s, w, cfg) }
	if fits(joined) {
		return joined
	}
	if len(tokens) == 1 {
		return cut(joined)
	}
	last := len(tokens) - 1
	start := last
	for start-1 >= 1 && nameParticles[strings.ToLower(tokens[start-1])] {
		start--
	}
	if start < last {
		var initials strings.Builder
		for _, p := range tokens[start:last] {
			initials.WriteString(initial(p))
		}
		tokens = append(append(append([]string{}, tokens[:start]...), initials.String()), tokens[last])
		last = len(tokens) - 1
		if s := strings.Join(tokens, " "); fits(s) {
			return s
		}
	}
	tokens[last] = initial(tokens[last])
	s := strings.Join(tokens, " ")
	if fits(s) {
		return s
	}
	return cut(s)
}

// initial is a token's first rune, in its own case, followed by ".".
func initial(token string) string {
	r := []rune(token)
	return string(r[0]) + "."
}

// layoutSlotLine packs "<left> <loc> … <right>" into exactly width DISPLAY cells:
// right is pinned to the far edge, loc is cut (text.go's mark)
// to fill the gap, keeping at least one space on each side of loc. If width can't
// hold even left+right, the left half is clipped. Widths are measured with
// ansi.StringWidth so the line matches renderTitledBox's padBody exactly even for
// wide (e.g. CJK) location names.
func layoutSlotLine(left, loc, right string, width int, cfg Config) string {
	if width <= 0 {
		return ""
	}
	lw := ansi.StringWidth(left)
	rw := ansi.StringWidth(right)
	// reserve one space after left and one before right.
	avail := width - lw - rw - 2
	if avail < 1 {
		return clampLine(left, right, width, cfg)
	}
	loc = cutRight(loc, avail, cfg)
	pad := width - lw - 1 - ansi.StringWidth(loc) - rw
	if pad < 1 {
		pad = 1
	}
	return left + " " + loc + strings.Repeat(" ", pad) + right
}

// clampLine renders "<left><pad><right>" within width when there is no room for
// a middle field, clipping left (then right) if even that overflows.
func clampLine(left, right string, width int, cfg Config) string {
	rw := ansi.StringWidth(right)
	if width <= rw {
		return cutRight(right, width, cfg)
	}
	lw := ansi.StringWidth(left)
	if lw+rw > width {
		left = cutRight(left, width-rw, cfg)
		lw = ansi.StringWidth(left)
	}
	return left + strings.Repeat(" ", width-lw-rw) + right
}
