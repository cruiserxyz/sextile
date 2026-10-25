package astrolog

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/cruiserxyz/sextile/internal/recipe"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// the specification: every call clears the host's calculation hooks
// by name, and a chart call refuses on any Astrolog warning.

// wantHookNames is the band, in the order the spec fixes: the 54 hooks of
// Astrolog 8.00 (the 52 its help advertises, expanded, plus XQ and WQ).
var wantHookNames = strings.Fields("g a a0 m ma j j0 7 L E Zd d dv t P O C A p p0 kO kA kv F FC FO FA FN FT v v3 v30 sd U U0 Ux Iv IV Ia IA Xv XL Xt XQ WQ q1 q2 Q1 Q2 Q3 5s 5f 5Y 5i")

// stripHookPrefix checks that argv starts with `-YQ 0` and then every ordered
// clear pair of HookClearBand, and returns the call's own argv after them. It
// is the shared helper behind every argv log reader here (N5): a missing `""`,
// a swapped pair or an empty band is an error, never a quiet strip of two
// elements.
func stripHookPrefix(argv []string) ([]string, error) {
	band := HookClearBand()
	if len(band) == 0 {
		return nil, errors.New("the hook-clear band is empty")
	}
	if len(argv) < 2 || argv[0] != "-YQ" || argv[1] != "0" {
		return nil, errors.New("argv does not start with -YQ 0")
	}
	rest := argv[2:]
	if len(rest) < len(band) || !slices.Equal(rest[:len(band)], band) {
		return nil, errors.New("argv does not carry every hook clear, in order, right after -YQ 0")
	}
	return rest[len(band):], nil
}

// hookBandCommandWords is the band as it appears in a recorded command line.
func hookBandCommandWords() []string {
	var out []string
	for _, w := range HookClearBand() {
		out = append(out, shellQuote(w))
	}
	return out
}

// rawArgvRecords reads the fake binary's log without stripping anything.
func rawArgvRecords(t *testing.T, argvLog string) [][]string {
	t.Helper()
	data, err := os.ReadFile(argvLog)
	if err != nil {
		t.Fatal(err)
	}
	var out [][]string
	var cur []string
	for _, line := range strings.Split(strings.TrimSuffix(string(data), "\n"), "\n") {
		if line == argvRecordEnd {
			out = append(out, cur)
			cur = nil
			continue
		}
		cur = append(cur, line)
	}
	return out
}

// C1 / N5: every process starts with -YQ 0, the 54 clears in order, then the
// call's own argv; no argv carries -0~ or -~0.
func TestEveryCallClearsHooks(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	_, argvLog := fakeBinary(t, dir, "exit 0")
	own := []string{"-qa", "3", "21", "1985", "14:30", "0", "0:10W", "51:30N"}
	calls := []struct {
		name string
		call spawnCall
		own  []string
	}{
		{"body call", newBodyCall(own, newBand()), own},
		{"exempt call", newExempt("test: no chart", "-HO"), []string{"-HO"}},
		{"bounded exempt call", newExempt("test: no chart", "-Hc").bounded(), []string{"-Hc"}},
	}
	for _, c := range calls {
		if _, _, err := spawn(ctx, c.call); err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
	}
	band := HookClearBand()
	if len(band) != 2*len(wantHookNames) {
		t.Fatalf("HookClearBand has %d elements, want %d (54 pairs)", len(band), 2*len(wantHookNames))
	}
	for i, name := range wantHookNames {
		if band[2*i] != "-~"+name || band[2*i+1] != "" {
			t.Fatalf("band pair %d = %q %q, want %q \"\"", i, band[2*i], band[2*i+1], "-~"+name)
		}
	}
	records := rawArgvRecords(t, argvLog)
	if len(records) != len(calls) {
		t.Fatalf("%d spawns logged, want %d", len(records), len(calls))
	}
	for i, c := range calls {
		want := append(append([]string{"-YQ", "0"}, band...), c.own...)
		if !reflect.DeepEqual(records[i], want) {
			t.Errorf("%s: the process received %q, want -YQ 0, the band, then %q", c.name, records[i], c.own)
		}
		for _, a := range records[i] {
			if a == "-0~" || a == "-~0" {
				t.Errorf("%s: argv carries %q, which a host file can defeat", c.name, a)
			}
		}
		got, err := stripHookPrefix(records[i])
		if err != nil || !slices.Equal(got, c.own) {
			t.Errorf("%s: stripHookPrefix = %q, %v; want %q", c.name, got, err, c.own)
		}
	}
	// The shared helper must not mask a missing or swapped clear.
	t.Run("helper refuses a damaged band", func(t *testing.T) {
		whole := append(append([]string{"-YQ", "0"}, band...), "-HO")
		missingEmpty := slices.Delete(slices.Clone(whole), 3, 4) // drops the first ""
		swapped := slices.Clone(whole)
		swapped[2], swapped[4] = swapped[4], swapped[2]
		for name, argv := range map[string][]string{"missing empty": missingEmpty, "swapped pair": swapped, "no band": {"-YQ", "0", "-HO"}} {
			if _, err := stripHookPrefix(argv); err == nil {
				t.Errorf("%s: stripHookPrefix accepted %q", name, argv[:min(8, len(argv))])
			}
		}
	})
}

