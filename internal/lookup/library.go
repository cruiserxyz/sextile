package lookup

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/cruiserxyz/sextile/internal/astrolog"
	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/recipe"
	"github.com/cruiserxyz/sextile/internal/store"
)

// Library is one chart-library read: the charts the picker can show, and the
// files it could not read (named, with reasons).
type Library struct {
	Charts []store.NamedChart
	Drops  []store.ChartDrop
	// Refused maps a listed chart's file name to the reason the chart guard
	// refuses it (a *ChartRefused): the row lists, and does not open.
	Refused map[string]error
	// Seen maps each listed chart's file name to a fingerprint of the bytes
	// the listing read. A retitle compares it at submit, so a chart that
	// changed after the picker listed it is refused rather than rewritten
	// (the specification). It is not a moment, and it is
	// kept off NamedChart, whose fields C2 holds closed.
	Seen map[string]string
}

// LoadChartLibrary returns the chart library the picker shows: the astrolog
// chart-info files in ChartsDir(): every file there that opens with Astrolog's
// `@AI` header, whatever it is called. It SPAWNS NOTHING. Every fact a listing row displays — a
// name, a place and a date — is read from the file's own text, so a library of
// forty clients costs forty file reads and no subprocesses, and a first launch
// is as fast as the hundredth.
//
// What a row does NOT carry is the birth moment. That is obtained by
// ResolvePicked when a chart is actually picked, from the file the row names:
// whether a file is a chart Astrolog can cast is Astrolog's to say, at the
// pick, so the list never drops a file for what Sextile cannot read in its
// text. A file that cannot be read at all is named in Drops rather than
// dropped in silence. Two chart files claiming ONE identity is a
// conflict between two charts, where the loser is named like any other file
// that did not become a row. A visible `.yaml` chart is named too: YAML charts
// were retired on 2026-10-02 (the specification), and a client file that is
// present must never leave the list quietly short. A hidden file, `.yaml` or
// not, is never a chart and is skipped silently. Charts are returned sorted
// by ID; drops by filename.
//
// The chart grammar lives here because the store may not read it (it cannot
// import astrolog).
func LoadChartLibrary() (Library, error) {
	return LoadChartLibraryIn(store.ChartsDir())
}

// LoadChartLibraryIn is LoadChartLibrary over the directory dir names; an
// empty dir means store.ChartsDir(). A caller that has already captured a
// directory lists that one and never asks the resolver again.
func LoadChartLibraryIn(dir string) (Library, error) {
	if dir == "" {
		dir = store.ChartsDir()
	}
	// Fingerprints are taken before the rows are read and again after, and a
	// file is given one only when both agree: then the row was read from
	// those bytes and no others. A file saved in between gets none, and a
	// retitle of it is refused rather than made against the wrong version.
	before := fingerprintDir(dir)
	dat, drops, refused := translateChartFiles(dir)

	bySlug := make(map[string]store.NamedChart)
	for _, c := range dat {
		// Two chart files claiming one identity is a conflict, not a
		// precedence: their filenames are unique, so they can only get
		// here by stems that slugify alike ("A.as" and "a.as"). The
		// first in filename order keeps the row and the other is NAMED —
		// a file that was present and did not become a row may never
		// leave the list quietly short.
		if prior, taken := bySlug[c.ID]; taken {
			// The same stem under two names ("X.as" and "X.legacy", both
			// charts by their bytes) is the plain case and gets the plain
			// words; stems that merely slugify alike name the shared id.
			reason := fmt.Sprintf("%s already uses the id %q", filepath.Base(prior.Source.File), c.ID)
			if stem(c.Source.File) == stem(prior.Source.File) {
				reason = fmt.Sprintf("another file in the folder has the same name (%s)", filepath.Base(prior.Source.File))
			}
			drops = append(drops, store.ChartDrop{
				File:   filepath.Base(c.Source.File),
				Reason: reason,
			})
			continue
		}
		bySlug[c.ID] = c
	}

	out := make([]store.NamedChart, 0, len(bySlug))
	for _, c := range bySlug {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	sort.Slice(drops, func(i, j int) bool { return drops[i].File < drops[j].File })
	betweenListingReads()
	after := fingerprintDir(dir)
	seen := make(map[string]string, len(out))
	for _, c := range out {
		if fp := before[c.Source.File]; fp != "" && fp == after[c.Source.File] {
			seen[c.Source.File] = fp
		}
	}
	return Library{Charts: out, Drops: drops, Refused: refused, Seen: seen}, nil
}

// betweenListingReads is a test seam between the rows and the second set of
// fingerprints. Production never reassigns it.
var betweenListingReads = func() {}

// fingerprintDir fingerprints every chart file in dir, by name. A chart is a
// file that opens with Astrolog's chart header (entryKind), whatever it is
// called.
func fingerprintDir(dir string) map[string]string {
	out := map[string]string{}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return out
	}
	for _, e := range entries {
		if kind, err := entryKind(dir, e); err != nil || kind != astrolog.FileChart {
			continue
		}
		if fp, err := fingerprint(filepath.Join(dir, e.Name())); err == nil {
			out[e.Name()] = fp
		}
	}
	return out
}

