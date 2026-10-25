package astrolog

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/cruiserxyz/sextile/internal/recipe"
)

// multiWheelSpec is n synthetic charts with distinct dates, ring 1 first.
func multiWheelSpec(t *testing.T, n int) recipe.ChartSpec {
	t.Helper()
	rings := make([]recipe.Source, n)
	for i := range rings {
		rings[i] = recipe.MomentAtPlace{JD: 2446232.0 + 700*float64(i), Lon: 11.5667 - float64(i), Lat: 48.1333}
	}
	spec, err := recipe.NewChartSpec(recipe.MultiWheel{Rings: rings}, recipe.WholeSign, recipe.Tropical,
		recipe.ObjectPolicy{Baseline: recipe.AstrologDefault})
	if err != nil {
		t.Fatalf("multi-wheel spec: %v", err)
	}
	return spec
}

// TestCompileMultiWheelArgv: N rings lower to -i <f1> -i2 <f2> … -iN <fN> -rN
// with one staged chart per ring at the slot after its switch, in ring order,
// then the frame band and the image tail.
func TestCompileMultiWheelArgv(t *testing.T) {
	for n := 3; n <= 6; n++ {
		spec := multiWheelSpec(t, n)
		plan, err := Compile(spec, recipe.WheelImage{Render: recipe.DefaultWheelRender()})
		if err != nil {
			t.Fatalf("%d rings: Compile: %v", n, err)
		}
		if len(plan.Invocations) != 1 {
			t.Fatalf("%d rings: %d invocations, want 1 (no highlight layers)", n, len(plan.Invocations))
		}
		inv := plan.Invocations[0]
		var want []string
		for i := 1; i <= n; i++ {
			sw := "-i"
			if i > 1 {
				sw = fmt.Sprintf("-i%d", i)
			}
			want = append(want, sw, "")
		}
		want = append(want, fmt.Sprintf("-r%d", n), "-c", "14")
		if got := inv.Argv[:len(want)]; strings.Join(got, "|") != strings.Join(want, "|") {
			t.Errorf("%d rings: argv head = %q, want %q", n, got, want)
		}
		if len(inv.TempInputs) != n {
			t.Fatalf("%d rings: %d staged charts", n, len(inv.TempInputs))
		}
		for i, ti := range inv.TempInputs {
			if ti.ArgIndex != 2*i+1 {
				t.Errorf("%d rings: chart %d staged at argv[%d], want %d", n, i+1, ti.ArgIndex, 2*i+1)
			}
			if ti.Content == "" {
				t.Errorf("%d rings: chart %d content empty", n, i+1)
			}
		}
		// Distinct charts in ring order: the contents differ pairwise.
		for i := 1; i < n; i++ {
			if inv.TempInputs[i].Content == inv.TempInputs[i-1].Content {
				t.Errorf("%d rings: charts %d and %d are identical", n, i, i+1)
			}
		}
	}
}

// TestMultiWheelRefusesEveryTextProjection: Astrolog has no N-way listing, so
// every non-wheel projection must refuse a MultiWheel rather than answer a
// different question.
func TestMultiWheelRefusesEveryTextProjection(t *testing.T) {
	spec := multiWheelSpec(t, 3)
	for _, p := range []recipe.Projection{
		recipe.Positions{}, recipe.Aspects{}, recipe.ChartRawReport(),
		recipe.AspectsRawReport(), recipe.BodyRawReport(0),
	} {
		if _, err := Compile(spec, p); err == nil {
			t.Errorf("Compile accepted %s over a MultiWheel", p.Label())
		}
		if _, err := CompileBundle(spec, p); err == nil {
			t.Errorf("CompileBundle accepted %s over a MultiWheel", p.Label())
		}
	}
	if _, err := CompileBundle(spec, recipe.WheelImage{Render: recipe.DefaultWheelRender()}); err != nil {
		t.Errorf("CompileBundle refused the wheel: %v", err)
	}
}

