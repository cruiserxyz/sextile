package astrolog

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cruiserxyz/sextile/internal/recipe"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// blockStaging points the private temp base at a regular file, so that any
// attempt to stage a chart file fails at once with a *PrivateTempError, inside
// the call. A guard that runs after staging then shows as that error instead
// of ErrAspectsUnread.
func blockStaging(t *testing.T) {
	t.Helper()
	blocker := filepath.Join(t.TempDir(), "not-a-folder")
	if err := os.WriteFile(blocker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CACHE_HOME", blocker)
}

func isStagingRefusal(err error) bool {
	var pe *PrivateTempError
	return errors.As(err, &pe)
}

var guardChartFile = recipe.FileChart{Record: "@AI800\n-qb Dec 27 1571 14:37 ST LMT 8:52E 48:45N\n", JD: 2299000.1, Lon: 8.87, Lat: 48.75}

// guardPair is a comparison: the one chart source Compile stages files for.
func guardPair(t *testing.T, reg aspect.AspectRegistry) recipe.ChartSpec {
	t.Helper()
	spec := fixtureSpec(t, recipe.Comparison{Primary: fixLondon, Secondary: fixTromso, Relation: recipe.TransitToNatal}, recipe.Placidus, recipe.Tropical)
	spec.AspectRegistry = reg
	return spec
}

// With staging blocked, a comparison whose registry IS read reaches staging and
// is refused there: the seam sees staging.
func TestBlockedStagingIsSeenByAComparison(t *testing.T) {
	blockStaging(t)
	_, argvLog := fakeBinary(t, t.TempDir(), "exit 0")
	spec := guardPair(t, testAspects(t))
	plan, err := Compile(spec, recipe.Aspects{})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, cleanup, err := stageCall(plan.Invocations[0]); !isStagingRefusal(err) {
		cleanup()
		t.Fatalf("staging a comparison's charts: err = %v, want a *PrivateTempError", err)
	}
	if got := spawnArgvs(t, argvLog); len(got) != 0 {
		t.Errorf("the binary was started %d times", len(got))
	}
}

// An unread aspect registry is refused before any chart file is staged, in
// every aspect-producing entry point. Staging is blocked, so a guard placed
// below it would return the staging refusal and fail here.
func TestUnreadAspectRegistryIsRefusedBeforeStaging(t *testing.T) {
	blockStaging(t)
	_, argvLog := fakeBinary(t, t.TempDir(), "exit 0")

	pair := guardPair(t, aspect.AspectRegistry{})
	single := fixtureSpec(t, guardChartFile, recipe.Placidus, recipe.Tropical)
	single.AspectRegistry = aspect.AspectRegistry{}
	req := TimelineRequest{Year: 2026, Month: time.October, Zone: time.UTC, Registry: testRegistry(t)}

	calls := map[string]func() error{
		"ComputeChart":        func() error { _, err := ComputeChart(context.Background(), single); return err },
		"ComputeCrossAspects": func() error { _, err := ComputeCrossAspects(context.Background(), pair); return err },
		"ComputeTimeline": func() error {
			_, _, err := ComputeTimeline(context.Background(), req, []TimelineSource{TimelineSky}, nil)
			return err
		},
		"ComputeTimelineCalls": func() error { _, err := ComputeTimelineCalls(context.Background(), req, TimelineSky, nil); return err },
		"RunVoidSearch": func() error {
			_, err := RunVoidSearch(context.Background(), VoidRequest{Moment: time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC), Registry: testRegistry(t)}, nil)
			return err
		},
		"Compile then Run": func() error {
			plan, err := Compile(pair, recipe.Aspects{})
			if err != nil {
				return err
			}
			_, err = Run(context.Background(), plan.Invocations[0].call)
			return err
		},
	}
	for name, call := range calls {
		err := call()
		if isStagingRefusal(err) {
			t.Errorf("%s: reached staging before refusing the unread registry: %v", name, err)
		} else if !errors.Is(err, ErrAspectsUnread) {
			t.Errorf("%s: err = %v, want ErrAspectsUnread", name, err)
		}
	}
	if got := spawnArgvs(t, argvLog); len(got) != 0 {
		t.Errorf("the binary was started %d times with no aspect registry", len(got))
	}
}
