package astrolog

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/cruiserxyz/sextile/internal/recipe"
)

// The verifiers C1-C9: every in-scope spawn records the command it ran.
//
// The tests below install a shell script as the astrolog binary. The script
// appends its argv to a log — one record per spawn, one line per argument,
// records terminated by an "@@" line — and then either replays a captured
// real Astrolog output (testdata/provenance, `-k` fixtures generated once from the
// live binary) or fails in the way the case needs. Spawns are sequential, so
// the log order is spawn order, and a recorded command can be checked against
// the argv the process really received rather than against the code that
// produced it.

const argvRecordEnd = "@@"

// fakeBinary installs script (a POSIX sh body) as the astrolog binary at
// dir/astrolog for the rest of the test and returns its absolute path together
// with the argv log it appends to.
func fakeBinary(t *testing.T, dir, script string) (path, argvLog string) {
	t.Helper()
	argvLog = filepath.Join(dir, "argv.log")
	path = filepath.Join(dir, "astrolog")
	body := "#!/bin/sh\n{ printf '%s\\n' \"$@\"; echo '" + argvRecordEnd + "'; } >> '" + argvLog + "'\n" + script + "\n"
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	old := binary
	binary = path
	t.Cleanup(func() { binary = old })
	return path, argvLog
}

// spawnArgvs reads the fake binary's log back: one argv per spawn, in spawn
// order. A log that was never written means no process ever started.
func spawnArgvs(t *testing.T, argvLog string) [][]string {
	t.Helper()
	data, err := os.ReadFile(argvLog)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	var out [][]string
	var cur []string
	for _, line := range strings.Split(strings.TrimSuffix(string(data), "\n"), "\n") {
		if line == argvRecordEnd {
			// C1: every spawn starts with -YQ 0 and then every hook clear, in
			// order. Check it here (N5), then hand the tests the call's own
			// argv, which is what they are about.
			own, err := stripHookPrefix(cur)
			if err != nil {
				t.Errorf("spawn argv %q: %v (C1)", cur, err)
			} else {
				cur = own
			}
			out = append(out, cur)
			cur = nil
			continue
		}
		cur = append(cur, line)
	}
	return out
}

// bareCommand is the recorded form of a spawn whose elements are all
// shell-safe: path and argv joined by single spaces, nothing quoted. Used
// where the argv is known to be safe, so equality proves both the argv and
// the format without going through formatCommand.
func bareCommand(path string, argv []string) string {
	// A word beginning `=` is quoted by shellQuote (zsh expands `=cmd`): the
	// display pins `=Yt` and `=Yd` are the only such words. An empty word is
	// quoted too: the unnamed chart's `-zi '' ''` (the specification C6).
	argv = slices.Clone(argv)
	for i, a := range argv {
		if strings.HasPrefix(a, "=") || a == "" {
			argv[i] = "'" + a + "'"
		}
	}
	return path + " -YQ 0 " + strings.Join(hookBandCommandWords(), " ") + " " + strings.Join(argv, " ")
}

// replayScript routes each spawn to the captured fixture its argv asks for.
func replayScript(t *testing.T) string {
	t.Helper()
	fix, err := filepath.Abs("testdata/provenance")
	if err != nil {
		t.Fatal(err)
	}
	return `case " $* " in
  *" -rt "*) cat '` + fix + `/cross-k.txt' ;;
  *" -aa "*) cat '` + fix + `/aspects-k.txt' ;;
  *) cat '` + fix + `/positions-k.txt' ;;
esac`
}

// failOnScript replays the fixtures except for the spawn whose argv contains
// marker, which runs onMatch instead (an exit, or a cat of a crafted file).
func failOnScript(t *testing.T, marker, onMatch string) string {
	t.Helper()
	fix, err := filepath.Abs("testdata/provenance")
	if err != nil {
		t.Fatal(err)
	}
	return `case " $* " in
  *" ` + marker + ` "*) ` + onMatch + ` ;;
  *" -rt "*) cat '` + fix + `/cross-k.txt' ;;
  *" -aa "*) cat '` + fix + `/aspects-k.txt' ;;
  *) cat '` + fix + `/positions-k.txt' ;;
esac`
}

func writeTestFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

var singleChartArgv = []string{"-qa", "3", "21", "1985", "14:30", "0", "0:10W", "51:30N"}

func transitSpec(t *testing.T) recipe.ChartSpec {
	t.Helper()
	spec, err := recipe.NewChartSpec(
		recipe.Comparison{
			Primary:   recipe.MomentAtPlace{JD: 2446232.0, Lon: 11.5667, Lat: 48.1333},
			Secondary: recipe.MomentAtPlace{JD: 2451545.0, Lon: -0.1278, Lat: 51.5074},
			Relation:  recipe.TransitToNatal,
		},
		recipe.WholeSign,
		recipe.Tropical,
		recipe.ObjectPolicy{Baseline: recipe.AstrologDefault},
	)
	if err != nil {
		t.Fatalf("comparison spec: %v", err)
	}
	return spec
}

// C1: the command is the path exec resolved plus the exact argv the process
// received — checked against the fake's own record of its argv, not against
// the code that built the command.
func TestInvokeRecordsExecutedCommand(t *testing.T) {
	dir := t.TempDir()
	path, argvLog := fakeBinary(t, dir, "exit 0")
	// Resolve through PATH exactly as the shipped binary does: the record
	// must carry the path exec resolved, not the bare name it was given.
	binary = "astrolog"
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	_, command, err := invoke(context.Background(), singleChartArgv)
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	spawns := spawnArgvs(t, argvLog)
	if len(spawns) != 1 || !slices.Equal(spawns[0], singleChartArgv) {
		t.Fatalf("fake received %q, want one spawn with %q", spawns, singleChartArgv)
	}
	if want := bareCommand(path, spawns[0]); command != want {
		t.Errorf("recorded command = %q, want %q", command, want)
	}

	// A process that never started leaves no command behind.
	binary = filepath.Join(dir, "missing")
	_, command, err = invoke(context.Background(), singleChartArgv)
	if err == nil {
		t.Fatal("missing binary must fail")
	}
	if command != "" {
		t.Errorf("a spawn that never started recorded %q", command)
	}
}

// C2: the quoting rule, element by element, then end to end — the recorded
// command pasted into a shell hands the binary the same argv again.
func TestRecordedCommandIsShellSafe(t *testing.T) {
	cases := []struct{ in, want string }{
		{"-qa", "-qa"},
		{"14:30", "14:30"},
		{"0:10W", "0:10W"},
		{"2446232.000000", "2446232.000000"},
		{"/usr/local/bin/astrolog", "/usr/local/bin/astrolog"},
		{"", "''"},
		{"a b", "'a b'"},
		{"tab\tx", "'tab\tx'"},
		{"it's", `'it'\''s'`},
		{`say "hi"`, `'say "hi"'`},
		{"=j0", "'=j0'"},
	}
	for _, c := range shellUnsafe {
		if c == '\'' {
			continue // covered by the it's case, which has its own encoding
		}
		cases = append(cases, struct{ in, want string }{"x" + string(c), "'x" + string(c) + "'"})
	}
	for _, c := range cases {
		if got := shellQuote(c.in); got != c.want {
			t.Errorf("shellQuote(%q) = %s, want %s", c.in, got, c.want)
		}
	}

	dir := t.TempDir()
	_, argvLog := fakeBinary(t, dir, "exit 0")
	argv := []string{"-qa", "a b", "it's", "$HOME", "", "x|y", "`id`", `say "hi"`, "=j0", "*", "~"}
	_, command, err := invoke(context.Background(), argv)
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	if out, err := exec.Command("sh", "-c", command).CombinedOutput(); err != nil {
		t.Fatalf("pasting the recorded command failed: %v\n%s\ncommand: %s", err, out, command)
	}
	spawns := spawnArgvs(t, argvLog)
	if len(spawns) != 2 {
		t.Fatalf("want the original spawn and the pasted one, got %d", len(spawns))
	}
	if !slices.Equal(spawns[1], spawns[0]) || !slices.Equal(spawns[0], argv) {
		t.Errorf("pasted argv %q differs from executed argv %q", spawns[1], spawns[0])
	}
}

