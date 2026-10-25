package store

import (
	"os"
	"strings"
)

// ClockConfig is the `clock:` block of preferences.yaml (the specification
// C10): how Astrolog's times and dates are printed.
//
//	clock:
//	  hours: 24          # 24 (default) or 12
//	  dates: month-first # month-first (default) or day-first
//
// Unlike KeysConfig it reports the RESOLVED value, because the defaults are the
// file format's own and a rejected entry falls back to them; the rejections
// are what internal/ui writes to the Log, once per start.
type ClockConfig struct {
	Hours string // "24" (default) or "12"
	Dates string // "month-first" (default) or "day-first"

	// Rejections lists, at most once per field, the winning entry that was
	// read and refused, in the order the fields are declared (hours, dates).
	Rejections []ClockRejection
}

// ClockRejection is an entry the reader refused: the field and the value as
// the file spelled it.
type ClockRejection struct {
	Field string // "clock.hours" or "clock.dates"
	Value string
}

// Clock reads the `clock:` block from preferences.yaml. An absent or
// unreadable file, block or field reports the defaults with no rejection.
func Clock() ClockConfig {
	return readClock(PreferencesPath())
}

// readClock follows readKeys: block boundaries come from topLevelKey, only the
// FIRST `clock:` block is read, and an entry this version does not know is
// ignored. Within the block the LAST entry of a field wins, and an empty last
// entry means the field's default (`hours: 12` then `hours:` resolves to 24,
// with no rejection; C10, N20).
func readClock(path string) ClockConfig {
	cfg := ClockConfig{Hours: "24", Dates: "month-first"}
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg
	}
	var hours, dates string
	inBlock := false
	for _, raw := range splitAfterNewlines(string(data)) {
		line := strings.TrimRight(raw, "\r\n")
		if key, ok := topLevelKey(line); ok {
			if inBlock {
				break
			}
			inBlock = key == "clock"
			continue
		}
		if !inBlock {
			continue
		}
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		key, val, ok := splitKV(stripInlineComment(trimmed))
		if !ok {
			continue
		}
		switch key {
		case "hours":
			hours = val
		case "dates":
			dates = val
		}
	}
	switch hours {
	case "":
	case "24", "12":
		cfg.Hours = hours
	default:
		cfg.Rejections = append(cfg.Rejections, ClockRejection{Field: "clock.hours", Value: hours})
	}
	switch dates {
	case "":
	case "month-first", "day-first":
		cfg.Dates = dates
	default:
		cfg.Rejections = append(cfg.Rejections, ClockRejection{Field: "clock.dates", Value: dates})
	}
	return cfg
}

// The seams of the preferences write (the specification C11): renameFile is
// the atomic replace, betweenReadAndRename runs after the new content is
// staged and before the file is read again to check nobody edited it.
var (
	renameFile           = os.Rename
	betweenReadAndRename func()
)