var helpHookLine = regexp.MustCompile(`^\s*-~([^\s:]*)`)
var helpBracket = regexp.MustCompile(`^([A-Za-z0-9]*)\[([^\]]*)\]$`)
var helpRange = regexp.MustCompile(`^(\d)-(\d)$`)

// C2: the band names exactly the hooks the installed Astrolog advertises, plus
// XQ and WQ, which only astrolog.h lists. Set equality, so a help format this
// parser does not recognise fails instead of passing on a partial parse.
func TestHookClearBandCoversAstrologHelp(t *testing.T) {
	needAstrolog(t)
	out, _ := exec.Command("astrolog", "-Y").CombinedOutput()
	utilities := map[string]bool{"": true, "M": true, "1": true, "2[0]": true, "3": true, "0": true}
	want := map[string]bool{"XQ": true, "WQ": true}
	lines := 0
	for _, line := range strings.Split(string(out), "\n") {
		m := helpHookLine.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		lines++
		name := m[1]
		if utilities[name] {
			continue
		}
		var names []string
		switch b := helpBracket.FindStringSubmatch(name); {
		case b != nil && helpRange.MatchString(b[2]):
			r := helpRange.FindStringSubmatch(b[2])
			for d := r[1][0]; d <= r[2][0]; d++ {
				names = append(names, b[1]+string(d))
			}
		case b != nil:
			names = append(names, b[1])
			for _, o := range strings.Split(b[2], ",") {
				names = append(names, b[1]+o)
			}
		case regexp.MustCompile(`^[A-Za-z0-9]+$`).MatchString(name):
			names = []string{name}
		default:
			t.Fatalf("help line %q has a hook form this verifier does not recognise", line)
		}
		for _, n := range names {
			want[n] = true
		}
	}
	if lines < 40 {
		t.Fatalf("parsed %d -~ help lines from `astrolog -Y`, want the whole listing", lines)
	}
	got := map[string]bool{}
	band := HookClearBand()
	for i := 0; i+1 < len(band); i += 2 {
		name, ok := strings.CutPrefix(band[i], "-~")
		if !ok || band[i+1] != "" {
			t.Fatalf("band pair %d = %q %q is not `-~<hook> \"\"`", i/2, band[i], band[i+1])
		}
		if got[name] {
			t.Errorf("hook %q is cleared twice", name)
		}
		got[name] = true
	}
	for n := range want {
		if !got[n] {
			t.Errorf("Astrolog advertises hook %q and the band does not clear it", n)
		}
	}
	for n := range got {
		if !want[n] {
			t.Errorf("the band clears %q, which the help does not account for", n)
		}
	}
}

// C3: the command spawn returns is exactly what ran, the band included.
func TestRecordedCommandIncludesHookClears(t *testing.T) {
	dir := t.TempDir()
	path, argvLog := fakeBinary(t, dir, "exit 0")
	_, bodyCmd, err := spawn(context.Background(), testCall(singleChartArgv))
	if err != nil {
		t.Fatal(err)
	}
	_, exemptCmd, err := invoke(context.Background(), []string{"-HO"})
	if err != nil {
		t.Fatal(err)
	}
	records := rawArgvRecords(t, argvLog)
	if len(records) != 2 {
		t.Fatalf("%d spawns logged, want 2", len(records))
	}
	for i, got := range []string{bodyCmd, exemptCmd} {
		if want := formatCommand(path, records[i]); got != want {
			t.Errorf("spawn %d: command %q is not what ran, %q", i, got, want)
		}
		if !strings.Contains(got, " -YQ 0 '-~g' '' '-~a' '' ") {
			t.Errorf("spawn %d: command %q does not show the clears", i, got)
		}
	}
}

// ---- the live hostile-host fixtures --------------------------------------

var hookHostLines = []string{
	`-~A "=z 0.0"`,
	`-~O "If Equ @v 1 =w Add @w 30.0"`,
	`-~C "=y Add @y 30.0"`,
	`-~t "=z 0.0"`,
}

