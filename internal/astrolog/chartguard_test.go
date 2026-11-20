package astrolog

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// hostileCharts are chart files that try to make Astrolog act outside the
// chart: write a file, read another, run a stored command line. Each names its
// targets under root ({R}). want is the word the refusal must name.
func hostileCharts(root string) map[string]struct{ text, want string } {
	head := "@AI800  ; Astrolog chart info.\n-qb Jun 1 1990 16:30 DT 1E 4:53E 52:22N\n"
	r := func(s string) string { return strings.ReplaceAll(s, "{R}", root) }
	return map[string]struct{ text, want string }{
		"-o to a path":                       {r(head + "-o {R}/written.as\n"), "-o"},
		"-o0":                                {r(head + "-o0 {R}/written.as\n"), "-o0"},
		"-os":                                {r(head + "-os {R}/written.txt\n"), "-os"},
		"/o":                                 {r(head + "/o {R}/written.as\n"), "/o"},
		"=o":                                 {r(head + "=o {R}/written.as\n"), "=o"},
		"a bare word o":                      {r(head + "o {R}/written.as\n"), "o"},
		"-i of another file":                 {r(head + "-i {R}/other.as\n"), "-i"},
		"a macro that holds -o":              {r(head + "-M0 1 \"-o {R}/written.as\"\n-M 1\n"), "-M0"},
		"an AstroExpression":                 {r(head + "-~1 \"Switch(\\\"-o {R}/written.as\\\")\"\n"), "-~1"},
		"a stored command line":              {r(head + "-Yq1 \"-o {R}/written.as\"\n"), "-Yq1"},
		"a switch after a quoted -zi":        {r(head + "-zi \"A\" \"B\" -o {R}/written.as\n"), "-o"},
		"a switch after -qb's eight":         {r("@AI800\n-qb Jun 1 1990 16:30 DT 1E 4:53E 52:22N -o {R}/written.as\n"), "-o"},
		"a second header":                    {r(head + "@AI800\n-o {R}/written.as\n"), "@AI800"},
		"-o behind a display switch":         {r(head + "-v -o {R}/written.as\n"), "-o"},
		"-o behind an object switch":         {r(head + "-R0 1 2 -o {R}/written.as\n"), "-o"},
		"a relationship reading files":       {r(head + "-r0 {R}/other.as {R}/other.as\n"), "-r0"},
		"a switch on the header line":        {r("@AI800 -o {R}/written.as\n-qb Jun 1 1990 16:30 DT 1E 4:53E 52:22N\n"), "-o"},
		"an expression as an argument":       {r(head + "-c \"~Switch 1\"\n"), "-c"},
		"an argument past Astrolog's buffer": {head + "-zl " + strings.Repeat("1", 600) + " 52N\n", "-zl"},
	}
}

// hostileControls is, for each hostile chart, the file that shows its harm
// when the guard is off: the chart itself, or, where the harm needs a setting
// the host holds (a command macro), the chart with that setting written in.
// crashes is a chart whose harm is that Astrolog dies reading it.
func hostileControls(root string) (writes map[string]string, crashes map[string]string) {
	writes, crashes = map[string]string{}, map[string]string{}
	for name, c := range hostileCharts(root) {
		writes[name] = c.text
	}
	head := "@AI800\n-qb Jun 1 1990 16:30 DT 1E 4:53E 52:22N\n"
	writes["an expression as an argument"] = "@AI800\n-M0 1 \"-o " + root + "/written.as\"\n" + head[len("@AI800\n"):] + "-c \"~Switch 1\"\n"
	crashes["an argument past Astrolog's buffer"] = writes["an argument past Astrolog's buffer"]
	delete(writes, "an argument past Astrolog's buffer")
	return writes, crashes
}

// snapshot is every file under root with its size and modification time.
func snapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	got := map[string]string{}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		got[p] = info.ModTime().Format(time.RFC3339Nano) + " " + strconv.FormatInt(info.Size(), 10)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return got
}

// No chart file can make Astrolog write or read a file: reading it (the pick,
// the restore) and retitling it change nothing under the watched root but
// what the test itself wrote, and a refusal names the switch.
func TestHostileChartFileActsOnNothing(t *testing.T) {
	needAstrolog(t)
	for name, c := range hostileCharts("{R}") {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			text := strings.ReplaceAll(c.text, "{R}", root)
			other := "@AI800\n-qb Jun 1 1990 16:30 DT 1E 4:53E 52:22N\n=o " + root + "/written-by-other.as\n"
			if err := os.WriteFile(filepath.Join(root, "other.as"), []byte(other), 0o600); err != nil {
				t.Fatal(err)
			}
			src := filepath.Join(root, "hostile.as")
			if err := os.WriteFile(src, []byte(text), 0o600); err != nil {
				t.Fatal(err)
			}
			before := snapshot(t, root)
			_, readErr := ResolveChartBytes(context.Background(), []byte(text), src)
			_, retitleErr := RetitleRecord(context.Background(), src, "T", "P")
			after := snapshot(t, root)
			for p, v := range after {
				if before[p] != v {
					t.Errorf("%s was created or changed by reading the chart", p)
				}
			}
			for _, err := range []error{readErr, retitleErr} {
				var refused *ChartRefused
				if !errors.As(err, &refused) || refused.Switch != c.want {
					t.Errorf("want a refusal naming %q, got %v", c.want, err)
				}
			}
		})
	}
}

