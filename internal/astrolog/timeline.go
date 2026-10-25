package astrolog

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cruiserxyz/sextile/internal/recipe"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// The timeline is Astrolog's own dated-event searches read as one list:
// `-t` transits to a natal chart, `-dm` the sky's own month, `-tr` returns
// (the specification). This file is the ONE seam all three are
// read through (D10), so the reading-contract migration (direction.md M1)
// replaces one function body and no caller.
//
// Astrolog computes every row; this file only reads when each row happened
// and keeps the row's text byte-for-byte (D9). It never rewrites a printed
// time — not even to correct a zone. See § The DST workaround in the spec for
// why a month may take more than one invocation per search.

// TimelineSource names which of the three searches emitted a row. The order
// of the constants is the tie-break order for rows that share an instant
// (C8): transit, then sky, then return.
type TimelineSource uint8

const (
	TimelineTransit TimelineSource = iota + 1 // `-t`, needs a natal chart
	TimelineSky                               // `-dm`, needs none
	TimelineReturn                            // `-tr`, needs a natal chart
)

func (s TimelineSource) String() string {
	switch s {
	case TimelineTransit:
		return "transit"
	case TimelineSky:
		return "sky"
	case TimelineReturn:
		return "return"
	}
	return "unknown"
}

// Switch is Astrolog's own switch for the search: `-t`, `-dm` or `-tr`. It is
// the name a failed search is reported under, spelled here so no caller
// outside this package writes an Astrolog switch.
func (s TimelineSource) Switch() string {
	switch s {
	case TimelineTransit:
		return "-t"
	case TimelineSky:
		return "-dm"
	case TimelineReturn:
		return "-tr"
	}
	return s.String()
}

// TimelineRow is one dated event: the absolute instant Astrolog's printed
// local time denotes, and Astrolog's line exactly as printed. ParseTimeline
// fills only those two; ComputeTimeline adds which search emitted the row
// and the command that emitted it (gate 12: every displayed row traces to
// the argv that produced it).
type TimelineRow struct {
	Instant time.Time
	Line    string
	Source  TimelineSource
	Command string

	// The columns of Line, cut from the regex groups that already matched it
	// (never re-read from Line by a caller). Weekday is empty on a transit
	// row. Date and Time are Astrolog's own text ("10/ 1/2026", "9:25am").
	// Body is the first body's name; Rest is everything after it, exactly as
	// Astrolog wrote it. Together with the spaces between them they are Line.
	Weekday string
	Date    string
	Time    string
	Transit bool
	Body    string
	Rest    string
	// Julian is the branch Astrolog's reader took for the printed date: it
	// names a day before 15 Oct 1582 (the specification C7).
	Julian bool

	// What the row is and the words of its Rest, cut from the same match
	// (the specification rule 1). Aspect is Astrolog's
	// abbreviation ("Con", "Sex", "Squ"), Other the second body's name, for an
	// aspect row; Sign and Direction ("-->" or "<--") for an ingress; Station
	// ("S/R" or "S/D") for a station. Empty where the kind has none.
	Kind      TimelineKind
	Aspect    string
	Other     string
	Sign      string
	Direction string
	Station   string

	// The names resolved to the registries Sextile already holds (bodyFullName,
	// aspectAbbrev), here because the ui may not import this package
	// (the specification rule 2). Resolved says every name the
	// kind needs resolved: an aspect row's two bodies and its aspect, an
	// ingress or station's body. A name with no entry (a cusp, a star) leaves
	// it false, and the row is unmarkable.
	BodyID   aspect.Body
	OtherID  aspect.Body
	AspectID aspect.AspectKind
	Resolved bool
}

// TimelineKind says which of the three things a timeline row can be it is.
// The zero value is none: a row built by hand, or read from no grammar.
type TimelineKind int

const (
	TimelineKindNone TimelineKind = iota
	TimelineKindAspect
	TimelineKindIngress
	TimelineKindStation
)

