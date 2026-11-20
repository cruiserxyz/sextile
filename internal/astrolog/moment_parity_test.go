package astrolog

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/cruiserxyz/sextile/internal/recipe"
)

// The parity half of the moment oracle: for every chart file in
// testdata/moments, the chart Sextile asks Astrolog for (the compiler's argv,
// which carries the file's guarded record inline) is the chart Astrolog casts
// when it reads the file itself.
// Both runs print seconds (`=b0`), so every body, both angles and all twelve
// cusps are compared to the arc-second. Sextile's settings in the spec are
// Astrolog's own defaults (Placidus, tropical, the mean node), so where the
// file names none, both runs use the same; where the file names its own, the
// file's stands in both.

var (
	reParityPos  = regexp.MustCompile(`^([A-Z][A-Za-z.]{0,3} *): *([0-9]{1,2}[A-Z][a-z]{2}[0-9]{2}'[0-9]{2}")`)
	reParityCusp = regexp.MustCompile(`<([0-9]{1,2})> *([0-9]{1,2}[A-Z][a-z]{2}[0-9]{2}'[0-9]{2}")|([0-9]{1,2}[A-Z][a-z]{2}[0-9]{2}'[0-9]{2}")<([0-9]{1,2})>`)
)

// parityRead is the bodies, angles and cusps one run prints, keyed by
// Astrolog's own four-letter label or by the cusp's number.
func parityRead(t *testing.T, listing, wheel string) map[string]string {
	t.Helper()
	got := map[string]string{}
	for _, l := range strings.Split(listing, "\n") {
		if m := reParityPos.FindStringSubmatch(l); m != nil {
			got[strings.TrimSpace(m[1])] = m[2]
		}
	}
	for _, m := range reParityCusp.FindAllStringSubmatch(wheel, -1) {
		if m[1] != "" {
			got["cusp "+m[1]] = m[2]
		} else {
			got["cusp "+m[4]] = m[3]
		}
	}
	return got
}

// everyBodyRow is Astrolog's label of every row the parity compares: the
// planets, Chiron, the four asteroids, both Nodes, Lilith, Fortune, the Vertex
// and the East Point, so a change to any of them shows (the Part of Fortune
// moves under `-YP`, for one).
var everyBodyRow = []string{"Sun", "Moon", "Merc", "Venu", "Mars", "Jupi", "Satu", "Uran", "Nept", "Plut", "Chir", "Cere", "Pall", "Juno", "Vest", "Nort", "Sout", "Lili", "Fort", "Vert", "East"}

// everyBody is the switches that list the rows of everyBodyRow, each of which
// both runs must print.
var everyBody = []string{"-R0", "-R", "1", "2", "3", "4", "5", "6", "7", "8", "9", "10", "11", "12", "13", "14", "15", "16", "17", "18", "19", "20", "21"}

func runAstrolog(t *testing.T, argv ...string) string {
	t.Helper()
	out, err := exec.Command("astrolog", argv...).CombinedOutput()
	if err != nil {
		t.Fatalf("astrolog %q: %v\n%s", argv, err, out)
	}
	return string(out)
}

