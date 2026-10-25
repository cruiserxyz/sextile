package ui

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/internal/recipe"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

const fixtureSpawnEnd = "@@"

// fixtureAstrolog installs a PATH-local Astrolog stand-in that records argv and
// replays the repository's constructed colored chart fixtures. A transform may
// alter only the aspect listing, which lets the omission tests drive unknown
// producer vocabulary through the real lookup/parser/UI path.
func fixtureAstrolog(t *testing.T, transform func(string) string) func() [][]string {
	t.Helper()
	dir := t.TempDir()
	positions, err := os.ReadFile(filepath.Join("..", "astrolog", "testdata", "provenance", "positions-k.txt"))
	if err != nil {
		t.Fatalf("read positions fixture: %v", err)
	}
	aspects, err := os.ReadFile(filepath.Join("..", "astrolog", "testdata", "provenance", "aspects-k.txt"))
	if err != nil {
		t.Fatalf("read aspects fixture: %v", err)
	}
	if transform != nil {
		aspects = []byte(transform(string(aspects)))
	}
	positionsPath := filepath.Join(dir, "positions.txt")
	aspectsPath := filepath.Join(dir, "aspects.txt")
	logPath := filepath.Join(dir, "argv.log")
	if err := os.WriteFile(positionsPath, positions, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(aspectsPath, aspects, 0o600); err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/sh\n" +
		"{ printf '%s\\n' \"$@\"; printf '%s\\n' '" + fixtureSpawnEnd + "'; } >> " + strconv.Quote(logPath) + "\n" +
		"case \" $* \" in\n" +
		"  *\" -aa \"*) exec /bin/cat " + strconv.Quote(aspectsPath) + " ;;\n" +
		"  *) exec /bin/cat " + strconv.Quote(positionsPath) + " ;;\n" +
		"esac\n"
	if err := os.WriteFile(filepath.Join(dir, "astrolog"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)

	return func() [][]string {
		data, err := os.ReadFile(logPath)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			t.Fatalf("read fake Astrolog argv: %v", err)
		}
		var out [][]string
		var current []string
		for _, line := range strings.Split(strings.TrimSuffix(string(data), "\n"), "\n") {
			if line == fixtureSpawnEnd {
				// C1: every spawn starts with -YQ 0 and the hook clears; the
				// tests are about the call's own argv.
				if own, _, ok := stripSpawnPrefix(current); !ok {
					t.Errorf("spawn argv %q does not start with -YQ 0 and the hook clears (C1)", current[:min(len(current), 6)])
				} else {
					current = own
				}
				out = append(out, current)
				current = nil
				continue
			}
			current = append(current, line)
		}
		return out
	}
}

func numberBand(argv []string, switchName string) []int {
	for i, token := range argv {
		if token != switchName {
			continue
		}
		var out []int
		for _, value := range argv[i+1:] {
			n, err := strconv.Atoi(value)
			if err != nil {
				break
			}
			out = append(out, n)
		}
		return out
	}
	return nil
}

func requireFirstSpawnBand(t *testing.T, spawns [][]string, switchName string, want []int) {
	t.Helper()
	if len(spawns) == 0 {
		t.Fatal("the requested compute started no Astrolog process")
	}
	if got := numberBand(spawns[0], switchName); !reflect.DeepEqual(got, want) {
		t.Fatalf("first Astrolog argv %s band = %v, want %v; argv=%v", switchName, got, want, spawns[0])
	}
}

func fixtureLookup(t *testing.T, lens lookup.Lens, transform func(string) string) (lookup.Result, [][]string) {
	t.Helper()
	spawns := fixtureAstrolog(t, transform)
	result, err := lookup.FirstCallTransits(
		context.Background(), nil,
		time.Date(1985, 6, 15, 12, 0, 0, 0, time.UTC),
		location.PlaceInfo{Name: "Constructed Test Place", Lat: 48.13, Lon: 11.57, TZ: "Etc/UTC"},
		lens,
	)
	if err != nil {
		t.Fatalf("fixture lookup: %v", err)
	}
	return result, spawns()
}

// C3: assert the complete UI→compiler→process boundary for both outcomes. The
// older half-tests stopped at the Lens seam; this one reads the argv received
// by the very next Astrolog process.
func TestBodyConfirmAppliesCancelRestores(t *testing.T) {
	oldChooserRule(t)
	entry := recipe.ObjectPolicy{Baseline: recipe.Explicit, Selection: []int{1, 2, 3}}
	cases := []struct {
		name string
		key  tea.KeyMsg
		want []int
	}{
		{name: "confirm applies the edited set", key: tea.KeyMsg{Type: tea.KeyEnter}, want: []int{0, 1, 2, 3}},
		{name: "cancel restores the entry set", key: tea.KeyMsg{Type: tea.KeyEsc}, want: []int{1, 2, 3}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			spawns := fixtureAstrolog(t, nil)
			m := readyModelWithSlots(t, testPlaceBerlin())
			m.width, m.height = 132, 40
			m.objectRegistry = testObjectRegistry()
			m.lens.Objects = entry

			tm, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlB})
			for i := 0; i < groupRows(); i++ { // past the group rows
				tm, _ = tm.(Model).Update(tea.KeyMsg{Type: tea.KeyDown})
			}
			tm, _ = tm.(Model).Update(tea.KeyMsg{Type: tea.KeySpace}) // tick Earth (registry row 0)
			_, cmd := tm.(Model).Update(tc.key)
			if cmd == nil {
				t.Fatal("closing an edited selector scheduled no recompute")
			}
			drainCmd(cmd)
			requireFirstSpawnBand(t, spawns(), "-R0", tc.want)
		})
	}
}

