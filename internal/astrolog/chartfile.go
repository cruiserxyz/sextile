package astrolog

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/cruiserxyz/sextile/internal/recipe"
)

// ChartFile is the neutral result of resolving an astrolog chart-info
// file: the authoritative birth moment (UT) plus the raw coordinates and labels
// read from the file. It carries NO location types — country/timezone resolution
// happens a layer up (lookup), against the atlas — so astrolog stays free of the
// geo plane.
type ChartFile struct {
	WhenUTC    time.Time // birth moment in UTC, from astrolog's own Julian Day
	JD         float64   // that Julian Day exactly as astrolog printed it (5 decimals)
	Lon, Lat   float64   // decimal degrees, East/North positive (PlaceInfo convention)
	Name       string    // chart label from -zi (first quoted string); "" if absent
	PlaceLabel string    // place label from -zi (second quoted string); "" if absent
	// Houses is the system the file names, as Astrolog's own "<Name> houses"
	// header reports it (the specification C2). HasHouses is false when the
	// file names none ("Null houses"), or Astrolog printed no header or one this
	// build's table does not know; Houses is then zero and means nothing.
	Houses    recipe.HouseSystem
	HasHouses bool
}

// jdUnixEpoch is the Julian Day of the Unix epoch (1970-01-01T00:00:00Z).
const jdUnixEpoch = 2440587.5

var (
	// -qb <Mon> <Day> <Year> <time> <DT|ST> <zone> <lon> <lat>. The listing reader
	// needs only the trailing lon/lat. Minutes and seconds are optional (DDD,
	// DDD:MM, DDD:MM:SS or DDD:MM'SS) so a whole-degree coordinate is accepted
	// too, matching parseDegMin; the greedy prefix anchors to the LAST E/N pair,
	// so an earlier zone (colon-less "1E" or half-hour "5:30E") is consumed by
	// the prefix rather than mistaken for the longitude. It is applied to the
	// eight arguments Astrolog's own argument reader took for a -qb switch
	// (scanChartSwitches), never to the file's text.
	reQB = regexp.MustCompile(`(?m)^\s*-qb\b.*\s+([0-9]+(?::[0-9]+(?:[':][0-9]+)?)?[EW])\s+([0-9]+(?::[0-9]+(?:[':][0-9]+)?)?[NS])\s*$`)
	// -qb <Mon> <Day> <Year> ... — the YEAR field only, the third whitespace-
	// separated token after the switch. Astrolog accepts both a month name and
	// a month number in the first field, so the pattern skips two tokens rather
	// than trying to spell either. A negative year (BC) is accepted. It is
	// applied to the single line reQB already matched, never to the whole file:
	// against the whole text it could take the year from one -qb line while the
	// coordinates came from another.
	//
	// The two skipped tokens are captured too, as the month and day of the
	// listing row's date label (the specification C10). They are
	// validated separately: a bad month or day costs the row its date, never
	// the row itself.
	reQBYear = regexp.MustCompile(`^\s*-qb\s+(\S+)\s+(\S+)\s+(-?[0-9]{1,5})(?:\s|$)`)
)

// chartSwitches is what Astrolog's own reader takes from a chart-info file's
// text: every -qb switch (its eight arguments) and every -zi switch (its name
// and place), in file order. Astrolog applies the LAST of each.
type chartSwitches struct {
	qb [][]string
	zi [][2]string
}

// textArgs is how many arguments Astrolog 8.00 takes for each chart-record
// switch whose arguments are free text (astrolog.cpp FProcessSwitches: -zj
// name and place, -zL and -zN a city, -qc ten fields ending in name and place).
var textArgs = map[string]int{"zj": 2, "zL": 1, "zN": 1, "qc": 10}

