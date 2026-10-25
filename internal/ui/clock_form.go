package ui

import (
	"fmt"
	"time"

	"github.com/cruiserxyz/sextile/internal/lookup"
)

// The characters Sextile prints for a moment itself (the specification): the
// chart rows and the chart picker spell what Astrolog prints under the same
// preference (the specification), so one instant reads the same on every
// part of the screen. Astrolog space-pads the day and the hour to two cells
// and Go has no space-padded hour, so both are padded by hand.
//
//	24-hour, month first   Mon Oct  5, 2026  9:05
//	24-hour, day first     Mon  5 Oct 2026  9:05
//	12-hour, month first   Mon Oct  5, 2026  9:05am
//	12-hour, day first     Mon  5 Oct 2026  9:05am
//
// In the 12-hour form midnight is 12:05am and noon 12:05pm.

// clockMoment is t in the held clock form: weekday, date and clock, no zone.
func clockMoment(t time.Time) string {
	return clockMomentIn(t, lookup.HeldClock())
}

func clockMomentIn(t time.Time, p lookup.ClockPref) string {
	// The date is the one Astrolog prints for the moment (the specification):
	// Julian before 15 Oct 1582, marked ` Jul` after the date, and in UT inside
	// the 1582 window. Go's calendar is never asked to name it.
	s := lookup.StampMoment(t, t.Location())
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
	wd, mon := s.Weekday.String()[:3], time.Month(s.Month).String()[:3]
	clock := fmt.Sprintf("%2d:%02d%s", hour, s.Minute, mark)
	jul := ""
	if s.Julian {
		jul = " Jul"
	}
	if p.DayFirst {
		return fmt.Sprintf("%s %2d %s %d%s %s", wd, s.Day, mon, s.Year, jul, clock)
	}
	return fmt.Sprintf("%s %s %2d, %d%s %s", wd, mon, s.Day, s.Year, jul, clock)
}

// civilDate is a date as a chart file states it, in the held date order and
// without a weekday or a clock: `Oct  5, 2026` or ` 5 Oct 2026`. It is
// spelled from the fields, not through time.Date, so a Julian `29 Feb 1500`
// stays what the file says.
func civilDate(year, month, day int) string {
	return lookup.CivilDate(year, month, day, lookup.HeldClock())
}
