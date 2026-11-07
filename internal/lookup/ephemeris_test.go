package lookup

import (
	"context"
	"slices"
	"testing"

	"github.com/cruiserxyz/sextile/internal/astrolog"
	"github.com/cruiserxyz/sextile/internal/recipe"
	"github.com/cruiserxyz/sextile/internal/testenv"
)

func stubEphemeris(t *testing.T, folder *string, calls *int, av astrolog.Availability) {
	t.Helper()
	oldProbe, oldFolder := ephemerisProbe, ephemerisFolder
	ephemerisProbe = func(context.Context) (astrolog.Availability, error) { *calls++; return av, nil }
	ephemerisFolder = func() string { return *folder }
	ephemerisMu.Lock()
	ephemerisCache = map[string]astrolog.Availability{}
	ephemerisMu.Unlock()
	t.Cleanup(func() {
		ephemerisProbe, ephemerisFolder = oldProbe, oldFolder
		ephemerisMu.Lock()
		ephemerisCache = map[string]astrolog.Availability{}
		ephemerisMu.Unlock()
		testenv.InstallHostDefault()
	})
}

func TestProbeEphemerisIsOncePerFolderAndInstallsTheAnswer(t *testing.T) {
	folder, calls := "A", 0
	av := astrolog.Availability{Absent: map[int]string{12: "seas_18.se1"}, Names: map[int]string{12: "Ceres"}}
	stubEphemeris(t, &folder, &calls, av)
	for i := 0; i < 3; i++ {
		got, err := ProbeEphemeris(context.Background())
		if err != nil || got.Folder != "A" || !got.IsAbsent(12) {
			t.Fatalf("probe %d = %+v, %v", i, got, err)
		}
	}
	if calls != 1 {
		t.Errorf("the probe ran %d times for one folder, want 1", calls)
	}
	if !astrolog.CurrentAvailability().IsAbsent(12) {
		t.Error("the answer was not installed for the argv builders")
	}
	folder = "B"
	if _, err := ProbeEphemeris(context.Background()); err != nil || calls != 2 {
		t.Errorf("a different folder: calls = %d, err %v; want a second probe", calls, err)
	}
}

func TestOnScreenObjectsIncludesTheHostDefaultOnlyForADelta(t *testing.T) {
	av := astrolog.Availability{Default: []int{1, 12}}
	delta := Lens{}
	got, err := OnScreenObjects(av, delta)
	if err != nil || !contains(got, 12) {
		t.Errorf("delta lens on screen = %v, %v; want the host default's Ceres in it", got, err)
	}
	explicit := Lens{Objects: recipe.ObjectPolicy{Baseline: recipe.Explicit, Selection: []int{1}}}
	got, err = OnScreenObjects(av, explicit)
	if err != nil || contains(got, 12) {
		t.Errorf("explicit lens on screen = %v, %v; want only what was ticked", got, err)
	}
	// Without the host's default set there is no answer, never a guess.
	if got, err := OnScreenObjects(astrolog.Availability{}, delta); err == nil {
		t.Errorf("no default set known: on screen = %v, want an error", got)
	}
}

func contains(s []int, n int) bool {
	for _, x := range s {
		if x == n {
			return true
		}
	}
	return false
}

// A mark on an object the host does not draw draws nothing, and the two
// consumers that live here say what the argv says (the other four are in
// internal/astrolog, TestDrawnIsTheHostDefaultWithFlips): the chooser's ticks
// and the ephemeris account.
func TestDrawnIsTheHostDefaultWithFlips(t *testing.T) {
	av := astrolog.Availability{
		Default: []int{1, 2, 3, 19, 22}, // no Midheaven (31)
		Absent:  map[int]string{12: "seas_18.se1"},
		Names:   map[int]string{12: "Ceres"},
	}
	astrolog.SetAvailability(av)
	t.Cleanup(testenv.InstallHostDefault)

	marked := Lens{Objects: recipe.ObjectPolicy{Only: recipe.NewObjectSet(1, 31)}}
	// The chooser's ticks: the Sun only, the Midheaven is not drawn.
	if got, err := marked.VisibleObjectNumbers(); err != nil || !slices.Equal(got, []int{1}) {
		t.Errorf("VisibleObjectNumbers = %v, %v; want [1]", got, err)
	}
	// The ephemeris account: what the view would draw, before the missing-file cut.
	if got, err := OnScreenObjects(av, marked); err != nil || !slices.Equal(got, []int{1}) {
		t.Errorf("OnScreenObjects = %v, %v; want [1]", got, err)
	}
	// Unmarked, on a host that lists the absent Ceres: the account names it (it
	// is what the view would draw), the ticks do not (it has no file).
	av.Default = []int{1, 2, 12, 22}
	astrolog.SetAvailability(av)
	plain := Lens{}
	if got, err := OnScreenObjects(av, plain); err != nil || !slices.Equal(got, []int{1, 2, 12, 22}) {
		t.Errorf("OnScreenObjects = %v, %v; want the host default [1 2 12 22], absent Ceres included", got, err)
	}
	if got, err := plain.VisibleObjectNumbers(); err != nil || !slices.Equal(got, []int{1, 2, 22}) {
		t.Errorf("VisibleObjectNumbers = %v, %v; want [1 2 22] (Ceres has no file)", got, err)
	}
	// Before the host's default set is known there is no answer.
	astrolog.SetAvailability(astrolog.Availability{})
	if got, err := plain.VisibleObjectNumbers(); err == nil {
		t.Errorf("VisibleObjectNumbers before the probe = %v, want an error", got)
	}
	narrowed := Lens{Objects: recipe.ObjectPolicy{Only: recipe.NewObjectSet(12)}}
	if got, err := narrowed.DrainedObjects(); err == nil {
		t.Errorf("DrainedObjects before the probe = %v, want an error", got)
	}
}
