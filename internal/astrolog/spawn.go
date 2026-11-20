package astrolog

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// This file is the only place Sextile starts the astrolog binary. The process
// start function, spawn, accepts a call value of exactly two types, and each
// answers one question before a process may exist:
//
//   - BodyCall: this call can print bodies, so it carries the chooser band the
//     compiler issued for it, and the seam refuses it unless the argv really
//     contains that band.
//   - ExemptCall: this call shows no bodies, and says why in a reason string.
//
// There is no entry point that takes a bare []string. A new call site that
// does neither fails to compile.

// ErrBandMissing means a body call reached the seam without carrying its
// chooser band: either the band was never issued by the compiler, or the argv
// does not contain the band's tokens as one contiguous run.
var ErrBandMissing = errors.New("astrolog: body call does not carry its chooser band")

// Band is the object/aspect/node-mode portion of an argv that makes a body call
// honour the operator's choosers. Its fields are unexported and only the
// compiler's band functions set them, so a Band cannot be written by hand
// outside this package.
//
// issued is the marker that distinguishes a legitimately tokenless band (a
// default policy emits nothing) from the zero value: without it, "the argv
// contains these zero tokens" would be vacuously true for a call that never
// asked the compiler anything.
type Band struct {
	tokens []string
	issued bool
}

// newBand issues a band over the given token runs, in order. It is called only
// by the compiler's band functions.
func newBand(parts ...[]string) Band {
	var tokens []string
	for _, p := range parts {
		tokens = append(tokens, p...)
	}
	return Band{tokens: tokens, issued: true}
}

// spawnCall is what spawn accepts. Its method is unexported, so only the two
// types below can satisfy it.
type spawnCall interface{ spawnable() }

// BodyCall is a spawn whose output can name bodies and aspects. Its fields are
// unexported, so no code outside this package can pair an argv with a band, and
// inside it only the functions that EMIT the argv and its band together make
// one (newBodyCall, called from Invocation.Call, TimelineCall and VoidCall). The zero value carries no issued band and is
// refused by the seam.
type BodyCall struct {
	argv []string
	band Band
	// missing, when set, receives the file names of the expected
	// missing-ephemeris stderr lines of the spawn (reportingMissing). It is
	// not part of the call's identity: derive and with do not carry it.
	missing *[]string
	// saying, when set, receives the first line the spawn wrote on stderr
	// (firstSaidLine). Like missing it is not part of the call's identity.
	saying *string
	// probing marks a call that asks about an object to learn whether it can be
	// drawn: every stderr line is evidence for it, none a refusal. Only the
	// probes set it (RunPositionsProbing, probeCast); reportingMissing alone
	// keeps refusing every line that is not a missing-ephemeris line.
	probing bool
	// leftOut, when set, receives the names of the objects the spawn left out
	// because their ephemeris does not reach the moment (outOfRange). Like
	// missing it is not part of the call's identity.
	leftOut *[]string
	// leftOutLines, when set, receives the warning lines that said so (Astrolog's
	// own words, for the large log).
	leftOutLines *[]string
	// heard, when set on a probing call, receives every stderr line the spawn
	// wrote (searchLacking reads which object a warning is about from it).
	heard *[]string
	// searching marks an event search (a timeline, the void search): when every
	// object it draws is one Astrolog cannot place, the answer is an empty list.
	searching bool
	// Registry is the binary's object list and Objects the numbers the argv
	// draws, ascending: what the parsers need to give each printed row
	// Astrolog's own object. Neither is part of the argv or the band; a
	// call built without them parses its rows without identity.
	Registry aspect.Registry
	Objects  []int
	// Aspects is the binary's aspect list: what the parsers need to give each
	// printed aspect row Astrolog's own identity. A row it does not hold
	// refuses the list. Not part of the argv or the band.
	Aspects aspect.AspectRegistry
}

func (BodyCall) spawnable() {}

// newBodyCall pairs an argv with the band emitted for it. It copies argv.
func newBodyCall(argv []string, band Band) BodyCall {
	return BodyCall{argv: append([]string(nil), argv...), band: band}
}

// reportingMissing returns the call with a collector for the missing-ephemeris
// file names its spawn sees (the specification C2).
func (c BodyCall) reportingMissing(into *[]string) BodyCall {
	c.missing = into
	return c
}

// sayingFirst returns the call with a collector for the first line its spawn
// wrote on stderr, which only a probing call keeps.
func (c BodyCall) sayingFirst(into *string) BodyCall {
	c.saying = into
	return c
}

