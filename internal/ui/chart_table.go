package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/internal/store"
	"github.com/cruiserxyz/sextile/internal/ui/term"
)

// The Chart panel draws its rows as one table: name, date, calendar mark,
// clock, zone, place and tag, each column as wide as its widest cell in every
// row. When the panel is too narrow the table narrows as a whole, in one
// order (narrowTable), so a column starts at the same cell in every row at
// every width, and no width shows a clock without its zone.

// rowCells is what a chart row shows of its moment, cell by cell. ok is false
// for a row with no moment to show.
type rowCells struct {
	ok      bool
	weekday string // "Thu"
	date    string // "Dec 27, 1571", or "27 Dec 1571" in the day-first order
	julian  bool   // the date is Julian: the calendar column reads "Jul"
	clock   string // "14:37:00", or " 2:37:00pm" in the 12-hour clock
	zone    string // "+1", "+2 DT", "+5:30", "LMT"
	// The place: its words, its coordinates in words, and whether an atlas
	// city near the coordinates carries the words.
	words, coords string
	confirmed     bool
}

// placeWitness says whether the atlas holds a city with the place words near
// the coordinates (location.ConfirmsNear). nil, as with no atlas, confirms
// nothing, so every place with words shows its coordinates too.
type placeWitness func(words string, lon, lat float64) bool

// placeCells is the row of a time at an atlas place: Now, an atlas pick, a chart
// whose file is not read yet. An atlas place is its own witness. A place with
// no zone (a chart file's, until its file is read) shows its instant in UT.
func placeCells(now time.Time, place location.PlaceInfo, witness placeWitness) rowCells {
	if now.IsZero() || place.Name == "" {
		return rowCells{}
	}
	coords := lookup.CoordinateWords(place.Lon, place.Lat)
	if !place.HasTimezone() {
		c := stampCells(lookup.StampMoment(now.UTC(), time.UTC), lookup.HeldClock(), now.UTC().Second())
		c.zone, c.words, c.coords = "+0", place.Name, coords
		c.confirmed = witness != nil && witness(place.Name, place.Lon, place.Lat)
		return c
	}
	local := zonedIn(now, place)
	stamp := lookup.StampMoment(local, local.Location())
	name, off := local.Zone()
	var c rowCells
	switch {
	case stamp.UT:
		// Inside the 1582 window the call carries no zone: the clock is UT.
		c = stampCells(stamp, lookup.HeldClock(), now.UTC().Second())
		c.zone = "+0"
	case name == "LMT":
		c = stampCells(stamp, lookup.HeldClock(), local.Second())
		c.zone = "LMT"
	default:
		c = stampCells(stamp, lookup.HeldClock(), local.Second())
		c.zone = lookup.OffsetCell(off)
		if lookup.DaylightHour(local) {
			c.zone += " DT"
		}
	}
	c.words, c.coords, c.confirmed = place.Display(), coords, true
	return c
}

// slotCells is a slot's row. A chart read from its file shows its moment as
// Astrolog read the file (store.ChartClock): its own clock, its zone and its
// own place. Every other slot shows its Time at its place.
func slotCells(s *store.Slot, witness placeWitness) rowCells {
	if s == nil {
		return rowCells{}
	}
	if k := s.Chart.Clock; !s.Floating && !k.IsZero() {
		c := stampCells(lookup.StampOfClock(k), lookup.HeldClock(), k.Second)
		c.zone, c.words, c.coords = k.Zone, k.Place, k.Coords
		c.confirmed = k.Place != "" && witness != nil && witness(k.Place, k.Lon, k.Lat)
		return c
	}
	return placeCells(s.Time, s.Location, witness)
}

// stampCells spells a stamp's date and clock in the clock form p. Every clock
// shows its seconds.
func stampCells(s lookup.MomentStamp, p lookup.ClockPref, second int) rowCells {
	hour, mark := s.Hour, ""
	if p.Hours12 {
		mark = "am"
		if hour >= 12 {
			mark = "pm"
		}
		hour %= 12
		if hour == 0 {
			hour = 12
		}
	}
	mon := time.Month(s.Month).String()[:3]
	date := fmt.Sprintf("%s %2d, %d", mon, s.Day, s.Year)
	if p.DayFirst {
		date = fmt.Sprintf("%2d %s %d", s.Day, mon, s.Year)
	}
	return rowCells{
		ok:      true,
		weekday: s.Weekday.String()[:3],
		date:    date,
		julian:  s.Julian,
		clock:   fmt.Sprintf("%2d:%02d:%02d%s", hour, s.Minute, second, mark),
	}
}