// The two line grammars, anchored at BOTH ends: a line is a dated event only
// if the whole of it has one of the shapes Astrolog was measured to print
// (spec § What the three grammars look like; census of 2026: every -dy row
// and every -t/-tr row of twelve months). A prefix match would accept a line
// cut short or reshaped by a format change, which is the change D10 exists to
// make loud.
const (
	tlBody = `(?P<body>[A-Z][A-Za-z.]*(?: [A-Za-z][A-Za-z.]*)*)`
	// tlBodyOther is the same shape without a capture: only the FIRST body
	// is a column.
	tlBodyOther = `[A-Z][A-Za-z.]*(?: [A-Za-z][A-Za-z.]*)*`
	tlSign      = `(?:\([A-Z][a-z]{2}\)|\[[A-Z][a-z]{2}\]|<[A-Z][a-z]{2}>)`
	// tlAsp is the three-letter aspect abbreviation as aspectAbbrev spells it:
	// `Con`, but also `BQn`, `SSx` and `SSq`, whose second letter is a capital.
	// A `[a-z]{2}` tail refused every minor-aspect row the operator's chooser
	// had turned on, and with it the whole month (2026-10-22).
	tlAsp   = `[A-Z][A-Za-z]{2}`
	tlLabel = `\([A-Z][A-Za-z ]*\)`
	// (v/c HH:MM) marks the Moon's last major aspect in its sign: HH:MM is how
	// long it stays void of course, the hours and minutes until the next Moon
	// ingress (checked against the October 2026 rows). It is a duration, not a
	// clock time, only ever the LAST thing on a line, and the event's own time
	// is read from its fixed place after the date, never from here.
	tlVoid = `\(v/c [0-9]{1,2}:[0-9]{2}\)`
	tlTail = `(?: ` + tlLabel + `)*(?: ` + tlVoid + `)?`
)

// tlDate is the date and time columns in the form the held clock preference
// pins (the specification C3). Astrolog prints exactly one of four forms
// for a given pair of pins, so the grammar is the preference's and no other:
// a row in another form is refused, never guessed (` 1-10-2026` read
// month-first is 10 January).
//
//	24h month-first  10/ 1/2026  0:40        12h month-first  10/ 1/2026 12:40am
//	24h day-first     1-10-2026  0:40        12h day-first     1-10-2026 12:40am
func tlDate(c Clock) string {
	// Astrolog prints the month and the day space-padded to two cells and the
	// year to four, with a minus sign for a year before 1 (year 0 is 1 BC):
	// ` 3/ 1/   5`, ` 3/ 1/-100`, `1- 3-1500`.
	date := `(?P<date>(?P<mo>[0-9]{1,2})/ *(?P<day>[0-9]{1,2})/ *(?P<year>-?[0-9]{1,5}))`
	if c.Dates == DayFirst {
		date = `(?P<date>(?P<day>[0-9]{1,2})- *(?P<mo>[0-9]{1,2})- *(?P<year>-?[0-9]{1,5}))`
	}
	clock := `(?P<time>(?P<hour>[0-9]{1,2}):(?P<min>[0-9]{2}))`
	if c.Hours == Hours12 {
		clock = `(?P<time>(?P<hour>[0-9]{1,2}):(?P<min>[0-9]{2})(?P<ampm>am|pm))`
	}
	return date + ` +` + clock
}

// timelineGrammar is the pair of line grammars for a clock, anchored at BOTH
// ends. The sky grammar (-dm) is a weekday, the date and time, then an aspect
// between two bodies, a sign ingress (--> forward, <-- by retrograde) or a
// station (S/R, S/D), then optional labels and void. The transit grammar
// (-t/-tr) is the date and time, Astrolog's `trans` and `natal` markers around
// an aspect, then an optional label such as (Solar Return).
func timelineGrammar(c Clock) (sky, transit *regexp.Regexp) {
	grammarMu.Lock()
	defer grammarMu.Unlock()
	if g, ok := grammars[c]; ok {
		return g[0], g[1]
	}
	d := tlDate(c)
	sky = regexp.MustCompile(`^(?P<wd>Sun|Mon|Tue|Wed|Thu|Fri|Sat) +` + d + ` +` +
		tlBody + `(?P<rest> ` + tlSign + ` (?:(?P<asp>` + tlAsp + `) ` + tlSign + ` (?P<other>` + tlBodyOther + `)|(?P<dir>-->|<--) (?P<ingress>[A-Z][a-z]+)|(?P<station>S/[RD]))` + tlTail + `)$`)
	transit = regexp.MustCompile(`^ *` + d + ` trans +` +
		tlBody + `(?P<rest> ` + tlSign + ` (?P<asp>` + tlAsp + `) natal ` + tlSign + ` (?P<other>` + tlBodyOther + `)(?: ` + tlLabel + `)*)$`)
	grammars[c] = [2]*regexp.Regexp{sky, transit}
	return sky, transit
}

