package store

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/cruiserxyz/sextile/internal/recipe"
)

// S4a A22: the retired base switch left a `base:` key in some preferences
// files. FrameConfig carries no field for it, the reader skips it wherever it
// stands in the block, and the keys beside it read as they always did.
func TestFrameConfigIgnoresBase(t *testing.T) {
	typ := reflect.TypeOf(FrameConfig{})
	for _, name := range []string{"Base", "BasePresent"} {
		if _, ok := typ.FieldByName(name); ok {
			t.Errorf("FrameConfig still carries %s", name)
		}
	}
	read := func(t *testing.T, text string) FrameConfig {
		t.Helper()
		path := filepath.Join(t.TempDir(), "preferences.yaml")
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
		return readFrame(path)
	}
	for name, text := range map[string]string{
		"base before houses":    "frame:\n  base: cursor\n  houses: koch\n",
		"base after houses":     "frame:\n  houses: koch\n  base: cursor  # try\n",
		"base between":          "frame:\n  houses: koch\n  base: cursor\n  zodiac: tropical\n",
		"unknown base value":    "frame:\n  base: sideways\n  houses: koch\n",
		"base in a later block": "frame:\n  houses: koch\nframe:\n  base: cursor\n",
	} {
		got := read(t, text)
		if !got.HousesPresent || got.Houses != "koch" {
			t.Errorf("%s: got %+v, want houses: koch read", name, got)
		}
	}
	if got := read(t, "frame:\n  houses: koch\n  base: cursor\n  zodiac: tropical\n"); !got.ZodiacPresent || got.Zodiac != "tropical" {
		t.Errorf("zodiac beside base: got %+v", got)
	}
	if got := readFrame(filepath.Join(t.TempDir(), "none.yaml")); got.HousesPresent || got.ZodiacPresent {
		t.Errorf("absent file: got %+v", got)
	}
}

// The reader's own comment on Frame() is the only documentation of the block,
// so it must say what the retired key does now.
func TestFrameConfigDocNamesTheRetiredBase(t *testing.T) {
	src, err := os.ReadFile("frame_config.go")
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(string(src), "\n")
	end := -1
	for i, l := range lines {
		if strings.HasPrefix(l, "func Frame()") {
			end = i
		}
	}
	if end < 0 {
		t.Fatal("func Frame() not found")
	}
	var doc []string
	for i := end - 1; i >= 0 && strings.HasPrefix(lines[i], "//"); i-- {
		doc = append(doc, lines[i])
	}
	text := strings.Join(doc, "\n")
	for _, want := range []string{"houses:", "`base:`", "ignored"} {
		if !strings.Contains(text, want) {
			t.Errorf("the comment on Frame() lacks %q:\n%s", want, text)
		}
	}
}

// SetFrame owns the `houses:` and `zodiac:` keys of the first `frame:` block
// and nothing else. A retired `base:`, a comment, a blank line, an unknown
// block and the key after the block are carried through byte for byte; a
// replaced value keeps the comment after it, and the whitespace before that
// comment, as written; a missing key goes after the block's last non-blank
// line; a second write is a no-op; what was written reads back as it was
// given.
func TestSetFrameOwnsTwoKeys(t *testing.T) {
	path := prefPath(t)
	before := "# my preferences\n" +
		"version: 1\n" +
		"unknown_block:\n  deep:\n    - a\n" +
		"frame:\n" +
		"  # kept by hand\n" +
		"  base: cursor\n" +
		"  houses: Koch  # the old default\r\n" +
		"  # a last note\n" +
		"\n" +
		"later: 1\n" +
		"frame:\n  houses: Equal\n"
	after := "# my preferences\n" +
		"version: 1\n" +
		"unknown_block:\n  deep:\n    - a\n" +
		"frame:\n" +
		"  # kept by hand\n" +
		"  base: cursor\n" +
		"  houses: Placidus  # the old default\r\n" +
		"  # a last note\n" +
		"  zodiac: sidereal Lahiri\n" +
		"\n" +
		"later: 1\n" +
		"frame:\n  houses: Equal\n"
	seedPreferences(t, path, before)

	if err := SetFrame("Placidus", "sidereal Lahiri"); err != nil {
		t.Fatalf("SetFrame: %v", err)
	}
	if got := readPreferences(t, path); got != after {
		t.Errorf("more than the two keys changed:%s", diffText(got, after))
	}
	want := FrameConfig{Houses: "Placidus", HousesPresent: true, Zodiac: "sidereal Lahiri", ZodiacPresent: true}
	if got := Frame(); got != want {
		t.Errorf("read back %+v, want %+v", got, want)
	}
	if err := SetFrame("Placidus", "sidereal Lahiri"); err != nil {
		t.Fatalf("second SetFrame: %v", err)
	}
	if got := readPreferences(t, path); got != after {
		t.Errorf("a second write changed the file:%s", diffText(got, after))
	}

	// Both keys present: both replaced in place, each keeping its place.
	seedPreferences(t, path, "frame:\n  zodiac: tropical\n  houses: Whole\nz: 1\n")
	if err := SetFrame("Koch", "sidereal Raman"); err != nil {
		t.Fatalf("SetFrame: %v", err)
	}
	if got, want := readPreferences(t, path), "frame:\n  zodiac: sidereal Raman\n  houses: Koch\nz: 1\n"; got != want {
		t.Errorf("replace in place:%s", diffText(got, want))
	}
}

