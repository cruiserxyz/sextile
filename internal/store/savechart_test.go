// All chart values in this file are constructed synthetic test inputs.
package store

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// astrologRecord is a stand-in for what astrolog writes. The bytes matter, not
// the astrology: this package never runs astrolog and must not interpret the
// record it is handed.
const astrologRecord = "@AI800  ; Astrolog chart info.\n" +
	"-qb Mar 14 1987  4:30pm ST 2E   4:54E 52:22N\n" +
	"-zi \"Test Person\" \"Amsterdam\"\n"

// C3.19 (the specification), superseding save-chart's C5: the saved file
// is astrolog's record byte for byte. The ";sextile-id:" line C5 appended is
// gone; a chart's id is its file stem.
func TestPlaceChartFileWritesTheRecordAsHanded(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SEXTILE_CHARTS_DIR", dir)

	path, err := PlaceChartFile(dir, []byte(astrologRecord), "test-person")
	if err != nil {
		t.Fatalf("PlaceChartFile: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(got) != astrologRecord {
		t.Errorf("the file is not the record as handed.\n got: %q\nwant: %q", got, astrologRecord)
	}
}

// C7: where the resolved charts directory does not exist, the save creates it
// readable and writable by the operator only, before writing.
func TestSaveCreatesTheResolvedChartsDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "charts-not-yet-there")
	t.Setenv("SEXTILE_CHARTS_DIR", dir)
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("precondition: %s already exists", dir)
	}

	if _, err := PlaceChartFile(dir, []byte(astrologRecord), "test-person"); err != nil {
		t.Fatalf("PlaceChartFile: %v", err)
	}
	fi, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("the charts directory was not created: %v", err)
	}
	if !fi.IsDir() {
		t.Fatalf("%s is not a directory", dir)
	}
	if perm := fi.Mode().Perm(); perm&0o077 != 0 {
		t.Errorf("charts directory mode = %04o, want no group or other access", perm)
	}
}

// C7a: the saved chart file is readable and writable by the operator only.
//
// A birth record is client data. The mode is asserted as "nobody else at all"
// rather than as an exact number, which is the requirement; an exact compare
// would also fail on a umask that cannot loosen it.
func TestSavedChartIsOperatorReadableOnly(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SEXTILE_CHARTS_DIR", dir)

	path, err := PlaceChartFile(dir, []byte(astrologRecord), "test-person")
	if err != nil {
		t.Fatalf("PlaceChartFile: %v", err)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	perm := fi.Mode().Perm()
	if perm&0o077 != 0 {
		t.Errorf("saved chart mode = %04o, want no group or other access", perm)
	}
	if perm&0o600 != 0o600 {
		t.Errorf("saved chart mode = %04o, want the operator to be able to read and write it", perm)
	}
}

// The exclusive create is the refusal, at the syscall. C6 states the
// requirement in library terms and internal/lookup verifies it there; this
// pins the half that cannot be reordered away.
func TestPlaceChartFileRefusesAnExistingFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SEXTILE_CHARTS_DIR", dir)
	path := filepath.Join(dir, "test-person.as")
	const existing = "@AI800  ; someone else's chart\n"
	if err := os.WriteFile(path, []byte(existing), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}

	_, err := PlaceChartFile(dir, []byte(astrologRecord), "test-person")
	if !errors.Is(err, ErrChartExists) {
		t.Fatalf("err = %v, want ErrChartExists", err)
	}
	got, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("read back: %v", readErr)
	}
	if string(got) != existing {
		t.Errorf("the existing chart was modified.\n got: %q\nwant: %q", got, existing)
	}
}

// Failure mode: the charts directory cannot be written.
func TestSaveWhenChartsDirIsReadOnly(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores the write bit, so this failure cannot be staged")
	}
	dir := t.TempDir()
	t.Setenv("SEXTILE_CHARTS_DIR", dir)
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	if _, err := PlaceChartFile(dir, []byte(astrologRecord), "test-person"); err == nil {
		t.Fatal("PlaceChartFile succeeded against a read-only charts directory")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("a failed save left %d entr(y/ies) in the charts directory", len(entries))
	}
}
