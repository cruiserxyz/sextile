package astrolog

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"

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

	// The registry must exceed what aspect.Body can name, or a selector bounded
	// by the enum would lose nothing and this whole read would be pointless.
	// Seventeen named bodies against 134 rows is the margin C5 depends on.
	named := 0
	for b := aspect.Sun; b.String() != aspect.BodyUnknown.String(); b++ {
		named++
	}
	if len(reg) <= named {
		t.Errorf("registry has %d objects and the enum names %d; the read must carry more, or a bare -HO has been substituted for -R1 -HO", len(reg), named)
	}

	idx := byNumber(reg)
	for _, want := range []struct {
		num  int
		name string
		star bool
	}{
		{0, "Earth", false},        // 0-BASED: row zero exists and is not the Sun
		{1, "Sun", false},          //
		{11, "Chiron", false},      // a dignity row the enum does name
		{12, "Ceres", false},       // ...and one it does not
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