// With no `frame:` block the block is appended, after an unterminated last
// line has been terminated; with no file the file is the block alone. An
// empty block, or one at the end of an unterminated file, takes the keys too.
func TestSetFrameAppendsWhenAbsent(t *testing.T) {
	path := prefPath(t)
	block := "frame:\n  houses: Whole\n  zodiac: tropical\n"
	if err := SetFrame("Whole", "tropical"); err != nil {
		t.Fatalf("SetFrame on no file: %v", err)
	}
	if got := readPreferences(t, path); got != block {
		t.Errorf("fresh file:%s", diffText(got, block))
	}
	seedPreferences(t, path, "theme:\n  accent: teal")
	if err := SetFrame("Whole", "tropical"); err != nil {
		t.Fatal(err)
	}
	if got, want := readPreferences(t, path), "theme:\n  accent: teal\n"+block; got != want {
		t.Errorf("append:%s", diffText(got, want))
	}
	// A lookalike is not the block, and an indented one belongs to a block.
	seedPreferences(t, path, "frame :\n  note: x\ntheme:\n  frame:\n    houses: Koch\n")
	if err := SetFrame("Whole", "tropical"); err != nil {
		t.Fatal(err)
	}
	if got, want := readPreferences(t, path), "frame :\n  note: x\ntheme:\n  frame:\n    houses: Koch\n"+block; got != want {
		t.Errorf("lookalikes were treated as the block:%s", diffText(got, want))
	}
	seedPreferences(t, path, "frame:\nlater: 1")
	if err := SetFrame("Whole", "tropical"); err != nil {
		t.Fatal(err)
	}
	if got, want := readPreferences(t, path), "frame:\n  houses: Whole\n  zodiac: tropical\nlater: 1"; got != want {
		t.Errorf("empty block:%s", diffText(got, want))
	}
	seedPreferences(t, path, "frame:\n  base: cursor")
	if err := SetFrame("Whole", "tropical"); err != nil {
		t.Fatal(err)
	}
	if got, want := readPreferences(t, path), "frame:\n  base: cursor\n  houses: Whole\n  zodiac: tropical\n"; got != want {
		t.Errorf("unterminated block:%s", diffText(got, want))
	}
}

