package astrolog

import (
	"errors"
	"fmt"
	"math"
	"time"
)

// This file is the one place Sextile names a calendar date, and it is the
// recorded exception to the rule that Astrolog-owned facts are extracted from
// Astrolog's output and never cloned (the specification, § The two calendar
// functions). Astrolog names a date before October 1582 in the Julian
// calendar and Go's time package is proleptic Gregorian, so Sextile hands
// Astrolog only Julian Days and copies exactly two of Astrolog 8.00's rules to
// SHOW and to READ the dates it names. Each function cites the source it
// copies; TestLabelMatchesAstrolog and TestParseMatchesAstrolog hold both
// against the live binary.
//
// The type names a calendar so another one (Old Style England, a Lady Day year)
// can be added later; none is built. Year numbering is astronomical, as
// Astrolog prints it: year 0 is 1 BC, year -1 is 2 BC.

// AstrologCalendar is Astrolog 8.00's calendar: Julian before its reform date,
// Gregorian from it, with the two reform dates its two rules use.
type AstrologCalendar struct{}

const (
	// jdnReform is the Julian Day number (noon-based) of 15 Oct 1582 Gregorian,
	// the day Astrolog's reader treats as the first Gregorian one (calc.cpp:76,
	// data.cpp:173 ciGreg = 10/15/1582).
	jdnReform = 2299161
	// jdLabelGregorian is the threshold Astrolog's PRINTER uses (calc.cpp:120,
	// JD >= 2299171.0): ten days after jdnReform, so the header names the
	// Julian dates 5-14 Oct 1582 (Astrolog NEXT.md BUG 3).
	jdLabelGregorian = 2299171.0
	// jdnOfUnixDayZero turns a Unix day into a Julian Day number:
	// JDN = floor(unixSec/86400) + 2440588 (the noon number of that civil day).
	jdnOfUnixDayZero = 2440588
	// jdZeroUnix is Unix second of Julian Day 0.
	jdZeroUnix = -210866760000
)

// ErrBeforeJulianDayZero means a moment lies before Julian Day 0, which
// Astrolog's date arithmetic cannot name (the spec's C10).
var ErrBeforeJulianDayZero = errors.New("astrolog: the moment is before Julian Day 0")

// CalendarDate is what Astrolog prints for a chart moment: the date and clock
// fields, the weekday and the branch the date came from.
type CalendarDate struct {
	Year, Month, Day int
	Hour, Minute     int
	Weekday          time.Weekday
	// Julian is the branch the date was named on, never a comparison with 15
	// Oct 1582: the ` Jul` mark follows it.
	Julian bool
}

// Label is what Astrolog PRINTS for a chart moment (rule a): the header date,
// weekday and time of `-qj <jdUT>` followed by `-zZ` for a zone offset seconds
// EAST of Greenwich (Astrolog's own -zZ takes it negated: -zZ -5 0 is five
// hours east, and it comes AFTER -qj, which resets the zone, astrolog.cpp:2096).
//
// It copies, in order:
//   - the `-qj` rounding and the Julian Day to date, astrolog.cpp:2092-2095:
//     JD+0.5, the fraction is the UT clock, the integer part names the day,
//     JulianToMdy with Gregorian from JD 2299171.0 (calc.cpp:120);
//   - the zone shift as field arithmetic: AdjustTimeZone (general.cpp:906-922)
//     keeps the whole hours and AdjustTime (atlas.cpp:1763) carries them into
//     the date, with month lengths from DayInMonth, which applies the Julian
//     leap rule to EVERY year up to 1582 whichever branch is in use
//     (general.cpp:740-741);
//   - the clock as SzTim prints it: hours, then the minute truncated, with
//     rSmall added first (general.cpp:2001-2006);
//   - the weekday as DayOfWeek computes it (general.cpp:767), the READ rule
//     applied to those fields, so inside the 1582 window it can differ from
//     the instant's real weekday.
func (AstrologCalendar) Label(jdUT float64, offset int) CalendarDate {
	jd := jdUT + 0.5 // astrolog.cpp:2092, rRound
	frac := jd - math.Floor(jd)
	day := jd - frac
	julian := day < jdLabelGregorian
	y, m, d := civilFromJDN(int64(day), !julian)
	tim := frac * 24.0

	// AdjustTimeZone: tim += off1 - off2 with off1 = 0 and off2 = -east.
	tim += float64(offset) / 3600.0
	secs := int(math.Floor(tim)) * 3600
	y, m, d, secs = adjustTime(y, m, d, secs)
	tim = float64(secs)/3600.0 + (tim - math.Floor(tim))

	// SzTim.
	t := math.Abs(tim) + 1.7453e-09
	hour := int(math.Floor(t))
	minute := int((t - math.Floor(t)) * 60.0)
	for minute >= 60 {
		minute -= 60
		hour++
	}
	for hour < 0 {
		hour += 24
	}
	for hour >= 24 {
		hour -= 24
	}
	return CalendarDate{
		Year: y, Month: m, Day: d, Hour: hour, Minute: minute,
		Weekday: dayOfWeek(m, d, y), Julian: julian,
	}
}

