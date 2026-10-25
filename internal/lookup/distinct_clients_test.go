package lookup

import (
	"strings"
	"testing"
)

// All chart values in this file are constructed synthetic test inputs.

// Two different clients who happen to share a name are two clients.
//
// A library that shows one of them is the same failure as one that shows
// neither: store.charts' own comment ("indistinguishable from having lost
// them") applies at every scale, and silently merging two real people is the
// worse half of it, because the astrologer is given no reason to look.
func TestTwoClientsSharingANameBothList(t *testing.T) {
	dir := useLibraryDir(t)
	writeChartFile(t, dir, "smith-john-1962.as",
		"@AI780\n-qb Sep 23 1962 6:00pm DT 1E 11:34E 48:08N\n-zi \"John Smith\" \"Munich, Germany\"\n")
	writeChartFile(t, dir, "smith-john-1991.as",
		"@AI780\n-qb Mar 20 1991 12:00pm DT 1E 8:48E 53:04N\n-zi \"John Smith\" \"Bremen, Germany\"\n")

	lib, err := LoadChartLibrary()
	if err != nil {
		t.Fatalf("LoadChartLibrary: %v", err)
	}
	if len(lib.Charts) != 2 {
		t.Fatalf("want both clients listed, got %d: %+v", len(lib.Charts), lib.Charts)
	}
	if lib.Charts[0].ID == lib.Charts[1].ID {
		t.Errorf("both rows carry ID %q; two clients need two identities", lib.Charts[0].ID)
	}
	// Each row must still name the file it can be reopened from, or a pick
	// resolves the wrong person's birth moment.
	if lib.Charts[0].Source.File == lib.Charts[1].Source.File {
		t.Errorf("both rows resolve from %q", lib.Charts[0].Source.File)
	}
	years := map[int]bool{lib.Charts[0].Year: true, lib.Charts[1].Year: true}
	if !years[1962] || !years[1991] {
		t.Errorf("want the 1962 and 1991 births, got %v", years)
	}
	if len(lib.Drops) != 0 {
		t.Errorf("nothing failed to read; want no drops, got %+v", lib.Drops)
	}
}

// The other half of the same defect: identity derived from the display label
// means renaming a client inside Astrolog silently makes them a different
// chart, which breaks every stored reference to them (session.yaml's `chart:`
// block names a library ID).
func TestRenamingAClientKeepsItsLibraryIdentity(t *testing.T) {
	dir := useLibraryDir(t)
	writeChartFile(t, dir, "client-a.as", munichChart)

	before, err := LoadChartLibrary()
	if err != nil {
		t.Fatal(err)
	}
	if len(before.Charts) != 1 {
		t.Fatalf("want 1 chart, got %+v", before.Charts)
	}

	// The same file, the same person, relabelled in Astrolog.
	writeChartFile(t, dir, "client-a.as",
		"@AI780\n-qb Sep 23 1995 6:00pm DT 1E 11:34E 48:08N\n-zi \"Test Person (rectified)\" \"Munich, Germany\"\n")

	after, err := LoadChartLibrary()
	if err != nil {
		t.Fatal(err)
	}
	if len(after.Charts) != 1 {
		t.Fatalf("want 1 chart, got %+v", after.Charts)
	}
	if after.Charts[0].ID != before.Charts[0].ID {
		t.Errorf("ID moved %q → %q on a rename; identity must come from the file, not the label",
			before.Charts[0].ID, after.Charts[0].ID)
	}
	if after.Charts[0].Name != "Test Person (rectified)" {
		t.Errorf("the LABEL must follow the file: got %q", after.Charts[0].Name)
	}
}

// The residual collision, once identity comes from the filename alone
// (C3.20): two .as files whose stems slugify alike, "Shared.as" and
// "shared.as". One row is all the library can give one id, but the file that
// lost must be NAMED, which is the hard rule.
func TestTwoChartFileFilesClaimingOneIDNameTheLoser(t *testing.T) {
	dir := useLibraryDir(t)
	writeChartFile(t, dir, "Shared.as", munichChart)
	writeChartFile(t, dir, "shared.as", munichChart)

	lib, err := LoadChartLibrary()
	if err != nil {
		t.Fatal(err)
	}
	if len(lib.Charts) != 1 {
		t.Fatalf("two files claiming one id → 1 row, got %d: %+v", len(lib.Charts), lib.Charts)
	}
	if len(lib.Drops) != 1 {
		t.Fatalf("the losing file must be named, got %d drops: %+v", len(lib.Drops), lib.Drops)
	}
	if lib.Drops[0].Reason == "" {
		t.Error("a named drop must carry a reason")
	}
	if !strings.Contains(lib.Drops[0].Reason, "shared") {
		t.Errorf("reason %q should name the identity that collided", lib.Drops[0].Reason)
	}
}

// store.Slugify is lossy: it keeps ASCII alphanumerics and throws the
// rest away, so a filename can slugify to nothing at all. an earlier change moved a .as's
// identity onto the filename stem without covering that, and an empty ID is not
// a harmless one — session.yaml's own reader treats an empty id as "no id"
// (internal/ingest/ingest.go), so the row would be unreferenceable, and two
// such files would collide with each other.
//
// A chart whose file yields no identity is NAMED, like every other file that
// could not become a row. Guessing one from the display label is what an earlier change
// removed.
func TestChartFileWithNoDerivableIDIsNamedNotListedEmpty(t *testing.T) {
	dir := useLibraryDir(t)
	writeChartFile(t, dir, "---.as", munichChart)

	lib, err := LoadChartLibrary()
	if err != nil {
		t.Fatalf("LoadChartLibrary: %v", err)
	}
	for _, c := range lib.Charts {
		if c.ID == "" {
			t.Errorf("chart %q listed with an empty ID; nothing can refer to it", c.Name)
		}
	}
	if len(lib.Charts) != 0 {
		t.Fatalf("want no row for a file with no derivable identity, got %+v", lib.Charts)
	}
	if len(lib.Drops) != 1 {
		t.Fatalf("the file must be named, got %d drops: %+v", len(lib.Drops), lib.Drops)
	}
	if lib.Drops[0].File != "---.as" {
		t.Errorf("drop names %q, want ---.as", lib.Drops[0].File)
	}
	if lib.Drops[0].Reason == "" {
		t.Error("a named drop must carry a reason")
	}
}

// Two such files must not collapse onto one empty ID either — they are named
// separately, because each is a different file the astrologer may have meant.
func TestTwoChartFilesWithNoDerivableIDAreBothNamed(t *testing.T) {
	dir := useLibraryDir(t)
	writeChartFile(t, dir, "---.as", munichChart)
	writeChartFile(t, dir, "!!!.as", munichChart)

	lib, err := LoadChartLibrary()
	if err != nil {
		t.Fatal(err)
	}
	if len(lib.Charts) != 0 {
		t.Fatalf("want no rows, got %+v", lib.Charts)
	}
	if len(lib.Drops) != 2 {
		t.Fatalf("want both files named, got %d: %+v", len(lib.Drops), lib.Drops)
	}
}
