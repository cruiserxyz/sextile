package astrolog

// the specification: a chart's name and place label reach
// Astrolog's own report header, through the argv for a single chart and through
// the staged chart files for a pair, and change no parsed value.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/cruiserxyz/sextile/internal/recipe"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// ---- fixtures ----------------------------------------------------------

const (
	nameJD = 2461314.5 // 2026-10-01 00:00 UT
	nameTZ = "Europe/Amsterdam"
)

func namedMoment(name, place string) recipe.MomentAtPlace {
	return recipe.MomentAtPlace{JD: nameJD, Lon: 4.8833, Lat: 52.3667, TZ: nameTZ, Name: name, Place: place}
}

func namedLocal(jd float64, name, place string) recipe.LocalMomentAtPlace {
	return recipe.LocalMomentAtPlace{JD: jd, Lon: 4.8833, Lat: 52.3667, TZ: nameTZ, Name: name, Place: place}
}

func namedMomentAt(jd float64, lon, lat float64, name, place string) recipe.MomentAtPlace {
	return recipe.MomentAtPlace{JD: jd, Lon: lon, Lat: lat, Name: name, Place: place}
}

// staged returns the content of every staged chart file a compiled invocation
// carries.
func stagedContents(inv Invocation) []string {
	var out []string
	for _, ti := range inv.TempInputs {
		out = append(out, ti.Content)
	}
	return out
}

// ziIn returns the operands of every `-zi` token pair in an argv.
func ziIn(argv []string) [][2]string {
	var out [][2]string
	for i := 0; i+2 < len(argv); i++ {
		if argv[i] == "-zi" {
			out = append(out, [2]string{argv[i+1], argv[i+2]})
		}
	}
	return out
}

// ziLines are the `-zi` lines of a staged chart file.
var reZILine = regexp.MustCompile(`(?m)^-zi .*$`)

func ziLinesOf(content string) []string { return reZILine.FindAllString(content, -1) }

// reportHeader returns the first two lines of a plain report.
func reportHeader(t *testing.T, report string) (first, second string) {
	t.Helper()
	lines := strings.Split(report, "\n")
	if len(lines) < 2 {
		t.Fatalf("no two-line header in %q", report)
	}
	return lines[0], lines[1]
}

func castSpec(t *testing.T, src recipe.Source) recipe.ChartSpec {
	return fixtureSpec(t, src, recipe.Placidus, recipe.Tropical)
}

// ---- C1 ----------------------------------------------------------------

func TestReportCallsCarryChartNames(t *testing.T) {
	const name, place = "Synthetic Aster", "Amsterdam, NL"
	t.Run("a single chart carries -zi on the argv", func(t *testing.T) {
		for _, tc := range []struct {
			src  recipe.Source
			proj recipe.Projection
		}{
			{namedMoment(name, place), recipe.Positions{}},
			{namedMoment(name, place), recipe.Aspects{}},
			{namedMoment(name, place), recipe.ChartRawReport()},
			{namedLocal(nameJD, name, place), recipe.ChartRawReport()},
			{namedLocal(nameJD, name, place), recipe.BodyRawReport(aspect.Sun)},
			{namedLocal(nameJD, name, place), recipe.AspectsRawReport()},
		} {
			label := fmt.Sprintf("%T/%s", tc.src, tc.proj.Label())
			inv := compileOne(t, castSpec(t, tc.src), tc.proj)
			for what, argv := range map[string][]string{"argv": inv.Argv, "call": inv.Call().Argv()} {
				got := ziIn(argv)
				if len(got) != 1 || got[0] != [2]string{name, place} {
					t.Errorf("%s %s: -zi operands %q, want exactly [%q %q]: %q", label, what, got, name, place, argv)
				}
			}
		}
	})
	t.Run("a pair carries -zi in each staged file", func(t *testing.T) {
		for _, rel := range []recipe.Relation{recipe.Synastry, recipe.TransitToNatal} {
			for _, local := range []bool{false, true} {
				var a, b recipe.Source = namedMoment("Aster", "Amsterdam, NL"), namedMomentAt(nameJD+30, 18.95, 69.6, "Birch", "Tromso, NO")
				if local {
					a, b = namedLocal(nameJD, "Aster", "Amsterdam, NL"), namedLocal(nameJD+30, "Birch", "Tromso, NO")
				}
				spec := castSpec(t, recipe.Comparison{Primary: a, Secondary: b, Relation: rel})
				for _, p := range []recipe.Projection{recipe.Aspects{}, recipe.AspectsRawReport(), recipe.BodyRawReport(aspect.Sun)} {
					inv := compileOne(t, spec, p)
					files := stagedContents(inv)
					if len(files) != 2 {
						t.Fatalf("rel %v local %v %s: %d staged files", rel, local, p.Label(), len(files))
					}
					for i, want := range []string{`-zi "Aster" "Amsterdam, NL"`, `-zi "Birch" "Tromso, NO"`} {
						if got := ziLinesOf(files[i]); !slices.Equal(got, []string{want}) {
							t.Errorf("rel %v local %v %s file %d: -zi lines %q, want %q", rel, local, p.Label(), i+1, got, want)
						}
					}
					if len(ziIn(inv.Argv)) != 0 {
						t.Errorf("a pair's names travel in its files, not its argv: %q", inv.Argv)
					}
				}
			}
		}
	})
}

// A pair's console lines show staged-file paths, never a name: the ephemeris
// probe that runs beside the relationship carries no label either.
func TestPairCommandsCarryNoName(t *testing.T) {
	needAstrolog(t)
	spec := castSpec(t, recipe.Comparison{
		Primary:   namedMomentAt(2449000.5, 4.9, 52.37, "Synthetic Aster", "Amsterdam, NL"),
		Secondary: namedMomentAt(nameJD, 4.9, 52.37, "Synthetic Birch", "Tromso, NO"),
		Relation:  recipe.TransitToNatal,
	})
	out, err := ComputeCrossAspects(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Commands) != 2 {
		t.Fatalf("commands = %d, want probe + relationship", len(out.Commands))
	}
	for _, c := range out.Commands {
		if strings.Contains(c, "Synthetic") || strings.Contains(c, "Amsterdam") || strings.Contains(c, "Tromso") {
			t.Errorf("a pair's command carries a label: %s", c)
		}
	}
}