var (
	hookSingle = recipe.MomentAtPlace{JD: 2461156.0, Lon: 13 + 25.0/60, Lat: 52 + 31.0/60}
	hookOther  = recipe.MomentAtPlace{JD: 2446232.0, Lon: 13 + 24.0/60, Lat: 52 + 31.0/60}
)

func hookTimelineRequest() TimelineRequest {
	// JD 2461156.0 is 9611 days after J2000 noon.
	when := time.Date(2000, 1, 1, 12, 0, 0, 0, time.UTC).Add(9611 * 24 * time.Hour)
	return TimelineRequest{
		Year: 2026, Month: time.June, Zone: time.UTC, Defaults: measuredSets,
		Natal:   &TimelineNatal{When: when, Lon: hookSingle.Lon, Lat: hookSingle.Lat},
		Houses:  recipe.WholeSign,
		Zodiac:  recipe.Tropical,
		Objects: recipe.ObjectPolicy{Baseline: recipe.AstrologDefault},
	}
}

// rawAstrolog runs the binary with -YQ 0 and the given argv and NO hook clears,
// returning stdout and stderr: the control that shows a hostile hook is live.
// A raw call that fails is a broken control, never a "difference": an empty
// stdout would otherwise parse as zero rows and pass a hook control vacuously.
func rawAstrolog(t *testing.T, argv []string) (stdout, stderr string) {
	t.Helper()
	cmd := exec.Command("astrolog", append([]string{"-YQ", "0"}, argv...)...)
	var o, e strings.Builder
	cmd.Stdout, cmd.Stderr = &o, &e
	if err := cmd.Run(); err != nil {
		t.Fatalf("raw control call failed (%v): stderr %q", err, e.String())
	}
	return o.String(), e.String()
}