// adjustTime is AdjustTime (atlas.cpp:1763): the seconds are brought within a
// day, carrying into the day, and the day within its month, carrying into the
// month and year. Month lengths come from dayInMonth.
func adjustTime(y, m, d, secs int) (int, int, int, int) {
	for secs < 0 {
		secs += 3600 * 24
		d--
	}
	for secs >= 3600*24 {
		secs -= 3600 * 24
		d++
	}
	for d < 1 {
		m--
		for m < 1 {
			m += 12
			y--
		}
		d += dayInMonth(m, y)
	}
	for {
		cd := dayInMonth(m, y)
		if d <= cd {
			break
		}
		m++
		for m > 12 {
			m -= 12
			y++
		}
		d -= cd
	}
	return y, m, d, secs
}

// dayInMonth is DayInMonth (general.cpp:731): February has 29 days in every
// year divisible by 4 up to and including 1582 (the Julian rule), and by the
// Gregorian rule after it.
func dayInMonth(month, year int) int {
	switch month {
	case 9, 4, 6, 11:
		return 30
	case 2:
		if year%4 == 0 && (year%100 != 0 || year%400 == 0 || year <= 1582) {
			return 29
		}
		return 28
	}
	return 31
}

// dayOfWeek is DayOfWeek (general.cpp:767): (MdyToJulian + 1) mod 7, Sunday 0,
// with MdyToJulian's reader branch (calc.cpp:66-89).
func dayOfWeek(month, day, year int) time.Weekday {
	n := mdyToJulian(month, day, year)
	w := (n + 1) % 7
	if w < 0 {
		w += 7
	}
	return time.Weekday(w)
}

// readsGregorian says whether Astrolog's reader takes a date as Gregorian:
// not before 15 Oct 1582 (calc.cpp:76-78).
func readsGregorian(month, day, year int) bool {
	return !(year < 1582 || (year == 1582 && (month < 10 || (month == 10 && day < 15))))
}

// mdyToJulian is MdyToJulian (calc.cpp:66): the Julian Day number (the noon
// number of the day) of a date as Astrolog reads it.
func mdyToJulian(month, day, year int) int64 {
	a := int64(14-month) / 12
	y := int64(year) + 4800 - a
	m := int64(month) + 12*a - 3
	n := int64(day) + (153*m+2)/5 + 365*y + floorDiv(y, 4)
	if readsGregorian(month, day, year) {
		return n - floorDiv(y, 100) + floorDiv(y, 400) - 32045
	}
	return n - 32083
}

// civilFromJDN is JulianToMdy for a whole Julian Day number: the date of the
// day whose noon is n, in the Gregorian or the Julian calendar.
func civilFromJDN(n int64, gregorian bool) (year, month, day int) {
	var b, c int64
	if gregorian {
		a := n + 32044
		b = (4*a + 3) / 146097
		c = a - 146097*b/4
	} else {
		b = 0
		c = n + 32082
	}
	d := (4*c + 3) / 1461
	e := c - 1461*d/4
	m := (5*e + 2) / 153
	day = int(e - (153*m+2)/5 + 1)
	month = int(m + 3 - 12*(m/10))
	if gregorian {
		year = int(100*b + d - 4800 + m/10)
	} else {
		year = int(d - 4800 + m/10)
	}
	return year, month, day
}

func floorDiv(a, b int64) int64 {
	q := a / b
	if (a%b != 0) && ((a < 0) != (b < 0)) {
		q--
	}
	return q
}

// Fields are the calendar fields of a printed event: what a search row says.
type Fields struct {
	Year, Month, Day, Hour, Minute int
}

// Parsed is what Astrolog's reader makes of a set of fields.
type Parsed struct {
	Instant time.Time // the absolute instant, in UTC
	Weekday time.Weekday
	// Julian is the branch the reader took: the fields name a date before 15
	// Oct 1582.
	Julian bool
}

// Parse is what Astrolog READS and what its searches step through (rule b): the
// fields of an event printed by `-t`, `-tr` or `-dm` at a zone offset seconds
// east, as an instant. It copies MdyToJulian with the reform date of 15 Oct
// 1582 (calc.cpp:66-89, data.cpp:173) and AddDay's skip from 4 to 15 Oct
// (general.cpp:786): the days 5-14 Oct 1582 are read as Julian dates and a
// search never prints them. Its inverse, MonthOf, names the calendar month a
// moment lies in.
//
// A month, day (DayInMonth: the Julian leap rule up to 1582), hour or minute
// that is out of range is an error: time.Date would have normalised it into
// another date.
func (AstrologCalendar) Parse(f Fields, offset int) (Parsed, error) {
	if f.Month < 1 || f.Month > 12 {
		return Parsed{}, fmt.Errorf("impossible date %d/%d/%d", f.Month, f.Day, f.Year)
	}
	if f.Day < 1 || f.Day > dayInMonth(f.Month, f.Year) {
		return Parsed{}, fmt.Errorf("impossible date %d/%d/%d", f.Month, f.Day, f.Year)
	}
	if f.Hour < 0 || f.Hour > 23 || f.Minute < 0 || f.Minute > 59 {
		return Parsed{}, fmt.Errorf("impossible clock time %d:%02d", f.Hour, f.Minute)
	}
	n := mdyToJulian(f.Month, f.Day, f.Year)
	sec := (n-jdnOfUnixDayZero)*86400 + int64(f.Hour)*3600 + int64(f.Minute)*60 - int64(offset)
	return Parsed{
		Instant: time.Unix(sec, 0).UTC(),
		Weekday: dayOfWeek(f.Month, f.Day, f.Year),
		Julian:  !readsGregorian(f.Month, f.Day, f.Year),
	}, nil
}