// ---- label rule (C2, C8, N27) -------------------------------------------

func TestLabelTextRule(t *testing.T) {
	for _, tc := range []struct{ name, in, want string }{
		{"plain", "Synthetic Aster", "Synthetic Aster"},
		{"quotes and backslash survive", `a"b'c\d;e`, `a"b'c\d;e`},
		{"leading dash survives", "-v", "-v"},
		{"combining marks survive", "Jose\u0301 \u00c7elik \u65e5\u672c", "Jose\u0301 \u00c7elik \u65e5\u672c"},
		{"NUL is a space", "a\x00b", "a b"},
		{"newline is a space", "a\nb", "a b"},
		{"DEL is a space", "a\x7fb", "a b"},
		{"U+0085 is a space", "a\u0085b", "a b"},
		{"U+009F is a space", "a\u009fb", "a b"},
		{"U+00A0 is not a control", "a\u00a0b", "a\u00a0b"},
		{"invalid UTF-8 is empty", "a\xf8b", ""},
		{"exactly 54 bytes is kept", strings.Repeat("a", 54), strings.Repeat("a", 54)},
		{"55 bytes is cut", strings.Repeat("a", 55), strings.Repeat("a", 51) + "…"},
		{"a rune straddling byte 54 is not split", strings.Repeat("a", 52) + "日本", strings.Repeat("a", 51) + "…"},
		{"a control is a space before the cut", strings.Repeat("\n", 70), strings.Repeat(" ", 51) + "…"},
	} {
		if got := labelText(tc.in); got != tc.want {
			t.Errorf("%s: labelText(%q) = %q, want %q", tc.name, tc.in, got, tc.want)
		}
	}
}

// ---- C2: names survive Astrolog -----------------------------------------

type labelCase struct {
	name, label string
	want        string // what the header must print; "" means label itself
}

func (c labelCase) printed() string {
	if c.want != "" {
		return c.want
	}
	return c.label
}

var labelClasses = []labelCase{
	{name: "plain", label: "Synthetic Aster"},
	{name: "double quote", label: `Dou"ble Quote`},
	{name: "single quote", label: `O'Brien`},
	{name: "backslash", label: `back\slash \n`},
	{name: "semicolon", label: "semi;colon ; ;"},
	{name: "leading dash", label: "-v -a"},
	{name: "leading dashes", label: "--help"},
	{name: "non-ASCII letters", label: "Zoë Çelik Łukasz Ωmega"},
	{name: "CJK and emoji", label: "日本語 🌙"},
	{name: "combining marks", label: "Jose\u0301 Ange\u0300le"},
	{name: "inner spaces", label: "a  b   c"},
}

func headerNameOf(t *testing.T, first string) string {
	t.Helper()
	const prefix = "Astrolog 8.00 chart for "
	if !strings.HasPrefix(first, prefix) {
		t.Fatalf("first header line %q does not begin %q", first, prefix)
	}
	return first[len(prefix):]
}

func TestChartNamesSurviveAstrolog(t *testing.T) {
	needAstrolog(t)
	ctx := context.Background()
	single := func(t *testing.T, name, place string) (string, string) {
		t.Helper()
		spec := castSpec(t, namedLocal(nameJD, name, place))
		report, _, err := ComputeRawReport(ctx, spec, recipe.AspectsRawReport())
		if err != nil {
			t.Fatalf("single %q/%q: %v", name, place, err)
		}
		return reportHeader(t, report)
	}
	pair := func(t *testing.T, n1, p1, n2, p2 string) (string, string, string) {
		t.Helper()
		spec := castSpec(t, recipe.Comparison{
			Primary:   namedLocal(nameJD, n1, p1),
			Secondary: namedLocal(nameJD+30, n2, p2),
			Relation:  recipe.Synastry,
		})
		report, _, err := ComputeRawReport(ctx, spec, recipe.AspectsRawReport())
		if err != nil {
			t.Fatalf("pair %q/%q: %v", n1, n2, err)
		}
		var second string
		for _, l := range strings.Split(report, "\n") {
			if strings.HasPrefix(l, "Chart #2 comparison for ") {
				second = l
			}
		}
		first, moment := reportHeader(t, report)
		_ = moment
		return first, second, report
	}
	for _, c := range labelClasses {
		t.Run(c.name+"/argv", func(t *testing.T) {
			first, second := single(t, c.label, "Pl "+c.label)
			if got := headerNameOf(t, first); got != c.printed() {
				t.Errorf("name printed %q, want %q", got, c.printed())
			}
			if !strings.Contains(second, "Pl "+c.printed()) {
				t.Errorf("place not printed whole: %q", second)
			}
		})
		t.Run(c.name+"/staged", func(t *testing.T) {
			first, second, report := pair(t, c.label, "Pl "+c.label, "Other "+c.label, "Q "+c.label)
			if got := headerNameOf(t, first); got != c.printed() {
				t.Errorf("chart 1 name printed %q, want %q", got, c.printed())
			}
			if want := "Chart #2 comparison for Other " + c.printed(); second != want {
				t.Errorf("chart 2 header %q, want %q", second, want)
			}
			if !strings.Contains(report, "Pl "+c.printed()) || !strings.Contains(report, "Q "+c.printed()) {
				t.Errorf("a place was not printed whole in\n%.400s", report)
			}
		})
	}

	t.Run("a label holding both quote kinds loses its double quotes in a staged file only", func(t *testing.T) {
		const label = `Both "dq" and 'sq'`
		first, _ := single(t, label, "p")
		if got := headerNameOf(t, first); got != label {
			t.Errorf("argv: printed %q, want %q", got, label)
		}
		want := `Both dq and 'sq'`
		first, second, _ := pair(t, label, "p", label, "q")
		if got := headerNameOf(t, first); got != want {
			t.Errorf("staged chart 1: printed %q, want %q", got, want)
		}
		if second != "Chart #2 comparison for "+want {
			t.Errorf("staged chart 2: %q, want %q", second, want)
		}
	})

	t.Run("every control rune is one space, through argv and a staged file", func(t *testing.T) {
		var controls []rune
		for r := rune(0); r <= 0x1f; r++ {
			controls = append(controls, r)
		}
		controls = append(controls, 0x7f)
		for r := rune(0x80); r <= 0x9f; r++ {
			controls = append(controls, r)
		}
		for _, r := range controls {
			label := "A" + string(r) + "B"
			first, _ := single(t, label, "P"+string(r)+"Q")
			if got := headerNameOf(t, first); got != "A B" {
				t.Errorf("U+%04X argv: printed %q, want %q", r, got, "A B")
			}
			first, second, _ := pair(t, label, "p", label, "q")
			if got := headerNameOf(t, first); got != "A B" {
				t.Errorf("U+%04X staged chart 1: printed %q", r, got)
			}
			if second != "Chart #2 comparison for A B" {
				t.Errorf("U+%04X staged chart 2: %q", r, second)
			}
		}
	})

	t.Run("every non-control rune is preserved", func(t *testing.T) {
		// Whole blocks, in batches short enough to stay under the byte bound.
		var runes []rune
		add := func(lo, hi rune) {
			for r := lo; r <= hi; r++ {
				if unicode.Is(unicode.Cc, r) || r == '"' || r == '\'' || !utf8.ValidRune(r) {
					continue
				}
				runes = append(runes, r)
			}
		}
		add(0x20, 0x7e)
		add(0xa0, 0x24f)
		add(0x300, 0x36f)
		add(0x370, 0x3ff)
		add(0x400, 0x4ff)
		add(0x2000, 0x206f) // spaces, dashes, quotation marks, U+2028 and U+2029 included
		add(0x2190, 0x21ff)
		add(0x2600, 0x26ff)
		add(0x3040, 0x30ff)
		add(0x1f300, 0x1f32f)
		checked := 0
		for len(runes) > 0 {
			n := min(len(runes), 12)
			batch := string(runes[:n])
			runes = runes[n:]
			if !utf8.ValidString(batch) || len(batch) > 64 {
				t.Fatalf("bad batch %q", batch)
			}
			label := "<" + batch + ">"
			// Astrolog may trim or fold a label's edge whitespace; the brackets pin the edges.
			first, _ := single(t, label, label)
			if got := headerNameOf(t, first); got != label {
				t.Errorf("argv batch %U: printed %q, want %q", []rune(batch)[0], got, label)
			}
			first, _, _ = pair(t, label, label, label, label)
			if got := headerNameOf(t, first); got != label {
				t.Errorf("staged batch %U: printed %q, want %q", []rune(batch)[0], got, label)
			}
			checked += n
		}
		if checked < 900 {
			t.Fatalf("only %d runes checked", checked)
		}
	})
}