// scanChartSwitches reads a chart-info file the way FProcessSwitchFile and
// NParseCommandLine do (Astrolog 8.00, io.cpp and astrolog.cpp): the file is
// cut into lines at any control character but the tab, each line is split
// into arguments, and the arguments are walked as switches. Several switches
// can share a line, so `-zi "A" "Paris" -zi "B" "Rome"` is two of them.
//
// Only -qb and -zi are read, and each takes the arguments Astrolog gives it
// (8 and 2), so an argument that looks like a switch (a name of `-zi`) is not
// read as one. The chart-record switches whose arguments are free text —
// names, places, cities — are passed over WITH their arguments, as
// FProcessSwitches consumes them (textArgs): the quotes are gone before
// Astrolog looks for a switch, so a quoted `"-qb"` in switch position would be
// one, and only the argument count keeps it from being. Every other switch is
// passed over alone: its arguments are numbers, which cannot spell -qb or
// -zi. A `;` at the start of an argument ends the line, as Astrolog's `-;`
// does.
func scanChartSwitches(text string) chartSwitches {
	var sw chartSwitches
	for _, line := range chartFileLines(text) {
		toks := astrologTokens(line)
	walk:
		for i := 0; i < len(toks); i++ {
			w := toks[i]
			if w != "" && strings.IndexByte("-/_=:", w[0]) >= 0 {
				w = w[1:]
			}
			switch {
			case strings.HasPrefix(w, ";"):
				break walk
			case strings.HasPrefix(w, "qb"):
				// Eight arguments, or what is left of the line: a line too
				// short for Astrolog still lists, and fails at pick time.
				end := min(i+9, len(toks))
				sw.qb = append(sw.qb, toks[i+1:end])
				i = end - 1
			case strings.HasPrefix(w, "zi") && i+2 < len(toks):
				sw.zi = append(sw.zi, [2]string{toks[i+1], toks[i+2]})
				i += 2
			case textArgs[w] > 0:
				i += textArgs[w]
			}
		}
	}
	return sw
}

// chartFileLines cuts a file into the lines Astrolog's reader sees. A line
// starts at the first byte that is not a control character (bytes from 0x80
// count as control there: Astrolog compares a signed char) and runs to the next
// control character other than the tab; one byte after a line end that is not
// a newline is dropped too, as its reader does.
func chartFileLines(text string) []string {
	var lines []string
	i := 0
	for i < len(text) {
		for i < len(text) && (text[i] < ' ' || text[i] >= 0x80) {
			i++
		}
		if i >= len(text) {
			break
		}
		start := i
		for i < len(text) && (text[i] >= ' ' || text[i] == '\t') {
			i++
		}
		end := i
		lines = append(lines, text[start:end])
		if i < len(text) && text[i] != '\n' {
			i++
		}
		i++
	}
	return lines
}

// astrologTokens splits one line into arguments exactly as NParseCommandLine
// does. Spaces and tabs separate arguments. An argument that begins with a
// double or a single quote runs to the next quote of the SAME kind that is
// followed by a character below '@' (or the end of the line): a quote followed
// by a letter is part of the text. The opening quote is dropped, the closing
// one too.
func astrologTokens(line string) []string {
	var toks []string
	space, quote, start := true, byte(0), 0
	for i := 0; i < len(line); i++ {
		c := line[i]
		switch {
		case c == ' ' || c == '\t':
			if !space && quote == 0 {
				toks = append(toks, line[start:i])
				space = true
			}
		case space:
			quote = 0
			if c == '"' || c == '\'' {
				quote = c
			}
			start = i
			if quote != 0 {
				start++
			}
			space = false
		case quote != 0 && c == quote && closesQuote(line, i+1):
			toks = append(toks, line[start:i])
			space = true
		}
	}
	if !space {
		toks = append(toks, line[start:])
	}
	return toks
}

// closesQuote says whether the byte at j lets a quote before it end an
// argument: Astrolog's `pch[1] < '@'` on a signed char, so the end of the line
// and every byte from 0x80 count as below.
func closesQuote(line string, j int) bool {
	return j >= len(line) || line[j] < '@' || line[j] >= 0x80
}

// labels is the name and place Astrolog itself prints for a chart-info file:
// where the file holds several `-zi` switches the LAST one wins
// (the specification C0, N26; the specification C5).
// "" and "" when it holds none.
func (sw chartSwitches) labels() (name, place string) {
	all := sw.zi
	if len(all) == 0 {
		return "", ""
	}
	last := all[len(all)-1]
	return last[0], last[1]
}

