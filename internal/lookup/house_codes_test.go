package lookup

// the specification C17 and the HouseName build note. The expected pairs
// are READ from the approved table in the specification, so the code
// cannot drift from what the operator approved.

import (
	"context"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/payload"
	"github.com/cruiserxyz/sextile/internal/recipe"
)

// approvedHouses parses the "<code> <name> · ..." house paragraph of the frames
// spec, in -c order.
func approvedHouses(t *testing.T) (codes, names []string) {
	t.Helper()
	b, err := os.ReadFile("testdata/house-zodiac-frames.md")
	if err != nil {
		t.Fatal(err)
	}
	text := string(b)
	i := strings.Index(text, "## The 22 house codes and 11 zodiac codes")
	if i < 0 {
		t.Fatal("the approved table is missing from the frames spec")
	}
	para := strings.TrimSpace(text[i+len("## The 22 house codes and 11 zodiac codes"):])
	para = strings.SplitN(para, "\n\n", 2)[0]
	para = strings.ReplaceAll(para, "\n", " ")
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

// approvedOrder maps the table's reading order (Astrolog's -c order is not the
// table's) by name.
func TestHouseCodesMatchTheApprovedTable(t *testing.T) {
	codes, names := approvedHouses(t)
	if len(codes) != recipe.HouseSystemCount {
		t.Fatalf("the table lists %d systems, want %d", len(codes), recipe.HouseSystemCount)
	}
	seen := map[string]bool{}
	for i, name := range names {
		var hs recipe.HouseSystem = -1
		for n := 0; n < recipe.HouseSystemCount; n++ {
			if (Lens{Houses: recipe.HouseSystem(n)}).HouseName() == name {
				hs = recipe.HouseSystem(n)
			}
		}
		if hs < 0 {
			t.Errorf("no system is named %q", name)
			continue
		}
		got := Lens{Houses: hs}.HouseAbbrev()
		if got != codes[i] {
			t.Errorf("%s: code %q, approved %q", name, got, codes[i])
		}
		if seen[got] {
			t.Errorf("%s: code %q is not unique", name, got)
		}
		seen[got] = true
	}
	if got := (Lens{Houses: recipe.HouseSystem(99)}).HouseAbbrev(); got == "Ho" {
		t.Error("the old Ho fallback is back")
	}
}

func TestHouseHeadersUseApprovedNames(t *testing.T) {
	_, names := approvedHouses(t)
	for n := 0; n < recipe.HouseSystemCount; n++ {
		got := Lens{Houses: recipe.HouseSystem(n)}.HouseName()
		if strings.HasPrefix(got, "House ") || got == "" {
			t.Errorf("-c %d: header name %q is the fallback", n, got)
		}
		found := false
		for _, name := range names {
			found = found || name == got
		}
		if !found {
			t.Errorf("-c %d: %q is not an approved name", n, got)
		}
	}
}

// HousesUsed reaches the Result, so the ui can see a polar substitution.
func TestResultCarriesTheHousesAstrologUsed(t *testing.T) {
	stub := ChartComputer(func(ctx context.Context, spec recipe.ChartSpec) (payload.RawOutput, error) {
		return payload.RawOutput{HousesUsed: "Porphyry"}, nil
	})
	place := location.PlaceInfo{Name: "Tromso", Lat: 69.6, Lon: 18.95, TZ: "Europe/Oslo"}
	res, err := FirstCallTransits(context.Background(), &Probers{Full: stub, PositionsOnly: stub}, time.Now(), place, Hellenistic)
	if err != nil || res.HousesUsed != "Porphyry" {
		t.Errorf("transits: HousesUsed=%q err=%v", res.HousesUsed, err)
	}
}