// ---- C8: long labels never crash Astrolog ------------------------------

func TestLongLabelsNeverCrashAstrolog(t *testing.T) {
	needAstrolog(t)
	ctx := context.Background()
	// shapeOf builds a label of exactly n bytes whose letter is c, so labels in
	// different roles never share a prefix. The multi-byte runes straddle the
	// bound, which is the same for a name and a place.
	shapeOf := func(c, kind string, n, bound int) string {
		switch kind {
		case "ascii":
			return strings.Repeat(c, n)
		case "two-byte": // é occupies the last two bytes up to the bound
			return strings.Repeat(c, bound-1) + strings.Repeat("é", (n-bound+1)/2) + strings.Repeat(c, (n-bound+1)%2)
		default: // 日 occupies the last three bytes up to the bound
			return strings.Repeat(c, bound-2) + strings.Repeat("日", (n-bound+2)/3) + strings.Repeat(c, (n-bound+2)%3)
		}
	}
	shape := func(kind string, n int) string { return shapeOf("a", kind, n, maxLabelBytes) }
	// wantCut is the exact cut: the longest rune-boundary prefix that fits in
	// bound bytes together with the ellipsis, or the label whole if it fits.
	wantCut := func(in string, bound int) string {
		if len(in) <= bound {
			return in
		}
		stem, size := "", 0
		for _, r := range in {
			if size+utf8.RuneLen(r) > bound-len("…") {
				break
			}
			size += utf8.RuneLen(r)
			stem += string(r)
		}
		return stem + "…"
	}
	// checkHeader requires the label line to be exactly prefix + the exact cut
	// of the name, and the moment line to print the exact cut of the place
	// right after the zone and, after the delimiter Astrolog prints (three
	// spaces), nothing but the coordinates.
	const first, second = "Astrolog 8.00 chart for ", "Chart #2 comparison for "
	reCoords := `   [0-9]+°[0-9]+[EW] [0-9]+°[0-9]+[NS]$`
	checkHeader := func(t *testing.T, what, kind string, n int, prefix, inName, inPlace string, lines []string, nameLine, momentLine int) {
		t.Helper()
		wantName, wantPlace := wantCut(inName, maxLabelBytes), wantCut(inPlace, maxLabelBytes)
		if got := lines[nameLine]; got != prefix+wantName {
			t.Fatalf("%s %s %d: label line %q, want exactly %q (the exact cut is %d bytes)", what, kind, n, got, prefix+wantName, len(wantName))
		}
		if !regexp.MustCompile(`\) ` + regexp.QuoteMeta(wantPlace) + reCoords).MatchString(lines[momentLine]) {
			t.Fatalf("%s %s %d: moment line %q is not the exact cut place %q then the coordinates", what, kind, n, lines[momentLine], wantPlace)
		}
	}
	kinds := []string{"ascii", "two-byte", "three-byte"}
	// other is a different length and shape from (kind, n), over the bound up
	// to 300 bytes.
	lo := maxLabelBytes + 1
	other := func(i, n, skew int) string {
		return shapeOf(string(rune('a'+skew)), kinds[(i+skew)%3], lo+(n-lo+skew*37)%(300-lo+1), maxLabelBytes)
	}
	for i, kind := range kinds {
		for n := lo; n <= 300; n++ {
			label := shape(kind, n)
			if !utf8.ValidString(label) || len(label) != n {
				t.Fatalf("%s %d: fixture is %d bytes", kind, n, len(label))
			}
			place1, name2, place2 := other(i, n, 1), other(i, n, 2), other(i, n, 4)
			spec := castSpec(t, namedLocal(nameJD, label, place1))
			// The raw reports Details shows (-v, -aa, -aa -RO).
			for _, p := range []recipe.RawReport{recipe.ChartRawReport(), recipe.AspectsRawReport(), recipe.BodyRawReport(aspect.Sun)} {
				report, _, err := ComputeRawReport(ctx, spec, p)
				if err != nil {
					t.Fatalf("%s %d %s: %v%s", kind, n, p.Label(), err, crashed(err))
				}
				checkHeader(t, p.Label(), kind, n, first, label, place1, strings.Split(report, "\n"), 0, 1)
			}
			// The chart compute's positions and aspects calls.
			out, err := ComputeChart(ctx, castSpec(t, namedMoment(label, place1)))
			if err != nil {
				t.Fatalf("%s %d chart compute: %v%s", kind, n, err, crashed(err))
			}
			checkHeader(t, "chart compute", kind, n, first, label, place1, strings.Split(out.AspectsReport, "\n"), 0, 1)
			// Its positions call too (review round 3, N29): Raw is the positions
			// stdout, then "\n---\n", then the aspects stdout.
			positionsOut, _, found := strings.Cut(out.Raw, "\n---\n")
			if !found {
				t.Fatalf("%s %d chart compute: Raw holds no positions/aspects separator", kind, n)
			}
			checkHeader(t, "chart compute positions", kind, n, first, label, place1, strings.Split(reportText(positionsOut), "\n"), 0, 1)
			// A pair, through its staged files: each chart its OWN labels.
			pairSpec := castSpec(t, recipe.Comparison{Primary: namedLocal(nameJD, label, place1), Secondary: namedLocal(nameJD+9, name2, place2), Relation: recipe.Synastry})
			report, _, err := ComputeRawReport(ctx, pairSpec, recipe.AspectsRawReport())
			if err != nil {
				t.Fatalf("%s %d pair: %v%s", kind, n, err, crashed(err))
			}
			lines := strings.Split(report, "\n")
			checkHeader(t, "pair chart 1", kind, n, first, label, place1, lines, 0, 1)
			checkHeader(t, "pair chart 2", kind, n, second, name2, place2, lines, 2, 3)
		}
	}
	// The bound itself is kept whole: a name and a place of exactly 54 bytes
	// print as typed, no ellipsis.
	for _, kind := range kinds {
		name, place := shape(kind, maxLabelBytes), shapeOf("b", kind, maxLabelBytes, maxLabelBytes)
		if len(name) != maxLabelBytes || len(place) != maxLabelBytes {
			t.Fatalf("%s: fixture is %d and %d bytes", kind, len(name), len(place))
		}
		report, _, err := ComputeRawReport(ctx, castSpec(t, namedLocal(nameJD, name, place)), recipe.ChartRawReport())
		if err != nil {
			t.Fatalf("%s at the bounds: %v%s", kind, err, crashed(err))
		}
		checkHeader(t, "at the bounds", kind, 0, first, name, place, strings.Split(report, "\n"), 0, 1)
	}
}