// C4 + C5: a host astrolog.as with four live hooks cannot move what Sextile
// parses; each hook is shown live by a raw call without the clears.
func TestHostHooksCannotMoveTheChart(t *testing.T) {
	needAstrolog(t)
	single := fixtureSpec(t, hookSingle, recipe.WholeSign, recipe.Tropical)
	comparison := fixtureSpec(t, recipe.Comparison{Primary: hookOther, Secondary: hookSingle, Relation: recipe.TransitToNatal}, recipe.WholeSign, recipe.Tropical)
	cleanDir, hostileDir := dataDirWith(t), dataDirWith(t, hookHostLines...)

	posCall := compileOne(t, single, recipe.Positions{}).Call()
	rawPositions, err := posCall.derive(colorPositionsArgv)
	if err != nil {
		t.Fatal(err)
	}
	rawAspects, err := posCall.derive(aspectsArgv)
	if err != nil {
		t.Fatal(err)
	}
	tlCall, err := TimelineCall(TimelineTransit, TimelineSegments(2026, time.June, time.UTC)[0], hookTimelineRequest())
	if err != nil {
		t.Fatal(err)
	}

	type answer struct {
		Single   any
		Cross    any
		Timeline []string
	}
	run := func(t *testing.T) answer {
		t.Helper()
		var a answer
		chart, err := ComputeChart(context.Background(), single)
		if err != nil {
			t.Fatalf("ComputeChart: %v", err)
		}
		if len(chart.Positions) < 10 || len(chart.Aspects) == 0 {
			t.Fatalf("parsed %d positions and %d aspects: two empty parses must never compare equal", len(chart.Positions), len(chart.Aspects))
		}
		// Twelve real cusps on every side: parseCusps answers the zero frame
		// when it cannot find them, and two zero frames compare equal.
		if !chart.Cusps.Valid() {
			t.Fatalf("parsed cusps %v are not twelve real values", chart.Cusps)
		}
		for i, v := range chart.Cusps {
			if i > 0 && v == chart.Cusps[0] {
				t.Fatalf("cusp %d equals cusp 1 (%v): not a parsed frame", i+1, chart.Cusps)
			}
		}
		a.Single = []any{chart.Positions, chart.Cusps, chart.Aspects}
		cross, err := ComputeCrossAspects(context.Background(), comparison)
		if err != nil {
			t.Fatalf("ComputeCrossAspects: %v", err)
		}
		if len(cross.Aspects) == 0 {
			t.Fatal("parsed no cross-aspects")
		}
		a.Cross = cross.Aspects
		rows, _, err := ComputeTimeline(context.Background(), hookTimelineRequest(), []TimelineSource{TimelineTransit}, nil)
		if err != nil {
			t.Fatalf("ComputeTimeline: %v", err)
		}
		if len(rows) == 0 {
			t.Fatal("parsed no timeline rows")
		}
		for _, r := range rows {
			a.Timeline = append(a.Timeline, r.Instant.UTC().Format(time.RFC3339)+" "+r.Line)
		}
		return a
	}
	useDataDir(t, cleanDir)
	clean := run(t)
	useDataDir(t, hostileDir)
	hostile := run(t)
	if !reflect.DeepEqual(clean.Single, hostile.Single) {
		t.Errorf("a host file moved the single chart (positions, cusps or aspects)")
	}
	if !reflect.DeepEqual(clean.Cross, hostile.Cross) {
		t.Errorf("a host file moved the cross-aspects")
	}
	if !reflect.DeepEqual(clean.Timeline, hostile.Timeline) {
		t.Errorf("a host file moved the timeline rows: %d clean, %d hostile", len(clean.Timeline), len(hostile.Timeline))
	}

	// C5: each hook is live in the file Astrolog loaded.
	both := func(argv []string) (c, h string) {
		useDataDir(t, cleanDir)
		c, _ = rawAstrolog(t, argv)
		useDataDir(t, hostileDir)
		h, _ = rawAstrolog(t, argv)
		return c, h
	}
	t.Run("control/A", func(t *testing.T) {
		c, h := both(rawAspects.Argv())
		if nc, nh := len(parseAspects(c)), len(parseAspects(h)); nc == 0 || nh != 0 {
			t.Errorf("~A is not live: %d clean aspects, %d hostile, want >0 and 0", nc, nh)
		}
	})
	t.Run("control/O", func(t *testing.T) {
		c, h := both(rawPositions.Argv())
		sun := func(out string) (float64, bool) {
			for _, p := range parsePositions(out) {
				if p.Body == aspect.Sun {
					return p.Longitude, true
				}
			}
			return 0, false
		}
		lc, okc := sun(c)
		lh, okh := sun(h)
		if !okc || !okh {
			t.Fatalf("no Sun parsed (clean %v, hostile %v)", okc, okh)
		}
		if d := math.Mod(lh-lc+360, 360); math.Abs(d-30) > 0.5 {
			t.Errorf("~O is not live: the Sun moved %.3f degrees (%.3f to %.3f), want about 30", d, lc, lh)
		}
	})
	t.Run("control/C", func(t *testing.T) {
		c, h := both(rawPositions.Argv())
		cc, ch := parseCusps(c), parseCusps(h)
		if !cc.Valid() || !ch.Valid() {
			t.Fatalf("cusps not parsed (clean %v, hostile %v)", cc, ch)
		}
		// Cusp 1 (the Ascendant's house) is the named cusp.
		if d := math.Mod(ch[0]-cc[0]+360, 360); math.Abs(d-30) > 0.5 {
			t.Errorf("~C is not live: cusp 1 moved %.3f degrees (%.3f to %.3f), want about 30", d, cc[0], ch[0])
		}
	})
	t.Run("control/t", func(t *testing.T) {
		// Its own hostile copy: ONLY ~t, so the difference is ~t's.
		only := dataDirWith(t, `-~t "=z 0.0"`)
		useDataDir(t, cleanDir)
		c, _ := rawAstrolog(t, tlCall.Argv())
		useDataDir(t, only)
		h, _ := rawAstrolog(t, tlCall.Argv())
		rc, errc := ParseTimeline(c, time.UTC)
		if errc != nil || len(rc) == 0 {
			t.Fatalf("clean timeline parsed %d rows, err %v", len(rc), errc)
		}
		if !strings.Contains(h, "Transits during") {
			t.Fatalf("the hostile -t run printed no search header, so it never searched: %q", h)
		}
		rh, errh := ParseTimeline(h, time.UTC)
		if errh != nil {
			t.Fatalf("hostile timeline: %v", errh)
		}
		if len(rh) != 0 {
			t.Errorf("~t is not live: the hostile run still parsed %d rows (clean %d), want 0", len(rh), len(rc))
		}
	})
}

