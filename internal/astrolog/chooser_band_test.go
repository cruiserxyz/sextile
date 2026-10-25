package astrolog

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/cruiserxyz/sextile/internal/recipe"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// The sets astrolog 8.00 listed on 2026-10-03 (specs/astrolog-calls-honour-
// choosers.md § Measured facts), used here only as INPUT to the band: the code
// under test reads them from the binary (TestDefaultSetsReadFromAstrolog).
var (
	measuredMain    = []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 16, 22, 31}
	measuredTransit = []int{1, 3, 4, 5, 6, 7, 8, 9, 10, 16}
	measuredSets    = DefaultSets{Main: measuredMain, Transit: measuredTransit}
)

func bandReq(t *testing.T, o recipe.ObjectPolicy, a recipe.AspectPolicy) TimelineRequest {
	t.Helper()
	return TimelineRequest{
		Year: 2026, Month: time.October, Zone: tlAmsterdam(t),
		Natal:   &TimelineNatal{When: time.Date(1980, 4, 15, 12, 0, 0, 0, time.UTC), Lon: 4.9, Lat: 52.3667},
		Objects: o, Aspects: a, Defaults: measuredSets,
	}
}

func hellenistic() recipe.ObjectPolicy {
	return recipe.ObjectPolicy{
		Baseline: recipe.AstrologDefault,
		Add:      recipe.NewBodySet(aspect.Chiron, aspect.SouthNode, aspect.Lilith, aspect.ImumCoeli, aspect.Descendant),

		TrueNodeMode: true,
	}
}

func without(o recipe.ObjectPolicy, bodies ...aspect.Body) recipe.ObjectPolicy {
	o.Remove = recipe.NewBodySet(bodies...)
	return o
}

func callTokens(t *testing.T, src TimelineSource, req TimelineRequest) (argv, tail []string) {
	t.Helper()
	segs := TimelineSegments(req.Year, req.Month, req.Zone)
	call, err := TimelineCall(src, segs[0], req)
	if err != nil {
		t.Fatal(err)
	}
	if err := checkBand(call); err != nil {
		t.Fatalf("the call's own argv does not carry its band: %v", err)
	}
	// The bands follow the mode switch's month and year: `-dm` ends the sky
	// search; `-t|-tr <month> <year>` ends the natal ones.
	i := slices.Index(call.argv, "-dm")
	switch src {
	case TimelineTransit:
		i = slices.Index(call.argv, "-t") + 2
	case TimelineReturn:
		i = slices.Index(call.argv, "-tr") + 2
	}
	// The frame band (the specification C5) sits between the mode
	// switch's month and year and the chooser band; the tail is what follows it.
	frame, err := frameBand(req.Houses, req.Zodiac)
	if err != nil {
		t.Fatal(err)
	}
	if end := i + 1 + len(frame); end > len(call.argv) || !slices.Equal(call.argv[i+1:end], frame) {
		t.Fatalf("the frame band %q does not follow the mode switch in %v", frame, call.argv)
	}
	// The reform-date pin follows the frame band (the specification C6), then
	// the clock pins (the specification C1); the chooser band is what follows
	// them.
	at := i + 1 + len(frame)
	if end := at + len(reformPin); end > len(call.argv) || !slices.Equal(call.argv[at:end], reformPin) {
		t.Fatalf("the reform pin %q does not follow the frame band in %v", reformPin, call.argv)
	}
	at += len(reformPin)
	pins := CurrentClock().Pins()
	if end := at + len(pins); end > len(call.argv) || !slices.Equal(call.argv[at:end], pins) {
		t.Fatalf("the clock pins %q do not follow the frame band in %v", pins, call.argv)
	}
	return call.argv, call.argv[at+len(pins):]
}