// placeText is the row's place cell fitted into w cells. Words an atlas city
// near the coordinates confirms stand alone, cut with "…" when long. A place
// with no words is its coordinates. Words no near city carries follow the
// coordinates in brackets, cut inside them to three letters at the least,
// or go when even that does not fit. The coordinates are never cut.
func placeText(c rowCells, w int, cfg Config) string {
	switch {
	case c.words != "" && c.confirmed:
		return term.CutRight(c.words, w, cfg)
	case c.words == "":
		return c.coords
	}
	full := c.coords + " (" + c.words + ")"
	if ansi.StringWidth(full) <= w {
		return full
	}
	room := w - ansi.StringWidth(c.coords) - 3
	if room < 4 {
		return c.coords
	}
	return c.coords + " (" + term.CutRight(c.words, room, cfg) + ")"
}

// placeWidth is the place cell's whole width.
func placeWidth(c rowCells) int {
	return ansi.StringWidth(placeText(c, 1<<20, Config{}))
}

// placeFloor is the narrowest the place cell may be in the last cut before the
// place goes: its coordinates whole, or a few letters of confirmed words.
func placeFloor(c rowCells) int {
	if c.words != "" && c.confirmed {
		return min(ansi.StringWidth(c.words), 4)
	}
	return ansi.StringWidth(c.coords)
}

// tableRow is one row of the table before it is fitted: its prefix (cursor
// and selection marks), its name, its cells and its tag. float marks the
// floating row, whose title shortens before anything else gives way.
type tableRow struct {
	prefix string
	name   string
	float  bool
	cells  rowCells
	tag    string
}

// tableShape is one step of the narrowing: what each column keeps.
type tableShape struct {
	floatTitle int  // index into floatTitles
	weekday    bool // the weekday stands before the date
	nameCap    int  // the widest a name may be
	place      bool // the place column is drawn
	placeCap   int  // the widest the place column may be; -1 for its whole width
	tagCap     int  // the widest the tag may be; -1 for its whole width
}

// floatTitles are the floating row's titles, longest first: `~ Now` gives way
// to `~Now` and `Now` before a place is cut (operator 2026-10-24). Never `~`
// alone, which says the chart has been moved off the clock.
var floatTitles = []string{"~ Now", "~Now", "Now"}

// tableWidths are the fitted columns of one shape.
type tableWidths struct {
	prefix, name, date, cal, clock, zone, tag int
	names                                     []string
	tags                                      []string
}

func measureTable(rows []tableRow, sh tableShape, cfg Config) tableWidths {
	var w tableWidths
	w.names = make([]string, len(rows))
	w.tags = make([]string, len(rows))
	for i, r := range rows {
		w.prefix = max(w.prefix, ansi.StringWidth(r.prefix))
		name := r.name
		if r.float && name == floatTitles[0] {
			name = floatTitles[sh.floatTitle]
		} else if name != "" {
			name = abbreviateNameTo(name, max(sh.nameCap, 1), cfg)
		}
		w.names[i] = name
		w.name = max(w.name, ansi.StringWidth(name))
		date := r.cells.date
		if sh.weekday {
			date = r.cells.weekday + " " + date
		}
		w.date = max(w.date, ansi.StringWidth(date))
		if r.cells.julian {
			w.cal = len(" Jul")
		}
		w.clock = max(w.clock, ansi.StringWidth(r.cells.clock))
		w.zone = max(w.zone, ansi.StringWidth(r.cells.zone))
		tag := r.tag
		if sh.tagCap >= 0 && ansi.StringWidth(tag) > sh.tagCap {
			tag = ansi.Truncate(tag, sh.tagCap, "")
		}
		w.tags[i] = tag
		w.tag = max(w.tag, ansi.StringWidth(tag))
	}
	return w
}

// fixed is the row's width without its place: every column, the gutter after
// the names, one space between columns and at least one before the tag.
func (w tableWidths) fixed() int {
	n := w.prefix + w.date + w.cal + 1 + w.clock + 1 + w.zone + 1 + w.tag
	if w.name > 0 {
		n += w.name + nameGutter
	}
	return n
}

// wordsFloor is how far confirmed place words are cut before the weekday and
// the names give way, when no row shows coordinates.
const wordsFloor = 10

