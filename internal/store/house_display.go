package store

import (
	"fmt"
	"os"
	"slices"
	"strings"
)

// HouseDisplayConfig is the top-level `house_display:` key of preferences.yaml
// exactly as it was read: how the Bodies house column is drawn.
//
// Like WheelFormatConfig it holds no defaults and no judgement. Whether the
// value is one of the forms, and what happens when it is not, is internal/ui's
// to decide.
type HouseDisplayConfig struct {
	// Value is the first `house_display:` entry, unquoted and without a
	// trailing comment. Present is false when there is none, or when it is
	// empty.
	Value   string
	Present bool
}

// HouseDisplay reads `house_display:` from preferences.yaml:
//
//	house_display: quadrants
//
// An absent or unreadable file reports it absent — the ordinary case.
func HouseDisplay() HouseDisplayConfig {
	return readHouseDisplay(PreferencesPath())
}

// readHouseDisplay reads only the first top-level `house_display:` line, which
// is what makes a hand-edited file with a duplicate resolve to one answer, and
// the same answer the writer replaces. An indented one belongs to some block
// and is not this key.
func readHouseDisplay(path string) HouseDisplayConfig {
	// The gate every preference reader goes through: a file from a later
	// generation, or one whose version cannot be read, is no file at all.
	// Guessing at a newer generation's keys is worse than asking again.
	if _, err := readableSchema(path); err != nil {
		return HouseDisplayConfig{}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return HouseDisplayConfig{}
	}
	for _, raw := range splitAfterNewlines(string(data)) {
		line := strings.TrimRight(raw, "\r\n")
		if key, ok := topLevelKey(line); !ok || key != houseDisplayKey {
			continue
		}
		_, val, _ := splitKV(stripInlineComment(strings.TrimSpace(line)))
		return HouseDisplayConfig{Value: val, Present: val != ""}
	}
	return HouseDisplayConfig{}
}

const houseDisplayKey = "house_display"

// houseDisplayWords are the only values the writer will put on the line: the
// four forms. The store is passive and judges nothing it reads, but a value it
// WRITES is its own to vouch for, and a closed list keeps the line a bare word
// that reads back as it was written.
var houseDisplayWords = []string{"angularity", "quadrants", "number", "marked"}

// SetHouseDisplay writes `house_display: <value>` to preferences.yaml
// atomically (tmpfile + rename), creating the file and its directory if
// missing.
//
// The write owns that one line and nothing else: every other line of the
// file — blocks, comments, blank lines, blocks this package has no parser
// for, even a `version:` it did not write — is carried through byte-for-byte.
// A file from a later generation is refused, as for the location, and so is
// any value that is not one of the four form words: nothing is written.
func SetHouseDisplay(value string) error {
	if !slices.Contains(houseDisplayWords, value) {
		return fmt.Errorf("store: house display: %q is not one of %s", value, strings.Join(houseDisplayWords, ", "))
	}
	return writePreferences(PreferencesPath(), func(existing string) string {
		return mergeLine(existing, houseDisplayKey, houseDisplayKey+": "+value)
	})
}

// mergeLine puts line, one whole top-level `key: value` line without its
// terminator, where the first top-level line named key is, and leaves every
// other byte alone. The replaced line keeps its own terminator. When no such
// line is there, the line is appended.
//
// Unlike mergeBlock it owns the one line, not the span up to the next
// top-level key: a scalar has no body, so whatever follows it is somebody
// else's and stays.
func mergeLine(existing, key, line string) string {
	lines := splitAfterNewlines(existing)
	for i, l := range lines {
		if k, ok := topLevelKey(l); !ok || k != key {
			continue
		}
		lines[i] = line + l[len(strings.TrimRight(l, "\r\n")):]
		return strings.Join(lines, "")
	}
	if existing != "" && !strings.HasSuffix(existing, "\n") {
		existing += "\n"
	}
	return existing + line + "\n"
}
