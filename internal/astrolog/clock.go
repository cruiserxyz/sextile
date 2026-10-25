package astrolog

import "sync"

// The clock preference (the specification): how Astrolog prints a time and a
// date, chosen by the operator in preferences.yaml and read ONCE at start. It
// is held here, beside the compiler that spells it, because it is a display
// pin on every call and not a property of any one chart; a per-call reader
// would be a live reload the spec rules out (C12).

// HourForm is the clock's hour form.
type HourForm uint8

const (
	Hours24 HourForm = iota // the default: 22:29
	Hours12                 // 10:29pm
)

// DateOrder is the date's order.
type DateOrder uint8

const (
	MonthFirst DateOrder = iota // the default: Mon Oct  5, 2026 / 10/ 1/2026
	DayFirst                    // Mon  5 Oct 2026 / 1-10-2026
)

// Clock is the pair of display pins. The zero value is the default: 24-hour,
// month first.
type Clock struct {
	Hours HourForm
	Dates DateOrder
}

var (
	clockMu   sync.RWMutex
	clockHeld Clock
)

// SetClock holds the session's clock preference. It is called once, when the
// startup read lands, before the first Astrolog call.
func SetClock(c Clock) {
	clockMu.Lock()
	clockHeld = c
	clockMu.Unlock()
}

// CurrentClock is the held preference.
func CurrentClock() Clock {
	clockMu.RLock()
	defer clockMu.RUnlock()
	return clockHeld
}

// Pins is the two display switches for the clock, as a fresh slice. Each is a
// SET, never a toggle: `=Yt` and `_Yt` name a state, where a bare `-Yt` flips
// whatever a host astrolog.as left (measured: a host `-Yt` plus argv `-Yt`
// prints 10:29pm).
func (c Clock) Pins() []string {
	hours, dates := "=Yt", "_Yd"
	if c.Hours == Hours12 {
		hours = "_Yt"
	}
	if c.Dates == DayFirst {
		dates = "=Yd"
	}
	return []string{hours, dates}
}

// reformPin pins Astrolog's Gregorian reform date to 15 Oct 1582, the date its
// searches and its `-qa` reader step by, so a host astrolog.as that moves it
// (`-qag 10 25 1582 ...`) cannot move a row or a chart-file moment
// (the specification C6). `-qag` sets the reform date and leaves the chart
// alone; it is a chart argument all the same, which is why only the chart and
// search calls and the chart-file read carry it and the exempt metadata calls
// (-Hc, -HO, -HA) do not. The 12-hour time token parses under both clock pins.
var reformPin = []string{"-qag", "10", "15", "1582", "12:00am", "0", "0:00E", "0:00N"}