var (
	grammarMu sync.Mutex
	grammars  = map[Clock][2]*regexp.Regexp{}
)

// The two lines Astrolog prints when a search has no events at all: `-t`/`-tr`
// print the first, `-dm` the second. A band that hides every body a month
// would have named (the operator unticked the only body with events, or chose
// none) makes this the ordinary result, not a format change, so they read as
// an empty list. Measured on 8.00, 2026-10-03: `-dm -R0` and `-t 10 2026 -R0`.
const (
	noTransitsFound  = "No transits found."
	noSkyEventsFound = "No transit events found."
)

// ParseTimeline reads the output of one `-t`, `-dm` or `-tr` search whose
// times are expressed in zone, and returns each dated event line's instant
// and the line byte-identically (C1). The same output under a different zone
// resolves to different instants (C1a): the zone is the reader's to supply,
// because the text carries only a local clock.
//
// It recognises and drops exactly these non-event lines: blank, a chart header
// beginning `Astrolog `, a search header beginning `Transits during ` (C4), and
// the two "no events" notices above. Every other line must be a dated event, or the
// whole parse fails naming it and returns no rows (C2) — including a line
// that looks dated but whose date or time cannot be read (C55). A format
// change is meant to be loud here rather than to lose a row silently.
func ParseTimeline(out string, zone *time.Location) ([]TimelineRow, error) {
	if zone == nil {
		return nil, fmt.Errorf("astrolog: ParseTimeline: no zone to read the printed times in")
	}
	var rows []TimelineRow
	for _, line := range strings.Split(out, "\n") {
		if strings.TrimSpace(line) == "" ||
			strings.HasPrefix(line, "Astrolog ") ||
			strings.HasPrefix(line, "Transits during ") ||
			line == noTransitsFound || line == noSkyEventsFound {
			continue
		}
		row, err := timelineRow(line, zone)
		if err != nil {
			return nil, &TimelineLineError{Line: line, Reason: err.Error()}
		}
		rows = append(rows, row)
	}
	return rows, nil
}

// TimelineLineError is ParseTimeline's refusal: the line it could not read,
// and why. It is typed so the perimeter can name the line to the operator
// (C41) instead of reducing it to a generic failure.
type TimelineLineError struct {
	Line   string
	Reason string
}

func (e *TimelineLineError) Error() string {
	return fmt.Sprintf("astrolog: ParseTimeline: %s: %q", e.Reason, e.Line)
}

// timelineRow reads one event line: its instant (date and clock time in
// zone) and its columns, cut from the groups of the grammar that matched.
func timelineRow(line string, zone *time.Location) (TimelineRow, error) {
	var re *regexp.Regexp
	var m []string
	transit := false
	clock := CurrentClock()
	skyLine, transitLine := timelineGrammar(clock)
	if m = skyLine.FindStringSubmatch(line); m != nil {
		re = skyLine
	} else if m = transitLine.FindStringSubmatch(line); m != nil {
		re, transit = transitLine, true
	} else {
		return TimelineRow{}, fmt.Errorf("not a dated event line")
	}
	g := func(name string) string {
		if i := re.SubexpIndex(name); i >= 0 {
			return m[i]
		}
		return ""
	}
	weekday := ""
	if !transit {
		weekday = g("wd")
	}
	ampm := ""
	if clock.Hours == Hours12 {
		ampm = g("ampm")
	}
	parsed, err := timelineInstant(weekday, []string{g("mo"), g("day"), g("year"), g("hour"), g("min"), ampm}, zone)
	if err != nil {
		return TimelineRow{}, err
	}
	row := TimelineRow{Instant: parsed.Instant, Line: line, Weekday: weekday, Date: g("date"), Time: g("time"),
		Transit: transit, Body: g("body"), Rest: g("rest"), Julian: parsed.Julian}
	row.cutKind(g("asp"), g("other"), g("dir"), g("ingress"), g("station"))
	return row, nil
}