// The control: with the guard switched off, every hostile file does act
// outside the chart (it writes, or Astrolog dies reading it), so the test
// above sees what the guard stops for each one.
func TestHostileChartFileControlWithoutTheGuard(t *testing.T) {
	needAstrolog(t)
	prev := guardChart
	t.Cleanup(func() { guardChart = prev })
	guardChart = func(text string) (string, []FileNote, error) { return text, nil, nil }
	writes, crashes := hostileControls("{R}")
	for name, text := range writes {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			text := strings.ReplaceAll(text, "{R}", root)
			other := "@AI800\n-qb Jun 1 1990 16:30 DT 1E 4:53E 52:22N\n=o " + root + "/written.as\n"
			if err := os.WriteFile(filepath.Join(root, "other.as"), []byte(other), 0o600); err != nil {
				t.Fatal(err)
			}
			src := filepath.Join(root, "hostile.as")
			if err := os.WriteFile(src, []byte(text), 0o600); err != nil {
				t.Fatal(err)
			}
			before := snapshot(t, root)
			_, _ = ResolveChartBytes(context.Background(), []byte(text), src)
			if after := snapshot(t, root); len(after) == len(before) {
				t.Errorf("without the guard nothing was written either: the test cannot see the guard")
			}
		})
	}
	for name, text := range crashes {
		t.Run(name, func(t *testing.T) {
			_, err := ResolveChartBytes(context.Background(), []byte(text), filepath.Join(t.TempDir(), "hostile.as"))
			if err == nil || !strings.Contains(err.Error(), "crashed") {
				t.Errorf("without the guard Astrolog read it (%v): the test cannot see the guard", err)
			}
		})
	}
}

// A file Sextile writes, and every file Astrolog's own writer makes, passes the
// guard unchanged in what it casts.
func TestGuardKeepsAChartFilesOwnLines(t *testing.T) {
	text := "@AI800  ; Astrolog chart info.\n; a comment\n-qb Dec 27 1571 14:37 ST LMT   8:52E 48:45N\n-zi \"Johannes Kepler\" \"Weil der Stadt, Germany\"\n-c Placidus\n=s 0.88\n_s\n-c3 1\n;sextile-id: abc\n"
	got, notes, err := GuardChartFile(text)
	if err != nil {
		t.Fatal(err)
	}
	want := "@AI800  ; Astrolog chart info.\n-qb Dec 27 1571 14:37 ST LMT   8:52E 48:45N\n-zi \"Johannes Kepler\" \"Weil der Stadt, Germany\"\n-c Placidus\n=s 0.88\n_s\n-c3 1\n"
	if got != want || len(notes) != 0 {
		t.Errorf("guarded copy:\n%s\nwant\n%s\nnotes %v", got, want, notes)
	}
	_, notes, err = GuardChartFile(text + "_Yn\n-R0 1 2\n=b0\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(notes) != 1 || notes[0].Class != SwitchLens {
		t.Errorf("notes %+v, want the object switch alone: the Node switch is taken out and not noted", notes)
	}
}

// A centre or a derived chart is a different chart, and the file does not
// open; the refusal says what the file asks for, in the operator's words.
func TestGuardRefusesACentreOrDerivedChart(t *testing.T) {
	head := "@AI800\n-qb Jun 1 1990 16:30 DT 1E 4:53E 52:22N\n"
	for _, tc := range []struct{ line, sw, asks string }{
		{"-h", "-h", "a heliocentric chart"},
		{"-h 3", "-h", "a heliocentric chart"},
		{"-x 5", "-x", "a harmonic chart"},
		{"-9", "-9", "navamsa positions"},
		{"=sr", "=sr", "right ascension positions"},
		{"-YT", "-YT", "true positions in space"},
	} {
		_, _, err := GuardChartFile(head + tc.line + "\n")
		var refused *ChartRefused
		if !errors.As(err, &refused) || refused.Switch != tc.sw || refused.Asks != tc.asks {
			t.Errorf("%q: %+v (%v), want a refusal of %s asking for %q", tc.line, refused, err, tc.sw, tc.asks)
		}
	}
}
