package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The floor must stay pinned to 1 even after SchemaVersion moves on. If it
// ever tracked SchemaVersion, every unversioned file on disk would be read as
// current — mistaking an old file for a new one is the exact failure the
// version line exists to prevent.
func TestUnversionedFloorDoesNotTrackCurrent(t *testing.T) {
	if unversionedSchema != 1 {
		t.Errorf("unversionedSchema = %d, want 1 — it is the generation of every file written before versioning, not the current one", unversionedSchema)
	}
}

func TestSchemaVersionOf(t *testing.T) {
	cases := []struct {
		name string
		text string
		want int
		bad  bool
	}{
		{"absent", "location:\n  name: Berlin\n", unversionedSchema, false},
		{"present", "version: 1\nlocation:\n  name: Berlin\n", 1, false},
		{"present after other blocks", "location:\n  name: Berlin\nversion: 2\n", 2, false},
		{"quoted", "version: \"3\"\n", 3, false},
		{"indented is not top level", "location:\n  version: 9\n", unversionedSchema, false},
		{"commented out is not a declaration", "# version: 9\n", unversionedSchema, false},
		{"unreadable", "version: tomorrow\n", 0, true},
		{"empty file", "", unversionedSchema, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := schemaVersionOf(c.text)
			if c.bad {
				if err == nil {
					t.Errorf("a file that tried to state its generation and failed must not be guessed at; got %d", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("schemaVersionOf: %v", err)
			}
			if got != c.want {
				t.Errorf("schemaVersionOf = %d, want %d", got, c.want)
			}
		})
	}
}

func TestCheckSchema(t *testing.T) {
	if err := checkSchema(SchemaVersion); err != nil {
		t.Errorf("this build must read its own generation: %v", err)
	}
	if err := checkSchema(unversionedSchema); err != nil {
		t.Errorf("an unversioned file must stay readable: %v", err)
	}
	err := checkSchema(SchemaVersion + 1)
	if err == nil {
		t.Fatal("a file from a later generation must be refused, not half-read")
	}
	if !strings.Contains(err.Error(), "newer Sextile") {
		t.Errorf("the refusal should say what is wrong, got %q", err)
	}
	if checkSchema(0) == nil {
		t.Error("a generation below the floor is malformed, not old")
	}
}

// A preferences file Sextile writes says what wrote it, on its first line.
func TestSetLocation_DeclaresTheGeneration(t *testing.T) {
	path := prefPath(t)
	if err := SetLocation(berlin); err != nil {
		t.Fatalf("SetLocation: %v", err)
	}
	body := readPreferences(t, path)
	if !strings.HasPrefix(body, "version: 1\n") {
		t.Errorf("a fresh preferences.yaml must open by saying what it is:\n%s", body)
	}
	// Writing again must rewrite that line in place, never stack a second one.
	if err := SetLocation(tokyo); err != nil {
		t.Fatalf("SetLocation twice: %v", err)
	}
	if got := strings.Count(readPreferences(t, path), "version:"); got != 1 {
		t.Errorf("version declared %d times after two writes, want 1", got)
	}
}

// A preferences file from a later generation is neither read nor written: the
// perimeter falls back to the picker, and the file on disk is left exactly as
// its author left it.
func TestPreferences_FromTheFutureIsRefusedAndUntouched(t *testing.T) {
	path := prefPath(t)
	before := "version: 99\nlocation:\n  name: Berlin\n  country: DE\n  lat: 52.5244\n  lon: 13.4105\n  tz: Europe/Berlin\nsomething-new:\n  we: cannot-know\n"
	seedPreferences(t, path, before)

	if _, ok := Location(); ok {
		t.Error("a location from a later generation must not be read as if we understood it")
	}
	if _, ok := LocationCandidate(); ok {
		t.Error("nor offered to the resolver as a candidate")
	}
	if err := SetLocation(tokyo); err == nil {
		t.Error("writing into a file from the future must be refused")
	}
	if got := readPreferences(t, path); got != before {
		t.Errorf("the file was modified:%s", diffText(got, before))
	}
}

// A preferences file whose version line is present but unreadable is refused
// the same way — it tried to say what it is and failed.
func TestPreferences_UnreadableVersionIsRefused(t *testing.T) {
	path := prefPath(t)
	seedPreferences(t, path, "version: tomorrow\nlocation:\n  name: Berlin\n  tz: Europe/Berlin\n")
	if _, ok := Location(); ok {
		t.Error("an unreadable generation must not be guessed at")
	}
	if err := SetLocation(tokyo); err == nil {
		t.Error("writing into it must be refused too")
	}
}

// Every preferences file written before versioning existed keeps working, and
// gains the line on the next write.
func TestPreferences_UnversionedStillReadsAndGainsTheLine(t *testing.T) {
	path := prefPath(t)
	seedPreferences(t, path, berlinBlock)
	if _, ok := Location(); !ok {
		t.Fatal("a file written before versioning must still read")
	}
	if err := SetLocation(tokyo); err != nil {
		t.Fatalf("SetLocation: %v", err)
	}
	if !strings.HasPrefix(readPreferences(t, path), "version: 1\n") {
		t.Error("the next write must declare the generation")
	}
}

// readableSchema reports a missing file as a missing file, so callers keep
// their own absent-file handling instead of confusing it with a bad version.
func TestReadableSchema_MissingFileIsNotASchemaProblem(t *testing.T) {
	_, err := readableSchema(filepath.Join(t.TempDir(), "nope.yaml"))
	if !os.IsNotExist(err) {
		t.Errorf("err = %v, want a not-exist error", err)
	}
}
