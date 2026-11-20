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
	WhenUTC time.Time // birth moment in UTC, from astrolog's own Julian Day, to the second
	JD      float64   // that Julian Day exactly as astrolog printed it (7 decimals)
	// Clock is the moment as the file states it, in Astrolog's own words from
	// its centre box: the date in Astrolog's calendar, the clock to the second,
	// the daylight word and the zone. Nothing here is computed by Sextile.
	Clock BoxClock
	// Record is the guarded copy of the file this read handed to Astrolog
	// (GuardChartFile): the chart Astrolog casts, every time, is this record.
	Record string
	// HouseName is the house system Astrolog cast the file in at its own
	// place, in Astrolog's own words ("Placidus", "3D Placidus", "Whole
	// (MC)"); "Null" when the file names none.
	HouseName string
	// HouseNameAsked is the system the file asks for, in Astrolog's words,
	// where Astrolog cast another: it casts Porphyry for a system it cannot
	// cast at the place (calc.cpp, ComputeHouses and the Swiss Ephemeris
	// path). "" when the cast one is the one asked, or when Astrolog's
	// second reading of the file failed: that costs only the mark of a
	// substitution, never the chart.
	HouseNameAsked string
	// StatedSeconds says the file states its moment to the second: its -qb
	// time carries seconds.
	StatedSeconds bool
	// Notes are the file's lines Astrolog does not get (objects, aspects,
	// orbs), for the Log.
	Notes      []FileNote
	Lon, Lat   float64 // decimal degrees, East/North positive (PlaceInfo convention)
	Name       string  // chart label from -zi (first quoted string); "" if absent
	PlaceLabel string  // place label from -zi (second quoted string); "" if absent
	// Houses is the system the file names, as Astrolog's own "<Name> houses"
	// header reports it (the specification C2). HasHouses is false when the
	// file names none ("Null houses"), or Astrolog printed no header or one this
	// build's table does not know; Houses is then zero and means nothing.
	Houses    recipe.HouseSystem
	HasHouses bool
	// Nodes is the form of the Nodes and Lilith the file asks for, walked over
	// its switches in file order (FileNodeForm); NodesStated is false when it
	// holds none, and Nodes is then the mean form and means nothing.
	Nodes       recipe.NodeForm
	NodesStated bool
}

// jdUnixEpoch is the Julian Day of the Unix epoch (1970-01-01T00:00:00Z).
const jdUnixEpoch = 2440587.5

// chartSwitches is what Astrolog's own reader takes from a chart-info file's
// text: every switch that sets the moment or the place (`-qb`, `-qa`, `-qc`,
// `-qj`, `-zl`), with its arguments, and every name and place a -zi or a -qc
// sets, in file order. Astrolog applies each as it reads it, so the last one stands.
type chartSwitches struct {
	moments []momentSwitch
	zi      [][2]string
}

// momentSwitch is one switch that sets the moment, the place or both: its name
// without the prefix ("qb", "qa", "qc", "qj", "zl") and the arguments Astrolog
// takes for it, or what is left of the line when the line is shorter.
type momentSwitch struct {
	name string
	args []string
}

// momentArgs is how many arguments Astrolog 8.00 takes for each switch that
// sets the moment or the place (astrolog.cpp FProcessSwitches): `-qb` month,
// day, year, time, ST/DT, zone, longitude, latitude; `-qa` the same without
// ST/DT; `-qc` the same as `-qb` with a name and a place after; `-qj` a Julian
// Day; `-zl` a longitude and a latitude.
var momentArgs = map[string]int{"qb": 8, "qa": 7, "qc": 10, "qj": 1, "zl": 2}

// textArgs is how many arguments Astrolog 8.00 takes for each chart-record
// switch whose arguments are free text (astrolog.cpp FProcessSwitches: -zj
// name and place, -zL and -zN a city).
var textArgs = map[string]int{"zj": 2, "zL": 1, "zN": 1}

