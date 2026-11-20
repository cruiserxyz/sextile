package astrolog

import (
	"errors"
	"regexp"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/cruiserxyz/sextile/internal/recipe"
)

// reNodeToken matches the two Node switches in any prefix: `Yn` (and `Ynx`,
// which Astrolog reads as `Yn`) and `Ynn`. `Yn0` is nutation and no Node
// switch.
var reNodeToken = regexp.MustCompile(`^[-/=_]Yn([^0n].*|n.*)?$`)

// reToggle matches a Node switch that toggles: the one spelling that casts one
// form on one host and another on a host whose file already set it.
var reToggle = regexp.MustCompile(`^[-/]Ynn?$`)

var allNodeForms = []recipe.NodeForm{recipe.NodesMean, recipe.NodesTrue, recipe.NodesMeanNatural, recipe.NodesTrueNatural}

// checkNodePins is N5 for one call: of the Node switches in the call's own argv
// (the host band that heads every command pins the mean form, so a call that
// carried none would pass on the head's), the last two are the form's, Node
// then Lilith, and none toggles.
func checkNodePins(t *testing.T, name string, argv []string, form recipe.NodeForm) {
	t.Helper()
	var nodes []string
	for _, tok := range append(CallHead(), argv...) {
		if reToggle.MatchString(tok) {
			t.Errorf("%s: carries the toggle %s: %q", name, tok, argv)
		}
		if reNodeToken.MatchString(tok) {
			nodes = append(nodes, tok)
		}
	}
	own := 0
	for _, tok := range argv {
		if reNodeToken.MatchString(tok) {
			own++
		}
	}
	want := form.Switches()
	if own < 2 || len(nodes) < 2 || nodes[len(nodes)-2] != want[0] || nodes[len(nodes)-1] != want[1] {
		t.Errorf("%s: the last Node switches are %v, want %v last; argv %q", name, nodes, want, argv)
	}
	// After every record: a record read later (`-i`, `-i2`…, `-r0`, a wheel's
	// `-r3`…) could set the form again after the pins.
	lastRecord, lastNode := -1, -1
	for i, tok := range argv {
		if reRecordSwitch.MatchString(tok) {
			lastRecord = i
		}
		if reNodeToken.MatchString(tok) && lastNode < i {
			lastNode = i
		}
	}
	if lastRecord >= 0 {
		second := -1
		for i := lastNode - 1; i >= 0; i-- {
			if reNodeToken.MatchString(argv[i]) {
				second = i
				break
			}
		}
		if second < lastRecord {
			t.Errorf("%s: a record (%s) comes after the Node pins; argv %q", name, argv[lastRecord], argv)
		}
	}
}

// reRecordSwitch is a switch that reads a chart record.
var reRecordSwitch = regexp.MustCompile(`^-(i[0-9]?|r[0-9t])$`)

// N5: every call that can compute, report or search a Node or Lilith carries
// both switches of the form it is cast in, last.
func TestEveryCastingCallPinsBothNodeSwitchesLast(t *testing.T) {
	record := "@AI800\n-qb Jun 1 1990 16:30 DT 1E 4:53E 52:22N\n-zi \"A B\" \"P\"\n"
	fileSrc := recipe.FileChart{Record: record, JD: 2448043.0, Lon: 4.9, Lat: 52.4}
	for _, form := range allNodeForms {
		words, lilith := form.Words()
		t.Run(words+"-"+lilith, func(t *testing.T) {
			sources := map[string]recipe.Source{
				"single":       zonedLondon,
				"local":        recipe.LocalMomentAtPlace{JD: 2450000.5, Lon: 0, Lat: 51.5, TZ: "Europe/London"},
				"file":         fileSrc,
				"comparison":   recipe.Comparison{Primary: fixLondon, Secondary: fixTromso},
				"transit":      recipe.Comparison{Primary: fixLondon, Secondary: fixTromso, Relation: recipe.TransitToNatal},
				"file pair":    recipe.Comparison{Primary: fileSrc, Secondary: fixTromso},
				"multiwheel 3": recipe.MultiWheel{Rings: []recipe.Source{fixLondon, fixTromso, fixOslo}},
				"multiwheel 4": recipe.MultiWheel{Rings: []recipe.Source{fixLondon, fixTromso, fixOslo, fileSrc}},
			}
			wheel := recipe.WheelImage{Render: recipe.DefaultWheelRender()}
			body := recipe.BodyRawReport(objVenus)
			for name, src := range sources {
				spec := fixtureSpec(t, src, recipe.Placidus, recipe.Tropical)
				spec.Objects.Nodes = form
				projs := []recipe.Projection{recipe.Aspects{}, recipe.AspectsRawReport(), body, wheel}
				switch name {
				case "single", "local", "file":
					projs = append(projs, recipe.Positions{}, recipe.ChartRawReport())
				case "multiwheel 3", "multiwheel 4":
					projs = []recipe.Projection{wheel}
				}
				for _, p := range projs {
					plan, err := Compile(spec, p)
					if err != nil {
						t.Fatalf("%s/%s: %v", name, p.Label(), err)
					}
					for _, inv := range plan.Invocations {
						checkNodePins(t, name+"/"+p.Label(), inv.Argv, form)
						checkNodePins(t, name+"/"+p.Label()+" call", inv.Call().Argv(), form)
					}
				}
			}

			// The three timeline sources, through timelineBand.
			natal := &TimelineNatal{When: time.Date(1990, 5, 1, 12, 0, 0, 0, time.UTC), Lon: 4.9, Lat: 52.37}
			natal.Objects.Nodes = form
			withFile := *natal
			withFile.Record = record
			for name, n := range map[string]*TimelineNatal{"natal": natal, "natal from file": &withFile} {
				req := TimelineRequest{Year: 2026, Month: time.October, Zone: time.UTC, Natal: n}
				req.Objects.Nodes = form
				for _, src := range []TimelineSource{TimelineTransit, TimelineSky, TimelineReturn} {
					call, err := TimelineCall(src, TimelineSegment{Offset: 3600}, req)
					if err != nil {
						t.Fatalf("%s/%s: %v", name, src, err)
					}
					checkNodePins(t, "timeline "+name+"/"+src.String(), call.Argv(), form)
				}
			}

			// The availability probe pins the mean form, whichever the chart is in.
			probe, err := probeCall([]int{1, 17, 18})
			if err != nil {
				t.Fatal(err)
			}
			checkNodePins(t, "availability probe", probe.Argv(), recipe.NodesMean)

			// The cycle search and the Sun pair.
			cycle, err := CycleCall(CycleRequest{StartYear: 2016, Years: 13, Other: objVenus, Nodes: form})
			if err != nil {
				t.Fatal(err)
			}
			checkNodePins(t, "cycle", cycle.Argv(), form)
			pair, err := SunPairCall(SunPairRequest{When: time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC), Lat: 51, Other: objVenus, Nodes: form})
			if err != nil {
				t.Fatal(err)
			}
			checkNodePins(t, "sun pair", pair.Argv(), form)
		})
	}
}