// C3: Run carries both spawns, positions first, then aspects.
func TestRunRecordsBothSpawns(t *testing.T) {
	path, argvLog := fakeBinary(t, t.TempDir(), replayScript(t))
	out, err := Run(context.Background(), testCall(singleChartArgv))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	spawns := spawnArgvs(t, argvLog)
	if len(spawns) != 2 {
		t.Fatalf("Run spawned %d times, want 2", len(spawns))
	}
	if slices.Contains(spawns[0], "-aa") || !slices.Contains(spawns[1], "-aa") {
		t.Errorf("spawn order is not positions then aspects: %q", spawns)
	}
	want := []string{bareCommand(path, spawns[0]), bareCommand(path, spawns[1])}
	if !slices.Equal(out.Commands, want) {
		t.Errorf("Commands = %q, want %q", out.Commands, want)
	}
	if len(out.Positions) == 0 || len(out.Aspects) == 0 {
		t.Errorf("fixture replay did not parse: %d positions, %d aspects", len(out.Positions), len(out.Aspects))
	}
}

// C4: the single-spawn entry point carries exactly that one command.
func TestRunPositionsOnlyRecordsOneSpawn(t *testing.T) {
	path, argvLog := fakeBinary(t, t.TempDir(), replayScript(t))
	out, err := RunPositionsOnly(context.Background(), testCall(singleChartArgv))
	if err != nil {
		t.Fatalf("RunPositionsOnly: %v", err)
	}
	spawns := spawnArgvs(t, argvLog)
	if len(spawns) != 1 {
		t.Fatalf("RunPositionsOnly spawned %d times, want 1", len(spawns))
	}
	if want := []string{bareCommand(path, spawns[0])}; !slices.Equal(out.Commands, want) {
		t.Errorf("Commands = %q, want %q", out.Commands, want)
	}
}

// C5: a return after a process actually started — nonzero exit, sanity
// failure, parser failure, at either spawn, on every in-scope path —
// carries every started spawn's command in spawn order.
func TestReturnAfterSpawnKeepsCommands(t *testing.T) {
	uncoloredPositions, err := filepath.Abs("testdata/natal_1985_positions.txt")
	if err != nil {
		t.Fatal(err)
	}
	type call func(ctx context.Context) ([]string, error)
	runSingle := func(ctx context.Context) ([]string, error) {
		out, err := Run(ctx, testCall(singleChartArgv))
		return out.Commands, err
	}
	runPositionsOnly := func(ctx context.Context) ([]string, error) {
		out, err := RunPositionsOnly(ctx, testCall(singleChartArgv))
		return out.Commands, err
	}
	cases := []struct {
		name       string
		script     func(dir string) string
		call       call
		wantSpawns int
		wantErr    error
	}{
		{"positions nonzero exit", func(string) string { return "exit 3" }, runSingle, 1, nil},
		{"positions sanity: checkPositionSanity", func(dir string) string {
			return "cat '" + writeTestFile(t, dir, "collapsed.txt", "Sun :  0Ari00   - 0:00' (-) [ 1st house]\nMoon:  0Ari00   - 0:00' (-) [ 1st house]\n") + "'"
		}, runSingle, 1, ErrEphemerisDisabled},
		{"positions parser: ParseBodies", func(string) string { return "cat '" + uncoloredPositions + "'" }, runSingle, 1, nil},
		{"aspects nonzero exit", func(string) string { return failOnScript(t, "-aa", "exit 2") }, runSingle, 2, nil},
		{"aspects parser: ParseAspectRows", func(dir string) string {
			return failOnScript(t, "-aa", "cat '"+writeTestFile(t, dir, "uncolored-aspects.txt", "  1: Sun (Ari) Con (Ari) Moon - orb: a0:01 - power: 10.00\n")+"'")
		}, runSingle, 2, nil},
		{"positions-only nonzero exit", func(string) string { return "exit 3" }, runPositionsOnly, 1, nil},
		{"cross sanity: checkCrossAspectSanity", func(string) string { return failOnScript(t, "-rt", "echo garbage") }, func(ctx context.Context) ([]string, error) {
			out, err := ComputeCrossAspects(ctx, transitSpec(t))
			return out.Commands, err
		}, 2, nil},
		{"cross parser: ParseCrossAspectRows", func(dir string) string {
			return failOnScript(t, "-rt", "cat '"+writeTestFile(t, dir, "uncolored-cross.txt", "Chart #2 comparison for Sat Jan  1, 2000\n  1:     Sun (Gem) Squ with (Pis) Jupiter   - orb: +0:22' - power: 15.26\n")+"'")
		}, func(ctx context.Context) ([]string, error) {
			out, err := ComputeCrossAspects(ctx, transitSpec(t))
			return out.Commands, err
		}, 2, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			path, argvLog := fakeBinary(t, dir, c.script(dir))
			commands, err := c.call(context.Background())
			if err == nil {
				t.Fatal("the case must fail")
			}
			if c.wantErr != nil && !errors.Is(err, c.wantErr) {
				t.Errorf("err = %v, want %v", err, c.wantErr)
			}
			spawns := spawnArgvs(t, argvLog)
			if len(spawns) != c.wantSpawns {
				t.Fatalf("%d processes started, want %d", len(spawns), c.wantSpawns)
			}
			var want []string
			for _, argv := range spawns {
				want = append(want, bareCommand(path, argv))
			}
			if !slices.Equal(commands, want) {
				t.Errorf("Commands = %q, want every started spawn %q", commands, want)
			}
		})
	}
}

