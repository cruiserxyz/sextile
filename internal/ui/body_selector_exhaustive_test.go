package ui

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/internal/recipe"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// body_selector_exhaustive_test.go is the guard for the whole selector rather
// than for a list of bodies someone remembered to name.
//
// The defect that motivated it: selecting the 11th cusp — or any of forty other
// objects — emptied the Bodies AND Aspects panels completely. The cause was not
// in the cusp. Astrolog has no Swiss Ephemeris file for objects like Eris,
// Hygiea and the planetary moons on this installation, so it renders them at
// `0Ari00`; checkPositionSanity counted `0Ari00` anywhere in stdout, and a
// whole-sign chart ALWAYS carries one in the house-cusp column beside the body
// rows. One unavailable object was therefore the second match, the frame was
// condemned as "ephemeris disabled", and every panel went blank.
//
// No per-body test would have found that, because the failing set is a property
// of the operator's Astrolog DATA, not of Sextile's code: which objects lack an
// ephemeris file differs per installation. So this asks the binary for its own
// list and walks all of it. If a future Astrolog ships more objects, or fewer,
// or a user installs the missing ephemerides, the test covers exactly what that
// machine can actually offer.
//
// Skipped without astrolog on PATH, like every other live-binary test here.

// exhaustiveLens is the shipped lens with an Explicit policy substituted, so
// each case is the real reference frame with one object added or alone.
func exhaustiveLens(selection []int) lookup.Lens {
	l := lookup.Hellenistic
	l.Objects = recipe.ObjectPolicy{
		Baseline:     recipe.Explicit,
		Selection:    selection,
		TrueNodeMode: l.Objects.TrueNodeMode,
	}
	return l
}

func exhaustiveRegistry(t *testing.T) []aspect.RegistryObject {
	t.Helper()
	reg, err := lookup.ObjectRegistry(context.Background())
	if err != nil {
		t.Fatalf("object registry: %v", err)
	}
	if len(reg) == 0 {
		t.Fatal("the registry is empty; there is nothing to guard")
	}
	return reg
}

// TestEverySelectableObjectKeepsTheUIIntact walks EVERY object the operator's
// binary offers in the selector and asserts that choosing it leaves a frame the
// UI can draw.
//
// Two shapes per object, because they fail differently: added to the lens's own
// set (what an operator actually does), and alone (the smallest chart the
// selector can produce, where there is no healthy body to dilute a bad one).
func TestEverySelectableObjectKeepsTheUIIntact(t *testing.T) {
	if _, err := exec.LookPath("astrolog"); err != nil {
		t.Skip("astrolog not on PATH; skipping live-binary exhaustive selector test")
	}
	reg := exhaustiveRegistry(t)
	lensDefault := lookup.Hellenistic.VisibleObjectNumbers()
	place := location.PlaceInfo{Name: "Berlin", Lat: 52.52, Lon: 13.405, TZ: "Europe/Berlin"}
	when := time.Date(1985, 6, 15, 12, 0, 0, 0, time.UTC)

	for _, obj := range reg {
		obj := obj
		for _, shape := range []struct {
			name      string
			selection []int
		}{
			{"with the lens", append(append([]int{}, lensDefault...), obj.Number)},
			{"alone", []int{obj.Number}},
		} {
			t.Run(shapeName(obj, shape.name), func(t *testing.T) {
				res, err := lookup.FirstCallTransits(context.Background(), nil, when, place, exhaustiveLens(shape.selection))
				if err != nil {
					t.Fatalf("selecting %d (%s) %s made the frame fail: %v", obj.Number, obj.Name, shape.name, err)
				}
				// An empty frame is allowed in exactly one case: every object
				// selected is one this Astrolog has no ephemeris file for, and
				// it SAID so. Silence with an empty panel is the failure; an
				// explained empty panel is the operator's own choice of a body
				// their data cannot supply.
				if len(res.Positions) == 0 {
					if len(res.Unavailable) == 0 {
						t.Fatalf("selecting %d (%s) %s produced no positions and named nothing unavailable — the panels would be blank with no reason given", obj.Number, obj.Name, shape.name)
					}
					return
				}

				// The panels are rendered, not merely computed: "no contents
				// at all" was a RENDER symptom, and a frame that parses and
				// then draws nothing is the same bug to the operator.
				state := chartViewState{positions: res.Positions, aspects: res.Aspects}
				bodies, _, _ := renderSingleBodies(res.Positions, res.Cusps, res.BodyStyles, nil, 0, 0, 64, 20, false, Config{Accessible: true, Color: ColorNever}, houseOverlay{})
				if strings.TrimSpace(bodies) == "" {
					t.Fatalf("selecting %d (%s) %s drew an empty Bodies panel from %d positions", obj.Number, obj.Name, shape.name, len(res.Positions))
				}
				// No row may be drawn at the no-data sentinel. This is the
				// assertion that would fail if the drop were removed and the
				// operator were shown `Eris Ari 0°00'` as a real position.
				for _, p := range res.Positions {
					if p.Longitude == 0 && p.Speed == 0 && p.Latitude == 0 {
						t.Fatalf("selecting %d (%s) %s drew %q at 0Ari00 with no motion — an object with no ephemeris must be dropped, not shown as a position", obj.Number, obj.Name, shape.name, p.DisplayName(p.Body.String()))
					}
				}
				// Aspects may legitimately be empty — a one-body chart has no
				// pairs — so the assertion is that rendering does not blow up
				// and that a multi-body chart draws something.
				aspects := renderAspectsBody(state, TabAspects, 64, 20, Config{Accessible: true, Color: ColorNever})
				if len(res.Aspects) > 0 && strings.TrimSpace(aspects) == "" {
					t.Fatalf("selecting %d (%s) %s drew an empty Aspects panel from %d aspects", obj.Number, obj.Name, shape.name, len(res.Aspects))
				}
			})
		}
	}
}

