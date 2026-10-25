// All chart values in this file are constructed synthetic test inputs.
package astrolog

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

// C4, C5: the argv seam refuses an over-long name AND place in both functions
// and never spawns. PATH is emptied, so a spawn attempt would answer
// ErrNotInstalled instead; the rows at exactly the bound (54 bytes, for a name
// and a place alike) get that answer, which proves the refusal is the length
// check and nothing else.
func TestSaveArgvRefusesALongLabel(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	at, over, accented := strings.Repeat("a", 54), strings.Repeat("a", 55), strings.Repeat("é", 28)
	save := func(name, place string) error {
		_, err := ChartRecord(context.Background(), ChartToSave{Name: name, Place: place,
			When: time.Date(1987, 3, 14, 16, 30, 0, 0, time.UTC), Lon: 4.9, Lat: 52.37})
		return err
	}
	retitle := func(name, place string) error {
		return RetitleTo(context.Background(), "src.as", name, place, "dest.as")
	}
	for _, fn := range []struct {
		name string
		call func(name, place string) error
	}{{"ChartRecord", save}, {"RetitleTo", retitle}} {
		for _, c := range []struct {
			name         string
			label, place string
			refused      bool
			field        string
			bytes        int
		}{
			{"name at 54", at, "Amsterdam", false, "", 0},
			{"place at 54", "Test", at, false, "", 0},
			{"name 55", over, "Amsterdam", true, "name", 55},
			{"place 55", "Test", over, true, "place", 55},
			{"name 28 accented", accented, "Amsterdam", true, "name", 56},
			{"place 28 accented", "Test", accented, true, "place", 56},
		} {
			t.Run(fn.name+"/"+c.name, func(t *testing.T) {
				err := fn.call(c.label, c.place)
				var long *LabelTooLongError
				if !c.refused {
					if !errors.Is(err, ErrNotInstalled) {
						t.Fatalf("a label at the bound got %v, want to reach the spawn (ErrNotInstalled)", err)
					}
					return
				}
				if !errors.As(err, &long) {
					t.Fatalf("an over-long %s got %v, want *LabelTooLongError", c.field, err)
				}
				want := c.field
				if fn.name == "RetitleTo" && want == "name" {
					want = "title" // RetitleTo calls the name a title
				}
				if long.Bytes != c.bytes || long.What != want {
					t.Errorf("error says %q %d bytes, want %s %d", long.What, long.Bytes, want, c.bytes)
				}
			})
		}
	}
}

// C6: a name AND a place at exactly the bound save, retitle, and are read back
// by the live binary with exit 0 (the bound is below the 104/88 crash points).
func TestNameAtTheBoundRoundTrips(t *testing.T) {
	needAstrolog(t)
	name, place := strings.Repeat("N", 54), strings.Repeat("P", 54)
	record, err := ChartRecord(context.Background(), ChartToSave{Name: name, Place: place,
		When: time.Date(1987, 3, 14, 16, 30, 0, 0, time.UTC), Lon: 4.9, Lat: 52.3667})
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "bound.as")
	if err := os.WriteFile(path, record, 0o600); err != nil {
		t.Fatal(err)
	}
	cf, err := ResolveChartFile(context.Background(), path)
	if err != nil {
		t.Fatalf("Astrolog cannot read back a name and place at the bound: %v", err)
	}
	if cf.Name != name || cf.PlaceLabel != place {
		t.Errorf("read back %d/%d bytes, want 54/54", len(cf.Name), len(cf.PlaceLabel))
	}
	dest := filepath.Join(dir, "retitled.as")
	if err := os.WriteFile(dest, []byte("placeholder"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := RetitleTo(context.Background(), path, strings.Repeat("T", 54), strings.Repeat("Q", 54), dest); err != nil {
		t.Fatal(err)
	}
	if cf, err = ResolveChartFile(context.Background(), dest); err != nil || len(cf.Name) != 54 || len(cf.PlaceLabel) != 54 {
		t.Errorf("retitled at the bound: %+v, %v", cf, err)
	}
}

// CutLabel's output is always valid UTF-8 within the bound, even when the input
// held an invalid byte before the cut.
func TestCutLabelIsValidUTF8(t *testing.T) {
	for _, in := range []string{
		strings.Repeat("a", 60) + "\xff" + strings.Repeat("b", 20),
		strings.Repeat("é", 40) + "\xc3",
		"short\xff",
	} {
		got := CutLabel(in)
		if !utf8.ValidString(got) || len(got) > MaxLabelBytes {
			t.Errorf("CutLabel(%q) = %q (%d bytes), want valid UTF-8 within %d", in, got, len(got), MaxLabelBytes)
		}
	}
	if got := labelText("a\xffb"); got != "" {
		t.Errorf("labelText of an invalid label = %q, want it sent empty", got)
	}
}

// One bound serves a name and a place, and it is pinned here so a change to it
// is a decision, not a drift. PrintHeader prints a place into an 80-byte buffer
// along with the location string (62 bytes safe, 56 with seconds shown) and a
// pair report overflows on the second chart's name of 55 (see maxLabelBytes).
func TestOneBoundServesNameAndPlace(t *testing.T) {
	if MaxLabelBytes != 54 {
		t.Fatalf("the bound is %d, want 54 for a name and a place", MaxLabelBytes)
	}
	for _, c := range []struct {
		name, in, want string
	}{
		{"54 bytes is kept whole", strings.Repeat("a", 54), strings.Repeat("a", 54)},
		{"55 bytes is cut", strings.Repeat("a", 55), strings.Repeat("a", 51) + "…"},
		{"a rune straddling byte 54 is not split", strings.Repeat("a", 52) + "日本", strings.Repeat("a", 51) + "…"},
	} {
		for kind, got := range map[string]string{"name": labelText(c.in), "place": labelArgv("n", c.in, true)[2], "cut": CutLabel(c.in)} {
			if got != c.want || len(got) > MaxLabelBytes {
				t.Errorf("%s as a %s = %q (%d bytes), want %q", c.name, kind, got, len(got), c.want)
			}
		}
	}
	// The staged form applies it to the place too.
	if got := stagedZI("n", strings.Repeat("p", 80)); !strings.Contains(got, `"`+strings.Repeat("p", 51)+`…"`) {
		t.Errorf("staged -zi line = %q, want the cut place", got)
	}
	var tooLong *LabelTooLongError
	for _, what := range []string{"name", "title", "place"} {
		if err := CheckLabelLength(what, strings.Repeat("a", 55)); !errors.As(err, &tooLong) || tooLong.Bytes != 55 {
			t.Errorf("a 55-byte %s got %v, want a LabelTooLongError", what, err)
		}
		if err := CheckLabelLength(what, strings.Repeat("a", 54)); err != nil {
			t.Errorf("a 54-byte %s was refused: %v", what, err)
		}
	}
}
