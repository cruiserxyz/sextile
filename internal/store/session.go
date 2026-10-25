package store

import (
	"bufio"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// session.go owns the one file Sextile writes for itself: what the workspace
// was holding when the operator quit, so that relaunching puts it back.
//
// It is MACHINE-MANAGED, which is what separates it from preferences.yaml and
// from a chart file. Sextile owns every byte, rewrites the whole file on quit,
// and preserves nothing it did not author — no comment, no unknown key, no
// unknown block. That is why it lives under SessionDir() rather than beside
// the files the operator writes by hand: gate 10 protects what the operator
// authored, and nobody authors this.
//
// The file names its chart THREE ways over, because they fail differently: by
// library `id`, by the `file` it was read from, and by a full snapshot of that
// chart's moment and place. The id is the identity and the file is the locator
//; either can change without the other, since astrolog erases a
// a chart file's `;sextile-id:` every time it rewrites the file while leaving the
// path alone. The file is what lets an edited chart file win on relaunch, the id
// is the library identity the next quit writes back, and the snapshot is what
// lets a deleted library file still come back.

const sessionFile = "session.yaml"

// SessionChart is the anchored chart as the session file remembers it: which
// library row it came from, what it is called, and the moment and place it was
// cast for.
//
// It is deliberately NOT a NamedChart. A listing row carries no birth moment
// precisely so there is only ever one copy of that fact, read from the
// chart file at pick time. This type is that resolved pick, written down — a
// snapshot with a known staleness, not a second listing.
type SessionChart struct {
	ID    string             // library slug; empty when the anchor has no library row
	Name  string             // chart display label, e.g. "Constructed One"
	When  time.Time          // the authoritative birth moment, as resolved
	Place location.PlaceInfo // the casting place

	// Source is which file this chart was read from, and is the LOCATOR
	// half of naming a chart — ID is the identity half. They are recorded
	// separately because they fail separately: astrolog erases a chart file's
	// `;sextile-id:` whenever it rewrites the file, so
	// an id can change while the file stays exactly where it was. A restore
	// that only knew the id would lose the chart to the astrologer's own
	// edit; one that only knew the file could not tell that the file now
	// holds somebody else.
	//
	// Zero for an anchor with no library row at all, in which case the
	// snapshot beside it is the whole record.
	Source ChartSource

	// Label is what this chart's reports print as its name and place. It is
	// NOT part of the session file: the snapshot's Name is a display label that
	// also holds the file stem of an untitled chart, so a restored chart sends
	// no name until its file has been read again.
	Label ReportLabel
}

// Usable reports whether this record can anchor a workspace on its own: it has
// a label, a moment, a named place, coordinates that are places rather than
// arithmetic, and a timezone the host can load.
//
// It is the ONE predicate for that question, so a chart arriving from the
// library is held to the same bar as one arriving from the snapshot. Without
// that, a live chart file broken in a way LoadCharts still lists — no `place:`,
// a `lat:` of NaN — would outrank a perfectly good snapshot and anchor a
// workspace the compute cannot use.
//
// It cannot see whether a key was ABSENT or merely unparsable, because 0 is a
// real coordinate; a reader that still knows checks presence beside this.
func (c SessionChart) Usable() bool {
	return c.UnusableReason() == ""
}

// UnusableReason names the FIRST thing Usable found missing, in the operator's
// vocabulary, or "" when the record can anchor. Usable is defined in terms of
// it, so the two cannot drift into disagreeing about the same file.
//
// One reason and not a list, unlike NamedChart.IncompleteReason: this fragment
// is read on the error lane, whose line width is a contract, and it names the
// next thing to go and fix. Repair that and the following resolve names what is
// behind it, in the order the checks are written here.
//
// It exists because the library lists on a weaker bar than anchoring needs, by
// design, so a perfectly
// pickable row can still be uncastable — and a pick that refuses has to say
// which of the five it was.
func (c SessionChart) UnusableReason() string {
	switch {
	case c.Name == "":
		return "no name"
	case c.When.IsZero():
		return "no birth time"
	case c.Place.Name == "":
		return "no birth place"
	case !finite(c.Place.Lat) || !finite(c.Place.Lon):
		return "no numeric coordinates"
	case !loadableTimezone(c.Place.TZ):
		return "an unknown timezone"
	default:
		return ""
	}
}

// sessionVersion is the generation SaveSession writes. LoadSession reads this
// 1 to 3: version 1 held a single chart, version 2 holds a `charts:` sequence,
// one entry per anchored slot in list order, version 3 adds one `frames:`
// sequence (session_frames.go). It is the
// session file's own constant rather than SchemaVersion, which versions the
// other files and did not change.
const sessionVersion = 3

// SessionSlot is one anchored slot as the session file remembers it: the chart
// it holds, and the selection and body marks it carried.
//
// Usable is a read-side diagnostic — LoadSession rebuilds it from the entry's
// text and SaveSession ignores it. A slot that is not usable could not anchor a
// workspace on its own and is dropped by whoever restores, which counts it.
type SessionSlot struct {
	Chart    SessionChart
	Usable   bool
	Selected bool
	Bodies   []aspect.Body
}

// Session is one relaunch's worth of workspace state.
//
// Slots are the anchored slots in list order; empty when the operator quit
// with nothing anchored. The floating slot's own time is deliberately absent:
// a relaunch always shows current transits, so it is recreated at now
// (operator decision, 2026-08-25). Its selection and body marks are kept.
//
// Version, DroppedSelections and UnknownBodies are read-side diagnostics —
// LoadSession rebuilds them from the file's text and SaveSession ignores them.
type Session struct {
	// Version is the generation the file declared; zero on a hand-built
	// Session. A version-1 file restores exactly as it always did, including
	// the line it says when its one chart cannot.
	Version int

	Slots []SessionSlot

	// FloatingSelected is whether the floating slot was selected.
	FloatingSelected bool

	// FloatingBodies are the bodies marked in the floating slot, exactly as the
	// file listed them — duplicates included. Collapsing those belongs to
	// whoever applies them, since applying a name twice would toggle a
	// selection back off.
	FloatingBodies []aspect.Body

	// UnknownBodies names every body name the reader could not parse, in
	// first-encounter order across the slots in list order and then the
	// floating slot, de-duplicated. It exists so a relaunch can say what it
	// skipped instead of silently dropping a selection.
	UnknownBodies []string

	// DroppedSelections is how many selections beyond MaxSelectedSlots the
	// reader turned off: the first six selected in list order are kept.
	DroppedSelections int

	// Frames are the version-3 `frames:` records, first-wins. DroppedFrames is
	// how many entries (or blocks, or fields) the reader dropped, and
	// FrameNotes names each; a dropped entry makes the file count as refused
	// for the specification C1.
	Frames        []FrameRecord
	DroppedFrames int
	FrameNotes    []string
}

// SessionPath returns the session file's path: SessionDir()/session.yaml.
func SessionPath() string {
	return filepath.Join(SessionDir(), sessionFile)
}

// SaveSession writes the whole session file at SessionPath(), creating
// SessionDir() when it is absent. The write is a same-directory tmpfile plus
// rename, so a relaunch reads either the previous session or this one and
// never a half-written file.
//
// The write is faithful: every field is written as handed, and nothing is
// normalized away. `chart:` names the anchor and `selection:` describes the
// workspace, and the two are independent — a session with no chart record
// still has marked bodies worth remembering, and dropping them because no
// chart was named would lose operator state for a reason the operator cannot
// see.
func SaveSession(s Session) error {
	path := SessionPath()
	dir := filepath.Dir(path)
	if err := mkdirAllSynced(dir, 0o755); err != nil {
		return fmt.Errorf("store: SaveSession: mkdir: %w", err)
	}
	tmp, err := os.CreateTemp(dir, "session.*.yaml.tmp")
	if err != nil {
		return fmt.Errorf("store: SaveSession: tmpfile: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath) // safe no-op after a successful rename

	if _, err := tmp.WriteString(formatSession(s)); err != nil {
		tmp.Close()
		return fmt.Errorf("store: SaveSession: write: %w", err)
	}
	// Synced before the rename, so a power cut cannot leave the new name
	// holding bytes that never reached the disk.
	if err := syncTemp(tmp); err != nil {
		tmp.Close()
		return fmt.Errorf("store: SaveSession: sync: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("store: SaveSession: close: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("store: SaveSession: rename: %w", err)
	}
	syncDir(dir) // the new name itself, not only its bytes
	return nil
}

// formatSession renders the whole file. The conventions are charts.go's — the
// version line first, values through yamlEscape, floats through FormatFloat's
// shortest round-trip form, the moment as RFC3339 in UTC.
//
// One `charts:` sequence holds an entry per anchored slot in list order, then a
// `floating:` block holds the floating slot's selection. Empty body lists are
// written as no key rather than as an empty one: an absent list and an empty
// list mean the same thing to the reader, and the shorter file is the one an
// operator can read at a glance.
func formatSession(s Session) string {
	var b strings.Builder
	fmt.Fprintf(&b, "version: %d\n", sessionVersion)
	if len(s.Slots) > 0 {
		b.WriteString("charts:\n")
	}
	for _, slot := range s.Slots {
		c := slot.Chart
		fmt.Fprintf(&b, "  - id: %s\n", yamlEscape(c.ID))
		// The locator is written only when there IS one this build reads: a
		// chart file. A record with no file, or one from a session written while
		// YAML charts existed (kind: yaml, now ChartKindNone), writes no
		// locator at all -- an absent pair reads back as exactly that, and a
		// `.yaml` path beside an empty kind would claim a file nothing reads.
		if c.Source.Kind == ChartKindFile && c.Source.File != "" {
			fmt.Fprintf(&b, "    kind: %s\n", yamlEscape(c.Source.Kind.String()))
			fmt.Fprintf(&b, "    file: %s\n", yamlEscape(c.Source.File))
		}
		fmt.Fprintf(&b, "    name: %s\n", yamlEscape(c.Name))
		fmt.Fprintf(&b, "    when: %s\n", yamlEscape(c.When.UTC().Format(time.RFC3339)))
		fmt.Fprintf(&b, "    place: %s\n", yamlEscape(c.Place.Name))
		fmt.Fprintf(&b, "    country: %s\n", yamlEscape(c.Place.Country))
		fmt.Fprintf(&b, "    lat: %s\n", strconv.FormatFloat(c.Place.Lat, 'f', -1, 64))
		fmt.Fprintf(&b, "    lon: %s\n", strconv.FormatFloat(c.Place.Lon, 'f', -1, 64))
		fmt.Fprintf(&b, "    tz: %s\n", yamlEscape(c.Place.TZ))
		fmt.Fprintf(&b, "    selected: %t\n", slot.Selected)
		// Always written, even empty: the last line of an entry is what tells a
		// reader the entry was not cut short.
		fmt.Fprintf(&b, "    bodies: %s\n", yamlEscape(bodyNames(slot.Bodies)))
	}
	b.WriteString("floating:\n")
	fmt.Fprintf(&b, "  selected: %t\n", s.FloatingSelected)
	if names := bodyNames(s.FloatingBodies); names != "" {
		fmt.Fprintf(&b, "  bodies: %s\n", names)
	}
	writeFrames(&b, s.Frames)
	return b.String()
}

// bodyNames joins a body list into the file's comma-separated wire form.
// BodyUnknown has no name to write and is dropped rather than written as
// "unknown", which would read back as a skipped body on the next launch.
func bodyNames(bodies []aspect.Body) string {
	names := make([]string, 0, len(bodies))
	for _, b := range bodies {
		if b == aspect.BodyUnknown {
			continue
		}
		names = append(names, b.String())
	}
	return strings.Join(names, ",")
}

// LoadSession reads the session file. It returns (zero, false, nil) when there
// is no session file — the ordinary first-run case, and not an error — and
// (zero, false, err) when the file exists but this build must not act on it.
//
// Unlike preferences.yaml, an ABSENT version line is refused here rather than
// read as generation 1. Versioning predates this format: every session file
// that has ever existed declares one, so a file that does not is not an old
// Sextile's session, it is something else wearing the name.
//
// That version line is the WHOLE of what this reader can refuse for being
// unparsable, and deliberately so. Below it the file is read the way every
// other reader in this package reads one — line by line, tolerantly — because
// C26 requires unknown keys and unknown blocks to be ignored rather than
// rejected. A trailing block this build has never heard of is therefore read
// past, not treated as corruption.
func LoadSession() (Session, bool, error) {
	path := SessionPath()
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Session{}, false, nil
		}
		return Session{}, false, fmt.Errorf("store: LoadSession: %w", err)
	}
	return parseSessionFile(path, b)
}

// parseSessionFile is LoadSession's judgement of bytes already read from path.
func parseSessionFile(path string, b []byte) (Session, bool, error) {
	text := string(b)
	if !declaresVersion(text) {
		return Session{}, false, fmt.Errorf("store: LoadSession: %s declares no generation", path)
	}
	v, err := schemaVersionOf(text)
	if err != nil {
		return Session{}, false, fmt.Errorf("store: LoadSession: %s: %w", path, err)
	}
	if err := checkSessionVersion(v); err != nil {
		return Session{}, false, fmt.Errorf("store: LoadSession: %s: %w", path, err)
	}
	var s Session
	if v == 1 {
		s, err = parseSessionV1(text)
	} else {
		s, err = parseSessionV2(text)
		s.Version = v
		if v == sessionVersion && err == nil {
			s.Frames, s.DroppedFrames, s.FrameNotes = parseFrames(text)
		}
	}
	if err != nil {
		return Session{}, false, fmt.Errorf("store: LoadSession: %s: %w", path, err)
	}
	return s, true, nil
}

// sessionWhen is the writer's own form of a moment: RFC 3339 in UTC, whose year
// is negative for a BC chart and runs to as many digits as the year has — Go
// formats both, and time.Parse reads neither. The writer has no year bound, so
// the reader takes any digit count from four up, and nothing the writer never
// emits.
var sessionWhen = regexp.MustCompile(`^(-?[0-9]{4,})-([0-9]{2})-([0-9]{2})T([0-9]{2}):([0-9]{2}):([0-9]{2})Z$`)

// parseSessionWhen reads a moment as time.RFC3339 does, and also the writer's
// negative and five-digit years. The result must format back to the same text,
// which rejects a date that does not exist (February 30th).
func parseSessionWhen(v string) (time.Time, bool) {
	if t, err := time.Parse(time.RFC3339, v); err == nil {
		return t, true
	}
	m := sessionWhen.FindStringSubmatch(v)
	if m == nil {
		return time.Time{}, false
	}
	n := make([]int, 6)
	for i := range n {
		var err error
		if n[i], err = strconv.Atoi(m[i+1]); err != nil {
			return time.Time{}, false
		}
	}
	t := time.Date(n[0], time.Month(n[1]), n[2], n[3], n[4], n[5], 0, time.UTC)
	return t, t.Format(time.RFC3339) == v
}

// sessionMaxLine is the longest line (content bytes, newline not counted) either session reader will hold: 1 MiB,
// far past any name or place a chart carries. A longer one is refused whole
// (ErrSessionLineTooLong).
const sessionMaxLine = 1 << 20

// ErrSessionLineTooLong is a session file with a line the reader's scanner
// cannot hold. The file is refused whole rather than read up to it: reading on
// past it would lose every chart after it, and the next quit would write the
// shorter list over the file.
var ErrSessionLineTooLong = errors.New("a line is too long to read")

// scanErr is what a finished scan says about the file.
func scanErr(sc *bufio.Scanner) error {
	if err := sc.Err(); err != nil {
		return fmt.Errorf("%w (%v)", ErrSessionLineTooLong, err)
	}
	return nil
}

// checkSessionVersion is the session file's compatibility rule: exactly the
// integers 1 to 3. A file from the FUTURE is refused, and so is anything below
// the floor — a half-read client record is worse than none.
func checkSessionVersion(v int) error {
	switch {
	case v >= 1 && v <= sessionVersion:
		return nil
	case v > sessionVersion:
		return fmt.Errorf("written by a newer Sextile (generation %d; this build reads 1 to %d)", v, sessionVersion)
	default:
		return fmt.Errorf("unreadable generation %d", v)
	}
}

// chartFields reads one chart's keys, the same eleven in either generation, and
// remembers which were present, since 0 is a real coordinate and an absent
// `lat:` is not the same fact as an unparsable one.
type chartFields struct {
	c                                 SessionChart
	sawName, sawWhen, sawPlace, sawTZ bool
	okWhen, okLat, okLon              bool
}

// set reads one key of a chart. It reports whether the key was one of the
// chart's own; the callers handle the rest.
func (f *chartFields) set(key, val string) bool {
	c := &f.c
	switch key {
	case "id":
		c.ID = val
	case "kind":
		c.Source.Kind = ParseChartKind(val)
	case "file":
		c.Source.File = val
	case "name":
		c.Name = val
		f.sawName = true
	case "when":
		f.sawWhen = true
		if t, ok := parseSessionWhen(val); ok {
			c.When = t
			f.okWhen = true
		}
	case "place":
		c.Place.Name = val
		f.sawPlace = true
	case "country":
		c.Place.Country = val
	case "lat":
		if v, err := parseFinite(val); err == nil {
			c.Place.Lat = v
			f.okLat = true
		}
	case "lon":
		if v, err := parseFinite(val); err == nil {
			c.Place.Lon = v
			f.okLon = true
		}
	case "tz":
		c.Place.TZ = val
		f.sawTZ = true
	default:
		return false
	}
	return true
}

// complete reports whether every key was present and parsed and the record
// could anchor on its own. Presence is checked separately from parseability: a
// lat: line that says "abc" leaves Lat at 0, which is a real coordinate, so the
// file having tried and failed has to be its own fact.
func (f *chartFields) complete() bool {
	return f.sawName && f.sawWhen && f.okWhen && f.sawPlace &&
		f.okLat && f.okLon && f.sawTZ && f.c.Usable()
}

// parseSessionV1 reads a version-1 file — one `chart:` block and a `selection:`
// block — into the one-slot shape. Everything else in it — unknown keys,
// unknown blocks, comments — is ignored, and since the writer regenerates the
// whole file it is not preserved either (C26).
//
// Version 1's one comparison flag selected the anchored+floating pair, so it
// becomes both slots' selection. With no usable chart it has nothing to select
// and the anchored marks have nowhere to go.
func parseSessionV1(text string) (Session, error) {
	s := Session{Version: 1}
	var f chartFields
	var comparison bool
	var anchoredBodies []aspect.Body
	var sawChart bool
	var unknownAnchored, unknownFloating []string

	inChart, inSelection := false, false
	scanner := bufio.NewScanner(strings.NewReader(text))
	scanner.Buffer(make([]byte, 0, 64*1024), sessionMaxLine+1) // +1: the newline after a full line
	for scanner.Scan() {
		raw := scanner.Text()
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		// A line in column zero opens a new top-level block and closes
		// whichever one was open — including `version:`, which belongs to no
		// block. topLevelKey is the same boundary rule the writer uses.
		if key, ok := topLevelKey(raw); ok {
			inChart = key == "chart"
			inSelection = key == "selection"
			sawChart = sawChart || inChart
			continue
		}
		key, val, ok := splitKV(trimmed)
		if !ok {
			continue
		}
		switch {
		case inChart:
			f.set(key, val)
		case inSelection:
			switch key {
			case "comparison":
				comparison = val == "true"
			case "anchored_bodies":
				anchoredBodies, unknownAnchored = parseBodyList(val)
			case "floating_bodies":
				s.FloatingBodies, unknownFloating = parseBodyList(val)
			}
		}
	}

	if err := scanErr(scanner); err != nil {
		return Session{}, err
	}
	if sawChart {
		usable := f.complete()
		s.Slots = []SessionSlot{{Chart: f.c, Usable: usable, Selected: comparison, Bodies: anchoredBodies}}
		s.FloatingSelected = usable && comparison
	}
	// Anchored names come first however the file ordered its keys, so the
	// line a relaunch prints reads the same as the slots it describes.
	s.UnknownBodies = dedupe(append(unknownAnchored, unknownFloating...))
	return s, nil
}

// v2KeyOrder is the order the writer emits an entry's keys in. A reader that
// accepts only the exact layout accepts them only in this order, each at most
// once, which is what makes a swapped, doubled or cut-short entry detectable.
var v2KeyOrder = map[string]int{
	"id": 0, "kind": 1, "file": 2, "name": 3, "when": 4, "place": 5,
	"country": 6, "lat": 7, "lon": 8, "tz": 9, "selected": 10, "bodies": 11,
}

// v2Entry is one `charts:` entry being read.
type v2Entry struct {
	f       chartFields
	slot    SessionSlot
	last    int // v2KeyOrder of the last key read, -1 before the marker
	lastKey string
	seen    map[string]bool
	bad     bool
	// stray marks a span that no valid marker opened: it still counts as one
	// dropped entry.
	stray bool
}

// canonicalValue reports whether raw is a value in the form the writer emits
// for key, and returns it decoded. Anything else — a quote the writer would not
// have used, a float spelt another way, stray spaces — is not the writer's.
func canonicalValue(key, raw string) (string, bool) {
	switch key {
	case "lat", "lon":
		f, err := parseFinite(raw)
		return raw, err == nil && strconv.FormatFloat(f, 'f', -1, 64) == raw
	case "selected":
		return raw, raw == "true" || raw == "false"
	case "when":
		v := unquoteScalar(raw)
		t, ok := parseSessionWhen(v)
		return v, ok && yamlEscape(t.UTC().Format(time.RFC3339)) == raw
	case "bodies":
		v := unquoteScalar(raw)
		if yamlEscape(v) != raw {
			return "", false
		}
		if v == "" {
			return v, true
		}
		bodies, unknown := parseBodyList(v)
		return v, len(unknown) == 0 && bodyNames(bodies) == v
	case "kind":
		return raw, raw == ChartKindFile.String()
	default: // id, file, name, place, country, tz
		v := unquoteScalar(raw)
		return v, yamlEscape(v) == raw
	}
}

// parseSessionV2 reads a version-2 file: a `charts:` sequence and a `floating:`
// block. session.yaml is machine-managed, so the charts block is read STRICTLY
// (operator, 2026-10-04, "option 1, strict"): an entry is exactly one marker
// line `  - id: <value>` followed by `    <key>: <value>` lines, keys from the
// writer's set, in the writer's order, each at most once, values in the form the
// writer emits, ending with `bodies:`. Any other line inside `charts:` — other
// indentation, a tab, a column-zero or bare `-`, an unknown or repeated or
// out-of-order key, a nested block, a blank or comment, a value in another form
// — makes the entry it falls in unusable, and the lines after it are skipped to
// the next valid marker: the whole span is ONE dropped entry, never partially
// restored. Top-level keys and `floating:` are read as before (C26).
//
// More than MaxSelectedSlots selections — counting the usable entries in list
// order and then the floating slot — keeps the first six and counts the rest.
func parseSessionV2(text string) (Session, error) {
	s := Session{Version: sessionVersion}
	var cur *v2Entry

	flush := func() {
		if cur == nil {
			return
		}
		e := cur
		cur = nil
		required := []string{"id", "name", "when", "place", "country", "lat", "lon", "tz", "selected", "bodies"}
		for _, k := range required {
			if !e.seen[k] {
				e.bad = true
			}
		}
		if e.seen["kind"] != e.seen["file"] {
			e.bad = true
		}
		e.slot.Chart = e.f.c
		e.slot.Usable = !e.bad && !e.stray && e.f.complete() && (e.f.c.ID != "" || e.f.c.Source.File != "")
		s.Slots = append(s.Slots, e.slot)
	}
	// invalid is a line that is not the writer's: it spoils the entry it falls
	// in, or opens a dropped span when it falls in none.
	invalid := func() {
		if cur == nil {
			cur = &v2Entry{bad: true, stray: true, last: -1}
			return
		}
		cur.bad = true
	}

	inCharts, inFloating := false, false
	scanner := bufio.NewScanner(strings.NewReader(text))
	scanner.Buffer(make([]byte, 0, 64*1024), sessionMaxLine+1) // +1: the newline after a full line
	for scanner.Scan() {
		raw := scanner.Text()
		if key, ok := topLevelKey(raw); ok && !strings.HasPrefix(raw, "-") {
			flush()
			inCharts = key == "charts"
			inFloating = key == "floating"
			continue
		}
		if inCharts {
			var rest string
			marker := false
			switch {
			case strings.HasPrefix(raw, "  - ") && len(raw) > 4 && raw[4] != ' ' && raw[4] != '\t':
				rest, marker = raw[4:], true
			case strings.HasPrefix(raw, "    ") && len(raw) > 4 && raw[4] != ' ' && raw[4] != '\t':
				rest = raw[4:]
			default:
				invalid()
				continue
			}
			idx := strings.Index(rest, ": ")
			if idx < 0 {
				invalid()
				continue
			}
			key, rawVal := rest[:idx], rest[idx+2:]
			pos, known := v2KeyOrder[key]
			val, formOK := canonicalValue(key, rawVal)
			if !known || !formOK || marker != (key == "id") {
				invalid()
				continue
			}
			if marker {
				flush()
				cur = &v2Entry{last: -1, seen: map[string]bool{}}
			}
			if cur == nil || cur.bad {
				invalid()
				continue
			}
			// The writer's order, each key once; `file` only straight after `kind`.
			if pos <= cur.last || (cur.lastKey == "kind" && key != "file") || (key == "file" && cur.lastKey != "kind") {
				cur.bad = true
				continue
			}
			cur.last, cur.lastKey = pos, key
			cur.seen[key] = true
			switch key {
			case "selected":
				cur.slot.Selected = val == "true"
			case "bodies":
				cur.slot.Bodies, _ = parseBodyList(val)
			default:
				cur.f.set(key, val)
			}
			continue
		}
		trimmed := strings.TrimSpace(raw)
		if inFloating && trimmed != "" && !strings.HasPrefix(trimmed, "#") {
			key, val, ok := splitKV(trimmed)
			if !ok {
				continue
			}
			switch key {
			case "selected":
				s.FloatingSelected = val == "true"
			case "bodies":
				var unknown []string
				s.FloatingBodies, unknown = parseBodyList(val)
				s.UnknownBodies = dedupe(unknown)
			}
		}
	}
	if err := scanErr(scanner); err != nil {
		return Session{}, err
	}
	flush()

	kept := 0
	for i := range s.Slots {
		if !s.Slots[i].Usable || !s.Slots[i].Selected {
			continue
		}
		if kept == MaxSelectedSlots {
			s.Slots[i].Selected = false
			s.DroppedSelections++
			continue
		}
		kept++
	}
	if s.FloatingSelected {
		if kept == MaxSelectedSlots {
			s.FloatingSelected = false
			s.DroppedSelections++
		}
	}
	return s, nil
}

// parseBodyList splits the comma-separated wire form into bodies plus the
// names that answered to none. An empty entry is not a name and is skipped,
// so a trailing comma is not a skipped body.
func parseBodyList(val string) ([]aspect.Body, []string) {
	var bodies []aspect.Body
	var unknown []string
	for _, name := range strings.Split(val, ",") {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if b, ok := aspect.ParseBody(name); ok {
			bodies = append(bodies, b)
			continue
		}
		unknown = append(unknown, name)
	}
	return bodies, unknown
}

// parseFinite parses a coordinate and refuses the values that parse but are
// not places: NaN and the infinities. astrolog would take them and produce
// nonsense, which is worse than declining to restore the chart.
func parseFinite(val string) (float64, error) {
	f, err := strconv.ParseFloat(val, 64)
	if err != nil {
		return 0, err
	}
	if !finite(f) {
		return 0, fmt.Errorf("store: %q is not a coordinate", val)
	}
	return f, nil
}

// finite refuses the values that parse as float64 but are not places. astrolog
// would take a NaN latitude and produce nonsense, which is worse than declining
// to restore the chart at all.
func finite(f float64) bool { return !math.IsNaN(f) && !math.IsInf(f, 0) }

// loadableTimezone reports whether the host can load the IANA name. A chart
// cast in a timezone this machine does not have is not restorable here, and
// saying so beats casting it an hour off.
func loadableTimezone(tz string) bool {
	if tz == "" {
		return false
	}
	_, err := time.LoadLocation(tz)
	return err == nil
}

func dedupe(names []string) []string {
	if len(names) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(names))
	out := make([]string, 0, len(names))
	for _, n := range names {
		if _, ok := seen[n]; ok {
			continue
		}
		seen[n] = struct{}{}
		out = append(out, n)
	}
	return out
}
