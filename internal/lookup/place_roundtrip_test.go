// All chart values in this file are constructed synthetic test inputs.
package lookup

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cruiserxyz/sextile/internal/astrolog"
	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/recipe"
)

// computeZL is the "-zl <lon> <lat>" text the floating slot's single-chart
// compute puts on Astrolog's argv for a place: the compiler's own output, so
// the formatting is not restated here.
func computeZL(t *testing.T, lon, lat float64) string {
	t.Helper()
	plan, err := astrolog.Compile(recipe.ChartSpec{
		Source:  recipe.MomentAtPlace{JD: 2446875.0, Lon: lon, Lat: lat},
		Objects: recipe.ObjectPolicy{Baseline: recipe.AstrologDefault},
	}, recipe.Positions{})
	if err != nil {
		t.Fatalf("Compile at %v,%v: %v", lon, lat, err)
	}
	argv := plan.Invocations[0].Argv
	for i, a := range argv {
		if a == "-zl" && i+2 < len(argv) {
			return strings.Join(argv[i:i+3], " ")
		}
	}
	t.Fatalf("no -zl in the compiled argv %q", argv)
	return ""
}

// stagedZL is the same text from the staged-file writer, which does not spawn
// or compile anything and so can be asked for every place in the atlas. The
// sampled test below proves it equal to computeZL.
func stagedZL(t *testing.T, lon, lat float64) string {
	t.Helper()
	text, err := astrolog.FormatChartFile(recipe.MomentAtPlace{JD: 2446875.0, Lon: lon, Lat: lat}, recipe.Placidus, recipe.Tropical)
	if err != nil {
		t.Fatalf("FormatChartFile at %v,%v: %v", lon, lat, err)
	}
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, "-zl ") {
			return line
		}
	}
	t.Fatalf("no -zl line in %q", text)
	return ""
}

// qbCoordinates is the longitude and latitude of a record's -qb line.
func qbCoordinates(record string) string {
	for _, line := range strings.Split(record, "\n") {
		if f := strings.Fields(line); len(f) > 2 && f[0] == "-qb" {
			return f[len(f)-2] + " " + f[len(f)-1]
		}
	}
	return ""
}

// The record a save writes for a floating slot at place, as far as this test
// is concerned: astrolog keeps the whole arc-minutes it is handed (it cuts
// finer text, never rounds it), so the -qb coordinates are the compile argv's
// own, and the -zi place label is savedPlaceLabel's, as astrolog records it.
func savedRecordText(zl, label string) string {
	f := strings.Fields(zl)
	return "@AI800  ; Astrolog chart info.\n" +
		"-qb Mar 14 1987  4:30pm ST 1E  " + f[1] + " " + f[2] + "\n" +
		`-zi "Exhaustive" "` + astrolog.RecordLabel(label) + "\"\n"
}

// castText is a compute -zl text with a zero written W or S read as the E or N
// the reader gives back: "0:00W" and "0:00E" are the same longitude (the
// reader's -0 formats as E), so the chart Astrolog casts is the same one. See
// TestZeroWestAndZeroEastCastTheSameChart, which proves that against astrolog.
func castText(zl string) string {
	f := strings.Fields(zl)
	f[1] = strings.Replace(f[1], "0:00W", "0:00E", 1)
	f[2] = strings.Replace(f[2], "0:00S", "0:00N", 1)
	return strings.Join(f, " ")
}

// exception is an atlas place that did not come back as itself.
type exception struct{ entry, reopened location.PlaceInfo }

func (e exception) String() string {
	return fmt.Sprintf("%s (%v,%v %s) -> %s (%v,%v %s)",
		e.entry.Display(), e.entry.Lon, e.entry.Lat, e.entry.TZ,
		e.reopened.Display(), e.reopened.Lon, e.reopened.Lat, e.reopened.TZ)
}

// P1: for EVERY entry of the real atlas, a floating slot saved there reopens
// as the same entry (name, country, timezone) and casts from the same
// coordinates, written exactly as the save writes them and read back by the
// reader the library uses.
func TestEveryAtlasPlaceReopensAsItself(t *testing.T) {
	path, err := location.AtlasPath()
	if err != nil {
		t.Skipf("no atlas on this host, so there is nothing to round-trip: %v", err)
	}
	atlas, err := location.LoadAtlas(path)
	if err != nil {
		t.Skipf("atlas unreadable: %v", err)
	}
	file := filepath.Join(t.TempDir(), "slot.as")
	var bad []exception
	zlOf := map[[2]float64]string{}
	for _, e := range atlas {
		label, _ := savedPlaceLabel(e)
		zl, ok := zlOf[[2]float64{e.Lon, e.Lat}]
		if !ok {
			zl = stagedZL(t, e.Lon, e.Lat)
			zlOf[[2]float64{e.Lon, e.Lat}] = zl
		}
		if err := os.WriteFile(file, []byte(savedRecordText(zl, label)), 0o644); err != nil {
			t.Fatal(err)
		}
		h, err := astrolog.ReadChartFileHeader(file)
		if err != nil {
			t.Fatalf("%s: %v", e.Display(), err)
		}
		got, ok := location.NearestNamed(atlas, h.Lon, h.Lat, h.PlaceLabel)
		if !ok || got.Name != e.Name || got.Country != e.Country || got.TZ != e.TZ ||
			castText(stagedZL(t, h.Lon, h.Lat)) != castText(zl) {
			bad = append(bad, exception{e, got})
		}
	}
	if len(bad) > 0 {
		var sb strings.Builder
		for i, b := range bad {
			if i == 300 {
				fmt.Fprintf(&sb, "\n... and %d more", len(bad)-i)
				break
			}
			sb.WriteString("\n" + b.String())
		}
		t.Errorf("%d of %d atlas places do not reopen as themselves:%s", len(bad), len(atlas), sb.String())
	}
}

