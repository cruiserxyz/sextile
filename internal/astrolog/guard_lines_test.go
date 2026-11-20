package astrolog

import (
	"context"
	"errors"
	"math"
	"strings"
	"testing"
)

// The guard decides a line by every switch on it, not by its first: a line
// that holds a chart switch beside one Sextile does not pass is refused,
// naming that switch, whatever order the two come in. Dropping it would drop
// the zodiac or the place with it, silently or under a note.
func TestGuardReadsEverySwitchOnALine(t *testing.T) {
	head := "@AI800\n-qb Jun 1 1990 16:30 DT 1E 4:53E 52:22N\n"
	for _, tc := range []struct{ line, refused string }{
		{"=Yt -s Lahiri", "=Yt"},
		{"-zi \"A\" \"B\" =b0", "=b0"},
		// Not print-only, though they read like display switches: -YP moves
		// the Part of Fortune, and an offset after -sz moves every body.
		{"=YP -1", "=YP"},
		{"-sz 3", "-sz"},
	} {
		_, _, err := GuardChartFile(head + tc.line + "\n")
		var refused *ChartRefused
		if !errors.As(err, &refused) || refused.Switch != tc.refused {
			t.Errorf("%q: %v, want a refusal naming %s", tc.line, err, tc.refused)
		}
	}
	// A line of display and node switches only is left out in silence: the
	// Node switches never reach Astrolog and are said nothing about
	// (TestGuardTakesNodeSwitchesOut).
	for _, tc := range []struct {
		line  string
		notes int
	}{
		{"=Yt _Yd", 0},
		{"=Yt -", 0},
		{"=YZ 1", 0},
		{"_Yn =Yt", 0},
		{"-Yn", 0},
	} {
		got, notes, err := GuardChartFile(head + tc.line + "\n")
		if err != nil || got != head || len(notes) != tc.notes {
			t.Errorf("%q: kept %q, notes %v, err %v; want it left out with %d notes", tc.line, got, notes, err, tc.notes)
		}
	}
}

// A title or place longer than Astrolog reads back is refused before Astrolog
// sees it, wherever the line carries it (-zi, -zj, the end of -qc): Astrolog
// 8.00 crashes reading it (exit 139 from 100 bytes of title). The control
// reads the same files with the guard off and sees Astrolog crash.
func TestGuardRefusesALabelAstrologCannotRead(t *testing.T) {
	needAstrolog(t)
	long := strings.Repeat("N", 100)
	cases := []struct{ line, sw string }{
		{`-zi "` + long + `" "P"`, "-zi"},
		{`-zi "T" "` + long + `"`, "-zi"},
		{`-zj "` + long + `" "P"`, "-zj"},
		{`-qc Jun 1 1990 16:30 DT 1E 4:53E 52:22N "` + long + `" "P"`, "-qc"},
	}
	for _, tc := range cases {
		text := "@AI800\n-qb Jun 1 1990 16:30 DT 1E 4:53E 52:22N\n" + tc.line + "\n"
		_, err := ResolveChartBytes(context.Background(), []byte(text), "long.as")
		var refused *ChartRefused
		if !errors.As(err, &refused) || !refused.Long || refused.Switch != tc.sw {
			t.Errorf("%.20s...: %v, want a refusal for a long label naming %s", tc.line, err, tc.sw)
		}
	}
	if _, _, err := GuardChartFile("@AI800\n-qb Jun 1 1990 16:30 DT 1E 4:53E 52:22N\n-zi \"" + strings.Repeat("N", MaxLabelBytes) + "\" \"P\"\n"); err != nil {
		t.Errorf("a label of exactly %d bytes is refused: %v", MaxLabelBytes, err)
	}
	prev := guardChart
	t.Cleanup(func() { guardChart = prev })
	guardChart = func(text string) (string, []FileNote, error) { return text, nil, nil }
	// -zj sets the labels of Astrolog's default chart, which this read does
	// not print, so it does not crash; it is held to the same length.
	for _, tc := range []struct{ line, sw string }{cases[0], cases[1], cases[3]} {
		text := "@AI800\n-qb Jun 1 1990 16:30 DT 1E 4:53E 52:22N\n" + tc.line + "\n"
		if _, err := ResolveChartBytes(context.Background(), []byte(text), "long.as"); err == nil || !strings.Contains(err.Error(), "crashed") {
			t.Errorf("without the guard %.20s... did not crash Astrolog (%v): the test cannot see the guard", tc.line, err)
		}
	}
}

