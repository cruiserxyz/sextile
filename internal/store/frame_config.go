package store

import (
	"fmt"
	"os"
	"strings"

	"github.com/cruiserxyz/sextile/internal/recipe"
)

// FrameConfig is the `frame:` block of preferences.yaml exactly as it was read
// (the specification C1, C11d): the default house system a chart falls
// back to when neither the session nor its chart file names one, and the
// default zodiac when its session record names none (the specification
// C1), in the form a record carries: `tropical` or `sidereal <name>`.
//
// Like KeysConfig it holds no judgement. Whether a value is a house system,
// and what happens when it is not, is internal/ui's to decide.
type FrameConfig struct {
	Houses        string // the first `houses:` value in the first `frame:` block
	HousesPresent bool
	Zodiac        string // likewise for `zodiac:`
	ZodiacPresent bool
}

// Frame reads the `frame:` block from preferences.yaml:
//
//	frame:
//	  houses: Koch
//
// A `base:` key left in the block by the retired base switch is ignored, and
// nothing rewrites it. An absent or unreadable file reports every value absent.
func Frame() FrameConfig {
	return readFrame(PreferencesPath())
}

// readFrame reads only the FIRST `frame:` block, and in it only the first
// `houses:` and the first `zodiac:` key among the block's direct children;
// later blocks, deeper lines and repeated keys are ignored, which is what makes
// a hand-edited file with a duplicate resolve to one answer. It goes through
// the gate every preference reader does: a file from a later generation, or one
// whose version cannot be read, is no file at all.
func readFrame(path string) FrameConfig {
	var cfg FrameConfig
	if _, err := readableSchema(path); err != nil {
		return cfg
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg
	}
	lines := splitAfterNewlines(string(data))
	fb, ok := findBlock(lines, "frame")
	if !ok || fb.scalar {
		return cfg
	}
	value := func(key string) (string, bool) {
		i, ok := fb.at[key]
		if !ok {
			return "", false
		}
		_, val, _ := splitKV(stripInlineComment(strings.TrimSpace(lines[i])))
		return val, true
	}
	cfg.Houses, cfg.HousesPresent = value("houses")
	cfg.Zodiac, cfg.ZodiacPresent = value("zodiac")
	return cfg
}

// prefBlock is where the first top-level block of one name sits in a file's
// lines, as its reader and its writer both find it, so they cannot disagree on
// what the block owns. The `frame:` and `experimental:` blocks share it.
type prefBlock struct {
	start, end int // the header line, and the line that closes the block
	// scalar reports `<name>: <value>`: the key has a value of its own, so
	// it opens no mapping and has no children to read or to add to.
	scalar bool
	// childIndent is the indent of the first line in the block that is
	// neither blank nor a comment. Only a key at exactly that indent is a
	// child of the block; a deeper line belongs to some nested mapping.
	childIndent string
	// at maps a direct child's key to its first line.
	at map[string]int
	// last is the block's last line that is neither blank nor a comment in
	// column zero (start when it has none). A column-zero comment introduces
	// what follows it, so a key added to the block goes above it.
	last int
	// stray is the first line of the span, -1 for none, that sits in column
	// zero and is neither blank, a comment nor a top-level key: YAML would not
	// read the lines under it as the block's children, so a writer that added
	// one would add a line its own reader then ignores.
	stray int
}

// findBlock finds the first top-level `<name>:` block. It ends at the next
// top-level key, or at a YAML document marker (`---`, `...` in column zero),
// whichever comes first.
func findBlock(lines []string, name string) (prefBlock, bool) {
	fb := prefBlock{start: -1, end: len(lines), at: map[string]int{}, stray: -1}
	for i, l := range lines {
		if k, ok := topLevelKey(l); ok && k == name {
			fb.start = i
			break
		}
	}
	if fb.start < 0 {
		return fb, false
	}
	head := strings.TrimRight(lines[fb.start], "\r\n")
	rest := head[len(name)+1:]
	value := strings.TrimLeft(rest, " \t")
	// A comment after the colon still opens a mapping; a value does not.
	fb.scalar = value != "" && !(len(value) < len(rest) && value[0] == '#')
	for i := fb.start + 1; i < len(lines); i++ {
		// A column-zero list entry (`- houses: Koch`) carries a colon, but it
		// is the block's value, not the next key: it stays in the span, where
		// it is a stray line. topLevelKey itself is left alone, because the
		// whole-block writers (mergeBlock) would then swallow such lines.
		if _, ok := topLevelKey(lines[i]); (ok && !isSequenceEntry(lines[i])) || isDocumentMarker(lines[i]) {
			fb.end = i
			break
		}
	}
	fb.last = fb.start
	indented := false
	for i := fb.start + 1; i < fb.end; i++ {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed == "" {
			continue
		}
		indent := lines[i][:len(lines[i])-len(strings.TrimLeft(lines[i], " \t"))]
		if strings.HasPrefix(trimmed, "#") {
			if indent != "" {
				fb.last = i
			}
			continue
		}
		fb.last = i
		if indent == "" && fb.stray < 0 {
			fb.stray = i
		}
		if !indented {
			indented = true
			if indent != "" {
				fb.childIndent = indent
			}
		}
		if indent != fb.childIndent || indent == "" {
			continue
		}
		if key, _, ok := splitKV(stripInlineComment(trimmed)); ok {
			if _, seen := fb.at[key]; !seen {
				fb.at[key] = i
			}
		}
	}
	if fb.childIndent == "" {
		fb.childIndent = "  "
	}
	return fb, true
}

