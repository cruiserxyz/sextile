package astrolog

import (
	"fmt"
	"strconv"
	"time"

	"github.com/cruiserxyz/sextile/internal/recipe"
)

// localClock is the one derivation of a slot's wall clock: the instant to the
// second in tz, and the marker and offset Astrolog is told (`-zZ`). The marker
// and the zone are NOT astrological values: they only tell Astrolog which wall
// clock to print. The instant itself is untouched, always a `-qj` Julian Day
// (the specification C1), so the chart body is the same with or without them.
//
// Daylight time is spoken as `DT` plus the STANDARD offset only when the shift
// is exactly one hour. Any other shift (Lord Howe's thirty minutes, a zone
// whose "daylight" is negative) is spoken as `ST` plus the TOTAL offset, so
// Astrolog never adds an hour the zone did not add.
func localClock(jd float64, tz string) (t time.Time, marker string, offset int, err error) {
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return time.Time{}, "", 0, fmt.Errorf("astrolog: Compile: timezone %q does not load: %w", tz, err)
	}
	t = julianDayToUTCSecond(jd).In(loc)
	marker, offset = "ST", zoneOffset(t)
	if t.IsDST() {
		if std, ok := standardOffset(t); ok && zoneOffset(t)-std == 3600 {
			marker, offset = "DT", std
		}
	}
	return t, marker, offset, nil
}

// momentArg is the one Julian Day every call for a MomentAtPlace sends, zoned
// or not, so the chart, the wheel and the reports of one slot ask Astrolog the
// identical instant (the specification). It is the instant truncated to the
// whole second (SlotJD), encoded the way savechart.go writes it: one float above
// the exact value, formatted shortest-exact. A moment before Julian Day 0 is
// refused, and no argv is built (the specification C10).
//
// Astrolog truncates the printed minute from the Julian Day it is given. A whole
// minute is usually a repeating binary fraction; if its nearest float lies just
// below the boundary, the minute prints as the one before it (measured: `-qj
// 2461319.438194` prints 22:30 for 22:31:00). One float up, which is far below
// the precision of the record, can never leave the minute.
func momentArg(jd float64) (string, error) {
	sec := julianDayToUTCSecond(jd).Unix()
	if err := refuseBeforeJD0(sec); err != nil {
		return "", err
	}
	return strconv.FormatFloat(sentJD(sec), 'f', -1, 64), nil
}

// chartZone is the single-chart compute's zone pin: `-zZ <zone> <dst>`
// (the specification C4).
//
// The zone and the dst flag are DT 1 + standard offset for a one-hour shift,
// otherwise ST 0 + total offset, NOT savechart.go's DST-0 form, which prints
// "ST Zone 2E" for a summer chart. It comes AFTER `-qj`, which resets the zone.
//
// Inside the 1582 window it is empty (see zoneArgs).
func chartZone(m recipe.MomentAtPlace) ([]string, error) { return zoneArgs(m.JD, m.TZ) }

// zoneArgs is `-zZ <zone> <dst>` for a slot's Julian Day at tz, or nothing inside
// the 1582 window. For a UT instant in [JD 2299170.5 - east/24, 2299170.5) with
// a zone east of Greenwich, Astrolog asked for the local clock carries the date
// into 15 Oct and then reads it as Gregorian, casting the chart ten days early
// (Astrolog NEXT.md BUG 2). Such a moment is sent with no zone at all and shown
// in UT (the specification, § The 1582 window).
func zoneArgs(jd float64, tz string) ([]string, error) {
	t, marker, offset, err := localClock(jd, tz)
	if err != nil {
		return nil, err
	}
	if jdWindow(sentJD(t.Unix()), zoneOffset(t)) {
		return nil, nil
	}
	dst := "0"
	if marker == "DT" {
		dst = "1"
	}
	return []string{"-zZ", formatSearchZone(offset), dst}, nil
}

// zonesChart says whether a projection is the single-chart compute (positions
// and the aspect text that supplies Details/Aspects).
func zonesChart(p recipe.Projection) bool {
	switch p.(type) {
	case recipe.Positions, recipe.Aspects:
		return true
	}
	return false
}

func zoneOffset(t time.Time) int {
	_, off := t.Zone()
	return off
}

// standardOffset is the offset (seconds east) of the zone's standard time
// around t: the offset of the nearest instant, probing outward a week at a
// time for up to a year either way, at which the zone is not in daylight
// time. ok is false for a zone that is never off daylight time nearby.
func standardOffset(t time.Time) (int, bool) {
	for d := 1; d <= 53; d++ {
		for _, sign := range []int{-1, 1} {
			p := t.AddDate(0, 0, sign*7*d)
			if !p.IsDST() {
				return zoneOffset(p), true
			}
		}
	}
	return 0, false
}