// C6: a hook that fails to parse is inert under the clears. Its own copy: a
// parse error disables every hook.
func TestBrokenHostHookIsInert(t *testing.T) {
	needAstrolog(t)
	spec := fixtureSpec(t, hookSingle, recipe.WholeSign, recipe.Tropical)
	cleanDir := dataDirWith(t)
	brokenDir := dataDirWith(t, `-~A "If Not Equ @v O_Jup =z 0.0 Bogus"`)
	useDataDir(t, cleanDir)
	clean, err := ComputeChart(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}
	useDataDir(t, brokenDir)
	broken, err := ComputeChart(context.Background(), spec)
	if err != nil {
		t.Fatalf("a cleared broken hook must not fail the call: %v", err)
	}
	if len(clean.Positions) < 10 || len(clean.Aspects) == 0 {
		t.Fatal("parsed an empty chart")
	}
	if !reflect.DeepEqual(clean.Positions, broken.Positions) || !reflect.DeepEqual(clean.Aspects, broken.Aspects) {
		t.Errorf("a broken host hook changed the chart")
	}
	t.Run("control", func(t *testing.T) {
		call, err := compileOne(t, spec, recipe.Positions{}).Call().derive(aspectsArgv)
		if err != nil {
			t.Fatal(err)
		}
		_, stderr := rawAstrolog(t, call.Argv())
		if !strings.Contains(stderr, "Unknown function: 'Bogus'") {
			t.Errorf("the broken hook is not live: raw stderr = %q", stderr)
		}
	})
}

// shimWith installs a shim astrolog whose body is script and returns the
// spawn helper for a body call.
func warnShim(t *testing.T, script string) string {
	t.Helper()
	_, argvLog := fakeBinary(t, t.TempDir(), script)
	return argvLog
}

// C7: any stderr line beyond the two wheel progress lines refuses a body call.
func TestChartCallRefusesOnWarning(t *testing.T) {
	ctx := context.Background()
	t.Run("one warning line", func(t *testing.T) {
		warnShim(t, "echo 'CHART-STDOUT'; echo 'SwissEph file se00010s.se1 not found' >&2")
		out, command, err := spawn(ctx, testCall(singleChartArgv))
		if !errors.Is(err, ErrAstrologWarned) {
			t.Fatalf("err = %v, want ErrAstrologWarned", err)
		}
		if !strings.Contains(err.Error(), "SwissEph file se00010s.se1 not found") {
			t.Errorf("the error does not carry the warning verbatim: %v", err)
		}
		if command == "" {
			t.Errorf("a refused call must still return its command")
		}
		if out != "" {
			t.Errorf("the refused call's stdout was returned for parsing: %q", out)
		}
	})
	t.Run("Run returns no chart", func(t *testing.T) {
		warnShim(t, "echo 'warned' >&2\n"+replayScript(t))
		raw, err := Run(ctx, testCall(singleChartArgv))
		if !errors.Is(err, ErrAstrologWarned) {
			t.Fatalf("err = %v, want ErrAstrologWarned", err)
		}
		if len(raw.Positions) != 0 || len(raw.Commands) == 0 {
			t.Errorf("positions %d, commands %d: want a refused chart that still names what ran", len(raw.Positions), len(raw.Commands))
		}
	})
	t.Run("cancellation wins over a warning", func(t *testing.T) {
		warnShim(t, "echo 'warned' >&2; exec sleep 5")
		cctx, cancel := context.WithCancel(ctx)
		time.AfterFunc(300*time.Millisecond, cancel)
		_, _, err := spawn(cctx, testCall(singleChartArgv))
		if !errors.Is(err, context.Canceled) || errors.Is(err, ErrAstrologWarned) {
			t.Fatalf("err = %v, want the cancellation error alone", err)
		}
	})
	t.Run("nonzero exit with a warning matches both", func(t *testing.T) {
		warnShim(t, "echo 'warned' >&2; exit 3")
		_, _, err := spawn(ctx, testCall(singleChartArgv))
		var exit *exec.ExitError
		if !errors.Is(err, ErrAstrologWarned) || !errors.As(err, &exit) {
			t.Fatalf("err = %v, want both ErrAstrologWarned and the exit failure", err)
		}
	})
}

// N1: a nonzero exit with a warning, or a cancellation, keeps its identity and
// the command the warning split must not drop.
func TestChartWarningWithNonzeroExitKeepsCommand(t *testing.T) {
	ctx := context.Background()
	t.Run("nonzero exit", func(t *testing.T) {
		warnShim(t, "echo 'warned' >&2; exit 3")
		_, command, err := spawn(ctx, testCall(singleChartArgv))
		var exit *exec.ExitError
		if !errors.As(err, &exit) || exit.ExitCode() != 3 {
			t.Errorf("err = %v: the exit failure was replaced by the warning sentinel", err)
		}
		if !strings.Contains(command, "-YQ 0") {
			t.Errorf("command = %q: the refused call lost its command", command)
		}
	})
	t.Run("cancellation", func(t *testing.T) {
		warnShim(t, "echo 'warned' >&2; exec sleep 5")
		cctx, cancel := context.WithCancel(ctx)
		time.AfterFunc(300*time.Millisecond, cancel)
		_, command, err := spawn(cctx, testCall(singleChartArgv))
		if !errors.Is(err, context.Canceled) {
			t.Errorf("err = %v, want cancellation", err)
		}
		if command == "" {
			t.Errorf("the cancelled call lost its command")
		}
	})
}

