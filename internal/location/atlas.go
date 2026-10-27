package location

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

var atlasRow = regexp.MustCompile(`^\s*(-?\d+\.\d+)\s+(-?\d+\.\d+)\s+([A-Za-z]{2})\s+.*$`)

// systemAtlasPaths are the system-wide tiers, in order: the default install
// and the Debian/Ubuntu packages. A variable only so a test can stand a temp
// file in for them (TestAtlasPath_SystemFallbackOrder).
var systemAtlasPaths = []string{
	"/usr/local/share/astrolog/atlas.as",
	"/usr/share/astrolog/atlas.as",
}

// AtlasPath returns the resolved on-disk path to astrolog's atlas.as.
//
// Search order:
//  1. $SEXTILE_ATLAS_PATH (env override; absolute path expected)
//  2. $XDG_DATA_HOME/sextile/astrolog/atlas.as (Sextile's own data root)
//  3. $XDG_DATA_HOME/astrolog/atlas.as (a real astrolog user install)
//  4. ./atlas.as (cwd — the convention astrolog itself honours)
//  5. <dir-of-sextile-binary>/atlas.as
//  6. /usr/local/share/astrolog/atlas.as (default install)
//  7. /usr/share/astrolog/atlas.as (Debian/Ubuntu packages)
//
// Tier 2 is a sibling of the session and chart directories under Sextile's own
// data root; scripts/install-astrolog-data.sh assembles it. It is read here and
// never written — nothing in this package creates it, and no chart data is ever
// placed in it.
//
// A user-local data directory outranks everything except the explicit
// override. That is deliberate and it has a cost worth stating: a portable
// bundle — a sextile binary shipped beside its own atlas.as, which is the mode
// that works where neither Go nor a system astrolog is installed — is
// overridden on any machine that already carries a populated
// $XDG_DATA_HOME/astrolog. The bundle still wins by absence, which is the
// common case for that deployment; where both exist, user-local is chosen.
//
// On miss, the returned error lists every searched path and points at the
// env override — so the user can fix it without grepping the source.
func AtlasPath() (string, error) {
	var tried []string
	check := func(path string) (string, bool) {
		tried = append(tried, path)
		if atlasReadable(path) == nil {
			return path, true
		}
		return "", false
	}

	if path := os.Getenv("SEXTILE_ATLAS_PATH"); path != "" {
		// Sticky override: if the user pointed at a specific path, do NOT
		// silently fall through to defaults — that hides typos and surprises
		// the operator with chart data from a different atlas than they
		// intended.
		if hit, ok := check(path); ok {
			return hit, nil
		}
		return "", fmt.Errorf("SEXTILE_ATLAS_PATH=%q not readable: %w", path, os.ErrNotExist)
	}
	if xdgData := xdgDataHome(); xdgData != "" {
		if hit, ok := check(filepath.Join(xdgData, "sextile", "astrolog", "atlas.as")); ok {
			return hit, nil
		}
		if hit, ok := check(filepath.Join(xdgData, "astrolog", "atlas.as")); ok {
			return hit, nil
		}
	}
	if cwd, err := os.Getwd(); err == nil {
		if hit, ok := check(filepath.Join(cwd, "atlas.as")); ok {
			return hit, nil
		}
	}
	if exe, err := os.Executable(); err == nil {
		if hit, ok := check(filepath.Join(filepath.Dir(exe), "atlas.as")); ok {
			return hit, nil
		}
	}
	for _, path := range systemAtlasPaths {
		if hit, ok := check(path); ok {
			return hit, nil
		}
	}

	return "", fmt.Errorf(
		"atlas.as not found in any of: %s — set SEXTILE_ATLAS_PATH to override, "+
			"or drop a copy next to the sextile binary: %w",
		strings.Join(tried, ", "),
		os.ErrNotExist,
	)
}

// xdgDataHome returns $XDG_DATA_HOME if set, else $HOME/.local/share, else "".
func xdgDataHome() string {
	if v := os.Getenv("XDG_DATA_HOME"); v != "" {
		return v
	}
	if h := os.Getenv("HOME"); h != "" {
		return filepath.Join(h, ".local", "share")
	}
	return ""
}