// crashed names the signal that killed Astrolog, if one did, in words that do
// not depend on the host: Go's own text for a signal exit gains " (core
// dumped)" wherever the core-dump limit allows a core.
func crashed(err error) string {
	var exit *exec.ExitError
	if !errors.As(err, &exit) {
		return ""
	}
	ws, ok := exit.Sys().(syscall.WaitStatus)
	if !ok || !ws.Signaled() {
		return ""
	}
	name := map[syscall.Signal]string{syscall.SIGSEGV: "SIGSEGV", syscall.SIGBUS: "SIGBUS", syscall.SIGABRT: "SIGABRT"}[ws.Signal()]
	if name == "" {
		name = fmt.Sprintf("signal %d", int(ws.Signal()))
	}
	return " (Astrolog crashed: killed by " + name + ")"
}

// ---- C3: names change no parsed value ----------------------------------

func TestNamesChangeNoParsedValue(t *testing.T) {
	needAstrolog(t)
	ctx := context.Background()
	objects := recipe.ObjectPolicy{Baseline: recipe.AstrologDefault, Add: recipe.NewBodySet(aspect.Chiron)}
	spec := func(src recipe.Source, houses recipe.HouseSystem) recipe.ChartSpec {
		s, err := recipe.NewChartSpec(src, houses, recipe.Tropical, objects)
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	t.Run("a single chart", func(t *testing.T) {
		bare, err := ComputeChart(ctx, spec(namedMoment("", ""), recipe.Placidus))
		if err != nil {
			t.Fatal(err)
		}
		if len(bare.Positions) == 0 || len(bare.Aspects) == 0 || bare.Cusps == (aspect.Cusps{}) || bare.HousesUsed == "" {
			t.Fatalf("the unnamed control is empty: %d positions, %d aspects, cusps %v, houses %q", len(bare.Positions), len(bare.Aspects), bare.Cusps, bare.HousesUsed)
		}
		for n := 0; n < recipe.HouseSystemCount; n++ {
			houses := recipe.HouseSystem(n)
			plain, err := ComputeChart(ctx, spec(namedMoment("", ""), houses))
			if err != nil {
				t.Fatalf("%s: %v", houses.StoredName(), err)
			}
			for _, name := range []string{"Synthetic Aster", "Veloc. " + houses.StoredName() + " Houses", "Veloc. " + recipe.HouseSystem((n+1)%recipe.HouseSystemCount).StoredName() + " Houses"} {
				got, err := ComputeChart(ctx, spec(namedMoment(name, "Veloc. Whole Houses"), houses))
				if err != nil {
					t.Fatalf("%q: %v", name, err)
				}
				for what, pair := range map[string][2]any{
					"positions": {got.Positions, plain.Positions}, "cusps": {got.Cusps, plain.Cusps},
					"aspects": {got.Aspects, plain.Aspects}, "body styles": {got.BodyStyles, plain.BodyStyles},
					"aspect styles": {got.AspectStyles, plain.AspectStyles}, "unavailable": {got.Unavailable, plain.Unavailable},
					"houses used": {got.HousesUsed, plain.HousesUsed},
				} {
					if !reflect.DeepEqual(pair[0], pair[1]) {
						t.Errorf("%s named %q: %s differ:\n named %v\n bare  %v", houses.StoredName(), name, what, pair[0], pair[1])
					}
				}
			}
		}
	})
	t.Run("unavailable bodies", func(t *testing.T) {
		hyg := recipe.ObjectPolicy{Baseline: recipe.Explicit, Selection: []int{1, 43}}
		mk := func(name string) recipe.ChartSpec {
			s, err := recipe.NewChartSpec(namedMoment(name, name), recipe.WholeSign, recipe.Tropical, hyg)
			if err != nil {
				t.Fatal(err)
			}
			return s
		}
		bare, err := ComputeChart(ctx, mk(""))
		if err != nil {
			t.Fatal(err)
		}
		named, err := ComputeChart(ctx, mk("Veloc. Whole Houses"))
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(bare.Unavailable, named.Unavailable) || !reflect.DeepEqual(bare.Positions, named.Positions) {
			t.Errorf("unavailable %v -> %v, positions differ %v", bare.Unavailable, named.Unavailable, !reflect.DeepEqual(bare.Positions, named.Positions))
		}
	})
	t.Run("a pair", func(t *testing.T) {
		mk := func(a, b string) recipe.ChartSpec {
			return spec(recipe.Comparison{
				Primary:   namedMomentAt(2449000.5, 4.9, 52.37, a, a),
				Secondary: namedMomentAt(nameJD, 4.9, 52.37, b, b),
				Relation:  recipe.TransitToNatal,
			}, recipe.Placidus)
		}
		bare, err := ComputeCrossAspects(ctx, mk("", ""))
		if err != nil {
			t.Fatal(err)
		}
		if len(bare.Aspects) == 0 {
			t.Fatal("the unnamed control has no cross-aspects")
		}
		named, err := ComputeCrossAspects(ctx, mk("Synthetic Aster", "Synthetic Birch"))
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(named.Aspects, bare.Aspects) || !reflect.DeepEqual(named.AspectStyles, bare.AspectStyles) {
			t.Errorf("cross-aspects moved with names")
		}
	})
}

// ---- C4 / N15: the two-line header -------------------------------------

func TestNamedHeaderParses(t *testing.T) {
	needAstrolog(t)
	ctx := context.Background()
	t.Run("a single chart's rows equal the one-line form's", func(t *testing.T) {
		bare, err := ComputeChart(ctx, castSpec(t, namedMoment("", "")))
		if err != nil {
			t.Fatal(err)
		}
		named, err := ComputeChart(ctx, castSpec(t, namedMoment("Synthetic Aster", "Amsterdam, NL")))
		if err != nil {
			t.Fatal(err)
		}
		if len(bare.Positions) == 0 || len(bare.Aspects) == 0 {
			t.Fatal("the one-line control parsed nothing")
		}
		if !reflect.DeepEqual(parsePositions(named.Raw), parsePositions(bare.Raw)) || !reflect.DeepEqual(parseAspects(named.Raw), parseAspects(bare.Raw)) {
			t.Error("named rows differ from the one-line form's")
		}
		if first, _ := reportHeader(t, named.AspectsReport); !strings.HasSuffix(first, "Synthetic Aster") {
			t.Errorf("the named run did not print the two-line header: %q", first)
		}
	})
	t.Run("a pair's cross-aspects equal the one-line form's, and the sanity check accepts it", func(t *testing.T) {
		mk := func(a, b string) recipe.ChartSpec {
			return castSpec(t, recipe.Comparison{
				Primary:   namedMomentAt(2449000.5, 4.9, 52.37, a, a),
				Secondary: namedMomentAt(nameJD, 4.9, 52.37, b, b),
				Relation:  recipe.TransitToNatal,
			})
		}
		bare, err := ComputeCrossAspects(ctx, mk("", ""))
		if err != nil {
			t.Fatal(err)
		}
		if len(bare.Aspects) == 0 {
			t.Fatal("the one-line control parsed no cross-aspects")
		}
		named, err := ComputeCrossAspects(ctx, mk("Synthetic Aster", "Synthetic Birch"))
		if err != nil {
			t.Fatalf("a named pair was refused: %v", err)
		}
		if !reflect.DeepEqual(named.Aspects, bare.Aspects) {
			t.Error("named cross-aspects differ from the one-line form's")
		}
	})
	t.Run("a name holding comparison for does not fake a chart-2 header", func(t *testing.T) {
		// A real pair's output with chart 2's header taken out: what remains is
		// a relationship list with NO chart-2 header, and chart 1's name says
		// the words the check looks for.
		cross, err := ComputeCrossAspects(ctx, castSpec(t, recipe.Comparison{
			Primary:   namedMomentAt(2449000.5, 4.9, 52.37, "Bob comparison for Al", "p"),
			Secondary: namedMomentAt(nameJD, 4.9, 52.37, "Other", "q"),
			Relation:  recipe.TransitToNatal,
		}))
		if err != nil {
			t.Fatal(err)
		}
		var kept []string
		dropped := 0
		for _, l := range strings.Split(cross.Report, "\n") {
			if strings.HasPrefix(l, "Chart #2 comparison for ") {
				dropped++
				continue
			}
			kept = append(kept, l)
		}
		if dropped != 1 {
			t.Fatalf("fixture: %d chart-2 headers in\n%.300s", dropped, cross.Report)
		}
		text := strings.Join(kept, "\n")
		if !strings.Contains(text, "comparison for Al") {
			t.Fatalf("fixture: chart 1's name is not in the text:\n%.300s", text)
		}
		if err := checkCrossAspectSanity(text); err == nil {
			t.Fatalf("a relationship list with no chart-2 header passed on chart 1's NAME alone:\n%.300s", text)
		}
	})
}

// ---- C6 / N12 ----------------------------------------------------------

func TestUnnamedChartsKeepTheOneLineHeader(t *testing.T) {
	needAstrolog(t)
	ctx := context.Background()
	oneLine := regexp.MustCompile(`^Astrolog 8\.00 chart for [A-Z][a-z]{2} [A-Z][a-z]{2} [ 0-9]{2}, -?[0-9]+ [ 0-9]{1,2}:[0-9]{2} \(`)
	secondLine := regexp.MustCompile(`^Chart #2 comparison for [A-Z][a-z]{2} [A-Z][a-z]{2} [ 0-9]{2}, -?[0-9]+ [ 0-9]{1,2}:[0-9]{2} \(`)
	observe := func(t *testing.T) {
		t.Helper()
		for _, src := range []recipe.Source{namedLocal(nameJD, "", ""), namedMoment("", "")} {
			var report string
			if _, local := src.(recipe.LocalMomentAtPlace); local {
				r, _, err := ComputeRawReport(ctx, castSpec(t, src), recipe.AspectsRawReport())
				if err != nil {
					t.Fatal(err)
				}
				report = r
			} else {
				c, err := ComputeChart(ctx, castSpec(t, src))
				if err != nil {
					t.Fatal(err)
				}
				report = c.AspectsReport
			}
			if first, _ := reportHeader(t, report); !oneLine.MatchString(first) {
				t.Errorf("%T: header %q is not today's one-line form", src, first)
			}
		}
		pair := castSpec(t, recipe.Comparison{Primary: namedLocal(nameJD, "", ""), Secondary: namedLocal(nameJD+3, "", ""), Relation: recipe.Synastry})
		report, _, err := ComputeRawReport(ctx, pair, recipe.AspectsRawReport())
		if err != nil {
			t.Fatal(err)
		}
		first, _ := reportHeader(t, report)
		if !oneLine.MatchString(first) {
			t.Errorf("pair chart 1 header %q", first)
		}
		ok := false
		for _, l := range strings.Split(report, "\n") {
			ok = ok || secondLine.MatchString(l)
		}
		if !ok {
			t.Errorf("pair has no one-line chart-2 header:\n%.300s", report)
		}
	}
	t.Run("clean data dir", func(t *testing.T) {
		useDataDir(t, dataDirWith(t))
		observe(t)
	})
	t.Run("a host astrolog.as that supplies a -zi", func(t *testing.T) {
		useDataDir(t, dataDirWith(t, `-zi "Host Label" "Host Place"`))
		// The control: the host label does reach an unpinned call.
		out, _, err := spawn(ctx, testCall([]string{"-qj", "2461314.500000", "-zl", "4:53E", "52:22N", "-v0"}))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out, "Host Label") {
			t.Skipf("this Astrolog does not read -zi from astrolog.as; the control cannot be shown")
		}
		observe(t)
	})
}