// scanChartSwitches reads a chart-info file the way FProcessSwitchFile and
// NParseCommandLine do (Astrolog 8.00, io.cpp and astrolog.cpp): the file is
// cut into lines at any control character but the tab, each line is split
// into arguments, and the arguments are walked as switches. Several switches
// can share a line, so `-zi "A" "Paris" -zi "B" "Rome"` is two of them.
//
// Only the moment switches and -zi are read, and each takes the arguments
// Astrolog gives it (momentArgs, and 2), so an argument that looks like a
// switch (a name of `-zi`) is not read as one. The chart-record switches whose arguments are free text —
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
			case momentArgs[w] > 0:
				// Its arguments, or what is left of the line: a line too
				// short for Astrolog still lists, and fails at pick time.
				end := min(i+1+momentArgs[w], len(toks))
				sw.moments = append(sw.moments, momentSwitch{name: w, args: toks[i+1 : end]})
				if w == "qc" && end == i+11 {
					// `-qc` ends with the chart's name and place, as -zi sets
					// them (astrolog.cpp:2128).
					sw.zi = append(sw.zi, [2]string{toks[i+9], toks[i+10]})
				}
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

// lastMoment is the switch that sets the file's moment, the last `-qb`,
// `-qa`, `-qc` or `-qj`; ok is false when the file sets none.
func (sw chartSwitches) lastMoment() (m momentSwitch, ok bool) {
	for i := len(sw.moments) - 1; i >= 0; i-- {
		if sw.moments[i].name != "zl" {
			return sw.moments[i], true
		}
	}
	return momentSwitch{}, false
}

// coordinates is where the file casts, read from its own text: the longitude
// and latitude of the last switch that sets them. ok is false when that
// switch is a `-qj`, which casts at Astrolog's default place, or when the text
// is not one parseCoordinate reads.
func (sw chartSwitches) coordinates() (lon, lat float64, ok bool) {
	for i := len(sw.moments) - 1; i >= 0; i-- {
		m := sw.moments[i]
		at := map[string]int{"qb": 6, "qa": 5, "qc": 6, "zl": 0}
		k, sets := at[m.name]
		if !sets {
			return 0, 0, false
		}
		if len(m.args) < k+2 {
			return 0, 0, false
		}
		lon, okLon := parseCoordinate(m.args[k], 'E', 'W')
		lat, okLat := parseCoordinate(m.args[k+1], 'N', 'S')
		return lon, lat, okLon && okLat
	}
	return 0, 0, false
}

// statedTime is the time field of the switch that sets the moment, "" for a
// `-qj` or a file that sets none.
func (sw chartSwitches) statedTime() string {
	m, ok := sw.lastMoment()
	if !ok || m.name == "qj" || len(m.args) < 4 {
		return ""
	}
	return m.args[3]
}

// isSwitchValue says a token that starts like a switch is a value: a signed
// number such as `-2.5` or `-0:30`.
func isSwitchValue(w string) bool {
	return w != "" && (w[0] >= '0' && w[0] <= '9' || w[0] == '.')
}

// FileSwitchClass is what Sextile does with one kind of switch a chart file
// holds (docs/05-workbench/release-0.1.4/moment-design.md, "The file's own
// settings"). Astrolog reads every one of them; the class says whose choice
// wins and whether the operator is told.
type FileSwitchClass int

const (
	// SwitchRecord: the moment, the place and the labels. Honoured, and shown.
	SwitchRecord FileSwitchClass = iota
	// SwitchStarting: the house system and the zodiac. The chart's starting
	// settings, shown in its frame code; a choice the operator makes wins.
	SwitchStarting
	// SwitchDisplay: how Astrolog prints, and the chart type it prints. Sextile's
	// own pins override them; they change no value, so nothing is said.
	SwitchDisplay
	// SwitchLens: objects, aspects and orbs. The lens decides what is drawn;
	// the operator is told once.
	SwitchLens
	// SwitchCentre: a centre, a plane or a derived chart Sextile has no
	// screen form for. The chart file does not open (ChartRefused.Asks).
	SwitchCentre
	// SwitchShift: a switch that moves the cast moment (`-YzO`, a progression,
	// a delta-T override), acts outside the chart (writes or reads a file, runs
	// a command line, waits for input), or that the table does not name. The
	// chart is refused, and the switch named.
	SwitchShift
)