// The void search lists the planets from the Sun to Pluto, so it can neither
// search nor print a Node or Lilith, and carries no Node switch. Were its
// object list to grow past Pluto this test fails and the call must pin the form.
func TestVoidSearchNeverSearchesTheNodes(t *testing.T) {
	reg := testRegistry(t)
	call, err := VoidCall(VoidRequest{Moment: time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC), Registry: reg})
	if err != nil {
		t.Fatal(err)
	}
	pluto, ok := reg.ByName("Pluto")
	if !ok {
		t.Fatal("no Pluto in the registry")
	}
	for _, o := range reg {
		if o.Number > pluto.Number && slices.Contains(numbersAfter(call.argv, flagMainShow), o.Number) {
			t.Errorf("the void search lists %s (%d), past Pluto", o.Name, o.Number)
		}
	}
	for _, tok := range call.argv {
		if reNodeToken.MatchString(tok) {
			t.Errorf("the void search carries the Node switch %s", tok)
		}
	}
}

// numbersAfter is the integers that follow flag on argv, up to the next switch.
func numbersAfter(argv []string, flag string) []int {
	i := slices.Index(argv, flag)
	if i < 0 {
		return nil
	}
	var out []int
	for _, tok := range argv[i+1:] {
		n, err := strconv.Atoi(tok)
		if err != nil {
			break
		}
		out = append(out, n)
	}
	return out
}

// S3: the pins are part of the band a search call is issued with. A cycle or
// Sun-pair call that loses either of them is refused at the seam; before the
// pins were in the band, `=Yn` was appended outside it and removing it still
// passed.
func TestSearchCallWithoutItsNodePinsFailsCheckBand(t *testing.T) {
	cycle, err := CycleCall(CycleRequest{StartYear: 2016, Years: 13, Other: objVenus, Nodes: recipe.NodesTrueNatural})
	if err != nil {
		t.Fatal(err)
	}
	pair, err := SunPairCall(SunPairRequest{When: time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC), Lat: 51, Other: objVenus, Nodes: recipe.NodesTrueNatural})
	if err != nil {
		t.Fatal(err)
	}
	for name, call := range map[string]BodyCall{"cycle": cycle, "sun pair": pair} {
		if err := checkBand(call); err != nil {
			t.Fatalf("%s: a call as issued is refused: %v", name, err)
		}
		for _, pin := range recipe.NodesTrueNatural.Switches() {
			var without []string
			for _, tok := range call.argv {
				if tok != pin {
					without = append(without, tok)
				}
			}
			if len(without) != len(call.argv)-1 {
				t.Fatalf("%s: %s is on the argv %d times, want once", name, pin, len(call.argv)-len(without))
			}
			cut := newBodyCall(without, call.band)
			if err := checkBand(cut); !errors.Is(err, ErrBandMissing) {
				t.Errorf("%s: without %s checkBand = %v, want ErrBandMissing", name, pin, err)
			}
		}
	}
}
