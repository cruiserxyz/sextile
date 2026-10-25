package ui

import (
	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/internal/store"
)

// applyClockConfig holds the operator's clock preference for the session
// (the specification C10, C12): it is read once, with the rest of the
// startup perimeter, and handed to the one place that spells Astrolog's
// display pins. Nothing re-reads preferences.yaml for it, so editing the file
// while Sextile runs changes nothing until a restart.
//
// A value the store resolved to nothing (a hand-built message) is the default.
// A refused entry is said once, as one Log line naming the field and the value
// (%q, so a line break or a control byte in the file cannot break the line).
func (m Model) applyClockConfig(cfg store.ClockConfig) Model {
	lookup.HoldClock(lookup.ClockPref{Hours12: cfg.Hours == "12", DayFirst: cfg.Dates == "day-first"})
	for _, r := range cfg.Rejections {
		want := "24 or 12"
		if r.Field == "clock.dates" {
			want = "month-first or day-first"
		}
		m = m.logNote("preferences", "%s %q ignored: expected %s", r.Field, r.Value, want)
	}
	return m
}
