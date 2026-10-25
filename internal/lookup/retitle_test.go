// All chart values reachable from this file are constructed synthetic test inputs.
package lookup

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/cruiserxyz/sextile/internal/astrolog"
	"github.com/cruiserxyz/sextile/internal/store"
)

// Verifiers for the specification item 3's write path: C3.7, C3.8, C3.15
// and the § Failure modes rows. Every one runs live astrolog on
// a COPY of a fixture in a temp directory.

// recordingAstrolog puts a shim first on PATH that logs each invocation's
// arguments and then runs the real astrolog, or fails instead when an argument
// equals failOn. It returns the recorded invocations.
//
// A PATH shim rather than a seam, for the reason astrologShim gives: the
// package spawns the bare name, and C22 forbids a hidden way around that.
func recordingAstrolog(t *testing.T, failOn string) func() [][]string {
	t.Helper()
	real, err := exec.LookPath("astrolog")
	if err != nil {
		t.Skip("astrolog is not on PATH")
	}
	dir := t.TempDir()
	log := filepath.Join(dir, "invocations")
	script := "#!/bin/sh\n" +
		"for a in \"$@\"; do printf '%s\\n' \"$a\" >> " + strconv.Quote(log) + "; done\n" +
		"printf '%s\\n' '--end--' >> " + strconv.Quote(log) + "\n"
	if failOn != "" {
		script += "for a in \"$@\"; do [ \"$a\" = " + strconv.Quote(failOn) + " ] && { echo 'Astrolog: injected failure'; exit 3; }; done\n"
	}
	script += "exec " + strconv.Quote(real) + " \"$@\"\n"
	if err := os.WriteFile(filepath.Join(dir, "astrolog"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return func() [][]string {
		b, _ := os.ReadFile(log)
		var calls [][]string
		var cur []string
		for _, l := range strings.Split(strings.TrimSuffix(string(b), "\n"), "\n") {
			if l == "--end--" {
				// C1: every spawn starts with -YQ 0 and the hook-clear band;
				// the tests are about the call's own argv.
				if own, ok := stripSpawnPrefix(cur); !ok {
					t.Errorf("spawn argv %q does not start with -YQ 0 and the hook clears (C1)", cur[:min(len(cur), 6)])
				} else {
					cur = own
				}
				calls = append(calls, cur)
				cur = nil
				continue
			}
			cur = append(cur, l)
		}
		return calls
	}
}

// untitledEquinox copies Equinox-2026-09.as into a fresh library and returns its row.
func untitledEquinox(t *testing.T) (string, store.NamedChart) {
	t.Helper()
	dir := useLibraryDir(t)
	copyFixture(t, dir, "Equinox-2026-09.as")
	lib, err := LoadChartLibrary()
	if err != nil {
		t.Fatal(err)
	}
	return dir, rowByFile(t, lib, "Equinox-2026-09.as")
}

func fileBytes(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// noTempLeft fails if any chart temp file survives in dir.
func noTempLeft(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name() == ".sextile" {
			continue // the backup folder: its own entries are checked below
		}
		if strings.HasPrefix(e.Name(), ".") || strings.Contains(e.Name(), "sextile-tmp") {
			t.Errorf("a temp file was left behind: %s", e.Name())
		}
	}
	backups, err := os.ReadDir(filepath.Join(dir, store.ChartBackupFolderRel()))
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	for _, e := range backups {
		if strings.HasPrefix(e.Name(), ".") || strings.Contains(e.Name(), "sextile-tmp") {
			t.Errorf("a temp file was left behind in the backup folder: %s", e.Name())
		}
	}
}

// C3.7, with the § Failure modes cases that share its setup.
func TestRetitleChartFile(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		dir, row := untitledEquinox(t)
		calls := recordingAstrolog(t, "")
		original := fileBytes(t, filepath.Join(dir, "Equinox-2026-09.as"))

		got, err := RetitleChart(context.Background(), dir, row, seenOf(t, dir, row), "Constructed One", "Bremen, Germany")
		if err != nil {
			t.Fatal(err)
		}
		var write []string
		for _, c := range calls() {
			if len(c) > 0 && c[0] == "-i" && len(c) == 7 && c[5] == "=o" {
				write = c
			}
		}
		if write == nil {
			t.Fatalf("no `-i <file> -zi <title> <place> =o <tmp>` call among %q", calls())
		}
		if write[1] != filepath.Join(dir, "Equinox-2026-09.as") || write[2] != "-zi" || write[3] != "Constructed One" ||
			write[4] != "Bremen, Germany" || filepath.Dir(write[6]) != dir {
			t.Errorf("write argv = %q, want -i <the chart> -zi <title> <place> =o <a temp file in its directory>", write)
		}
		// The temp file Astrolog wrote IS the chart now, byte for byte: the
		// expected bytes are what Astrolog writes for the same argv into a
		// file of our own.
		want := filepath.Join(t.TempDir(), "want.as")
		probe := filepath.Join(t.TempDir(), "Equinox-2026-09.as")
		if err := os.WriteFile(probe, []byte(original), 0o600); err != nil {
			t.Fatal(err)
		}
		if out, err := exec.Command("astrolog", "-i", probe, "-zi", "Constructed One", "Bremen, Germany", "=o", want).CombinedOutput(); err != nil {
			t.Fatalf("reference write: %v %s", err, out)
		}
		if g, w := fileBytes(t, filepath.Join(dir, "Equinox-2026-09.as")), fileBytes(t, want); g != w {
			t.Errorf("the chart is not Astrolog's record byte for byte.\n got: %q\nwant: %q", g, w)
		}
		if g := fileBytes(t, filepath.Join(dir, store.ChartBackupRel("Equinox-2026-09.as"))); g != original {
			t.Errorf("backup = %q, want the original", g)
		}
		if got.Name != "Constructed One" || got.Backup != store.ChartBackupRel("Equinox-2026-09.as") || got.ID != row.ID {
			t.Errorf("result = %+v", got)
		}
		noTempLeft(t, dir)
	})

	t.Run("astrolog fails", func(t *testing.T) {
		dir, row := untitledEquinox(t)
		recordingAstrolog(t, "=o")
		original := fileBytes(t, filepath.Join(dir, "Equinox-2026-09.as"))
		_, err := RetitleChart(context.Background(), dir, row, seenOf(t, dir, row), "Constructed One", "Bremen, Germany")
		if err == nil || !strings.Contains(err.Error(), "injected failure") {
			t.Fatalf("err = %v, want Astrolog's own first line", err)
		}
		if fileBytes(t, filepath.Join(dir, "Equinox-2026-09.as")) != original {
			t.Error("the chart changed after a failed write")
		}
		if _, err := os.Stat(filepath.Join(dir, store.ChartBackupRel("Equinox-2026-09.as"))); !os.IsNotExist(err) {
			t.Error("a failed write left a backup")
		}
		noTempLeft(t, dir)
	})

	t.Run("read-only dir", func(t *testing.T) {
		if os.Geteuid() == 0 {
			t.Skip("root ignores mode bits")
		}
		dir, row := untitledEquinox(t)
		original := fileBytes(t, filepath.Join(dir, "Equinox-2026-09.as"))
		readOnlyChartDir(t, dir)
		if _, err := RetitleChart(context.Background(), dir, row, seenOf(t, dir, row), "Constructed One", "Bremen, Germany"); err == nil {
			t.Fatal("a write into a read-only directory succeeded")
		}
		if fileBytes(t, filepath.Join(dir, "Equinox-2026-09.as")) != original {
			t.Error("the chart changed")
		}
		if _, err := os.Stat(filepath.Join(dir, store.ChartBackupRel("Equinox-2026-09.as"))); !os.IsNotExist(err) {
			t.Error("a backup was created")
		}
	})

	t.Run("double quote", func(t *testing.T) {
		dir, row := untitledEquinox(t)
		got, err := RetitleChart(context.Background(), dir, row, seenOf(t, dir, row), `Constructed "Q" One`, "Bremen, Germany")
		if err != nil {
			t.Fatal(err)
		}
		if got.Name != "Constructed 'Q' One" {
			t.Errorf("Name = %q, want the title as Astrolog wrote it", got.Name)
		}
		if !strings.Contains(fileBytes(t, filepath.Join(dir, "Equinox-2026-09.as")), `-zi "Constructed 'Q' One" "Bremen, Germany"`) {
			t.Errorf("file = %q", fileBytes(t, filepath.Join(dir, "Equinox-2026-09.as")))
		}
	})

	t.Run("second retitle keeps the first backup", func(t *testing.T) {
		dir, row := untitledEquinox(t)
		original := fileBytes(t, filepath.Join(dir, "Equinox-2026-09.as"))
		if _, err := RetitleChart(context.Background(), dir, row, seenOf(t, dir, row), "Constructed One", "Bremen, Germany"); err != nil {
			t.Fatal(err)
		}
		got, err := RetitleChart(context.Background(), dir, row, seenOf(t, dir, row), "Constructed Two", "Bremen, Germany")
		if err != nil {
			t.Fatal(err)
		}
		if got.Backup != "" {
			t.Errorf("Backup = %q on the second write, want \"\" (the first is kept)", got.Backup)
		}
		if fileBytes(t, filepath.Join(dir, store.ChartBackupRel("Equinox-2026-09.as"))) != original {
			t.Error("the second retitle replaced the first backup")
		}
	})

}