// isSequenceEntry reports a YAML block sequence entry in column zero: a dash
// alone, or followed by a space or tab.
func isSequenceEntry(line string) bool {
	line = strings.TrimRight(line, "\r\n")
	return line == "-" || strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "-\t")
}

// isDocumentMarker reports a YAML document start or end (`---`, `...`) in
// column zero, bare or followed by a space or tab.
func isDocumentMarker(line string) bool {
	line = strings.TrimRight(line, "\r\n")
	for _, m := range []string{"---", "..."} {
		if line == m || strings.HasPrefix(line, m+" ") || strings.HasPrefix(line, m+"\t") {
			return true
		}
	}
	return false
}

// SetFrame writes the default frame to preferences.yaml: `houses:` and
// `zodiac:` in the first top-level `frame:` block, atomically, creating the
// file, the block and either key as needed. houses is Astrolog's header name
// and zodiac the record form (`tropical` or `sidereal <name>`), the words
// recipe parses; a value outside them is refused and nothing is written,
// because the store judges nothing it reads but vouches for what it writes.
//
// The write owns those two keys' value tokens and nothing else. In the block it
// replaces the value of the first direct-child `houses:` line and `zodiac:`
// line in place (setValueToken), keeping the key as spelled, what follows the
// value and the terminator; a missing key goes after the block's last line
// that is neither blank nor a column-zero comment, at the block's child indent.
// Every other line — a `base:` the retired switch left, a comment, a nested
// mapping with a `houses:` of its own, a key this package does not know — and
// every byte outside the block is carried through byte-for-byte, which is why
// this is not mergeBlock: that owns the whole span and would drop what the
// writer did not author. A `frame:` that holds a value instead of a block is
// refused, because keys cannot be added under it, and so is a block with a
// column-zero line YAML would not read as part of it.
func SetFrame(houses, zodiac string) error {
	if _, ok := recipe.ParseHouseSystem(houses); !ok {
		return fmt.Errorf("store: frame: %q is not a house system name", houses)
	}
	if _, ok := recipe.ParseZodiacRecord(zodiac); !ok {
		return fmt.Errorf("store: frame: %q is not a zodiac", zodiac)
	}
	return writePreferencesChecked(PreferencesPath(), func(existing string) (string, error) {
		return mergeFrame(existing, houses, zodiac)
	})
}

// mergeFrame is SetFrame's text edit.
func mergeFrame(existing, houses, zodiac string) (string, error) {
	lines := splitAfterNewlines(existing)
	fb, found := findBlock(lines, "frame")
	if !found {
		if existing != "" && !strings.HasSuffix(existing, "\n") {
			existing += "\n"
		}
		return existing + "frame:\n  houses: " + houses + "\n  zodiac: " + zodiac + "\n", nil
	}
	if fb.scalar {
		return "", fmt.Errorf("store: preferences.yaml has '%s', not a block. The frame was not saved",
			strings.TrimSpace(lines[fb.start]))
	}
	if fb.stray >= 0 {
		return "", fmt.Errorf("store: preferences.yaml's frame: block holds '%s', which YAML does not read as part of it. The frame was not saved",
			strings.TrimSpace(lines[fb.stray]))
	}
	var inserted []string
	for _, kv := range [][2]string{{"houses", houses}, {"zodiac", zodiac}} {
		if i, ok := fb.at[kv[0]]; ok {
			// Only the value token moves (setValueToken, the experimental
			// writer's): the key as spelled, the whitespace and comment after
			// the value and the line ending stay as written.
			lines[i] = setValueToken(lines[i], kv[1])
			continue
		}
		inserted = append(inserted, fb.childIndent+kv[0]+": "+kv[1]+"\n")
	}
	if len(inserted) > 0 {
		if !strings.HasSuffix(lines[fb.last], "\n") {
			lines[fb.last] += "\n"
		}
		lines = append(lines[:fb.last+1], append(inserted, lines[fb.last+1:]...)...)
	}
	return strings.Join(lines, ""), nil
}
