package astrolog

import (
	"strings"
	"testing"

	"github.com/cruiserxyz/sextile/internal/recipe"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// highlightRender builds the production wheel styling with a highlight.
func highlightRender(selected []aspect.Body, focused aspect.Body) recipe.RenderSpec {
	r := recipe.DefaultWheelRender()
	r.Highlight = recipe.NewHighlight(selected, focused)
	return r
}

// showList extracts the object numbers following an argv's -R0, stopping at
// the next switch token. ok is false when the argv carries no -R0.
func showList(argv []string) (nums []string, ok bool) {
	for i, tok := range argv {
		if tok != "-R0" {
			continue
		}
		for _, t := range argv[i+1:] {
			if strings.HasPrefix(t, "-") {
				break
			}
			nums = append(nums, t)
		}
		return nums, true
	}
	return nil, false
}

// C1 — no highlight compiles to the single plain
// wheel, byte-identical to a zero-value highlight.
func TestCompileWheelNoHighlightEmitsNoYkO(t *testing.T) {
	plain := mustCompile(t, hellenisticSpec(t), recipe.WheelImage{Render: recipe.DefaultWheelRender()})
	zeroHl := mustCompile(t, hellenisticSpec(t), recipe.WheelImage{Render: highlightRender(nil, aspect.BodyUnknown)})
	for _, plan := range []ExecutionPlan{plain, zeroHl} {
		if len(plan.Invocations) != 1 {
			t.Fatalf("inactive highlight must compile to one invocation, got %d", len(plan.Invocations))
		}
		for _, tok := range plan.Invocations[0].Argv {
			if tok == "-YkO" || tok == "-R0" {
				t.Errorf("inactive highlight leaked %q into %v", tok, plan.Invocations[0].Argv)
			}
		}
	}
	if a, b := strings.Join(plain.Invocations[0].Argv, " "), strings.Join(zeroHl.Invocations[0].Argv, " "); a != b {
		t.Errorf("zero highlight must compile identically to no highlight:\n plain: %s\n zero:  %s", a, b)
	}
}

// M9 (amended: ≥1 drawn member) — an active highlight compiles to exactly
// three WheelImage invocations in the order base, foreground, furniture.
func TestCompileWheelHighlightEmitsThreeInvocations(t *testing.T) {
	plan := mustCompile(t, hellenisticSpec(t), recipe.WheelImage{Render: highlightRender([]aspect.Body{aspect.Venus}, aspect.Moon)})
	if len(plan.Invocations) != 3 {
		t.Fatalf("want 3 invocations (base, foreground, furniture), got %d", len(plan.Invocations))
	}
	base, fore, furn := plan.Invocations[0], plan.Invocations[1], plan.Invocations[2]
	if _, ok := showList(base.Argv); ok {
		t.Errorf("base layer must not carry -R0: %v", base.Argv)
	}
	if nums, ok := showList(fore.Argv); !ok || len(nums) == 0 {
		t.Errorf("foreground layer must carry a populated -R0 show-list: %v", fore.Argv)
	}
	if nums, ok := showList(furn.Argv); !ok || len(nums) != 0 {
		t.Errorf("furniture layer must carry a bare -R0: %v", furn.Argv)
	}
	for _, inv := range plan.Invocations {
		if inv.Artifact != ImageFile || inv.Parser != ParseNone {
			t.Errorf("every layer is an image invocation: %+v", inv)
		}
	}
}

// an earlier change — the base layer is the plain wheel MINUS the highlight members: one
// trailing -R flip per member in ascending object order, so the dimmed
// context cannot carry a shadow twin of a body the mask draws. Dimming
// itself stays compose's pixel job — no color switch appears.
func TestBaseLayerHidesHighlightMembers(t *testing.T) {
	plain := mustCompile(t, hellenisticSpec(t), recipe.WheelImage{Render: recipe.DefaultWheelRender()})
	hl := mustCompile(t, hellenisticSpec(t), recipe.WheelImage{Render: highlightRender([]aspect.Body{aspect.Sun, aspect.Mars}, aspect.BodyUnknown)})
	want := strings.Join(plain.Invocations[0].Argv, " ") + " -R 1 -R 5"
	got := strings.Join(hl.Invocations[0].Argv, " ")
	if got != want {
		t.Errorf("base layer must be the plain wheel plus member -R flips:\n want: %s\n got:  %s", want, got)
	}
}

// M10 — the foreground show-list is the highlight members' object numbers in
// ascending order: selected ∪ focused, Venus(4) + Moon(2) + Saturn(7) → 2 4 7.
func TestForegroundInvocationShowList(t *testing.T) {
	plan := mustCompile(t, hellenisticSpec(t), recipe.WheelImage{Render: highlightRender([]aspect.Body{aspect.Venus, aspect.Saturn}, aspect.Moon)})
	nums, ok := showList(plan.Invocations[1].Argv)
	if !ok {
		t.Fatalf("foreground carries no -R0: %v", plan.Invocations[1].Argv)
	}
	if got, want := strings.Join(nums, " "), "2 4 7"; got != want {
		t.Errorf("show-list = %q, want %q (ascending object numbers, selected ∪ focused)", got, want)
	}
}

// No layer carries recolor switches: the operator dropped the cursor tint
// (2026-08-23), so the focused body is a plain foreground member and every
// member keeps Astrolog's own colors.
func TestNoLayerCarriesRecolorSwitches(t *testing.T) {
	plan := mustCompile(t, hellenisticSpec(t), recipe.WheelImage{Render: highlightRender([]aspect.Body{aspect.Venus}, aspect.Moon)})
	for i, inv := range plan.Invocations {
		if argv := strings.Join(inv.Argv, " "); strings.Contains(argv, "-YkO") {
			t.Errorf("layer %d carries a recolor switch: %s", i, argv)
		}
	}
}

// M12 (amended) — a highlight whose every member is undrawn or unnumbered
// compiles to the single plain invocation.
func TestSingleSelectionEmitsOneInvocation(t *testing.T) {
	spec := siderealHidePlutoSpec(t)
	// Pluto is hidden by the lens; Body(99) has no astrolog number.
	plan := mustCompile(t, spec, recipe.WheelImage{Render: highlightRender([]aspect.Body{aspect.Pluto, aspect.Body(99)}, aspect.BodyUnknown)})
	if len(plan.Invocations) != 1 {
		t.Fatalf("no drawable member → single invocation, got %d", len(plan.Invocations))
	}
	plain := mustCompile(t, spec, recipe.WheelImage{Render: recipe.DefaultWheelRender()})
	if a, b := strings.Join(plan.Invocations[0].Argv, " "), strings.Join(plain.Invocations[0].Argv, " "); a != b {
		t.Errorf("undrawable highlight must compile to the plain wheel:\n got:   %s\n plain: %s", a, b)
	}
	// One drawable member IS enough for the composite: the glyph over the
	// dimmed base shares the visual language of the growing web.
	one := mustCompile(t, spec, recipe.WheelImage{Render: highlightRender([]aspect.Body{aspect.Sun}, aspect.BodyUnknown)})
	if len(one.Invocations) != 3 {
		t.Errorf("one drawn member → composite plan, got %d invocations", len(one.Invocations))
	}
}

// M19 (amended for the focused tint) — the furniture argv equals the
// foreground argv except its -R0 carries no object numbers.
func TestFurnitureArgvMatchesForegroundExceptShowList(t *testing.T) {
	plan := mustCompile(t, hellenisticSpec(t), recipe.WheelImage{Render: highlightRender([]aspect.Body{aspect.Venus, aspect.Mars}, aspect.Moon)})
	fore, furn := plan.Invocations[1].Argv, plan.Invocations[2].Argv
	nums, _ := showList(fore)
	// Removing exactly the show-list numbers from fore must yield furn.
	var trimmed []string
	skip := len(nums)
	for i := 0; i < len(fore); i++ {
		if fore[i] == "-R0" {
			trimmed = append(trimmed, fore[i])
			i += skip
			continue
		}
		trimmed = append(trimmed, fore[i])
	}
	if strings.Join(trimmed, " ") != strings.Join(furn, " ") {
		t.Errorf("furniture argv must be foreground minus show-list numbers:\n fore: %v\n furn: %v", fore, furn)
	}
}

// F3 retargeted — a selected body the lens does not draw is omitted from the
// show-list and never unrestricted into it.
func TestForegroundOmitsInvisibleSelection(t *testing.T) {
	spec := siderealHidePlutoSpec(t)
	plan := mustCompile(t, spec, recipe.WheelImage{Render: highlightRender([]aspect.Body{aspect.Sun, aspect.Pluto}, aspect.BodyUnknown)})
	if len(plan.Invocations) != 3 {
		t.Fatalf("Sun is drawable → composite plan, got %d invocations", len(plan.Invocations))
	}
	nums, _ := showList(plan.Invocations[1].Argv)
	if got, want := strings.Join(nums, " "), "1"; got != want {
		t.Errorf("show-list = %q, want %q (hidden Pluto omitted)", got, want)
	}
}

// F7/F8 retargeted — an unmapped or invisible focused body contributes no
// member and no tint.
func TestForegroundOmitsUndrawableFocusedBody(t *testing.T) {
	spec := siderealHidePlutoSpec(t)
	for _, focused := range []aspect.Body{aspect.Body(99), aspect.Pluto} {
		plan := mustCompile(t, spec, recipe.WheelImage{Render: highlightRender([]aspect.Body{aspect.Sun}, focused)})
		if len(plan.Invocations) != 3 {
			t.Fatalf("focused %v: Sun still drawable → composite plan, got %d", focused, len(plan.Invocations))
		}
		fore := strings.Join(plan.Invocations[1].Argv, " ")
		if strings.Contains(fore, "-YkO") {
			t.Errorf("focused %v is undrawable, no tint may be emitted: %s", focused, fore)
		}
		nums, _ := showList(plan.Invocations[1].Argv)
		if got := strings.Join(nums, " "); got != "1" {
			t.Errorf("focused %v: show-list = %q, want \"1\"", focused, got)
		}
	}
}

// C7 — text projections carry neither -YkO nor -R0.
func TestCompileTextProjectionsCarryNoYkO(t *testing.T) {
	spec := hellenisticSpec(t)
	for _, p := range []recipe.Projection{
		recipe.Positions{}, recipe.Aspects{}, recipe.ChartRawReport(),
		recipe.BodyRawReport(aspect.Moon), recipe.AspectsRawReport(),
	} {
		plan := mustCompile(t, spec, p)
		for _, tok := range plan.Invocations[0].Argv {
			if tok == "-YkO" || tok == "-R0" {
				t.Errorf("%s: text projection carries %q: %v", p.Label(), tok, plan.Invocations[0].Argv)
			}
		}
	}
}

// Comparison sources compile to the single plain bi-wheel with no highlight
// switches, whatever the render spec carries.
func TestCompileComparisonWheelSuppressesHighlight(t *testing.T) {
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
	plan := mustCompile(t, spec, recipe.WheelImage{Render: highlightRender([]aspect.Body{aspect.Sun}, aspect.Moon)})
	if len(plan.Invocations) != 1 {
		t.Fatalf("comparison must compile to one invocation, got %d", len(plan.Invocations))
	}
	for _, tok := range plan.Invocations[0].Argv {
		if tok == "-YkO" || tok == "-R0" {
			t.Errorf("comparison wheel carries %q: %v", tok, plan.Invocations[0].Argv)
		}
	}
}

// siderealHidePlutoSpec mirrors the invariants' sidereal case: astrolog
// default set with Pluto removed.
func siderealHidePlutoSpec(t *testing.T) recipe.ChartSpec {
	t.Helper()
	spec, err := recipe.NewChartSpec(
		recipe.MomentAtPlace{JD: 2451545.0, Lon: -0.1278, Lat: 51.5074},
		recipe.Placidus, recipe.Sidereal,
		recipe.ObjectPolicy{Baseline: recipe.AstrologDefault, Remove: recipe.NewBodySet(aspect.Pluto)},
	)
	if err != nil {
		t.Fatalf("sidereal spec: %v", err)
	}
	return spec
}