// C1-C4, C7, C8, C10: the band is carried, only where it differs, and after
// the month and year with -z still first.
func TestTimelineArgvCarriesChooserBand(t *testing.T) {
	mainNoMoon := "-R0 1 3 4 5 6 7 8 9 10 16 22 31"
	cases := []struct {
		name string
		src  TimelineSource
		req  TimelineRequest
		tail string
	}{
		{"sky, Moon hidden (C2)", TimelineSky, bandReq(t, without(hellenistic(), aspect.Moon), recipe.AspectPolicy{}), "-Yn " + mainNoMoon},
		{"transit, Moon hidden: natal side banded, transit side not (C3)", TimelineTransit, bandReq(t, without(hellenistic(), aspect.Moon), recipe.AspectPolicy{}), "-Yn " + mainNoMoon},
		{"return, Moon hidden (C3)", TimelineReturn, bandReq(t, without(hellenistic(), aspect.Moon), recipe.AspectPolicy{}), "-Yn " + mainNoMoon},
		{"transit, Sun hidden: both sides banded (C3)", TimelineTransit, bandReq(t, without(hellenistic(), aspect.Sun), recipe.AspectPolicy{}),
			"-Yn -R0 2 3 4 5 6 7 8 9 10 16 22 31 -RT0 3 4 5 6 7 8 9 10 16"},
		{"sky, Sun hidden has no transit band", TimelineSky, bandReq(t, without(hellenistic(), aspect.Sun), recipe.AspectPolicy{}), "-Yn -R0 2 3 4 5 6 7 8 9 10 16 22 31"},
		{"unfiltered lens: only the node mode (C4, C8)", TimelineTransit, bandReq(t, hellenistic(), recipe.AspectPolicy{}), "-Yn"},
		{"unfiltered sky, mean nodes: no band at all (C4)", TimelineSky, bandReq(t, recipe.ObjectPolicy{Baseline: recipe.AstrologDefault}, recipe.AspectPolicy{}), ""},
		{"explicit selection Sun+Moon", TimelineTransit, bandReq(t, recipe.ObjectPolicy{Baseline: recipe.Explicit, Selection: []int{2, 1}}, recipe.AspectPolicy{}),
			"-R0 1 2 -RT0 1"},
		{"empty explicit selection is a bare band on each side", TimelineReturn, bandReq(t, recipe.ObjectPolicy{Baseline: recipe.Explicit}, recipe.AspectPolicy{}), "-R0 -RT0"},
		{"aspect band exactly as emitAspects (C7)", TimelineSky, bandReq(t, recipe.ObjectPolicy{Baseline: recipe.AstrologDefault}, recipe.AspectPolicy{Explicit: true, Indices: []int{6, 1}}), "-RA0 1 6"},
		{"empty aspect selection is a bare -RA0 (C7)", TimelineTransit, bandReq(t, recipe.ObjectPolicy{Baseline: recipe.AstrologDefault}, recipe.AspectPolicy{Explicit: true}), "-RA0"},
		{"object band then aspect band", TimelineSky, bandReq(t, without(hellenistic(), aspect.Moon), recipe.AspectPolicy{Explicit: true, Indices: []int{1}}), "-Yn " + mainNoMoon + " -RA0 1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			argv, tail := callTokens(t, tc.src, tc.req)
			if got := strings.Join(tail, " "); got != tc.tail {
				t.Errorf("band = %q, want %q (argv %v)", got, tc.tail, argv)
			}
			if argv[0] != "-z" {
				t.Errorf("-z is not first: %v (C10)", argv)
			}
		})
	}
}

