package store

import (
	"bufio"
	"fmt"
	"strings"
	"time"

	"github.com/cruiserxyz/sextile/internal/recipe"
)

// session_frames.go is the session file's version-3 `frames:` sequence
// (the specification C11): one record per chart whose house system or
// zodiac (the specification C6) the operator chose, keyed by the chart file's base name, plus
// at most one `slot: floating` record. Like `charts:` it is machine-managed
// and read strictly; unlike `charts:` a bad entry costs only itself.

// Where a recorded house system came from.
const (
	FrameFromChosen = "chosen" // the operator picked it
	FrameFromFile   = "file"   // preserved from the chart file at a rename (C9)
)

// validZodiac reports whether v is a zodiac record value in the one form
// the specification C6 allows: `tropical`, or `sidereal <name>` with the
// ayanamsa's name exactly as C2 spells it. A bare name (the form an earlier
// build accepted, and never wrote) is not one.
func validZodiac(v string) bool {
	_, ok := recipe.ParseZodiacRecord(v)
	return ok
}

// FrameRecord is one `frames:` entry. A field that is absent resolves through
// the precedence rule on every read, so a zodiac-only record never freezes the
// houses (HasHouses false).
type FrameRecord struct {
	File     string    // chart file base name; empty for the floating slot
	Floating bool      // the `slot: floating` record
	When     time.Time // the chart's birth moment (file records only)

	Houses    recipe.HouseSystem
	HasHouses bool
	From      string // FrameFromChosen or FrameFromFile; set with HasHouses

	Zodiac    string
	HasZodiac bool
}

// validFrameFile is the rule for a `file:` value: one base name.
func validFrameFile(f string) bool {
	return f != "" && f != "." && f != ".." && !strings.ContainsAny(f, "/\\\x00")
}

// writable reports whether the record is one the writer can emit in the
// specified shape.
func (r FrameRecord) writable() bool {
	if r.Floating == (r.File != "") {
		return false
	}
	if !r.Floating && (!validFrameFile(r.File) || r.When.IsZero()) {
		return false
	}
	if r.HasHouses && (!r.Houses.Valid() || (r.From != FrameFromChosen && r.From != FrameFromFile)) {
		return false
	}
	if r.HasZodiac && !validZodiac(r.Zodiac) {
		return false
	}
	return r.HasHouses || r.HasZodiac
}

func writeFrames(b *strings.Builder, frames []FrameRecord) {
	var out strings.Builder
	for _, r := range frames {
		if !r.writable() {
			continue
		}
		if r.Floating {
			out.WriteString("  - slot: floating\n")
		} else {
			fmt.Fprintf(&out, "  - file: %s\n", yamlEscape(r.File))
			fmt.Fprintf(&out, "    when: %s\n", yamlEscape(r.When.UTC().Format(time.RFC3339)))
		}
		if r.HasHouses {
			fmt.Fprintf(&out, "    houses: %s\n", yamlEscape(r.Houses.StoredName()))
			fmt.Fprintf(&out, "    from: %s\n", r.From)
		}
		if r.HasZodiac {
			fmt.Fprintf(&out, "    zodiac: %s\n", yamlEscape(r.Zodiac))
		}
	}
	if out.Len() > 0 {
		b.WriteString("frames:\n")
		b.WriteString(out.String())
	}
}

var frameKeyOrder = map[string]int{"file": 0, "slot": 0, "when": 1, "houses": 2, "from": 3, "zodiac": 4}

type frameEntry struct {
	rec    FrameRecord
	last   int
	seen   map[string]bool
	reason string // the first reason the entry is not the writer's
	stray  bool
	zodBad bool
}

func (e *frameEntry) fail(why string) {
	if e.reason == "" {
		e.reason = why
	}
}

