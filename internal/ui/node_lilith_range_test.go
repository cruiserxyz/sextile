package ui

// N16: a natural Lilith that Astrolog cannot place is left out and named, never
// shown at 0 Aries.

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/internal/recipe"
)

// outOfRangeLilith puts a stand-in for astrolog first on PATH: it runs the real
// one and, for the casts that toggle objects with -R (the start-up probe, which lists its own, stays real), says what Astrolog says for a date past its interpolated
// apogee: Lilith printed at 0 Aries and, on stderr, the one warning that names
// no object (sweph.cpp, SE_INTP_APOG). No data file of this installation covers
// a date outside JD 625000.5 to 2818000.5, so the real binary cannot be asked.
func outOfRangeLilith(t *testing.T) {
	t.Helper()
	real, err := exec.LookPath("astrolog")
	if err != nil {
		t.Skip("astrolog not on PATH; skipping live-binary test")
	}
	dir := t.TempDir()
	script := `#!/bin/sh
LC_ALL=C
export LC_ALL
out=$('` + real + `' "$@"); rc=$?
case " $* " in
*" -R "*) ;;
*) printf '%s\n' "$out"; exit $rc ;;
esac
case "$out" in
*"Lili: "*) echo "Interpolated apsides are restricted to JD 625000.5 - JD 2818000.5" >&2 ;;
esac
printf '%s\n' "$out" | sed -E "/Lili: /{s/(Lili: .\[[0-9;]*m)[ 0-9][0-9][A-Za-z]{3}[0-9]{2}/\\1 0Ari00/;s/(\\[0m)( R)? +[-+] [^']*'/\\1   + 0:00'/;s/(\\[[-a-zA-Z]\\]) [-+][0-9.]+/\\1 +0.000/}"
exit $rc
`
	if err := os.WriteFile(filepath.Join(dir, "astrolog"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestNaturalLilithOutOfRangeIsLeftOut(t *testing.T) {
	outOfRangeLilith(t)
	m, _ := liveScreens(t)
	m, cmd := m.chooseNodes(len(m.slots.All())-1, recipe.NodesMeanNatural)
	t.Cleanup(func() {
		if t.Failed() {
			t.Log(strings.Join(logTexts(m), "\n"))
		}
	})
	m = pumpPatient(t, m, cmd)

	view := ansi.Strip(m.View())
	for _, l := range strings.Split(view, "\n") {
		if strings.Contains(l, "Lilith") && strings.Contains(l, "Ari") {
			t.Errorf("Lilith is on screen at 0 Aries: %q", l)
		}
	}
	// Named: the model's own cast of Now, asked in that form, answers with Lilith
	// as the body it could not place and no row for it.
	now := m.slots.Floating()
	lens := m.lensFor(&now)
	r, err := m.rows[surfaceText].spawn.cast.transit(context.Background(), m.probers, now.Time, now.Location, lens)
	if err != nil {
		t.Fatalf("the cast was refused: %v", err)
	}
	if !slices.Equal(r.Unavailable, []string{"Lilith"}) {
		t.Errorf("the cast names %v as left out, want [Lilith]", r.Unavailable)
	}
	for _, p := range r.Positions {
		if p.Object.Name == "Lilith" {
			t.Errorf("the cast holds Lilith at %v", p.Longitude)
		}
	}
	for _, l := range logTexts(m) {
		if strings.Contains(l, " not cast,") {
			t.Errorf("the chart was refused whole: %q", l)
		}
	}
}