// C5a: the sole object-registry read is the startup command. Once it lands,
// opening and reopening under an Explicit policy must use the kept registry and
// issue no narrowing re-read.
func TestRegistryReadPrecedesAnyRestriction(t *testing.T) {
	old := objectRegistryRead
	t.Cleanup(func() { objectRegistryRead = old })
	reads := 0
	objectRegistryRead = func(context.Context) ([]aspect.RegistryObject, error) {
		reads++
		return testObjectRegistry(), nil
	}

	msg, ok := loadObjectRegistryCmd()().(ObjectRegistryLoadedMsg)
	if !ok || msg.Err != nil {
		t.Fatalf("startup registry command = %#v, want one successful read", msg)
	}
	if reads != 1 {
		t.Fatalf("startup registry reads = %d, want 1", reads)
	}

	m := readyModelWithObjects(nil)
	m, _ = applyObjectRegistryLoaded(m, msg)
	m.lens.Objects = recipe.ObjectPolicy{Baseline: recipe.Explicit, Selection: []int{1, 2}}
	for i := 0; i < 2; i++ {
		opened, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlB})
		if cmd != nil {
			t.Fatalf("open %d started work; the startup registry must be reused", i+1)
		}
		m = opened.(Model)
		closed, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
		if cmd != nil {
			t.Fatalf("unedited close %d started work", i+1)
		}
		m = closed.(Model)
	}
	if reads != 1 {
		t.Fatalf("opening under -R0 policy caused %d registry reads, want the single unrestricted startup read", reads)
	}
}

// C9: one body cannot form a pair. Exercise both a body Sextile can type and a
// runtime-registry object it cannot name, against Astrolog itself.
func TestSingleBodySelectionHasNoAspects(t *testing.T) {
	if _, err := exec.LookPath("astrolog"); err != nil {
		t.Skip("astrolog not on PATH; skipping live singleton chart test")
	}
	for _, tc := range []struct {
		name   string
		number int
	}{
		{name: "typed Sun", number: 1},
		{name: "registry-only Ceres", number: 12},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lens := lookup.Hellenistic
			lens.Objects = recipe.ObjectPolicy{Baseline: recipe.Explicit, Selection: []int{tc.number}}
			result, err := lookup.FirstCallTransits(
				context.Background(), nil,
				time.Date(1985, 6, 15, 12, 0, 0, 0, time.UTC),
				location.PlaceInfo{Name: "Berlin", Lat: 52.52, Lon: 13.405, TZ: "Europe/Berlin"},
				lens,
			)
			if err != nil {
				t.Fatalf("singleton object %d failed the frame: %v", tc.number, err)
			}
			if len(result.Aspects) != 0 {
				t.Fatalf("singleton object %d produced %d aspects: %+v", tc.number, len(result.Aspects), result.Aspects)
			}
			body := renderAspectsBody(chartViewState{aspects: result.Aspects}, TabAspects, 64, 6, Config{Accessible: true, Color: ColorNever})
			if strings.TrimSpace(body) != "" {
				t.Errorf("singleton object %d rendered an aspect row: %q", tc.number, body)
			}
		})
	}
}

