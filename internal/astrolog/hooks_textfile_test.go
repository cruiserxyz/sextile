package astrolog

import (
	"slices"
	"testing"
)

// The collector takes the quoted file name of every `SwissEph file '...' not
// found in PATH '...'` line whatever its suffix: the fixed-star and Uranian
// text files and the moon files are not .se1 names.
func TestMissingEphemerisCollectsEveryFileWhateverItsSuffix(t *testing.T) {
	stderr := "SwissEph file 'sefstars.txt' not found in PATH '/x/'\n" +
		"SwissEph file 'seorbel.txt' not found in PATH '/x/'\n" +
		"SwissEph file 'sepm9401.se1' not found in PATH '/x/'\n" +
		"SwissEph file 'sefstars.txt' not found in PATH '/x/'\n"
	want := []string{"sefstars.txt", "seorbel.txt", "sepm9401.se1"}
	if got := missingEphemerisFiles(stderr); !slices.Equal(got, want) {
		t.Errorf("missingEphemerisFiles = %v, want %v", got, want)
	}
	if got := warningLinesFor(stderr, true); len(got) != 0 {
		t.Errorf("a collecting call kept %v as warnings, want none", got)
	}
	if got := warningLines(stderr); len(got) != 4 {
		t.Errorf("a normal call kept %d warnings, want all 4", len(got))
	}
}
