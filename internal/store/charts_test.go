package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cruiserxyz/sextile/internal/env"
	"github.com/cruiserxyz/sextile/internal/location"
)

// All chart values in this file are constructed synthetic test inputs.

// withDevTier swaps the tier-3 decision for the test and restores it after.
func withDevTier(t *testing.T, on bool) {
	t.Helper()
	old := devTier
	devTier = func() bool { return on }
	t.Cleanup(func() { devTier = old })
}

func TestChartsDir_EnvPrecedence(t *testing.T) {
	setup := func(t *testing.T) {
		t.Setenv("HOME", "/home/u")
		t.Setenv("XDG_DATA_HOME", "")
		t.Setenv("SEXTILE_CHARTS_DIR", "")
		t.Setenv("SEXTILE_DEV_CHARTS_DIR", "")

		if got, want := ChartsDir(), filepath.FromSlash("/home/u/.local/share/sextile/charts"); got != want {
			t.Errorf("default: ChartsDir() = %q, want %q", got, want)
		}

		t.Setenv("XDG_DATA_HOME", "/xdg")
		if got, want := ChartsDir(), filepath.FromSlash("/xdg/sextile/charts"); got != want {
			t.Errorf("xdg default: ChartsDir() = %q, want %q", got, want)
		}
		t.Setenv("SEXTILE_DEV_CHARTS_DIR", "/dev/charts")
	}

	t.Run("dev build", func(t *testing.T) {
		withDevTier(t, true)
		setup(t)
		// Dev fixtures win over a missing default tier...
		if got := ChartsDir(); got != "/dev/charts" {
			t.Errorf("dev override should win when default is missing: ChartsDir() = %q", got)
		}
		// ...but the live operator dir wins over dev.
		t.Setenv("SEXTILE_CHARTS_DIR", "/live/charts")
		if got := ChartsDir(); got != "/live/charts" {
			t.Errorf("live override should win over dev: ChartsDir() = %q", got)
		}
		// An EXISTING XDG library (tier 2) beats the dev fixtures (tier 3).
		xdg := t.TempDir()
		existing := filepath.Join(xdg, "sextile", "charts")
		if err := os.MkdirAll(existing, 0o755); err != nil {
			t.Fatal(err)
		}
		t.Setenv("SEXTILE_CHARTS_DIR", "")
		t.Setenv("XDG_DATA_HOME", xdg)
		if got := ChartsDir(); got != existing {
			t.Errorf("an existing XDG library must beat the dev tier: ChartsDir() = %q, want %q", got, existing)
		}
	})

	t.Run("release build", func(t *testing.T) {
		withDevTier(t, false)
		setup(t)
		// the specification C5: the dev tier is ignored.
		if got, want := ChartsDir(), filepath.FromSlash("/xdg/sextile/charts"); got != want {
			t.Errorf("a release build must ignore SEXTILE_DEV_CHARTS_DIR: ChartsDir() = %q, want %q", got, want)
		}
		t.Setenv("SEXTILE_CHARTS_DIR", "/live/charts")
		if got := ChartsDir(); got != "/live/charts" {
			t.Errorf("live override must still win: ChartsDir() = %q", got)
		}
	})
}

func TestChartsDir_ExistingDefaultBeatsDevFixtures(t *testing.T) {
	withDevTier(t, true) // with the tier off this would pass for the wrong reason
	xdg := t.TempDir()
	defaultDir := filepath.Join(xdg, "sextile", "charts")
	if err := os.MkdirAll(defaultDir, 0o755); err != nil {
		t.Fatalf("mkdir default chart dir: %v", err)
	}
	t.Setenv("HOME", filepath.Join(t.TempDir(), "home"))
	t.Setenv("XDG_DATA_HOME", xdg)
	t.Setenv("SEXTILE_CHARTS_DIR", "")
	t.Setenv("SEXTILE_DEV_CHARTS_DIR", "/dev/charts")

	if got := ChartsDir(); got != defaultDir {
		t.Errorf("existing default should beat dev fixtures: ChartsDir() = %q, want %q", got, defaultDir)
	}
}

