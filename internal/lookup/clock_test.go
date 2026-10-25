package lookup

import (
	"testing"

	"github.com/cruiserxyz/sextile/internal/astrolog"
)

// amPmMonthFirst holds the 12-hour, month-first clock for a test whose canned
// Astrolog timeline text is in that form (the specification C3: the parser
// reads the form the held preference pins, and no other).
func amPmMonthFirst(t *testing.T) {
	t.Helper()
	astrolog.SetClock(astrolog.Clock{Hours: astrolog.Hours12, Dates: astrolog.MonthFirst})
	t.Cleanup(func() { astrolog.SetClock(astrolog.Clock{}) })
}