// C8: the wheel's two progress lines are not warnings.
func TestWheelProgressLinesAreNotWarnings(t *testing.T) {
	ctx := context.Background()
	progress := "echo 'Creating graphics chart in memory.' >&2; echo 'Writing chart bitmap to file.  ' >&2; echo >&2\n"
	t.Run("both lines alone succeed with stdout alone", func(t *testing.T) {
		warnShim(t, "echo 'CHART'\n"+progress)
		out, _, err := spawn(ctx, testCall(singleChartArgv))
		if err != nil || out != "CHART\n" {
			t.Fatalf("out %q, err %v; want stdout alone and no error", out, err)
		}
	})
	t.Run("a third line refuses", func(t *testing.T) {
		warnShim(t, "echo 'CHART'\n"+progress+"echo 'a third line' >&2")
		if _, _, err := spawn(ctx, testCall(singleChartArgv)); !errors.Is(err, ErrAstrologWarned) {
			t.Fatalf("err = %v, want ErrAstrologWarned", err)
		}
	})
	t.Run("Run keeps its Raw format", func(t *testing.T) {
		warnShim(t, progress+replayScript(t))
		raw, err := Run(ctx, testCall(singleChartArgv))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(raw.Raw, "\n---\n") || strings.Contains(raw.Raw, "Creating graphics") {
			t.Errorf("Raw = %.200q: want positions --- aspects with no stderr in it", raw.Raw)
		}
	})
}

// C9: an expression the host file evaluates while loading prints to stderr
// before any switch applies; a chart call refuses on it.
func TestHostStartupExpressionRefusesChart(t *testing.T) {
	needAstrolog(t)
	useDataDir(t, dataDirWith(t, `-~ "Add 40 2"`))
	_, err := ComputeChart(context.Background(), fixtureSpec(t, hookSingle, recipe.WholeSign, recipe.Tropical))
	if !errors.Is(err, ErrAstrologWarned) || !strings.Contains(fmt.Sprint(err), "Expression returned: 42") {
		t.Fatalf("err = %v, want ErrAstrologWarned naming `Expression returned: 42`", err)
	}
}

// C10: a chart selecting Hygiea succeeds. With the file missing, Hygiea is
// absent from the positions and named unavailable (today's "not drawn" path);
// with it present, its speed is non-zero. The missing-ephemeris line is the one
// expected stderr shape; beside any other line, or anywhere but the whole
// line, it still refuses.
func TestMissingEphemerisFileKeepsTheChart(t *testing.T) {
	missing := missingLine("se00010s.se1")
	t.Run("live Hygiea", func(t *testing.T) {
		needAstrolog(t)
		spec, err := recipe.NewChartSpec(hookOther, recipe.WholeSign, recipe.Tropical, recipe.ObjectPolicy{Baseline: recipe.Explicit, Selection: []int{1, 43}})
		if err != nil {
			t.Fatal(err)
		}
		out, err := ComputeChart(context.Background(), spec)
		if err != nil {
			t.Fatalf("a chart selecting Hygiea must succeed: %v", err)
		}
		var hygiea []string
		for _, p := range out.Positions {
			if strings.HasPrefix(p.Label, "Hygi") {
				hygiea = append(hygiea, p.Label)
				if p.Speed == 0 {
					t.Errorf("Hygiea is listed at speed 0: %v", p)
				}
			}
		}
		if len(hygiea) > 0 {
			return // the file is present on this host
		}
		if !slices.Contains(out.Unavailable, "Hygiea") && !slices.ContainsFunc(out.Unavailable, func(n string) bool { return strings.HasPrefix(n, "Hygi") }) {
			t.Errorf("Hygiea is neither listed nor named unavailable: positions %v, unavailable %v", out.Positions, out.Unavailable)
		}
	})
	t.Run("the expected line alone succeeds", func(t *testing.T) {
		warnShim(t, "echo 'CHART'; echo \""+missing+"\" >&2")
		out, _, err := spawn(context.Background(), testCall(singleChartArgv))
		if err != nil || out != "CHART\n" {
			t.Fatalf("out %q err %v; want the chart and no error", out, err)
		}
	})
	t.Run("beside another warning refuses", func(t *testing.T) {
		warnShim(t, "echo 'CHART'; echo \""+missing+"\" >&2; echo 'another warning' >&2")
		if _, _, err := spawn(context.Background(), testCall(singleChartArgv)); !errors.Is(err, ErrAstrologWarned) {
			t.Fatalf("err = %v, want ErrAstrologWarned", err)
		}
	})
	t.Run("shape is anchored", func(t *testing.T) {
		for _, line := range []string{
			"xx SwissEph file 'a.se1' not found in PATH 'p'",
			"SwissEph file 'a.se1' not found in PATH 'p' and more",
			"SwissEph file 'a.txt' not found in PATH 'p'",
			"SwissEph file 'a.se1' not found",
		} {
			warnShim(t, "echo 'CHART'; echo \""+line+"\" >&2")
			if _, _, err := spawn(context.Background(), testCall(singleChartArgv)); !errors.Is(err, ErrAstrologWarned) {
				t.Errorf("%q: err = %v, want ErrAstrologWarned", line, err)
			}
		}
	})
}