// ClassifyFileSwitch is the class of one switch token as a chart file writes
// it, prefix included (`-c`, `=s`, `_Yn`). The families are Astrolog 8.00's own
// (astrolog.cpp FProcessSwitches and NProcessSwitchesRare, xscreen.cpp for
// `-X`), recorded in the moment fix's audit. A switch the table does not name
// is SwitchShift: refused, because what Sextile cannot name it cannot vouch for.
func ClassifyFileSwitch(tok string) FileSwitchClass {
	w := tok
	if w != "" && strings.IndexByte("-/_=:", w[0]) >= 0 {
		w = w[1:]
	}
	if w == "" {
		return SwitchShift
	}
	in := func(set ...string) bool {
		for _, x := range set {
			if w == x {
				return true
			}
		}
		return false
	}
	pre := func(set ...string) bool {
		for _, x := range set {
			if strings.HasPrefix(w, x) {
				return true
			}
		}
		return false
	}
	switch {
	// Moves the cast moment or reaches outside the chart: refused.
	case in("Yz1"):
		return SwitchDisplay
	// The Node and Lilith switches never get here: the guard takes them out
	// of the line (takeNodeSwitches) and the form they ask for is read from
	// the whole file (FileNodeForm).
	case in("Yn", "Ynn"):
		return SwitchDisplay
	case pre("Yz", "Yo", "Yq", "YQ", "Y5", "YY", "YB", "Yi", "Ye", "YM", "M", "~", "o", "i", "p", "r", "y", "+", ".", "0", "Q", "XI", "n", "q", ">"):
		return SwitchShift
	case w == "b" || pre("bs", "bj", "bp", "bm", "ba", "bU", "bJ"):
		return SwitchShift
	// What is drawn and found.
	case pre("YA", "YR", "YC", "Yw", "YU", "R", "A", "C", "u", "U"):
		return SwitchLens
	// A centre, a frame or a derived chart Sextile has no screen form for.
	case in("Yn0", "YT", "YV", "Yf", "Yh", "Ym", "Yc", "YE", "Yp", "G", "1", "10", "2", "20", "3", "4", "9", "f") ||
		pre("h", "sr", "Ys", "x", "F", "Y1"):
		return SwitchCentre
	// How Astrolog prints and which chart type: Sextile's pins set these. Each
	// was checked against Astrolog's reader and on a chart to change no body,
	// angle or cusp (tmp/moment-silent-class.md, 2026-11-15). `-YP` (it moves
	// the Part of Fortune) and `-sz -sh -sd -sn` (an offset after them moves
	// every position) are not here: they refuse the chart.
	case pre("b0", "b1", "b2", "Yd", "Yt", "Yv", "Yr", "Ya", "Y8", "YO", "Y0", "YZ", "Yl", "Yb", "Yu", "YS", "YJ", "Yj", "Y7", "YI", "Yk", "YD", "YX", "X",
		"v", "w", "g", "a", "m", "Z", "S", "l", "j", "7", "L", "K", "d", "D", "E", "8", "e", "t", "T", "B", "V", "P", "I", "N", "H", "?", "k", "5") || w == "Y":
		return SwitchDisplay
	case pre("YP", "sz", "sh", "sd", "sn"):
		return SwitchShift
	case pre("c", "s", "z", "q"):
		return SwitchStarting
	}
	return SwitchShift
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
	for _, sp := range astrologSpans(line) {
		toks = append(toks, sp.text)
	}
	return toks
}

// tokenSpan is one argument of a line: its text, and the bytes of the line it
// occupies with its quotes, so that a caller can cut it out and leave every
// other argument exactly as written.
type tokenSpan struct {
	text       string
	start, end int
}

