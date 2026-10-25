package astrolog

// the specification C2, C3 and its build notes: the ayanamsa table is a
// copy of an Astrolog fact, so a live test keeps it Astrolog's, and every call
// and staged file carries the chosen offset numerically.

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/cruiserxyz/sextile/internal/recipe"
)

// wantOffsets are the ten ayanamsas' offsets from Fagan-Bradley, in degrees, as
// they must be spelled on an argv or in a staged file. They were measured from
// Astrolog 8.00 on 2026-10-08 (Sun, forty dates between 1850 and 2150, the
// differences constant to the millisecond of arc) and are written here
// independently of the production table, so a drift in either is caught by
// the live test or by this pin.
var wantOffsets = map[recipe.Zodiac]string{
	recipe.FaganBradley:   "0",
	recipe.Lahiri:         "0.883208",
	recipe.Deluce:         "-3.075453",
	recipe.Raman:          "2.329509",
	recipe.UshaShasi:      "4.682759",
	recipe.Krishnamurti:   "0.98006",
	recipe.DjwhalKhul:     "-3.619379",
	recipe.Yukteshwar:     "2.261497",
	recipe.JNBhasin:       "1.978163",
	recipe.GalacticCenter: "-2.105736",
}

// ayanamsas is the ten, in C2's order.
func ayanamsas() []recipe.Zodiac {
	var out []recipe.Zodiac
	for z := recipe.FaganBradley; int(z) < recipe.ZodiacCount; z++ {
		out = append(out, z)
	}
	return out
}

// bandFor is the frame band C3 pins for a zodiac, written from wantOffsets.
func bandFor(code string, z recipe.Zodiac) []string {
	if z == recipe.Tropical {
		return []string{"-c", code, "_s", "_Ys", "_sr0", ":sz"}
	}
	return []string{"-c", code, "=s", wantOffsets[z], "_Ys", "_sr0", ":sz"}
}

// arcsecondField is a position printed to the second of arc (`-b0`). The casts
// once added `-b1` for the millisecond, but Astrolog's SzLocation formats into a
// static char szLoc[25] that the two switches together overflow (measured
// 2026-10-24: "*** buffer overflow detected ***" under _FORTIFY_SOURCE, on any
// argv or chart file carrying both), so a second of arc is the finest a cast
// here can print. The ayanamsa table is still derived to the millisecond
// (ayanamsa.go); these casts check it at the second.
var arcsecondField = regexp.MustCompile(`\d+[A-Z][a-z]{2}\d{2}'\d{2}"`)

// zodiacFixture stages one fixed chart (a made-up moment and place) and casts it
// with the given zodiac switch, printing to the second of arc (`-b0`).
func zodiacCast(t *testing.T, zodiacArgs ...string) string {
	t.Helper()
	const chart = "@AI780  ; Astrolog chart info.\n-qb Mar 20 1995 11:00:00am ST 0W 5:00E 52:00N\n-zi \"\" \"\"\n-c 14\n"
	path := filepath.Join(t.TempDir(), "fixed.as")
	if err := os.WriteFile(path, []byte(chart), 0o600); err != nil {
		t.Fatal(err)
	}
	argv := append([]string{"-i", path}, zodiacArgs...)
	argv = append(argv, "_Ys", "_sr0", ":sz", "-b0", "-v0", "-k")
	out, _, err := spawn(context.Background(), testCall(argv))
	if err != nil {
		t.Fatalf("spawn %q: %v (%.300s)", argv, err, out)
	}
	var rows []string
	for _, l := range strings.Split(stripSGR(out), "\n") {
		if arcsecondField.MatchString(l) {
			rows = append(rows, l)
		}
	}
	return strings.Join(rows, "\n")
}

var sgrPattern = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func stripSGR(s string) string { return sgrPattern.ReplaceAllString(s, "") }