// cutKind fills the kind columns from the groups of the match and resolves the
// names through the registries parse.go already holds (rule 2 of
// the specification). Only the groups that matched are set.
func (r *TimelineRow) cutKind(asp, other, dir, ingress, station string) {
	body, bodyOK := bodyFullName[r.Body]
	r.BodyID = body
	switch {
	case asp != "":
		r.Kind, r.Aspect, r.Other = TimelineKindAspect, asp, other
		second, otherOK := bodyFullName[other]
		kind, aspectOK := aspectAbbrev[asp]
		r.OtherID, r.AspectID = second, kind
		r.Resolved = bodyOK && otherOK && aspectOK
	case dir != "":
		r.Kind, r.Direction, r.Sign = TimelineKindIngress, dir, ingress
		r.Resolved = bodyOK
	case station != "":
		r.Kind, r.Station = TimelineKindStation, station
		r.Resolved = bodyOK
	}
}

// timelineInstant reads one event's date and clock time in zone, in Astrolog's
// calendar (AstrologCalendar.ParseIn): a date before 15 Oct 1582 is a Julian
// date, 29 February 1500 is a real day, and a month, day, hour or minute that is
// not one is refused, never normalised into another date.
func timelineInstant(weekday string, fields []string, zone *time.Location) (Parsed, error) {
	n := make([]int, 5)
	for i := range n {
		v, err := strconv.Atoi(fields[i])
		if err != nil {
			return Parsed{}, fmt.Errorf("unreadable number %q", fields[i])
		}
		n[i] = v
	}
	month, day, year, hour, minute := n[0], n[1], n[2], n[3], n[4]
	if fields[5] == "" {
		// 24-hour form: 0:00 to 23:59.
		if hour > 23 || minute > 59 {
			return Parsed{}, fmt.Errorf("impossible clock time %d:%02d", hour, minute)
		}
	} else {
		if hour < 1 || hour > 12 || minute > 59 {
			return Parsed{}, fmt.Errorf("impossible clock time %d:%02d%s", hour, minute, fields[5])
		}
		hour %= 12
		if fields[5] == "pm" {
			hour += 12
		}
	}
	p, err := AstrologCalendar{}.ParseIn(Fields{Year: year, Month: month, Day: day, Hour: hour, Minute: minute}, zone)
	if err != nil {
		return Parsed{}, err
	}
	if weekday != "" && p.Weekday.String()[:3] != weekday {
		return Parsed{}, fmt.Errorf("weekday %s does not match the date %d/%d/%d", weekday, month, day, year)
	}
	return p, nil
}

// TimelineSegment is one offset segment (C45): a maximal half-open instant
// interval [Start, End) over which the floating zone holds a single UTC
// offset, intersected with the month. Offset is seconds east of UTC.
//
// Segments exist ONLY to work around an Astrolog defect: its searches apply
// one fixed offset to every day of the span (charts3.cpp passes the constant
// Dst/Zon into each day's conversion), so no single `-z` can print a DST
// transition month correctly. Reported upstream.
// When that is fixed, segments, TimelineSegments and their tests go in one
// change and each search returns to one call per month (spec § The DST
// workaround — temporary, and why it is here).
type TimelineSegment struct {
	Start, End time.Time
	Offset     int
}

// TimelineSegments divides the month of the given year, in zone, into offset
// segments. The month is Astrolog's: it starts on the first of the month in the
// calendar Astrolog names it in (AstrologCalendar.MonthBounds), so a month before
// the Gregorian reform is bounded by its Julian days. Boundaries are INSTANTS,
// never dates: Amsterdam's 2026 fall-back is at 03:00 local on 25 October, inside
// a date, and a date-grained split has no correct place for that day. A month
// with no transition is one segment. A month that does not exist has none.
func TimelineSegments(year int, month time.Month, zone *time.Location) []TimelineSegment {
	segs, _ := timelineSegments(year, month, zone)
	return segs
}

func timelineSegments(year int, month time.Month, zone *time.Location) ([]TimelineSegment, error) {
	start, end, err := AstrologCalendar{}.MonthBounds(year, month, zone)
	if err != nil {
		return nil, fmt.Errorf("astrolog: timeline: %w", err)
	}
	if err := refuseBeforeJD0(start.Unix()); err != nil {
		return nil, fmt.Errorf("astrolog: timeline: %d-%02d: %w", year, int(month), err)
	}
	var segs []TimelineSegment
	for cur := start; cur.Before(end); {
		_, offset := cur.Zone()
		segEnd := end
		if _, zoneEnd := cur.ZoneBounds(); !zoneEnd.IsZero() && zoneEnd.Before(end) {
			segEnd = zoneEnd
		}
		// A zone change that keeps the offset (an abbreviation change) is
		// not a boundary: segments are maximal.
		if n := len(segs); n > 0 && segs[n-1].Offset == offset {
			segs[n-1].End = segEnd
		} else {
			segs = append(segs, TimelineSegment{Start: cur, End: segEnd, Offset: offset})
		}
		cur = segEnd
	}
	return segs, nil
}