// ---- C5a ---------------------------------------------------------------

func TestWheelCarriesNoChartName(t *testing.T) {
	const name, place = "Synthetic Aster", "Amsterdam, NL"
	wheel := recipe.WheelImage{Render: recipe.DefaultWheelRender()}
	// An empty `-zi "" ""` is the pin that keeps a host astrolog.as label off
	// the image; a NAME or PLACE is what must not appear.
	clean := func(argv []string) bool {
		for _, z := range ziIn(argv) {
			if z != [2]string{"", ""} {
				return false
			}
		}
		return !slices.Contains(argv, name) && !slices.Contains(argv, place)
	}
	leak := func(t *testing.T, what string, inv Invocation) {
		t.Helper()
		for _, argv := range [][]string{inv.Argv, inv.Call().Argv()} {
			if !clean(argv) {
				t.Errorf("%s: the wheel argv carries a label: %q", what, argv)
			}
		}
		for i, c := range stagedContents(inv) {
			if strings.Contains(c, name) || strings.Contains(c, place) || strings.Contains(c, "Birch") {
				t.Errorf("%s: staged file %d carries a label:\n%s", what, i+1, c)
			}
		}
	}
	sources := map[string]recipe.Source{
		"single":     namedMoment(name, place),
		"comparison": recipe.Comparison{Primary: namedMoment(name, place), Secondary: namedMomentAt(nameJD+1, 18.95, 69.6, "Birch", "Tromso"), Relation: recipe.Synastry},
		"local pair": recipe.Comparison{Primary: namedLocal(nameJD, name, place), Secondary: namedLocal(nameJD+1, "Birch", "Tromso")},
		"multiwheel": recipe.MultiWheel{Rings: []recipe.Source{namedMoment(name, place), namedMomentAt(nameJD+1, 18.95, 69.6, "Birch", "Tromso"), namedMomentAt(nameJD+2, 10.7, 59.9, name, place)}},
	}
	for what, src := range sources {
		spec := castSpec(t, src)
		leak(t, what, compileOne(t, spec, wheel))
		bundle, err := CompileBundle(spec, wheel)
		if err != nil {
			t.Fatalf("%s bundle: %v", what, err)
		}
		for _, plan := range bundle.Plans {
			for _, inv := range plan.Invocations {
				leak(t, what+" bundle", inv)
			}
		}
		if !clean(bundle.NormalizedPrefix) {
			t.Errorf("%s: the bundle prefix carries a label: %q", what, bundle.NormalizedPrefix)
		}
	}
}

