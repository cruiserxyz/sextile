// All chart values reachable from this file are constructed synthetic test inputs.
package lookup

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cruiserxyz/sextile/internal/astrolog"
	"github.com/cruiserxyz/sextile/internal/store"
)

// Verifiers for a rename's re-apply step (K1 to K6a, K11, K12 of transfer
// specs/sextile-release/variants/chart-rewrite-reapplies-lines.md). Each runs
// live Astrolog on a chart file written into a fresh library.

const lmtQB = "-qb Jan 3 1890 14:00 ST -0:19:32 4:53:28E 52:22N\n"

// fullChart is a chart file holding what Astrolog's writer drops: a note, a
// house line and a -qb line with zone and coordinate seconds.
const fullChart = "@AI780  ; Astrolog chart info.\n" +
	"; a note about this chart\n" +
	lmtQB +
	"-zi \"Old Title\" \"Amsterdam\"\n" +
	"; a second note\n" +
	"-c 1\n"

// renamed renames the chart in name, holding body, to title and place, and
// returns the file's bytes after.
func renamed(t *testing.T, name, body, title, place string) (string, string, error) {
	t.Helper()
	astrologOnPath(t)
	dir := useLibraryDir(t)
	writeChartFile(t, dir, name, body)
	lib, err := LoadChartLibrary()
	if err != nil {
		t.Fatal(err)
	}
	row := rowByFile(t, lib, name)
	_, err = RetitleChart(context.Background(), dir, row, seenOf(t, dir, row), title, place, nextStamp())
	return dir, fileBytes(t, filepath.Join(dir, name)), err
}

func astrologOnPath(t *testing.T) {
	t.Helper()
	if err := astrolog.Available(); err != nil {
		t.Skip("astrolog is not on PATH")
	}
}

// K1
func TestRetitleChangesOnlyTheNameLine(t *testing.T) {
	_, got, err := renamed(t, "Anna.as", fullChart, "New Title", "Amsterdam, NL")
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Replace(fullChart, "-zi \"Old Title\" \"Amsterdam\"", "-zi \"New Title\" \"Amsterdam, NL\"", 1)
	if got != want {
		t.Errorf("the rename changed more than the -zi line:\ngot\n%s\nwant\n%s", got, want)
	}
	t.Run("the -zi line ends the file without a newline", func(t *testing.T) {
		src := "@AI780  ; Astrolog chart info.\n" + lmtQB + "-zi \"Old Title\" \"Amsterdam\""
		_, got, err := renamed(t, "Anna.as", src, "New Title", "Amsterdam, NL")
		if err != nil {
			t.Fatal(err)
		}
		if want := strings.Replace(src, "\"Old Title\" \"Amsterdam\"", "\"New Title\" \"Amsterdam, NL\"", 1); got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})
	t.Run("CRLF line ends", func(t *testing.T) {
		src := strings.ReplaceAll(fullChart, "\n", "\r\n")
		_, got, err := renamed(t, "Anna.as", src, "New Title", "Amsterdam, NL")
		if err != nil {
			t.Fatal(err)
		}
		if want := strings.Replace(src, "\"Old Title\" \"Amsterdam\"", "\"New Title\" \"Amsterdam, NL\"", 1); got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})
}

// K2
func TestRetitleInsertsMissingNameLine(t *testing.T) {
	src := strings.Replace(fullChart, "-zi \"Old Title\" \"Amsterdam\"\n", "", 1)
	_, got, err := renamed(t, "Anna.as", src, "New Title", "Amsterdam, NL")
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Replace(src, lmtQB, lmtQB+"-zi \"New Title\" \"Amsterdam, NL\"\n", 1)
	if got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
}

// K3
func TestRetitleKeepsCommentLines(t *testing.T) {
	_, got, err := renamed(t, "Anna.as", fullChart, "New Title", "Amsterdam, NL")
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.SplitAfter(got, "\n")
	for i, want := range strings.SplitAfter(fullChart, "\n") {
		if strings.HasPrefix(want, ";") && (i >= len(lines) || lines[i] != want) {
			t.Errorf("comment line %d %q is not in its place", i+1, want)
		}
	}
}

