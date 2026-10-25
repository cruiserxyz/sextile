package lookup

// the specification C0: a slot's name and place for Astrolog.

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cruiserxyz/sextile/internal/astrolog"
	"github.com/cruiserxyz/sextile/internal/store"
)

// A library chart's report label is its file's own title and the place text
// the chart list shows, from the one read that resolves the moment. An
// untitled file shows its stem in the list and sends Astrolog nothing.
func TestSlotNameAndPlaceForAstrolog(t *testing.T) {
	dir := useLibraryDir(t)
	writeChartFile(t, dir, "titled.as", munichChart)
	writeChartFile(t, dir, "untitled.as", "@AI780  ; test\n-qb Sep 23 1995 6:00pm DT 1E 11:34E 48:08N\n")
	writeChartFile(t, dir, "empty-zi.as", "@AI780  ; test\n-qb Sep 23 1995 6:00pm DT 1E 11:34E 48:08N\n-zi \"\" \"Somewhere\"\n")
	writeChartFile(t, dir, "twice.as", "@AI780  ; test\n-qb Sep 23 1995 6:00pm DT 1E 11:34E 48:08N\n-zi \"First\" \"x\"\n-zi \"Last\" \"y\"\n")

	lib, err := LoadChartLibrary()
	if err != nil {
		t.Fatal(err)
	}
	rows := map[string]store.NamedChart{}
	for _, c := range lib.Charts {
		rows[c.Source.File] = c
	}
	for _, tc := range []struct {
		file         string
		listedName   string
		untitled     bool
		wantLabel    store.ReportLabel
		wantListedAs string
	}{
		{"titled.as", "Test Person", false, store.ReportLabel{Name: "Test Person", Place: "Munich, DE"}, "Munich, DE"},
		{"untitled.as", "untitled", true, store.ReportLabel{}, "Munich, DE"},
		{"empty-zi.as", "empty-zi", true, store.ReportLabel{}, "Munich, DE"},
		{"twice.as", "Last", false, store.ReportLabel{Name: "Last", Place: "Munich, DE"}, "Munich, DE"},
	} {
		row, ok := rows[tc.file]
		if !ok {
			t.Fatalf("%s is not in the library: %+v", tc.file, lib.Drops)
		}
		if row.Name != tc.listedName || row.Untitled != tc.untitled {
			t.Fatalf("%s: the list shows %q (untitled %v), want %q (%v)", tc.file, row.Name, row.Untitled, tc.listedName, tc.untitled)
		}
		if got := row.Place.Display(); got != tc.wantListedAs {
			t.Fatalf("%s: the list shows the place %q, want %q", tc.file, got, tc.wantListedAs)
		}
		picked, err := ResolvePickedChart(context.Background(), row)
		if err != nil {
			t.Fatalf("%s: %v", tc.file, err)
		}
		if picked.Label != tc.wantLabel {
			t.Errorf("%s: label %+v, want %+v", tc.file, picked.Label, tc.wantLabel)
		}
		if !picked.HasPlace || picked.Place.Display() != tc.wantListedAs {
			t.Errorf("%s: resolved place %+v (has %v), want the list's %q", tc.file, picked.Place, picked.HasPlace, tc.wantListedAs)
		}
	}
}

// Name, place and moment are one read: a file replaced between the listing and
// the pick gives the replacement's name WITH the replacement's moment.
func TestResolveReadsNameMomentAndPlaceTogether(t *testing.T) {
	dir := useLibraryDir(t)
	writeChartFile(t, dir, "client.as", munichChart)
	lib, err := LoadChartLibrary()
	if err != nil || len(lib.Charts) != 1 {
		t.Fatalf("library: %v %+v", err, lib)
	}
	row := lib.Charts[0]
	// The file is replaced by another client's, born elsewhere.
	writeChartFile(t, dir, "client.as", "@AI780  ; test\n-qb Jan 1 2001 9:00am ST 0W 13:24E 52:31N\n-zi \"Other Client\" \"Berlin\"\n")
	picked, err := ResolvePickedChart(context.Background(), row)
	if err != nil {
		t.Fatal(err)
	}
	if picked.Label.Name != "Other Client" || strings.Contains(picked.Label.Place, "Munich") || picked.Label.Place != picked.Place.Display() {
		t.Errorf("label %+v is not the replacement's", picked.Label)
	}
	if picked.When.Year() != 2001 {
		t.Errorf("moment %v is not the replacement's", picked.When)
	}
	if !picked.HasPlace || picked.Place.Name == "Munich" || picked.Place.TZ != "Europe/Berlin" {
		t.Errorf("resolved place %+v is not the replacement's", picked.Place)
	}
}

