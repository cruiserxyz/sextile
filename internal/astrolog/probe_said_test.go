package astrolog

import (
	"context"
	"strings"
	"testing"

	"github.com/cruiserxyz/sextile/internal/testenv/astrodir"
)

// The probe keeps the line Astrolog wrote for an absent object, not only the
// file it names.
func TestProbeKeepsAstrologsLine(t *testing.T) {
	needAstrolog(t)
	astrodir.Reduced(t, []string{"sepm9606.se1"})
	av, err := ProbeAvailability(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	titan := -1
	for n, name := range av.Names {
		if name == "Titan" {
			titan = n
		}
	}
	if titan < 0 || av.Absent[titan] != "sepm9606.se1" {
		t.Fatalf("Titan absent as %v, want a probe that lacks sepm9606.se1", av.Absent)
	}
	said := av.Said[titan]
	if !strings.HasPrefix(said, "SwissEph file 'sepm9606.se1' not found in PATH '") {
		t.Errorf("Said[Titan] = %q, want Astrolog's SwissEph line naming sepm9606.se1", said)
	}
	if strings.ContainsAny(said, "\x1b\r\n") || said != strings.TrimSpace(said) {
		t.Errorf("Said[Titan] = %q carries control bytes or spaces at its ends", said)
	}
	for n, s := range av.Said {
		if !av.IsAbsent(n) {
			t.Errorf("Said holds %d, which is not absent", n)
		}
		if s == "" {
			t.Errorf("Said[%d] is an empty entry; the rule is no entry when Astrolog wrote nothing", n)
		}
	}
}

// The line is the first the object's own cast wrote, control bytes removed.
func TestFirstSaidLine(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", ""},
		{"Creating graphics chart in memory.\n\n", ""},
		{"first\nsecond\n", "first"},
		{"\x1b[31m  odd\x07 line \x1b[0m\r\nnext\n", "odd line"},
		{"other\nSwissEph file 'a.se1' not found in PATH '/x/'\n", "SwissEph file 'a.se1' not found in PATH '/x/'"},
	}
	for _, c := range cases {
		if got := firstSaidLine(c.in); got != c.want {
			t.Errorf("firstSaidLine(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