// K4
func TestRetitleKeepsHouseLine(t *testing.T) {
	dir, got, err := renamed(t, "Anna.as", fullChart, "New Title", "Amsterdam, NL")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "\n-c 1\n") {
		t.Errorf("the -c line is gone:\n%s", got)
	}
	before := filepath.Join(t.TempDir(), "before.as")
	if err := os.WriteFile(before, []byte(fullChart), 0o600); err != nil {
		t.Fatal(err)
	}
	a, err := astrolog.ResolveChartFile(context.Background(), before)
	if err != nil {
		t.Fatal(err)
	}
	b, err := astrolog.ResolveChartFile(context.Background(), filepath.Join(dir, "Anna.as"))
	if err != nil {
		t.Fatal(err)
	}
	if !a.HasHouses || a.Houses != b.Houses || a.HasHouses != b.HasHouses {
		t.Errorf("houses: before %v (%v), after %v (%v)", a.Houses, a.HasHouses, b.Houses, b.HasHouses)
	}
}

// K5
func TestRetitleKeepsSeconds(t *testing.T) {
	_, got, err := renamed(t, "Anna.as", fullChart, "New Title", "Amsterdam, NL")
	if err != nil {
		t.Fatalf("a chart with seconds was refused: %v", err)
	}
	want := strings.Replace(fullChart, "-zi \"Old Title\" \"Amsterdam\"", "-zi \"New Title\" \"Amsterdam, NL\"", 1)
	if got != want || strings.Count(got, "-qb ") != 1 || !strings.Contains(got, "\n"+lmtQB) {
		t.Errorf("the -qb line is not the source's, in its place:\n%s", got)
	}
}

// corruptReapplied changes from to to in the re-applied record, before the
// read-back.
func corruptReapplied(t *testing.T, pairs ...string) *bool {
	t.Helper()
	prev := beforeVerify
	t.Cleanup(func() { beforeVerify = prev })
	ran := false
	beforeVerify = func(rec []byte) []byte {
		b := string(rec)
		for i := 0; i < len(pairs); i += 2 {
			if !strings.Contains(b, pairs[i]) {
				t.Fatalf("the corruption no longer applies: %q not in %q", pairs[i], b)
			}
			b = strings.Replace(b, pairs[i], pairs[i+1], 1)
		}
		ran = true
		return []byte(b)
	}
	return &ran
}

// renamedRefused checks that err is a read-back refusal and that nothing was
// written: the file is unchanged, no backup, no temp file.
func renamedRefused(t *testing.T, dir, got string, err error) {
	t.Helper()
	if !errors.Is(err, ErrBadReadBack) {
		t.Fatalf("err = %v, want a read-back refusal", err)
	}
	if got != fullChart {
		t.Error("the chart changed")
	}
	if noBackup(t, dir) != "" {
		t.Error("a refused write made a backup")
	}
	noTempLeft(t, dir)
}

// K6
func TestRetitleVerifiesTheReappliedFile(t *testing.T) {
	for _, tc := range []struct{ name, from, to string }{
		{"moment", "14:00 ST", "14:07 ST"},
		{"coordinates", "4:53:28E", "4:53:29E"},
		{"houses", "-c 1\n", "-c 3\n"},
		{"title", "\"New Title\"", "\"Somebody Else\""},
		{"place", "\"Amsterdam, NL\"", "\"Elsewhere\""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ran := corruptReapplied(t, tc.from, tc.to)
			dir, got, err := renamed(t, "Anna.as", fullChart, "New Title", "Amsterdam, NL")
			if !*ran {
				t.Fatal("the seam never ran")
			}
			renamedRefused(t, dir, got, err)
			if !strings.Contains(err.Error(), tc.name) {
				t.Errorf("err = %v, want it to name the %s", err, tc.name)
			}
		})
	}
}