// Every house system and zodiac the reader's table holds reads back as it was
// written, and a value outside the table writes nothing.
func TestSetFrameWritesOnlyWhatReadsBack(t *testing.T) {
	path := prefPath(t)
	for n := 0; n < recipe.HouseSystemCount; n++ {
		hs := recipe.HouseSystem(n).StoredName()
		z := recipe.Zodiac(n % recipe.ZodiacCount).RecordName()
		if err := SetFrame(hs, z); err != nil {
			t.Fatalf("SetFrame(%q, %q): %v", hs, z, err)
		}
		if got := Frame(); got.Houses != hs || got.Zodiac != z {
			t.Errorf("wrote %q, %q; read back %+v", hs, z, got)
		}
	}
	const kept = "frame:\n  houses: Koch\n  zodiac: tropical\n"
	seedPreferences(t, path, kept)
	for _, bad := range [][2]string{
		{"Placidus\nzodiac: x", "tropical"}, {"Placidus # c", "tropical"}, {" Placidus", "tropical"},
		{"Placidus ", "tropical"}, {"placidus", "tropical"}, {"", "tropical"},
		{"Placidus", "sidereal Lahiri\n"}, {"Placidus", "Lahiri"}, {"Placidus", ""}, {"Placidus", "tropical #"},
	} {
		if err := SetFrame(bad[0], bad[1]); err == nil {
			t.Errorf("SetFrame(%q, %q) was accepted", bad[0], bad[1])
		}
		if got := readPreferences(t, path); got != kept {
			t.Errorf("SetFrame(%q, %q) refused but wrote:%s", bad[0], bad[1], diffText(got, kept))
		}
	}
	// A later generation's file is refused, as for the other writers.
	seedPreferences(t, path, "version: 99\n")
	if err := SetFrame("Koch", "tropical"); err == nil {
		t.Error("a version 99 file was written into")
	}
	if got := readPreferences(t, path); got != "version: 99\n" {
		t.Errorf("the refused write changed the file: %q", got)
	}
}

// An inline comment after an owned value stays, with the exact whitespace
// before it; only the value moves. A `#` inside quotes is part of the value.
func TestSetFrameKeepsInlineComments(t *testing.T) {
	path := prefPath(t)
	seedPreferences(t, path, "frame:\n  houses: Koch\t# tab  #  two\n  zodiac: \"tropical\"     # a long gap\n")
	if err := SetFrame("Whole", "sidereal Raman"); err != nil {
		t.Fatal(err)
	}
	want := "frame:\n  houses: Whole\t# tab  #  two\n  zodiac: sidereal Raman     # a long gap\n"
	if got := readPreferences(t, path); got != want {
		t.Errorf("a comment was lost or reflowed:%s", diffText(got, want))
	}
}

// Only the block's direct children are frame keys. The child indent is the
// indent of the first line that is neither blank nor a comment; a deeper line
// is some nested mapping's, whatever it is called, and neither the reader nor
// the writer touches it. A key the block lacks goes in at the child indent.
func TestFrameKeysAreDirectChildren(t *testing.T) {
	path := prefPath(t)
	nested := "frame:\n  # first, a comment\n    # deeper comment\n  vendor_options:\n    houses: Koch\n    zodiac: sidereal Lahiri\n  base: cursor\nlater: 1\n"
	seedPreferences(t, path, nested)
	if got := readFrame(path); got.HousesPresent || got.ZodiacPresent {
		t.Errorf("a nested houses/zodiac was read as the frame: %+v", got)
	}
	if err := SetFrame("Placidus", "tropical"); err != nil {
		t.Fatal(err)
	}
	want := "frame:\n  # first, a comment\n    # deeper comment\n  vendor_options:\n    houses: Koch\n    zodiac: sidereal Lahiri\n  base: cursor\n" +
		"  houses: Placidus\n  zodiac: tropical\nlater: 1\n"
	if got := readPreferences(t, path); got != want {
		t.Errorf("a nested key was written to or the new keys misplaced:%s", diffText(got, want))
	}
	if got, w := readFrame(path), (FrameConfig{Houses: "Placidus", HousesPresent: true, Zodiac: "tropical", ZodiacPresent: true}); got != w {
		t.Errorf("read back %+v, want %+v", got, w)
	}

	// The child indent is the block's own: four spaces here, and a direct
	// child's key at that indent is replaced in place.
	seedPreferences(t, path, "frame:\n    base: cursor\n    houses: Koch\n      houses: Equal\n")
	if err := SetFrame("Whole", "tropical"); err != nil {
		t.Fatal(err)
	}
	want = "frame:\n    base: cursor\n    houses: Whole\n      houses: Equal\n    zodiac: tropical\n"
	if got := readPreferences(t, path); got != want {
		t.Errorf("four-space block:%s", diffText(got, want))
	}

	// A block with no children yet takes two spaces.
	seedPreferences(t, path, "frame: # empty\nlater: 1\n")
	if err := SetFrame("Whole", "tropical"); err != nil {
		t.Fatal(err)
	}
	want = "frame: # empty\n  houses: Whole\n  zodiac: tropical\nlater: 1\n"
	if got := readPreferences(t, path); got != want {
		t.Errorf("empty block:%s", diffText(got, want))
	}
}

