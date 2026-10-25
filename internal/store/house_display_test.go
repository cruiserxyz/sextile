package store

import (
	"os"
	"strings"
	"testing"
)

// The write owns the one `house_display:` line. Everything else in the file —
// comments, blank lines, a block the store has no parser for, a CRLF line — is
// carried through byte for byte, and the line keeps its place.
func TestSetHouseDisplayOwnsOneLine(t *testing.T) {
	path := prefPath(t)
	head := "# my preferences\n" +
		"version: 1\n" +
		"theme:\n" +
		"  accent: teal\n" +
		"\n"
	tail := "  # an indented comment that belongs to nobody\n" +
		berlinBlock +
		"weird: [1, 2, 3]\r\n" +
		"nested:\n" +
		"  deep:\n" +
		"    - a\n"
	seedPreferences(t, path, head+"house_display: angularity  # the default\n"+tail)

	if err := SetHouseDisplay("marked"); err != nil {
		t.Fatalf("SetHouseDisplay: %v", err)
	}
	if got, want := readPreferences(t, path), head+"house_display: marked\n"+tail; got != want {
		t.Errorf("more than the one line changed:%s", diffText(got, want))
	}
	if got := HouseDisplay(); got != (HouseDisplayConfig{Value: "marked", Present: true}) {
		t.Errorf("read back %+v", got)
	}
}

// With no such line the line is appended, after an unterminated last line has
// been terminated; with no file at all the file is the line alone.
func TestSetHouseDisplayAppendsWhenAbsent(t *testing.T) {
	path := prefPath(t)
	if err := SetHouseDisplay("quadrants"); err != nil {
		t.Fatalf("SetHouseDisplay on no file: %v", err)
	}
	if got, want := readPreferences(t, path), "house_display: quadrants\n"; got != want {
		t.Errorf("fresh file:%s", diffText(got, want))
	}

	seedPreferences(t, path, "theme:\n  accent: teal\nlast: 1")
	if err := SetHouseDisplay("number"); err != nil {
		t.Fatalf("SetHouseDisplay: %v", err)
	}
	if got, want := readPreferences(t, path), "theme:\n  accent: teal\nlast: 1\nhouse_display: number\n"; got != want {
		t.Errorf("append:%s", diffText(got, want))
	}

	// A lookalike is not the owned key, and an indented one belongs to a block.
	seedPreferences(t, path, "house_display :\n  note: keep me\ntheme:\n  house_display: marked\n")
	if err := SetHouseDisplay("number"); err != nil {
		t.Fatalf("SetHouseDisplay: %v", err)
	}
	if got, want := readPreferences(t, path), "house_display :\n  note: keep me\ntheme:\n  house_display: marked\nhouse_display: number\n"; got != want {
		t.Errorf("lookalikes were treated as the owned line:%s", diffText(got, want))
	}
}

// The location writer and this one do not touch each other's lines, in either
// order: a location rewritten after the house display keeps it, and the house
// display written after a location leaves the block as it was.
func TestHouseDisplayAndLocationLeaveEachOther(t *testing.T) {
	path := prefPath(t)
	if err := SetLocation(berlin); err != nil {
		t.Fatal(err)
	}
	if err := SetHouseDisplay("quadrants"); err != nil {
		t.Fatal(err)
	}
	if got, want := readPreferences(t, path), versionLine+berlinBlock+"house_display: quadrants\n"; got != want {
		t.Errorf("house display after location:%s", diffText(got, want))
	}
	if err := SetLocation(tokyo); err != nil {
		t.Fatal(err)
	}
	if got, want := readPreferences(t, path), versionLine+tokyoBlock+"house_display: quadrants\n"; got != want {
		t.Errorf("location after house display:%s", diffText(got, want))
	}
}