// Astrolog reads only a comment after the header's tag, and an allowed
// switch's argument only as a plain word: a `~` (with or without spaces before
// it) makes Astrolog run the rest as an AstroExpression, and an argument past
// its 255-byte buffer crashes it. Each refuses the chart, naming the switch.
// A title or place is text to Astrolog and may hold any of them.
func TestGuardReadsTheHeaderAndEveryArgument(t *testing.T) {
	head := "@AI800\n-qb Jun 1 1990 16:30 DT 1E 4:53E 52:22N\n"
	for _, tc := range []struct{ text, refused string }{
		{"@AI800 -o x.as\n-qb Jun 1 1990 16:30 DT 1E 4:53E 52:22N\n", "-o"},
		{"@AI800 =b0\n-qb Jun 1 1990 16:30 DT 1E 4:53E 52:22N\n", "=b0"},
		{head + "-c \"~Switch 1\"\n", "-c"},
		{head + "-c \" ~Switch 1\"\n", "-c"},
		{"@AI800\n-qb ~1 1 1990 16:30 DT 1E 4:53E 52:22N\n", "-qb"},
		{head + "-zl 4:53E ~1\n", "-zl"},
		{head + "-qj ~2447000\n", "-qj"},
		{head + "-zZ DT ~1\n", "-zZ"},
		{head + "-zl " + strings.Repeat("1", 300) + " 52N\n", "-zl"},
		{head + "=s \"~Switch 1\"\n", "=s"},
		{head + "=s " + strings.Repeat("1", 300) + "\n", "=s"},
		{head + "-c3 9\n", "-c3"},
		{head + "-s \"\"\n", "-s"},
		{head + "\"\"\n", `""`},
		{head + "-zl 4:53E 52:22N \"\"\n", `""`},
		// Astrolog reads 98 words of a line and ignores the rest, so the
		// last -qb here would be cast by Sextile and not by Astrolog.
		{"@AI800\n" + strings.Repeat("-qb Jun 1 1990 16:30 DT 1E 4:53E 52:22N ", 10) + "_s _s _s _s _s _s _s _s -qb Jul 2 2001 16:30 DT 1E 4:53E 52:22N\n", "-qb"},
	} {
		_, _, err := GuardChartFile(tc.text)
		var refused *ChartRefused
		if !errors.As(err, &refused) || refused.Switch != tc.refused {
			t.Errorf("%.40q: %v, want a refusal naming %.20s", tc.text, err, tc.refused)
		}
	}
	for _, ok := range []string{
		"@AI800  ; Astrolog chart info.\n-qb Jun 1 1990 16:30 DT 1E 4:53E 52:22N\n",
		head + "-zi \"~Switch 1\" \"Paris; -o x\"\n",
		head + "-c \"Pullen (SR, Vertex)\"\n",
		head + "-zl 77:13'00E 28:36'00N\n",
		head + "-zi \"\" \"\"\n",
		"@AI800\n" + strings.Repeat("-qb Jun 1 1990 16:30 DT 1E 4:53E 52:22N ", 10) + "_s _s _s _s _s _s _s _s\n",
	} {
		if _, _, err := GuardChartFile(ok); err != nil {
			t.Errorf("%q is refused: %v", ok, err)
		}
	}
}

// Astrolog reads four digits after a colon as minutes and seconds (`16:3045`
// is 16:30:45, `4E5320` is 4°53'20"); the list reads a file's clock and
// coordinates the same way.
func TestTheListReadsTheCompactMinutesAndSeconds(t *testing.T) {
	for clock, want := range map[string]bool{"16:3045": true, "16:30:45": true, "16:30": false, "4:30pm": false, "16:304": false} {
		if got := statesSeconds(clock); got != want {
			t.Errorf("statesSeconds(%q) = %v, want %v", clock, got, want)
		}
	}
	for tok, want := range map[string]float64{"4E5320": 4 + 53.0/60 + 20.0/3600, "4:5320W": -(4 + 53.0/60 + 20.0/3600), "52N2207": 52 + 22.0/60 + 7.0/3600, "4:53E": 4 + 53.0/60} {
		pos, neg := byte('E'), byte('W')
		if strings.ContainsAny(tok, "NS") {
			pos, neg = 'N', 'S'
		}
		got, ok := parseCoordinate(tok, pos, neg)
		if !ok || math.Abs(got-want) > 1e-9 {
			t.Errorf("parseCoordinate(%q) = %v %v, want %v", tok, got, ok, want)
		}
	}
}
