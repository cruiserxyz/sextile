package recipe

// the specification C2 and C6: the eleven zodiacs, in C2's order, with
// the record form a session and the preferences carry.

import "testing"

func TestZodiacRecordForms(t *testing.T) {
	want := []string{
		"tropical", "sidereal Fagan-Bradley", "sidereal Lahiri", "sidereal Deluce",
		"sidereal Raman", "sidereal Usha-Shasi", "sidereal Krishnamurti",
		"sidereal Djwhal Khul", "sidereal Yukteshwar", "sidereal J.N. Bhasin",
		"sidereal Galactic Center",
	}
	if ZodiacCount != len(want) {
		t.Fatalf("ZodiacCount = %d, want %d", ZodiacCount, len(want))
	}
	for i, w := range want {
		z := Zodiac(i)
		if got := z.RecordName(); got != w {
			t.Errorf("Zodiac(%d).RecordName = %q, want %q", i, got, w)
		}
		back, ok := ParseZodiacRecord(w)
		if !ok || back != z {
			t.Errorf("ParseZodiacRecord(%q) = %v, %v; want %v", w, back, ok, z)
		}
	}
	for _, bad := range []string{"", "Tropical", "Lahiri", "sidereal", "sidereal lahiri", "sidereal Nonsense", "tropical Lahiri", " tropical"} {
		if z, ok := ParseZodiacRecord(bad); ok {
			t.Errorf("ParseZodiacRecord(%q) = %v, true; want a refusal", bad, z)
		}
	}
	if Sidereal != FaganBradley || Tropical.IsSidereal() || !Lahiri.IsSidereal() || Zodiac(ZodiacCount).Valid() {
		t.Error("Sidereal must alias Fagan-Bradley; Tropical is not sidereal; the count is not a zodiac")
	}
	if Lahiri.Name() != "Lahiri" || Tropical.Name() != "Tropical" {
		t.Errorf("names: %q %q", Lahiri.Name(), Tropical.Name())
	}
}