// the specification C3: a file holding lines Astrolog does not write
// back is retitled like any other. The first backup keeps every original byte,
// and the record reads back to the same moment and place (C3.8 is unchanged).
// Live Astrolog.
func TestRetitleKeepsTheOriginalOfAFullFile(t *testing.T) {
	for _, tc := range []struct{ name, extra string }{
		{"house system", "-c 3\n"},
		{"switch", "-YXg 1\n"},
		{"note", "; private note\n"},
		{"all three", "-c 3\n-YXg 1\n; private note\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := useLibraryDir(t)
			copyFixture(t, dir, "Equinox-2026-09.as")
			path := filepath.Join(dir, "Equinox-2026-09.as")
			original := fileBytes(t, path) + tc.extra
			writeChartFile(t, dir, "Equinox-2026-09.as", original)
			lib, err := LoadChartLibrary()
			if err != nil {
				t.Fatal(err)
			}
			row := rowByFile(t, lib, "Equinox-2026-09.as")
			got, err := RetitleChart(context.Background(), dir, row, seenOf(t, dir, row), "Constructed Full", "Bremen, Germany")
			if err != nil {
				t.Fatalf("a full file was refused: %v", err)
			}
			if got.Backup == "" || fileBytes(t, filepath.Join(dir, got.Backup)) != original {
				t.Errorf("the backup does not hold the original bytes (backup %q)", got.Backup)
			}
			if got.Name != "Constructed Full" || got.PlaceLabel != "Bremen, Germany" {
				t.Errorf("read back %q / %q", got.Name, got.PlaceLabel)
			}
			sameMomentAndPlace(t, filepath.Join(dir, got.Backup), path)
		})
	}
	t.Run("fixture", func(t *testing.T) {
		dir := useLibraryDir(t)
		copyFixture(t, dir, "FullMoon-2026-02.as")
		original := fileBytes(t, filepath.Join(dir, "FullMoon-2026-02.as"))
		lib, _ := LoadChartLibrary()
		row := rowByFile(t, lib, "FullMoon-2026-02.as")
		got, err := RetitleChart(context.Background(), dir, row, seenOf(t, dir, row), "Constructed", "Hamburg")
		if err != nil {
			t.Fatalf("FullMoon-2026-02.as was refused: %v", err)
		}
		if fileBytes(t, filepath.Join(dir, got.Backup)) != original {
			t.Error("the backup does not hold the original bytes")
		}
		sameMomentAndPlace(t, filepath.Join(dir, got.Backup), filepath.Join(dir, "FullMoon-2026-02.as"))
	})
}