// A file from a later generation is refused untouched, as for the location.
func TestSetHouseDisplayRefusesANewerFile(t *testing.T) {
	path := prefPath(t)
	body := "version: 99\nhouse_display: marked\n"
	seedPreferences(t, path, body)
	err := SetHouseDisplay("number")
	if err == nil || !strings.Contains(err.Error(), "newer Sextile") {
		t.Fatalf("SetHouseDisplay on a newer file = %v, want a refusal naming the newer generation", err)
	}
	if got := readPreferences(t, path); got != body {
		t.Errorf("a refused write changed the file:%s", diffText(got, body))
	}
}

// The reader answers with the first top-level line, unquoted and without its
// comment, and reports an empty or absent value as absent.
func TestHouseDisplayReader(t *testing.T) {
	path := prefPath(t)
	cases := []struct {
		name, body string
		want       HouseDisplayConfig
	}{
		{"absent", "theme:\n  accent: teal\n", HouseDisplayConfig{}},
		{"bare", "house_display: number\n", HouseDisplayConfig{Value: "number", Present: true}},
		{"quoted with a comment", "house_display: \"marked\"  # why not\n", HouseDisplayConfig{Value: "marked", Present: true}},
		{"first wins", "house_display: quadrants\nhouse_display: marked\n", HouseDisplayConfig{Value: "quadrants", Present: true}},
		{"indented is a block's", "theme:\n  house_display: marked\n", HouseDisplayConfig{}},
		{"empty", "house_display:\n", HouseDisplayConfig{}},
		{"unknown word is reported as read", "house_display: spiral\n", HouseDisplayConfig{Value: "spiral", Present: true}},
	}
	for _, c := range cases {
		seedPreferences(t, path, c.body)
		if got := HouseDisplay(); got != c.want {
			t.Errorf("%s: read %+v, want %+v", c.name, got, c.want)
		}
	}
}

// A newer or unreadable file is no file: the reader goes through the same gate
// as every other preference reader and reports the key absent, so a generation
// this build cannot read never sets a form at start.
func TestHouseDisplayReaderRefusesANewerFile(t *testing.T) {
	path := prefPath(t)
	for _, body := range []string{
		"version: 99\nhouse_display: marked\n",
		"house_display: marked\nversion: 2\n",
		"version: banana\nhouse_display: marked\n",
		"version: 0\nhouse_display: marked\n",
	} {
		seedPreferences(t, path, body)
		if got := HouseDisplay(); got != (HouseDisplayConfig{}) {
			t.Errorf("%q read as %+v, want absent", body, got)
		}
	}
	// The same line in a readable file is read, so the refusal is the gate's.
	seedPreferences(t, path, "version: 1\nhouse_display: marked\n")
	if got := HouseDisplay(); got != (HouseDisplayConfig{Value: "marked", Present: true}) {
		t.Errorf("a version 1 file read as %+v", got)
	}
}

// The writer takes the four form words and nothing else: any other value is an
// error and no file is created or changed.
func TestSetHouseDisplayRefusesAnythingButTheFourWords(t *testing.T) {
	path := prefPath(t)
	bad := []string{"", "spiral", "Marked", " marked", "marked ", "a\"b", "x\ny", "marked # why", "\"marked\""}

	for _, v := range bad {
		if err := SetHouseDisplay(v); err == nil {
			t.Errorf("SetHouseDisplay(%q) = nil, want a refusal", v)
		}
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("a refused value created the file (stat err %v)", err)
	}

	body := "version: 1\nhouse_display: number\ntheme:\n  accent: teal\n"
	seedPreferences(t, path, body)
	for _, v := range bad {
		_ = SetHouseDisplay(v)
	}
	if got := readPreferences(t, path); got != body {
		t.Errorf("a refused value changed the file:%s", diffText(got, body))
	}

	for _, v := range []string{"angularity", "quadrants", "number", "marked"} {
		if err := SetHouseDisplay(v); err != nil {
			t.Errorf("SetHouseDisplay(%q): %v", v, err)
		}
		if got := HouseDisplay(); got != (HouseDisplayConfig{Value: v, Present: true}) {
			t.Errorf("wrote %q, read back %+v", v, got)
		}
	}
}