// TimelineNatal is the anchored chart as a search's natal input: an absolute
// moment and a place. It carries no zone, because the moment is an instant —
// it is entered in UT, so the floating zone the search reports in can never
// move it (C6).
type TimelineNatal struct {
	When     time.Time
	Lon, Lat float64
}

// TimelineRequest is everything the searches depend on: the floating slot's
// month and IANA zone, the anchored chart when there is one, and the lens's
// object and aspect policy — so the choosers reach every search the way they
// reach the chart (the specification).
//
// Defaults are the two object sets Astrolog lists unfiltered, as read from the
// binary (ReadDefaultSets): the timeline's body band is hide-only against them.
type TimelineRequest struct {
	Year  int
	Month time.Month
	Zone  *time.Location
	Natal *TimelineNatal

	// Houses and Zodiac are the lens's reference frame; TimelineCall pins them
	// with the same band every chart call carries (the specification C5).
	// The zero values are Placidus and Tropical.
	Houses recipe.HouseSystem
	Zodiac recipe.Zodiac

	Objects  recipe.ObjectPolicy
	Aspects  recipe.AspectPolicy
	Defaults DefaultSets
}

// TimelineCall is one search as a body call: the argv and the chooser band the
// compiler issued for it.
//
// ORDER IS THE CONTRACT (C6, spec § The zone hazard): `-z` comes FIRST, so it
// sets the default zone the search reports in, and the natal `-qj` that
// follows resets the zone to UT and is not moved by it. Put `-z` after `-qj`
// and Astrolog rewrites the natal chart into the floating zone.
// The frame band, then the chooser band, follow the mode switch's month and year.
//
// Sextile's anchored chart is an instant, not a local time with a zone, so
// the natal is entered as `-qj <jd> -zl <lon> <lat>` — its UT moment, the very
// Julian Day every other call for the slot sends, never calendar fields built in
// Go (the specification C1). The spec's measurements used `-qa <local time>
// <zone>`; the UT form was measured to emit byte-identical rows (2026-09-30).
func TimelineCall(src TimelineSource, seg TimelineSegment, req TimelineRequest) (BodyCall, error) {
	if req.Month < time.January || req.Month > time.December {
		return BodyCall{}, fmt.Errorf("astrolog: TimelineArgv: month %d out of range", int(req.Month))
	}
	if _, err := timelineSegments(req.Year, req.Month, time.UTC); err != nil {
		return BodyCall{}, err
	}
	zone := []string{"-z", formatSearchZone(seg.Offset)}
	month, year := strconv.Itoa(int(req.Month)), strconv.Itoa(req.Year)
	var argv []string
	switch src {
	case TimelineSky:
		argv = append(zone, "-qm", month, year, "-dm")
	case TimelineTransit, TimelineReturn:
		if req.Natal == nil {
			return BodyCall{}, fmt.Errorf("astrolog: TimelineArgv: the %s search needs a natal chart", src)
		}
		// The same second and the same Julian Day every other call for this slot
		// names (momentArg): the moment is never built from Go's calendar
		// fields, which would drift before the Gregorian reform.
		natalJD, err := momentArg(SlotJD(req.Natal.When))
		if err != nil {
			return BodyCall{}, fmt.Errorf("astrolog: TimelineArgv: %w", err)
		}
		mode := "-t"
		if src == TimelineReturn {
			mode = "-tr"
		}
		argv = append(zone, "-qj", natalJD, "-zl", formatLon(req.Natal.Lon), formatLat(req.Natal.Lat))
		argv = append(argv, mode, month, year)
	default:
		return BodyCall{}, fmt.Errorf("astrolog: TimelineArgv: unknown source %d", int(src))
	}
	// The frame band follows the mode switch and its month and year, before the
	// chooser band, and is the very band chart calls carry (frameBand). Without
	// it a host astrolog.as with `=s` searches the sidereal sky while the list
	// is labelled by a tropical lens (the specification C5).
	frame, err := frameBand(req.Houses, req.Zodiac)
	if err != nil {
		return BodyCall{}, err
	}
	argv = append(argv, frame...)
	argv = append(argv, reformPin...)
	argv = append(argv, CurrentClock().Pins()...)
	band, err := timelineBand(src, req.Objects, req.Aspects, req.Defaults)
	if err != nil {
		return BodyCall{}, err
	}
	return newBodyCall(append(argv, band.tokens...), band), nil
}