// sameMomentAndPlace resolves both files through Astrolog, independently of
// the write path's own read-back, and requires the same moment and place.
func sameMomentAndPlace(t *testing.T, original, rewritten string) {
	t.Helper()
	a, err := astrolog.ResolveChartFile(context.Background(), original)
	if err != nil {
		t.Fatalf("resolve the original: %v", err)
	}
	b, err := astrolog.ResolveChartFile(context.Background(), rewritten)
	if err != nil {
		t.Fatalf("resolve the rewrite: %v", err)
	}
	if a.JD != b.JD || a.Lon != b.Lon || a.Lat != b.Lat {
		t.Errorf("the rewrite moved the chart: JD %v -> %v, lon %v -> %v, lat %v -> %v", a.JD, b.JD, a.Lon, b.Lon, a.Lat, b.Lat)
	}
}

// C3.8: a record that reads back to a different moment, title, place or
// coordinates is never swapped in; a title holding `"` passes once read as
// Astrolog rewrites it.
func TestRetitleRefusesABadReadBack(t *testing.T) {
	for _, tc := range []struct {
		name, from, to string
	}{
		{"moment", "Sep 23 2026  2:06", "Sep 23 2026  2:13"},
		{"title", `"Constructed One"`, `"Somebody Else"`},
		{"place", `"Bremen, Germany"`, `"Elsewhere"`},
		{"coordinates", "8:48E 53:04N", "8:49E 53:04N"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir, row := untitledEquinox(t)
			original := fileBytes(t, filepath.Join(dir, "Equinox-2026-09.as"))
			prev := beforeVerify
			t.Cleanup(func() { beforeVerify = prev })
			corrupted := false
			beforeVerify = func(tmp string) {
				b := fileBytes(t, tmp)
				if !strings.Contains(b, tc.from) {
					t.Fatalf("the corruption no longer applies: %q not in %q", tc.from, b)
				}
				if err := os.WriteFile(tmp, []byte(strings.Replace(b, tc.from, tc.to, 1)), 0o600); err != nil {
					t.Fatal(err)
				}
				corrupted = true
			}
			_, err := RetitleChart(context.Background(), dir, row, seenOf(t, dir, row), "Constructed One", "Bremen, Germany")
			if !corrupted {
				t.Fatal("the seam never ran")
			}
			if !errors.Is(err, ErrBadReadBack) || !strings.Contains(err.Error(), tc.name) {
				t.Fatalf("err = %v, want a read-back refusal naming the %s", err, tc.name)
			}
			if fileBytes(t, filepath.Join(dir, "Equinox-2026-09.as")) != original {
				t.Error("the original changed")
			}
			if _, err := os.Stat(filepath.Join(dir, store.ChartBackupRel("Equinox-2026-09.as"))); !os.IsNotExist(err) {
				t.Error("a refused write made a backup")
			}
			noTempLeft(t, dir)
		})
	}
	t.Run("double quote passes", func(t *testing.T) {
		dir, row := untitledEquinox(t)
		if _, err := RetitleChart(context.Background(), dir, row, seenOf(t, dir, row), `Constructed "Q" One`, `Bremen "Old Town"`); err != nil {
			t.Fatalf("a title holding a double quote was refused: %v", err)
		}
	})
}

