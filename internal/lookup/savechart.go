package lookup

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/cruiserxyz/sextile/internal/astrolog"
	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/store"
)

// SavedChart is what one successful save produced. Name is the name ASTROLOG
// recorded, read back from the file it wrote, not the text that was typed:
// astrolog rewrites a double quote to a single one inside -zi, so the two can
// differ and the operator should be shown the one that is on disk.
type SavedChart struct {
	ID          string // the slug, and the id the library will resolve
	Name        string // the label astrolog recorded
	File        string // base name within the charts directory
	Path        string // the full path it was written to
	AtlasPlaced bool   // whether the atlas can place it, and so whether it will list
	PlaceCut    string // non-empty: the place label was cut to this to fit Astrolog's bound
}

// MaxLabelBytes is the longest name or place Astrolog can read back.
const MaxLabelBytes = astrolog.MaxLabelBytes

// CheckLabelLength is nil for a typed name, title or place Astrolog can read
// back, and a *astrolog.LabelTooLongError for one over the bound. The UI asks
// it before it runs a save or a rename (the specification).
func CheckLabelLength(what, s string) error { return astrolog.CheckLabelLength(what, s) }

// ErrEmptySlug means the typed name has no character a slug can be built
// from. store.Slugify keeps ASCII alphanumerics and drops everything else, so
// a name of punctuation alone yields nothing to save under.
var ErrEmptySlug = errors.New("lookup: that name has no letters or digits to save under")

// ErrSlugTaken means a chart in the library already owns the slug. It carries
// the file it found, because "that name is taken" without saying by what
// leaves the operator to go and look.
type ErrSlugTaken struct {
	Slug string
	File string
}

func (e *ErrSlugTaken) Error() string {
	return fmt.Sprintf("a chart is already saved as %q (%s)", e.Slug, e.File)
}

// beforePlace runs between the collision scan and the exclusive create; a test
// lands another window's save of the same name there.
var beforePlace = func() {}

// SaveFloatingChart writes the given moment and place into the chart library
// as an astrolog .as file under a slug derived from name, and returns what it
// saved.
//
// It is the entry point the perimeter calls: internal/ui may not import
// internal/astrolog (TestPerimeterDoesNotReachActive) and internal/store may
// not either, so this package -- the one that already holds both -- is where
// the three halves meet.
//
// It REFUSES rather than replacing. A slug the library already owns is an
// error and nothing is written; that limit is what the 2026-09-20 reversal of
// an earlier change was granted under, and it is enforced twice on purpose: here, where
// the refusal can name the file it collided with, and again in
// store.PlaceChartFile as an exclusive create, which is the half that cannot
// be raced.
func SaveFloatingChart(ctx context.Context, dir, name string, when time.Time, place location.PlaceInfo) (SavedChart, error) {
	// The charts directory is resolved ONCE, by the caller, and the same value
	// reaches the collision scan and the write. See store.PlaceChartFile for
	// why that matters; the short version is that store.ChartsDir() can answer
	// differently two calls apart, and the operator was shown one answer.
	if dir == "" {
		dir = store.ChartsDir()
	}
	slug := store.Slugify(name)
	if slug == "" {
		return SavedChart{}, ErrEmptySlug
	}
	if file, err := chartOwning(dir, slug); err != nil {
		return SavedChart{}, err
	} else if file != "" {
		return SavedChart{}, &ErrSlugTaken{Slug: slug, File: file}
	}

	// The place is the slot's, never typed in the prompt, so an over-long one
	// is cut where reports cut theirs and the caller says so; a name is the
	// operator's and is refused by ChartRecord instead.
	placeLabel, placeCut := savedPlaceLabel(place)
	record, err := astrolog.ChartRecord(ctx, astrolog.ChartToSave{
		Name:  name,
		Place: placeLabel,
		When:  when,
		Lon:   place.Lon,
		Lat:   place.Lat,
	})
	if err != nil {
		return SavedChart{}, err
	}
	beforePlace()
	path, err := store.PlaceChartFile(dir, record, slug)
	if err != nil {
		return SavedChart{}, err
	}

	saved := SavedChart{ID: slug, Name: name, File: filepath.Base(path), Path: path, PlaceCut: placeCut}
	// The label astrolog actually recorded, taken from the file rather than
	// from the argv it was handed.
	if h, err := astrolog.ReadChartFileHeader(path); err == nil && h.Name != "" {
		saved.Name = h.Name
	}
	saved.AtlasPlaced = atlasCanPlace(place.Lon, place.Lat)
	return saved, nil
}

// chartOwning names the chart already holding slug, or "" if it is free.
//
// It reads the DIRECTORY rather than the loaded library, and the difference is
// the point: the library drops a chart whose coordinates no atlas city is near,
// and such a file still sits on disk owning its name. Asking the library would
// report a slug as free and the save would then meet a file that is already
// there -- which the exclusive create would refuse, correctly, but with a
// message about a filename instead of about a chart.
//
// A chart owns the slug of its file stem, whatever its name ends in (C3.20; a
// chart is known by its first bytes), and whether or not astrolog can read it.
// A file that opens with no chart header owns nothing; one that cannot be read
// at all might be a chart, so it owns its slug too. A .yaml no longer owns
// anything (YAML charts retired 2026-10-02), and a chart claiming the slug
// through a `;sextile-id:` line went with that line.
func chartOwning(dir, slug string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil // nothing is saved yet, so nothing is taken
		}
		return "", fmt.Errorf("lookup: reading the chart folder: %w", err)
	}
	for _, e := range entries {
		name := e.Name()
		// The slug of the stem, by name alone: "Test Person.as" owns
		// "test-person" as surely as "test-person.as" does.
		if chartFileID(name) != slug {
			continue
		}
		kind, err := entryKind(dir, e)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err == nil && kind != astrolog.FileChart {
			continue
		}
		return name, nil
	}
	return "", nil
}

// atlasCanPlace reports whether the library will be able to turn these
// coordinates back into a city. A chart file carries no timezone, so the library
// reconstructs one from the nearest atlas city and DROPS a row it cannot
// place; a chart saved for a spot more than location.NearestMaxDeg from any
// city is written correctly and then never listed. The operator is told at
// save time rather than left to notice the absence.
func atlasCanPlace(lon, lat float64) bool {
	atlas, err := loadAtlas()
	if err != nil {
		return false
	}
	_, ok := location.Nearest(atlas, lon, lat)
	return ok
}

// savedPlaceLabel is the -zi place label a save writes for place: its Display,
// cut where reports cut theirs. cut is the label as astrolog records it
// (RecordLabel, since astrolog rewrites a double quote inside -zi) when the
// cut changed it, else "".
func savedPlaceLabel(place location.PlaceInfo) (label, cut string) {
	label = place.Display()
	if c := astrolog.CutLabel(label); c != label {
		return c, astrolog.RecordLabel(c)
	}
	return label, ""
}
