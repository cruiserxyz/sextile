package lookup

import (
	"fmt"
	"time"

	"github.com/cruiserxyz/sextile/internal/astrolog"
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