// reportingLeftOut returns the call with a collector for the objects its spawn
// leaves out because their ephemeris does not reach the moment.
func (c BodyCall) reportingLeftOut(into *[]string) BodyCall {
	c.leftOut = into
	return c
}

// reportingLeftOutLines is reportingLeftOut for Astrolog's own warning lines.
func (c BodyCall) reportingLeftOutLines(into *[]string) BodyCall {
	c.leftOutLines = into
	return c
}

// asSearch marks the call as an event search (see searching).
func (c BodyCall) asSearch() BodyCall {
	c.searching = true
	return c
}

// without is the call with the objects nums taken out of what it draws, in
// the argv and in the band alike. A band that lists its objects (`-R0 1 2
// ...`, and the transiting `-RT0`) loses them from the list. A band that
// toggles Astrolog's default set (`-R 11` turns Chiron on or off) loses the
// toggle that turned one on, or gains one that turns a default one off. Only
// spawn and Run use it, for objects Astrolog cannot place at the moment.
func (c BodyCall) without(nums []int) BodyCall {
	listed := slices.Contains(c.argv, flagMainShow)
	drop := func(toks []string) []string {
		var out []string
		inList := false
		toggled := map[int]bool{}
		for i := 0; i < len(toks); i++ {
			t := toks[i]
			if t == flagMainShow || t == flagTransitShow {
				inList = true
				out = append(out, t)
				continue
			}
			n, err := strconv.Atoi(t)
			if inList && err == nil {
				if slices.Contains(nums, n) {
					continue
				}
				out = append(out, t)
				continue
			}
			inList = false
			if !listed && t == "-R" && i+1 < len(toks) {
				if n, err := strconv.Atoi(toks[i+1]); err == nil && slices.Contains(nums, n) {
					toggled[n] = true
					i++
					continue
				}
			}
			out = append(out, t)
		}
		if !listed {
			for _, n := range nums {
				if !toggled[n] {
					out = append(out, "-R", strconv.Itoa(n))
				}
			}
		}
		return out
	}
	objects := slices.DeleteFunc(append([]int(nil), c.Objects...), func(n int) bool { return slices.Contains(nums, n) })
	return BodyCall{argv: drop(c.argv), band: Band{tokens: drop(c.band.tokens), issued: c.band.issued},
		missing: c.missing, saying: c.saying, probing: c.probing, leftOut: c.leftOut, leftOutLines: c.leftOutLines, searching: c.searching, Registry: c.Registry, Aspects: c.Aspects, Objects: objects}
}

// probingCall returns the call marked as a probe: no stderr line refuses it.
func (c BodyCall) probingCall() BodyCall {
	c.probing = true
	return c
}

// Argv is a copy of the call's argv, read-only.
func (c BodyCall) Argv() []string { return append([]string(nil), c.argv...) }

// ErrSmuggledRestriction means a caller tried to add an object restriction
// (-R…) or node-mode (-Y…) token to a body call after its band was issued.
var ErrSmuggledRestriction = errors.New("astrolog: a body call may not gain a -R or -Y token after its band was issued")

// bandSwitch says whether a token is a restriction (`R…`) or `Y…` switch in any
// of Astrolog's three forms: `-` toggles, `=` sets on, `_` sets off. Those
// belong to the band, which only the compiler issues; the node mode is `=Yn`
// or `_Yn` there.
func bandSwitch(tok string) bool {
	if len(tok) < 2 || !strings.ContainsRune("-=_", rune(tok[0])) {
		return false
	}
	return tok[1] == 'R' || tok[1] == 'Y'
}

// with returns the call with extra tokens appended (colour, mode, an image
// sink). A token that is a restriction or node-mode switch is refused: those
// belong to the band, which only the compiler issues.
func (c BodyCall) with(extra ...string) (BodyCall, error) {
	for _, tok := range extra {
		if bandSwitch(tok) {
			return BodyCall{}, fmt.Errorf("%w: %q", ErrSmuggledRestriction, tok)
		}
	}
	return BodyCall{argv: append(append([]string(nil), c.argv...), extra...), band: c.band, Registry: c.Registry, Aspects: c.Aspects, Objects: c.Objects}, nil
}

// derive applies a pure argv derivation (colour, an `-aa` list mode) and keeps
// only a result that begins with the original argv and adds no -R or -Y token.
func (c BodyCall) derive(f func([]string) []string) (BodyCall, error) {
	out := f(append([]string(nil), c.argv...))
	if len(out) < len(c.argv) || !slices.Equal(out[:len(c.argv)], c.argv) {
		return BodyCall{}, fmt.Errorf("astrolog: derived argv does not extend the call's argv")
	}
	return c.with(out[len(c.argv):]...)
}