// TestValidateArgv_MultiRingRoot: -r3..-r6 is a chart entry only with exactly
// that many file switches, each followed by an argument.
func TestValidateArgv_MultiRingRoot(t *testing.T) {
	ok := [][]string{
		{"-i", "a", "-i2", "b", "-i3", "c", "-r3", "-c", "14"},
		{"-r4", "-i2", "b", "-i", "a", "-i4", "d", "-i3", "c"},
		{"-i", "a", "-i2", "b", "-i3", "c", "-i4", "d", "-i5", "e", "-i6", "f", "-r6"},
	}
	for _, argv := range ok {
		if err := validateArgv(argv); err != nil {
			t.Errorf("refused %v: %v", argv, err)
		}
	}
	bad := map[string][]string{
		"too few files":           {"-i", "a", "-i2", "b", "-r3"},
		"too many files":          {"-i", "a", "-i2", "b", "-i3", "c", "-i4", "d", "-r3"},
		"duplicate switch":        {"-i", "a", "-i2", "b", "-i2", "c", "-r3"},
		"missing -i2":             {"-i", "a", "-i3", "c", "-i4", "d", "-r3"},
		"file switch no argument": {"-i", "a", "-i2", "b", "-i3", "-r3"},
		"last file no argument":   {"-r3", "-i", "a", "-i2", "b", "-i3"},
		"root without files":      {"-r3", "-c", "14"},
		"-r5 with four":           {"-i", "a", "-i2", "b", "-i3", "c", "-i4", "d", "-r5"},
		"two ring set only":       {"-i", "a", "-i2", "b", "-r2"},
		"files without a root":    {"-i", "a", "-i2", "b", "-i3", "c"},
		"naked -b":                {"-i", "a", "-i2", "b", "-i3", "c", "-r3", "-b"},
		"-i beside -qj":           {"-qj", "1", "1", "2000", "0", "0", "0", "-i", "a"},
		"ring set with -r0":       {"-i", "a", "-i2", "b", "-i3", "c", "-r3", "-r0", "x", "y"},
		"ring set with -qj":       {"-i", "a", "-i2", "b", "-i3", "c", "-r3", "-qj", "1", "1", "2000", "0", "0", "0"},
	}
	for name, argv := range bad {
		if err := validateArgv(argv); !errors.Is(err, ErrUnsafeArgv) {
			t.Errorf("%s: %v accepted (err=%v)", name, argv, err)
		}
	}
}

// failNthStagedWrite replaces the staging write seam with one that records
// every path and fails the k-th call (1-based); the real write happens for the
// others, so the dir has live files in it when the failure comes.
func failNthStagedWrite(t *testing.T, k int) *[]string {
	t.Helper()
	prev := writeStagedFile
	t.Cleanup(func() { writeStagedFile = prev })
	var paths []string
	writeStagedFile = func(name string, data []byte, perm os.FileMode) error {
		paths = append(paths, name)
		if len(paths) == k {
			return errors.New("injected write failure")
		}
		return os.WriteFile(name, data, perm)
	}
	return &paths
}

// TestStagingFailureLeavesNoFiles: when the third of four staged charts cannot
// be written, the two already written and their directory are gone, and the
// error comes back.
func TestStagingFailureLeavesNoFiles(t *testing.T) {
	plan, err := Compile(multiWheelSpec(t, 4), recipe.WheelImage{Render: recipe.DefaultWheelRender()})
	if err != nil {
		t.Fatal(err)
	}
	paths := failNthStagedWrite(t, 3)
	_, err = RenderImage(context.Background(), plan.Invocations[0])
	if err == nil || !strings.Contains(err.Error(), "injected write failure") {
		t.Fatalf("RenderImage err = %v, want the injected write failure", err)
	}
	if len(*paths) != 3 {
		t.Fatalf("%d writes attempted, want exactly 3 (stop at the failure)", len(*paths))
	}
	dir := filepath.Dir((*paths)[0])
	if _, statErr := os.Stat(dir); !os.IsNotExist(statErr) {
		t.Errorf("staging dir %s still exists (stat err %v)", dir, statErr)
	}
	for _, p := range (*paths)[:2] {
		if _, statErr := os.Stat(p); !os.IsNotExist(statErr) {
			t.Errorf("staged file %s survived the failure", p)
		}
	}
}