// C2: the table cannot drift from the installed Astrolog.
func TestAyanamsaTableMatchesAstrolog(t *testing.T) {
	needAstrolog(t)
	if _, ok := AyanamsaOffset(recipe.Tropical); ok {
		t.Error("Tropical has an ayanamsa offset: the table is the ten only")
	}
	tropical := zodiacCast(t, "_s")
	if !arcsecondField.MatchString(tropical) {
		t.Fatalf("the cast carries no arcsecond fields, so it cannot prove an offset:\n%s", tropical)
	}
	fagan := zodiacCast(t, "=s", "Fagan-Bradley")
	nonzeroDiffers := false
	for _, z := range ayanamsas() {
		t.Run(z.Name(), func(t *testing.T) {
			off, ok := AyanamsaOffset(z)
			if !ok {
				t.Fatalf("no offset for %s", z.Name())
			}
			byName := zodiacCast(t, "=s", z.Name())
			if !arcsecondField.MatchString(byName) {
				t.Fatalf("no arcsecond fields in the by-name cast:\n%s", byName)
			}
			byOffset := zodiacCast(t, "=s", OffsetArg(off))
			if byName != byOffset {
				t.Errorf("=s %s and =s %s disagree\nby name:\n%s\nby offset:\n%s", z.Name(), OffsetArg(off), byName, byOffset)
			}
			if off != 0 {
				if byOffset == tropical {
					t.Errorf("a nonzero offset cast equals the tropical cast: the switch is inert")
				}
				if byOffset == fagan {
					t.Errorf("a nonzero offset cast equals Fagan-Bradley's: the offset is inert")
				}
				nonzeroDiffers = true
			}
		})
	}
	if !nonzeroDiffers {
		t.Error("no nonzero offset was checked against an _s cast")
	}
}

// C3: the frame band's zodiac part is `_s _Ys` or `=s <offset> _Ys`, on every
// call and in every staged file; never a bare -s, never a name.
func TestSiderealBandIsNumericAndPinned(t *testing.T) {
	sources := map[string]recipe.Source{
		"single":     fixLondon,
		"local":      recipe.LocalMomentAtPlace{JD: 2450000.5, Lon: 0, Lat: 51.5, TZ: "Europe/London"},
		"comparison": recipe.Comparison{Primary: fixLondon, Secondary: fixTromso},
		"transit":    recipe.Comparison{Primary: fixLondon, Secondary: fixTromso, Relation: recipe.TransitToNatal},
		"multiwheel": recipe.MultiWheel{Rings: []recipe.Source{fixLondon, fixTromso, fixOslo}},
	}
	projections := func(name string) []recipe.Projection {
		switch name {
		case "single":
			return []recipe.Projection{recipe.Positions{}, recipe.Aspects{}, recipe.ChartRawReport(), recipe.WheelImage{Render: recipe.DefaultWheelRender()}}
		case "local":
			return []recipe.Projection{recipe.ChartRawReport()}
		case "multiwheel":
			return []recipe.Projection{recipe.WheelImage{Render: recipe.DefaultWheelRender()}}
		}
		return []recipe.Projection{recipe.Aspects{}, recipe.AspectsRawReport(), recipe.WheelImage{Render: recipe.DefaultWheelRender()}}
	}
	for _, z := range append([]recipe.Zodiac{recipe.Tropical}, ayanamsas()...) {
		t.Run(z.Name(), func(t *testing.T) {
			for name, src := range sources {
				spec := fixtureSpec(t, src, recipe.Placidus, z)
				for _, p := range projections(name) {
					plan, err := Compile(spec, p)
					if err != nil {
						t.Fatalf("%s/%s: %v", name, p.Label(), err)
					}
					for _, inv := range plan.Invocations {
						if want := bandFor("0", z); !containsRun(inv.Argv, want) {
							t.Errorf("%s/%s: argv has no contiguous %q: %q", name, p.Label(), want, inv.Argv)
						}
						for _, a := range inv.Argv {
							if a == "-s" || (z != recipe.Tropical && a == z.Name()) {
								t.Errorf("%s/%s: bare -s or a name in argv: %q", name, p.Label(), inv.Argv)
							}
						}
						for i, ti := range inv.TempInputs {
							want := "-c 0\n_s\n_Ys\n"
							if z != recipe.Tropical {
								want = "-c 0\n=s " + wantOffsets[z] + "\n_Ys\n"
							}
							if !strings.Contains(ti.Content, want) {
								t.Errorf("%s/%s: staged chart %d lacks %q:\n%s", name, p.Label(), i, want, ti.Content)
							}
							if strings.Contains(ti.Content, "\n-s") || (z != recipe.Tropical && strings.Contains(ti.Content, z.Name())) {
								t.Errorf("%s/%s: staged chart %d carries a bare -s or a name:\n%s", name, p.Label(), i, ti.Content)
							}
						}
					}
				}
			}
			// A timeline search carries the very same band (C10).
			call, err := TimelineCall(TimelineSky, TimelineSegment{}, TimelineRequest{Year: 2026, Month: 10, Houses: recipe.Placidus, Zodiac: z})
			if err != nil {
				t.Fatalf("TimelineCall: %v", err)
			}
			if want := bandFor("0", z); !containsRun(call.Argv(), want) {
				t.Errorf("timeline argv has no contiguous %q: %q", want, call.Argv())
			}
		})
	}
}

