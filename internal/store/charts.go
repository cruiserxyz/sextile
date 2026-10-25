package store

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/cruiserxyz/sextile/internal/env"
	"github.com/cruiserxyz/sextile/internal/location"
)

// ChartKind is which file a library row came from. It is half of a chart's
// source identity; the other half is the filename under ChartsDir().
type ChartKind uint8

const (
	// ChartKindNone is a record that names no chart file this build reads:
	// a session written while YAML charts existed (retired 2026-10-02,
	// the specification), or one with no `kind:` at all. Such a record
	// still restores from its snapshot; there is just no file to re-check.
	ChartKindNone ChartKind = iota
	// ChartKindFile is an astrolog chart-info file, the only kind of chart
	// the library holds. It is known by its first bytes, never by its name.
	ChartKindFile
)

// Wire names for ChartKind, as the session file writes them. They are their own
// constants rather than a %d of the enum: the session file is meant to be read
// by an operator, and a renumbered enum must never silently turn one kind of
// file into another on the next launch.
const chartKindFileWire = "file"

// String is the wire name this kind is written under: "file", or "" for a
// record naming no readable file. ParseChartKind reads both back to what
// they were, so a round trip through the session file changes nothing.
func (k ChartKind) String() string {
	if k == ChartKindFile {
		return chartKindFileWire
	}
	return ""
}

// ParseChartKind reads a wire name back. Anything but "file" -- the "yaml" or
// the earlier word for an Astrolog file that older sessions wrote, or no
// `kind:` at all -- is ChartKindNone.
func ParseChartKind(s string) ChartKind {
	if s == chartKindFileWire {
		return ChartKindFile
	}
	return ChartKindNone
}

// ChartSource is where a library row came from, and everything a resolve needs
// to reopen the exact file it was built from: the kind of file, and its name
// under ChartsDir().
//
// It deliberately carries NOTHING about the birth moment — not the file's text,
// not a parsed `when:`, not a UTC time, Unix seconds, a UTC offset or a Julian
// Day. The authoritative moment is obtained by reopening the file at pick time
// and never by reading it off a listing row, so there is no second copy of it
// to drift. An opaque handle that smuggled one back would break that in fact
// while satisfying it in letter.
type ChartSource struct {
	Kind ChartKind // which file grammar to reopen it with
	File string    // base filename under ChartsDir()
}

// NamedChart is one row of the chart library: a human label, the place, the
// display year, and where it came from. A chart is one astrolog
// chart-info file under ChartsDir().
//
// It carries no birth moment. Year, Month and Day are LABELS — the date a
// listing row shows (the specification C7) — and must never reach a compute: the authoritative moment comes from
// a resolve at pick time (lookup.ResolvePicked), which reopens Source. That
// separation is what lets the picker list a whole library without spawning
// astrolog for a fact no row displays.
type NamedChart struct {
	ID     string             // slug of the file stem; the library identity
	Name   string             // chart display label, e.g. "Constructed One"
	Year   int                // display year only; never an astrological input
	Month  int                // display month (1-12) beside Year; 0 when the file states no date
	Day    int                // display day beside Month; 0 when the file states no date
	Place  location.PlaceInfo // resolved casting place
	Source ChartSource        // where to reopen it for the authoritative moment

	// Untitled: the file states no title (an empty `-zi` first string, no
	// `-zi` line). Name then holds the file stem,
	// for display (the specification C3.1).
	Untitled bool
	// PlaceLabel is the place text the FILE states: the `-zi` second
	// string. Place.Name is the atlas city the library matched,
	// which is not the same thing and is not what a rewrite would keep.
	PlaceLabel string
}

// IsValid reports whether the chart carries the minimum to be a usable row: a
// label and a place with an IANA timezone. An untitled chart carries its file
// stem as its label, so it passes. It deliberately does NOT require a
// moment — the row does not hold one. LoadCharts drops files that don't satisfy
// this rather than surfacing half-charts, and names each one it drops
// (ChartDrop) rather than dropping it in silence.
func (c NamedChart) IsValid() bool {
	return c.Name != "" && c.Place.HasTimezone()
}

// IncompleteReason names what IsValid found missing, in the operator's own
// vocabulary, or "" when the chart is complete. It exists so every caller that
// drops an incomplete chart can say the same thing about it — including
// lookup, which builds rows of its own from chart files.
func (c NamedChart) IncompleteReason() string {
	var missing []string
	if c.Name == "" {
		missing = append(missing, "name")
	}
	if !c.Place.HasTimezone() {
		missing = append(missing, "timezone")
	}
	switch len(missing) {
	case 0:
		return ""
	case 1:
		return "no " + missing[0]
	default:
		return "no " + strings.Join(missing[:len(missing)-1], ", ") + " and " + missing[len(missing)-1]
	}
}

// ChartDrop is one file in the library directory that did not become a row: the
// file it came from, and why. The picker names these instead of leaving the
// list quietly short — for a tool whose premise is local client data, showing
// an astrologer no clients when their files failed to parse is
// indistinguishable from having lost them.
type ChartDrop struct {
	File   string // base filename as it sits in the library directory
	Reason string // why it did not become a chart, in the operator's vocabulary
}

