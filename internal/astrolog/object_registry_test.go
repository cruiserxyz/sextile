package astrolog

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/cruiserxyz/sextile/internal/astrologstyle"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

func byNumber(reg []aspect.RegistryObject) map[int]aspect.RegistryObject {
	m := make(map[int]aspect.RegistryObject, len(reg))
	for _, o := range reg {
		m[o.Number] = o
	}
	return m
}

// TestParseObjectRegistry_FullCapture: the shipped read must find every object
// the binary carries, in both row shapes. The count is asserted because the
// failure this guards is a parser that "succeeds" on a subset — the star rows
// have different columns, and a parser written for the dignity columns drops
// fifty objects without erroring.
func TestParseObjectRegistry_FullCapture(t *testing.T) {
	reg, err := ParseObjectRegistry(readFixture(t, "ho-all.txt"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(reg) != 134 {
		t.Errorf("got %d objects, want 134", len(reg))
	}

	idx := byNumber(reg)
	for _, want := range []struct {
		num  int
		name string
		star bool
	}{
		{0, "Earth", false},        // 0-BASED: row zero exists and is not the Sun
		{1, "Sun", false},          //
		{11, "Chiron", false},      // a dignity row
		{12, "Ceres", false},       // ...and another
		{16, "North Node", false},  // a name with a space in it
		{84, "Sirius", true},       // the first star row
		{131, "Zeta Retic.", true}, // a star name followed by a SINGLE space
		{133, "Great Attr.", true}, // the last row
	} {
		got, ok := idx[want.num]
		if !ok {
			t.Errorf("object %d missing from the registry", want.num)
			continue
		}
		if got.Name != want.name {
			t.Errorf("object %d name = %q, want %q", want.num, got.Name, want.name)
		}
		if got.Star != want.star {
			t.Errorf("object %d Star = %v, want %v", want.num, got.Star, want.star)
		}
	}

	stars := 0
	for _, o := range reg {
		if o.Star {
			stars++
		}
	}
	if stars != 50 {
		t.Errorf("got %d star rows, want 50", stars)
	}
}

// TestObjectNumbersComeFromTheRegistry is C2: the mapping between an object and
// its number comes from the binary, not from a table compiled in here. Asserted
// against two fixtures that number the same objects differently — a hardcoded
// table passes every other assertion in this file and then miscompiles for any
// binary whose registry is ordered differently.
func TestObjectNumbersComeFromTheRegistry(t *testing.T) {
	shipped, err := ParseObjectRegistry(readFixture(t, "ho-all.txt"))
	if err != nil {
		t.Fatalf("parse shipped: %v", err)
	}
	other, err := ParseObjectRegistry(readFixture(t, "ho-renumbered.txt"))
	if err != nil {
		t.Fatalf("parse renumbered: %v", err)
	}

	nameOf := func(reg []aspect.RegistryObject, n int) string {
		for _, o := range reg {
			if o.Number == n {
				return o.Name
			}
		}
		return ""
	}
	if got := nameOf(shipped, 1); got != "Sun" {
		t.Errorf("shipped registry object 1 = %q, want Sun", got)
	}
	if got := nameOf(other, 1); got == "Sun" {
		t.Fatal("the renumbered fixture must NOT call object 1 the Sun, or it cannot detect a hardcoded table")
	}
	if got := nameOf(other, 7); got != "Sun" {
		t.Errorf("renumbered registry object 7 = %q, want Sun — the parse must follow the fixture, not a compiled-in table", got)
	}
	if got := nameOf(other, 99); got != "Zeta Retic." {
		t.Errorf("renumbered registry object 99 = %q, want Zeta Retic.", got)
	}
}

// A host with colour on is a valid host, and this parse is column-based, so an
// SGR escape at the start of the line shifts every field. Found by the
// independent cross-family reviewer reading the producer's own source: the
// object list emits a colour change before each number whenever `=k` is set.
//
//	astrolog -k -R1 -HO > testdata/ho-ansi.txt
func TestParseObjectRegistrySurvivesColouredOutput(t *testing.T) {
	reg, err := ParseObjectRegistry(readFixture(t, "ho-ansi.txt"))
	if err != nil {
		t.Fatalf("a colour-enabled host lost the whole selector: %v", err)
	}
	plain, err := ParseObjectRegistry(readFixture(t, "ho-all.txt"))
	if err != nil {
		t.Fatalf("baseline: %v", err)
	}
	if len(reg) != len(plain) {
		t.Fatalf("coloured output parsed %d objects, plain parsed %d", len(reg), len(plain))
	}
	for i := range reg {
		if reg[i] != plain[i] {
			t.Errorf("row %d differs with colour on:\n coloured %+v\n plain    %+v", i, reg[i], plain[i])
		}
	}
}

// TestParseObjectRegistry_Refusals is C10's parse half. Each case reaches the
// argv if it is accepted, so each must be refused rather than guessed at.
func TestParseObjectRegistry_Refusals(t *testing.T) {
	const header = "Astrolog planets and objects:\nNum. Name       Rule\n\n"
	cases := []struct {
		name string
		out  string
	}{
		{"empty output", ""},
		{"header only — what a fully restricted -HO prints", header},
		{"two rows sharing a number", header +
			"  1 Sun         Leo\n" +
			"  1 Moon        Can\n"},
		{"a row with no name", header +
			"  1             \n"},
		// A number field that is PART digits is a corrupted row, not the
		// prose astrolog folds into this stream. Skipping one opens the
		// selector missing an object the binary has.
		{"a negative object number", header +
			"  1 Sun         Leo\n" +
			" -1 Broken      Leo\n"},
		{"an object number with a stray character", header +
			"  1 Sun         Leo\n" +
			" 2x Broken      Leo\n"},
		{"an object number that is not an integer", header +
			"  1 Sun         Leo\n" +
			"1.2 Broken      Leo\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := ParseObjectRegistry(tc.out); err == nil {
				t.Error("must be refused: a registry that parses into an ambiguous table reaches the argv")
			}
		})
	}
}