// narrowTable is the shape the table takes at width, trying each step of the
// narrowing in order until the rows fit: the floating row's title shortens;
// the place words are cut down to the widest coordinates; the weekday goes;
// the names are abbreviated, then cut down to six cells; the place is cut
// down to its coordinates; the place goes; last, the tag is cut from its right
// end. The clock and the zone are never cut. It returns the shape and the
// place column's width (0 when the place is not drawn).
func narrowTable(rows []tableRow, width int, cfg Config) (tableShape, int) {
	sh := tableShape{weekday: true, nameCap: nameColumnMax, place: true, placeCap: -1, tagCap: -1}
	// coordsFloor is the widest coordinates a row shows; confirmed words
	// show none, so with only those the words keep wordsFloor cells.
	full, coordsFloor, lastFloor := 0, 0, 0
	for _, r := range rows {
		full = max(full, placeWidth(r.cells))
		if r.cells.words == "" || !r.cells.confirmed {
			coordsFloor = max(coordsFloor, ansi.StringWidth(r.cells.coords))
		}
		lastFloor = max(lastFloor, placeFloor(r.cells))
	}
	if coordsFloor == 0 {
		coordsFloor = wordsFloor
	}
	coordsFloor = min(coordsFloor, full)
	room := func(sh tableShape) int { return width - measureTable(rows, sh, cfg).fixed() - 1 }
	// fits says whether the shape fits with a place column at least floor
	// wide, and how wide that column may be.
	fits := func(sh tableShape, floor int) (int, bool) {
		r := room(sh)
		if r >= full {
			return full, true
		}
		return r, r >= floor
	}
	if w, ok := fits(sh, full); ok {
		return sh, w
	}
	// The floating row's title shortens only while it is what makes the name
	// column wide: shortening it beside a longer name saves nothing.
	for sh.floatTitle+1 < len(floatTitles) && hasFloatTitle(rows) {
		next := sh
		next.floatTitle++
		if measureTable(rows, next, cfg).name >= measureTable(rows, sh, cfg).name {
			break
		}
		sh = next
		if w, ok := fits(sh, full); ok {
			return sh, w
		}
	}
	if w, ok := fits(sh, coordsFloor); ok {
		return sh, w
	}
	sh.weekday = false
	if w, ok := fits(sh, coordsFloor); ok {
		return sh, w
	}
	for cap := measureTable(rows, sh, cfg).name - 1; cap >= 6; cap-- {
		sh.nameCap = cap
		if w, ok := fits(sh, coordsFloor); ok {
			return sh, w
		}
	}
	if w, ok := fits(sh, lastFloor); ok {
		return sh, w
	}
	sh.place = false
	if room(sh)+1 >= 0 {
		return sh, 0
	}
	for cap := measureTable(rows, sh, cfg).tag - 1; cap >= 0; cap-- {
		sh.tagCap = cap
		if room(sh)+1 >= 0 {
			return sh, 0
		}
	}
	return sh, 0
}

func hasFloatTitle(rows []tableRow) bool {
	for _, r := range rows {
		if r.float && r.name == floatTitles[0] {
			return true
		}
	}
	return false
}

// renderTable fits the rows into width cells each, by narrowTable's shape. A
// row with no moment renders "". Below every shape's width, at widths no
// layout offers, a row is cut at the right edge.
func renderTable(rows []tableRow, width int, cfg Config) []string {
	sh, placeW := narrowTable(rows, width, cfg)
	w := measureTable(rows, sh, cfg)
	out := make([]string, len(rows))
	for i, r := range rows {
		if !r.cells.ok {
			continue
		}
		var b strings.Builder
		pad := func(s string, n int) {
			b.WriteString(s)
			b.WriteString(strings.Repeat(" ", max(n-ansi.StringWidth(s), 0)))
		}
		pad(r.prefix, w.prefix)
		if w.name > 0 {
			pad(w.names[i], w.name+nameGutter)
		}
		date := r.cells.date
		if sh.weekday {
			date = r.cells.weekday + " " + date
		}
		pad(date, w.date)
		if w.cal > 0 {
			cal := ""
			if r.cells.julian {
				cal = " Jul"
			}
			pad(cal, w.cal)
		}
		b.WriteString(" ")
		pad(r.cells.clock, w.clock)
		b.WriteString(" ")
		pad(r.cells.zone, w.zone)
		if sh.place && placeW > 0 {
			b.WriteString(" ")
			pad(placeText(r.cells, placeW, cfg), placeW)
		}
		line := b.String()
		gap := width - ansi.StringWidth(line) - w.tag
		if gap < 1 {
			out[i] = term.CutRight(line+" "+w.tags[i], width, cfg)
			continue
		}
		out[i] = line + strings.Repeat(" ", gap) + w.tags[i] + strings.Repeat(" ", w.tag-ansi.StringWidth(w.tags[i]))
	}
	return out
}

// placeMemo keeps each place's witness answer, so the atlas is searched once
// per place, never again on a later frame.
type placeMemo struct {
	seen map[placeKey]bool
}

type placeKey struct {
	words    string
	lon, lat float64
}

// placeWitness is the atlas witness for the Chart panel's places, or nil
// while no atlas is loaded.
func (m Model) placeWitness() placeWitness {
	index := m.atlas.Index
	if index == nil {
		return nil
	}
	memo := m.places
	return func(words string, lon, lat float64) bool {
		k := placeKey{words, lon, lat}
		if memo != nil {
			if ok, hit := memo.seen[k]; hit {
				return ok
			}
		}
		ok := location.ConfirmsNear(index, words, lon, lat)
		if memo != nil {
			memo.seen[k] = ok
		}
		return ok
	}
}
