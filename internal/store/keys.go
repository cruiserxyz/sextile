package store

import (
	"os"
	"strconv"
	"strings"
)

// KeysConfig is the `keys:` block exactly as it was read: each value with a
// presence marker, plus the entries that could not be used.
//
// The store is passive. It holds no key map and no resolver: it does not know
// what a leader is for, which keys would strand the operator, or what any
// default should be. It reports what the file said and what it could not
// make sense of; internal/ui decides what to do about it and writes any Log
// line, exactly as the `location:` reader already documents.
type KeysConfig struct {
	// Leader is the configured leader key. LeaderPresent is false when the
	// entry is missing or empty — an empty value is a deleted entry, not a
	// mistake, so it carries no rejection.
	Leader        string
	LeaderPresent bool

	// TimeoutlenMS is how long a pending leader waits, in milliseconds.
	// Present only when the entry parsed as a whole number above zero.
	TimeoutlenMS      int
	TimeoutlenPresent bool

	// Rejections lists the entries that were read and refused, in file
	// order, so the caller can report each one exactly once.
	Rejections []KeyRejection
}

// KeyRejection is one entry the reader refused: which field, what it said,
// and why it could not be used. The reason is a plain phrase the caller can
// print as-is.
type KeyRejection struct {
	Field  string
	Value  string
	Reason string
}

// Keys reads the `keys:` block from preferences.yaml. An absent or unreadable
// file reports every value absent with no rejections — absence is not an
// error here, it is the ordinary case.
func Keys() KeysConfig {
	return readKeys(PreferencesPath())
}

// readKeys parses the same tolerant hand-rolled YAML subset the location
// reader uses:
//
//	keys:
//	  leader: ","
//	  timeoutlen: 750
//
// Block boundaries come from topLevelKey, so the reader and the writer agree
// on where a top-level block starts and ends. Unrecognised entries inside the
// block are ignored rather than rejected: a key this version has not heard of
// is a later version's business, not a mistake to report.
//
// Only the FIRST `keys:` block is read. A writer owns the first block of a
// name and carries any later one through verbatim, so a reader that merged
// them would report values from a block no writer will ever touch — and a
// rejection from one block could sit beside a value from another.
func readKeys(path string) KeysConfig {
	var cfg KeysConfig
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg
	}
	inBlock := false
	for _, raw := range splitAfterNewlines(string(data)) {
		line := strings.TrimRight(raw, "\r\n")
		if key, ok := topLevelKey(line); ok {
			if inBlock {
				break // the block we own has ended
			}
			inBlock = key == "keys"
			continue
		}
		if !inBlock {
			continue
		}
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		key, val, ok := splitKV(trimmed)
		if !ok {
			continue
		}
		switch key {
		case "leader":
			if val == "" {
				continue
			}
			cfg.Leader = val
			cfg.LeaderPresent = true
		case "timeoutlen":
			ms, err := strconv.Atoi(val)
			if err != nil || ms <= 0 {
				cfg.Rejections = append(cfg.Rejections, KeyRejection{
					Field:  "keys.timeoutlen",
					Value:  val,
					Reason: "expected a whole number of milliseconds above zero",
				})
				continue
			}
			cfg.TimeoutlenMS = ms
			cfg.TimeoutlenPresent = true
		}
	}
	return cfg
}
