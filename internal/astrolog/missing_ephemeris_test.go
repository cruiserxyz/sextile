package astrolog

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/cruiserxyz/sextile/internal/payload"
	"github.com/cruiserxyz/sextile/internal/recipe"
	"github.com/cruiserxyz/sextile/internal/testenv/astrodir"
)

// missingLine is Astrolog's own line for an absent ephemeris file.
func missingLine(file string) string {
	return "SwissEph file '" + file + "' not found in PATH '" + missingPath() + "'"
}

// missingPath is the search path that line names: the working directory and
// ephemeris folders, then the DEFAULT_DIR Astrolog was built with, which is
// where this host installed its data (scripts/build-astrolog, PREFIX), not
// always /usr/local/share/astrolog.
func missingPath() string {
	dir := originalDataDir
	if dir == "" {
		dir = "/usr/local/share/astrolog"
	}
	return ".;:;:ephem;:" + dir + "/"
}

// sansMissing blanks the one field C2 adds, so every OTHER field can be
// compared with the same call made before the field existed.
//
// Commands is compared by count: each shim lives in its own temp directory, so
// the recorded path legitimately differs between the two calls.
func sansMissing(r payload.RawOutput) payload.RawOutput {
	r.MissingEphemeris = nil
	r.Said = ""
	r.Commands = make([]string, len(r.Commands))
	return r
}

// C2 (and N1): the expected missing-ephemeris line is carried on the result as
// a file name, on both the full and the positions-only path, and never leaks
// into Raw. Every pre-existing field is compared with the same call made with
// no stderr at all.
func TestResultCarriesMissingEphemerisFiles(t *testing.T) {
	ctx := context.Background()
	const posFile, aspFile = "se00010s.se1", "se00011s.se1"
	both := "case \" $* \" in *\" -aa \"*) echo \"" + missingLine(aspFile) + "\" >&2;; *) echo \"" + missingLine(posFile) + "\" >&2;; esac\n"

	// Point 3 of the 2026-11-02 order: a `not found` line refuses every call that
	// did not ask for it. Only the probing positions call collects the file names.
	t.Run("shim, a full result with the line is refused", func(t *testing.T) {
		warnShim(t, both+replayScript(t))
		got, err := Run(ctx, testCall(singleChartArgv))
		if !errors.Is(err, ErrAstrologWarned) {
			t.Fatalf("Run err = %v, want ErrAstrologWarned", err)
		}
		if len(got.MissingEphemeris) != 0 || len(got.Positions) != 0 {
			t.Errorf("a refused result carried data: %+v", got)
		}
	})
	t.Run("shim, positions-only is refused and probing carries the file", func(t *testing.T) {
		warnShim(t, replayScript(t))
		clean, err := RunPositionsOnly(ctx, testCall(singleChartArgv))
		if err != nil {
			t.Fatal(err)
		}
		warnShim(t, both+replayScript(t))
		if _, err := RunPositionsOnly(ctx, testCall(singleChartArgv)); !errors.Is(err, ErrAstrologWarned) {
			t.Fatalf("RunPositionsOnly err = %v, want ErrAstrologWarned", err)
		}
		got, err := RunPositionsProbing(ctx, testCall(singleChartArgv))
		if err != nil {
			t.Fatalf("the probing call must not refuse: %v", err)
		}
		if want := []string{posFile}; !reflect.DeepEqual(got.MissingEphemeris, want) {
			t.Fatalf("MissingEphemeris = %v, want %v", got.MissingEphemeris, want)
		}
		if got.Said != missingLine(posFile) {
			t.Errorf("Said = %q, want the stderr line %q", got.Said, missingLine(posFile))
		}
		if !reflect.DeepEqual(sansMissing(got), sansMissing(clean)) {
			t.Errorf("a pre-existing field changed when the stderr line appeared")
		}
		if got.Unavailable != nil || got.AspectsReport != "" || len(got.Aspects) != 0 {
			t.Errorf("positions-only result drifted: Unavailable %v, aspects %d", got.Unavailable, len(got.Aspects))
		}
		if strings.Contains(got.Raw, "SwissEph") {
			t.Errorf("the line leaked into Raw")
		}
	})
	t.Run("an extra unexpected line still refuses", func(t *testing.T) {
		warnShim(t, "echo \""+missingLine(posFile)+"\" >&2; echo 'another warning' >&2\n"+replayScript(t))
		if _, err := Run(ctx, testCall(singleChartArgv)); !errors.Is(err, ErrAstrologWarned) {
			t.Fatalf("Run err = %v, want ErrAstrologWarned", err)
		}
		if _, err := RunPositionsOnly(ctx, testCall(singleChartArgv)); !errors.Is(err, ErrAstrologWarned) {
			t.Fatalf("RunPositionsOnly err = %v, want ErrAstrologWarned", err)
		}
	})
	t.Run("live Hygiea names its file", func(t *testing.T) {
		needAstrolog(t)
		astrodir.Reduced(t, []string{"se00010s.se1"}) // a host without the file, whatever this host has
		spec, err := recipe.NewChartSpec(hookOther, recipe.WholeSign, recipe.Tropical, recipe.ObjectPolicy{Baseline: recipe.Explicit, Selection: []int{43}})
		if err != nil {
			t.Fatal(err)
		}
		spec.Registry = testRegistry(t)
		// A normal compute refuses a missing file's line. The folder is reduced
		// above, so the refusal is required, not one of two possible answers.
		for name, run := range map[string]func(context.Context, recipe.ChartSpec) (payload.RawOutput, error){
			"full": ComputeChart, "positions-only": ComputeChartPositionsOnly,
		} {
			_, err := run(ctx, spec)
			if !errors.Is(err, ErrAstrologWarned) {
				t.Errorf("%s: err = %v, want a refusal (ErrAstrologWarned) for the missing file", name, err)
			}
		}
		// Only the probing variant collects the file name.
		out, err := ComputeChartProbe(ctx, spec)
		if err != nil {
			t.Fatalf("probe: %v", err)
		}
		if len(out.MissingEphemeris) != 1 || !strings.HasSuffix(out.MissingEphemeris[0], ".se1") {
			t.Errorf("probe: MissingEphemeris = %v, want the one missing .se1 file", out.MissingEphemeris)
		}
		if strings.Contains(out.Raw, "SwissEph") {
			t.Errorf("probe: the expected line leaked into Raw")
		}
		if len(out.Unavailable) != 0 {
			t.Errorf("probe Unavailable = %v, want empty", out.Unavailable)
		}
	})
}