// substitute fills one placeholder token (a staged chart path). It refuses a
// value that is a restriction or node-mode switch.
func (c BodyCall) substitute(i int, value string) (BodyCall, error) {
	if i < 0 || i >= len(c.argv) {
		return BodyCall{}, fmt.Errorf("astrolog: substitute index %d out of range (argv len %d)", i, len(c.argv))
	}
	if bandSwitch(value) {
		return BodyCall{}, fmt.Errorf("%w: %q", ErrSmuggledRestriction, value)
	}
	argv := append([]string(nil), c.argv...)
	argv[i] = value
	return BodyCall{argv: argv, band: c.band, Registry: c.Registry, Aspects: c.Aspects, Objects: c.Objects}, nil
}

// ExemptCall is a spawn that shows no bodies: it reads a metadata list or a
// chart record. The reason is mandatory and says why no band applies.
type ExemptCall struct {
	argv    []string
	reason  string
	dir     string
	limited bool // see bounded
}

func (ExemptCall) spawnable() {}

// newExempt builds an exempt call. The reason is a string literal at every
// call site; the invariant pins that and the set of call sites.
func newExempt(reason string, argv ...string) ExemptCall {
	return ExemptCall{argv: argv, reason: reason}
}

// bounded makes the call killable as a unit: spawn puts the process in its own
// group, kills the whole group when the context ends, and stops waiting on the
// output pipe one second after that. Only a call whose caller has a deadline and
// whose answer is cheap to lose opts in (the version read); every other call's
// exec.Cmd is left exactly as exec.CommandContext made it, because a wrapper
// whose child holds the pipe a moment longer is a successful run there.
func (c ExemptCall) bounded() ExemptCall {
	c.limited = true
	return c
}

// inDir runs the exempt call in dir. Only the chart writer passes one: astrolog
// reads astrolog.as from its working directory, so a save that inherited the
// operator's directory would write a record that depends on where Sextile was
// started.
func (c ExemptCall) inDir(dir string) ExemptCall {
	c.dir = dir
	return c
}

// checkBand is the seam's refusal of a body call that does not carry its band.
func checkBand(c BodyCall) error {
	if !c.band.issued {
		return fmt.Errorf("%w: the band was not issued by the compiler", ErrBandMissing)
	}
	if !containsRun(c.argv, c.band.tokens) {
		return fmt.Errorf("%w: argv lacks the band %q as one contiguous run", ErrBandMissing, strings.Join(c.band.tokens, " "))
	}
	return nil
}