// `frame: <value>` is a scalar, not a block: nothing can be read from it, and
// a write refuses, names the line, and leaves the file as it was. `frame:`
// with only a comment after it still opens a mapping.
func TestSetFrameRefusesAScalarFrame(t *testing.T) {
	path := prefPath(t)
	for _, text := range []string{
		"frame: retired-default\n",
		"version: 1\nframe: {houses: Koch}\nlater: 1\n",
		"frame: \"x\" # note\n  houses: Koch\n",
	} {
		seedPreferences(t, path, text)
		if got := readFrame(path); got.HousesPresent || got.ZodiacPresent {
			t.Errorf("%q: read %+v from a scalar frame", text, got)
		}
		err := SetFrame("Placidus", "tropical")
		if err == nil {
			t.Errorf("%q: SetFrame wrote into a scalar frame", text)
			continue
		}
		if !strings.Contains(err.Error(), "not a block") || !strings.Contains(err.Error(), "'frame:") {
			t.Errorf("%q: the error does not name the line: %v", text, err)
		}
		if got := readPreferences(t, path); got != text {
			t.Errorf("%q: the refused write changed the file:%s", text, diffText(got, text))
		}
	}
	seedPreferences(t, path, "frame: # still a block\n  houses: Koch\n")
	if got := readFrame(path); !got.HousesPresent || got.Houses != "Koch" {
		t.Errorf("a comment after frame: hid its block: %+v", got)
	}
	if err := SetFrame("Whole", "tropical"); err != nil {
		t.Errorf("a commented block was refused: %v", err)
	}
}

// A YAML document marker in column zero ends the block, for the reader and
// for the writer: a missing key goes in before it, inside the first document.
func TestFrameBlockEndsAtADocumentMarker(t *testing.T) {
	path := prefPath(t)
	for _, marker := range []string{"---", "...", "--- # next", "---\r"} {
		seedPreferences(t, path, "frame:\n  base: cursor\n"+marker+"\nframe:\n  zodiac: tropical\nhouses: x\n")
		if got := readFrame(path); got.ZodiacPresent {
			t.Errorf("%q: read past the marker: %+v", marker, got)
		}
		if err := SetFrame("Koch", "sidereal Lahiri"); err != nil {
			t.Fatal(err)
		}
		want := "frame:\n  base: cursor\n  houses: Koch\n  zodiac: sidereal Lahiri\n" + marker + "\nframe:\n  zodiac: tropical\nhouses: x\n"
		if got := readPreferences(t, path); got != want {
			t.Errorf("%q:%s", marker, diffText(got, want))
		}
	}
	seedPreferences(t, path, "frame:\n---\nlater: preserved\n")
	if err := SetFrame("Koch", "tropical"); err != nil {
		t.Fatal(err)
	}
	want := "frame:\n  houses: Koch\n  zodiac: tropical\n---\nlater: preserved\n"
	if got := readPreferences(t, path); got != want {
		t.Errorf("empty first document:%s", diffText(got, want))
	}
}

// A file from a later generation is no file at all to the reader, as for the
// other preferences: its frame is not applied.
func TestFrameIgnoresAFutureSchema(t *testing.T) {
	path := prefPath(t)
	for _, head := range []string{"version: 99\n", "version: nope\n"} {
		seedPreferences(t, path, head+"frame:\n  houses: Koch\n  zodiac: sidereal Lahiri\n")
		if got := Frame(); got.HousesPresent || got.ZodiacPresent {
			t.Errorf("%q: read %+v from a file this build cannot read", head, got)
		}
	}
	seedPreferences(t, path, "version: 1\nframe:\n  houses: Koch\n")
	if got := Frame(); !got.HousesPresent || got.Houses != "Koch" {
		t.Errorf("a current file was ignored: %+v", got)
	}
}