// C5, C6: a body Astrolog does not list for a side is never added to it, and
// the visible set is the policy's own.
func TestTimelineArgvHideOnly(t *testing.T) {
	cases := []struct {
		name string
		src  TimelineSource
		o    recipe.ObjectPolicy
		tail string
	}{
		{"delta Add of Chiron adds nothing to the band (C5)", TimelineTransit, hellenistic(), "-Yn"},
		{"Sun hidden and Chiron added: band lists no 11 (C5, C6)", TimelineSky, without(hellenistic(), aspect.Sun), "-Yn -R0 2 3 4 5 6 7 8 9 10 16 22 31"},
		{"explicit selection of Chiron and the Moon: the sky band drops 11, the transit band drops the Moon (C5)", TimelineTransit,
			recipe.ObjectPolicy{Baseline: recipe.Explicit, Selection: []int{1, 2, 11}}, "-R0 1 2 -RT0 1"},
		{"explicit selection of every default plus Chiron: no band, Chiron is not added (C5)", TimelineSky,
			recipe.ObjectPolicy{Baseline: recipe.Explicit, Selection: append(slices.Clone(measuredMain), 11, 17)}, ""},
		{"Moon ticked on the transiting side: still no -RT0 (C5)", TimelineReturn,
			recipe.ObjectPolicy{Baseline: recipe.Explicit, Selection: append(slices.Clone(measuredMain), 11)}, ""},
		{"a hidden Descendant that Astrolog never listed changes nothing (C6)", TimelineSky,
			without(hellenistic(), aspect.Moon, aspect.NorthNode), "-Yn -R0 1 3 4 5 6 7 8 9 10 22 31"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, tail := callTokens(t, tc.src, bandReq(t, tc.o, recipe.AspectPolicy{}))
			if got := strings.Join(tail, " "); got != tc.tail {
				t.Errorf("band = %q, want %q", got, tc.tail)
			}
		})
	}
}

// C11: the sets come from the binary, through exempt calls, with exactly the
// argv the measurement used. The fake lists sets that are NOT astrolog 8.00's,
// so a literal list in Go cannot pass.
func TestDefaultSetsReadFromAstrolog(t *testing.T) {
	dir := t.TempDir()
	registry, err := filepath.Abs("testdata/ho-all.txt")
	if err != nil {
		t.Fatal(err)
	}
	header := "Astrolog planets and objects:\nNum. Name       Rule     Detriment Exalt Fall Esoteric Hierarchical Ray\n\n"
	main := writeTestFile(t, dir, "main.txt", header+
		"  1 Sun         Leo      Aqu       Ari   Lib  Leo      Leo          2\n"+
		"  4 Venus       Lib Tau  Ari Sco   Pis   Vir  Gem      Cap          5\n"+
		" 10 Pluto       Sco      Tau       Aqu   Leo  Pis      Pis          1\n")
	transit := writeTestFile(t, dir, "transit.txt", header+
		"  2 Moon        Can      Cap       Tau   Sco  Vir      Aqu          4\n"+
		"  4 Venus       Lib Tau  Ari Sco   Pis   Vir  Gem      Cap          5\n")
	_, argvLog := fakeBinary(t, dir, `case " $* " in
  *" -R1 "*) cat '`+registry+`' ;;
  *" -R2 "*) cat '`+transit+`' ;;
  *) cat '`+main+`' ;;
esac`)
	got, err := ReadDefaultSets(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got.Main, []int{1, 4, 10}) || !slices.Equal(got.Transit, []int{2, 4}) {
		t.Errorf("sets = %+v, want Main [1 4 10] Transit [2 4] — what the binary printed", got)
	}
	var spawned []string
	for _, a := range spawnArgvs(t, argvLog) {
		spawned = append(spawned, strings.Join(a, " "))
	}
	if want := []string{"-R1 -HO", "-HO", "-R2 -HO"}; !slices.Equal(spawned, want) {
		t.Errorf("spawns = %q, want %q", spawned, want)
	}
}

// C12 at its source: every way a default-set read can be unusable is an error,
// never an empty or partial set.
func TestDefaultSetsRefuseAnUnusableRead(t *testing.T) {
	registry, err := filepath.Abs("testdata/ho-all.txt")
	if err != nil {
		t.Fatal(err)
	}
	good := "  1 Sun         Leo      Aqu       Ari   Lib  Leo      Leo          2\n"
	header := "Astrolog planets and objects:\nNum. Name       Rule     Detriment Exalt Fall Esoteric Hierarchical Ray\n\n"
	cases := []struct {
		name, main, transit string
	}{
		{"main read exits nonzero", "exit 3", "cat"},
		{"main lists nothing", "echo nothing here", "cat"},
		{"transit lists an object the registry lacks", "cat", "printf '%s' '" + header + "  3 Mercury     Gem Vir  Sag Pis   Vir   Pis  Ari      Sco          4\n'"},
		{"main lists a number twice", "printf '%s' '" + header + good + good + "'", "cat"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mainOut := tc.main
			if mainOut == "cat" {
				mainOut = "printf '%s' '" + header + good + "'"
			}
			transitOut := tc.transit
			if transitOut == "cat" {
				transitOut = "printf '%s' '" + header + good + "'"
			}
			reg := registry
			if strings.HasPrefix(tc.name, "transit lists an object") {
				// A registry of Sun and Moon only: Mercury is then foreign to it.
				if reg, err = filepath.Abs("testdata/ho-restricted.txt"); err != nil {
					t.Fatal(err)
				}
			}
			fakeBinary(t, t.TempDir(), `case " $* " in
  *" -R1 "*) cat '`+reg+`' ;;
  *" -R2 "*) `+transitOut+` ;;
  *) `+mainOut+` ;;
esac`)
			if sets, err := ReadDefaultSets(context.Background()); err == nil {
				t.Errorf("ReadDefaultSets = %+v, nil; want an error", sets)
			}
		})
	}
}

