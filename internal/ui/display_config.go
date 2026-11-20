package ui

import (
	"strconv"
	"strings"

	"github.com/cruiserxyz/sextile/internal/store"
)

// applyDisplayConfig takes the display: block into the active configuration.
// The reader reports; the decision and any Log line are made here, as for the
// keys: block. Silence is the contract for the good cases.
//
// A colour accepts the two forms lipgloss renders the same on every terminal
// it supports: a 256-colour palette number, or #rrggbb. Anything else would be
// handed to the terminal as a colour it might draw as anything, so it is
// refused with one Log note and the default stands. The retired row_stripe
// gets one note per run, however often the block is applied.
func (m Model) applyDisplayConfig(d store.DisplayConfig) Model {
	for _, c := range []struct {
		key, val string
		into     *string
	}{
		{"display.focus_color", d.FocusColour, &m.cfg.FocusColourName},
		{"display.cursor_row_color", d.CursorRowColour, &m.cfg.CursorRowColourName},
	} {
		if c.val == "" {
			continue
		}
		if !validDisplayColour(c.val) {
			m = m.logNote("settings", "colour %q not valid, ignored", c.val)
			continue
		}
		*c.into = c.val
	}
	if d.RetiredStripePresent && !m.rowStripeNoted {
		m.rowStripeNoted = true
		m = m.logNote("settings", "display.row_stripe no longer does anything, rows have no stripes")
	}
	return m
}

func validDisplayColour(s string) bool {
	if n, err := strconv.Atoi(s); err == nil {
		return n >= 0 && n <= 255
	}
	if len(s) != 7 || s[0] != '#' {
		return false
	}
	for _, c := range s[1:] {
		if !strings.ContainsRune("0123456789abcdefABCDEF", c) {
			return false
		}
	}
	return true
}