// C3, live: the host's astrolog.as cannot choose the ayanamsa either. A host
// `:s Lahiri` would otherwise pick the offset under a bare `=s`.
func TestChosenAyanamsaSurvivesAHostileHost(t *testing.T) {
	needAstrolog(t)
	spec := fixtureSpec(t, fixLondon, recipe.Placidus, recipe.Deluce)
	clean, hostile := underBoth(t, func() castResult { return castChart(t, spec) })
	clean.valid(t, "clean")
	tropical := castChart(t, fixtureSpec(t, fixLondon, recipe.Placidus, recipe.Tropical))
	if sameCast(clean, tropical) {
		t.Fatal("a Deluce cast equals the tropical one: the offset never reached Astrolog")
	}
	if !sameCast(clean, hostile) {
		t.Errorf("a hostile astrolog.as moved a Deluce cast\nclean   %+v\nhostile %+v", clean, hostile)
	}
}

func sameCast(a, b castResult) bool {
	if len(a.Positions) != len(b.Positions) || a.Cusps != b.Cusps {
		return false
	}
	for i := range a.Positions {
		if a.Positions[i] != b.Positions[i] {
			return false
		}
	}
	return true
}

// Pass-7 build note: the preliminary probe and the relationship both carry the
// chosen offset, and the relationship equals a direct Astrolog run.
func TestChosenAyanamsaReachesComparisonProbe(t *testing.T) {
	needAstrolog(t)
	spec := fixtureSpec(t, recipe.Comparison{Primary: fixLondon, Secondary: fixTromso}, recipe.WholeSign, recipe.Lahiri)
	out, err := ComputeCrossAspects(context.Background(), spec)
	if err != nil {
		t.Fatalf("ComputeCrossAspects: %v", err)
	}
	if len(out.Commands) != 2 {
		t.Fatalf("want probe + relationship (2 commands), got %q", out.Commands)
	}
	want := formatCommandTokens(bandFor("14", recipe.Lahiri))
	for i, cmd := range out.Commands {
		if !strings.Contains(cmd, want) {
			t.Errorf("command %d lacks the Lahiri band %q:\n%s", i, want, cmd)
		}
	}
}

// directByName runs the compiled invocation again the way a person would run
// Astrolog on the same two charts: the staged files are plain chart files
// (Astrolog's own writer never puts a zodiac switch in one), and the zodiac is
// Astrolog's own name for the ayanamsa on the argv alone.
func directByName(t *testing.T, inv Invocation, z recipe.Zodiac) string {
	t.Helper()
	off := wantOffsets[z]
	argv := append([]string(nil), inv.Argv...)
	swapped := 0
	for i := 0; i+1 < len(argv); i++ {
		if argv[i] == "=s" && argv[i+1] == off {
			argv[i+1] = z.Name()
			swapped++
		}
	}
	if swapped != 1 {
		t.Fatalf("argv carries =s %s %d times, want once: %q", off, swapped, inv.Argv)
	}
	dir := t.TempDir()
	for i, ti := range inv.TempInputs {
		pin := "=s " + off + "\n_Ys\n"
		content := strings.Replace(ti.Content, pin, "", 1)
		if content == ti.Content {
			t.Fatalf("staged chart %d carries no %q:\n%s", i, pin, ti.Content)
		}
		p := filepath.Join(dir, "direct"+string(rune('0'+i))+".as")
		if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		argv[ti.ArgIndex] = p
	}
	out, _, err := spawn(context.Background(), testCall(argv))
	if err != nil {
		t.Fatalf("direct spawn: %v (%.300s)", err, out)
	}
	return out
}

