package ui

import (
	"github.com/cruiserxyz/sextile/internal/ui/menu"
	"github.com/cruiserxyz/sextile/internal/ui/term"
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

// chartSlotsBody renders the [2] Chart panel body — one line per slot in the
// workspace, as one table (chart_table.go). The cursor row carries the shared
// cursor-row treatment while Chart owns the keyboard (menu.RenderRowState);
// with colours off it also gets the cursor column. Persistent slot selection
// remains visible independently. Pre-resolution (slots == nil) falls back to
// the clock and place of now, so the body is never empty.
//
// Each anchored row opens with its OWN chart's name (Slot.Chart.Name). The
// floating row shows floatTitle in that column, or nothing when it is empty.
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
	return chartSlotsBodyLayout(slots, cursorIdx, panelFocused, now, place, lens, width, cfg, tag, "", nil)
}

// chartSlotsBodyLayout is chartSlotsBodyTagged with the floating row's title
// (`~ Now`, `~`; empty gives it no name) and the atlas witness of the places.
func chartSlotsBodyLayout(slots *store.Slots, cursorIdx int, panelFocused bool, now time.Time, place location.PlaceInfo, lens lookup.Lens, width int, cfg Config, tag func(*store.Slot) string, floatTitle string, witness placeWitness) string {
	if width <= 0 {
		return ""
	}
	var all []*store.Slot
	if slots != nil {
		all = slots.All()
	}
	if len(all) == 0 {
		prefix := menu.RowPrefix(panelFocused, true, false, false, cfg)
		rows := []tableRow{{prefix: prefix, cells: placeCells(now, place, witness), tag: lens.HouseAbbrev() + lens.ZodiacAbbrev()}}
		line := renderTable(rows, width, cfg)[0]
		if line == "" {
			return ""
		}
		return menu.RenderRow(line, width, menu.CursorRowState(panelFocused, true), cfg)
	}
	set := selectionOf(slots)
	selectionColumn := len(set) > 0
	// Honest time semantics: every slot displays its own Time field. For the
	// anchored slot this is the natal moment; for the floating slot it is the
	// time the chart payload was computed for. Reading the live clock here
	// would advertise a wall-clock time the bodies and aspects panels have not
	// been recomputed against. Time-stepping advances slot.Time and triggers
	// a recompute, so the two stay in step.
	rows := make([]tableRow, len(all))
	for i, s := range all {
		name := s.Chart.Name
		if s.Floating {
			name = floatTitle
		}
		rows[i] = tableRow{
			prefix: menu.ChartRowPrefix(panelFocused, i == cursorIdx, numeralMark(numeralIn(set, s), cfg), selectionColumn, cfg),
			name:   name,
			float:  s.Floating,
			cells:  slotCells(s, witness),
			tag:    rowTagOf(s, lens, tag),
		}
	}
	lines := renderTable(rows, width, cfg)
	for i := range lines {
		lines[i] = menu.RenderRow(lines[i], width, menu.CursorRowState(panelFocused, i == cursorIdx), cfg)
	}
	return strings.Join(lines, "\n")
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

// nameGutter is the two spaces between the name column and the date.
const nameGutter = 2

// rowTagOf is the row's tag: the given function's, else the lens's own.
func rowTagOf(s *store.Slot, lens lookup.Lens, tag func(*store.Slot) string) string {
	if tag != nil {
		return tag(s)
	}
	return lens.HouseAbbrev() + lens.ZodiacAbbrev()
}

// zonedIn is now on the place's wall clock (lookup.PlaceZone): its zone, or
// before the zone's standard time the local mean time of its longitude.
func zonedIn(now time.Time, place location.PlaceInfo) time.Time {
	if loc, err := lookup.PlaceZone(now, place); err == nil {
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
	cut := func(s string) string { return term.CutRight(s, w, cfg) }
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