// entryKind is what one directory entry of the charts folder is, judged by its
// first bytes and by nothing in its name except one rule: a name that starts
// with a dot (`.sextile/`, a `.sextile-tmp` temp file, any dotfile) is never a
// chart, and neither is a folder or anything that is not a regular file once a
// link is followed (a pipe would block the read). A link to a chart is a chart.
//
// The error is the open or read failure of a file that could be one; a file
// that vanished between the listing and this read is os.ErrNotExist.
func entryKind(dir string, e os.DirEntry) (astrolog.FileKind, error) {
	if e.IsDir() || strings.HasPrefix(e.Name(), ".") {
		return astrolog.FileOther, nil
	}
	path := filepath.Join(dir, e.Name())
	fi, err := os.Stat(path)
	if err != nil {
		return astrolog.FileOther, err
	}
	if !fi.Mode().IsRegular() {
		return astrolog.FileOther, nil
	}
	return astrolog.SniffFile(path)
}

// ResolvePicked obtains a picked chart's AUTHORITATIVE birth moment by
// reopening the file its row came from. This is the only place a library chart
// gets a moment, and it runs once, for the one chart the operator chose.
//
// A chart file is resolved through astrolog, which performs the local->UT
// conversion via timezone.as — Sextile never re-derives it. It writes no byte
// into the library directory.
func ResolvePicked(ctx context.Context, c store.NamedChart) (time.Time, error) {
	p, err := ResolvePickedChart(ctx, c)
	return p.When, err
}

// PickedChart is what one read of a chart file says: its authoritative birth
// moment and the house system the file names (the specification C2).
// HasHouses is false when the file names none.
type PickedChart struct {
	When      time.Time
	Houses    recipe.HouseSystem
	HasHouses bool

	// Label is the name and place text this chart's reports print in
	// Astrolog's header (the specification C0), taken from the SAME read of
	// the file as When: the file's own title and place. Both are empty for an
	// untitled file, whatever stem the listing shows for it.
	Label store.ReportLabel
	// Place is where that read casts: the file's own place (filePlace), with
	// the coordinates Astrolog read.
	Place location.PlaceInfo
	// Record is the file's text from that same read: what Astrolog casts.
	Record string
	// Notes are the file's lines Astrolog does not get, for the Log.
	Notes []FileNote
	// Clock is the chart's moment as Astrolog read it, for its row.
	Clock store.ChartClock
	// Nodes is the form of the Nodes and Lilith the file states, when it
	// states one (NodesStated).
	Nodes       recipe.NodeForm
	NodesStated bool
}