// formatSearchZone renders an offset (seconds east) as Astrolog's `-z` value,
// which counts hours WEST: `-z 2` prints `2W`, so UTC+2 is `-2`. Asserted,
// never inferred (spec § The zone hazard).
func formatSearchZone(offset int) string {
	west := -offset
	sign := ""
	if west < 0 {
		sign, west = "-", -west
	}
	h, m, s := west/3600, west%3600/60, west%60
	// Astrolog reads the sign from the HOURS field, so `-0:19:32` parses as
	// 0:19:32 WEST (measured 2026-10-03: `-z -0:19:32` lists the same times as
	// `-z 0:19:32W`). An east zone under one hour — Amsterdam Time, +0:19:32,
	// until 1940; most local mean times east of Greenwich — takes Astrolog's
	// explicit `E` suffix instead, which it honours.
	if sign == "-" && h == 0 {
		if s != 0 {
			return fmt.Sprintf("0:%02d:%02dE", m, s)
		}
		return fmt.Sprintf("0:%02dE", m)
	}
	switch {
	case s != 0:
		return fmt.Sprintf("%s%d:%02d:%02d", sign, h, m, s)
	case m != 0:
		return fmt.Sprintf("%s%d:%02d", sign, h, m)
	}
	return fmt.Sprintf("%s%d", sign, h)
}

// TimelineSearcher runs one search call and returns Astrolog's output and the
// command that ran (empty when no process started). RunTimelineSearch is the
// real one; lookup's tests inject canned output through the same shape. The
// call carries its band so the real searcher reaches Astrolog only as a body
// call.
type TimelineSearcher func(ctx context.Context, call BodyCall) (out, command string, err error)

// RunTimelineSearch spawns Astrolog for one search, as a body call.
func RunTimelineSearch(ctx context.Context, call BodyCall) (string, string, error) {
	if err := validateArgv(call.argv); err != nil {
		return "", "", err
	}
	out, command, err := spawn(ctx, call)
	if err != nil {
		return out, command, fmt.Errorf("astrolog: timeline search: %w (output: %.400s)", err, out)
	}
	return out, command, nil
}

// ComputeTimeline runs the given searches for the request's month — each
// once per offset segment (C24, C46) — and returns their rows in source order
// (the order of srcs), each source's rows in Astrolog's output order. It does
// not merge, sort or deduplicate: arranging the union is lookup's job.
//
// A row is kept iff the instant read from its printed time at its segment's
// offset falls inside that segment (C46). Each invocation returns the whole
// month at its own offset, so in a transition month every event comes back
// once per segment and exactly one copy resolves into its own segment —
// including both occurrences of the repeated hour of a fall-back.
//
// Every row carries the command of the invocation that emitted it (C5). The
// commands of every spawn that started are returned in spawn order, on the
// error path too. Any failed or unreadable search fails the whole timeline.
func ComputeTimeline(ctx context.Context, req TimelineRequest, srcs []TimelineSource, search TimelineSearcher) ([]TimelineRow, []string, error) {
	if req.Zone == nil {
		return nil, nil, fmt.Errorf("astrolog: ComputeTimeline: no floating zone")
	}
	if search == nil {
		search = RunTimelineSearch
	}
	segs, err := timelineSegments(req.Year, req.Month, req.Zone)
	if err != nil {
		return nil, nil, err
	}
	var rows []TimelineRow
	var commands []string
	for _, src := range srcs {
		for _, seg := range segs {
			got, command, err := timelineSegment(ctx, req, src, seg, search)
			commands = recordCommand(commands, command)
			if err != nil {
				return nil, commands, err
			}
			rows = append(rows, got...)
		}
	}
	return rows, commands, nil
}

// skyOverflowLine is the line Astrolog 8.00 can print in a `-dm` search that
// is a miscount, not a loss (the specification). It is printed
// inside the day loop, so event rows may follow it.
const skyOverflowLine = "Too many transit events found."