// astrologSpans is astrologTokens with the place of each argument.
func astrologSpans(line string) []tokenSpan {
	var spans []tokenSpan
	space, quote, start, outer := true, byte(0), 0, 0
	for i := 0; i < len(line); i++ {
		c := line[i]
		switch {
		case c == ' ' || c == '\t':
			if !space && quote == 0 {
				spans = append(spans, tokenSpan{line[start:i], outer, i})
				space = true
			}
		case space:
			quote = 0
			if c == '"' || c == '\'' {
				quote = c
			}
			start, outer = i, i
			if quote != 0 {
				start++
			}
			space = false
		case quote != 0 && c == quote && closesQuote(line, i+1):
			spans = append(spans, tokenSpan{line[start:i], outer, i + 1})
			space = true
		}
	}
	if !space {
		spans = append(spans, tokenSpan{line[start:], outer, len(line)})
	}
	return spans
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
// straight from its own text with no process spawned: the date, the
// coordinates and the labels, for the file's row in the chart list.
//
// None of it is the chart. The date is the fields of the switch that sets the
// moment, a label for the row; the moment itself needs Astrolog, at the pick
// (ResolveChartFile). A field Sextile cannot read from the text is left empty,
// and the row still lists: whether the file is a chart is Astrolog's to say.
type ChartFileHeader struct {
	Year       int     // year field of the switch that sets the moment; 0 when there is none to read
	Month, Day int     // month (1-12) and day of the same switch; 0, 0 when they do not form a date
	HasDate    bool    // Year, Month and Day are a date the file states
	Lon, Lat   float64 // decimal degrees, East/North positive, when HasCoords
	HasCoords  bool    // the file states where it casts, in a form Sextile reads
	Name       string  // chart label from -zi (first quoted string); "" if absent
	PlaceLabel string  // place label from -zi (second quoted string); "" if absent
}

// ReadChartFileHeader reads a chart-info file's row in the chart list without
// running Astrolog. It refuses only a file it cannot read or one with no `@AI`
// header. The date is the last `-qb`, `-qa` or `-qc`, when that is the switch
// that sets the moment; a `-qj` gives its Julian Day's UT date in Astrolog's
// calendar, and the coordinates are those of the last switch that sets them
// (the specification C9). The labels are the last -zi switch, so a file lists with the name and
// place it opens with.
func ReadChartFileHeader(path string) (ChartFileHeader, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return ChartFileHeader{}, fmt.Errorf("astrolog: ReadChartFileHeader: %w", err)
	}
	if KindOfPrefix(raw) != FileChart {
		return ChartFileHeader{}, fmt.Errorf("astrolog: ReadChartFileHeader: %q is not an astrolog chart-info file (no @AI header)", path)
	}
	sw := scanChartSwitches(string(raw))
	var h ChartFileHeader
	if m, ok := sw.lastMoment(); ok && m.name != "qj" && len(m.args) >= 3 {
		if year, err := strconv.Atoi(m.args[2]); err == nil {
			h.Year = year
			h.Month, h.Day = qbMonthDay(m.args[0], m.args[1], year)
			h.HasDate = h.Month != 0
		}
	}
	if m, ok := sw.lastMoment(); ok && m.name == "qj" && len(m.args) >= 1 {
		if jd, err := strconv.ParseFloat(m.args[0], 64); err == nil {
			d := AstrologCalendar{}.Label(jd, 0)
			h.Year, h.Month, h.Day, h.HasDate = d.Year, d.Month, d.Day, true
		}
	}
	h.Lon, h.Lat, h.HasCoords = sw.coordinates()
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
// that sets no moment, one astrolog cannot place, or one whose box does not
// have the expected shape.
func ResolveChartFile(ctx context.Context, path string) (ChartFile, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return ChartFile{}, fmt.Errorf("astrolog: ResolveChartFile: %w", err)
	}
	return ResolveChartBytes(ctx, raw, path)
}