// ParseIn is Parse for a wall clock in a zone rather than at a fixed offset: the
// instant is resolved the way Go's time.Date resolves a wall clock in zone (a
// historical zone's local mean time, a daylight shift), and returned in zone.
func (c AstrologCalendar) ParseIn(f Fields, zone *time.Location) (Parsed, error) {
	p, err := c.Parse(f, 0)
	if err != nil {
		return Parsed{}, err
	}
	u := p.Instant
	p.Instant = time.Date(u.Year(), u.Month(), u.Day(), u.Hour(), u.Minute(), 0, 0, zone)
	return p, nil
}

// MonthOf is Parse's inverse for a month: the year and month Astrolog's reader
// would write for the local day of t at offset seconds east.
func (AstrologCalendar) MonthOf(t time.Time, offset int) (int, time.Month) {
	n := floorDiv(t.Unix()+int64(offset), 86400) + jdnOfUnixDayZero
	y, m, _ := civilFromJDN(n, n >= jdnReform)
	return y, time.Month(m)
}

// MonthBounds is the half-open instant interval [start, end) of a calendar
// month in zone, in Astrolog's calendar: the first of the month at 0:00 local
// to the first of the next. A wall clock inside a zone's gap or overlap is
// resolved the way Go's time.Date does.
func (c AstrologCalendar) MonthBounds(year int, month time.Month, zone *time.Location) (start, end time.Time, err error) {
	first, err := c.ParseIn(Fields{Year: year, Month: int(month), Day: 1}, zone)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	ny, nm := year, int(month)+1
	if nm > 12 {
		ny, nm = year+1, 1
	}
	next, err := c.ParseIn(Fields{Year: ny, Month: nm, Day: 1}, zone)
	return first.Instant, next.Instant, err
}

// jdWindow reports the 1582 window (Astrolog NEXT.md BUG 2): a UT instant in
// [JD 2299170.5 - east/24, 2299170.5) with a zone east of Greenwich. Asked for
// the local clock, Astrolog's field arithmetic carries the date into 15 Oct
// and its reader then takes it as Gregorian, casting the chart ten days early.
func jdWindow(jd float64, offset int) bool {
	return offset > 0 && jd >= 2299170.5-float64(offset)/86400.0 && jd < 2299170.5
}

// SlotStamp is what Sextile shows for a slot's moment: Astrolog's own header
// date and clock, and whether the moment is shown in UT (inside the 1582
// window, where no zone is sent).
type SlotStamp struct {
	CalendarDate
	UT bool
}

// SlotLabel is the stamp for the instant t (to the second, as every call for a
// slot sends it) in zone: Label of the Julian Day Sextile sends, at the zone's
// offset at t, or at UT inside the 1582 window.
func SlotLabel(t time.Time, zone *time.Location) SlotStamp {
	sec := t.Truncate(time.Second).Unix()
	jd := sentJD(sec)
	_, off := t.In(zone).Zone()
	if jdWindow(jd, off) {
		return SlotStamp{CalendarDate: AstrologCalendar{}.Label(jd, 0), UT: true}
	}
	return SlotStamp{CalendarDate: AstrologCalendar{}.Label(jd, off)}
}

// TimelineMonth is the month a timeline asks Astrolog for when the floating
// slot is at t in zone: Parse's inverse for the zone's local day, in Astrolog's
// calendar (a Julian 29 February 1500 is February, whatever Go's calendar says).
func TimelineMonth(t time.Time, zone *time.Location) (int, time.Month) {
	_, off := t.In(zone).Zone()
	return AstrologCalendar{}.MonthOf(t, off)
}

// BeforeJulianDayZero says whether t lies before Julian Day 0, the earliest
// moment Astrolog's date arithmetic can name: no chart or search is asked for
// such a moment (the specification C10).
func BeforeJulianDayZero(t time.Time) bool { return t.Unix() < jdZeroUnix }

// refuseBeforeJD0 is the one refusal of a moment before Julian Day 0.
func refuseBeforeJD0(sec int64) error {
	if sec < jdZeroUnix {
		return fmt.Errorf("%w", ErrBeforeJulianDayZero)
	}
	return nil
}
