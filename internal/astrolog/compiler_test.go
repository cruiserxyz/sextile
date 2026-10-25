package astrolog

import (
	"reflect"
	"testing"

	"github.com/cruiserxyz/sextile/internal/recipe"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// hellenisticSpec mirrors internal/lookup's Hellenistic lens (Whole Sign,
// tropical, true node/Lilith, the five extra objects) at a fixed Munich
// moment-at-place, so the compiler's output can be checked for byte-parity with
// the legacy hand-assembler it superseded.
func hellenisticSpec(t *testing.T) recipe.ChartSpec {
	t.Helper()
	return hellenisticSpecAt(t, 2446232.0, 11.5667, 48.1333)
}

// hellenisticSpecAt is hellenisticSpec at a caller-chosen moment-at-place — the
// shared canonical spec builder for the live integration tests (run_test,
// image_test), which pin specific epochs/locations.
func hellenisticSpecAt(t *testing.T, jd, lon, lat float64) recipe.ChartSpec {
	t.Helper()
	spec, err := recipe.NewChartSpec(
		recipe.MomentAtPlace{JD: jd, Lon: lon, Lat: lat},
		recipe.WholeSign,
		recipe.Tropical,
		recipe.ObjectPolicy{
			Baseline:     recipe.AstrologDefault,
			Add:          recipe.NewBodySet(aspect.Chiron, aspect.SouthNode, aspect.Lilith, aspect.ImumCoeli, aspect.Descendant),
			TrueNodeMode: true,
		},
	)
	if err != nil {
		t.Fatalf("build hellenistic spec: %v", err)
	}
	return spec
}

func mustCompile(t *testing.T, spec recipe.ChartSpec, p recipe.Projection) ExecutionPlan {
	t.Helper()
	plan, err := Compile(spec, p)
	if err != nil {
		t.Fatalf("Compile(%s): %v", p.Label(), err)
	}
	if len(plan.Invocations) == 0 {
		t.Fatalf("Compile(%s): empty plan", p.Label())
	}
	return plan
}

func argv(plan ExecutionPlan) []string { return plan.Invocations[0].Argv }

// TestCompile_PositionsArgv pins the Hellenistic positions argv exactly. This
// began as the v0b cutover gate (Compile == legacy buildChartArgv); buildChartArgv
// is now retired, so the expected argv is hardcoded here as a permanent guard,
// independent of the invariants golden.
func TestCompile_PositionsArgv(t *testing.T) {
	got := argv(mustCompile(t, hellenisticSpec(t), recipe.Positions{}))
	want := []string{
		"-qj", "2446232.0000000005", "-zl", "11:34E", "48:08N", "-zi", "", "", "-c", "14", "_s", "_Ys", "_sr0", ":sz", "-qag", "10", "15", "1582", "12:00am", "0", "0:00E", "0:00N", "=Yt", "_Yd",
		"-Yn", "-R", "11", "-R", "17", "-R", "18", "-R", "25", "-R", "28", "=v0", "-Yw", "0",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Positions argv\n got: %v\nwant: %v", got, want)
	}
}

// TestCompile_PositionsArgv_Bare pins the bare case: sidereal, Placidus, no true
// node, no restrictions.
func TestCompile_PositionsArgv_Bare(t *testing.T) {
	spec, err := recipe.NewChartSpec(
		recipe.MomentAtPlace{JD: 2446232.0, Lon: 0, Lat: 0},
		recipe.Placidus, recipe.Sidereal,
		recipe.ObjectPolicy{Baseline: recipe.AstrologDefault},
	)
	if err != nil {
		t.Fatalf("spec: %v", err)
	}
	got := argv(mustCompile(t, spec, recipe.Positions{}))
	want := []string{"-qj", "2446232.0000000005", "-zl", "0:00E", "0:00N", "-zi", "", "", "-c", "0", "=s", "0", "_Ys", "_sr0", ":sz", "-qag", "10", "15", "1582", "12:00am", "0", "0:00E", "0:00N", "=Yt", "_Yd", "=v0", "-Yw", "0"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("bare Positions argv\n got: %v\nwant: %v", got, want)
	}
}

// TestCompile_AspectsSharesPrefixSwapsMode: Aspects is the same prefix as
// Positions with -aa in place of the `=v0 -Yw 0` tail (no redundant -v0, unlike the legacy
// Run-appends-"-a" path). -aa, not -a: the list carries Astrolog's own
// applying/separating markers.
func TestCompile_AspectsSharesPrefixSwapsMode(t *testing.T) {
	spec := hellenisticSpec(t)
	pos := argv(mustCompile(t, spec, recipe.Positions{}))
	asp := argv(mustCompile(t, spec, recipe.Aspects{}))
	tail := pos[len(pos)-3:]
	if !reflect.DeepEqual(tail, []string{"=v0", "-Yw", "0"}) {
		t.Fatalf("positions must end with =v0 -Yw 0, got %v", pos)
	}
	want := append(append([]string{}, pos[:len(pos)-3]...), "-aa")
	if !reflect.DeepEqual(asp, want) {
		t.Errorf("aspects argv\n got: %v\nwant: %v", asp, want)
	}
	if plan := mustCompile(t, spec, recipe.Aspects{}); plan.Invocations[0].Parser != ParseAspects {
		t.Errorf("aspects parser = %v, want ParseAspects", plan.Invocations[0].Parser)
	}
}

func TestCompile_RawReportArgv(t *testing.T) {
	spec := hellenisticSpec(t)
	cases := []struct {
		name string
		p    recipe.RawReport
		tail []string
	}{
		// `=j0`, not `-j`/`-j0`: the forced-on form is the whole point — see
		// influenceChartMode. A bare toggle here would make the report depend
		// on the host's astrolog.as.
		{"chart", recipe.ChartRawReport(), []string{"=j0"}},
		{"body", recipe.BodyRawReport(aspect.Moon), []string{"-aa", "-RO", "2"}},
		{"aspects", recipe.AspectsRawReport(), []string{"-aa"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			inv := mustCompile(t, spec, tc.p).Invocations[0]
			if inv.Artifact != TextStdout || inv.Parser != ParseRawReport {
				t.Fatalf("artifact/parser = %v/%v, want TextStdout/ParseRawReport", inv.Artifact, inv.Parser)
			}
			if !reflect.DeepEqual(inv.Argv[len(inv.Argv)-len(tc.tail):], tc.tail) {
				t.Errorf("argv tail = %v, want %v (full argv %v)", inv.Argv[len(inv.Argv)-len(tc.tail):], tc.tail, inv.Argv)
			}
			if contains(inv.Argv, "-k") {
				t.Errorf("raw report argv must remain uncolored: %v", inv.Argv)
			}
		})
	}
}

func TestCompile_RawReportComparisonCompatibility(t *testing.T) {
	spec, err := recipe.NewChartSpec(
		recipe.Comparison{
			Primary:   recipe.MomentAtPlace{JD: 2446232, Lon: 11.5667, Lat: 48.1333},
			Secondary: recipe.MomentAtPlace{JD: 2451545, Lon: -0.1278, Lat: 51.5074},
		},
		recipe.WholeSign, recipe.Tropical,
		recipe.ObjectPolicy{Baseline: recipe.AstrologDefault},
	)
	if err != nil {
		t.Fatalf("comparison spec: %v", err)
	}
	inv := mustCompile(t, spec, recipe.AspectsRawReport()).Invocations[0]
	if inv.Argv[0] != "-r0" || inv.Argv[len(inv.Argv)-1] != "-aa" || len(inv.TempInputs) != 2 {
		t.Errorf("comparison raw aspects did not compile to staged -r0 ... -aa: argv=%v temps=%d", inv.Argv, len(inv.TempInputs))
	}
	// a BODY raw report is well defined over a comparison — `-RO <n>`
	// restricts the relationship list to the rows where that object is one of
	// the two ends.
	body := mustCompile(t, spec, recipe.BodyRawReport(aspect.Moon)).Invocations[0]
	wantTail := []string{"-aa", "-RO", "2"}
	if body.Argv[0] != "-r0" || len(body.TempInputs) != 2 || !reflect.DeepEqual(body.Argv[len(body.Argv)-3:], wantTail) {
		t.Errorf("comparison raw body did not compile to staged -r0 ... -aa -RO 2: argv=%v temps=%d", body.Argv, len(body.TempInputs))
	}
	// A transit-to-natal pair lowers it to -rt like every other phase-bearing
	// text projection, so a/s follows the transiting body.
	transitSpec, err := recipe.NewChartSpec(
		recipe.Comparison{
			Primary:   recipe.MomentAtPlace{JD: 2446232, Lon: 11.5667, Lat: 48.1333},
			Secondary: recipe.MomentAtPlace{JD: 2451545, Lon: -0.1278, Lat: 51.5074},
			Relation:  recipe.TransitToNatal,
		},
		recipe.WholeSign, recipe.Tropical,
		recipe.ObjectPolicy{Baseline: recipe.AstrologDefault},
	)
	if err != nil {
		t.Fatalf("transit comparison spec: %v", err)
	}
	if got := mustCompile(t, transitSpec, recipe.BodyRawReport(aspect.Moon)).Invocations[0].Argv; got[0] != "-rt" {
		t.Errorf("transit comparison raw body source = %q, want -rt: %v", got[0], got)
	}
	// The chart report stays undefined over a pair: which chart's influences?
	if _, err := Compile(spec, recipe.ChartRawReport()); err == nil {
		t.Error("comparison must reject the chart raw report")
	}
}

// TestCompile_WheelImage: the image projection swaps the text mode for the -X
// graphics tail, carries Artifact=ImageFile + ParseNone, and never emits -Xo
// (the runner appends the output path).
func TestCompile_WheelImage(t *testing.T) {
	spec := hellenisticSpec(t)
	plan := mustCompile(t, spec, recipe.WheelImage{Render: recipe.DefaultWheelRender()})
	inv := plan.Invocations[0]
	if inv.Artifact != ImageFile {
		t.Errorf("artifact = %v, want ImageFile", inv.Artifact)
	}
	if inv.Parser != ParseNone {
		t.Errorf("parser = %v, want ParseNone", inv.Parser)
	}
	joined := inv.Argv
	for _, bad := range []string{"-v0", "-a", "-Xo"} {
		if contains(joined, bad) {
			t.Errorf("wheel image argv must not contain %q: %v", bad, joined)
		}
	}
	for _, want := range []string{"-XV", "-Xw", "1200", "-Xi", "-XA", "-Xr", "-Xu", "-Xs", "400", "-XS", "-Xt", "-XL"} {
		if !contains(joined, want) {
			t.Errorf("wheel image argv missing %q: %v", want, joined)
		}
	}
	if contains(joined, "-Xbp") {
		t.Errorf("wheel image argv still selects bitmap output: %v", joined)
	}
	// prefix shared with positions (everything up to the first -X switch)
	pos := argv(mustCompile(t, spec, recipe.Positions{}))
	prefix := pos[:len(pos)-3] // drop =v0 -Yw 0
	for i, tok := range prefix {
		if joined[i] != tok {
			t.Errorf("wheel image must share the chart prefix; token %d = %q, want %q", i, joined[i], tok)
		}
	}
}

func TestCompile_WheelImagePNG(t *testing.T) {
	render := recipe.DefaultWheelRender()
	render.Format = recipe.ImagePNG
	plan := mustCompile(t, hellenisticSpec(t), recipe.WheelImage{Render: render})
	argv := plan.Invocations[0].Argv
	if !contains(argv, "-Xbp") {
		t.Fatalf("PNG wheel image argv missing -Xbp: %v", argv)
	}
	if contains(argv, "-XV") {
		t.Fatalf("PNG wheel image argv still selects SVG output: %v", argv)
	}
}

// TestObjectPolicy_RestrictionAlgebra: callers express desired visibility as
// Add/Remove deltas; the compiler emits the canonical sorted -R toggles. Hiding
// a default body and showing a restricted one both compile to -R.
func TestObjectPolicy_RestrictionAlgebra(t *testing.T) {
	spec, err := recipe.NewChartSpec(
		recipe.MomentAtPlace{JD: 2446232.0, Lon: 0, Lat: 0},
		recipe.WholeSign, recipe.Tropical,
		recipe.ObjectPolicy{
			Baseline: recipe.AstrologDefault,
			Add:      recipe.NewBodySet(aspect.Chiron), // astrolog 11 (restricted by default)
			Remove:   recipe.NewBodySet(aspect.Pluto),  // astrolog 10 (default-visible)
		},
	)
	if err != nil {
		t.Fatalf("spec: %v", err)
	}
	got := argv(mustCompile(t, spec, recipe.Positions{}))
	// toggles sorted ascending by object number: Pluto=10, Chiron=11
	if !subseq(got, []string{"-R", "10", "-R", "11"}) {
		t.Errorf("expected '-R 10 -R 11' (Pluto then Chiron) in %v", got)
	}
}

// TestCompile_Determinism: Compile is pure — identical spec yields identical
// argv across repeated calls (no map-iteration nondeterminism).
func TestCompile_Determinism(t *testing.T) {
	spec := hellenisticSpec(t)
	first := argv(mustCompile(t, spec, recipe.Positions{}))
	for i := 0; i < 50; i++ {
		if got := argv(mustCompile(t, spec, recipe.Positions{})); !reflect.DeepEqual(got, first) {
			t.Fatalf("non-deterministic argv on run %d:\n got: %v\nfirst: %v", i, got, first)
		}
	}
}

// TestCompile_ValidationRejects: a zero-value spec, an out-of-range coordinate,
// and an unknown house are all rejected before any argv is emitted.
func TestCompile_ValidationRejects(t *testing.T) {
	if _, err := Compile(recipe.ChartSpec{}, recipe.Positions{}); err == nil {
		t.Error("zero-value ChartSpec must be rejected")
	}
	bad := recipe.ChartSpec{
		Source:  recipe.MomentAtPlace{JD: 2446232.0, Lon: 999, Lat: 0},
		Houses:  recipe.WholeSign,
		Zodiac:  recipe.Tropical,
		Objects: recipe.ObjectPolicy{Baseline: recipe.AstrologDefault},
	}
	if _, err := Compile(bad, recipe.Positions{}); err == nil {
		t.Error("out-of-range longitude must be rejected")
	}
}

// TestCompileBundle_PrefixSharing: all projections of one spec share an
// identical compiled prefix, and the fingerprint is stable.
func TestCompileBundle_PrefixSharing(t *testing.T) {
	spec := hellenisticSpec(t)
	bundle, err := CompileBundle(spec, recipe.Positions{}, recipe.Aspects{}, recipe.WheelImage{Render: recipe.DefaultWheelRender()})
	if err != nil {
		t.Fatalf("CompileBundle: %v", err)
	}
	if len(bundle.Plans) != 3 {
		t.Fatalf("want 3 plans, got %d", len(bundle.Plans))
	}
	if bundle.PrefixFingerprint == "" {
		t.Error("empty PrefixFingerprint")
	}
	for _, plan := range bundle.Plans {
		a := plan.Invocations[0].Argv
		if len(a) < len(bundle.NormalizedPrefix) {
			t.Fatalf("plan %s shorter than shared prefix", plan.Projection)
		}
		if !reflect.DeepEqual(a[:len(bundle.NormalizedPrefix)], bundle.NormalizedPrefix) {
			t.Errorf("plan %s does not start with the shared prefix\n got: %v\nprefix: %v", plan.Projection, a[:len(bundle.NormalizedPrefix)], bundle.NormalizedPrefix)
		}
	}
	// CompileBundle ≡ Compile per projection.
	for _, p := range []recipe.Projection{recipe.Positions{}, recipe.Aspects{}} {
		single := argv(mustCompile(t, spec, p))
		var found bool
		for _, plan := range bundle.Plans {
			if plan.Projection == p.Label() {
				found = true
				if !reflect.DeepEqual(plan.Invocations[0].Argv, single) {
					t.Errorf("bundle plan %s != Compile output", p.Label())
				}
			}
		}
		if !found {
			t.Errorf("bundle missing plan for %s", p.Label())
		}
	}
}

// TestBodyObjectNum_CoversAllKnownBodies guards the contract that a
// recipe-validated ObjectPolicy can always be compiled: every body recipe
// treats as real (non-"unknown" String) must have an astrolog object number
// here. If recipe accepts a body the compiler can't lower, the two have
// drifted.
func TestBodyObjectNum_CoversAllKnownBodies(t *testing.T) {
	for i := 1; i <= 255; i++ {
		b := aspect.Body(i)
		if b.String() == "unknown" {
			continue
		}
		if _, ok := bodyObjectNum[b]; !ok {
			t.Errorf("bodyObjectNum missing known body %s (%d) — recipe would accept it but the compiler can't lower it", b, i)
		}
	}
}

// TestCompile_RejectsZeroWheelRender: a degenerate WheelImage is rejected, not
// silently compiled to a bare image selector.
func TestCompile_RejectsZeroWheelRender(t *testing.T) {
	if _, err := Compile(hellenisticSpec(t), recipe.WheelImage{}); err == nil {
		t.Error("WheelImage with a zero render must be rejected")
	}
}

// TestCompile_RejectsNilProjection: a nil projection returns an error, not a
// panic — the nil check must precede p.Validate() (review round-4 finding).
func TestCompile_RejectsNilProjection(t *testing.T) {
	if _, err := Compile(hellenisticSpec(t), nil); err == nil {
		t.Error("Compile must reject a nil projection")
	}
	if _, err := CompileBundle(hellenisticSpec(t), recipe.Positions{}, nil); err == nil {
		t.Error("CompileBundle must reject a nil projection")
	}
}

func contains(ss []string, s string) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
}