// A relative chart dir (the shape the .env uses, SEXTILE_DEV_CHARTS_DIR=
// ./test_charts) resolves against the repo root, not CWD, so the binary and
// `go test` (CWD = this package dir) agree on the absolute target.
func TestChartsDir_RelativeResolvesAgainstRepoRoot(t *testing.T) {
	setup := func(t *testing.T) {
		// Point the default tier at an empty scratch home: on a host with a real
		// library at the XDG default, the dev tier under test would be shadowed.
		t.Setenv("HOME", filepath.Join(t.TempDir(), "home"))
		t.Setenv("XDG_DATA_HOME", "")
		t.Setenv("SEXTILE_CHARTS_DIR", "")
		t.Setenv("SEXTILE_DEV_CHARTS_DIR", "./test_charts")
	}

	t.Run("dev build", func(t *testing.T) {
		withDevTier(t, true)
		setup(t)
		got := ChartsDir()
		// The tier is honoured and goes through env.ResolveDir. In an untagged
		// test binary the repo root is unknown, so ResolveDir leaves the value
		// as given; in a -tags dev build it resolves against go.mod's directory.
		// This subtest proves the routing; the resolution itself is proved on a
		// built dev probe (internal/env TestReleaseResolveDirKeepsRelative).
		if want := env.ResolveDir("./test_charts"); got != want {
			t.Fatalf("ChartsDir() = %q, want env.ResolveDir(%q) = %q", got, "./test_charts", want)
		}
		if env.Dev() {
			if !filepath.IsAbs(got) {
				t.Fatalf("ChartsDir() = %q is not absolute (relative value was not resolved)", got)
			}
			if filepath.Base(got) != "test_charts" {
				t.Errorf("ChartsDir() = %q, want a path ending in test_charts", got)
			}
			if fi, err := os.Stat(filepath.Join(filepath.Dir(got), "go.mod")); err != nil || fi.IsDir() {
				t.Errorf("ChartsDir() = %q did not resolve against the repo root (no go.mod alongside)", got)
			}
		}
	})

	t.Run("release build", func(t *testing.T) {
		withDevTier(t, false)
		setup(t)
		if got, want := ChartsDir(), defaultChartsDir(); got != want {
			t.Errorf("a release build must ignore SEXTILE_DEV_CHARTS_DIR: ChartsDir() = %q, want %q", got, want)
		}
	})
}

