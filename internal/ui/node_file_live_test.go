package ui

// The form a chart file asks for is what Astrolog casts from it, and every
// casting call holds against a host's own settings, against the real binary.

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/internal/recipe"
	"github.com/cruiserxyz/sextile/internal/store"
	"github.com/cruiserxyz/sextile/internal/testenv/astrodir"
)

const nodeFileHead = "@AI800\n-qb Jun 1 1990 16:30 DT 1E 4:53E 52:22N\n"

var (
	reNodeSGR = regexp.MustCompile(`\x1b\[[0-9;]*m`)
	reNodeRow = regexp.MustCompile(`(?m)^(Nort|Sout|Lili): *([0-9]{1,2}[A-Z][a-z]{2}[0-9]{2}'[0-9]{2}")`)
)

// nodeRows are the North Node, South Node and Lilith of a positions listing
// that prints seconds, by Astrolog's own label.
func nodeRows(listing string) map[string]string {
	out := map[string]string{}
	for _, m := range reNodeRow.FindAllStringSubmatch(reNodeSGR.ReplaceAllString(listing, ""), -1) {
		out[m[1]] = m[2]
	}
	return out
}

// secondsShim puts a stand-in for astrolog first on PATH. It runs the real one
// for the caller and, for each call that lists positions, once more with the
// call's own argv and `=b0` after it (seconds on), appending that listing to a
// log: the call's chart at the precision the cast itself does not print. The
// second run happens inside the call, while a staged chart file still exists.
func secondsShim(t *testing.T) func() []string {
	t.Helper()
	real, err := exec.LookPath("astrolog")
	if err != nil {
		t.Skip("astrolog not on PATH; skipping live-binary test")
	}
	dir := t.TempDir()
	log := filepath.Join(dir, "seconds.log")
	script := `#!/bin/sh
out=$('` + real + `' "$@"); rc=$?
printf '%s\n' "$out"
case " $* " in
*" -aa "*) ;;
*" -R"*) { echo '@@'; '` + real + `' "$@" =b0 2>/dev/null; } >> '` + log + `' ;;
esac
exit $rc
`
	if err := os.WriteFile(filepath.Join(dir, "astrolog"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return func() []string {
		b, err := os.ReadFile(log)
		if err != nil {
			return nil
		}
		var out []string
		for _, part := range strings.Split(string(b), "@@\n") {
			if strings.TrimSpace(part) != "" {
				out = append(out, part)
			}
		}
		return out
	}
}

// shortHostDir is astrodir.Reduced with short folder names: the data folder is
// Astrolog's data folder with lines added to its astrolog.as, named in the
// environment as the launcher names it. Its names stay short because the Swiss
// Ephemeris refuses a search path past 255 characters, which the long names of
// a test's own temporary folders reach after other tests have run.
func shortHostDir(t *testing.T, lines ...string) {
	t.Helper()
	src := astrodir.Need(t)
	dst, err := os.MkdirTemp("", "h")
	if err != nil {
		t.Fatal(err)
	}
	ents, err := os.ReadDir(src)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range ents {
		if e.IsDir() {
			continue
		}
		if e.Name() != "astrolog.as" {
			if err := os.Symlink(filepath.Join(src, e.Name()), filepath.Join(dst, e.Name())); err != nil {
				t.Fatal(err)
			}
			continue
		}
		b, err := os.ReadFile(filepath.Join(src, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		b = append(b, []byte("\n"+strings.Join(lines, "\n")+"\n")...)
		if err := os.WriteFile(filepath.Join(dst, e.Name()), b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("ASTROLOG", dst)
	t.Setenv("SE_EPHE_PATH", dst)
	t.Setenv("ASTR800", os.Getenv("ASTR800"))
	t.Setenv("ASTR", os.Getenv("ASTR"))
	os.Unsetenv("ASTR800")
	os.Unsetenv("ASTR")
	cwd, err := os.MkdirTemp("", "c")
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(cwd)
}

// castFileChart casts the chart file at path the way the model casts a chart:
// the form the file states (the home form otherwise, the file's walk), through
// the model's own cast function.
func castFileChart(t *testing.T, path string) (nodes recipe.NodeForm, stated bool) {
	t.Helper()
	p, err := lookup.ReadChartFileIn(context.Background(), filepath.Dir(path), filepath.Base(path))
	if err != nil {
		t.Fatalf("Sextile does not read %s: %v", path, err)
	}
	m := timelineModel(t, Config{Accessible: true})
	form := recipe.NodesMean
	if p.NodesStated {
		form = p.Nodes
	}
	lens := m.lensFor(slotAt(m, 0))
	lens.Objects = recipe.ObjectPolicy{Baseline: recipe.Explicit, Selection: []int{16, 17, 18}, Nodes: form}
	reg, err := lookup.ObjectRegistry(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	lens.Registry = reg
	anchor := store.AnchorChart{When: p.When, Place: p.Place, Label: p.Label, Record: p.Record}
	if _, err := m.rows[surfaceText].spawn.cast.natal(context.Background(), m.probers, anchor, lens); err != nil {
		t.Fatalf("the model's cast of %s: %v", path, err)
	}
	return form, p.NodesStated
}

// N3: for every fixture file that states a form, the Node, the South Node and
// Lilith that Astrolog with stock settings casts from the file are those of the
// chart cast under the walked form, to the arc-second. The Sextile side runs
// under a host astrolog.as that sets `=Yn =Ynn`, which the call's own pins
// must outrank.
func TestFileNodeFormIsWhatAstrologCasts(t *testing.T) {
	real, err := exec.LookPath("astrolog")
	if err != nil {
		t.Skip("astrolog not on PATH; skipping live-binary test")
	}
	astrodir.Need(t)
	// The rows of N2 that state a form (Yn0, which states none, is not here).
	fixtures := []struct{ name, lines string }{
		{"mean and mean set explicitly", "_Yn _Ynn"},
		{"true and true", "=Yn _Ynn"},
		{"mean and natural", "_Yn =Ynn"},
		{"true and natural", "=Yn =Ynn"},
		{"=Yn alone", "=Yn"},
		{"=Ynn alone", "=Ynn"},
		{"-Yn", "-Yn"},
		{"/Yn", "/Yn"},
		{"-Yn twice", "-Yn -Yn"},
		{"=Yn then -Yn", "=Yn\n-Yn"},
		{"two that conflict on one line", "=Yn _Yn"},
		{"the same across lines", "=Yn =Ynn\n_Ynn"},
		{"Ynx", "=Ynx"},
	}
	const named = 13 // the rows of N2 that state a form
	if len(fixtures) < named {
		t.Fatalf("%d fixtures, N2 names %d that state a form", len(fixtures), named)
	}
	dir := t.TempDir()
	everyBody := []string{"-R0", "-R", "16", "17", "18"}
	signatures := map[string]bool{}
	paths := make([]string, len(fixtures))
	truths := make([]map[string]string, len(fixtures))
	for i, f := range fixtures {
		paths[i] = filepath.Join(dir, fmt.Sprintf("f%d.as", i))
		if err := os.WriteFile(paths[i], []byte(nodeFileHead+f.lines+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		out, err := exec.Command(real, append([]string{"-i", paths[i], "=b0", "=Yt", "-v"}, everyBody...)...).CombinedOutput()
		if err != nil {
			t.Fatalf("astrolog reading %q: %v\n%s", f.name, err, out)
		}
		truths[i] = nodeRows(string(out))
		if len(truths[i]) != 3 {
			t.Fatalf("Astrolog's own reading of %q printed %v", f.name, truths[i])
		}
		signatures[fmt.Sprint(truths[i])] = true
	}
	if len(signatures) < 4 {
		t.Errorf("the fixtures cast only %d different charts of the Nodes and Lilith, so the oracle cannot tell the forms apart", len(signatures))
	}
	// The Sextile side, under a host file that sets `=Yn =Ynn`, which the call's
	// own pins must outrank. One data folder for all, so the paths stay short
	// (the Swiss Ephemeris refuses a search path of 255 characters).
	shortHostDir(t, "=Yn", "=Ynn")
	read := secondsShim(t)
	for i, f := range fixtures {
		t.Run(f.name, func(t *testing.T) {
			before := len(read())
			if _, stated := castFileChart(t, paths[i]); !stated {
				t.Fatalf("Sextile read no form from %q", f.lines)
			}
			listings := read()
			if len(listings) <= before {
				t.Fatal("the model's cast made no positions call")
			}
			got := nodeRows(listings[len(listings)-1])
			for k, want := range truths[i] {
				if got[k] != want {
					t.Errorf("%s: Sextile's cast %q, Astrolog reading the file %q", k, got[k], want)
				}
			}
		})
	}
}

// N6: every call casts with the Swiss Ephemeris whatever the host settings say,
// so all four pairs cast as with stock settings under host files that set `_b`,
// `=bs`, `=bj`, `=bp` and `=bm`, and under one that sets `=Yn =Ynn`.
func TestFourPairsCastUnderHostileHostFiles(t *testing.T) {
	if _, err := exec.LookPath("astrolog"); err != nil {
		t.Skip("astrolog not on PATH; skipping live-binary test")
	}
	astrodir.Need(t)
	read := secondsShim(t) // once: a second stand-in would find the first as the program
	type rows map[string]string
	castAll := func(t *testing.T) map[recipe.NodeForm]rows {
		t.Helper()
		out := map[recipe.NodeForm]rows{}
		dir := t.TempDir()
		for _, f := range nodeForms {
			node, lilith := f.Words()
			path := filepath.Join(dir, node+"-"+lilith+".as")
			sw := f.Switches()
			if err := os.WriteFile(path, []byte(nodeFileHead+sw[0]+" "+sw[1]+"\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			before := len(read())
			castFileChart(t, path)
			listings := read()
			if len(listings) <= before {
				t.Fatalf("%v: the cast made no positions call", f)
			}
			out[f] = nodeRows(listings[len(listings)-1])
			if len(out[f]) != 3 {
				t.Fatalf("%v: rows %v", f, out[f])
			}
		}
		return out
	}
	// Each data folder is made in this test, not in a subtest, so its path stays
	// short (the Swiss Ephemeris refuses a search path of 255 characters).
	shortHostDir(t)
	stock := castAll(t)
	distinct := map[string]bool{}
	for _, r := range stock {
		distinct[fmt.Sprint(r)] = true
	}
	if len(distinct) != 4 {
		t.Fatalf("the four pairs cast %d different charts under stock settings: %v", len(distinct), stock)
	}
	for _, host := range [][]string{{"_b"}, {"=bs"}, {"=bj"}, {"=bp"}, {"=bm"}, {"=Yn", "=Ynn"}} {
		shortHostDir(t, host...)
		got := castAll(t)
		for f, want := range stock {
			if fmt.Sprint(got[f]) != fmt.Sprint(want) {
				t.Errorf("%v under a host that sets %v: %v, with stock settings %v", f, host, got[f], want)
			}
		}
	}
}