// ---- C7 / N5 / N30 -----------------------------------------------------

func TestNamedStagedFilesAreRemoved(t *testing.T) {
	spec := castSpec(t, recipe.Comparison{
		Primary:   namedLocal(nameJD, "Synthetic Aster", "Amsterdam, NL"),
		Secondary: namedLocal(nameJD+30, "Synthetic Birch", "Tromso, NO"),
		Relation:  recipe.TransitToNatal,
	})
	inv := compileOne(t, spec, recipe.AspectsRawReport())
	leftovers := func(t *testing.T, tmp string) []string {
		t.Helper()
		var out []string
		_ = filepath.WalkDir(tmp, func(p string, d os.DirEntry, err error) error {
			if err == nil && p != tmp {
				out = append(out, p)
			}
			return nil
		})
		return out
	}
	t.Run("files are 0600 and named", func(t *testing.T) {
		tmp := t.TempDir()
		t.Setenv("TMPDIR", tmp)
		call, dir, cleanup, err := stageCall(inv)
		if err != nil {
			t.Fatal(err)
		}
		_ = call
		for i, want := range []string{"Synthetic Aster", "Synthetic Birch"} {
			p := filepath.Join(dir, fmt.Sprintf("chart%d.as", i))
			fi, err := os.Stat(p)
			if err != nil {
				t.Fatal(err)
			}
			if fi.Mode().Perm() != 0o600 {
				t.Errorf("%s mode %v, want 0600", p, fi.Mode().Perm())
			}
			b, _ := os.ReadFile(p)
			if !strings.Contains(string(b), want) {
				t.Errorf("%s does not carry %q:\n%s", p, want, b)
			}
		}
		cleanup()
		if left := leftovers(t, tmp); len(left) != 0 {
			t.Errorf("cleanup left %v", left)
		}
	})
	t.Run("a failed second write removes the first and the directory", func(t *testing.T) {
		tmp := t.TempDir()
		t.Setenv("TMPDIR", tmp)
		old := writeStagedFile
		t.Cleanup(func() { writeStagedFile = old })
		n := 0
		writeStagedFile = func(path string, data []byte, perm os.FileMode) error {
			n++
			if n == 2 {
				return errors.New("disk full")
			}
			return os.WriteFile(path, data, perm)
		}
		_, _, _, err := stageCall(inv)
		var se *StagingError
		if !errors.As(err, &se) || se.Ring != 2 {
			t.Fatalf("err %v, want a StagingError for ring 2", err)
		}
		if left := leftovers(t, tmp); len(left) != 0 {
			t.Errorf("a failed second write left %v", left)
		}
	})
	t.Run("success and failure both clean up", func(t *testing.T) {
		needAstrolog(t)
		tmp := t.TempDir()
		t.Setenv("TMPDIR", tmp)
		if _, _, err := ComputeRawReport(context.Background(), spec, recipe.AspectsRawReport()); err != nil {
			t.Fatal(err)
		}
		if left := leftovers(t, tmp); len(left) != 0 {
			t.Errorf("a finished report left %v", left)
		}
		t.Setenv("PATH", t.TempDir()) // astrolog cannot start
		_, _, _ = ComputeRawReport(context.Background(), spec, recipe.AspectsRawReport())
		if left := leftovers(t, tmp); len(left) != 0 {
			t.Errorf("a failed report left %v", left)
		}
	})
	t.Run("canceled call", func(t *testing.T) {
		needAstrolog(t)
		tmp := t.TempDir()
		t.Setenv("TMPDIR", tmp)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, _, _ = ComputeRawReport(ctx, spec, recipe.AspectsRawReport())
		_, _ = ComputeCrossAspects(ctx, spec)
		if left := leftovers(t, tmp); len(left) != 0 {
			t.Errorf("a canceled call left %v", left)
		}
		// Canceled while Astrolog is running.
		ctx2, cancel2 := context.WithTimeout(context.Background(), time.Millisecond)
		defer cancel2()
		for i := 0; i < 20; i++ {
			_, _, _ = ComputeRawReport(ctx2, spec, recipe.AspectsRawReport())
		}
		if left := leftovers(t, tmp); len(left) != 0 {
			t.Errorf("a call canceled mid-run left %v", left)
		}
	})
}

