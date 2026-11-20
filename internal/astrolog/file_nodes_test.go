package astrolog

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/cruiserxyz/sextile/internal/recipe"
)

const nodeTestHead = "@AI800\n-qb Jun 1 1990 16:30 DT 1E 4:53E 52:22N\n"

// N2: a file's form is one walk from Astrolog's default (the mean Node and the
// mean Lilith) over its Node switches in file order.
func TestFileNodeFormWalk(t *testing.T) {
	for _, tc := range []struct {
		name  string
		lines string
		want  recipe.NodeForm
		state bool
	}{
		{"mean and mean set explicitly", "_Yn _Ynn", recipe.NodesMean, true},
		{"true and true", "=Yn _Ynn", recipe.NodesTrue, true},
		{"mean and natural", "_Yn =Ynn", recipe.NodesMeanNatural, true},
		{"true and natural", "=Yn =Ynn", recipe.NodesTrueNatural, true},
		{"=Yn alone", "=Yn", recipe.NodesTrue, true},
		{"=Ynn alone", "=Ynn", recipe.NodesMeanNatural, true},
		{"-Yn toggles the default on", "-Yn", recipe.NodesTrue, true},
		{"/Yn toggles the default on", "/Yn", recipe.NodesTrue, true},
		{"-Ynn toggles the natural Lilith on", "-Ynn", recipe.NodesMeanNatural, true},
		{"-Yn twice is off again", "-Yn -Yn", recipe.NodesMean, true},
		{"=Yn then -Yn", "=Yn\n-Yn", recipe.NodesMean, true},
		{"two that conflict on one line, the last wins", "=Yn _Yn", recipe.NodesMean, true},
		{"the same conflict the other way", "_Yn =Yn", recipe.NodesTrue, true},
		{"across lines, the last wins", "=Yn =Ynn\n_Ynn", recipe.NodesTrue, true},
		{"across lines, set after clear", "_Yn\n=Yn", recipe.NodesTrue, true},
		{"Yn0 is nutation, no Node switch", "=Yn0", recipe.NodesMean, false},
		{"_Yn0 and -Yn0 are nutation too", "_Yn0 -Yn0", recipe.NodesMean, false},
		{"Ynx is the true Node and Lilith", "=Ynx", recipe.NodesTrue, true},
		{"Ynn0 is the natural Lilith", "=Ynn0", recipe.NodesMeanNatural, true},
		{"nutation beside a Node switch", "=Yn0 =Yn", recipe.NodesTrue, true},
		{"no Node switch states nothing", "-zi \"A\" \"B\"", recipe.NodesMean, false},
		{"a comment holds none", "; =Yn =Ynn", recipe.NodesMean, false},
		{"Astrolog's -; switch ends the line too", "-; =Yn =Ynn", recipe.NodesMean, false},
		{"=; and the rest after a switch", "=Yn =; =Ynn", recipe.NodesTrue, true},
		{"the rest of a line after a comment holds none", "=Yn ;=Ynn", recipe.NodesTrue, true},
		{"a label that spells one is a label", "-zi \"=Yn\" \"=Ynn\"", recipe.NodesMean, false},
		{"a quoted word in switch position is a switch", "\"=Yn\"", recipe.NodesTrue, true},
		{"on a line with other switches", "-zi \"A\" \"B\" =Yn =Ynn", recipe.NodesTrueNatural, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, stated := FileNodeForm(nodeTestHead + tc.lines + "\n")
			if got != tc.want || stated != tc.state {
				t.Errorf("walk of %q = %v, stated %v; want %v, stated %v", tc.lines, got, stated, tc.want, tc.state)
			}
		})
	}
	// The walk starts after the header, which only a comment may follow.
	if got, stated := FileNodeForm("@AI800 ; =Yn\n-qb Jun 1 1990 16:30 DT 1E 4:53E 52:22N\n"); got != recipe.NodesMean || stated {
		t.Errorf("the header line is walked: %v, stated %v", got, stated)
	}
}