// TestEverySelectableObjectIsReachableInTheModal is the selector half: an object
// the registry offers must be findable by typing its own name and tickable, or
// it is in the list and not actually selectable.
//
// The registry read is the only live part, so this skips with the binary too —
// but it spawns nothing per object, because the question is about the modal and
// not about the chart.
func TestEverySelectableObjectIsReachableInTheModal(t *testing.T) {
	if _, err := exec.LookPath("astrolog"); err != nil {
		t.Skip("astrolog not on PATH; skipping live-binary registry read")
	}
	reg := exhaustiveRegistry(t)

	for _, obj := range reg {
		obj := obj
		t.Run(shapeName(obj, "reachable"), func(t *testing.T) {
			st := openBodyModal(reg, recipe.ObjectPolicy{Baseline: recipe.Explicit}, nil)

			// Type the object's own name and find it among the matches. This
			// is what an operator does with a 134-row list, and it catches a
			// filter that cannot express a name — one with a space in it, a
			// trailing period, or a leading digit ("11th Cusp").
			for _, r := range obj.Name {
				st.appendRunes([]rune{r})
			}
			st.refilter()

			row := -1
			for i, m := range st.matches {
				if m.Number == obj.Number {
					row = i
					break
				}
			}
			if row < 0 {
				t.Fatalf("object %d (%q) cannot be found by typing its own name; %d rows matched", obj.Number, obj.Name, len(st.matches))
			}

			st.toggleAt(groupRows() + row)
			if !st.selected[obj.Number] {
				t.Fatalf("object %d (%q) did not tick", obj.Number, obj.Name)
			}
			sel := st.policy().Selected()
			if len(sel) != 1 || sel[0] != obj.Number {
				t.Fatalf("object %d (%q) produced selection %v", obj.Number, obj.Name, sel)
			}
			// ...and the policy it produces must be one the compiler accepts.
			if _, err := recipe.NewChartSpec(
				recipe.MomentAtPlace{JD: 2446232.0, Lon: 13.405, Lat: 52.52},
				recipe.WholeSign, recipe.Tropical, st.policy(),
			); err != nil {
				t.Fatalf("object %d (%q) produced a policy the recipe refuses: %v", obj.Number, obj.Name, err)
			}
		})
	}
}

// shapeName builds a subtest name that survives go test's escaping and still
// names the object, so a failure says which one without a lookup.
func shapeName(o aspect.RegistryObject, suffix string) string {
	name := strings.NewReplacer(" ", "_", ".", "", "/", "_").Replace(o.Name)
	return name + "_" + suffix
}

// TestEverySelectableObjectPairKeepsTheUIIntact closes the gap the independent
// review found in the test above: it walked each object with the lens and
// alone, never a selection in which EVERY object is one this Astrolog cannot
// compute. That case is different in kind — with no healthy body present,
// every body row is the no-data sentinel, and a guard that asks "are they all
// collapsed?" condemns the whole frame. Selecting Eris and Io together blanked
// the panels exactly as the original bug did.
//
// Pairs, not the full cross product: 134 x 134 is 17956 chart computations and
// the failure is a property of the SET being entirely unavailable, which two
// members demonstrate as well as forty.
func TestEverySelectableObjectPairKeepsTheUIIntact(t *testing.T) {
	if _, err := exec.LookPath("astrolog"); err != nil {
		t.Skip("astrolog not on PATH; skipping live-binary exhaustive pair test")
	}
	reg := exhaustiveRegistry(t)
	place := location.PlaceInfo{Name: "Berlin", Lat: 52.52, Lon: 13.405, TZ: "Europe/Berlin"}
	when := time.Date(1985, 6, 15, 12, 0, 0, 0, time.UTC)

	for i := 0; i+1 < len(reg); i++ {
		a, b := reg[i], reg[i+1]
		t.Run(shapeName(a, "with_"+shapeName(b, "pair")), func(t *testing.T) {
			res, err := lookup.FirstCallTransits(context.Background(), nil, when, place,
				exhaustiveLens([]int{a.Number, b.Number}))
			if err != nil {
				t.Fatalf("selecting %d (%s) and %d (%s) together made the frame fail: %v",
					a.Number, a.Name, b.Number, b.Name, err)
			}
			if len(res.Positions) == 0 && len(res.Unavailable) == 0 {
				t.Fatalf("selecting %d (%s) and %d (%s) produced no positions and named nothing unavailable",
					a.Number, a.Name, b.Number, b.Name)
			}
			for _, p := range res.Positions {
				if p.Longitude == 0 && p.Speed == 0 && p.Latitude == 0 {
					t.Fatalf("selecting %d and %d drew %q at the no-data sentinel", a.Number, b.Number, p.DisplayName(p.Body.String()))
				}
			}
		})
	}
}