// ChartFileHeader is what an Astrolog chart-info file states about itself, read
// straight from its own text with no process spawned: the year of the birth
// moment, the coordinates, and the labels.
//
// The Year is the -qb line's year FIELD — a label for a listing row, not a
// moment. Month and Day are the same kind of label, read off the same line, so
// the row can show the date as the file states it (the specification). It is deliberately not a time: the file's local time, zone and
// daylight flag still need astrolog to become an instant, and that conversion
// belongs to ResolveChartFile, which does it at pick time. Anything that needs
// to KNOW WHEN the chart is must call ResolveChartFile; anything that only
// needs to SHOW a year can read it here for free.
type ChartFileHeader struct {
	Year       int     // year field of the -qb line; a display label only
	Month, Day int     // month (1-12) and day of the same line; 0, 0 when they do not form a date
	Lon, Lat   float64 // decimal degrees, East/North positive
	Name       string  // chart label from -zi (first quoted string); "" if absent
	PlaceLabel string  // place label from -zi (second quoted string); "" if absent
}

// ReadChartFileHeader parses an Astrolog chart-info file without running astrolog.
//
// It applies ResolveChartFile's FILE-LEVEL rejections — no `@AI` header, no -qb
// coordinates — plus its own: a -qb line whose year field is not a number has
// nothing to put on a row. It deliberately does NOT reproduce astrolog's full
// -qb grammar. Whether astrolog can turn a well-formed-looking line into an
// instant is only knowable by asking astrolog, and not asking is the point of
// this function: a file that passes here and fails there now surfaces at pick
// time, where the failure is reported with a reason, instead of vanishing from
// the listing without one.
//
// The year and the coordinates are read from THE SAME -qb switch, and it is
// the one Astrolog uses: the LAST (the specification C9). The labels
// are the last -zi switch, so a file lists with the place, name and moment it
// opens with. reQB is applied to the eight arguments of that one switch (as
// ResolveChartFile's file check does), and the year pattern to the same text.
func ReadChartFileHeader(path string) (ChartFileHeader, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return ChartFileHeader{}, fmt.Errorf("astrolog: ReadChartFileHeader: %w", err)
	}
	text := string(raw)
	if KindOfPrefix(raw) != FileChart {
		return ChartFileHeader{}, fmt.Errorf("astrolog: ReadChartFileHeader: %q is not an astrolog chart-info file (no @AI header)", path)
	}
	sw := scanChartSwitches(text)
	if len(sw.qb) == 0 {
		return ChartFileHeader{}, fmt.Errorf("astrolog: ReadChartFileHeader: no -qb coordinates in %q", path)
	}
	line := "-qb " + strings.Join(sw.qb[len(sw.qb)-1], " ")
	q := reQB.FindStringSubmatch(line)
	if q == nil {
		return ChartFileHeader{}, fmt.Errorf("astrolog: ReadChartFileHeader: no -qb coordinates in %q", path)
	}
	y := reQBYear.FindStringSubmatch(line)
	if y == nil {
		return ChartFileHeader{}, fmt.Errorf("astrolog: ReadChartFileHeader: no year in the -qb line of %q", path)
	}
	year, err := strconv.Atoi(y[3])
	if err != nil {
		return ChartFileHeader{}, fmt.Errorf("astrolog: ReadChartFileHeader: bad year %q in %q: %w", y[3], path, err)
	}
	month, day := qbMonthDay(y[1], y[2], year)

	h := ChartFileHeader{
		Year:  year,
		Month: month,
		Day:   day,
		Lon:   parseDegMin(q[1]), // East positive
		Lat:   parseDegMin(q[2]), // North positive
	}
	h.Name, h.PlaceLabel = sw.labels()
	return h, nil
}

// qbMonths are the month names a -qb line may spell, by their first three
// letters — the part astrolog itself compares.
var qbMonths = []string{"jan", "feb", "mar", "apr", "may", "jun", "jul", "aug", "sep", "oct", "nov", "dec"}

