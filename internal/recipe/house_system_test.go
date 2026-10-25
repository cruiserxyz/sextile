package recipe

import "testing"

// the specification C3: a HouseSystem's value is Astrolog's -c number.
func TestHouseSystemValueIsAstrologsNumber(t *testing.T) {
	if Placidus != 0 || WholeSign != 14 || SavardA != 21 || HouseSystemCount != 22 {
		t.Fatalf("Placidus=%d WholeSign=%d SavardA=%d count=%d", Placidus, WholeSign, SavardA, HouseSystemCount)
	}
	seen := map[string]bool{}
	for n := 0; n < HouseSystemCount; n++ {
		h := HouseSystem(n)
		if !h.Valid() || h.StoredName() == "" || seen[h.StoredName()] {
			t.Fatalf("%d: valid=%v name=%q seen=%v", n, h.Valid(), h.StoredName(), seen[h.StoredName()])
		}
		seen[h.StoredName()] = true
		if back, ok := ParseHouseSystem(h.StoredName()); !ok || back != h {
			t.Errorf("%d: ParseHouseSystem(%q) = %d,%v", n, h.StoredName(), back, ok)
		}
	}
	for _, bad := range []HouseSystem{-1, 22, 99} {
		if bad.Valid() || bad.StoredName() != "" {
			t.Errorf("%d must be invalid", bad)
		}
	}
	if _, ok := ParseHouseSystem("Null"); ok {
		t.Error("Null is not an offered system")
	}
}