// skyDayRowBound is the printed-date row count at which a sky output carrying
// skyOverflowLine is refused instead of read. It is a conservative refusal
// bound, not a claim that this many rows on one day imply loss.
//
// The proof, from charts3.cpp ChartInDaySearch in Astrolog 8.00, with hooks
// cleared as every Sextile call clears them: at each day's start B events are
// held back for the void-of-course mark, and that day collects at most 300-B.
// An event is lost only when one day finds 300-B or more. Mid-month, held
// events are flushed 75 at a time once more than 150 are held, so while every
// day finds fewer than 75 events, B never exceeds 150. Hence if no day has 75
// or more rows no day can have reached its limit, and the line, wherever it
// appears, is a false alarm: on the last day it adds the held events to the
// day's count and compares with a limit already reduced by them. If some day
// has 75 or more rows the month may really be cut short, so we refuse.
const skyDayRowBound = 75

// parseSearchOutput is ParseTimeline for one search call's output. Only the
// sky search, where the source is known, tolerates skyOverflowLine, and only
// when no printed date has skyDayRowBound or more rows (counted over this
// one invocation's raw rows, before the segment filter). Every other case is
// ParseTimeline's own refusal.
func parseSearchOutput(src TimelineSource, out string, zone *time.Location) ([]TimelineRow, error) {
	rows, err := ParseTimeline(out, zone)
	var lineErr *TimelineLineError
	if err == nil || src != TimelineSky || !errors.As(err, &lineErr) || lineErr.Line != skyOverflowLine {
		return rows, err
	}
	var kept []string
	for _, line := range strings.Split(out, "\n") {
		if line != skyOverflowLine {
			kept = append(kept, line)
		}
	}
	stripped, serr := ParseTimeline(strings.Join(kept, "\n"), zone)
	if serr != nil {
		return nil, serr
	}
	perDate := map[string]int{}
	for _, row := range stripped {
		perDate[row.Date]++
		if perDate[row.Date] >= skyDayRowBound {
			return nil, err
		}
	}
	return stripped, nil
}

// timelineSegment is one search call: one source over one offset segment. It
// returns the rows that fall inside the segment and the command of the spawn
// (empty when none started), and the error of the call, if any.
func timelineSegment(ctx context.Context, req TimelineRequest, src TimelineSource, seg TimelineSegment, search TimelineSearcher) ([]TimelineRow, string, error) {
	call, err := TimelineCall(src, seg, req)
	if err != nil {
		return nil, "", err
	}
	out, command, err := search(ctx, call)
	if err != nil {
		return nil, command, err
	}
	parsed, err := parseSearchOutput(src, out, time.FixedZone("", seg.Offset))
	if err != nil {
		return nil, command, err
	}
	var rows []TimelineRow
	for _, row := range parsed {
		if row.Instant.Before(seg.Start) || !row.Instant.Before(seg.End) {
			continue
		}
		row.Source, row.Command = src, command
		rows = append(rows, row)
	}
	return rows, command, nil
}

// TimelineCallResult is one search call's own outcome: the source and segment
// it ran, the rows it contributed, the command of the spawn (empty when none
// started) and its error. A failed call has no rows.
type TimelineCallResult struct {
	Source  TimelineSource
	Segment TimelineSegment
	Rows    []TimelineRow
	Command string
	Err     error
}

// ComputeTimelineCalls runs one source over each offset segment of the
// request's month, like ComputeTimeline, but isolates failure at the CALL: a
// failed segment is that call's Err and the next segment still runs, so a DST
// month does not lose a successful segment to its sibling's failure (C17).
// Results come in segment order, one per call. A cancelled context stops the
// run: the call that saw it is the last result. ComputeTimeline is unchanged
// and still fails whole.
func ComputeTimelineCalls(ctx context.Context, req TimelineRequest, src TimelineSource, search TimelineSearcher) ([]TimelineCallResult, error) {
	if req.Zone == nil {
		return nil, fmt.Errorf("astrolog: ComputeTimelineCalls: no floating zone")
	}
	if search == nil {
		search = RunTimelineSearch
	}
	segs, err := timelineSegments(req.Year, req.Month, req.Zone)
	if err != nil {
		return nil, err
	}
	var results []TimelineCallResult
	for _, seg := range segs {
		rows, command, err := timelineSegment(ctx, req, src, seg, search)
		results = append(results, TimelineCallResult{Source: src, Segment: seg, Rows: rows, Command: command, Err: err})
		if ctx.Err() != nil {
			break
		}
	}
	return results, nil
}