// qbMonthDay reads a -qb line's month and day fields as a calendar date in
// year. The month may be a name or a number. Anything that is not a real date
// — month 13, a day that is not a number, Feb 30 — returns 0, 0: the row loses
// its date label and keeps everything else.
//
// "A real date" is judged in astrolog's calendar, not Go's: astrolog reads a
// date before the Gregorian reform (15 Oct 1582) as Julian, where every fourth
// year is a leap year, so `Feb 29 1500` is a date it accepts and Go's
// proleptic Gregorian calendar does not.
func qbMonthDay(monthField, dayField string, year int) (int, int) {
	month, err := strconv.Atoi(monthField)
	if err != nil {
		month = 0
		if f := strings.ToLower(monthField); len(f) >= 3 {
			for i, name := range qbMonths {
				if f[:3] == name {
					month = i + 1
				}
			}
		}
	}
	day, err := strconv.Atoi(dayField)
	if err != nil || month < 1 || month > 12 || day < 1 || day > qbDaysIn(year, month) {
		return 0, 0
	}
	return month, day
}

// qbDaysIn is the length of a month in astrolog's calendar: Julian before
// 1582, Gregorian from it.
func qbDaysIn(year, month int) int {
	switch month {
	case 2:
		leap := year%4 == 0
		if year >= 1582 {
			leap = leap && (year%100 != 0 || year%400 == 0)
		}
		if leap {
			return 29
		}
		return 28
	case 4, 6, 9, 11:
		return 30
	}
	return 31
}

// afterChartFileRead is a test seam between Go's read of a chart file and the
// moment Astrolog is asked about it. Production never reassigns it.
var afterChartFileRead = func() {}

// ResolveChartFile reads an astrolog chart-info file and resolves it to a
// neutral ChartFile. Everything but the labels is Astrolog's own reading of
// the file: `astrolog -i <path> -w -b0` prints a centre box, and the birth
// moment (UT Julian Day), the coordinates (to the second) and the house system
// are read from that box's FIXED lines, counted from its lower inner border
// (readCentreBox). No value is found by searching the output for a pattern,
// since the chart's name and place print in the same box and could spell one
// (the specification). The labels are the last -zi switch of the file,
// split as Astrolog splits it, because the box cuts a long label.
//
// A file without an `@AI` header (e.g. arbitrary text) is rejected, as is one
// with no -qb switch, one astrolog cannot place, or one whose box does not have
// the expected shape.
func ResolveChartFile(ctx context.Context, path string) (ChartFile, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return ChartFile{}, fmt.Errorf("astrolog: ResolveChartFile: %w", err)
	}
	text := string(raw)
	if KindOfPrefix(raw) != FileChart {
		return ChartFile{}, fmt.Errorf("astrolog: ResolveChartFile: %q is not an astrolog chart-info file (no @AI header)", path)
	}
	sw := scanChartSwitches(text)
	if len(sw.qb) == 0 {
		return ChartFile{}, fmt.Errorf("astrolog: ResolveChartFile: no -qb coordinates in %q", path)
	}
	afterChartFileRead()

	// `-c 22` (Null) goes BEFORE -i: a house line in the file overrides it, so
	// the box names the file's own system, and a file with none prints Null.
	// `=b0` sets (never toggles) the seconds display: without it the box rounds
	// the coordinates to the minute and the Julian Day to five decimals.
	//
	// Astrolog reads a private copy of the bytes read above, never the path
	// again: the labels, the coordinates and the moment are then ONE read of
	// the file, and a file replaced between Go's read and Astrolog's cannot
	// pair one client's name with another's birth (C0, N28).
	dir, err := os.MkdirTemp("", "sextile-chartfile-*")
	if err != nil {
		return ChartFile{}, fmt.Errorf("astrolog: ResolveChartFile: %w", err)
	}
	defer func() { _ = os.RemoveAll(dir) }()
	private := filepath.Join(dir, "chart.as")
	if err := os.WriteFile(private, raw, 0o600); err != nil {
		return ChartFile{}, fmt.Errorf("astrolog: ResolveChartFile: %w", err)
	}
	// The reform date is pinned BEFORE -i: the file's own -qb fields are turned
	// into a Julian Day by the rule in force when they are read, and a host
	// astrolog.as that moved it would move a library chart's moment
	// (the specification C6). It is the one exempt call that carries the pin.
	argv := append([]string{"-c", "22"}, reformPin...)
	argv = append(argv, "-i", private, flagMainShow, "-w", "=b0")
	out, _, err := spawn(ctx, newExempt("chart-file read: -c 22 -i <path> -R0 -w =b0 reads a chart RECORD (moment, place, house system) from the centre box; -R0 restricts every object, so it prints no bodies or aspects", argv...))
	if err != nil {
		return ChartFile{}, fmt.Errorf("astrolog: ResolveChartFile: %w", err)
	}
	box, err := readCentreBox(out)
	if err != nil {
		return ChartFile{}, fmt.Errorf("astrolog: ResolveChartFile: the chart box for %q was not understood: %w", path, err)
	}

	cf := ChartFile{
		JD:      box.jd,
		WhenUTC: julianDayToUTC(box.jd),
		Lon:     box.lon, // East positive
		Lat:     box.lat, // North positive
	}
	cf.Name, cf.PlaceLabel = sw.labels()
	cf.Houses, cf.HasHouses = recipe.ParseHouseSystem(box.houses)
	return cf, nil
}