// § Failure modes: a chart whose bytes changed since the listing read them,
// or while Astrolog runs, is not replaced.
func TestRetitleRefusesAChangedFile(t *testing.T) {
	t.Run("since the listing", func(t *testing.T) {
		dir := useLibraryDir(t)
		copyFixture(t, dir, "Equinox-2026-09.as")
		lib, err := LoadChartLibrary()
		if err != nil {
			t.Fatal(err)
		}
		row := rowByFile(t, lib, "Equinox-2026-09.as")
		path := filepath.Join(dir, "Equinox-2026-09.as")
		// Still untitled and plain, so only the fingerprint can tell.
		edited := strings.Replace(fileBytes(t, path), "2026 2:06 DT", "2026 2:36 DT", 1)
		if err := os.WriteFile(path, []byte(edited), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := RetitleChart(context.Background(), dir, row, lib.Seen["Equinox-2026-09.as"], "Constructed One", "Bremen, Germany"); !errors.Is(err, store.ErrChartChanged) {
			t.Fatalf("err = %v, want ErrChartChanged", err)
		}
		if fileBytes(t, path) != edited {
			t.Error("the edited chart was rewritten")
		}
		if _, err := os.Stat(filepath.Join(filepath.Dir(path), store.ChartBackupRel(filepath.Base(path)))); !os.IsNotExist(err) {
			t.Error("a refused write made a backup")
		}
		if lib.Seen["Equinox-2026-09.as"] == "" {
			t.Fatal("the listing recorded no fingerprint")
		}
	})

	dir, row := untitledEquinox(t)
	path := filepath.Join(dir, "Equinox-2026-09.as")
	prev := beforeVerify
	t.Cleanup(func() { beforeVerify = prev })
	const edited = "@AI780  ; Astrolog chart info.\n-qb Mar 20 1995 12:00pm DT 1E   8:48E 53:04N\n-zi \"Edited Elsewhere\" \"Bremen, Germany\"\n"
	beforeVerify = func(string) {
		if err := os.WriteFile(path, []byte(edited), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := RetitleChart(context.Background(), dir, row, seenOf(t, dir, row), "Constructed One", "Bremen, Germany"); !errors.Is(err, store.ErrChartChanged) {
		t.Fatalf("err = %v, want ErrChartChanged", err)
	}
	if fileBytes(t, path) != edited {
		t.Error("the edit made meanwhile was overwritten")
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(path), store.ChartBackupRel(filepath.Base(path)))); !os.IsNotExist(err) {
		t.Error("a refused write made a backup")
	}
	noTempLeft(t, dir)
}

// C3.15: the write path refuses a control character itself, whoever calls it.
func TestRetitleRefusesControlChars(t *testing.T) {
	dir, row := untitledEquinox(t)
	original := fileBytes(t, filepath.Join(dir, "Equinox-2026-09.as"))
	for _, tc := range []struct{ title, place string }{
		{"Constructed\tOne", "Bremen, Germany"},
		{"Constructed One", "Bremen,\nGermany"},
	} {
		if _, err := RetitleChart(context.Background(), dir, row, seenOf(t, dir, row), tc.title, tc.place); !errors.Is(err, astrolog.ErrLabelHasControlChar) {
			t.Errorf("%q / %q: err = %v, want ErrLabelHasControlChar", tc.title, tc.place, err)
		}
	}
	if fileBytes(t, filepath.Join(dir, "Equinox-2026-09.as")) != original {
		t.Error("the chart changed")
	}
	noTempLeft(t, dir)
	if _, err := os.Stat(filepath.Join(dir, store.ChartBackupRel("Equinox-2026-09.as"))); !os.IsNotExist(err) {
		t.Error("a refused write made a backup")
	}
}

// seenOf is the fingerprint a listing would record for row's file now, for
// the tests that start from a row and are not about the listing check.
func seenOf(t *testing.T, dir string, row store.NamedChart) string {
	t.Helper()
	fp, err := fingerprint(filepath.Join(dir, row.Source.File))
	if err != nil {
		t.Fatal(err)
	}
	return fp
}

// § Failure modes, round 2: a file saved WHILE the listing reads it gets no
// fingerprint, so its row -- which may describe either version -- cannot be
// retitled; and a retitle with no fingerprint at all is refused.
func TestRetitleNeedsAFingerprintFromTheListing(t *testing.T) {
	dir := useLibraryDir(t)
	copyFixture(t, dir, "Equinox-2026-09.as")
	path := filepath.Join(dir, "Equinox-2026-09.as")
	edited := strings.Replace(fileBytes(t, path), "2026 2:06 DT", "2026 2:36 DT", 1)
	prev := betweenListingReads
	t.Cleanup(func() { betweenListingReads = prev })
	betweenListingReads = func() { os.WriteFile(path, []byte(edited), 0o600) }
	lib, err := LoadChartLibrary()
	if err != nil {
		t.Fatal(err)
	}
	if fp, ok := lib.Seen["Equinox-2026-09.as"]; ok {
		t.Errorf("a file saved during the listing got a fingerprint %q", fp)
	}
	row := rowByFile(t, lib, "Equinox-2026-09.as")
	if _, err := RetitleChart(context.Background(), dir, row, lib.Seen["Equinox-2026-09.as"], "Constructed One", "Bremen, Germany"); !errors.Is(err, store.ErrChartChanged) {
		t.Fatalf("err = %v, want ErrChartChanged", err)
	}
	if fileBytes(t, path) != edited {
		t.Error("the chart was rewritten")
	}
}

// stripSpawnPrefix returns argv without `-YQ 0` and the hook-clear band, or
// false when argv does not start with exactly that.
func stripSpawnPrefix(argv []string) ([]string, bool) {
	want := append([]string{"-YQ", "0"}, astrolog.HookClearBand()...)
	if len(argv) < len(want) || !slices.Equal(argv[:len(want)], want) {
		return nil, false
	}
	return argv[len(want):], true
}

// A linked `.sextile` or `.sextile/backup` makes the retitle refuse, with the
// chart untouched and the file the link leads to untouched.
func TestRetitleRefusesALinkedBackupFolder(t *testing.T) {
	for _, linked := range []string{".sextile", ".sextile/backup"} {
		t.Run(linked, func(t *testing.T) {
			dir, row := untitledEquinox(t)
			recordingAstrolog(t, "")
			original := fileBytes(t, filepath.Join(dir, "Equinox-2026-09.as"))
			outside := t.TempDir()
			external := filepath.Join(outside, "Equinox-2026-09.as")
			if linked == ".sextile" {
				mustMkdirAll(t, filepath.Join(outside, "backup"))
				external = filepath.Join(outside, "backup", "Equinox-2026-09.as")
			} else {
				mustMkdirAll(t, filepath.Join(dir, ".sextile"))
			}
			if err := os.WriteFile(external, []byte("somebody else's file\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(outside, filepath.Join(dir, linked)); err != nil {
				t.Fatal(err)
			}
			if _, err := RetitleChart(context.Background(), dir, row, seenOf(t, dir, row), "Constructed One", "Bremen, Germany"); err == nil {
				t.Fatal("the retitle went ahead through a linked backup folder")
			}
			if fileBytes(t, filepath.Join(dir, "Equinox-2026-09.as")) != original {
				t.Error("the chart changed")
			}
			if fileBytes(t, external) != "somebody else's file\n" {
				t.Error("the file the link leads to changed")
			}
		})
	}
}

func mustMkdirAll(t *testing.T, p string) {
	t.Helper()
	if err := os.MkdirAll(p, 0o700); err != nil {
		t.Fatal(err)
	}
}