// ResolveChartBytes is ResolveChartFile on a record held in memory; name only
// labels its errors. Astrolog reads a private copy outside the charts folder.
func ResolveChartBytes(ctx context.Context, raw []byte, path string) (ChartFile, error) {
	if KindOfPrefix(raw) != FileChart {
		return ChartFile{}, fmt.Errorf("astrolog: ResolveChartFile: %q is not an astrolog chart-info file (no @AI header)", path)
	}
	// Astrolog reads only the guarded copy: the file's chart lines, never a
	// switch that acts outside the chart (GuardChartFile).
	text, notes, err := guardChart(string(raw))
	if err != nil {
		return ChartFile{}, fmt.Errorf("astrolog: ResolveChartFile: %w", err)
	}
	sw := scanChartSwitches(text)
	if _, ok := sw.lastMoment(); !ok {
		return ChartFile{}, fmt.Errorf("astrolog: ResolveChartFile: %q sets no moment", path)
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
	dir, err := privateTempDir("sextile-chartfile-*")
	if err != nil {
		return ChartFile{}, fmt.Errorf("astrolog: ResolveChartFile: %w", err)
	}
	defer func() { _ = os.RemoveAll(dir) }()
	private := filepath.Join(dir, "chart.as")
	if err := os.WriteFile(private, []byte(text), 0o600); err != nil {
		return ChartFile{}, fmt.Errorf("astrolog: ResolveChartFile: %w", err)
	}
	// The reform date is pinned BEFORE -i: the file's own -qb fields are turned
	// into a Julian Day by the rule in force when they are read, and a host
	// astrolog.as that moved it would move a library chart's moment
	// (the specification C6). It is the one exempt call that carries the pin.
	argv := append([]string{"-c", "22"}, reformPin...)
	argv = append(argv, "-i", private, flagMainShow, "-w")
	// The display pins come AFTER -i, so a display switch in the file (`_Yt`,
	// `-b2`, `-Yz1`) cannot change the form the box is read in: seconds shown
	// (`=b0`), never hidden when zero (`_b2`) nor in thousandths (`_b1`), a
	// 24-hour clock, month first (`=Yt _Yd`), and the zone as a zone with its
	// daylight word (`_Yz1`). They also zero a host's time additions (boxPins).
	argv = append(argv, boxPins...)
	out, _, err := spawn(ctx, newExempt("chart-file read: -c 22 -i <path> -R0 -w <box pins> reads a chart RECORD (moment, place, house system) from the centre box; -R0 restricts every object, so it prints no bodies or aspects", argv...))
	if err != nil {
		return ChartFile{}, fmt.Errorf("astrolog: ResolveChartFile: %w", err)
	}
	_, placeLabel := sw.labels()
	box, err := readCentreBox(out, placeLabel != "")
	if err != nil {
		return ChartFile{}, fmt.Errorf("astrolog: ResolveChartFile: the chart box for %q was not understood: %w", path, err)
	}

	cf := ChartFile{
		JD:        box.jd,
		WhenUTC:   julianDayToUTC(box.jd),
		Clock:     box.clock,
		HouseName: box.houses,
		Record:    text,
		Notes:     notes,
		// The time field of the switch that sets the moment, with two colons.
		StatedSeconds: statesSeconds(sw.statedTime()),
		Lon:           box.lon, // East positive
		Lat:           box.lat, // North positive
	}
	cf.Name, cf.PlaceLabel = sw.labels()
	cf.Nodes, cf.NodesStated = FileNodeForm(string(raw))
	if strings.HasSuffix(box.houses, substituteHouses) {
		// Astrolog casts Porphyry in place of a system it cannot cast at the
		// file's place, and the box names the one it cast. The same read at
		// 0°N 0°E, where every system is defined, names the one the file
		// asks for, by Astrolog's own reading of its -c. Only that name is read
		// from it: for a file in local mean time the moment moves with the
		// place, and no house system's name depends on the moment.
		asked := append(append([]string(nil), argv[:len(argv)-len(boxPins)]...), "-zl", "0:00E", "0:00N")
		asked = append(asked, boxPins...)
		if out, _, err := spawn(ctx, newExempt("chart-file read at 0N 0E: the house system the file names, which Astrolog may not cast at the file's own place", asked...)); err == nil {
			if at, err := readCentreBox(out, placeLabel != ""); err == nil && at.houses != box.houses {
				cf.HouseNameAsked = at.houses
			}
		}
	}
	name := cf.HouseName
	if cf.HouseNameAsked != "" {
		name = cf.HouseNameAsked
	}
	cf.Houses, cf.HasHouses = recipe.ParseHouseSystem(name)
	return cf, nil
}

// substituteHouses is the system Astrolog casts in place of one it cannot
// cast at a place: Placidus and Koch inside the polar circle, and any system
// the Swiss Ephemeris fails on (calc.cpp, ComputeHouses and its Swiss path).
// In 3D houses the box names it "3D Porphyry".
const substituteHouses = "Porphyry"

var (
	// reBoxBorder is the inner border of the centre box: a house cusp label,
	// a run of dashes between two bars, a house cusp label. Only Astrolog's own
	// border lines begin with `<n>`, so a name cannot spell one: it prints
	// inside a row that starts with a bar.
	reBoxBorder = regexp.MustCompile(`^<[0-9]+>[^|]*\|-+\|[^|]*<[0-9]+>$`)
	reBoxJD     = regexp.MustCompile(`^Julian Day:\s*(-?[0-9]+\.[0-9]+)$`)
	// DT Zone 1:00E,   4:53'41E 52:22'07N — the daylight word, the zone in
	// Astrolog's words and the place to the arc-second. boxPins fix this form
	// after the file is read (`_Yz1` keeps the daylight word, `_b2` the zero
	// seconds), so any other form is refused.
	reBoxZone = regexp.MustCompile(`^(ST|DT) Zone ([^,]*),\s+([0-9]+:[0-9]+'[0-9]+[EW])\s+([0-9]+:[0-9]+'[0-9]+[NS])$`)
	// Thu Dec 27 1571 14:37:00 — the box's date row under boxPins
	// (charts1.cpp PrintWheelCenter case 1): weekday, month, day, year in
	// Astrolog's calendar, and the clock to the second.
	reBoxDate = regexp.MustCompile(`^([A-Z][a-z]{2}) ([A-Z][a-z]{2}) +([0-9]{1,2}) +(-?[0-9]+) +([0-9]{1,2}):([0-9]{2}):([0-9]{2})$`)
	// The UT and zodiac rows carry no value the reader keeps; their shape is
	// checked so a box whose rows moved is refused rather than read at the
	// wrong count (charts1.cpp PrintWheelCenter cases 5 and 7). A progressed
	// chart prints "Prog: ..." in the UT row and is refused: its Julian Day is
	// not the birth moment.
	reBoxUT     = regexp.MustCompile(`^UT: +([0-9]+:[0-9]+:[0-9]+), Sid\.T: +[0-9]+:[0-9]+:[0-9]+$`)
	reBoxZodiac = regexp.MustCompile(`^(?:Tropical|Sidereal|Sidereal Inv), [A-Z][A-Za-z]*(?: [A-Za-z]+)*$`)
)

// BoxClock is a chart's moment as Astrolog's centre box prints it.
type BoxClock struct {
	Weekday, Month string // "Thu", "Dec"
	Day, Year      int    // the day and the year in Astrolog's calendar (Julian before 15 Oct 1582)
	Hour, Minute   int
	Second         int
	DST            string // "ST" or "DT", the word before "Zone"
	Zone           string // as the box prints it: "LMT", "1:00E", "5:30E", "0:09E"
	UT             string // "14:01:32"
}

// boxPins fix the form of the centre box readCentreBox reads, and zero the two
// time additions a host astrolog.as may hold: `-YzO` would move the Julian Day
// the box prints (calc.cpp:1242, 1269), `-YzC` the cusps.
var boxPins = []string{"=b0", "_b1", "_b2", "=Yt", "_Yd", "_Yz1", "-YzO", "0", "-YzC", "0"}

// centreBox is what ResolveChartFile takes from Astrolog's centre box.
type centreBox struct {
	jd       float64
	lon, lat float64
	houses   string // Astrolog's name for the system, "Null" when the file names none
	clock    BoxClock
}

// readCentreBox reads the `-w =b0` centre box by position. Counted upward from
// its lower inner border, the lines are fixed (Astrolog 8.00, probed
// 2026-10-11): 1 the Julian Day, 2 the zodiac and centre, 3 the house system,
// 4 the UT and sidereal time, 5 the zone, longitude and latitude. Above them
// stand the place (only when the file names one), the date with the clock, the
// name and the title. The date is read at its place, counted the same way; the
// place, the name and the title are never read, so no label can pass for a
// value.
//
// The box has nine rows and prints no row for an empty name or place, so a
// chart with either has blank rows between the Julian Day and the border (the
// spec's probe had both labels). Those padding rows are skipped first; the
// count starts at the lowest row that has text.
//
// A box of another shape is refused whole: a missing border, a line that is not
// the kind it should be — all five counted lines are checked, the two whose
// values are not kept included. Nothing is guessed.
func readCentreBox(out string, hasPlace bool) (centreBox, error) {
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
		m := row.re.FindStringSubmatch(c)
		if m == nil {
			return centreBox{}, fmt.Errorf("line %d above the border is %q, not %s line", row.n, c, row.kind)
		}
		if row.n == 4 {
			box.clock.UT = m[1]
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
	box.clock.DST, box.clock.Zone = z[1], strings.TrimSpace(z[2])
	lon, okLon := parseCoordinate(z[3], 'E', 'W')
	lat, okLat := parseCoordinate(z[4], 'N', 'S')
	if !okLon || !okLat {
		return centreBox{}, fmt.Errorf("the coordinates %q %q are not read", z[3], z[4])
	}
	box.lon, box.lat = lon, lat
	// The date row stands above the zone row, with the place row between them
	// when the file names a place (an empty label prints no row).
	n := 6
	if hasPlace {
		n = 7
	}
	date, err := cell(n)
	if err != nil {
		return centreBox{}, err
	}
	d := reBoxDate.FindStringSubmatch(date)
	if d == nil {
		return centreBox{}, fmt.Errorf("line %d above the border is %q, not a date with a clock", n, date)
	}
	box.clock.Weekday, box.clock.Month = d[1], d[2]
	box.clock.Day, _ = strconv.Atoi(d[3])
	box.clock.Year, _ = strconv.Atoi(d[4])
	box.clock.Hour, _ = strconv.Atoi(d[5])
	box.clock.Minute, _ = strconv.Atoi(d[6])
	box.clock.Second, _ = strconv.Atoi(d[7])
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
	zone, err := zoneArgs(m.JD, m.TZ, m.Lon)
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
// the nearest second. The box prints the Julian Day to 7 decimals (8.6 ms). A
// -qb clock and a numeric zone are whole seconds, so for them the nearest second
// is the file's own moment. LMT is longitude/15, and an arc-second of longitude
// is 1/15 of a second, so there the nearest second is within half a second.
func julianDayToUTC(jd float64) time.Time {
	sec := (jd - jdUnixEpoch) * 86400.0
	return time.Unix(int64(math.Round(sec)), 0).UTC()
}

// parseCoordinate reads a longitude (pos 'E', neg 'W') or a latitude (pos
// 'N', neg 'S') in the spellings Astrolog's chart files use: the hemisphere
// letter last (`4:53E`, `4:53'20E`, `4.88E`) or between degrees and minutes
// (`4E53`, `4E53:20`), in either case. ok is false for anything else.
// compactMMSS is the minutes and seconds Astrolog reads from four digits
// after a colon with nothing after them, as `30:3045` for 30°30'45" (io.cpp
// RParseSz, the Astrodienst form).
func compactMMSS(after string) (minutes, seconds float64, ok bool) {
	if len(after) != 4 {
		return 0, 0, false
	}
	n, err := strconv.Atoi(after)
	if err != nil || after[0] == '+' || after[0] == '-' {
		return 0, 0, false
	}
	return float64(n / 100), float64(n % 100), true
}

// statesSeconds says whether a clock as written carries its seconds:
// `16:30:45`, or the compact `16:3045`.
func statesSeconds(clock string) bool {
	if strings.Count(clock, ":") == 2 {
		return true
	}
	at := strings.IndexByte(clock, ':')
	if at < 0 {
		return false
	}
	end := at + 1
	for end < len(clock) && clock[end] >= '0' && clock[end] <= '9' {
		end++
	}
	_, _, ok := compactMMSS(clock[at+1 : end])
	return ok && end-at-1 == 4
}

func parseCoordinate(tok string, pos, neg byte) (float64, bool) {
	t := strings.ToUpper(tok)
	at := strings.IndexAny(t, string([]byte{pos, neg}))
	if at <= 0 {
		return 0, false
	}
	sign := 1.0
	if t[at] == neg {
		sign = -1
	}
	var parts []string
	if at == len(t)-1 {
		parts = strings.Split(strings.ReplaceAll(t[:at], "'", ":"), ":")
	} else {
		parts = append([]string{t[:at]}, strings.Split(strings.ReplaceAll(t[at+1:], "'", ":"), ":")...)
	}
	if len(parts) > 3 {
		return 0, false
	}
	if len(parts) == 2 {
		if m, sec, ok := compactMMSS(parts[1]); ok {
			parts = []string{parts[0], strconv.FormatFloat(m, 'f', -1, 64), strconv.FormatFloat(sec, 'f', -1, 64)}
		}
	}
	var v float64
	for i, p := range parts {
		x, err := strconv.ParseFloat(p, 64)
		if err != nil || x < 0 || i > 0 && x >= 60 {
			return 0, false
		}
		v += x / math.Pow(60, float64(i))
	}
	return sign * v, true
}