// C17, C18: the seam refuses a body call without its band, and starts no
// process doing it. The valid call at the end proves the fake would have run.
func TestSeamRefusesBodyCallWithoutBand(t *testing.T) {
	argv := []string{"-z", "-1", "-qm", "10", "2026", "-dm", "-R0", "1", "3"}
	cases := []struct {
		name string
		call BodyCall
	}{
		{"zero band (never issued), even though the argv has band-looking tokens", BodyCall{argv: argv}},
		{"zero band with an empty argv", BodyCall{}},
		{"issued band whose tokens are absent from argv", BodyCall{argv: argv[:6], band: newBand([]string{"-R0", "1", "3"})}},
		{"issued band whose tokens are present but not contiguous", BodyCall{argv: []string{"-qm", "-R0", "-dm", "1", "3"}, band: newBand([]string{"-R0", "1", "3"})}},
		{"issued band whose tokens are in the wrong order", BodyCall{argv: argv, band: newBand([]string{"-R0", "3", "1"})}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, argvLog := fakeBinary(t, t.TempDir(), "echo ran")
			out, command, err := spawn(context.Background(), tc.call)
			if !errors.Is(err, ErrBandMissing) {
				t.Fatalf("err = %v, want ErrBandMissing", err)
			}
			if out != "" || command != "" {
				t.Errorf("out=%q command=%q, want neither: no process may start", out, command)
			}
			if _, statErr := os.Stat(argvLog); !errors.Is(statErr, os.ErrNotExist) {
				t.Errorf("a process was started for a refused body call (argv log exists: %v)", statErr)
			}
		})
	}
	t.Run("a tokenless issued band is accepted", func(t *testing.T) {
		_, argvLog := fakeBinary(t, t.TempDir(), "echo ran")
		if _, _, err := spawn(context.Background(), BodyCall{argv: argv, band: newBand()}); err != nil {
			t.Fatal(err)
		}
		if got := spawnArgvs(t, argvLog); len(got) != 1 {
			t.Errorf("spawns = %d, want 1: a valid call must run, or the refusals above prove nothing", len(got))
		}
	})
	t.Run("a contiguous issued band is accepted", func(t *testing.T) {
		_, argvLog := fakeBinary(t, t.TempDir(), "echo ran")
		if _, _, err := spawn(context.Background(), BodyCall{argv: argv, band: newBand([]string{"-R0", "1", "3"})}); err != nil {
			t.Fatal(err)
		}
		if got := spawnArgvs(t, argvLog); len(got) != 1 {
			t.Errorf("spawns = %d, want 1", len(got))
		}
	})
}

// An exempt call with no reason is refused too: the reason is what makes it a
// classification rather than a bypass.
func TestSeamRefusesExemptCallWithoutReason(t *testing.T) {
	_, argvLog := fakeBinary(t, t.TempDir(), "echo ran")
	if _, _, err := spawn(context.Background(), newExempt("  ", "-HO")); err == nil {
		t.Fatal("an exempt call with a blank reason was spawned")
	}
	if _, statErr := os.Stat(argvLog); !errors.Is(statErr, os.ErrNotExist) {
		t.Error("a process was started for an exempt call with no reason")
	}
}