// TestStagedMultiWheelRendersAPicture is the live half of the ring contract:
// three and six synthetic charts render as SVG holding exactly N-1 ring
// separator circles (#007f00, measured on Astrolog 8.00: 1, 2, 3, 5 for 2, 3,
// 4, 6 rings), and every staged file and the staging dir are gone afterwards.
// Ring ORDER is not observable here; the operator confirms it on the image.
func TestStagedMultiWheelRendersAPicture(t *testing.T) {
	if _, err := exec.LookPath("astrolog"); err != nil {
		t.Skip("astrolog not on PATH; skipping live-binary multi-wheel test")
	}
	for _, n := range []int{3, 6} {
		paths := recordStagedWrites(t)
		data, err := RenderWheelImage(context.Background(), multiWheelSpec(t, n), recipe.DefaultWheelRender())
		if err != nil {
			t.Fatalf("%d rings: RenderWheelImage: %v", n, err)
		}
		if !bytes.HasPrefix(data, []byte("<?xml ")) || !bytes.Contains(data, []byte("<svg ")) {
			t.Fatalf("%d rings: not an SVG; starts %q", n, data[:min(80, len(data))])
		}
		if got := separatorCircles(string(data)); got != n-1 {
			t.Errorf("%d rings: %d #007f00 separator circles, want %d", n, got, n-1)
		}
		if len(*paths) != n {
			t.Fatalf("%d rings: %d files staged, want %d", n, len(*paths), n)
		}
		if _, err := os.Stat(filepath.Dir((*paths)[0])); !os.IsNotExist(err) {
			t.Errorf("%d rings: staging dir still exists (stat err %v)", n, err)
		}
		for _, p := range *paths {
			if _, err := os.Stat(p); !os.IsNotExist(err) {
				t.Errorf("%d rings: staged file %s survived the call", n, p)
			}
		}
	}
}

// ringSeparatorGroup matches Astrolog's SVG group of ring-separator strokes:
// the circles sit together inside one <g style="stroke:#007f00"> element.
var ringSeparatorGroup = regexp.MustCompile(`(?s)<g style="stroke:#007f00">(.*?)</g>`)

// separatorCircles counts the circles drawn in the #007f00 stroke group.
func separatorCircles(svg string) int {
	n := 0
	for _, m := range ringSeparatorGroup.FindAllStringSubmatch(svg, -1) {
		n += strings.Count(m[1], "<circle")
	}
	return n
}

// recordStagedWrites records every staged path without changing the write.
func recordStagedWrites(t *testing.T) *[]string {
	t.Helper()
	prev := writeStagedFile
	t.Cleanup(func() { writeStagedFile = prev })
	var paths []string
	writeStagedFile = func(name string, data []byte, perm os.FileMode) error {
		paths = append(paths, name)
		return os.WriteFile(name, data, perm)
	}
	return &paths
}

// TestTimelineSourceSwitch: a failed search is reported under Astrolog's own
// switch.
func TestTimelineSourceSwitch(t *testing.T) {
	for src, want := range map[TimelineSource]string{TimelineTransit: "-t", TimelineSky: "-dm", TimelineReturn: "-tr"} {
		if got := src.Switch(); got != want {
			t.Errorf("%v.Switch() = %q, want %q", src, got, want)
		}
	}
}

// TestSaveArgvTitleIsNeverASwitch: a chart's title and place are text the
// operator typed, and reach Astrolog as `-zi <title> <place>`. A title that
// spells a switch — "-i", "-i2", "-r3", "-b" — must neither unlock nor trip the
// runner's file-switch and ring-set rules (review round 2, item 2).
func TestSaveArgvTitleIsNeverASwitch(t *testing.T) {
	when := time.Date(1990, 6, 15, 14, 30, 0, 0, time.UTC)
	for _, label := range []string{"-i", "-i2", "-r3", "-b", "-zi"} {
		c := ChartToSave{When: when, Lon: 8.5, Lat: 52.9, Name: label, Place: label}
		if err := validateArgv(saveArgv(c, "/tmp/x.as")); err != nil {
			t.Errorf("title and place %q refused: %v", label, err)
		}
	}
}