var (
	// reBoxBorder is the inner border of the centre box: a house cusp label,
	// a run of dashes between two bars, a house cusp label. Only Astrolog's own
	// border lines begin with `<n>`, so a name cannot spell one: it prints
	// inside a row that starts with a bar.
	reBoxBorder = regexp.MustCompile(`^<[0-9]+>[^|]*\|-+\|[^|]*<[0-9]+>$`)
	reBoxJD     = regexp.MustCompile(`^Julian Day:\s*(-?[0-9]+\.[0-9]+)$`)
	// DT Zone 1:00E,   4:53'41E 52:22'07N — the zone is read past, not kept. With
	// -Yz1 in the file (general.cpp SzOffset) the row is "Zone h1e,   ..." with
	// no leading DT/ST word, so the word is optional. With -b2 in the file a
	// place whose seconds are both zero prints to the minute ("4:53E 52:22N")
	// even under =b0, so the seconds are optional too.
	reBoxZone = regexp.MustCompile(`^(?:\S+ )?Zone [^,]*,\s+([0-9]+:[0-9]+(?:'[0-9]+)?[EW])\s+([0-9]+:[0-9]+(?:'[0-9]+)?[NS])$`)
	// The UT and zodiac rows carry no value the reader keeps; their shape is
	// checked so a box whose rows moved is refused rather than read at the
	// wrong count (charts1.cpp PrintWheelCenter cases 5 and 7). A progressed
	// chart prints "Prog: ..." in the UT row and is refused: its Julian Day is
	// not the birth moment.
	reBoxUT     = regexp.MustCompile(`^UT: +[0-9]+:[0-9]+(?::[0-9]+)?(?:[ap]m)?, Sid\.T: +[0-9]+:[0-9]+(?::[0-9]+)?$`)
	reBoxZodiac = regexp.MustCompile(`^(?:Tropical|Sidereal|Sidereal Inv), [A-Z][A-Za-z]*(?: [A-Za-z]+)*$`)
)

// centreBox is what ResolveChartFile takes from Astrolog's centre box.
type centreBox struct {
	jd       float64
	lon, lat float64
	houses   string // Astrolog's name for the system, "Null" when the file names none
}