// The an earlier change regression: an existing operator library at the XDG default must
// never capture chart writes from a test that isolated via useChartsDir.
func TestUseChartsDir_BeatsExistingOperatorLibrary(t *testing.T) {
	xdg := t.TempDir()
	operatorLib := filepath.Join(xdg, "sextile", "charts")
	if err := os.MkdirAll(operatorLib, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_DATA_HOME", xdg)
	dir := t.TempDir()
	useChartsDir(t, dir)

	if got := ChartsDir(); got != dir {
		t.Fatalf("ChartsDir() = %q, want isolated %q", got, dir)
	}
	writeChart(t, dir, "isolated.as", "@AI800  ; Astrolog chart info.\n")
	if entries, _ := os.ReadDir(operatorLib); len(entries) != 0 {
		t.Errorf("operator library holds %d file(s); the test dir must be the only one touched", len(entries))
	}
}

// C9: IsValid asks for a name and an IANA timezone, and does NOT ask for a
// moment — the row does not carry one.
func TestNamedChart_IsValidIgnoresMoment(t *testing.T) {
	full := NamedChart{Name: "X", Place: location.PlaceInfo{Name: "Y", TZ: "UTC"}}
	if !full.IsValid() {
		t.Error("a chart with a name and a timezone is valid without any moment")
	}
	if full.Year != 0 {
		t.Fatal("precondition: this chart carries no year at all")
	}
	if (NamedChart{Place: location.PlaceInfo{TZ: "UTC"}}).IsValid() {
		t.Error("a chart with no name must not be valid")
	}
	if (NamedChart{Name: "X", Place: location.PlaceInfo{Name: "Y"}}).IsValid() {
		t.Error("a chart with no IANA timezone must not be valid")
	}
}

func TestFilterCharts_ExactBeatsSubstringBeatsFuzzy(t *testing.T) {
	charts := []NamedChart{
		{Name: "Carla Juniper"},   // 0: substring "carl"  → tier 2
		{Name: "Carlo Sage"},      // 1: substring "carl"  → tier 2
		{Name: "Constructed Two"}, // 2: no 'l' after c-a-r → no match
		{Name: "Carl"},            // 3: exact              → tier 3
	}
	got, trunc := FilterCharts(charts, "carl", 0)
	if trunc {
		t.Error("unexpected truncation at cap=0")
	}
	assertNames(t, got, []string{"Carl", "Carla Juniper", "Carlo Sage"})
}

func TestFilterCharts_StartsWithBeatsContains(t *testing.T) {
	charts := []NamedChart{
		{Name: "Old Carl Reed"}, // contains
		{Name: "Carlton Reed"},  // starts with
		{Name: "Carl"},          // exact
		{Name: "Cxarxl"},        // loose
	}
	got, _ := FilterCharts(charts, "carl", 0)
	assertNames(t, got, []string{"Carl", "Carlton Reed", "Old Carl Reed", "Cxarxl"})
}

func TestFilterCharts_SubstringBeatsFuzzy(t *testing.T) {
	charts := []NamedChart{
		{Name: "Carla Juniper"}, // fuzzy c..l → tier 1
		{Name: "Clara Signal"},  // substring "cl" → tier 2
	}
	got, _ := FilterCharts(charts, "cl", 0)
	assertNames(t, got, []string{"Clara Signal", "Carla Juniper"})
}

func TestFilterCharts_TruncationAndEmpty(t *testing.T) {
	charts := []NamedChart{{Name: "Carla Juniper"}, {Name: "Carlo Sage"}, {Name: "Carlton Reed"}}
	got, trunc := FilterCharts(charts, "carl", 2)
	if !trunc || len(got) != 2 {
		t.Errorf("cap=2 → (2 results, truncated), got (%d, %v)", len(got), trunc)
	}

	if got, trunc := FilterCharts(charts, "", 0); got != nil || trunc {
		t.Errorf("empty query → (nil, false), got (%v, %v)", got, trunc)
	}
}

func TestSlugify(t *testing.T) {
	cases := map[string]string{
		"Constructed One":  "constructed-one",
		"A.B.C. Example":   "a-b-c-example",
		"  Spaces  ":       "spaces",
		"Carla Juniper!!!": "carla-juniper",
		"":                 "",
	}
	for in, want := range cases {
		if got := slugify(in); got != want {
			t.Errorf("slugify(%q) = %q, want %q", in, got, want)
		}
	}
}

// useChartsDir points ChartsDir at dir for the test via the top-precedence
// live tier. Isolating through SEXTILE_DEV_CHARTS_DIR is not enough: an
// existing operator library at the XDG default beats the dev tier, so
// dev-tier "isolation" would write test fixtures into real data.
func useChartsDir(t *testing.T, dir string) {
	t.Helper()
	t.Setenv("SEXTILE_CHARTS_DIR", dir)
	t.Setenv("SEXTILE_DEV_CHARTS_DIR", "")
}

// writeChart authors one library file as literal text. Sextile has no chart
// writer any more (C10), so fixtures state what is on disk instead of being
// round-tripped through the code under test.
func writeChart(t *testing.T, dir, file, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, file), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func names(charts []NamedChart) []string {
	out := make([]string, len(charts))
	for i, c := range charts {
		out[i] = c.Name
	}
	return out
}

func assertNames(t *testing.T, got []NamedChart, want []string) {
	t.Helper()
	g := names(got)
	if len(g) != len(want) {
		t.Fatalf("match count = %d (%v), want %d (%v)", len(g), g, len(want), want)
	}
	for i := range want {
		if g[i] != want[i] {
			t.Errorf("match[%d] = %q, want %q (full order: %v)", i, g[i], want[i], g)
		}
	}
}

func TestNamedChart_IncompleteReason(t *testing.T) {
	full := NamedChart{Name: "N", Place: location.PlaceInfo{TZ: "UTC"}}
	if got := full.IncompleteReason(); got != "" {
		t.Errorf("a complete chart should have no reason, got %q", got)
	}
	noName := full
	noName.Name = ""
	if got := noName.IncompleteReason(); got != "no name" {
		t.Errorf("missing name reason = %q, want \"no name\"", got)
	}
	noTZ := full
	noTZ.Place = location.PlaceInfo{Name: "Y"}
	if got := noTZ.IncompleteReason(); got != "no timezone" {
		t.Errorf("missing timezone reason = %q, want \"no timezone\"", got)
	}
	got := (NamedChart{}).IncompleteReason()
	for _, want := range []string{"name", "timezone"} {
		if !strings.Contains(got, want) {
			t.Errorf("an empty chart's reason %q does not mention %q", got, want)
		}
	}
	if !strings.Contains(got, " and ") {
		t.Errorf("two missing parts should read as a list, got %q", got)
	}
	// A chart with no moment is NOT incomplete: rows do not carry one (C9).
	if got := (NamedChart{Name: "N", Place: location.PlaceInfo{TZ: "UTC"}}).IncompleteReason(); got != "" {
		t.Errorf("a missing moment must not make a row incomplete, got %q", got)
	}
}

// C12: after a save, ChartsDir() resolves to the directory it resolved to
// before the save.
//
// This test lives here, in the one file TestTestsIsolateChartsDirOnTopTier
// exempts, and deliberately leaves SEXTILE_CHARTS_DIR UNSET: the precedence it
// guards is between the XDG default and the dev fixtures, and an explicit
// SEXTILE_CHARTS_DIR outranks both, so isolating this test the usual way would
// make the thing it checks unreachable.
//
// The hazard is quiet and specific. ChartsDir() prefers the XDG default when
// that directory EXISTS, so a save that created the XDG default -- rather than
// the directory the resolver actually returned -- would move every later
// lookup onto an empty directory, and the operator's configured chart library
// would simply stop listing. Nothing would report an error.
func TestSaveDoesNotChangeChartsDirPrecedence(t *testing.T) {
	withDevTier(t, true)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, ".local", "share"))
	t.Setenv("SEXTILE_CHARTS_DIR", "")
	dev := t.TempDir()
	t.Setenv("SEXTILE_DEV_CHARTS_DIR", dev)

	xdg := filepath.Join(home, ".local", "share", "sextile", "charts")
	if _, err := os.Stat(xdg); !os.IsNotExist(err) {
		t.Fatalf("precondition: the XDG default %s already exists", xdg)
	}
	before := ChartsDir()
	if before != dev {
		t.Fatalf("precondition: ChartsDir() = %q, want the dev fixtures %q", before, dev)
	}

	if _, err := PlaceChartFile(before, []byte("@AI800  ; Astrolog chart info.\n"), "test-person"); err != nil {
		t.Fatalf("PlaceChartFile: %v", err)
	}

	if after := ChartsDir(); after != before {
		t.Errorf("the save moved the chart library: ChartsDir() was %q and is now %q", before, after)
	}
	if _, err := os.Stat(xdg); err == nil {
		t.Errorf("the save created the XDG default %s, which outranks the configured library", xdg)
	}
}
