package lookup

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/cruiserxyz/sextile/internal/astrolog"
	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/recipe"
	"github.com/cruiserxyz/sextile/internal/store"
)

// ClockPref is the operator's clock preference (the specification C10): the
// hour form and the date order Astrolog is asked to print. The zero value is
// the default, 24-hour and month first.
type ClockPref struct {
	Hours12  bool // 12-hour with am/pm (default: 24-hour)
	DayFirst bool // day before month (default: month first)
}

// HoldClock holds the preference for the session. It is called once, with the
// startup read, before the first Astrolog call; every call after it pins this
// clock, and the timeline parser reads only this form. The perimeter reaches
// the compiler's display pins through here and nowhere else.
func HoldClock(p ClockPref) {
	c := astrolog.Clock{}
	if p.Hours12 {
		c.Hours = astrolog.Hours12
	}
	if p.DayFirst {
		c.Dates = astrolog.DayFirst
	}
	astrolog.SetClock(c)
}

// HeldClock is the preference HoldClock holds, for the one reader that prints
// a moment itself: the ui, whose chart rows and picker dates must spell the
// same characters Astrolog prints (the specification). It reads the value
// held at start; it never re-reads the file.
func HeldClock() ClockPref {
	c := astrolog.CurrentClock()
	return ClockPref{Hours12: c.Hours == astrolog.Hours12, DayFirst: c.Dates == astrolog.DayFirst}
}

// CivilDate is a date in the held clock's order and nothing else — no weekday,
// no clock: `Oct  5, 2026` or ` 5 Oct 2026`, each a fixed width within its
// order. It is spelled from the fields, so a Julian date stays what its source
// says. The ui's chart rows and the body tab's lists both take it from here.
func CivilDate(year, month, day int, p ClockPref) string {
	mon := time.Month(month).String()[:3]
	if p.DayFirst {
		return fmt.Sprintf("%2d %s %04d", day, mon, year)
	}
	return fmt.Sprintf("%s %2d, %04d", mon, day, year)
}

// TimelineMonth is the year and month the timeline asks Astrolog for when the
// floating slot is at t in zone: the month in Astrolog's calendar, which is the
// Julian one before 15 Oct 1582. The ui keys its list by it and TimelineWith
// asks for it, so the two cannot disagree (the specification C8).
func TimelineMonth(t time.Time, zone *time.Location) (year int, month time.Month) {
	return astrolog.TimelineMonth(t, zone)
}

// BeforeJulianDayZero says whether t lies before Julian Day 0, the earliest
// moment Astrolog can name.
func BeforeJulianDayZero(t time.Time) bool { return astrolog.BeforeJulianDayZero(t) }

// MomentStamp is the date and clock Astrolog itself prints in the header of a
// chart cast for a moment (the specification): its weekday, whether the date
// is a Julian one (shown with ` Jul`), and whether the moment is shown in UT
// because the call carries no zone (the 1582 window).
type MomentStamp struct {
	Year, Month, Day int
	Hour, Minute     int
	Weekday          time.Weekday
	Julian, UT       bool
	// Second is shown only with ShowSeconds: a chart whose file states its
	// moment to the second.
	Second      int
	ShowSeconds bool
}

// StampOfClock is the stamp of a chart's own clock, as Astrolog read it from
// the chart's file.
func StampOfClock(c store.ChartClock) MomentStamp {
	return MomentStamp{Year: c.Year, Month: c.Month, Day: c.Day, Hour: c.Hour, Minute: c.Minute, Second: c.Second,
		Weekday: c.Weekday, Julian: c.Julian, ShowSeconds: c.ShowSeconds}
}

// StampMoment is the header Astrolog prints for the instant t, to the second as
// every call for a slot sends it, shown in zone.
func StampMoment(t time.Time, zone *time.Location) MomentStamp {
	s := astrolog.SlotLabel(t, zone)
	return MomentStamp{
		Year: s.Year, Month: s.Month, Day: s.Day, Hour: s.Hour, Minute: s.Minute,
		Weekday: s.Weekday, Julian: s.Julian, UT: s.UT,
	}
}