// readCentreBox reads the `-w =b0` centre box by position. Counted upward from
// its lower inner border, the lines are fixed (Astrolog 8.00, probed
// 2026-10-11): 1 the Julian Day, 2 the zodiac and centre, 3 the house system,
// 4 the UT and sidereal time, 5 the zone, longitude and latitude. The place,
// the date, the name and the title are above those and are never read.
//
// The box has nine rows and prints no row for an empty name or place, so a
// chart with either has blank rows between the Julian Day and the border (the
// spec's probe had both labels). Those padding rows are skipped first; the
// count starts at the lowest row that has text.
//
// A box of another shape is refused whole: a missing border, a line that is not
// the kind it should be — all five counted lines are checked, the two whose
// values are not kept included. Nothing is guessed.
func readCentreBox(out string) (centreBox, error) {
	lines := strings.Split(strings.ReplaceAll(out, "\r", ""), "\n")
	b := -1
	for i := len(lines) - 1; i >= 0; i-- {
		if reBoxBorder.MatchString(lines[i]) {
			b = i
			break
		}
	}
	if b < 5 {
		return centreBox{}, fmt.Errorf("no inner border with five lines above it")
	}
	// The box's own columns: the border's two bars. Every row of the box has
	// its bars in the same two columns.
	l, r := strings.IndexByte(lines[b], '|'), strings.LastIndexByte(lines[b], '|')
	rowCell := func(i int) (string, bool) {
		if i < 0 || len(lines[i]) <= r || lines[i][l] != '|' || lines[i][r] != '|' {
			return "", false
		}
		return strings.TrimSpace(lines[i][l+1 : r]), true
	}
	top := b - 1
	for c, ok := rowCell(top); ok && c == "" && top > 0; c, ok = rowCell(top) {
		top--
	}
	cell := func(n int) (string, error) {
		c, ok := rowCell(top - n + 1)
		if !ok {
			return "", fmt.Errorf("line %d above the border is not a row of the box", n)
		}
		return c, nil
	}
	var box centreBox
	jd, err := cell(1)
	if err != nil {
		return centreBox{}, err
	}
	m := reBoxJD.FindStringSubmatch(jd)
	if m == nil {
		return centreBox{}, fmt.Errorf("the line above the border is %q, not a Julian Day", jd)
	}
	if box.jd, err = strconv.ParseFloat(m[1], 64); err != nil {
		return centreBox{}, fmt.Errorf("bad Julian Day %q: %w", m[1], err)
	}
	for _, row := range []struct {
		n    int
		re   *regexp.Regexp
		kind string
	}{{2, reBoxZodiac, "a zodiac"}, {4, reBoxUT, "a UT"}} {
		c, err := cell(row.n)
		if err != nil {
			return centreBox{}, err
		}
		if !row.re.MatchString(c) {
			return centreBox{}, fmt.Errorf("line %d above the border is %q, not %s line", row.n, c, row.kind)
		}
	}
	houses, err := cell(3)
	if err != nil {
		return centreBox{}, err
	}
	name, ok := strings.CutSuffix(houses, " houses")
	if !ok || name == "" {
		return centreBox{}, fmt.Errorf("the house line is %q, not \"<name> houses\"", houses)
	}
	box.houses = name
	zone, err := cell(5)
	if err != nil {
		return centreBox{}, err
	}
	z := reBoxZone.FindStringSubmatch(zone)
	if z == nil {
		return centreBox{}, fmt.Errorf("the zone line is %q, not a zone with coordinates", zone)
	}
	box.lon, box.lat = parseDegMin(z[1]), parseDegMin(z[2])
	return box, nil
}

// FormatChartFile renders a MomentAtPlace + house system as an astrolog chart file
// chart-info file, used to stage the two charts of a `-r0` comparison. The
// moment is written as the very `-qj <Julian Day>` every other call for the slot
// sends (momentArg), in UT: no calendar fields are built in Go, so a date before
// the Gregorian reform cannot drift (the specification C1), and the moment is
// full-fidelity to the second, not the minute granularity ResolveChartFile
// rounds to for chart-library display. The `-zi` line
// carries the source's Name and Place through labelText (empty for an unnamed
// chart): the header Astrolog prints for a pair names its charts
// (the specification C1). The wheel stages empty labels (C5a).
func FormatChartFile(m recipe.MomentAtPlace, houses recipe.HouseSystem, zodiac recipe.Zodiac) (string, error) {
	code, ok := houseCode(houses)
	if !ok {
		return "", fmt.Errorf("astrolog: FormatChartFile: unknown HouseSystem %d", int(houses))
	}
	jd, err := momentArg(m.JD)
	if err != nil {
		return "", fmt.Errorf("astrolog: FormatChartFile: %w", err)
	}
	return fmt.Sprintf(
		"@AI780  ; Astrolog chart info.\n-qj %s\n-zl %s %s\n%s\n-c %s\n%s",
		jd, formatLon(m.Lon), formatLat(m.Lat), stagedZI(m.Name, m.Place), code, zodiacLines(zodiac),
	), nil
}