// C11: a metadata call keeps today's folded output and does not refuse.
func TestMetadataCallsKeepWarningsInOutput(t *testing.T) {
	ctx := context.Background()
	t.Run("exempt folds, body refuses", func(t *testing.T) {
		warnShim(t, "echo 'STDOUT-LINE'; echo 'warning on stderr' >&2")
		out, _, err := invoke(ctx, []string{"-HO"})
		if err != nil || !strings.Contains(out, "STDOUT-LINE") || !strings.Contains(out, "warning on stderr") {
			t.Fatalf("exempt call: out %q, err %v; want both streams folded and no error", out, err)
		}
		if _, _, err := spawn(ctx, testCall(singleChartArgv)); !errors.Is(err, ErrAstrologWarned) {
			t.Fatalf("body call: err = %v, want ErrAstrologWarned", err)
		}
	})
	t.Run("interleaved and nonzero", func(t *testing.T) {
		warnShim(t, "echo one; echo two >&2; echo three; echo four >&2; exit 3")
		out, command, err := invoke(ctx, []string{"-HO"})
		var exit *exec.ExitError
		if !errors.As(err, &exit) {
			t.Fatalf("err = %v, want the exit failure", err)
		}
		if out != "one\ntwo\nthree\nfour\n" || command == "" {
			t.Errorf("out %q command %q: want the lines folded in written order, and the command", out, command)
		}
	})
	t.Run("live object registry", func(t *testing.T) {
		needAstrolog(t)
		objs, err := ObjectRegistry(ctx)
		if err != nil || len(objs) < 100 {
			t.Fatalf("ObjectRegistry: %d objects, %v; want the full registry", len(objs), err)
		}
	})
}

// N12: HookClearBand hands out an independent slice.
func TestHookClearBandReturnsIndependentArgv(t *testing.T) {
	first := HookClearBand()
	if len(first) == 0 {
		t.Fatal("HookClearBand is empty")
	}
	want := slices.Clone(first)
	first[0] = "-X"
	_ = append(first[:1], "junk")
	second := HookClearBand()
	if !slices.Equal(second, want) {
		t.Fatalf("a caller's edit reached a later result: %q", second[:4])
	}
	_, argvLog := fakeBinary(t, t.TempDir(), "exit 0")
	if _, _, err := invoke(context.Background(), []string{"-HO"}); err != nil {
		t.Fatal(err)
	}
	if got := rawArgvRecords(t, argvLog)[0]; !slices.Equal(got[2:2+len(want)], want) {
		t.Errorf("a caller's edit reached the launch: %q", got[:6])
	}
}