// C6: the comparison path carries probe then relationship; a probe that
// spawns and fails carries the probe alone; a non-Comparison spawns nothing.
func TestCrossAspectsRecordsSpawns(t *testing.T) {
	t.Run("probe then relationship", func(t *testing.T) {
		path, argvLog := fakeBinary(t, t.TempDir(), replayScript(t))
		out, err := ComputeCrossAspects(context.Background(), transitSpec(t))
		if err != nil {
			t.Fatalf("ComputeCrossAspects: %v", err)
		}
		spawns := spawnArgvs(t, argvLog)
		if len(spawns) != 2 || !slices.Contains(spawns[0], "=v0") || !slices.Contains(spawns[1], "-rt") {
			t.Fatalf("want an ephemeris probe then a relationship run, got %q", spawns)
		}
		want := []string{bareCommand(path, spawns[0]), bareCommand(path, spawns[1])}
		if !slices.Equal(out.Commands, want) {
			t.Errorf("Commands = %q, want %q", out.Commands, want)
		}
		if len(out.Aspects) == 0 {
			t.Error("fixture replay did not parse any cross-aspect")
		}
	})
	t.Run("probe spawns then fails", func(t *testing.T) {
		path, argvLog := fakeBinary(t, t.TempDir(), failOnScript(t, "=v0", "exit 3"))
		out, err := ComputeCrossAspects(context.Background(), transitSpec(t))
		if err == nil {
			t.Fatal("a failed probe must fail the compute")
		}
		spawns := spawnArgvs(t, argvLog)
		if len(spawns) != 1 {
			t.Fatalf("%d processes started, want the probe alone", len(spawns))
		}
		if want := []string{bareCommand(path, spawns[0])}; !slices.Equal(out.Commands, want) {
			t.Errorf("Commands = %q, want the probe alone %q", out.Commands, want)
		}
	})
	t.Run("not a comparison", func(t *testing.T) {
		_, argvLog := fakeBinary(t, t.TempDir(), replayScript(t))
		out, err := ComputeCrossAspects(context.Background(), hellenisticSpec(t))
		if err == nil {
			t.Fatal("a single-chart spec must be refused")
		}
		if spawns := spawnArgvs(t, argvLog); len(spawns) != 0 {
			t.Errorf("refusal spawned %q", spawns)
		}
		if len(out.Commands) != 0 {
			t.Errorf("refusal recorded %q", out.Commands)
		}
	})
}

