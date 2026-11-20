package astrolog

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"

	"github.com/cruiserxyz/sextile/internal/recipe"
)

const chironRange = "Chiron's ephemeris is restricted to JD 1967601.5 - JD 3419437.5"

// chironShim is an Astrolog that warns about Chiron whenever the argv names
// object 11, and prints an empty list.
func chironShim(t *testing.T) (argvLog string) {
	t.Helper()
	return warnShim(t, `case " $* " in *" 11 "*) echo "`+chironRange+`" >&2;; esac
echo "No transits found."`)
}

func searchCallFor(t *testing.T, src TimelineSource, o recipe.ObjectPolicy) (BodyCall, *LeftOut) {
	t.Helper()
	req := bandReq(t, o, recipe.AspectPolicy{})
	req.Registry = testRegistry(t)
	req.AspectRegistry = testAspects(t)
	left := &LeftOut{}
	req.LeftOut = left
	call, err := TimelineCall(src, TimelineSegments(req.Year, req.Month, req.Zone)[0], req)
	if err != nil {
		t.Fatal(err)
	}
	return call, left
}

// A search whose Astrolog warns that one body has no ephemeris at the date is
// run again without that body and lists the other events, with no error; the
// body and Astrolog's own line are kept for the large log.
func TestSearchLeavesOutABodyWithoutEphemerisQuietly(t *testing.T) {
	for _, src := range []TimelineSource{TimelineTransit, TimelineReturn} {
		t.Run(src.String(), func(t *testing.T) {
			argvLog := chironShim(t)
			call, left := searchCallFor(t, src, hellenistic())
			out, _, err := RunTimelineSearch(context.Background(), call)
			if err != nil {
				t.Fatalf("a body without data refused the search: %v", err)
			}
			if out == "" {
				t.Errorf("the retry returned no output")
			}
			if !slices.Equal(left.Bodies, []string{"Chiron"}) || !slices.Equal(left.Lines, []string{chironRange}) {
				t.Errorf("left out = %+v, want Chiron and Astrolog's own line", *left)
			}
			argvs := spawnArgvs(t, argvLog)
			if len(argvs) != 2 || !slices.Contains(argvs[0], "11") || slices.Contains(argvs[1], "11") {
				t.Errorf("spawns = %q, want one with object 11 and a retry without it", argvs)
			}
		})
	}
}

// A search with no body left lists nothing: an empty list, not an error.
func TestSearchWithNoBodyLeftIsAnEmptyList(t *testing.T) {
	argvLog := chironShim(t)
	call, left := searchCallFor(t, TimelineTransit, recipe.ObjectPolicy{Baseline: recipe.Explicit, Selection: []int{11}})
	out, _, err := RunTimelineSearch(context.Background(), call)
	if err != nil || out != "" {
		t.Fatalf("out %q, err %v, want an empty answer", out, err)
	}
	if !slices.Equal(left.Bodies, []string{"Chiron"}) {
		t.Errorf("left out = %v, want Chiron", left.Bodies)
	}
	if n := len(spawnArgvs(t, argvLog)); n != 1 {
		t.Errorf("%d spawns, want 1: nothing is left to ask for", n)
	}
}

// A missing-file line names no object, and a search prints no no-data row that
// would: the call is run over halves of its objects until the line is pinned to
// Chiron, which is then left out and the search lists its events.
func TestSearchPinsAMissingFileToItsObject(t *testing.T) {
	argvLog := warnShim(t, `case " $* " in *" 11 "*) echo "`+missingLine("seas_30.se1")+`" >&2;; esac
echo "No transits found."`)
	call, left := searchCallFor(t, TimelineTransit, hellenistic())
	out, _, err := RunTimelineSearch(context.Background(), call)
	if err != nil || out == "" {
		t.Fatalf("out %q, err %v, want the retry's answer", out, err)
	}
	if !slices.Equal(left.Bodies, []string{"Chiron"}) {
		t.Errorf("left out = %v, want only Chiron", left.Bodies)
	}
	argvs := spawnArgvs(t, argvLog)
	if last := argvs[len(argvs)-1]; slices.Contains(last, "11") {
		t.Errorf("the last spawn still asks for Chiron: %q", last)
	}
}

// A chart call warned with an asteroid line that names no object of the
// registry, and whose output shows no no-data row to match, is cast again
// without the body the halving probe pins it to: the cast succeeds with the
// others. The stand-in runs the real program and adds the line whenever
// Hygiea (43) is in the argv.
func TestChartCastDropsABodyWhoseWarningNamesNoObject(t *testing.T) {
	needAstrolog(t)
	real, err := exec.LookPath(binary)
	if err != nil {
		t.Skip("no astrolog on PATH")
	}
	script := `#!/bin/sh
case " $* " in *" 43 "*) echo "asteroid No. 10 (se00010.se1): jd 1684536.1 < lower limit 2268932.5;" >&2;; esac
exec '` + real + `' "$@"
`
	path := filepath.Join(t.TempDir(), "astrolog")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	old := binary
	binary = path
	t.Cleanup(func() { binary = old })

	spec, err := recipe.NewChartSpec(recipe.MomentAtPlace{JD: 2451545.0, Lon: 4.9, Lat: 52.37}, recipe.Placidus, recipe.Tropical,
		recipe.ObjectPolicy{Baseline: recipe.Explicit, Selection: []int{1, 2, 43}})
	if err != nil {
		t.Fatal(err)
	}
	spec.Registry = testRegistry(t)
	spec.AspectRegistry = testAspects(t)
	out, err := ComputeChart(context.Background(), spec)
	if err != nil {
		t.Fatalf("the cast was refused: %v", err)
	}
	if !slices.Equal(out.Unavailable, []string{"Hygiea"}) {
		t.Errorf("left out %v, want only Hygiea", out.Unavailable)
	}
	if len(out.Positions) == 0 {
		t.Error("no position left")
	}
	for _, p := range out.Positions {
		if p.Object.Name == "Hygiea" {
			t.Error("Hygiea is in the positions")
		}
	}
}