// twoDates is a pair of charts a precession apart: the sidereal pair differs
// from the tropical pair by it (C9), so equality with a direct run is not
// the tropical answer in disguise.
var (
	twoDateFirst  = recipe.MomentAtPlace{JD: 2433000.5, Lon: 10.75, Lat: 59.91} // 1949
	twoDateSecond = recipe.MomentAtPlace{JD: 2461000.25, Lon: 4.9, Lat: 52.37}  // 2025
)

func twoDateMatchesDirect(t *testing.T, rel recipe.Relation) {
	t.Helper()
	needAstrolog(t)
	tropical := ""
	for _, z := range append([]recipe.Zodiac{recipe.Tropical}, ayanamsas()...) {
		spec := fixtureSpec(t, recipe.Comparison{Primary: twoDateFirst, Secondary: twoDateSecond, Relation: rel}, recipe.WholeSign, z)
		inv := compileOne(t, spec, recipe.AspectsRawReport())
		got := runInvocation(t, inv)
		if z == recipe.Tropical {
			tropical = got
			continue
		}
		if want := directByName(t, inv, z); got != want {
			t.Errorf("%s: Sextile's relationship differs from the direct Astrolog run, first at %s", z.Name(), firstDifference(got, want))
		}
		if got == tropical {
			t.Errorf("%s: the relationship equals the tropical one: the offset never reached it", z.Name())
		}
		stagedFilesHonourTheirPins(t, inv, z)
	}
}

// castContent casts one chart file's content to the second of arc and
// returns its position rows. The given switches go BEFORE `-i`, so a zodiac
// line inside the file overrides them, and a file without one is cast in them.
func castContent(t *testing.T, content string, argvZodiac ...string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "staged.as")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	argv := append(append([]string{}, argvZodiac...), "-i", path)
	argv = append(argv, "-b0", "-v0", "-k")
	out, _, err := spawn(context.Background(), testCall(argv))
	if err != nil {
		t.Fatalf("spawn %q: %v (%.300s)", argv, err, out)
	}
	var rows []string
	for _, l := range strings.Split(stripSGR(out), "\n") {
		if arcsecondField.MatchString(l) {
			rows = append(rows, l)
		}
	}
	return strings.Join(rows, "\n")
}

// stagedFilesHonourTheirPins casts each staged chart exactly as Sextile wrote
// it, after an explicit tropical start (`_s _Ys` before `-i`, so the host's
// astrolog.as decides nothing), so the file's own `=s <offset>` and `_Ys`
// lines must do the work. Its positions must equal a direct cast of the
// same moment with `=s <name>` on the argv, and differ from the same file with
// its zodiac lines removed (the control that keeps the equality from being
// vacuous).
func stagedFilesHonourTheirPins(t *testing.T, inv Invocation, z recipe.Zodiac) {
	t.Helper()
	pin := "=s " + wantOffsets[z] + "\n_Ys\n"
	for i, ti := range inv.TempInputs {
		if !strings.Contains(ti.Content, pin) {
			t.Fatalf("%s: staged chart %d carries no %q:\n%s", z.Name(), i, pin, ti.Content)
		}
		own := castContent(t, ti.Content, "_s", "_Ys")
		if !arcsecondField.MatchString(own) {
			t.Fatalf("%s: staged chart %d printed no arcsecond rows", z.Name(), i)
		}
		bare := strings.Replace(ti.Content, pin, "", 1)
		if direct := castContent(t, bare, "=s", z.Name(), "_Ys"); own != direct {
			t.Errorf("%s: staged chart %d cast as written differs from a direct =s %s cast, first at %s", z.Name(), i, z.Name(), firstDifference(own, direct))
		}
		if castContent(t, bare, "_s", "_Ys") == own {
			t.Errorf("%s: staged chart %d without its zodiac lines casts the same: the pins did no work", z.Name(), i)
		}
	}
}

