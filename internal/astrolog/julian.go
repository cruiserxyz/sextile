package astrolog

import (
	"math"
	"time"
)

// GregorianToJD converts a time.Time to its Julian Day number, the float
// astrolog accepts via the `-qj` switch.
//
// It is the Unix arithmetic and nothing else: the instant's Unix seconds split
// into whole days and a fraction, plus the Julian Day of the Unix epoch
// (2440587.5, noon-based Julian Days start at noon, so midnight UTC is a .5).
// No calendar date is named on the way, so the answer is exact for every
// instant at or after Julian Day 0, whichever calendar a date near it belongs
// to (the former Meeus date formula with truncating division was one day late
// at 1 March -100, -200, -300 and -500: the specification C2).
//
// The input is converted to UTC first; astrolog operates on absolute time,
// not civil-local — chart calc must pass JD computed in UTC.
func GregorianToJD(t time.Time) float64 {
	return unixJD(t.Unix(), t.Nanosecond())
}

// unixJD is the Julian Day of a Unix second plus nanoseconds.
func unixJD(sec int64, nsec int) float64 {
	days := floorDiv(sec, 86400)
	rem := sec - days*86400
	frac := (float64(rem) + float64(nsec)/1e9) / 86400.0
	return float64(days) + jdUnixEpoch + frac
}

// sentJD is the Julian Day Sextile sends for a Unix second: one float above the
// exact value (see momentArg).
func sentJD(sec int64) float64 {
	return math.Nextafter(unixJD(sec, 0), math.Inf(1))
}

// SlotJD is the one way a slot's instant becomes a Julian Day for a chart,
// report, wheel or timeline call. Saving a chart (saveArgv) rounds to the minute
// under its own contract and is not covered by this rule. The
// instant is made a WHOLE SECOND first, by truncation: the second Sextile shows
// (23:59:59.9995 reads 23:59, never the next day's midnight) and the second the
// timeline's -qb natal sends. Because the fraction is gone before any Julian Day
// exists, julianDayToUTCSecond's 1 ms tolerance only ever absorbs float error
// (a Julian Day's ulp is under 0.1 ms across Astrolog's whole date range) and
// never meets a real fraction.
func SlotJD(t time.Time) float64 { return GregorianToJD(t.Truncate(time.Second)) }
