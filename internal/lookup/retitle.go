package lookup

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/cruiserxyz/sextile/internal/astrolog"
	"github.com/cruiserxyz/sextile/internal/store"
)

// retitle.go gives an untitled chart a title (the specification item 3).
//
// Sextile authors no byte of a chart file. Astrolog writes the record whole
// into a temp file store created; this file reads that record back through
// Astrolog and checks it says what it should; only then does store swap it in,
// keeping the first original under .sextile/backup. Every failure leaves the library as it
// was.

// Retitled is what one successful retitle produced.
type Retitled struct {
	ID         string // the chart's library id; unchanged by a retitle
	Name       string // the title Astrolog recorded, read back from the file
	PlaceLabel string // the place label Astrolog recorded
	File       string // the chart's file name after the write
	Backup     string // where the original was kept (relative to the charts folder), or "" if one already existed

	// Row is the chart as the library now lists it, read back after the
	// write: what a caller anchors, so the anchor and the session carry what
	// the file holds. Zero when the library cannot list it, e.g. without an
	// atlas.
	Row store.NamedChart
}

// ErrBadReadBack means the record Astrolog wrote does not read back to the
// chart it was made from (C3.8). The sentence names which fields.
var ErrBadReadBack = errors.New("Astrolog's record read back different")

// Test seams. Production never reassigns them.
var (
	// beforeVerify runs after Astrolog has written the temp record and before
	// it is read back: the tests of C3.8 corrupt it here.
	beforeVerify = func(tmp string) {}
)

// RetitleChart writes title and place into the chart c names, in dir. It
// refuses any label with a control character (C3.15), and an
// empty title or place.
//
// seen is the fingerprint the listing recorded for c's file (Library.Seen).
// If the file no longer matches it -- or the listing could not record one,
// because the file changed while it was being read -- nothing is written and
// the error is store.ErrChartChanged.
func RetitleChart(ctx context.Context, dir string, c store.NamedChart, seen, title, place string) (Retitled, error) {
	if dir == "" {
		dir = store.ChartsDir()
	}
	// Refused before anything is created. The form strips control
	// characters already (C2.8); this is the write path's own refusal, for
	// a caller that is not the form.
	for _, l := range []struct{ what, s string }{{"title", title}, {"place", place}} {
		if i := strings.IndexFunc(l.s, unicode.IsControl); i >= 0 {
			return Retitled{}, fmt.Errorf("%w: the %s contains a control character at position %d",
				astrolog.ErrLabelHasControlChar, l.what, i)
		}
	}
	if strings.TrimSpace(title) == "" || strings.TrimSpace(place) == "" {
		return Retitled{}, errors.New("lookup: a title and a place are both needed")
	}
	if fp, err := fingerprint(filepath.Join(dir, filepath.Base(c.Source.File))); seen == "" || err != nil || fp != seen {
		return Retitled{}, fmt.Errorf("%w: %s", store.ErrChartChanged, filepath.Base(c.Source.File))
	}
	if c.Source.Kind != store.ChartKindFile {
		return Retitled{}, fmt.Errorf("lookup: RetitleChart: %q has no readable source", c.Name)
	}
	r, err := retitleChartFile(ctx, dir, c, title, place)
	if err != nil {
		return Retitled{}, err
	}
	// The row as the library lists it now. Listing spawns nothing.
	if lib, lerr := LoadChartLibraryIn(dir); lerr == nil {
		for _, row := range lib.Charts {
			if row.Source.File == r.File {
				r.Row = row
			}
		}
	}
	return r, nil
}

func retitleChartFile(ctx context.Context, dir string, c store.NamedChart, title, place string) (Retitled, error) {
	file := filepath.Base(c.Source.File)
	path := filepath.Join(dir, file)
	original, err := os.ReadFile(path)
	if err != nil {
		return Retitled{}, fmt.Errorf("lookup: reading %s: %w", file, err)
	}
	// Re-read rather than trusting the listing: the file may have changed
	// since the picker was opened. Whatever else it holds, Astrolog writes
	// back only the record; the first backup keeps every original byte, and
	// the read-back below refuses a rewrite that moved the chart (C3.8).
	if _, err := astrolog.ReadChartFileHeader(path); err != nil {
		return Retitled{}, err
	}
	before, err := astrolog.ResolveChartFile(ctx, path)
	if err != nil {
		return Retitled{}, err
	}

	tmp, err := store.CreateChartTemp(dir, stem(file))
	if err != nil {
		return Retitled{}, err
	}
	keep := false
	defer func() {
		if !keep {
			store.RemoveChartTemp(tmp)
		}
	}()
	if err := astrolog.RetitleTo(ctx, path, title, place, tmp); err != nil {
		return Retitled{}, err
	}
	beforeVerify(tmp)
	after, err := astrolog.ResolveChartFile(ctx, tmp)
	if err != nil {
		return Retitled{}, fmt.Errorf("lookup: Astrolog cannot read back the record it wrote: %w", err)
	}
	if err := sameRecord(after, after.JD == before.JD, before.Lon, before.Lat, title, place); err != nil {
		return Retitled{}, err
	}

	backedUp, err := store.ReplaceChartWithBackup(dir, file, tmp, original)
	if err != nil {
		return Retitled{}, err
	}
	keep = true // renamed into place; there is no temp file left to remove
	r := Retitled{ID: c.ID, Name: after.Name, PlaceLabel: after.PlaceLabel, File: file}
	if backedUp {
		r.Backup = store.ChartBackupRel(file)
	}
	return r, nil
}

// sameRecord is C3.8's check on a read-back: the moment (judged by the
// caller, who knows the precision it can hold the record to), the
// coordinates, and both labels after Astrolog's documented quote rewrite.
// Any difference refuses the write.
func sameRecord(got astrolog.ChartFile, momentOK bool, lon, lat float64, title, place string) error {
	var bad []string
	if !momentOK {
		bad = append(bad, "moment")
	}
	if got.Lon != lon || got.Lat != lat {
		bad = append(bad, "coordinates")
	}
	if got.Name != astrolog.RecordLabel(title) {
		bad = append(bad, "title")
	}
	if got.PlaceLabel != astrolog.RecordLabel(place) {
		bad = append(bad, "place")
	}
	if len(bad) > 0 {
		return fmt.Errorf("%w (%s); nothing was written", ErrBadReadBack, strings.Join(bad, ", "))
	}
	return nil
}