// P1, through the real writer: astrolog itself writes the record and the
// library's reader and NearestNamed take it back, for places that are the
// likely trouble (a neighbour within an arc-minute, a label over the 54-byte
// cut, western and southern hemispheres) and an even sample of the rest.
func TestSavedFloatingSlotReopensAsItselfThroughAstrolog(t *testing.T) {
	path, err := location.AtlasPath()
	if err != nil {
		t.Skipf("no atlas on this host: %v", err)
	}
	atlas, err := location.LoadAtlas(path)
	if err != nil {
		t.Skipf("atlas unreadable: %v", err)
	}
	if err := astrolog.Available(); err != nil {
		t.Skipf("astrolog not installed: %v", err)
	}
	step := 400
	if os.Getenv("SEXTILE_FULL_ATLAS_SAVE") != "" {
		step = 1
	}
	// Every step-th place, the places a save is most likely to get wrong (the
	// neighbours of Berlin, the longest Displays, all of which are cut), and
	// the first of each hemisphere pairing.
	var sample []location.PlaceInfo
	cutSeen, longest := 0, 0
	for i, e := range atlas {
		label, cut := savedPlaceLabel(e)
		pick := i%step == 0 || strings.HasPrefix(e.Name, "Berlin") && e.Country == "DE" ||
			e.Name == "Mitte" && e.Country == "DE" || e.Name == "Hamburg-Mitte"
		if cut != "" && cutSeen < 12 && label != "" {
			cutSeen++
			pick = true
		}
		if len(e.Display()) > longest {
			longest, pick = len(e.Display()), true
		}
		if pick {
			sample = append(sample, e)
		}
	}
	var sawBerlin, sawMitte, sawCut bool
	for _, e := range sample {
		label, cut := savedPlaceLabel(e)
		record, err := astrolog.ChartRecord(context.Background(), astrolog.ChartToSave{
			Name: "Exhaustive", Place: label,
			When: time.Date(1987, 3, 14, 16, 30, 0, 0, time.FixedZone("CET", 3600)),
			Lon:  e.Lon, Lat: e.Lat,
		})
		if err != nil {
			t.Fatalf("ChartRecord at %s: %v", e.Display(), err)
		}
		file := filepath.Join(t.TempDir(), "slot.as")
		if err := os.WriteFile(file, record, 0o644); err != nil {
			t.Fatal(err)
		}
		h, err := astrolog.ReadChartFileHeader(file)
		if err != nil {
			t.Fatalf("%s: %v", e.Display(), err)
		}
		got, ok := location.NearestNamed(atlas, h.Lon, h.Lat, h.PlaceLabel)
		if !ok || got.Name != e.Name || got.Country != e.Country || got.TZ != e.TZ {
			t.Errorf("%s saved and reopened as %s", e.Display(), got.Display())
		}
		if a, b := computeZL(t, e.Lon, e.Lat), computeZL(t, h.Lon, h.Lat); castText(a) != castText(b) {
			t.Errorf("%s: compute argv %q before saving, %q after reopening", e.Display(), a, b)
		}
		// What the exhaustive test assumes: astrolog wrote the coordinates it
		// was handed, and the staged writer formats them as compute does.
		zl := computeZL(t, e.Lon, e.Lat)
		if q := qbCoordinates(string(record)); castText("-zl "+q) != castText(zl) {
			t.Errorf("%s: the record carries %q, the compute argv %q", e.Display(), q, zl)
		}
		if st := stagedZL(t, e.Lon, e.Lat); st != zl {
			t.Errorf("%s: staged writer %q, compute argv %q", e.Display(), st, zl)
		}
		sawBerlin = sawBerlin || e.Display() == "Berlin, DE"
		sawMitte = sawMitte || e.Display() == "Mitte, DE"
		sawCut = sawCut || cut != ""
	}
	if step > 1 && (!sawBerlin || !sawMitte || !sawCut) {
		t.Errorf("the sample must hold Berlin (%v), Mitte (%v) and a cut label (%v)", sawBerlin, sawMitte, sawCut)
	}
}

// A zero longitude or latitude has no side: astrolog writes 0:00W for what
// compute sent as 0:00E, and the reader's -0 formats as E again. The two must
// be one chart, or castText would be hiding a difference.
func TestZeroWestAndZeroEastCastTheSameChart(t *testing.T) {
	if err := astrolog.Available(); err != nil {
		t.Skipf("astrolog not installed: %v", err)
	}
	cast := func(lon, lat string) string {
		out, err := exec.Command("astrolog", "-qj", "2446875.0", "-zl", lon, lat, "-v0").CombinedOutput()
		if err != nil {
			t.Fatalf("astrolog -zl %s %s: %v\n%s", lon, lat, err, out)
		}
		if len(out) < 100 {
			t.Fatalf("astrolog printed no chart: %q", out)
		}
		return string(out)
	}
	if w, e := cast("0:00W", "51:30N"), cast("0:00E", "51:30N"); w != e {
		t.Errorf("-zl 0:00W and 0:00E cast different charts:\n%s\n%s", w, e)
	}
	if s, n := cast("10:00E", "0:00S"), cast("10:00E", "0:00N"); s != n {
		t.Errorf("-zl 0:00S and 0:00N cast different charts:\n%s\n%s", s, n)
	}
}