func TestFileChartIsTheFilesChartToTheArcSecond(t *testing.T) {
	needAstrolog(t)
	dir := filepath.Join("testdata", "moments")
	ents, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range ents {
		file := e.Name()
		t.Run(file, func(t *testing.T) {
			path, _ := filepath.Abs(filepath.Join(dir, file))
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			truth := parityRead(t,
				runAstrolog(t, append([]string{"-i", path, "=b0", "=Yt", "-v"}, everyBody...)...),
				runAstrolog(t, "-i", path, "=b0", "-w"))
			cf, err := ResolveChartBytes(context.Background(), raw, path)
			if err != nil {
				t.Fatalf("Sextile does not read %s: %v", file, err)
			}
			spec, err := recipe.NewChartSpec(
				recipe.FileChart{Record: cf.Record, JD: cf.JD, Lon: cf.Lon, Lat: cf.Lat},
				recipe.Placidus, recipe.Tropical, recipe.ObjectPolicy{Baseline: recipe.AstrologDefault})
			if err != nil {
				t.Fatal(err)
			}
			argv, temps, _, err := emitPrefix(spec, true, true)
			if err != nil {
				t.Fatal(err)
			}
			stage := t.TempDir()
			for i, ti := range temps {
				p := filepath.Join(stage, "chart.as")
				if err := os.WriteFile(p, []byte(ti.Content), 0o600); err != nil {
					t.Fatal(err)
				}
				argv[ti.ArgIndex] = p
				_ = i
			}
			got := parityRead(t,
				runAstrolog(t, append(append(append([]string(nil), argv...), "=b0", "-v"), everyBody...)...),
				runAstrolog(t, append(append([]string(nil), argv...), "=b0", "-w")...))
			if len(truth) < 20 {
				t.Fatalf("Astrolog's own reading gave only %d values: %v", len(truth), truth)
			}
			for _, k := range append(everyBodyRow, "cusp 1", "cusp 12") {
				if truth[k] == "" {
					t.Errorf("Astrolog's own reading has no %s: %v", k, truth)
				}
			}
			for k, want := range truth {
				if got[k] != want {
					t.Errorf("%s: Sextile's chart %q, Astrolog reading the file %q", k, got[k], want)
				}
			}
		})
	}
}

// The zodiac half of a chart file's row tag is Sextile's reading of the file
// (FileZodiac), so it is held to what Astrolog cast: for every file that sets
// a zodiac, Astrolog reading the file, and the same file cast again under the
// zodiac FileZodiac names (tropical, or `=s <offset>` as a plain number), print
// every body at the same arc-second.
func TestFileZodiacIsTheZodiacAstrologCast(t *testing.T) {
	needAstrolog(t)
	dir := filepath.Join("testdata", "moments")
	ents, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var paths []string
	for _, e := range ents {
		path, _ := filepath.Abs(filepath.Join(dir, e.Name()))
		paths = append(paths, path)
	}
	// The ways Astrolog spells an offset: decimals, minutes and seconds, the
	// compact Astrodienst MMSS, a sign, and a name by a prefix.
	spelled := t.TempDir()
	for i, offset := range []string{"0:3045", "24:0730", "24:07:30", "-1:30", "+23.5", "Fag", "lahiri"} {
		path := filepath.Join(spelled, fmt.Sprintf("offset-%d.as", i))
		text := "@AI800\n-qb Jun 1 1990 16:30 DT 1E 4:53E 52:22N\n=s " + offset + "\n"
		if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, path)
	}
	checked := 0
	for _, path := range paths {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		guarded, _, err := GuardChartFile(string(raw))
		if err != nil {
			t.Fatal(err)
		}
		set, sidereal, offset := FileZodiac(guarded)
		if !set {
			continue
		}
		checked++
		t.Run(filepath.Base(path), func(t *testing.T) {
			named := []string{"_s"}
			if sidereal {
				named = []string{"=s", OffsetArg(offset)}
			}
			truth := parityRead(t, runAstrolog(t, "-i", path, "=b0", "=Yt", "-v"), "")
			ours := parityRead(t, runAstrolog(t, append(append([]string{"-i", path}, named...), "=b0", "=Yt", "-v")...), "")
			if len(truth) < 10 {
				t.Fatalf("Astrolog's own reading gave only %d values", len(truth))
			}
			for k, want := range truth {
				if ours[k] != want {
					t.Errorf("%s: under FileZodiac's %v %v, %q; Astrolog reading the file, %q", k, sidereal, offset, ours[k], want)
				}
			}
		})
	}
	if checked < 11 {
		t.Errorf("only %d files set a zodiac; the oracle needs the sidereal and the named ones", checked)
	}
}