// C15: an aspect kind Astrolog can select but Sextile cannot type is omitted at
// the parser boundary; the remaining typed rows still render a healthy frame.
func TestAspectsOmitUnnameableKinds(t *testing.T) {
	baseline, _ := fixtureLookup(t, lookup.Hellenistic, nil)
	lens := lookup.Hellenistic
	lens.Aspects = recipe.AspectPolicy{Explicit: true, Indices: []int{1, 13}}
	changed, spawns := fixtureLookup(t, lens, func(in string) string {
		return strings.ReplaceAll(in, "Con ", "Sep ")
	})
	requireFirstSpawnBand(t, spawns, "-RA0", []int{1, 13})
	if len(changed.Aspects) == 0 || len(changed.Aspects) >= len(baseline.Aspects) {
		t.Fatalf("unknown-kind fixture produced %d typed rows from baseline %d; want only the unnameable rows omitted", len(changed.Aspects), len(baseline.Aspects))
	}
	if body := renderAspectsBody(chartViewState{aspects: changed.Aspects}, TabAspects, 64, 6, Config{Accessible: true, Color: ColorNever}); strings.TrimSpace(body) == "" {
		t.Fatal("omitting unknown kinds blanked the remaining typed aspect rows")
	}
}

// (Name kept: the specification C10a cites it. The behaviour now KEEPS the row.)
// C10a is deliberately separate from C15: an unknown endpoint and an unknown
// kind are resolved by different tables and can regress independently. An
// unknown KIND is still omitted (C15); an unknown ENDPOINT is no longer: the
// operator's 2026-10-05 report ("it has to work anyway, even with a
// placeholder") reversed the old omission, so the row is kept, carries
// Astrolog's printed name, and draws the placeholder.
func TestAspectsKeepUnnameableEndpoints(t *testing.T) {
	baseline, _ := fixtureLookup(t, lookup.Hellenistic, nil)
	lens := lookup.Hellenistic
	lens.Objects = recipe.ObjectPolicy{Baseline: recipe.Explicit, Selection: []int{1, 12}}
	changed, spawns := fixtureLookup(t, lens, func(in string) string {
		return strings.ReplaceAll(in, "Mercury", "Ceres  ")
	})
	requireFirstSpawnBand(t, spawns, "-R0", []int{1, 12})
	if len(changed.Aspects) != len(baseline.Aspects) {
		t.Fatalf("unknown-endpoint fixture produced %d rows from baseline %d; want every row kept", len(changed.Aspects), len(baseline.Aspects))
	}
	labelled := 0
	for _, a := range changed.Aspects {
		if a.A == aspect.BodyUnknown {
			if a.ALabel != "Ceres" {
				t.Errorf("untyped side of %+v lacks the printed name Ceres", a)
			}
			labelled++
		}
	}
	if labelled == 0 {
		t.Fatal("no aspect kept the untyped Ceres side")
	}
	body := renderAspectsBody(chartViewState{aspects: changed.Aspects}, TabAspects, 64, 6, Config{Accessible: true, Color: ColorNever})
	if strings.TrimSpace(body) == "" || strings.Contains(body, "?") || !strings.Contains(body, "·") {
		t.Fatalf("kept untyped endpoints must draw a placeholder and no question mark: %q", body)
	}
}

// stripSpawnPrefix splits a recorded spawn argv into the call's own argv and
// what stood before it: `-YQ 0`, then the hook-clear band, a run of `-~<hook>`
// `""` pairs (the specification C1). internal/ui may not import
// internal/astrolog, so the exact band is pinned there
// (TestEveryCallClearsHooks); this helper holds only its shape and that a band
// is present.
func stripSpawnPrefix(argv []string) (own, prefix []string, ok bool) {
	if len(argv) < 2 || argv[0] != "-YQ" || argv[1] != "0" {
		return nil, nil, false
	}
	i := 2
	for i+1 < len(argv) && strings.HasPrefix(argv[i], "-~") && argv[i+1] == "" {
		i += 2
	}
	if i == 2 {
		return nil, nil, false
	}
	return argv[i:], argv[:i], true
}