// LoadAtlas parses the atlas file at path and returns the entries in source-file order.
func LoadAtlas(path string) ([]PlaceInfo, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open atlas %q: %w", path, err)
	}
	defer f.Close()

	var entries []PlaceInfo
	var tz string
	// Rows that LOOK like data (the regex accepts any whitespace) but that the
	// tab split rejects. If every one of them is rejected the file is not in
	// the format this parser reads, and saying so is the difference between
	// "this atlas has no cities" and "this is not an atlas".
	looksLikeData := 0

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		switch line[0] {
		case ';', '@':
			continue
		}

		match := atlasRow.FindStringSubmatch(line)
		if match == nil {
			continue
		}

		// The atlas is TAB-separated and its COLUMN COUNT says whether a row
		// carries a timezone:
		//
		//	lon <TAB> lat <TAB> country <TAB> name                 -- inherits
		//	lon <TAB> lat <TAB> country <TAB> name <TAB> timezone  -- explicit
		//
		// Measured against the shipped atlas.as (2026-09-20): 33,297 rows of
		// four columns, 405 of five, none of three or six, and all 356
		// distinct timezone values load. So the count is an exact
		// discriminator and nothing here has to guess.
		//
		// It replaced one that did guess. The previous parser split on ANY
		// whitespace -- which destroys the column count, because a city name
		// contains spaces -- and then took the last field as a timezone if it
		// contained a slash. City names contain slashes too: Biel/Bienne,
		// Neustadt/Sued, Fenway/Kenmore, Reichenbach/Vogtland. For those rows
		// the name was eaten as the zone, and `tz = rowTZ` carried the bogus
		// value forward to every later row that inherits, so 831 entries --
		// Windsor Ontario, Chur, Suzano, Niederkassel -- ended up with a city
		// name where an IANA zone belongs. Such a chart would not anchor
		// ("an unknown timezone"), and everywhere that falls back silently on
		// LoadLocation showed its time in the wrong zone without saying so.
		looksLikeData++
		cols := strings.Split(line, "\t")
		if len(cols) < 4 {
			continue
		}

		lon, err := strconv.ParseFloat(match[1], 64)
		if err != nil {
			continue
		}
		lat, err := strconv.ParseFloat(match[2], 64)
		if err != nil {
			continue
		}

		// An explicit fifth column sets the zone for this row and every row
		// after it that has only four; anything else inherits. An empty fifth
		// column is treated as absent rather than as a zone of "", which
		// would blank the inheritance for the rest of the file.
		rowTZ := tz
		if len(cols) >= 5 {
			if z := strings.TrimSpace(cols[4]); z != "" {
				rowTZ = z
			}
		}

		entries = append(entries, PlaceInfo{
			Name:    strings.TrimSpace(cols[3]),
			Country: upperASCII(match[3]),
			Lat:     lat,
			Lon:     -lon,
			TZ:      rowTZ,
		})
		tz = rowTZ
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan atlas %q: %w", path, err)
	}
	// An atlas whose every data row was rejected is a format mismatch, not an
	// empty atlas. Returned as an error because the callers treat an empty
	// result as "no place list" and carry on -- which would present a wrong
	// format as a missing one, and leave every chart unplaceable with no clue
	// why.
	if len(entries) == 0 && looksLikeData > 0 {
		return nil, fmt.Errorf("atlas %q: %d rows look like data but none are tab-separated into at least 4 columns (lon, lat, country, name). This parser reads astrolog's tab-separated atlas.as", path, looksLikeData)
	}

	return entries, nil
}

func atlasReadable(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("%q: %w", path, os.ErrNotExist)
		}
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%q: %w", path, os.ErrNotExist)
	}

	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("%q: %w", path, os.ErrNotExist)
		}
		return err
	}
	return f.Close()
}

func upperASCII(s string) string {
	b := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'a' && c <= 'z' {
			c -= 'a' - 'A'
		}
		b[i] = c
	}
	return string(b)
}
