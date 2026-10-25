// Package store is the state plane of the system.
// Passive storage only. Perimeter packages (ingest, ui) read and write here;
// active packages never touch it.
package store

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/cruiserxyz/sextile/internal/location"
)

const (
	preferencesFile = "preferences.yaml"
	sessionDirName  = "session"
)

// PreferencesPath returns the user-editable preferences file path.
// XDG-standard: $XDG_CONFIG_HOME/sextile/preferences.yaml
// Fallback:     $HOME/.config/sextile/preferences.yaml
func PreferencesPath() string {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		base = filepath.Join(os.Getenv("HOME"), ".config")
	}
	return filepath.Join(base, "sextile", preferencesFile)
}

// SessionDir returns the directory for machine-managed session state.
// XDG-standard: $XDG_DATA_HOME/sextile/session/
// Fallback:     $HOME/.local/share/sextile/session/
func SessionDir() string {
	base := os.Getenv("XDG_DATA_HOME")
	if base == "" {
		base = filepath.Join(os.Getenv("HOME"), ".local", "share")
	}
	return filepath.Join(base, "sextile", sessionDirName)
}

// Location reads the persisted PlaceInfo from preferences.yaml. Returns
// (zero, false) when the file is absent, unreadable, or has no `location:`
// block.
//
// The store is passive: errors below the file-not-exist threshold are
// swallowed silently — the perimeter (ingest) treats absence and parse
// failure identically (open the modal). Logging belongs upstream.
func Location() (location.PlaceInfo, bool) {
	return readLocation(PreferencesPath())
}

// LocationCandidate reads the location: block as a partial RawPlace,
// preserving the lat/lon presence bits the resolver needs. Returns
// (Candidate, false) when the file is absent or has no location: block.
//
// This is the resolver-aware variant of Location(). Where Location() applies
// "name + tz must be present" validation, LocationCandidate returns whatever
// keys were actually in the YAML — the resolver decides what's usable.
func LocationCandidate() (location.Candidate, bool) {
	raw, ok := readLocationRaw(PreferencesPath())
	if !ok {
		return location.Candidate{}, false
	}
	return location.Candidate{Source: "stored-location", Place: raw}, true
}

// SetLocation writes the PlaceInfo to preferences.yaml atomically (tmpfile
// + rename). Creates the parent directory if missing.
//
// The write owns the `location:` block and nothing else: every other line of
// the file — other top-level blocks, comments, blank lines, blocks this
// package has no parser for — is carried through byte-for-byte. Inside the
// block the reverse holds; it is regenerated, so comments and unrecognised
// keys there do not survive.
func SetLocation(p location.PlaceInfo) error {
	return writeLocation(PreferencesPath(), p)
}

// readLocation parses a tiny hand-rolled YAML subset:
//
//	location:
//	  name: Berlin
//	  country: DE
//	  lat: 52.5244
//	  lon: 13.4105
//	  tz: Europe/Berlin
//
// Indentation is tolerant (any whitespace prefix); the parser keys on the
// `name:` etc. line tags inside a `location:` block. A real yaml.v3 parser
// would be more robust; cp2 sticks to stdlib to keep go.mod clean.
// readLocationRaw returns the location: block contents with explicit
// presence markers. Caller decides what's usable.
func readLocationRaw(path string) (location.RawPlace, bool) {
	if _, err := readableSchema(path); err != nil {
		// A file this build cannot read is treated exactly as an absent one:
		// the perimeter opens the picker. Guessing at a newer generation's
		// location: block is the one outcome worse than asking again.
		return location.RawPlace{}, false
	}
	f, err := os.Open(path)
	if err != nil {
		return location.RawPlace{}, false
	}
	defer f.Close()

	var raw location.RawPlace
	inBlock := false
	sawAny := false
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if strings.HasPrefix(trimmed, "location:") {
			inBlock = true
			sawAny = true
			continue
		}
		if !inBlock {
			continue
		}
		if line == strings.TrimLeft(line, " \t") && !strings.HasPrefix(trimmed, "location:") {
			inBlock = false
			continue
		}
		key, val, ok := splitKV(trimmed)
		if !ok {
			continue
		}
		switch key {
		case "name":
			raw.Name = val
		case "country":
			raw.Country = val
		case "lat":
			if f, err := strconv.ParseFloat(val, 64); err == nil {
				raw.Lat = f
				raw.LatPresent = true
			}
		case "lon":
			if f, err := strconv.ParseFloat(val, 64); err == nil {
				raw.Lon = f
				raw.LonPresent = true
			}
		case "tz":
			raw.TZ = val
		}
	}
	if scanner.Err() != nil {
		return location.RawPlace{}, false
	}
	return raw, sawAny
}

