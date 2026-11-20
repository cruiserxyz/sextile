package lookup

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/cruiserxyz/sextile/internal/astrolog"
	"github.com/cruiserxyz/sextile/internal/store"
)

// retitle.go gives an untitled chart a title (the specification item 3).
//
// Astrolog writes the record with the new labels into a private temp folder.
// Reapply puts back every line of the source except the -zi line Astrolog
// wrote, the result is read back through Astrolog and checked to cast as the
// same chart with the new labels, and only then does the store door replace
// the file, keeping a dated copy of it outside the charts folder. Every
// failure leaves the library as it was.

// Retitled is what one successful retitle produced.
type Retitled struct {
	ID         string // the chart's library id; unchanged by a retitle
	Name       string // the title Astrolog recorded, read back from the file
	PlaceLabel string // the place label Astrolog recorded
	File       string // the chart's file name after the write
	Record     string // the file's text after the write, as Astrolog read it back
	// Clock is the chart's moment as Astrolog read the written file back, for
	// its row: the moment is unchanged, the place may be new.
	Clock  store.ChartClock
	Backup string // the dated copy of the file as it was (absolute; the host's path in Docker)

	// Row is the chart as the library now lists it, read back after the
	// write: what a caller anchors, so the anchor and the session carry what
	// the file holds. Zero when the library cannot list it, e.g. without an
	// atlas.
	Row store.NamedChart
}

// ErrBadReadBack means the record Astrolog wrote does not read back to the
// chart it was made from (C3.8). The sentence names which fields.
var ErrBadReadBack = errors.New("the changed file would cast another chart, so nothing was written")

// Test seams. Production never reassigns them.
var (
	// afterAstrologWrite sees Astrolog's record before the re-apply step.
	afterAstrologWrite = func(b []byte) []byte { return b }
	// beforeVerify sees the re-applied record before it is read back: the
	// tests of C3.8 corrupt it here.
	beforeVerify = func(b []byte) []byte { return b }
)

// RetitleChart writes title and place into the chart c names, in dir. It
// refuses any label with a control character (C3.15), and an
// empty title or place. stamp names the dated copy (store.BackupName).
//
// seen is the fingerprint the listing recorded for c's file (Library.Seen).
// If the file no longer matches it -- or the listing could not record one,
// because the file changed while it was being read -- nothing is written and
// the error is store.ErrChartChanged.
func RetitleChart(ctx context.Context, dir string, c store.NamedChart, seen, title, place string, stamp time.Time) (Retitled, error) {
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
	// The file is read once: the fingerprint, the cast before, the re-apply
	// and the door's own check all use these bytes.
	original, err := os.ReadFile(filepath.Join(dir, filepath.Base(c.Source.File)))
	if seen == "" || err != nil || fingerprintOf(original) != seen {
		return Retitled{}, fmt.Errorf("%w: %s", store.ErrChartChanged, filepath.Base(c.Source.File))
	}
	if c.Source.Kind != store.ChartKindFile {
		return Retitled{}, fmt.Errorf("lookup: RetitleChart: %q has no readable source", c.Name)
	}
	r, err := retitleChartFile(ctx, dir, c, original, title, place, stamp)
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

func retitleChartFile(ctx context.Context, dir string, c store.NamedChart, original []byte, title, place string, stamp time.Time) (Retitled, error) {
	file := filepath.Base(c.Source.File)
	path := filepath.Join(dir, file)
	if astrolog.KindOfPrefix(original) != astrolog.FileChart {
		return Retitled{}, fmt.Errorf("lookup: %s is not a chart record", file)
	}
	before, err := astrolog.ResolveChartBytes(ctx, original, path)
	if err != nil {
		return Retitled{}, err
	}
	written, err := astrolog.RetitleRecord(ctx, path, title, place)
	if err != nil {
		return Retitled{}, err
	}
	written = afterAstrologWrite(written)
	record, err := Reapply(file, original, written, OwnsName, false)
	if err != nil {
		return Retitled{}, err
	}
	record = beforeVerify(record)
	after, err := astrolog.ResolveChartBytes(ctx, record, path)
	if err != nil {
		return Retitled{}, fmt.Errorf("lookup: Astrolog cannot read back the record it wrote: %w", err)
	}
	if err := sameRecord(after, before, title, place); err != nil {
		return Retitled{}, err
	}
	backup, err := store.OriginalsDoor(dir).Replace(file, original, record, stamp)
	if err != nil {
		return Retitled{}, err
	}
	return Retitled{ID: c.ID, Name: after.Name, PlaceLabel: after.PlaceLabel, File: file, Record: after.Record, Clock: ClockOfRead(after), Backup: backup}, nil
}

// sameRecord is the check on a read-back of the re-applied file: the moment,
// the coordinates and the house system as cast must equal the source's
// exactly, as must the form of the Nodes and Lilith the file states, and both
// labels must be the new ones after Astrolog's documented
// quote rewrite. Any difference refuses the write.
func sameRecord(got, before astrolog.ChartFile, title, place string) error {
	var bad []string
	if got.JD != before.JD || got.Clock != before.Clock {
		bad = append(bad, "moment")
	}
	if got.Lon != before.Lon || got.Lat != before.Lat {
		bad = append(bad, "coordinates")
	}
	if got.HasHouses != before.HasHouses || got.Houses != before.Houses {
		bad = append(bad, "houses")
	}
	if got.NodesStated != before.NodesStated || got.Nodes != before.Nodes {
		bad = append(bad, "form of the Nodes and Lilith")
	}
	if got.Name != astrolog.RecordLabel(title) {
		bad = append(bad, "title")
	}
	if got.PlaceLabel != astrolog.RecordLabel(place) {
		bad = append(bad, "place")
	}
	if len(bad) > 0 {
		return fmt.Errorf("%w (%s). Nothing was written", ErrBadReadBack, strings.Join(bad, ", "))
	}
	return nil
}