// ResolvePickedChart is ResolvePicked with the file's own house system, from
// the same single Astrolog read, and the chart's report label and resolved
// place from that read too.
func ResolvePickedChart(ctx context.Context, c store.NamedChart) (PickedChart, error) {
	return ResolvePickedChartIn(ctx, "", c)
}

// ResolvePickedChartIn is ResolvePickedChart over the directory dir names; an
// empty dir means store.ChartsDir().
func ResolvePickedChartIn(ctx context.Context, dir string, c store.NamedChart) (PickedChart, error) {
	switch c.Source.Kind {
	case store.ChartKindFile:
		cf, err := readChartRecord(ctx, dir, c.Source.File)
		if err != nil {
			return PickedChart{}, err
		}
		return pickedFrom(cf), nil
	default:
		return PickedChart{}, fmt.Errorf("lookup: ResolvePicked: %q has no readable source", c.Name)
	}
}

// ReadChartFile reads the chart file named by file (a base name inside the
// chart directory) through Astrolog. It writes no byte into the directory.
func ReadChartFile(ctx context.Context, file string) (PickedChart, error) {
	return ReadChartFileIn(ctx, store.ChartsDir(), file)
}

// ReadChartFileIn is ReadChartFile over the directory dir names; an empty dir
// means store.ChartsDir().
func ReadChartFileIn(ctx context.Context, dir, file string) (PickedChart, error) {
	cf, err := readChartRecord(ctx, dir, file)
	if err != nil {
		return PickedChart{}, err
	}
	return pickedFrom(cf), nil
}

func readChartRecord(ctx context.Context, dir, file string) (astrolog.ChartFile, error) {
	if dir == "" {
		dir = store.ChartsDir()
	}
	path := filepath.Join(dir, filepath.Base(file))
	return astrolog.ResolveChartFile(ctx, path)
}

func pickedFrom(cf astrolog.ChartFile) PickedChart {
	p := PickedChart{When: cf.WhenUTC, Houses: cf.Houses, HasHouses: cf.HasHouses, Record: cf.Record, Notes: cf.Notes, Clock: ClockOfRead(cf), Nodes: cf.Nodes, NodesStated: cf.NodesStated,
		Place: filePlace(cf.PlaceLabel, cf.Lon, cf.Lat, true)}
	if cf.Name != "" {
		p.Label = store.ReportLabel{Name: cf.Name, Place: cf.PlaceLabel}
	}
	return p
}

// filePlace is where a chart file casts, in the file's own words: its -zi
// place, or its coordinates when it names none (hasCoords false leaves the
// name empty). It carries no time zone, because a chart file is cast from its
// own record and shows its own clock (the specification).
func filePlace(label string, lon, lat float64, hasCoords bool) location.PlaceInfo {
	name := label
	if name == "" && hasCoords {
		name = CoordinateWords(lon, lat)
	}
	return location.PlaceInfo{Name: name, Lon: lon, Lat: lat}
}