// parseFrames reads the `frames:` block of a version-3 file. Returns the kept
// records, how many entries (or blocks) were dropped, and one note per drop.
func parseFrames(text string) (frames []FrameRecord, dropped int, notes []string) {
	var cur *frameEntry
	blocks := 0
	inBlock, skipBlock := false, false
	files := map[string]bool{}
	floating := false

	drop := func(why string) {
		dropped++
		notes = append(notes, "a frames entry was dropped: "+why)
	}
	flush := func() {
		if cur == nil {
			return
		}
		e := cur
		cur = nil
		r := e.rec
		if e.stray {
			drop("a line that is not a frames entry")
			return
		}
		if !e.seen["file"] && !e.seen["slot"] {
			e.fail("no file")
		}
		if e.seen["file"] && !e.seen["when"] {
			e.fail("no when")
		}
		if e.seen["houses"] != e.seen["from"] {
			e.fail("houses and from must come together")
		}
		if e.zodBad {
			// Only the field goes; the entry keeps whatever else it has.
			notes = append(notes, "a frames entry's zodiac is not one this build knows; the field was dropped")
			dropped++
			r.HasZodiac, r.Zodiac = false, ""
		}
		if !r.HasHouses && !r.HasZodiac {
			e.fail("it carries neither houses nor zodiac")
		}
		if e.reason != "" {
			drop(e.reason)
			return
		}
		if r.Floating {
			if floating {
				drop("a second floating-slot record")
				return
			}
			floating = true
		} else {
			if files[r.File] {
				drop("a second record for " + r.File)
				return
			}
			files[r.File] = true
		}
		frames = append(frames, r)
	}
	invalid := func(why string) {
		if skipBlock {
			return
		}
		if cur == nil {
			cur = &frameEntry{stray: true, last: -1}
			return
		}
		cur.fail(why)
	}

	sc := bufio.NewScanner(strings.NewReader(text))
	sc.Buffer(make([]byte, 0, 64*1024), sessionMaxLine+1)
	for sc.Scan() {
		raw := sc.Text()
		if key, ok := topLevelKey(raw); ok && !strings.HasPrefix(raw, "-") {
			flush()
			inBlock = key == "frames"
			skipBlock = false
			if inBlock {
				blocks++
				if blocks > 1 {
					skipBlock = true
					dropped++
					notes = append(notes, "a second frames block was dropped")
				}
			}
			continue
		}
		if !inBlock || skipBlock {
			continue
		}
		var rest string
		marker := false
		switch {
		case strings.HasPrefix(raw, "  - ") && len(raw) > 4 && raw[4] != ' ' && raw[4] != '\t':
			rest, marker = raw[4:], true
		case strings.HasPrefix(raw, "    ") && len(raw) > 4 && raw[4] != ' ' && raw[4] != '\t':
			rest = raw[4:]
		default:
			invalid("a line that is not a frames entry")
			continue
		}
		idx := strings.Index(rest, ": ")
		if idx < 0 {
			invalid("a line that is not a frames entry")
			continue
		}
		key, rawVal := rest[:idx], rest[idx+2:]
		pos, known := frameKeyOrder[key]
		if !known {
			if marker {
				flush()
				cur = &frameEntry{last: -1, seen: map[string]bool{}}
			}
			invalid("an unknown key")
			continue
		}
		if marker != (pos == 0) {
			invalid("a key out of place")
			continue
		}
		if marker {
			flush()
			cur = &frameEntry{last: -1, seen: map[string]bool{}}
		}
		if cur == nil || cur.stray {
			invalid("a key out of place")
			continue
		}
		val := unquoteScalar(rawVal)
		// A zodiac that is not in the writer's form is the zodiac's problem
		// alone (C6, house-chooser C11b): its field is dropped below and the
		// entry's houses stand. Every other key still loses the entry.
		canonical := yamlEscape(val) == rawVal
		if !canonical && key != "zodiac" {
			cur.fail("a value that is not in the writer's form")
			continue
		}
		if pos <= cur.last && !marker {
			cur.fail("a key repeated or out of order")
			continue
		}
		cur.last = pos
		cur.seen[key] = true
		switch key {
		case "file":
			if !validFrameFile(val) {
				cur.fail("a file that is not a single base name")
			}
			cur.rec.File = val
		case "slot":
			if val != "floating" {
				cur.fail("an unknown slot")
			}
			cur.rec.Floating = true
		case "when":
			t, ok := parseSessionWhen(val)
			if !ok || cur.seen["slot"] {
				cur.fail("a when that does not belong or is not a moment")
			}
			cur.rec.When = t
		case "houses":
			h, ok := recipe.ParseHouseSystem(val)
			if !ok {
				cur.fail("an unknown house system")
			}
			cur.rec.Houses, cur.rec.HasHouses = h, ok
		case "from":
			if val != FrameFromChosen && val != FrameFromFile {
				cur.fail("an unknown from")
			}
			cur.rec.From = val
		case "zodiac":
			if canonical && validZodiac(val) {
				cur.rec.Zodiac, cur.rec.HasZodiac = val, true
			} else {
				cur.zodBad = true
			}
		}
	}
	flush()
	// A line past the scanner's limit stops the scan before the end of the
	// text. If that happens inside the frames block, the records after it
	// were never read, and a block that merely looked short would be
	// rewritten without them at the next quit: the end is counted and said,
	// like a bad entry (transfer bugs/20261017T0300Z). A line in a later,
	// unrelated block is not the frames block's problem. LoadSession never
	// reaches this -- parseSessionV2 refuses such a file first -- so it
	// guards the parser itself, kept as a standalone-parser invariant.
	if err := sc.Err(); err != nil && inBlock && !skipBlock {
		dropped++
		notes = append(notes, "the frames block ended early at a line the reader could not finish ("+err.Error()+"): the records after it were not read")
	}
	return frames, dropped, notes
}