// containsRun reports whether run occurs in argv as consecutive elements. An
// empty run is contained everywhere, which is why Band carries the issued
// marker.
func containsRun(argv, run []string) bool {
	if len(run) == 0 {
		return true
	}
	for i := 0; i+len(run) <= len(argv); i++ {
		match := true
		for j, tok := range run {
			if argv[i+j] != tok {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

// spawn runs the astrolog binary for one classified call and returns its
// output together with the command it ran. An ExemptCall's output is the
// combined stdout/stderr (Astrolog occasionally prints usage errors to stdout
// regardless of exit code, so folding keeps a metadata answer self-contained).
// A BodyCall's output is stdout alone, and any stderr line beyond the wheel's
// two progress lines refuses the call with ErrAstrologWarned.
//
// The command is the single choke point of an earlier change: it is built from the path
// exec resolved (cmd.Path) and the very argv slice handed to
// exec.CommandContext, so the recorded text cannot drift from what ran (C1).
// It is returned on every exit after the process started — a nonzero exit
// included — and is empty only when the process never started.
//
// A BodyCall that carries a missing-ephemeris collector (reportingMissing)
// receives the file names of the expected missing-ephemeris stderr lines,
// which are not warnings (hooks.go). An exempt call, a cancelled one and a
// refused one leave it untouched.
func spawn(ctx context.Context, c spawnCall) (out string, command string, err error) {
	cmd, argv, err := buildCmd(ctx, c)
	if err != nil {
		return "", "", err
	}
	if cmd.Dir == "" {
		// The root folder: it exists everywhere, Sextile writes nothing there,
		// and it is never the charts folder whatever TMPDIR or the starting
		// folder say. Docker runs Sextile there already.
		cmd.Dir = string(filepath.Separator)
	}
	// An ExemptCall folds the two streams into one pipe, as it always did: a
	// metadata answer keeps its warnings in its text. A BodyCall keeps them
	// apart: its stdout is a chart, and a stderr line means the chart cannot be
	// trusted (the specification C7). Both writers are drained by
	// exec's own goroutines, so neither pipe can fill and block the process.
	_, exempt := c.(ExemptCall)
	var stdout, stderr bytes.Buffer
	if exempt {
		cmd.Stdout, cmd.Stderr = &stdout, &stdout
	} else {
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
	}
	if err := cmd.Start(); err != nil {
		return "", "", invokeError(ctx, err)
	}
	command = formatCommand(cmd.Path, argv)
	err = cmd.Wait()
	if cmd.SysProcAttr != nil && cmd.SysProcAttr.Setpgid {
		// A bounded call returns only after SIGKILL has been SENT to its whole
		// process group, on timeout and on normal exit alike: a wrapper that
		// exits first leaves its children behind. Sending is not waiting; the
		// kill is not awaited here. ESRCH (nothing left) is the usual answer.
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	out = stdout.String()
	if err != nil {
		err = invokeError(ctx, err)
	}
	if errors.Is(err, ErrAstrologCrashed) {
		// A process that died on a signal printed an unfinished chart at best:
		// none of its stdout is returned or parsed.
		return "", command, err
	}
	if exempt {
		return out, command, err
	}
	if err != nil && ctx.Err() != nil {
		// A cancelled call is a superseded one, whatever it wrote first.
		return "", command, err
	}
	collecting, probing := false, false
	if bc, ok := c.(BodyCall); ok {
		collecting, probing = bc.missing != nil, bc.probing
	}
	warnings := warningLinesFor(stderr.String(), collecting)
	if probing {
		// A probing call takes every stderr line as evidence: the object asked
		// for is absent, with a file name only where a line carries one.
		warnings = nil
		if bc, ok := c.(BodyCall); ok && bc.heard != nil {
			*bc.heard = warningLines(stderr.String())
		}
	}
	if bc, ok := c.(BodyCall); ok && len(warnings) > 0 {
		nums, names, ok := unplaceable(warnings, out, bc, nil)
		if !ok {
			// A line that names no object the output can match: ask which of the
			// call's objects lack data, by halving what it draws.
			nums, names, ok = unplaceable(warnings, out, bc, searchLacking(ctx, bc, warnings))
		}
		if ok {
			// Astrolog cannot place these objects at this moment: their
			// ephemeris does not reach it, or its file for this era is not in
			// the data folder. It prints them at 0 Aries and finds aspects to
			// them (NEXT.md BUG 11), so the chart is cast again without them
			// and the caller is told which were left out.
			if bc.leftOut != nil {
				*bc.leftOut = append(*bc.leftOut, names...)
			}
			if bc.leftOutLines != nil {
				*bc.leftOutLines = append(*bc.leftOutLines, warnings...)
			}
			if len(nums) == len(bc.Objects) {
				// An event search with no object left has no events to list.
				return "", command, nil
			}
			return spawn(ctx, bc.without(nums))
		}
	}
	if len(warnings) > 0 {
		// The refusal carries stderr and the command, never the chart: the
		// stdout of a call Astrolog warned on is not parsed and not returned.
		warned := &WarnedError{Stderr: stderr.String()}
		if err != nil {
			return "", command, fmt.Errorf("%w — %w", warned, err)
		}
		return "", command, warned
	}
	// The expected missing-ephemeris lines were filtered above; a caller that
	// passed somewhere to put them gets their file names (the body popup's
	// probe reads them from the result, the specification C2).
	if bc, ok := c.(BodyCall); ok && bc.missing != nil {
		*bc.missing = missingEphemerisFiles(stderr.String())
	}
	if bc, ok := c.(BodyCall); ok && bc.saying != nil {
		*bc.saying = firstSaidLine(stderr.String())
	}
	return out, command, err
}

// buildCmd classifies the call and builds its exec.Cmd, unstarted. Only a
// bounded ExemptCall gets a process group, a group kill and a WaitDelay.
func buildCmd(ctx context.Context, c spawnCall) (*exec.Cmd, []string, error) {
	var argv []string
	var dir string
	var limited bool
	switch call := c.(type) {
	case BodyCall:
		if err := checkBand(call); err != nil {
			return nil, nil, err
		}
		argv = call.argv
	case ExemptCall:
		if strings.TrimSpace(call.reason) == "" {
			return nil, nil, fmt.Errorf("astrolog: exempt call has no reason")
		}
		argv, dir, limited = call.argv, call.dir, call.limited
	default:
		return nil, nil, fmt.Errorf("astrolog: spawn: unclassified call %T", c)
	}
	// The call's own argv follows CallHead, untouched; a fresh slice keeps the
	// call's backing array from being written through.
	argv = append(CallHead(), argv...)
	cmd := exec.CommandContext(ctx, binary, argv...)
	// Astrolog reads an astrolog.as from its working directory (io.cpp, after
	// its own folder), so it never runs where Sextile was started: a charts
	// folder holding an astrolog.as would otherwise be read as settings, `-o`
	// lines and all. A call that names no folder gets a fresh, empty one.
	// spawn makes and removes that folder (buildCmd leaves Dir empty here).
	cmd.Dir = dir
	if limited {
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
		cmd.WaitDelay = time.Second
	}
	return cmd, argv, nil
}

// rangeWarning is the line Astrolog writes for an object whose ephemeris does
// not reach the chart's moment (calc.cpp): "Chiron's ephemeris is restricted
// to JD 1967601.5 - JD 3419437.5".
var rangeWarning = regexp.MustCompile(`^(.+)'s ephemeris is restricted to JD -?[0-9.]+ - JD -?[0-9.]+$`)

// apsidesWarning is the line Astrolog writes when the natural Lilith (the
// interpolated apogee) is asked for outside the range its ephemeris covers
// (sweph.cpp, swe_calc for SE_INTP_APOG). It names no object, and the row the
// chart prints for it is the no-data row, as for a missing file.
var apsidesWarning = regexp.MustCompile(`^Interpolated apsides are restricted to JD +-?[0-9.]+ - JD +-?[0-9.]+$`)

// asteroidWarning is the line Astrolog writes for an asteroid whose file does
// not reach the moment: "asteroid No. 10 (se00010.se1): jd 1684536.1 < lower
// limit 2268932.5;". It names the minor planet's number, not an object of the
// registry, so it is read like a missing file's line.
var asteroidWarning = regexp.MustCompile(`^asteroid No\. \d+ \([^)]*\): jd -?[0-9.]+ [<>] (?:lower|upper) limit -?[0-9.]+[\x3b]?$`)

// namesNoObject is whether a warning is one of the no-data lines that name no
// object of the registry: a missing file, the natural Lilith's range, an asteroid's.
func namesNoObject(w string) bool {
	return missingEphemeris.MatchString(w) || apsidesWarning.MatchString(w) || asteroidWarning.MatchString(w)
}

// unplaceable is the objects Astrolog could not place at the chart's moment,
// when that is all its warnings say: each warning is a range line or a
// missing-ephemeris line, and every object they concern is one the call draws.
// A range line names its object; a missing file and the natural Lilith's range
// line do not, so those objects are the ones stdout prints as the no-data row
// (0Ari00, no speed), or, for an event search that prints no such rows, the
// ones searchLacking found out. ok is false when any warning is something else,
// when a missing file is named and neither shows an object that lacks it, or,
// outside an event search, when no object would be left.
func unplaceable(warnings []string, stdout string, c BodyCall, lacking []int) (nums []int, names []string, ok bool) {
	add := func(o aspect.Object) bool {
		if !slices.Contains(c.Objects, o.Number) {
			return false
		}
		if !slices.Contains(nums, o.Number) {
			nums, names = append(nums, o.Number), append(names, o.Name)
		}
		return true
	}
	missingFile := false
	for _, w := range warnings {
		if namesNoObject(w) {
			missingFile = true
			continue
		}
		m := rangeWarning.FindStringSubmatch(w)
		if m == nil {
			return nil, nil, false
		}
		o, known := c.Registry.ByName(m[1])
		if !known || !add(o) {
			return nil, nil, false
		}
	}
	if missingFile {
		found := false
		// An event search prints no rows that name the object: searchLacking asked.
		for _, n := range lacking {
			if o, known := c.Registry.ByNumber(n); known && add(o) {
				found = true
			}
		}
		for _, cut := range unavailableObjects(stdout) {
			// A row names its object by the first four letters of its name:
			// it is the one object the call draws whose name begins so.
			var match []aspect.Object
			for _, n := range c.Objects {
				if o, ok := c.Registry.ByNumber(n); ok && strings.HasPrefix(o.Name, cut) {
					match = append(match, o)
				}
			}
			if len(match) != 1 || !add(match[0]) {
				return nil, nil, false
			}
			found = true
		}
		if !found {
			return nil, nil, false
		}
	}
	return nums, names, len(nums) > 0 && (len(nums) < len(c.Objects) || c.searching)
}

// NoDataBody is the body a warning line of Astrolog says it cannot place at
// the chart's moment (rangeWarning), or "" for any other line.
func NoDataBody(line string) string {
	if m := rangeWarning.FindStringSubmatch(line); m != nil {
		return m[1]
	}
	return ""
}