// C16a: a body call cannot gain a restriction or node-mode token after its band
// was issued, by any route a caller has, and a refused call starts no process.
func TestBodyCallRefusesSmuggledRestriction(t *testing.T) {
	base := newBodyCall([]string{"-qj", "2451545.000000", "-zl", "0:08W", "51:30N", "-R0", "1", "3"}, newBand([]string{"-R0", "1", "3"}))
	_, argvLog := fakeBinary(t, t.TempDir(), "echo ran")
	routes := map[string]func() (BodyCall, error){
		"with -R0 2": func() (BodyCall, error) { return base.with("-R0", "2") },
		"with -R 2":  func() (BodyCall, error) { return base.with("-R", "2") },
		"with -Yn":   func() (BodyCall, error) { return base.with("-Yn") },
		"derive appending": func() (BodyCall, error) {
			return base.derive(func(a []string) []string { return append(a, "-RT0", "2") })
		},
		"derive rewriting": func() (BodyCall, error) {
			return base.derive(func(a []string) []string { return append([]string{"-k"}, a[1:]...) })
		},
		"substitute": func() (BodyCall, error) { return base.substitute(0, "-R0") },
	}
	for name, route := range routes {
		call, err := route()
		if err == nil {
			t.Errorf("%s: a call gained a restriction without error: %v", name, call.argv)
			continue
		}
		if _, _, spawnErr := spawn(context.Background(), call); spawnErr == nil {
			t.Errorf("%s: the refused (zero) call was spawned", name)
		}
	}
	if _, statErr := os.Stat(argvLog); !errors.Is(statErr, os.ErrNotExist) {
		t.Error("a process was started for a refused call")
	}
	// Control: the permitted additions go through, and the result still runs.
	ok, err := base.with("-k", "-aa", "-Xo", "/tmp/x.svg")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := spawn(context.Background(), ok); err != nil {
		t.Fatalf("a call with permitted additions was refused: %v", err)
	}
	if got := spawnArgvs(t, argvLog); len(got) != 1 {
		t.Errorf("spawns = %d, want 1", len(got))
	}
}

// C16a, after compilation: the Invocation's visible Argv is mutable, but the
// call the compiler built with its band is a private snapshot, so appending a
// restriction afterwards never reaches Astrolog.
func TestInvocationMutationAfterCompileNeverReachesAstrolog(t *testing.T) {
	spec, err := recipe.NewChartSpec(recipe.MomentAtPlace{JD: 2451545.0, Lon: 0, Lat: 51.5}, recipe.WholeSign, recipe.Tropical,
		recipe.ObjectPolicy{Baseline: recipe.AstrologDefault})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := Compile(spec, recipe.Positions{})
	if err != nil {
		t.Fatal(err)
	}
	inv := plan.Invocations[0]

	_, argvLog := fakeBinary(t, t.TempDir(), "echo ran")
	if _, err := RunPositionsOnly(context.Background(), inv.Call()); err == nil {
		// Control: the unmutated call runs (a fake binary prints no chart, so
		// the run may fail later; what matters is that it spawned).
		_ = err
	}
	if got := spawnArgvs(t, argvLog); len(got) != 1 {
		t.Fatalf("control: the unmutated invocation spawned %d times, want 1", len(got))
	}
	clean := spawnArgvs(t, argvLog)[0]

	inv.Argv = append(inv.Argv, "-R0", "2")
	if err := os.Remove(argvLog); err != nil {
		t.Fatal(err)
	}
	_, runErr := RunPositionsOnly(context.Background(), inv.Call())
	for _, argv := range spawnArgvs(t, argvLog) {
		if slices.Contains(argv, "2") && slices.Contains(argv, "-R0") && !slices.Equal(argv, clean) {
			t.Errorf("a restriction appended after compile reached the binary: %v", argv)
		}
	}
	if runErr == nil {
		t.Error("the mutated invocation ran")
	}
	if got := spawnArgvs(t, argvLog); len(got) != 0 {
		t.Errorf("the mutated invocation started %d processes, want none", len(got))
	}
}