// K6a
func TestRetitleRefusalNamesFields(t *testing.T) {
	ran := corruptReapplied(t, "14:00 ST", "14:07 ST", "52:22N", "52:23N", "-c 1\n", "-c 3\n",
		"\"New Title\"", "\"Somebody Else\"", "\"Amsterdam, NL\"", "\"Elsewhere\"")
	dir, got, err := renamed(t, "Anna.as", fullChart, "New Title", "Amsterdam, NL")
	if !*ran {
		t.Fatal("the seam never ran")
	}
	renamedRefused(t, dir, got, err)
	msg := err.Error()
	if !strings.Contains(msg, "(moment, coordinates, houses, title, place)") || !strings.HasSuffix(msg, "Nothing was written") {
		t.Errorf("err = %q", msg)
	}
}

// K11
func TestRetitleWritesNothingWhenAstrologFails(t *testing.T) {
	t.Run("fails", func(t *testing.T) {
		astrologOnPath(t)
		dir := useLibraryDir(t)
		writeChartFile(t, dir, "Anna.as", fullChart)
		lib, err := LoadChartLibrary()
		if err != nil {
			t.Fatal(err)
		}
		row := rowByFile(t, lib, "Anna.as")
		recordingAstrolog(t, "=o")
		if _, err := RetitleChart(context.Background(), dir, row, seenOf(t, dir, row), "New Title", "Amsterdam, NL", nextStamp()); err == nil {
			t.Fatal("a failed Astrolog write was not reported")
		}
		renamedUnchanged(t, dir)
	})
	t.Run("missing", func(t *testing.T) {
		dir := useLibraryDir(t)
		writeChartFile(t, dir, "Anna.as", fullChart)
		lib, err := LoadChartLibrary()
		if err != nil {
			t.Fatal(err)
		}
		row := rowByFile(t, lib, "Anna.as")
		t.Setenv("PATH", t.TempDir())
		if _, err := RetitleChart(context.Background(), dir, row, seenOf(t, dir, row), "New Title", "Amsterdam, NL", nextStamp()); err == nil {
			t.Fatal("a missing Astrolog was not reported")
		}
		renamedUnchanged(t, dir)
	})
}

func renamedUnchanged(t *testing.T, dir string) {
	t.Helper()
	if fileBytes(t, filepath.Join(dir, "Anna.as")) != fullChart {
		t.Error("the chart changed")
	}
	if noBackup(t, dir) != "" {
		t.Error("a failed write made a backup")
	}
	noTempLeft(t, dir)
}

// K12: the fingerprint guard and the control-character refusals are as before;
// every rename keeps its own dated copy (the door spec's O12).
func TestRetitleGuardsUnchanged(t *testing.T) {
	astrologOnPath(t)
	dir := useLibraryDir(t)
	writeChartFile(t, dir, "Anna.as", fullChart)
	lib, err := LoadChartLibrary()
	if err != nil {
		t.Fatal(err)
	}
	row := rowByFile(t, lib, "Anna.as")
	seen := seenOf(t, dir, row)
	ctx := context.Background()

	if _, err := RetitleChart(ctx, dir, row, seen+"x", "New Title", "Amsterdam, NL", nextStamp()); !errors.Is(err, store.ErrChartChanged) {
		t.Errorf("a stale fingerprint: err = %v, want ErrChartChanged", err)
	}
	for _, tc := range []struct{ title, place string }{{"New\tTitle", "Amsterdam"}, {"New Title", "Amster\ndam"}} {
		if _, err := RetitleChart(ctx, dir, row, seen, tc.title, tc.place, nextStamp()); !errors.Is(err, astrolog.ErrLabelHasControlChar) {
			t.Errorf("%q / %q: err = %v, want ErrLabelHasControlChar", tc.title, tc.place, err)
		}
	}
	renamedUnchanged(t, dir)

	first, err := RetitleChart(ctx, dir, row, seen, "New Title", "Amsterdam, NL", nextStamp())
	if err != nil {
		t.Fatal(err)
	}
	if first.Backup == "" || fileBytes(t, first.Backup) != fullChart {
		t.Fatalf("the first rename did not keep the original (backup %q)", first.Backup)
	}
	lib, err = LoadChartLibrary()
	if err != nil {
		t.Fatal(err)
	}
	row = rowByFile(t, lib, "Anna.as")
	second, err := RetitleChart(ctx, dir, row, seenOf(t, dir, row), "Third Title", "Amsterdam, NL", nextStamp())
	if err != nil {
		t.Fatal(err)
	}
	if second.Backup == first.Backup || fileBytes(t, first.Backup) != fullChart {
		t.Errorf("a second rename did not keep its own dated copy (%q, %q)", first.Backup, second.Backup)
	}
}