// C7: the raw report returns its command beside the text, on failure too.
func TestComputeRawReportReturnsCommand(t *testing.T) {
	t.Run("success pastes back", func(t *testing.T) {
		path, argvLog := fakeBinary(t, t.TempDir(), "echo report text")
		report, command, err := ComputeRawReport(context.Background(), hellenisticSpec(t), recipe.ChartRawReport())
		if err != nil {
			t.Fatalf("ComputeRawReport: %v", err)
		}
		if report != "report text\n" {
			t.Errorf("report = %q", report)
		}
		if !strings.HasPrefix(command, path+" ") {
			t.Errorf("command %q does not start with the resolved path %q", command, path)
		}
		// The raw-chart argv carries `=j0`, an unsafe element: the round
		// trip proves the quoting rather than a bare join.
		if out, err := exec.Command("sh", "-c", command).CombinedOutput(); err != nil {
			t.Fatalf("pasting the recorded command failed: %v\n%s", err, out)
		}
		spawns := spawnArgvs(t, argvLog)
		if len(spawns) != 2 || !slices.Equal(spawns[0], spawns[1]) {
			t.Errorf("pasted argv differs from executed argv: %q", spawns)
		}
	})
	t.Run("failure after spawn", func(t *testing.T) {
		path, argvLog := fakeBinary(t, t.TempDir(), "exit 3")
		_, command, err := ComputeRawReport(context.Background(), hellenisticSpec(t), recipe.ChartRawReport())
		if err == nil {
			t.Fatal("nonzero exit must fail")
		}
		spawns := spawnArgvs(t, argvLog)
		if len(spawns) != 1 || !strings.HasPrefix(command, path+" ") {
			t.Errorf("command = %q after %d spawns; want the started spawn's command", command, len(spawns))
		}
	})
	t.Run("no spawn", func(t *testing.T) {
		fakeBinary(t, t.TempDir(), "exit 0")
		binary = filepath.Join(t.TempDir(), "missing")
		_, command, err := ComputeRawReport(context.Background(), hellenisticSpec(t), recipe.ChartRawReport())
		if err == nil || command != "" {
			t.Errorf("command = %q, err = %v; want no command and an error", command, err)
		}
	})
}

// C8: the argv every in-scope spawn hands Astrolog is byte-for-byte what it
// was before recording existed — pinned as literals derived from the
// compiler golden (internal/invariants/testdata/astrolog-compiler-argv.golden)
// plus the runner's own `-k` / `-aa` suffixes.
func TestInScopeArgvUnchanged(t *testing.T) {
	prefix := []string{"-qj", "2446232.0000000005", "-zl", "11:34E", "48:08N", "-zi", "", "", "-c", "14", "_s", "_Ys", "_sr0", ":sz", "-qag", "10", "15", "1582", "12:00am", "0", "0:00E", "0:00N", "=Yt", "_Yd", "-Yn", "-R", "11", "-R", "17", "-R", "18", "-R", "25", "-R", "28"}
	positions := slices.Concat(prefix, []string{"=v0", "-Yw", "0", "-k"})
	aspects := slices.Concat(prefix, []string{"=v0", "-Yw", "0", "-aa", "-k"})
	// C8: a raw report also asks for the degree character (`-k0`, no colour).
	rawChart := slices.Concat(prefix, []string{"=j0", "-k0"})
	probe := []string{"-qj", "2446232.0000000005", "-zl", "11:34E", "48:08N", "-zi", "", "", "-c", "14", "_s", "_Ys", "_sr0", ":sz", "-qag", "10", "15", "1582", "12:00am", "0", "0:00E", "0:00N", "=Yt", "_Yd", "=v0", "-Yw", "0", "-k"}

	cases := []struct {
		name string
		call func(ctx context.Context) error
		want [][]string
	}{
		{"ComputeChart", func(ctx context.Context) error { _, err := ComputeChart(ctx, hellenisticSpec(t)); return err }, [][]string{positions, aspects}},
		{"ComputeChartPositionsOnly", func(ctx context.Context) error {
			_, err := ComputeChartPositionsOnly(ctx, hellenisticSpec(t))
			return err
		}, [][]string{positions}},
		{"ComputeRawReport", func(ctx context.Context) error {
			_, _, err := ComputeRawReport(ctx, hellenisticSpec(t), recipe.ChartRawReport())
			return err
		}, [][]string{rawChart}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, argvLog := fakeBinary(t, t.TempDir(), replayScript(t))
			if err := c.call(context.Background()); err != nil {
				t.Fatalf("%s: %v", c.name, err)
			}
			if got := spawnArgvs(t, argvLog); !slices.EqualFunc(got, c.want, slices.Equal) {
				t.Errorf("argv drifted:\n got %q\nwant %q", got, c.want)
			}
		})
	}
	t.Run("ComputeCrossAspects", func(t *testing.T) {
		_, argvLog := fakeBinary(t, t.TempDir(), replayScript(t))
		if _, err := ComputeCrossAspects(context.Background(), transitSpec(t)); err != nil {
			t.Fatalf("ComputeCrossAspects: %v", err)
		}
		got := spawnArgvs(t, argvLog)
		if len(got) != 2 || !slices.Equal(got[0], probe) {
			t.Fatalf("probe argv drifted:\n got %q\nwant %q", got, probe)
		}
		rel := got[1]
		if len(rel) != 21 || rel[0] != "-rt" || !strings.HasSuffix(rel[1], "/chart0.as") || !strings.HasSuffix(rel[2], "/chart1.as") || !slices.Equal(rel[3:], []string{"-c", "14", "_s", "_Ys", "_sr0", ":sz", "-qag", "10", "15", "1582", "12:00am", "0", "0:00E", "0:00N", "=Yt", "_Yd", "-aa", "-k"}) {
			t.Errorf("relationship argv drifted: %q", rel)
		}
	})
}