// A walk over two flags can end on none of the pairs Astrolog cannot cast:
// every combination of the true Node and the natural Lilith is a form, and
// `Yn` never sets Lilith apart from the Node (astrolog.cpp:538-545), so the
// true Node with the mean Lilith is not a state the walk reaches.
func TestFileNodeFormWalkNeverEndsOnTheUncastablePair(t *testing.T) {
	switches := []string{"=Yn", "_Yn", "-Yn", "/Yn", "=Ynn", "_Ynn", "-Ynn", "/Ynn", "=Ynx", "=Yn0"}
	castable := map[recipe.NodeForm]bool{}
	for _, f := range allNodeForms {
		castable[f] = true
	}
	var walk func(lines []string, depth int)
	walk = func(lines []string, depth int) {
		form, _ := FileNodeForm(nodeTestHead + strings.Join(lines, "\n") + "\n")
		node, lilith := form.Words()
		if _, ok := recipe.NodeFormFromWords(node, lilith); !ok || !castable[form] {
			t.Fatalf("the walk of %q ends on %v, which is no castable pair", lines, form)
		}
		if depth == 0 {
			return
		}
		for _, s := range switches {
			walk(append(slices.Clone(lines), s), depth-1)
		}
	}
	walk(nil, 3)
}

// N4: no Node or Lilith switch reaches Astrolog, and the guard says nothing
// about one.
func TestGuardTakesNodeSwitchesOut(t *testing.T) {
	for _, tc := range []struct{ name, lines, want string }{
		{"a line of Node switches only", "=Yn =Ynn", ""},
		{"each prefix and each spelling", "-Yn\n/Yn\n=Yn\n_Yn\n-Ynn\n/Ynn\n=Ynn\n_Ynn\n=Ynx", ""},
		{"before display switches the whole line is left out as before", "_Yn =Yt", ""},
		{"beside a moment switch the moment stays", "-qb Jun 2 1990 16:30 DT 1E 4:53E 52:22N =Yn", "-qb Jun 2 1990 16:30 DT 1E 4:53E 52:22N"},
		{"before it", "_Ynn -qb Jun 2 1990 16:30 DT 1E 4:53E 52:22N", "-qb Jun 2 1990 16:30 DT 1E 4:53E 52:22N"},
		{"between two allowed switches", "-c Koch =Yn -s Lahiri", "-c Koch -s Lahiri"},
		{"after a labelled switch", "-zi \"A B\" \"P Q\" =Ynn", `-zi "A B" "P Q"`},
		{"a label that spells one stays", "-zi \"=Yn\" \"=Ynn\"", `-zi =Yn =Ynn`},
		{"a quoted switch is cut with its quotes", "-c Koch \"=Yn\"", "-c Koch"},
		{"before a comment", "-c Koch =Yn ;=Ynn", "-c Koch ;=Ynn"},
		{"nothing for a comment", "; =Yn", ""},
		{"nothing after Astrolog's -; switch", "-; =Yn", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, notes, err := GuardChartFile(nodeTestHead + tc.lines + "\n")
			if err != nil {
				t.Fatalf("refused: %v", err)
			}
			if len(notes) != 0 {
				t.Errorf("notes %v, want none", notes)
			}
			want := nodeTestHead
			if tc.want != "" {
				want += tc.want + "\n"
			}
			// Compare what Astrolog would read, never the spaces left behind.
			if strings.Join(InlineRecord(got), " ") != strings.Join(InlineRecord(want), " ") {
				t.Errorf("the record Astrolog gets is %q, want %q", InlineRecord(got), InlineRecord(want))
			}
			for _, tok := range InlineRecord(got) {
				if reNodeToken.MatchString(tok) && !strings.Contains(tc.lines, `"`+tok+`"`) {
					t.Errorf("the guarded record still holds %s", tok)
				}
			}
		})
	}
	// After a switch whose arguments Sextile does not know, a word that reads as
	// a Node switch may be its argument to Astrolog: the file is refused, naming it.
	for _, line := range []string{`=YZ "=Yn"`, "=Yt _Yn", "-YAo 1 2 =Ynn", `=YZ ";" =Yn`, "-c3 =Yn 2", "-c Koch =Yn Placidus"} {
		_, _, err := GuardChartFile(nodeTestHead + line + "\n")
		var refused *ChartRefused
		if !errors.As(err, &refused) || !reNodeToken.MatchString(refused.Switch) {
			t.Errorf("%q: %v, want the file refused naming the Node word", line, err)
		}
	}
	// Nutation is no Node switch: it is refused as before.
	_, _, err := GuardChartFile(nodeTestHead + "=Yn0\n")
	var refused *ChartRefused
	if !errors.As(err, &refused) || refused.Switch != "=Yn0" {
		t.Errorf("=Yn0: %v, want a refusal naming =Yn0", err)
	}
	// What is left of a line is classified as before: an unknown switch
	// beside a Node switch still refuses the chart.
	_, _, err = GuardChartFile(nodeTestHead + "-qb Jun 2 1990 16:30 DT 1E 4:53E 52:22N =Yn =YP -1\n")
	if !errors.As(err, &refused) || refused.Switch != "=YP" {
		t.Errorf("=YP beside =Yn: %v, want a refusal naming =YP", err)
	}
}