// TestParseObjectRegistry_SkipsNoise: astrolog folds stderr into the same
// stream, so an ephemeris warning rides along with the table. It is not a
// registry row and must not become one, nor abort the read.
func TestParseObjectRegistry_SkipsNoise(t *testing.T) {
	out := "SwissEph file 'se00010s.se1' not found in PATH '.;:;:ephem'\n" +
		"Astrolog planets and objects:\n" +
		"Num. Name       Rule     Detriment\n\n" +
		"  0 Earth       Sag      Gem\n" +
		"  1 Sun         Leo      Aqu\n"
	reg, err := ParseObjectRegistry(out)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(reg) != 2 {
		t.Fatalf("got %d objects, want 2 — the warning line must be skipped, not parsed", len(reg))
	}
	if reg[0].Number != 0 || reg[0].Name != "Earth" {
		t.Errorf("first row = %+v, want {0 Earth}", reg[0])
	}
}

// TestObjectRegistryReadIsUnrestricted is C5a's trapdoor, asserted on a real
// capture rather than argued in prose: `-HO` reports only UNRESTRICTED objects,
// so a read taken under this program's own restrictions returns the selection
// and nothing else. `ho-restricted.txt` is a capture of `-R0 1 2 -HO`.
//
// If the shipped read ever loses its `-R1`, the operator could never re-add a
// body and each open would narrow further. No other test can see that, because
// every other one holds the registry fixed.
func TestObjectRegistryReadIsUnrestricted(t *testing.T) {
	restricted, err := ParseObjectRegistry(readFixture(t, "ho-restricted.txt"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(restricted) >= 134 {
		t.Fatalf("the restricted capture has %d objects; it is supposed to demonstrate narrowing", len(restricted))
	}
	full, err := ParseObjectRegistry(readFixture(t, "ho-all.txt"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(full) <= len(restricted) {
		t.Errorf("full read has %d objects and the restricted one %d; -R1 must come first", len(full), len(restricted))
	}
}

// TestObjectRegistryAgainstLiveAstrolog: the fixtures above are only as good as
// their capture. This asks the binary directly, so a drift between the shipped
// fixture and the installed Astrolog shows up here rather than in the
// operator's modal. Skipped when astrolog is not on PATH.
func TestObjectRegistryAgainstLiveAstrolog(t *testing.T) {
	if _, err := exec.LookPath("astrolog"); err != nil {
		t.Skip("astrolog not on PATH; skipping live-binary registry test")
	}
	live, err := ObjectRegistry(context.Background())
	if err != nil {
		t.Fatalf("ObjectRegistry: %v", err)
	}
	fixture, err := ParseObjectRegistry(readFixture(t, "ho-all.txt"))
	if err != nil {
		t.Fatalf("parse fixture: %v", err)
	}
	if len(live) != len(fixture) {
		t.Errorf("live registry has %d objects, fixture has %d — recapture testdata/ho-all.txt", len(live), len(fixture))
	}
	liveIdx, fixIdx := byNumber(live), byNumber(fixture)
	for n, want := range fixIdx {
		got, ok := liveIdx[n]
		if !ok {
			t.Errorf("object %d (%s) is in the fixture and not in the live binary", n, want.Name)
			continue
		}
		if got.Name != want.Name || got.Star != want.Star {
			t.Errorf("object %d live = %+v, fixture = %+v", n, got, want)
		}
	}
}

// TestObjectRegistryCommandCarriesR1 pins the command itself. The parse cannot
// see which switches produced its input, so without this the `-R1` could be
// dropped and every parse test would still pass.
func TestObjectRegistryCommandCarriesR1(t *testing.T) {
	src, err := os.ReadFile("object_registry.go")
	if err != nil {
		t.Fatalf("read source: %v", err)
	}
	// The call is an exempt call since astrolog-calls-honour-choosers: the argv
	// is the trailing `"-R1", "-HO"` of its newExempt, after the reason.
	if !strings.Contains(string(src), `"-R1", "-HO"))`) {
		t.Error(`ObjectRegistry must invoke "-R1", "-HO"; a bare -HO returns only what is currently unrestricted (C5a)`)
	}
}

// testRegistry is the object list of the Astrolog the fixtures were captured
// from (testdata/ho-all.txt), read through ParseObjectRegistry: what every
// parse test hands the parser. The path is fixed when the package loads, so a
// test that changes directory still finds it.
func testRegistry(t *testing.T) aspect.Registry {
	t.Helper()
	reg, err := loadTestRegistry()
	if err != nil {
		t.Fatal(err)
	}
	return reg
}

func loadTestRegistry() (aspect.Registry, error) {
	testRegistryOnce.Do(func() {
		// go test runs with the package folder as the working directory, and
		// testPackageDir holds it from before any test can change directory. A
		// path from runtime.Caller is the module path under -trimpath, which
		// the release check sets, and the file was not found there.
		b, err := os.ReadFile(filepath.Join(testPackageDir, "testdata", "ho-all.txt"))
		if err == nil {
			testRegistryList, err = ParseObjectRegistry(string(b))
		}
		testRegistryErr = err
	})
	return testRegistryList, testRegistryErr
}

// testPackageDir is the package folder, read when the test binary loads.
var testPackageDir, _ = os.Getwd()

var (
	testRegistryOnce sync.Once
	testRegistryList []aspect.RegistryObject
	testRegistryErr  error
)

// defaultSet is the thirteen objects Astrolog draws without a restriction:
// the ten planets, the North Node, the Ascendant and the Midheaven.
var defaultSet = []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 16, 22, 31}

// mustPositions parses a positions listing that was asked for the given
// objects, through the test registry.
func mustPositions(t *testing.T, out string, asked ...int) []aspect.Position {
	t.Helper()
	ps, err := parsePositions(out, asked, testRegistry(t))
	if err != nil {
		t.Fatal(err)
	}
	return ps
}

// printedPositions parses whatever rows a listing printed, naming them by the
// order of the default set: for a test that compares two listings of the same
// call and so needs the rows read, not the objects named.
func printedPositions(t *testing.T, out string) []aspect.Position {
	t.Helper()
	// The asked numbers are what the printed labels say, not a guessed prefix of
	// replaySet: a host that prints no Chiron would give every later row the
	// wrong identity (the parser now refuses that).
	reg := testRegistry(t)
	var asked []int
	for _, line := range strings.Split(astrologstyle.StripSGR(out), "\n") {
		m := bodyPositionPat.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		o, ok := reg.ByCut(m[1], 4)
		if !ok {
			t.Fatalf("row label %q does not name one object", m[1])
		}
		asked = append(asked, o.Number)
	}
	return mustPositions(t, out, asked...)
}