// C9, reversed by the specification C5 (V7): a title and a place of exactly
// 54 bytes survive save and retitle whole, read back by the library's reader
// AND by the live binary; 55 is refused (a title), or cut for the slot's place
// on save. The earlier 100- and 90-byte version saved files Astrolog cannot
// read back (bug report of 2026-10-09).
func TestSaveAndRetitleLongLabelsRoundTrip(t *testing.T) {
	needLiveAstrolog(t)
	atBound := strings.Repeat("T", 54)
	over := strings.Repeat("T", 55)
	// amsterdamPlace shows as "<Name>, NL": size Name so Display is 54 bytes.
	placeAt := amsterdamPlace()
	placeAt.Name = strings.Repeat("P", 54-len(", NL"))
	if len(placeAt.Display()) != 54 {
		t.Fatalf("test setup: Display is %d bytes", len(placeAt.Display()))
	}
	t.Run("save", func(t *testing.T) {
		dir := useLibraryDir(t)
		saved, err := SaveFloatingChart(context.Background(), dir, atBound, amsterdamMoment(), placeAt)
		if err != nil {
			t.Fatal(err)
		}
		if saved.PlaceCut != "" {
			t.Errorf("a 54-byte place was cut to %q", saved.PlaceCut)
		}
		h, err := astrolog.ReadChartFileHeader(saved.Path)
		if err != nil {
			t.Fatal(err)
		}
		if h.Name != atBound || h.PlaceLabel != placeAt.Display() {
			t.Errorf("saved and read back as %d/%d bytes, want 54/54", len(h.Name), len(h.PlaceLabel))
		}
		if _, err := astrolog.ResolveChartFile(context.Background(), saved.Path); err != nil {
			t.Errorf("Astrolog cannot read back what was saved at the bound: %v", err)
		}
	})
	t.Run("save refuses a 55-byte name and cuts the slot's place", func(t *testing.T) {
		dir := useLibraryDir(t)
		_, err := SaveFloatingChart(context.Background(), dir, over, amsterdamMoment(), placeAt)
		var long *astrolog.LabelTooLongError
		if !errors.As(err, &long) || long.Bytes != 55 {
			t.Fatalf("a 55-byte name got %v, want a 55-byte LabelTooLongError", err)
		}
		if es, _ := os.ReadDir(dir); len(es) != 0 {
			t.Errorf("a refused save wrote %d files", len(es))
		}
		longPlace := amsterdamPlace()
		longPlace.Name = strings.Repeat("P", 90)
		saved, err := SaveFloatingChart(context.Background(), dir, "Cut", amsterdamMoment(), longPlace)
		if err != nil {
			t.Fatal(err)
		}
		if want := astrolog.CutLabel(longPlace.Display()); saved.PlaceCut != want || len(want) > 54 || !strings.HasSuffix(want, "…") {
			t.Errorf("PlaceCut = %q, want %q", saved.PlaceCut, want)
		}
		if h, err := astrolog.ReadChartFileHeader(saved.Path); err != nil || h.PlaceLabel != saved.PlaceCut {
			t.Errorf("the file carries %q (%v), want the cut place", h.PlaceLabel, err)
		}
		// One byte over the bound is cut too: 55 bytes become at most 54.
		justOver := amsterdamPlace()
		justOver.Name = strings.Repeat("P", 55-len(", NL"))
		if len(justOver.Display()) != 55 {
			t.Fatalf("test setup: Display is %d bytes", len(justOver.Display()))
		}
		saved, err = SaveFloatingChart(context.Background(), dir, "Just over", amsterdamMoment(), justOver)
		if err != nil {
			t.Fatal(err)
		}
		if len(saved.PlaceCut) == 0 || len(saved.PlaceCut) > 54 || !strings.HasSuffix(saved.PlaceCut, "…") {
			t.Errorf("a 55-byte place was cut to %q, want at most 54 bytes ending in an ellipsis", saved.PlaceCut)
		}
	})
	t.Run("retitle", func(t *testing.T) {
		dir := useLibraryDir(t)
		writeChartFile(t, dir, "munich.as", munichChart)
		dest := filepath.Join(dir, "out.as")
		if err := os.WriteFile(dest, []byte("placeholder"), 0o600); err != nil {
			t.Fatal(err)
		}
		src := filepath.Join(dir, "munich.as")
		if err := astrolog.RetitleTo(context.Background(), src, atBound, atBound, dest); err != nil {
			t.Fatal(err)
		}
		h, err := astrolog.ReadChartFileHeader(dest)
		if err != nil {
			t.Fatal(err)
		}
		if h.Name != atBound || h.PlaceLabel != atBound {
			t.Errorf("retitled and read back as %d/%d bytes, want 54/54", len(h.Name), len(h.PlaceLabel))
		}
		if _, err := astrolog.ResolveChartFile(context.Background(), dest); err != nil {
			t.Errorf("Astrolog cannot read back what was retitled at the bound: %v", err)
		}
		var long *astrolog.LabelTooLongError
		if err := astrolog.RetitleTo(context.Background(), src, over, atBound, dest); !errors.As(err, &long) {
			t.Errorf("a 55-byte title got %v, want a LabelTooLongError", err)
		}
		if err := astrolog.RetitleTo(context.Background(), src, atBound, over, dest); !errors.As(err, &long) || long.Bytes != 55 || long.What != "place" {
			t.Errorf("a 55-byte place got %v, want a LabelTooLongError of 55 bytes", err)
		}
	})
}

func needLiveAstrolog(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("astrolog"); err != nil {
		t.Skip("astrolog not on PATH; skipping live-binary test")
	}
}