// C9: a binary path with whitespace is quoted like any other element, so the
// recorded string still pastes as one command.
func TestRecordedCommandQuotesBinaryPath(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "with space")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path, argvLog := fakeBinary(t, dir, "exit 0")
	_, command, err := invoke(context.Background(), singleChartArgv)
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	if want := "'" + path + "' -YQ 0 " + strings.Join(hookBandCommandWords(), " ") + " " + strings.Join(singleChartArgv, " "); command != want {
		t.Fatalf("command = %q, want %q", command, want)
	}
	if out, err := exec.Command("sh", "-c", command).CombinedOutput(); err != nil {
		t.Fatalf("pasting the recorded command failed: %v\n%s", err, out)
	}
	spawns := spawnArgvs(t, argvLog)
	if len(spawns) != 2 || !slices.Equal(spawns[0], singleChartArgv) || !slices.Equal(spawns[1], singleChartArgv) {
		t.Errorf("pasted argv differs from executed argv: %q", spawns)
	}
}

// Acceptance evidence: a real single-chart compute for a FIXED moment
// records two commands; each pasted into a shell reproduces Astrolog's stdout
// byte for byte — the positions spawn and the aspects spawn alike. Raw holds
// both captured stdouts joined by "\n---\n". Live binary, skipped without it.
func TestAcceptanceSingleChartCommandsReplay(t *testing.T) {
	if _, err := exec.LookPath("astrolog"); err != nil {
		t.Skip("astrolog not on PATH; skipping live-binary acceptance test")
	}
	out, err := ComputeChart(context.Background(), hellenisticSpec(t))
	if err != nil {
		t.Fatalf("ComputeChart: %v", err)
	}
	captured := strings.SplitN(out.Raw, "\n---\n", 2)
	if len(out.Commands) != 2 || len(captured) != 2 {
		t.Fatalf("want two commands and two captured stdouts, got %d and %d", len(out.Commands), len(captured))
	}
	for i, name := range []string{"positions", "aspects"} {
		pasted, err := exec.Command("sh", "-c", out.Commands[i]).CombinedOutput()
		if err != nil {
			t.Fatalf("%s: pasting %q failed: %v", name, out.Commands[i], err)
		}
		if string(pasted) != captured[i] {
			t.Errorf("%s: pasted stdout differs from the captured stdout\ncommand: %s", name, out.Commands[i])
		}
	}
}