// renameRefusesRepeatedNameLine is K8 through the rename: a repeated -zi
// line writes nothing.
func renameRefusesRepeatedNameLine(t *testing.T) {
	src := fullChart + "-zi \"Other\" \"Elsewhere\"\n"
	dir, got, err := renamed(t, "Anna.as", src, "New Title", "Amsterdam, NL")
	if !errors.Is(err, ErrReapply) || err.Error() != "Anna.as has more than one -zi line. Nothing was written." {
		t.Fatalf("err = %v", err)
	}
	if got != src {
		t.Error("the chart changed")
	}
	if noBackup(t, dir) != "" {
		t.Error("a refused write made a backup")
	}
	noTempLeft(t, dir)
}

// renameRefusesUnclearAstrologOutput is K10 through the rename: Astrolog's
// record with no -zi line, or two, writes nothing.
func renameRefusesUnclearAstrologOutput(t *testing.T) {
	for name, edit := range map[string]func(string) string{
		"none": func(b string) string {
			var keep []string
			for _, l := range strings.SplitAfter(b, "\n") {
				if !strings.HasPrefix(l, "-zi") {
					keep = append(keep, l)
				}
			}
			return strings.Join(keep, "")
		},
		"two": func(b string) string { return b + "-zi \"Other\" \"Elsewhere\"\n" },
	} {
		t.Run(name, func(t *testing.T) {
			prev := afterAstrologWrite
			t.Cleanup(func() { afterAstrologWrite = prev })
			afterAstrologWrite = func(b []byte) []byte { return []byte(edit(string(b))) }
			dir, got, err := renamed(t, "Anna.as", fullChart, "New Title", "Amsterdam, NL")
			if !errors.Is(err, ErrReapply) || err.Error() != "Astrolog did not write the -zi line as expected. Nothing was written." {
				t.Fatalf("err = %v", err)
			}
			if got != fullChart {
				t.Error("the chart changed")
			}
			if noBackup(t, dir) != "" {
				t.Error("a refused write made a backup")
			}
			noTempLeft(t, dir)
		})
	}
}

// A chart file that sets its moment with -qa or -qj renames like any other:
// only its -zi line changes.
func TestRenameAChartWithoutAQBLine(t *testing.T) {
	for _, src := range []string{
		"@AI800  ; Astrolog chart info.\n-qa Jun 1 1990 16:30 1E 4:53E 52:22N\n-zi \"Moment by qa\" \"Amsterdam, Netherlands\"\n",
		"@AI800  ; Astrolog chart info.\n-qj 2448044.1458333\n-zl 4:53E 52:22N\n",
	} {
		_, got, err := renamed(t, "Anna.as", src, "New Title", "Amsterdam, NL")
		if err != nil {
			t.Fatalf("%q: %v", src, err)
		}
		var want string
		if strings.Contains(src, "-zi") {
			want = strings.Replace(src, `-zi "Moment by qa" "Amsterdam, Netherlands"`, `-zi "New Title" "Amsterdam, NL"`, 1)
		} else {
			want = strings.Replace(src, "-qj 2448044.1458333\n", "-qj 2448044.1458333\n-zi \"New Title\" \"Amsterdam, NL\"\n", 1)
		}
		if got != want {
			t.Errorf("renamed to\n%s\nwant\n%s", got, want)
		}
	}
}