// ClockOfRead is a chart's own clock as Astrolog read it from the chart's file
// (its centre box): the date in Astrolog's calendar, the clock, the zone in
// Astrolog's words, and the file's place.
func ClockOfRead(cf astrolog.ChartFile) store.ChartClock {
	b := cf.Clock
	mon := 0
	for m := time.January; m <= time.December; m++ {
		if m.String()[:3] == b.Month {
			mon = int(m)
		}
	}
	wd := time.Sunday
	for d := time.Sunday; d <= time.Saturday; d++ {
		if d.String()[:3] == b.Weekday {
			wd = d
		}
	}
	julian := b.Year < 1582 || b.Year == 1582 && (mon < 10 || mon == 10 && b.Day < 15)
	houses := cf.HouseName
	if houses == "Null" {
		houses = ""
	}
	set, sidereal, offset := astrolog.FileZodiac(cf.Record)
	return store.ChartClock{Year: b.Year, Month: mon, Day: b.Day, Hour: b.Hour, Minute: b.Minute, Second: b.Second,
		Weekday: wd, Julian: julian, ShowSeconds: cf.StatedSeconds, Zone: ZoneCell(b.DST, b.Zone),
		Place: cf.PlaceLabel, Coords: CoordinateWords(cf.Lon, cf.Lat), Lon: cf.Lon, Lat: cf.Lat,
		Houses: houses, HousesAsked: cf.HouseNameAsked, ZodiacSet: set, Sidereal: sidereal, Offset: offset}
}

// ZoneCell is a chart's zone as the Chart panel shows it, from the zone and
// daylight word of Astrolog's chart header: the clock's offset from Greenwich
// in hours, with minutes when it has them ("+1", "+5:30", "-3", "+0"). In
// daylight time the hour is added once, here, and "DT" follows ("+2 DT").
// Astrolog prints any other daylight shift as "AT" with no amount, so that
// cell is the zone's own number and "AT". "LMT" and "LAT" stay words.
func ZoneCell(dst, zone string) string {
	if zone == "LMT" || zone == "LAT" {
		return zone
	}
	n := len(zone)
	if n < 2 || (zone[n-1] != 'E' && zone[n-1] != 'W') {
		return zone
	}
	h, m, ok := hoursMinutes(zone[:n-1])
	if !ok {
		return zone
	}
	minutes := h*60 + m
	if zone[n-1] == 'W' {
		minutes = -minutes
	}
	switch dst {
	case "DT":
		return OffsetCell(minutes*60+3600) + " DT"
	case "AT":
		return OffsetCell(minutes*60) + " AT"
	}
	return OffsetCell(minutes * 60)
}

// hoursMinutes reads Astrolog's "H" or "H:MM".
func hoursMinutes(s string) (h, m int, ok bool) {
	hs, ms, colon := strings.Cut(s, ":")
	h, err := strconv.Atoi(hs)
	if err != nil || h < 0 {
		return 0, 0, false
	}
	if colon {
		if m, err = strconv.Atoi(ms); err != nil || m < 0 || m > 59 {
			return 0, 0, false
		}
	}
	return h, m, true
}

// DaylightHour says whether t's clock is its zone's standard time plus one
// hour, the rule a saved chart's DT flag follows.
func DaylightHour(t time.Time) bool { return astrolog.DaylightHour(t) }

// OffsetCell is an offset from Greenwich in seconds in the Chart panel's one
// notation: a sign, the hours, then minutes and seconds only where the
// offset has them ("+1", "-4", "+5:30", "+1:19:32", "+0").
func OffsetCell(seconds int) string {
	sign := "+"
	if seconds < 0 {
		sign, seconds = "-", -seconds
	}
	h, m, sec := seconds/3600, seconds%3600/60, seconds%60
	switch {
	case sec != 0:
		return fmt.Sprintf("%s%d:%02d:%02d", sign, h, m, sec)
	case m != 0:
		return fmt.Sprintf("%s%d:%02d", sign, h, m)
	}
	return fmt.Sprintf("%s%d", sign, h)
}

