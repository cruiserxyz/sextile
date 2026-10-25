package astrolog

import (
	"os/exec"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/cruiserxyz/sextile/internal/recipe"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// C1a of the specification: when an Explicit policy and the earlier change
// highlight composite are both in force, each of the three layers still renders
// the layer it names.
//
// The spec marked this "expected, not verified" — Astrolog is last-wins and
// `-R` terminates the preceding list, so the composite was expected to compose.
// Two tests here: the first pins the argv, the second actually asks the binary,
// which is what turns the expectation into a verification. The failure mode
// being guarded is a highlight that silently shows the WRONG bodies once the
// operator picks a set — silently, because an image renders either way.

// explicitHighlightSpec builds the shared case: four objects chosen, two of
// them highlighted.
func explicitHighlightSpec(t *testing.T) recipe.ChartSpec {
	t.Helper()
	spec, err := recipe.NewChartSpec(
		recipe.MomentAtPlace{JD: 2446232.0, Lon: 11.5667, Lat: 48.1333},
		recipe.WholeSign, recipe.Tropical,
		// Sun 1, Moon 2, Venus 4, Pluto 10.
		recipe.ObjectPolicy{Baseline: recipe.Explicit, Selection: []int{1, 2, 4, 10}},
	)
	if err != nil {
		t.Fatalf("spec: %v", err)
	}
	return spec
}

func TestExplicitComposesWithHighlight(t *testing.T) {
	spec := explicitHighlightSpec(t)
	render := recipe.DefaultWheelRender()
	render.Highlight = recipe.NewHighlight([]aspect.Body{aspect.Moon, aspect.Venus}, aspect.BodyUnknown)

	plan, err := Compile(spec, recipe.WheelImage{Render: render})
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if len(plan.Invocations) != 3 {
		t.Fatalf("got %d invocations, want the three-layer composite", len(plan.Invocations))
	}

	// Every layer carries the chosen set as its object band; the layer tails
	// differ and are what make each layer its own.
	const band = "-R0 1 2 4 10"
	wantTails := []string{
		"-R 2 -R 4", // base: the plain wheel MINUS the highlight members
		"-R0 2 4",   // foreground: a show-list of exactly the members
		"-R0",       // furniture: the same render with an empty show-list
	}
	for i, inv := range plan.Invocations {
		got := strings.Join(inv.Argv, " ")
		if !strings.Contains(got, band) {
			t.Errorf("layer%d argv = %q, want the chosen set %q as its object band", i, got, band)
		}
		if !strings.HasSuffix(got, wantTails[i]) {
			t.Errorf("layer%d argv = %q, want it to end with %q", i, got, wantTails[i])
		}
		if strings.Index(got, band) > strings.LastIndex(got, wantTails[i]) {
			t.Errorf("layer%d: the object band must PRECEDE the layer tail, or last-wins drops the chosen set: %q", i, got)
		}
	}
}

// TestExplicitHighlightMembersAreDrawnOnly: the highlight names bodies, the
// selection names numbers, and a body the operator did not choose is not on the
// wheel to highlight. Mercury (3) is absent from the selection, so highlighting
// it must contribute nothing — otherwise the foreground layer draws a body the
// base layer never dimmed.
func TestExplicitHighlightMembersAreDrawnOnly(t *testing.T) {
	spec := explicitHighlightSpec(t)
	render := recipe.DefaultWheelRender()
	render.Highlight = recipe.NewHighlight([]aspect.Body{aspect.Moon, aspect.Mercury}, aspect.BodyUnknown)

	plan, err := Compile(spec, recipe.WheelImage{Render: render})
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if len(plan.Invocations) != 3 {
		t.Fatalf("got %d invocations, want the three-layer composite", len(plan.Invocations))
	}
	got := strings.Join(plan.Invocations[1].Argv, " ")
	if !strings.HasSuffix(got, "-R0 2") {
		t.Errorf("foreground argv = %q, want a show-list of Moon alone — Mercury is not in the chosen set", got)
	}
}

// TestExplicitHighlightOfNothingDrawnIsPlain: if every highlighted body is
// outside the chosen set there is nothing to composite, and the plan collapses
// to the single plain wheel exactly as it does for an inactive highlight
// .
func TestExplicitHighlightOfNothingDrawnIsPlain(t *testing.T) {
	spec := explicitHighlightSpec(t)
	render := recipe.DefaultWheelRender()
	render.Highlight = recipe.NewHighlight([]aspect.Body{aspect.Mercury}, aspect.BodyUnknown)

	plan, err := Compile(spec, recipe.WheelImage{Render: render})
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if len(plan.Invocations) != 1 {
		t.Fatalf("got %d invocations, want the single plain wheel", len(plan.Invocations))
	}
}

// TestExplicitCompositeAgainstLiveAstrolog is the verification the spec said it
// did not have. It asks the binary what each layer's restriction band actually
// resolves to, using the positions projection as the probe — the image layers
// cannot be read back, but they carry the same band, and the band is the whole
// claim. Skipped when astrolog is not on PATH so the suite stays hermetic.
func TestExplicitCompositeAgainstLiveAstrolog(t *testing.T) {
	if _, err := exec.LookPath("astrolog"); err != nil {
		t.Skip("astrolog not on PATH; skipping live-binary composite test")
	}

	base := []string{"-qj", "2446232.000000", "-zl", "11:34E", "48:08N", "-c", "14"}
	cases := []struct {
		name string
		tail []string
		want []string // body row prefixes, in astrolog's own order
	}{
		{"chosen set alone", []string{"-R0", "1", "2", "4", "10"}, []string{"Sun", "Moon", "Venu", "Plut"}},
		{"base layer", []string{"-R0", "1", "2", "4", "10", "-R", "2", "-R", "4"}, []string{"Sun", "Plut"}},
		{"foreground layer", []string{"-R0", "1", "2", "4", "10", "-R0", "2", "4"}, []string{"Moon", "Venu"}},
		{"furniture layer", []string{"-R0", "1", "2", "4", "10", "-R0"}, nil},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			argv := append(append([]string{}, base...), tc.tail...)
			argv = append(argv, "-v0")
			out, err := exec.Command("astrolog", argv...).Output()
			if err != nil {
				t.Fatalf("astrolog %v: %v", argv, err)
			}
			got := bodyRowNames(string(out))
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("astrolog %v\ndrew %v\nwant %v", argv, got, tc.want)
			}
		})
	}
}

// bodyRowNames pulls the four-character body labels out of a `-v0` listing.
// A body row is `Name: <position>`; the house-cusp column that shares the line
// is introduced by a space, so splitting on the colon and taking the first
// field is enough to tell them apart.
func bodyRowNames(out string) []string {
	var names []string
	for _, line := range strings.Split(out, "\n") {
		if len(line) < 5 || line[4] != ':' {
			continue
		}
		name := strings.TrimSpace(line[:4])
		if name == "" || strings.ContainsAny(name, "0123456789") {
			continue
		}
		if _, err := strconv.Atoi(name); err == nil {
			continue
		}
		names = append(names, name)
	}
	return names
}
