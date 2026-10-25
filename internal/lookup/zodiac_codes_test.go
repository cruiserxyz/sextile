package lookup

// the specification C5 and the pass-7 header note. The eleven codes are
// READ from the approved table in the specification, as the house
// codes are, so they cannot drift from what the operator approved.

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/cruiserxyz/sextile/internal/recipe"
)

// approvedZodiacs parses the zodiac paragraph of the frames spec: the second
// paragraph under "The 22 house codes and 11 zodiac codes", in the table's
// reading order (Tropical first, then C2's ten).
func approvedZodiacs(t *testing.T) (codes, names []string) {
	t.Helper()
	b, err := os.ReadFile("testdata/house-zodiac-frames.md")
	if err != nil {
		t.Fatal(err)
	}
	text := string(b)
	const head = "## The 22 house codes and 11 zodiac codes"
	i := strings.Index(text, head)
	if i < 0 {
		t.Fatal("the approved table is missing from the frames spec")
	}
	paras := strings.Split(strings.TrimSpace(text[i+len(head):]), "\n\n")
	if len(paras) < 2 {
		t.Fatalf("the approved table has %d paragraphs, want the house one and the zodiac one", len(paras))
	}
	para := strings.ReplaceAll(strings.TrimSpace(paras[1]), "\n", " ")
	re := regexp.MustCompile(`^([A-Za-z]{2}) (.+)$`)
	for _, item := range strings.Split(para, " · ") {
		m := re.FindStringSubmatch(strings.TrimSpace(item))
		if m == nil {
			t.Fatalf("cannot read table item %q", item)
		}
		codes, names = append(codes, m[1]), append(names, m[2])
	}
	return codes, names
}

// All eleven codes, from the approved table, never a bare "sidereal" code.
func TestZodiacCodesMatchTheApprovedTable(t *testing.T) {
	codes, names := approvedZodiacs(t)
	if len(codes) != recipe.ZodiacCount {
		t.Fatalf("the table lists %d zodiacs, want %d", len(codes), recipe.ZodiacCount)
	}
	seen := map[string]bool{}
	for i, name := range names {
		z := recipe.Zodiac(i)
		if z.Name() != name {
			t.Errorf("table row %d is %q, the zodiac in that place is %q", i, name, z.Name())
		}
		if got := (Lens{Zodiac: z}).ZodiacAbbrev(); got != codes[i] {
			t.Errorf("%s: code %q, want the approved %q", name, got, codes[i])
		}
		if seen[codes[i]] {
			t.Errorf("code %q is used twice", codes[i])
		}
		seen[codes[i]] = true
	}
	if got := (Lens{Zodiac: recipe.Zodiac(99)}).ZodiacAbbrev(); got != "??" {
		t.Errorf("a value that is no zodiac has the code %q, want ??", got)
	}
}

// Pass-7 build note: a sidereal header names its ayanamsa.
func TestSiderealHeadersNameTheirAyanamsa(t *testing.T) {
	for z := recipe.Zodiac(0); int(z) < recipe.ZodiacCount; z++ {
		got := (Lens{Zodiac: z}).ZodiacName()
		if got != z.Name() || got == "Sidereal" || got == "" {
			t.Errorf("zodiac %d is named %q, want %q", int(z), got, z.Name())
		}
	}
}

// The finder row's offset text comes from the astrolog table and says its sign.
func TestZodiacOffsetText(t *testing.T) {
	for _, c := range []struct {
		z    recipe.Zodiac
		want string
	}{
		{recipe.Tropical, ""},
		{recipe.FaganBradley, "Astrolog's default"},
		{recipe.Lahiri, "Fagan-Bradley + 0.883208°"},
		{recipe.Deluce, "Fagan-Bradley − 3.075453°"},
		{recipe.Krishnamurti, "Fagan-Bradley + 0.98006°"},
	} {
		if got := ZodiacOffsetText(c.z); got != c.want {
			t.Errorf("ZodiacOffsetText(%s) = %q, want %q", c.z.Name(), got, c.want)
		}
	}
}