// CoordinateWords is a place given by its coordinates, East and North
// positive: "8°52'E 48°45'N", with seconds where the place has them.
func CoordinateWords(lon, lat float64) string {
	part := func(v float64, pos, neg string) string {
		h := pos
		if v < 0 {
			h, v = neg, -v
		}
		sec := int(v*3600 + 0.5)
		d, m, s := sec/3600, sec%3600/60, sec%60
		if s != 0 {
			return fmt.Sprintf("%d°%02d'%02d\"%s", d, m, s, h)
		}
		return fmt.Sprintf("%d°%02d'%s", d, m, h)
	}
	return part(lon, "E", "W") + " " + part(lat, "N", "S")
}

// HouseTag is the row tag of a house system Astrolog cast, from the name it
// gives it in a chart header (operator, 2026-11-15, "Short marks"): the
// approved two-letter code for the 22 offered systems, `3` before the code for
// 3D houses, and `#` with Astrolog's number for any other ("#23").
func HouseTag(name string) string {
	if h, ok := recipe.ParseHouseSystem(name); ok {
		return HouseCode(h)
	}
	if rest, ok := strings.CutPrefix(name, "3D "); ok {
		return "3" + HouseTag(rest)
	}
	if n, ok := astrolog.HouseSystemNumber(name); ok {
		return fmt.Sprintf("#%d", n)
	}
	return "#?"
}

// ZodiacOfOffset is the zodiac a sidereal flag and offset name: tropical, or
// the named ayanamsa with exactly that offset; false for an offset none has.
func ZodiacOfOffset(sidereal bool, offset float64) (recipe.Zodiac, bool) {
	if !sidereal {
		return recipe.Tropical, true
	}
	for z := recipe.Zodiac(0); int(z) < recipe.ZodiacCount; z++ {
		if off, ok := astrolog.AyanamsaOffset(z); ok && z != recipe.Tropical && math.Abs(off-offset) < 1e-6 {
			return z, true
		}
	}
	return 0, false
}

// ZodiacTag is the row tag of a zodiac Astrolog cast: the approved two-letter
// code of tropical or a named ayanamsa, else the sidereal offset from
// Fagan-Bradley in signed degrees ("+2.5").
func ZodiacTag(sidereal bool, offset float64) string {
	if z, ok := ZodiacOfOffset(sidereal, offset); ok {
		return ZodiacCode(z)
	}
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%+.4f", offset), "0"), ".")
}

// PlaceZone is the zone a moment at place is shown in at t (astrolog.NowZone):
// the place's IANA zone, or before its standard time the local mean time of
// the place's own longitude, named LMT, as Astrolog has it. A zone that does
// not load is UTC, and err says so.
func PlaceZone(t time.Time, place location.PlaceInfo) (*time.Location, error) {
	loc, err := astrolog.NowZone(t, place.TZ, place.Lon)
	if err != nil {
		return time.UTC, err
	}
	return loc, nil
}

// StepMoment is t moved by years, months and days of Astrolog's calendar at
// place's wall clock (Julian before 15 October 1582), then by dur of elapsed
// time. A calendar step that cannot be made leaves t where it is.
func StepMoment(t time.Time, place location.PlaceInfo, years, months, days int, dur time.Duration) time.Time {
	if years != 0 || months != 0 || days != 0 {
		zone, _ := PlaceZone(t, place)
		if stepped, err := (astrolog.AstrologCalendar{}).Step(t, zone, years, months, days); err == nil {
			t = stepped
		}
	}
	return t.Add(dur)
}

// GregorianDay is the day a date as Astrolog reads it falls on in Go's
// (proleptic Gregorian) calendar: a date before 15 October 1582 is Julian, so
// Julian 27 December 1571 is Gregorian 6 January 1572. ok is false for a date
// Astrolog's reader refuses.
func GregorianDay(year, month, day int) (y int, m time.Month, d int, ok bool) {
	p, err := astrolog.AstrologCalendar{}.Parse(astrolog.Fields{Year: year, Month: month, Day: day}, 0)
	if err != nil {
		return 0, 0, 0, false
	}
	y, m, d = p.Instant.Date()
	return y, m, d, true
}