func readLocation(path string) (location.PlaceInfo, bool) {
	if _, err := readableSchema(path); err != nil {
		return location.PlaceInfo{}, false
	}
	f, err := os.Open(path)
	if err != nil {
		return location.PlaceInfo{}, false
	}
	defer f.Close()

	var p location.PlaceInfo
	inBlock := false
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		raw := scanner.Text()
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "location:") {
			inBlock = true
			continue
		}
		if !inBlock {
			continue
		}
		// A new top-level key (no leading whitespace) ends the block.
		if raw == strings.TrimLeft(raw, " \t") && !strings.HasPrefix(line, "location:") {
			inBlock = false
			continue
		}
		key, val, ok := splitKV(line)
		if !ok {
			continue
		}
		switch key {
		case "name":
			p.Name = val
		case "country":
			p.Country = val
		case "lat":
			f, err := strconv.ParseFloat(val, 64)
			if err == nil {
				p.Lat = f
			}
		case "lon":
			f, err := strconv.ParseFloat(val, 64)
			if err == nil {
				p.Lon = f
			}
		case "tz":
			p.TZ = val
		}
	}
	if scanner.Err() != nil {
		return location.PlaceInfo{}, false
	}
	if p.Name == "" || !p.HasTimezone() {
		return location.PlaceInfo{}, false
	}
	return p, true
}

func splitKV(line string) (string, string, bool) {
	idx := strings.Index(line, ":")
	if idx < 0 {
		return "", "", false
	}
	key := strings.TrimSpace(line[:idx])
	val := unquoteScalar(strings.TrimSpace(line[idx+1:]))
	return key, val, true
}

// unquoteScalar reverses yamlEscape: a double-quoted scalar is decoded with
// strconv.Unquote (so embedded \" and \\ restore to " and \), a single-quoted
// scalar has its matched quotes stripped, and a bare scalar is returned as-is.
// The earlier naive strings.Trim(val, `"'`) corrupted any value carrying an
// escaped quote or backslash; this matches the writer, which quotes via
// strconv.Quote.
func unquoteScalar(v string) string {
	if len(v) >= 2 && v[0] == '"' && v[len(v)-1] == '"' {
		if uq, err := strconv.Unquote(v); err == nil {
			return uq
		}
	}
	if len(v) >= 2 && v[0] == '\'' && v[len(v)-1] == '\'' {
		return v[1 : len(v)-1]
	}
	return v
}

func writeLocation(path string, p location.PlaceInfo) error {
	body := fmt.Sprintf(
		"location:\n  name: %s\n  country: %s\n  lat: %s\n  lon: %s\n  tz: %s\n",
		yamlEscape(p.Name),
		yamlEscape(p.Country),
		strconv.FormatFloat(p.Lat, 'f', -1, 64),
		strconv.FormatFloat(p.Lon, 'f', -1, 64),
		yamlEscape(p.TZ),
	)
	// The write owns two blocks now: `version:` and `location:`. A file that
	// already declares a generation has that line rewritten in place; one that
	// does not gets it PREPENDED, so a Sextile-managed preferences.yaml always
	// opens by saying what it is. Prepending inserts a block and moves nobody
	// else's bytes — the preservation contract is about not dropping what this
	// package did not author, and nothing is dropped.
	return writePreferences(path, func(existing string) string {
		merged := existing
		if declaresVersion(merged) {
			merged = mergeBlock(merged, "version", schemaBlock())
		} else {
			merged = schemaBlock() + merged
		}
		return mergeBlock(merged, "location", body)
	})
}

