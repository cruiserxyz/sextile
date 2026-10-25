package ui

import (
	"testing"

	"github.com/cruiserxyz/sextile/internal/lookup"
)

// amPmMonthFirst holds the 12-hour, month-first clock for a test whose canned
// Astrolog timeline text is in that form (the specification C3: the parser
// reads the form the held preference pins, and no other).
func amPmMonthFirst(t *testing.T) {
	t.Helper()
	lookup.HoldClock(lookup.ClockPref{Hours12: true})
	t.Cleanup(func() { lookup.HoldClock(lookup.ClockPref{}) })
}

// defaultClock holds the default clock (24-hour, month first) for a test whose
// canned Astrolog timeline text is in that form (the specification C8: the
// goldens are recorded under the default).
func defaultClock(t *testing.T) {
	t.Helper()
	lookup.HoldClock(lookup.ClockPref{})
	t.Cleanup(func() { lookup.HoldClock(lookup.ClockPref{}) })
}
