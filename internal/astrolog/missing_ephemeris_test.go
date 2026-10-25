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

	t.Run("shim, full result carries both invocations' files", func(t *testing.T) {
		warnShim(t, replayScript(t))
		clean, err := Run(ctx, testCall(singleChartArgv))
		if err != nil {
			t.Fatal(err)
		}
		if len(clean.MissingEphemeris) != 0 {
			t.Fatalf("no stderr, yet MissingEphemeris = %v", clean.MissingEphemeris)
		}
		warnShim(t, both+replayScript(t))
		got, err := Run(ctx, testCall(singleChartArgv))
		if err != nil {
			t.Fatalf("the expected line must not refuse: %v", err)
		}
		if want := []string{posFile, aspFile}; !reflect.DeepEqual(got.MissingEphemeris, want) {
			t.Fatalf("MissingEphemeris = %v, want %v (positions first, then aspects)", got.MissingEphemeris, want)
		}
		if !reflect.DeepEqual(sansMissing(got), sansMissing(clean)) {
			t.Errorf("a pre-existing field changed when the stderr line appeared")
		}
		// Fixed expectations, not another call through the new code: the replayed
		// positions block has no no-data row, so the full path always named
		// nothing unavailable here.
		if len(got.Unavailable) != 0 || len(got.Positions) == 0 || got.AspectsReport == "" {
			t.Errorf("full result drifted from what the path always produced: Unavailable %v, %d positions, report %q", got.Unavailable, len(got.Positions), got.AspectsReport)
		}
		if strings.Contains(got.Raw, "se0001") || strings.Contains(got.Raw, "SwissEph") {
			t.Errorf("the expected line leaked into Raw")
		}
	})
	t.Run("shim, positions-only result", func(t *testing.T) {
		warnShim(t, replayScript(t))
		clean, err := RunPositionsOnly(ctx, testCall(singleChartArgv))
		if err != nil {
			t.Fatal(err)
		}
		warnShim(t, both+replayScript(t))
		got, err := RunPositionsOnly(ctx, testCall(singleChartArgv))
		if err != nil {
			t.Fatalf("the expected line must not refuse: %v", err)
		}
		if want := []string{posFile}; !reflect.DeepEqual(got.MissingEphemeris, want) {
			t.Fatalf("MissingEphemeris = %v, want %v", got.MissingEphemeris, want)
		}
		if !reflect.DeepEqual(sansMissing(got), sansMissing(clean)) {
			t.Errorf("a pre-existing field changed when the stderr line appeared")
		}
		// The positions-only path never filled Unavailable (it would feed the
		// "not drawn" Log note); it must stay empty.
		if got.Unavailable != nil || got.AspectsReport != "" || len(got.Aspects) != 0 {
			t.Errorf("positions-only result drifted: Unavailable %v, aspects %d", got.Unavailable, len(got.Aspects))
		}
		if strings.Contains(got.Raw, "SwissEph") {
			t.Errorf("the expected line leaked into Raw")
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
		spec, err := recipe.NewChartSpec(hookOther, recipe.WholeSign, recipe.Tropical, recipe.ObjectPolicy{Baseline: recipe.Explicit, Selection: []int{43}})
		if err != nil {
			t.Fatal(err)
		}
		for name, run := range map[string]func(context.Context, recipe.ChartSpec) (payload.RawOutput, error){
			"full": ComputeChart, "positions-only": ComputeChartPositionsOnly,
		} {
			out, err := run(ctx, spec)
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			if len(out.Positions) > 0 {
				t.Skip("the Hygiea file is present on this host")
			}
			if len(out.MissingEphemeris) != 1 || !strings.HasSuffix(out.MissingEphemeris[0], ".se1") {
				t.Errorf("%s: MissingEphemeris = %v, want the one missing .se1 file", name, out.MissingEphemeris)
			}
			if strings.Contains(out.Raw, "SwissEph") {
				t.Errorf("%s: the expected line leaked into Raw", name)
			}
			if name == "positions-only" && len(out.Unavailable) != 0 {
				t.Errorf("positions-only Unavailable = %v, want empty as before this slice", out.Unavailable)
			}
			if name == "full" && (len(out.Unavailable) != 1 || !strings.HasPrefix(out.Unavailable[0], "Hygi")) {
				t.Errorf("full Unavailable = %v, want Hygiea as the full path always named it", out.Unavailable)
			}
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
	spec, err := recipe.NewChartSpec(hookOther, recipe.WholeSign, recipe.Tropical, recipe.ObjectPolicy{Baseline: recipe.Explicit, Selection: []int{43}})
	if err != nil {
		t.Fatal(err)
	}
	out, err := ComputeChartPositionsOnly(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.MissingEphemeris) == 0 {
		t.Skip("the Hygiea file is present on this host")
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