// The read of a file takes its form and hands Astrolog a record without any
// Node switch: the Log gets no note, and the box read is the file's moment.
func TestChartFileReadStatesItsFormAndHandsAstrologNone(t *testing.T) {
	needAstrolog(t)
	text := nodeTestHead + "=Yn =Ynn\n-zi \"A\" \"B\"\n"
	cf, err := ResolveChartBytes(context.Background(), []byte(text), "forms.as")
	if err != nil {
		t.Fatal(err)
	}
	if !cf.NodesStated || cf.Nodes != recipe.NodesTrueNatural {
		t.Errorf("form %v, stated %v; want true and natural, stated", cf.Nodes, cf.NodesStated)
	}
	if len(cf.Notes) != 0 {
		t.Errorf("notes %v, want none", cf.Notes)
	}
	if strings.Contains(cf.Record, "Yn") {
		t.Errorf("the record holds a Node switch: %q", cf.Record)
	}
	plain, err := ResolveChartBytes(context.Background(), []byte(nodeTestHead), "plain.as")
	if err != nil {
		t.Fatal(err)
	}
	if plain.NodesStated || plain.JD != cf.JD {
		t.Errorf("a file with no switch states %v, JD %v against %v", plain.NodesStated, plain.JD, cf.JD)
	}
}

// N16, the engine's half: a natural Lilith that Astrolog cannot place (its
// interpolated apogee stops at JD 2818000.5; the chart prints it at 0 Aries,
// the no-data row, with one warning that names no object) is left out and
// named by the way every body it cannot place is, and the rest of the chart is
// cast. Without the warning being read as one of those, the whole chart was
// refused. The warning is Astrolog's own text (sweph.cpp, SE_INTP_APOG). The
// era beyond it needs a Swiss Ephemeris file that is not in every data folder,
// and a missing file's warning comes first, so the case is the call's stderr
// and stdout, read by the one function that decides it.
func TestNaturalLilithOutOfRangeIsLeftOutByTheEngine(t *testing.T) {
	reg := testRegistry(t)
	south, ok1 := reg.ByName("South Node")
	lilith, ok2 := reg.ByName("Lilith")
	if !ok1 || !ok2 {
		t.Fatal("no South Node or Lilith in the registry")
	}
	call := BodyCall{Registry: reg, Objects: []int{south.Number, lilith.Number}}
	stdout := "Sout: 28Sag10 R + 0:00' (X) [ 2nd house] [-] -0.052  -  House cusp  1: 21Sco27\n" +
		"Lili:  0Ari00   + 0:00' (-) [ 5th house] [-] +0.000  -  House cusp  2: 19Sag56\n"
	warning := "Interpolated apsides are restricted to JD 625000.5 - JD 2818000.5"
	nums, names, ok := unplaceable([]string{warning}, stdout, call, nil)
	if !ok || !slices.Equal(nums, []int{lilith.Number}) || !slices.Equal(names, []string{"Lilith"}) {
		t.Errorf("unplaceable = %v %v %v, want only Lilith", nums, names, ok)
	}
	// The same row without the warning is a body Astrolog placed at 0 Aries:
	// nothing here says it could not be placed, so nothing is left out.
	if _, _, ok := unplaceable(nil, stdout, call, nil); ok {
		t.Error("a no-data row with no warning was left out")
	}
	// A call that draws nothing else has nothing left to cast.
	if _, _, ok := unplaceable([]string{warning}, stdout, BodyCall{Registry: reg, Objects: []int{lilith.Number}}, nil); ok {
		t.Error("the only object of a call was left out")
	}
	// A warning of some other kind beside it refuses the chart as before.
	if _, _, ok := unplaceable([]string{warning, "something else"}, stdout, call, nil); ok {
		t.Error("an unknown warning was passed over")
	}
}