// zodiacLines is the zodiac and plane pin a staged chart file carries after its
// `-c` line, the same two switches frameBand puts on the argv, so a file read
// back is never left to the host's astrolog.as (the specification C2).
// There is never a bare `-s`: it toggles against whatever the host left.
func zodiacLines(z recipe.Zodiac) string {
	pin, ok := zodiacPin(z)
	if !ok {
		// Unreachable through Compile, which validates the spec; a hand call
		// with no such zodiac gets the tropical pin rather than a bare switch.
		pin = []string{"_s"}
	}
	return strings.Join(pin, " ") + "\n_Ys\n"
}

// FormatLocalChartFile is FormatChartFile for a LocalMomentAtPlace: the same
// chart file, but a `-zZ` line after the place gives the moment the place's
// local clock, so Astrolog prints the slot's own time in the header. The
// instant is unchanged: it is the same `-qj` Julian Day (and inside the 1582
// window, where no zone is sent, the header is UT: zoneArgs).
func FormatLocalChartFile(m recipe.LocalMomentAtPlace, houses recipe.HouseSystem, zodiac recipe.Zodiac) (string, error) {
	code, ok := houseCode(houses)
	if !ok {
		return "", fmt.Errorf("astrolog: FormatLocalChartFile: unknown HouseSystem %d", int(houses))
	}
	jd, err := momentArg(m.JD)
	if err != nil {
		return "", fmt.Errorf("astrolog: FormatLocalChartFile: %w", err)
	}
	zone, err := zoneArgs(m.JD, m.TZ)
	if err != nil {
		return "", err
	}
	zoneLine := ""
	if len(zone) > 0 {
		zoneLine = strings.Join(zone, " ") + "\n"
	}
	return fmt.Sprintf(
		"@AI780  ; Astrolog chart info.\n-qj %s\n-zl %s %s\n%s%s\n-c %s\n%s",
		jd, formatLon(m.Lon), formatLat(m.Lat), zoneLine, stagedZI(m.Name, m.Place), code, zodiacLines(zodiac),
	), nil
}

// julianDayToUTCSecond converts an astrolog Julian Day (UT) to a UTC time
// TRUNCATED to the whole second, the writer's resolution (finer than
// julianDayToUTC's minute rounding, which serves the chart-library display).
//
// Truncated, not rounded: one slot has one second, the one Sextile shows (the
// floating slot's clock reads 23:59 at 23:59:59.6, never the next day's
// midnight) and the one the timeline's -qb natal already sends. Every call for
// the slot (-qj, the staged local and UT chart files) reads it here. The 1 ms
// tolerance absorbs float error in the Julian Day (about 40 microseconds at
// today's dates), so a whole second that lands a hair below its value does not
// fall to the second before. Floor is also correct before 1970.
func julianDayToUTCSecond(jd float64) time.Time {
	sec := (jd - jdUnixEpoch) * 86400.0
	return time.Unix(int64(math.Floor(sec+0.001)), 0).UTC()
}

// julianDayToUTC converts an astrolog Julian Day (UT) to a UTC time rounded to
// the nearest minute. astrolog prints the JD to 5 decimals (~0.86 s) and the
// chart file -qb grammar is minute-precision, so minute-rounding is lossless and
// stable across the quantization.
func julianDayToUTC(jd float64) time.Time {
	sec := (jd - jdUnixEpoch) * 86400.0
	return time.Unix(int64(math.Round(sec/60.0))*60, 0).UTC()
}

// parseDegMin parses an astrolog "DDD:MMx" coordinate (x ∈ E/W/N/S), with
// optional seconds ("DDD:MM'SSx" or "DDD:MM:SSx"), into signed decimal degrees
// with East and North positive.
func parseDegMin(s string) float64 {
	if len(s) < 2 {
		return 0
	}
	hemi := s[len(s)-1]
	parts := strings.Split(strings.ReplaceAll(s[:len(s)-1], "'", ":"), ":")
	var deg, min, sec float64
	deg, _ = strconv.ParseFloat(parts[0], 64)
	if len(parts) > 1 {
		min, _ = strconv.ParseFloat(parts[1], 64)
	}
	if len(parts) > 2 {
		sec, _ = strconv.ParseFloat(parts[2], 64)
	}
	val := deg + min/60.0 + sec/3600.0
	if hemi == 'W' || hemi == 'S' {
		val = -val
	}
	return val
}