// subseq reports whether want appears as a contiguous subsequence of got.
func subseq(got, want []string) bool {
	if len(want) == 0 {
		return true
	}
	for i := 0; i+len(want) <= len(got); i++ {
		match := true
		for j := range want {
			if got[i+j] != want[j] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

// TestCompile_TransitToNatalLowersTextToRt: a TransitToNatal
// comparison compiles its phase-bearing text projections (aspects list, raw
// aspects report) to -rt, so Astrolog zeroes the natal velocities before
// judging applying/separating, while the bi-wheel keeps -r0 (whose outer ring
// is not subject to Astrolog's transit restrictions). A Synastry comparison
// stays -r0 throughout.
func TestCompile_TransitToNatalLowersTextToRt(t *testing.T) {
	mk := func(rel recipe.Relation) recipe.ChartSpec {
		spec, err := recipe.NewChartSpec(
			recipe.Comparison{
				Primary:   recipe.MomentAtPlace{JD: 2446232, Lon: 11.5667, Lat: 48.1333},
				Secondary: recipe.MomentAtPlace{JD: 2451545, Lon: -0.1278, Lat: 51.5074},
				Relation:  rel,
			},
			recipe.WholeSign, recipe.Tropical,
			recipe.ObjectPolicy{Baseline: recipe.AstrologDefault},
		)
		if err != nil {
			t.Fatalf("comparison spec: %v", err)
		}
		return spec
	}
	transit := mk(recipe.TransitToNatal)
	for _, tc := range []struct {
		proj recipe.Projection
		want string
	}{
		{recipe.Aspects{}, "-rt"},
		{recipe.AspectsRawReport(), "-rt"},
		{recipe.WheelImage{Render: recipe.DefaultWheelRender()}, "-r0"},
	} {
		inv := mustCompile(t, transit, tc.proj).Invocations[0]
		if inv.Argv[0] != tc.want {
			t.Errorf("TransitToNatal %s: argv[0] = %q, want %q (%v)", tc.proj.Label(), inv.Argv[0], tc.want, inv.Argv)
		}
		if len(inv.TempInputs) != 2 {
			t.Errorf("TransitToNatal %s: %d temp inputs, want 2", tc.proj.Label(), len(inv.TempInputs))
		}
	}
	synastry := mk(recipe.Synastry)
	for _, proj := range []recipe.Projection{recipe.Aspects{}, recipe.AspectsRawReport(), recipe.WheelImage{Render: recipe.DefaultWheelRender()}} {
		if inv := mustCompile(t, synastry, proj).Invocations[0]; inv.Argv[0] != "-r0" {
			t.Errorf("Synastry %s: argv[0] = %q, want -r0", proj.Label(), inv.Argv[0])
		}
	}
	// The shared bundle prefix is relation-neutral (-r0): -rt is a projection
	// lowering over a copy, so the fingerprint and I8 still hold.
	bundle, err := CompileBundle(transit, recipe.Aspects{}, recipe.WheelImage{Render: recipe.DefaultWheelRender()})
	if err != nil {
		t.Fatal(err)
	}
	if bundle.NormalizedPrefix[0] != "-r0" {
		t.Errorf("bundle prefix[0] = %q, want -r0", bundle.NormalizedPrefix[0])
	}
	if bundle.Plans[0].Invocations[0].Argv[0] != "-rt" || bundle.Plans[1].Invocations[0].Argv[0] != "-r0" {
		t.Errorf("bundle plans = %v / %v, want -rt aspects and -r0 wheel", bundle.Plans[0].Invocations[0].Argv[0], bundle.Plans[1].Invocations[0].Argv[0])
	}
}

// TestValidate_RejectsUnknownRelation pins the closed Relation set.
func TestValidate_RejectsUnknownRelation(t *testing.T) {
	_, err := recipe.NewChartSpec(
		recipe.Comparison{
			Primary:   recipe.MomentAtPlace{JD: 2446232, Lon: 11.5667, Lat: 48.1333},
			Secondary: recipe.MomentAtPlace{JD: 2451545, Lon: -0.1278, Lat: 51.5074},
			Relation:  recipe.Relation(7),
		},
		recipe.WholeSign, recipe.Tropical,
		recipe.ObjectPolicy{Baseline: recipe.AstrologDefault},
	)
	if err == nil {
		t.Error("NewChartSpec accepted an unknown Relation")
	}
}

// TestPositionsPinsTheRatioAndStationColumn: every positions invocation pins
// `=v0` (the ratio ON — `-v0` is a toggle a host astrolog.as with `=v0` turns
// OFF) and `-Yw 0` (no station `S` in the R column, which the parser's row
// pattern would silently drop). the specification C13.
func TestPositionsPinsTheRatioAndStationColumn(t *testing.T) {
	for name, spec := range map[string]recipe.ChartSpec{"hellenistic": hellenisticSpec(t)} {
		got := argv(mustCompile(t, spec, recipe.Positions{}))
		n := len(got)
		if n < 3 || got[n-3] != "=v0" || got[n-2] != "-Yw" || got[n-1] != "0" {
			t.Errorf("%s: positions argv does not end =v0 -Yw 0: %v", name, got)
		}
		for _, a := range got {
			if a == "-v0" {
				t.Errorf("%s: positions argv still carries the toggle -v0: %v", name, got)
			}
		}
	}
}