// ---- N8: library headers read unchanged ---------------------------------

func TestLibraryChartHeadersReadUnchanged(t *testing.T) {
	needAstrolog(t)
	dir := t.TempDir()
	write := func(name, zi string) string {
		p := filepath.Join(dir, name)
		body := "@AI780  ; Astrolog chart info.\n-qb Jun 15 1990 4:30pm ST 0W 4:53E 52:22N\n" + zi + "-c 0\n"
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	for _, tc := range []struct {
		name, zi, wantName, wantPlace string
	}{
		{"titled.as", "-zi \"Synthetic Aster\" \"Amsterdam, NL\"\n", "Synthetic Aster", "Amsterdam, NL"},
		{"empty.as", "-zi \"\" \"\"\n", "", ""},
		{"none.as", "", "", ""},
		{"unicode.as", "-zi \"Zoë 日本\" \"Zürich, CH\"\n", "Zoë 日本", "Zürich, CH"},
		{"semicolon.as", "-zi \"a;b\" \"c;d\"\n", "a;b", "c;d"},
	} {
		p := write(tc.name, tc.zi)
		h, err := ReadChartFileHeader(p)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if h.Name != tc.wantName || h.PlaceLabel != tc.wantPlace {
			t.Errorf("%s: header read %q/%q, want %q/%q", tc.name, h.Name, h.PlaceLabel, tc.wantName, tc.wantPlace)
		}
		cf, err := ResolveChartFile(context.Background(), p)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if cf.Name != tc.wantName || cf.PlaceLabel != tc.wantPlace {
			t.Errorf("%s: resolve read %q/%q, want %q/%q", tc.name, cf.Name, cf.PlaceLabel, tc.wantName, tc.wantPlace)
		}
	}
	t.Run("a staged named file reads back as written", func(t *testing.T) {
		content, err := FormatChartFile(namedMomentAt(nameJD, 4.8833, 52.3667, "Synthetic Aster", "Amsterdam, NL"), recipe.Placidus, recipe.Tropical)
		if err != nil {
			t.Fatal(err)
		}
		// A staged file names its moment as `-qj <jd>` and no longer as `-qb`
		// calendar fields (the specification C1), so the library header reader,
		// which wants the -qb coordinates, is not the reader of a staged file:
		// its labels are read the way that reader reads them, by lastZI.
		name, place := lastZI(content)
		if name != "Synthetic Aster" || place != "Amsterdam, NL" {
			t.Errorf("staged file read back %q/%q", name, place)
		}
		if strings.Contains(content, "-qb") {
			t.Errorf("a staged file names its moment with -qb calendar fields:\n%s", content)
		}
	})
}

// ---- N17 / N22: a label cannot spoof the house line ----------------------

func TestNamedHeaderCannotSpoofHousesUsed(t *testing.T) {
	needAstrolog(t)
	plain, err := ComputeChart(context.Background(), castSpec(t, namedMoment("", "")))
	if err != nil {
		t.Fatal(err)
	}
	if plain.HousesUsed != "Placidus" {
		t.Fatalf("control: HousesUsed %q", plain.HousesUsed)
	}
	for _, label := range []string{"Veloc. Campanus Houses", "Veloc.  Koch Houses", "x Veloc. Whole Houses"} {
		for _, src := range []recipe.MomentAtPlace{namedMoment(label, "p"), namedMoment("n", label)} {
			out, err := ComputeChart(context.Background(), castSpec(t, src))
			if err != nil {
				t.Fatal(err)
			}
			if out.HousesUsed != "Placidus" {
				t.Errorf("label %q: HousesUsed = %q, want Placidus", label, out.HousesUsed)
			}
		}
	}
}

func TestNamesCannotSpoofAnyHousesUsed(t *testing.T) {
	needAstrolog(t)
	for n := 0; n < recipe.HouseSystemCount; n++ {
		houses := recipe.HouseSystem(n)
		other := recipe.HouseSystem((n + 7) % recipe.HouseSystemCount)
		spec := fixtureSpec(t, namedMoment("Veloc. "+other.StoredName()+" Houses", "Veloc. "+other.StoredName()+" Houses"), houses, recipe.Tropical)
		out, err := ComputeChart(context.Background(), spec)
		if err != nil {
			t.Fatalf("%s: %v", houses.StoredName(), err)
		}
		if out.HousesUsed != houses.StoredName() {
			t.Errorf("%s with a name spoofing %s: HousesUsed = %q", houses.StoredName(), other.StoredName(), out.HousesUsed)
		}
	}
}

// ---- N26: the last -zi line is Astrolog's ---------------------------------

func TestDuplicateZiUsesAstrologsLabel(t *testing.T) {
	needAstrolog(t)
	p := filepath.Join(t.TempDir(), "dup.as")
	body := "@AI780  ; Astrolog chart info.\n-qb Jun 15 1990 4:30pm ST 0W 4:53E 52:22N\n-zi \"First\" \"Place 1\"\n-zi \"Second\" \"Place 2\"\n-c 0\n"
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	out, _, err := spawn(context.Background(), newExempt("test: print a chart record's header", "-i", p, flagMainShow, "-w"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Second") || strings.Contains(out, "First") {
		t.Fatalf("Astrolog's own effective label is not the last -zi:\n%.300s", out)
	}
	h, err := ReadChartFileHeader(p)
	if err != nil {
		t.Fatal(err)
	}
	if h.Name != "Second" || h.PlaceLabel != "Place 2" {
		t.Errorf("ReadChartFileHeader read %q/%q, Astrolog prints Second/Place 2", h.Name, h.PlaceLabel)
	}
	cf, err := ResolveChartFile(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	if cf.Name != "Second" || cf.PlaceLabel != "Place 2" {
		t.Errorf("ResolveChartFile read %q/%q, Astrolog prints Second/Place 2", cf.Name, cf.PlaceLabel)
	}
}

// ---- N27: an invalid UTF-8 label is sent empty ---------------------------

func TestInvalidUTF8LabelIsSentEmpty(t *testing.T) {
	bad := "Jos\xf8 Bad"
	t.Run("argv", func(t *testing.T) {
		for _, tc := range []struct{ name, place, wantName, wantPlace string }{
			{bad, "Amsterdam", "", "Amsterdam"},
			{"Aster", bad, "Aster", ""},
			{bad, bad, "", ""},
		} {
			inv := compileOne(t, castSpec(t, namedMoment(tc.name, tc.place)), recipe.Positions{})
			got := ziIn(inv.Argv)
			if len(got) != 1 || got[0] != [2]string{tc.wantName, tc.wantPlace} {
				t.Errorf("name %q place %q: -zi %q, want [%q %q]", tc.name, tc.place, got, tc.wantName, tc.wantPlace)
			}
		}
	})
	t.Run("staged", func(t *testing.T) {
		spec := castSpec(t, recipe.Comparison{Primary: namedMoment(bad, bad), Secondary: namedMomentAt(nameJD, 1, 2, "ok", bad)})
		files := stagedContents(compileOne(t, spec, recipe.Aspects{}))
		if got := ziLinesOf(files[0]); !slices.Equal(got, []string{`-zi "" ""`}) {
			t.Errorf("chart 1: %q", got)
		}
		if got := ziLinesOf(files[1]); !slices.Equal(got, []string{`-zi "ok" ""`}) {
			t.Errorf("chart 2: %q", got)
		}
	})
	t.Run("live: no 0xF8 byte becomes a degree sign", func(t *testing.T) {
		needAstrolog(t)
		report, _, err := ComputeRawReport(context.Background(), castSpec(t, namedLocal(nameJD, bad, "x")), recipe.AspectsRawReport())
		if err != nil {
			t.Fatal(err)
		}
		first, _ := reportHeader(t, report)
		if strings.Contains(first, "Jos") || strings.Contains(first, "°") {
			t.Errorf("an invalid label reached the header: %q", first)
		}
	})
}

// ---- N28: one consistent read of a chart file -----------------------------

func TestResolveReplacementDuringAstrologRead(t *testing.T) {
	needAstrolog(t)
	dir := t.TempDir()
	p := filepath.Join(dir, "client.as")
	original := "@AI780  ; Astrolog chart info.\n-qb Jun 15 1990 4:30pm ST 0W 4:53E 52:22N\n-zi \"Original Client\" \"Amsterdam\"\n-c 0\n"
	replacement := "@AI780  ; Astrolog chart info.\n-qb Jan 1 2001 9:00am ST 0W 18:57E 69:39N\n-zi \"Other Client\" \"Tromso\"\n-c 14\n"
	if err := os.WriteFile(p, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	want, err := ResolveChartFile(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	old := afterChartFileRead
	t.Cleanup(func() { afterChartFileRead = old })
	afterChartFileRead = func() {
		afterChartFileRead = old
		if err := os.WriteFile(p, []byte(replacement), 0o600); err != nil {
			t.Error(err)
		}
	}
	got, err := ResolveChartFile(context.Background(), p)
	if err != nil {
		// A refused resolve is the other consistent answer.
		return
	}
	if got.Name != want.Name || got.PlaceLabel != want.PlaceLabel || got.Lon != want.Lon || got.Lat != want.Lat {
		t.Fatalf("labels and coordinates came from one read (%q %v,%v) but the moment from another", got.Name, got.Lon, got.Lat)
	}
	if got.JD != want.JD || got.Houses != want.Houses {
		t.Errorf("the replacement's moment or houses leaked in: JD %v (want %v), houses %v (want %v)", got.JD, want.JD, got.Houses, want.Houses)
	}
}

// ---- C9: save and retitle send titles exactly as before -------------------

func TestSaveAndRetitleLabelsUnchanged(t *testing.T) {
	long := strings.Repeat("T", 100) + `"q"`
	place := strings.Repeat("P", 90)
	save := saveArgv(ChartToSave{Name: long, Place: place, When: time.Date(1990, 6, 15, 14, 30, 0, 0, time.UTC), Lon: 4.9, Lat: 52.4}, "/dest")
	if got := ziIn(save); len(got) != 1 || got[0] != [2]string{long, place} {
		t.Errorf("save sends %q, want the title and place whole", got)
	}
	re := retitleArgv("/src", long, place, "/dest")
	if got := ziIn(re); len(got) != 1 || got[0] != [2]string{long, place} {
		t.Errorf("retitle sends %q, want the title and place whole", got)
	}
}