// translateChartFiles turns every chart file in ChartsDir() (known by its first
// bytes, see entryKind) into a listing row by reading the file's own text: its
// title, its place and the date it states. No process is started, and the
// moment the row does not show is never computed.
//
// Every failure — an unreadable directory or file, a filename that gives no
// id — returns as a named drop instead of vanishing. A file the chart guard
// refuses (astrolog.GuardChartFile, which spawns nothing either) lists, and
// its reason comes back in refused, by file name.
func translateChartFiles(dir string) ([]store.NamedChart, []store.ChartDrop, map[string]error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			// Nothing is there to fail: an empty library is the honest
			// report, and the picker says so in its own words.
			return nil, nil, nil
		}
		return nil, []store.ChartDrop{{File: dir, Reason: "this chart folder could not be read"}}, nil
	}
	refused := map[string]error{}

	var out []store.NamedChart
	var drops []store.ChartDrop
	drop := func(name, reason string) {
		drops = append(drops, store.ChartDrop{File: name, Reason: reason})
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") {
			continue // hidden files are never charts, and are never reported
		}
		if !e.IsDir() && strings.EqualFold(filepath.Ext(e.Name()), ".yaml") {
			drop(e.Name(), "Sextile no longer reads charts in the old .yaml form")
			continue
		}
		kind, err := entryKind(dir, e)
		if err != nil {
			// Vanished since the listing: nothing is there to drop. Anything
			// else is a file that could be a chart and cannot be read, and a
			// file that was present may not leave the list quietly short.
			if !errors.Is(err, os.ErrNotExist) {
				drop(e.Name(), "this file could not be read")
			}
			continue
		}
		if kind != astrolog.FileChart {
			continue // not a chart by its bytes: ignored, silently
		}
		path := filepath.Join(dir, e.Name())
		h, err := astrolog.ReadChartFileHeader(path)
		if err != nil {
			drop(e.Name(), "this file could not be read")
			continue
		}
		id := chartFileID(path)
		if id == "" {
			// store.Slugify keeps ASCII alphanumerics and drops the rest, so a
			// filename can yield nothing at all ("---.as"). An empty id is not
			// a harmless one: ingest reads an empty id in session.yaml as "no
			// id", so the row would be unreferenceable, and two such files
			// would collide with each other. Name it, like every other file
			// that could not become a row — guessing an identity from the
			// display label is exactly what an earlier change removed.
			drop(e.Name(), "its file name has no letters or digits")
			continue
		}
		nc := store.NamedChart{
			ID:         id,
			Name:       chartFileName(path, h),
			Year:       h.Year,
			Month:      h.Month,
			Day:        h.Day,
			Place:      filePlace(h.PlaceLabel, h.Lon, h.Lat, h.HasCoords),
			Source:     store.ChartSource{Kind: store.ChartKindFile, File: e.Name()},
			Untitled:   h.Name == "",
			PlaceLabel: h.PlaceLabel,
		}
		if reason := nc.IncompleteReason(); reason != "" {
			drop(e.Name(), reason)
			continue
		}
		if raw, err := os.ReadFile(path); err == nil {
			if _, _, err := astrolog.GuardChartFile(string(raw)); err != nil {
				refused[e.Name()] = err
			}
		}
		out = append(out, nc)
	}
	return out, drops, refused
}

// chartFileName is the chart's display label: the file's -zi name, or the filename
// stem when that is empty (fixtures like FullMoon-2026-02.as carry no label).
func chartFileName(path string, h astrolog.ChartFileHeader) string {
	if h.Name != "" {
		return h.Name
	}
	return stem(path)
}

// chartFileID is the chart's library identity: the slug of its FILENAME STEM, and
// nothing else (the specification C3.20). It goes through store.Slugify so
// a translated chart and its saved counterpart collide on the SAME ID.
//
// A `;sextile-id:` line an older Sextile appended is ignored. Astrolog erases
// it on every rewrite, so an id read from it changed under a file
// nobody had renamed; the stem is the half that stays put.
//
// The fallback is the stem and not the display label, for the reason a
// saved YAML chart's ID is its filename: the file is the locator, filenames are
// unique inside one directory, and the label is neither. Deriving identity from
// the label made two different clients who share a name ONE library row, and
// made renaming a client in Astrolog silently change which chart every stored
// reference to them (session.yaml's `chart:` block) points at.
// It returns "" when the stem yields nothing — Slugify is lossy, so a filename
// of punctuation alone has no identity to give. The caller names that file
// rather than listing it with an empty id.
func chartFileID(path string) string {
	return store.Slugify(stem(path))
}

// fingerprint is a SHA-256 of a file's bytes, hex-encoded.
func fingerprint(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return fingerprintOf(b), nil
}

// fingerprintOf is fingerprint of bytes already read.
func fingerprintOf(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func stem(path string) string {
	base := filepath.Base(path)
	return strings.TrimSuffix(base, filepath.Ext(base))
}