// N2: a body Astrolog has no data for, asked for alone on the positions-only
// path, is no Position, is named by the file Astrolog reported missing, and
// leaves Unavailable exactly as it always was (empty on this path: filling it
// would add "not drawn" Log notes). The probe reads Positions and
// MissingEphemeris, so no-data cannot read as drawable.
func TestNoDataBodyIsNotDrawableOnPositionsOnly(t *testing.T) {
	needAstrolog(t)
	astrodir.Reduced(t, []string{"se00010s.se1"}) // a host without the file, whatever this host has
	spec, err := recipe.NewChartSpec(hookOther, recipe.WholeSign, recipe.Tropical, recipe.ObjectPolicy{Baseline: recipe.Explicit, Selection: []int{43}})
	if err != nil {
		t.Fatal(err)
	}
	spec.Registry = testRegistry(t)
	out, err := ComputeChartProbe(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.MissingEphemeris) == 0 {
		t.Fatal("the reduced folder still found the Hygiea file: the fixture does not reduce")
	}
	if len(out.Positions) != 0 {
		t.Errorf("a no-data body produced positions: %v", out.Positions)
	}
	if !slices.Contains(out.MissingEphemeris, "se00010s.se1") {
		t.Errorf("MissingEphemeris = %v, want it to name se00010s.se1", out.MissingEphemeris)
	}
	if len(out.Unavailable) != 0 {
		t.Errorf("Unavailable = %v, want empty as before this slice", out.Unavailable)
	}
}