// N13: a shim filling both pipes past their buffers completes.
func TestChartWarningDrainsBothStreams(t *testing.T) {
	warnShim(t, "(head -c 400000 /dev/zero | tr '\\0' b >&2) &\nhead -c 400000 /dev/zero | tr '\\0' a\nwait")
	done := make(chan error, 1)
	go func() {
		_, _, err := spawn(context.Background(), testCall(singleChartArgv))
		done <- err
	}()
	select {
	case err := <-done:
		if !errors.Is(err, ErrAstrologWarned) {
			t.Fatalf("err = %v, want ErrAstrologWarned", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("spawn did not complete: a full pipe blocked the process")
	}
}

// N14: a refusal carries the warning and no chart output.
func TestWarningErrorCarriesNoChartOutput(t *testing.T) {
	const secret = "CLIENT-POSITIONS-SENTINEL"
	ctx := context.Background()
	warnShim(t, "echo '"+secret+"'; echo 'the warning' >&2")
	check := func(name string, err error) {
		t.Helper()
		if !errors.Is(err, ErrAstrologWarned) {
			t.Errorf("%s: err = %v, want ErrAstrologWarned", name, err)
			return
		}
		if strings.Contains(err.Error(), secret) || !strings.Contains(err.Error(), "the warning") {
			t.Errorf("%s: error text %q must carry the warning and no chart stdout", name, err.Error())
		}
	}
	_, err := Run(ctx, testCall(singleChartArgv))
	check("Run", err)
	_, err = RunPositionsOnly(ctx, testCall(singleChartArgv))
	check("RunPositionsOnly", err)
	out, _, err := RunTimelineSearch(ctx, testCall(singleChartArgv))
	check("RunTimelineSearch", err)
	if out != "" {
		t.Errorf("RunTimelineSearch returned the refused call's stdout: %q", out)
	}
	_, _, err = ComputeRawReport(ctx, hellenisticSpec(t), recipe.ChartRawReport())
	check("ComputeRawReport", err)
}

// N2: a wheel returns no image although Astrolog wrote one, when it also warned.
func TestWheelWarningDiscardsImage(t *testing.T) {
	script := `prev=""; for a in "$@"; do [ "$prev" = -Xo ] && printf IMAGE > "$a"; prev="$a"; done
echo 'Creating graphics chart in memory.' >&2
echo 'Writing chart bitmap to file.' >&2
`
	t.Run("progress lines alone return the image", func(t *testing.T) {
		warnShim(t, script)
		img, commands, err := RenderWheelImageCommands(context.Background(), hellenisticSpec(t), recipe.DefaultWheelRender())
		if err != nil || string(img) != "IMAGE" || len(commands) != 1 {
			t.Fatalf("img %q commands %d err %v; want the image from a clean wheel", img, len(commands), err)
		}
	})
	t.Run("a warning beside them discards it", func(t *testing.T) {
		warnShim(t, script+"echo 'SwissEph file se00010s.se1 not found' >&2")
		img, commands, err := RenderWheelImageCommands(context.Background(), hellenisticSpec(t), recipe.DefaultWheelRender())
		if !errors.Is(err, ErrAstrologWarned) {
			t.Fatalf("err = %v, want ErrAstrologWarned", err)
		}
		if img != nil || len(commands) != 1 {
			t.Errorf("img %q commands %d: want no image and the command that ran", img, len(commands))
		}
	})
}

// N7: positions succeed and the aspect spawn warns: no chart, both commands.
func TestRunAspectWarningReturnsNoChart(t *testing.T) {
	warnShim(t, failOnScript(t, "-aa", "echo 'aspect warning' >&2"))
	raw, err := Run(context.Background(), testCall(singleChartArgv))
	if !errors.Is(err, ErrAstrologWarned) {
		t.Fatalf("err = %v, want ErrAstrologWarned", err)
	}
	if len(raw.Positions) != 0 || len(raw.Aspects) != 0 || len(raw.Commands) != 2 {
		t.Errorf("positions %d aspects %d commands %d: want no chart and both commands", len(raw.Positions), len(raw.Aspects), len(raw.Commands))
	}
}

// N8: a raw Details report that warns returns no report.
func TestRawReportWarningReturnsNoReport(t *testing.T) {
	warnShim(t, "echo 'REPORT BODY'; echo 'line one' >&2; echo 'line two' >&2")
	report, command, err := ComputeRawReport(context.Background(), hellenisticSpec(t), recipe.ChartRawReport())
	if !errors.Is(err, ErrAstrologWarned) {
		t.Fatalf("err = %v, want ErrAstrologWarned", err)
	}
	if report != "" || command == "" || !strings.Contains(err.Error(), "line one") || !strings.Contains(err.Error(), "line two") {
		t.Errorf("report %q command %q err %v: want no report, the command, both warning lines", report, command, err)
	}
}

// N9: the ephemeris probe succeeds and the relationship warns: no
// cross-aspects, both commands.
func TestRelationshipWarningAfterProbeReturnsNoAspects(t *testing.T) {
	warnShim(t, failOnScript(t, "-rt", "echo 'relationship warning' >&2"))
	out, err := ComputeCrossAspects(context.Background(), transitSpec(t))
	if !errors.Is(err, ErrAstrologWarned) {
		t.Fatalf("err = %v, want ErrAstrologWarned", err)
	}
	if len(out.Aspects) != 0 || len(out.Commands) != 2 {
		t.Errorf("aspects %d commands %d: want none and both commands", len(out.Aspects), len(out.Commands))
	}
}