// ChartsDir resolves the directory holding the chart library, by precedence:
//
//  1. $SEXTILE_CHARTS_DIR     — live operator data wins (real charts)
//  2. existing XDG chart dir   — host production default, including symlinks
//  3. $SEXTILE_DEV_CHARTS_DIR — dev/test fixtures (the shared .env default)
//  4. XDG chart dir            — default save target when nothing exists yet
//
// Live wins over dev in two ways. An explicit SEXTILE_CHARTS_DIR always wins,
// and a real host library at the XDG path wins over the repo .env's
// SEXTILE_DEV_CHARTS_DIR. This keeps dev fixtures useful in the container when
// no host library exists, while a host symlink at ~/.local/share/sextile/charts
// is treated as production data rather than shadowed by ./test_charts.
// Relative dev values resolve against the repo root via env.ResolveDir, so the
// binary and `go test` (run from a package directory) agree on the target
// regardless of CWD.
func ChartsDir() string {
	if d := os.Getenv("SEXTILE_CHARTS_DIR"); d != "" {
		return env.ResolveDir(d)
	}
	defaultDir := defaultChartsDir()
	if dirExists(defaultDir) {
		return defaultDir
	}
	// Tier 3 is a development fixture: a release build never lets it steer.
	if d := os.Getenv("SEXTILE_DEV_CHARTS_DIR"); d != "" && devTier() {
		return env.ResolveDir(d)
	}
	return defaultDir
}

// devTier decides whether tier 3 may steer. Production never reassigns it; a
// test swaps it (withDevTier) so both answers are exercised in one untagged
// build. The real build's answer is pinned on built probes in internal/env.
var devTier = env.Dev

func defaultChartsDir() string {
	base := os.Getenv("XDG_DATA_HOME")
	if base == "" {
		base = filepath.Join(os.Getenv("HOME"), ".local", "share")
	}
	return filepath.Join(base, "sextile", "charts")
}

func dirExists(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.IsDir()
}

// FilterCharts applies the same four-tier match as location.MatchTier — exact
// (tier 4), starts-with (tier 3), substring (tier 2), then subsequence-fuzzy
// (tier 1) against the
// chart label — returning matches in tier-then-original order plus whether the
// result was truncated to cap (cap <= 0 disables truncation). It deliberately
// mirrors location.Filter's shape and ranking so the chart picker can reuse
// the location-picker UI pattern unchanged. lowerASCII/fuzzyMatch are duplicated
// from location (different package, unexported there) on purpose — the parity is
// the contract, asserted by FilterCharts' tests.
func FilterCharts(charts []NamedChart, query string, cap int) ([]NamedChart, bool) {
	if query == "" {
		return nil, false
	}
	query = lowerASCII(query)

	type match struct {
		chart NamedChart
		index int
		tier  int
	}
	matches := make([]match, 0)
	for i := range charts {
		key := lowerASCII(charts[i].Name)
		tier := 0
		switch {
		case query == key:
			tier = 4
		case strings.HasPrefix(key, query):
			tier = 3
		case strings.Contains(key, query):
			tier = 2
		case fuzzyMatch(key, query):
			tier = 1
		}
		if tier == 0 {
			continue
		}
		matches = append(matches, match{chart: charts[i], index: i, tier: tier})
	}

	sort.SliceStable(matches, func(i, j int) bool {
		if matches[i].tier != matches[j].tier {
			return matches[i].tier > matches[j].tier
		}
		return matches[i].index < matches[j].index
	})

	truncated := false
	if cap > 0 && len(matches) > cap {
		matches = matches[:cap]
		truncated = true
	}

	out := make([]NamedChart, len(matches))
	for i := range matches {
		out[i] = matches[i].chart
	}
	return out, truncated
}

// Slugify exposes the chart-filename slug rule so callers that translate
// external charts into the library's identity space (see
// lookup.LoadChartLibrary) derive the SAME slug a saved chart's filename
// carries, keeping a translated chart and its saved counterpart deduplicated
// on one ID.
func Slugify(name string) string { return slugify(name) }

// slugify turns a chart label into a filesystem-safe slug: lowercased, ASCII
// alphanumerics kept, every run of other characters collapsed to a single '-',
// edges trimmed. "Constructed One" → "constructed-one"; "A.B.C. Example" →
// "a-b-c-example". Non-ASCII letters are dropped (best-effort ASCII slug).
func slugify(name string) string {
	var b strings.Builder
	prevDash := false
	for _, r := range strings.ToLower(name) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			prevDash = false
		default:
			if !prevDash && b.Len() > 0 {
				b.WriteByte('-')
				prevDash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

// lowerASCII and fuzzyMatch mirror internal/location's unexported helpers so
// FilterCharts ranks identically to location.Filter (see FilterCharts).
func lowerASCII(s string) string {
	b := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		b[i] = c
	}
	return string(b)
}

func fuzzyMatch(haystack, needle string) bool {
	if needle == "" {
		return true
	}
	j := 0
	for i := 0; i < len(haystack) && j < len(needle); i++ {
		if haystack[i] == needle[j] {
			j++
		}
	}
	return j == len(needle)
}
