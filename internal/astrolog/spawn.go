package astrolog

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"slices"
	"strings"
	"syscall"
	"time"
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
// does neither fails to compile, or fails internal/invariants
// TestAstrologSpawnSeamIsTyped by name (the specification).

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
// one (newBodyCall, called from Invocation.Call and TimelineCall — pinned by
// TestAstrologSpawnSeamIsTyped). The zero value carries no issued band and is
// refused by the seam.
type BodyCall struct {
	argv []string
	band Band
	// missing, when set, receives the file names of the expected
	// missing-ephemeris stderr lines of the spawn (reportingMissing). It is
	// not part of the call's identity: derive and with do not carry it.
	missing *[]string
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

// Argv is a copy of the call's argv, read-only.
func (c BodyCall) Argv() []string { return append([]string(nil), c.argv...) }

// ErrSmuggledRestriction means a caller tried to add an object restriction
// (-R…) or node-mode (-Y…) token to a body call after its band was issued.
var ErrSmuggledRestriction = errors.New("astrolog: a body call may not gain a -R or -Y token after its band was issued")

// with returns the call with extra tokens appended (colour, mode, an image
// sink). A token that is a restriction or node-mode switch is refused: those
// belong to the band, which only the compiler issues.
func (c BodyCall) with(extra ...string) (BodyCall, error) {
	for _, tok := range extra {
		if strings.HasPrefix(tok, "-R") || strings.HasPrefix(tok, "-Y") {
			return BodyCall{}, fmt.Errorf("%w: %q", ErrSmuggledRestriction, tok)
		}
	}
	return BodyCall{argv: append(append([]string(nil), c.argv...), extra...), band: c.band}, nil
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
	if strings.HasPrefix(value, "-R") || strings.HasPrefix(value, "-Y") {
		return BodyCall{}, fmt.Errorf("%w: %q", ErrSmuggledRestriction, value)
	}
	argv := append([]string(nil), c.argv...)
	argv[i] = value
	return BodyCall{argv: argv, band: c.band}, nil
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
	if exempt {
		return out, command, err
	}
	if err != nil && ctx.Err() != nil {
		// A cancelled call is a superseded one, whatever it wrote first.
		return "", command, err
	}
	if len(warningLines(stderr.String())) > 0 {
		// The refusal carries stderr and the command, never the chart: the
		// stdout of a call Astrolog warned on is not parsed and not returned.
		warned := &WarnedError{Stderr: stderr.String()}
		if err != nil {
			return "", command, fmt.Errorf("%w; %w", warned, err)
		}
		return "", command, warned
	}
	// The expected missing-ephemeris lines were filtered above; a caller that
	// passed somewhere to put them gets their file names (the body popup's
	// probe reads them from the result, the specification C2).
	if bc, ok := c.(BodyCall); ok && bc.missing != nil {
		*bc.missing = missingEphemerisFiles(stderr.String())
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
	// Never page: a user's astrolog.as may set -YQ 24, and a pager waiting for
	// a keypress on a pipe loops until the process crashes. -YQ 0 comes first,
	// then the hook-clear band (hooks.go), so the call's own argv is untouched;
	// a fresh slice keeps the call's backing array from being written through.
	argv = append(append([]string{"-YQ", "0"}, HookClearBand()...), argv...)
	cmd := exec.CommandContext(ctx, binary, argv...)
	cmd.Dir = dir
	if limited {
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
		cmd.WaitDelay = time.Second
	}
	return cmd, argv, nil
}