// In a year the interpolated apogee covers the natural Lilith is cast, so the
// rule above leaves out only what the warning names.
func TestNaturalLilithInRangeIsCast(t *testing.T) {
	needAstrolog(t)
	reg, err := ObjectRegistry(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	spec, err := recipe.NewChartSpec(recipe.MomentAtPlace{JD: 2451545.0, Lon: 0, Lat: 0}, recipe.Placidus, recipe.Tropical,
		recipe.ObjectPolicy{Baseline: recipe.Explicit, Selection: []int{17, 18}, Nodes: recipe.NodesMeanNatural})
	if err != nil {
		t.Fatal(err)
	}
	spec.Registry = reg
	spec.AspectRegistry = testAspects(t)
	out, err := ComputeChart(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}
	var lon float64
	for _, p := range out.Positions {
		if p.Object.Name == "Lilith" {
			lon = p.Longitude
		}
	}
	if lon == 0 || len(out.Unavailable) != 0 {
		t.Errorf("the natural Lilith in the year 2000 is at %v, left out %v", lon, out.Unavailable)
	}
}

// The natural Lilith past its range, through the object toggles a lens adds
// (`-R 18` turns Lilith on in Astrolog's default set), against a stand-in that
// runs the real program and then prints what Astrolog prints for such a date:
// the no-data row and, on stderr, the one warning that names no object. No data
// file of an installation covers a date outside JD 625000.5 to 2818000.5.
func TestNaturalLilithOutOfRangeIsLeftOutThroughToggles(t *testing.T) {
	needAstrolog(t)
	real, err := exec.LookPath(binary)
	if err != nil {
		t.Skip("no astrolog on PATH")
	}
	dir := t.TempDir()
	script := `#!/bin/sh
LC_ALL=C
export LC_ALL
out=$('` + real + `' "$@"); rc=$?
case "$out" in
*"Lili: "*) echo "Interpolated apsides are restricted to JD 625000.5 - JD 2818000.5" >&2 ;;
esac
printf '%s\n' "$out" | sed -E "/Lili: /{s/(Lili: .\[[0-9;]*m)[ 0-9][0-9][A-Za-z]{3}[0-9]{2}/\\1 0Ari00/;s/(\\[0m)( R)? +[-+] [^']*'/\\1   + 0:00'/;s/(\\[[-a-zA-Z]\\]) [-+][0-9.]+/\\1 +0.000/}"
exit $rc
`
	path := filepath.Join(dir, "astrolog")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	old := binary
	binary = path
	t.Cleanup(func() { binary = old })

	reg, err := ObjectRegistry(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	spec, err := recipe.NewChartSpec(recipe.MomentAtPlace{JD: 2451545.0, Lon: 0, Lat: 0}, recipe.Placidus, recipe.Tropical,
		recipe.ObjectPolicy{Baseline: recipe.AstrologDefault, Add: recipe.NewObjectSet(11, 17, 18), Nodes: recipe.NodesMeanNatural})
	if err != nil {
		t.Fatal(err)
	}
	spec.Registry = reg
	spec.AspectRegistry = testAspects(t)
	out, err := ComputeChart(context.Background(), spec)
	if err != nil {
		t.Fatalf("the chart was refused: %v", err)
	}
	if !slices.Equal(out.Unavailable, []string{"Lilith"}) {
		t.Errorf("left out %v, want only Lilith", out.Unavailable)
	}
	for _, p := range out.Positions {
		if p.Object.Name == "Lilith" {
			t.Errorf("Lilith is in the positions at %v", p.Longitude)
		}
	}
}

// A command as the console shows it (formatCommand quotes every `=` word)
// names the form its call cast.
func TestCommandNodeFormReadsTheConsoleCommand(t *testing.T) {
	for _, form := range allNodeForms {
		sw := form.Switches()
		cmd := formatCommand("/usr/local/bin/astrolog", []string{"-qj", "2451545", "_Yn", "_Ynn", "-R0", "16", sw[0], sw[1]})
		if got := CommandNodeForm(cmd); got != form {
			t.Errorf("%q names %v, want %v", cmd, got, form)
		}
	}
}