// writePreferences is the one door every write to preferences.yaml goes
// through: it reads the file, lets merge turn its text into the new text, and
// replaces the file. merge decides WHAT is owned; everything here is about
// doing it safely, whoever the caller is.
func writePreferences(path string, merge func(existing string) string) error {
	return writePreferencesChecked(path, func(existing string) (string, error) {
		return merge(existing), nil
	})
}

// writePreferencesChecked is writePreferences for a merge that can refuse the
// file it is given: the error is returned and nothing is written.
func writePreferencesChecked(path string, merge func(existing string) (string, error)) error {
	// An edit made to the file between the read and the replace (the
	// operator's editor, a second Sextile) would be overwritten with the old
	// bytes of every block this package does not own, `clock:` included, so the
	// file is read AGAIN just before the rename and, if it moved, the merge is
	// redone from the latest bytes (the specification C11).
	const attempts = 4
	for i := 0; i < attempts; i++ {
		changed, err := writePreferencesOnce(path, merge)
		if err != nil {
			return err
		}
		if !changed {
			return nil
		}
	}
	return fmt.Errorf("store: write: %s kept changing while it was being written; nothing was replaced", path)
}

// writePreferencesOnce is one read-merge-replace. changed reports that the file
// was edited after the read, in which case nothing was replaced.
func writePreferencesOnce(path string, merge func(existing string) (string, error)) (changed bool, err error) {
	existing, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return false, fmt.Errorf("store: read: %w", err)
	}
	// Refuse to write into a file from a later generation. mergeBlock would
	// faithfully preserve its unknown blocks, but it would also stamp this
	// build's version over the newer one — leaving a file that lies about what
	// wrote it, which is worse than not persisting the setting.
	if v, verr := schemaVersionOf(string(existing)); verr != nil {
		return false, fmt.Errorf("store: write: %s: %w", path, verr)
	} else if cerr := checkSchema(v); cerr != nil {
		return false, fmt.Errorf("store: write: %s: %w", path, cerr)
	}
	merged, merr := merge(string(existing))
	if merr != nil {
		return false, merr
	}
	if err := mkdirAllSynced(filepath.Dir(path), 0o755); err != nil {
		return false, fmt.Errorf("store: mkdir: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "preferences.*.yaml.tmp")
	if err != nil {
		return false, fmt.Errorf("store: tmpfile: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath) // safe no-op after rename

	if _, err := tmp.WriteString(merged); err != nil {
		tmp.Close()
		return false, fmt.Errorf("store: write: %w", err)
	}
	// The replacement keeps the file's own mode (CreateTemp makes 0600).
	if fi, serr := os.Stat(path); serr == nil {
		if err := tmp.Chmod(fi.Mode().Perm()); err != nil {
			tmp.Close()
			return false, fmt.Errorf("store: chmod: %w", err)
		}
	}
	// Synced before the rename, so a power cut cannot leave the new name
	// holding bytes that never reached the disk.
	if err := syncTemp(tmp); err != nil {
		tmp.Close()
		return false, fmt.Errorf("store: sync: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return false, fmt.Errorf("store: close: %w", err)
	}
	if betweenReadAndRename != nil {
		betweenReadAndRename()
	}
	now, rerr := os.ReadFile(path)
	if rerr != nil && !errors.Is(rerr, os.ErrNotExist) {
		return false, fmt.Errorf("store: re-read: %w", rerr)
	}
	if string(now) != string(existing) {
		return true, nil
	}
	if err := renameFile(tmpPath, path); err != nil {
		return false, fmt.Errorf("store: rename: %w", err)
	}
	syncDir(filepath.Dir(path)) // the new name itself, not only its bytes
	return false, nil
}

// mergeBlock splices body — a whole newline-terminated top-level block — into
// existing, replacing the first block named name and leaving every other byte
// alone. When no such block is there, body is appended.
//
// The file is never parsed for this: it is cut into lines, the owned block's
// span is found, and the two verbatim halves are re-joined around the new
// body. Anything the rest of this package cannot read is therefore carried
// through untouched, which is the point.
func mergeBlock(existing, name, body string) string {
	lines := splitAfterNewlines(existing)
	start, end := -1, len(lines)
	for i, line := range lines {
		key, ok := topLevelKey(line)
		if !ok {
			continue
		}
		if start < 0 {
			if key == name {
				start = i
			}
			continue
		}
		end = i // the next top-level key closes the block
		break
	}

	if start < 0 {
		if existing == "" {
			return body
		}
		// An unterminated final line needs a separator before the append,
		// or the block header would land on the end of it.
		if !strings.HasSuffix(existing, "\n") {
			return existing + "\n" + body
		}
		return existing + body
	}

	// Blank lines and column-zero comments at the tail of the span read as
	// the next block's preamble, not as this block's body. Hand them back so
	// they keep their position (they would otherwise be regenerated away).
	for end > start+1 {
		tail := strings.TrimRight(lines[end-1], "\r\n")
		if tail == "" || strings.HasPrefix(tail, "#") {
			end--
			continue
		}
		break
	}
	return strings.Join(lines[:start], "") + body + strings.Join(lines[end:], "")
}

// splitAfterNewlines cuts s into lines that each keep their own terminator,
// so joining them again reproduces s exactly. The final line has no
// terminator when s does not end in one.
func splitAfterNewlines(s string) []string {
	var out []string
	for len(s) > 0 {
		i := strings.IndexByte(s, '\n')
		if i < 0 {
			return append(out, s)
		}
		out = append(out, s[:i+1])
		s = s[i+1:]
	}
	return out
}

// topLevelKey reports the key of a line that opens a top-level block: it
// starts in column zero, is neither blank nor a comment, and carries a colon.
//
// The key is returned unTRIMMED on purpose. The readers above start a block
// only on the exact prefix `location:`, so `location :` is a block they can
// never see — and a key this function trimmed into "location" would hand that
// block to the writer to overwrite. Writer and readers have to agree on what
// the owned block is, or the write eats a block nobody could read.
func topLevelKey(line string) (string, bool) {
	line = strings.TrimRight(line, "\r\n")
	if line == "" || line[0] == ' ' || line[0] == '\t' || line[0] == '#' {
		return "", false
	}
	idx := strings.Index(line, ":")
	if idx < 0 {
		return "", false
	}
	return line[:idx], true
}

// yamlEscape wraps a value in double-quotes when it contains a colon, leading
// whitespace, or other YAML-significant characters; otherwise returns it bare.
// Sextile's stored values are city names + IANA tz strings — the colon test
// catches "America/New_York" → no, "12:34" → yes. Conservative.
//
// A control character forces quoting too, and a NEWLINE is the reason. These
// readers are line-based, so a value carrying one would be written as two
// physical lines and read back truncated at the first — a chart label of
// "Alice\nBob" becomes "Alice", silently, in a file about a client. Quoting
// sends it through strconv.Quote, which unquoteScalar reverses exactly.
func yamlEscape(v string) string {
	if v == "" {
		return `""`
	}
	if strings.ContainsAny(v, ":#\"'") || strings.HasPrefix(v, " ") || strings.HasSuffix(v, " ") {
		return strconv.Quote(v)
	}
	if strings.ContainsFunc(v, func(r rune) bool { return r < 0x20 || r == 0x7f }) {
		return strconv.Quote(v)
	}
	return v
}