// Pass-4 build note: both staged charts and the live relationship equal a
// direct Astrolog run, two dates apart.
func TestTwoDateSiderealComparisonMatchesDirectAstrolog(t *testing.T) {
	twoDateMatchesDirect(t, recipe.Synastry)
}

// Pass-5 build note: the `-rt` transit path, as `-r0` is covered above.
func TestTwoDateSiderealTransitMatchesDirectAstrolog(t *testing.T) {
	twoDateMatchesDirect(t, recipe.TransitToNatal)
}

// firstDifference names the first line two outputs disagree on.
func firstDifference(a, b string) string {
	la, lb := strings.Split(a, "\n"), strings.Split(b, "\n")
	for i := 0; i < len(la) || i < len(lb); i++ {
		var x, y string
		if i < len(la) {
			x = la[i]
		}
		if i < len(lb) {
			y = lb[i]
		}
		if x != y {
			return "line " + strconv.Itoa(i+1) + ":\n  sextile: " + x + "\n  direct:  " + y
		}
	}
	return "nowhere (equal)"
}

// Pass-4 build note: a zodiac switch in a chart file never applies. Otherwise
// identical files, with and without `_s`, `=s` or `=s Lahiri` (Equinox-2026-03.as
// carries `_s` today), read to the same moment and place, and the charts cast
// from that moment and place are equal in every zodiac Sextile offers.
func TestChartFileZodiacSwitchIsIgnored(t *testing.T) {
	needAstrolog(t)
	const body = "-qb Sep 23 1995 6:00:00pm DT 1E 11:34E 48:08N\n-zi \"\" \"\"\n-c 14\n"
	variants := map[string]string{
		"none":       "",
		"_s":         "_s\n",
		"=s":         "=s\n",
		"=s Lahiri":  "=s Lahiri\n",
		"=s 0.88":    "=s 0.883208\n",
		"=s Deluce ": "=s -3.075453\n",
	}
	dir := t.TempDir()
	paths := map[string]string{}
	for name, extra := range variants {
		p := filepath.Join(dir, strings.NewReplacer(" ", "_", "=", "eq", ".", "p").Replace(name)+".as")
		if err := os.WriteFile(p, []byte("@AI780  ; Astrolog chart info.\n"+body+extra), 0o600); err != nil {
			t.Fatal(err)
		}
		paths[name] = p
	}
	// The control: with the file alone deciding, the switch DOES move a cast, so
	// the equalities below are not equalities of an inert fixture.
	rawCast := func(name string) string {
		out, _, err := spawn(context.Background(), testCall([]string{"-i", paths[name], "-v0", "-k", "-b0"}))
		if err != nil {
			t.Fatalf("raw cast of %s: %v", name, err)
		}
		return stripSGR(out)
	}
	if rawCast("none") == rawCast("=s Lahiri") {
		t.Fatal("control: a file's own =s Lahiri moves no raw cast, so this test could not see the switch apply")
	}

	var baseline ChartFile
	var baseCasts map[recipe.Zodiac]castResult
	for _, name := range []string{"none", "_s", "=s", "=s Lahiri", "=s 0.88", "=s Deluce "} {
		cf, err := ResolveChartFile(context.Background(), paths[name])
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		moment := recipe.MomentAtPlace{JD: cf.JD, Lon: cf.Lon, Lat: cf.Lat}
		casts := map[recipe.Zodiac]castResult{}
		for _, z := range []recipe.Zodiac{recipe.Tropical, recipe.Lahiri, recipe.Deluce} {
			casts[z] = castChart(t, fixtureSpec(t, moment, recipe.Placidus, z))
		}
		if name == "none" {
			baseline, baseCasts = cf, casts
			continue
		}
		if cf != baseline {
			t.Errorf("%s: the file reads as %+v, want the plain file's %+v", name, cf, baseline)
		}
		for z, c := range casts {
			if !sameCast(c, baseCasts[z]) {
				t.Errorf("%s: the %s cast differs from the plain file's", name, z.Name())
			}
		}
	}
}
