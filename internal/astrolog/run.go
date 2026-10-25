package astrolog

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/cruiserxyz/sextile/internal/astrologstyle"
	"github.com/cruiserxyz/sextile/internal/payload"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// binary is the astrolog executable looked up on PATH. Exposed as a package
// variable so a test could substitute a fake invocation, though the live-
// binary test invokes the real astrolog directly.
var binary = "astrolog"

// ErrNotInstalled means the astrolog binary is not on PATH. It is a SEPARATE
// cause from every other spawn failure because it is the likeliest one on a
// machine that has never run Sextile, and because it is the only one whose fix
// is "install astrolog" rather than anything about the chart.
//
// The perimeter recognises it through internal/lookup and says so in the
// operator's own words; nothing here formats a sentence for a user.
var ErrNotInstalled = errors.New("astrolog: the astrolog binary was not found on PATH")

// Available reports whether the astrolog binary can be resolved, without
// starting it. It is the startup probe's whole mechanism: PATH is consulted,
// no process runs, and a missing binary comes back as ErrNotInstalled rather
// than as an exec detail the perimeter would have to interpret.
func Available() error {
	if _, err := exec.LookPath(binary); err != nil {
		return ErrNotInstalled
	}
	return nil
}

// ErrUnsafeArgv means the CommandSpec.Argv violated the switch-toggle safety
// contract: the argv did not carry one of the chart-entry switches (-qa,
// -qb, -qc) or it contained the known-toxic naked -b.
//
// Provenance: bake-pipeline neuron astrolog-switch-toggle-semantics (removed in Phase 1)
// (safe_invocation_patterns, anti_patterns).
var ErrUnsafeArgv = errors.New("astrolog: CommandSpec.Argv violates the switch-toggle safety contract")

// Run executes astrolog against a compiled body call and returns the combined
// RawOutput. The astrolog invokes astrolog twice: once with CommandSpec.Argv as
// given (yielding the default body-position chart), and once with "-a"
// appended (yielding the aspect list). Both stdouts are parsed into typed
// records; the raw positions output is checked against the 0Ari00 forbid
// pattern first, and the call is abandoned with ErrEphemerisDisabled if
// that sanity predicate triggers.
//
// call is the body call the compiler emitted (argv and chooser band together);
// the seam refuses the spawn unless the argv carries the band (spawn.go).
//
// Two invocations are required because astrolog's chart-type switches are
// toggles rather than additive — passing -v and -a in one command selects
// only the last one passed. The cost is a second process start per Run;
// the astrolog accepts that in exchange for a single-call contract to the
// payload and a single-call sanity check against the positions chart.
func Run(ctx context.Context, call BodyCall) (payload.RawOutput, error) {
	if err := validateArgv(call.argv); err != nil {
		return payload.RawOutput{}, err
	}

	// Every return after a spawn started carries that spawn's command: a failed sanity check or parse is still a process that ran.
	var commands []string
	positionsCall, err := call.derive(colorPositionsArgv)
	if err != nil {
		return payload.RawOutput{}, err
	}
	var positionsMissing, aspectsMissing []string
	positionsCall = positionsCall.reportingMissing(&positionsMissing)
	positionsOut, command, err := spawn(ctx, positionsCall)
	commands = recordCommand(commands, command)
	if err != nil {
		return payload.RawOutput{Commands: commands}, fmt.Errorf("astrolog positions invocation: %w", err)
	}
	if err := checkPositionSanity(positionsOut); err != nil {
		return payload.RawOutput{Commands: commands}, err
	}
	styles, err := astrologstyle.ParseBodies(positionsOut)
	if err != nil {
		return payload.RawOutput{Commands: commands}, err
	}

	aspectsCall, err := call.derive(aspectsArgv)
	if err != nil {
		return payload.RawOutput{Commands: commands}, err
	}
	aspectsCall = aspectsCall.reportingMissing(&aspectsMissing)
	aspectsOut, command, err := spawn(ctx, aspectsCall)
	commands = recordCommand(commands, command)
	if err != nil {
		return payload.RawOutput{Commands: commands}, fmt.Errorf("astrolog aspects invocation: %w", err)
	}
	aspectRows, err := astrologstyle.ParseAspectRows(aspectsOut)
	if err != nil {
		return payload.RawOutput{Commands: commands}, err
	}

	return payload.RawOutput{
		Positions:     parsePositions(positionsOut),
		Unavailable:   unavailableObjects(positionsOut),
		BodyStyles:    mapBodyStyles(styles),
		Aspects:       parseAspects(aspectsOut),
		AspectStyles:  mapAspectStyles(aspectRows),
		Raw:           positionsOut + "\n---\n" + aspectsOut,
		AspectsReport: reportText(aspectsOut),
		Commands:      commands,
		Cusps:         parseCusps(positionsOut),
		HousesUsed:    parseHousesUsed(positionsOut),
		// Each file once, positions invocation first.
		MissingEphemeris: mergeFiles(positionsMissing, aspectsMissing),
	}, nil
}

// mergeFiles joins file-name lists in order, each name once; nil when empty.
func mergeFiles(lists ...[]string) []string {
	var out []string
	for _, l := range lists {
		for _, f := range l {
			if !slices.Contains(out, f) {
				out = append(out, f)
			}
		}
	}
	return out
}

// reportText turns a colored (-k) Astrolog listing into the plain report a
// panel can print: SGR stripped, and the CP437 degree byte (0xF8) Astrolog
// emits in -k mode replaced by U+00B0 so the text is valid UTF-8. The
// characters are otherwise exactly Astrolog's own.
func reportText(stdout string) string {
	return strings.ReplaceAll(astrologstyle.StripSGR(stdout), "\xf8", "°")
}

// aspectsArgv derives the aspect-list invocation from a positions argv: the
// same chart switches plus the -aa mode (Astrolog's own applying/separating
// markers, an earlier change) and color. It is the one place the second spawn's mode is
// chosen, so it can never fall back to a plain -a list without phase.
func aspectsArgv(positionsArgv []string) []string {
	return colorAspectsArgv(append(append([]string{}, positionsArgv...), aspectListMode))
}

// RunPositionsOnly executes astrolog ONCE for the positions chart only,
// skipping the second `-a` aspects invocation Run() makes. Returned
// RawOutput has Positions populated and Aspects empty.
//
// It served the t+ε applying probes (retired by an earlier change) and still backs the
// ephemeris-liveness probe in ComputeCrossAspects and the Probers seam.
// Halves the per-probe astrolog spawn cost — load-bearing for the
// an earlier change follow-up that drops time-step coalescing (held-key sweep
// becomes feasible only after the per-frame exec count is brought
// down per WR-6).
//
// Sanity check is preserved: positions output is still scanned for
// the 0Ari00 forbid pattern.
func RunPositionsOnly(ctx context.Context, call BodyCall) (payload.RawOutput, error) {
	if err := validateArgv(call.argv); err != nil {
		return payload.RawOutput{}, err
	}
	positionsCall, err := call.derive(colorPositionsArgv)
	if err != nil {
		return payload.RawOutput{}, err
	}
	var positionsMissing []string
	positionsCall = positionsCall.reportingMissing(&positionsMissing)
	positionsOut, command, err := spawn(ctx, positionsCall)
	commands := recordCommand(nil, command)
	if err != nil {
		return payload.RawOutput{Commands: commands}, fmt.Errorf("astrolog positions invocation: %w", err)
	}
	if err := checkPositionSanity(positionsOut); err != nil {
		return payload.RawOutput{Commands: commands}, err
	}
	styles, err := astrologstyle.ParseBodies(positionsOut)
	if err != nil {
		return payload.RawOutput{Commands: commands}, err
	}
	return payload.RawOutput{
		Positions:        parsePositions(positionsOut),
		BodyStyles:       mapBodyStyles(styles),
		Raw:              positionsOut,
		Commands:         commands,
		Cusps:            parseCusps(positionsOut),
		HousesUsed:       parseHousesUsed(positionsOut),
		MissingEphemeris: mergeFiles(positionsMissing),
	}, nil
}

func colorPositionsArgv(argv []string) []string {
	return colorArgv(argv)
}

func colorAspectsArgv(argv []string) []string {
	return colorArgv(argv)
}

func colorArgv(argv []string) []string {
	for _, arg := range argv {
		if arg == "-k" {
			return append([]string{}, argv...)
		}
	}
	return append(append([]string{}, argv...), "-k")
}

func mapBodyStyles(rows astrologstyle.Rows) astrologstyle.Bodies {
	out := make(astrologstyle.Bodies)
	for abbrev, style := range rows {
		if body, ok := bodyAbbrev4[abbrev]; ok {
			out[body] = style
		}
	}
	return out
}

func mapAspectStyles(rows []astrologstyle.AspectRow) astrologstyle.Aspects {
	out := make(astrologstyle.Aspects)
	for _, row := range rows {
		first, okFirst := bodyFullName[row.FirstName]
		second, okSecond := bodyFullName[row.SecondName]
		kind, okKind := aspectAbbrev[row.KindName]
		if !okFirst || !okSecond || !okKind {
			continue
		}
		typed := aspect.NewAspect(first, second, kind, 0)
		style := row.Style
		if typed.A != first {
			style.First, style.Second = style.Second, style.First
		}
		out[astrologstyle.AspectIdentity{First: typed.A, Second: typed.B, Kind: kind}] = style
	}
	return out
}

func mapCrossAspectStyles(rows []astrologstyle.AspectRow) astrologstyle.Aspects {
	out := make(astrologstyle.Aspects)
	for _, row := range rows {
		primary, okPrimary := resolveCrossBody(row.FirstName)
		secondary, okSecondary := resolveCrossBody(row.SecondName)
		kind, okKind := aspectAbbrev[row.KindName]
		if !okPrimary || !okSecondary || !okKind {
			continue
		}
		out[astrologstyle.AspectIdentity{First: primary, Second: secondary, Kind: kind}] = row.Style
	}
	return out
}

// invokeError wraps a spawn failure. A canceled or expired context SIGKILLs
// the subprocess (the contract of exec.CommandContext). Surface the context
// error — not the generic "signal: killed" exec error — so a superseded run is
// cleanly recognizable via errors.Is(err, context.Canceled) and the refresh
// coordinator can drop it silently instead of logging a real astrolog
// failure. This is the cancellation contract the async-refresh work (S1,
// refresh-coordinator plan) builds on.
func invokeError(ctx context.Context, err error) error {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return fmt.Errorf("astrolog: canceled: %w", ctxErr)
	}
	// A binary that is not there is not an exec detail — it is the one spawn
	// failure the operator can act on directly, so it keeps its own identity
	// all the way to the perimeter instead of arriving as wrapped exec text.
	if errors.Is(err, exec.ErrNotFound) {
		return fmt.Errorf("astrolog exec: %w", ErrNotInstalled)
	}
	return fmt.Errorf("astrolog exec: %w", err)
}

// validateArgv enforces the switch-toggle safety contract. Refuses argv
// that lacks a chart-entry switch (-qa/-qb/-qc, or the file-based -r0
// comparison entry) or contains a naked -b. Every other switch astrolog
// exposes is left to the compiler; the runner is not a general argv linter.
//
// The one file-reading exception is the multi-wheel: an -i/-iN file switch is
// refused unless the argv's chart entry is a complete -r3..-r6 ring set, and a
// ring set is refused when the argv carries any other chart entry (-q*, -r0,
// -rt) or a second -rN root. A file switch beside another chart entry would
// read a file the caller never staged.
func validateArgv(argv []string) error {
	argv = withoutLabelOperands(argv)
	var hasChart, hasFileSwitch, completeRing bool
	var otherEntries, roots int
	for _, a := range argv {
		if n, ok := multiRingRoot(a); ok {
			roots++
			if ringFilesComplete(argv, n) {
				// Multi-wheel entry (-r3..-r6): the charts live in N staged chart
				// files, one behind each of -i, -i2 … -iN, so the root counts as a
				// chart entry only when all N are present.
				hasChart, completeRing = true, true
			}
		}
		if _, ok := fileSwitch(a); ok {
			hasFileSwitch = true
		}
		if a == "-b" {
			return fmt.Errorf("%w: contains naked -b (would disable Swiss Ephemeris)", ErrUnsafeArgv)
		}
		if strings.HasPrefix(a, "-q") && len(a) == 3 {
			// -qa, -qb, -qc, -qd, -qj, -qL, -ql — any chart-entry switch
			// satisfies the baseline requirement.
			hasChart = true
			otherEntries++
		}
		if a == "-r0" || a == "-rt" {
			// File-based comparison entry: the chart data lives in the two
			// chart file arguments (each carries its own -qb), so the argv's
			// chart-entry marker is the -r0/-rt switch itself (-rt: the
			// second chart transits the first, an earlier change).
			hasChart = true
			otherEntries++
		}
	}
	if !hasChart {
		return fmt.Errorf("%w: missing chart-entry switch (expected -qa/-qb/-qc, -r0, -rt or a complete -r3..-r6 ring set)", ErrUnsafeArgv)
	}
	if hasFileSwitch && !completeRing {
		return fmt.Errorf("%w: -i file switch outside a complete -r3..-r6 ring set", ErrUnsafeArgv)
	}
	if completeRing && (otherEntries > 0 || roots > 1) {
		return fmt.Errorf("%w: a -r3..-r6 ring set must be the argv's only chart entry", ErrUnsafeArgv)
	}
	return nil
}

// labelOperands are the switches whose operands are free text the operator
// typed — `-zi <title> <place>` (savechart.go) — and so are never switches,
// whatever they spell: a chart titled "-i" or "-b" is a title.
var labelOperands = map[string]int{"-zi": 2}

// withoutLabelOperands drops the free-text operands of labelOperands, so the
// scan below sees only switches and their machine-made operands.
func withoutLabelOperands(argv []string) []string {
	out := make([]string, 0, len(argv))
	for i := 0; i < len(argv); i++ {
		out = append(out, argv[i])
		i += labelOperands[argv[i]]
	}
	return out
}

// multiRingRoot reports whether tok is a multi-wheel root -r3..-r6 and its N.
func multiRingRoot(tok string) (int, bool) {
	if len(tok) == 3 && tok[0] == '-' && tok[1] == 'r' && tok[2] >= '3' && tok[2] <= '6' {
		return int(tok[2] - '0'), true
	}
	return 0, false
}

// fileSwitch reports whether tok is a chart-file switch (-i, -i2 … -i9) and
// its 1-based position.
func fileSwitch(tok string) (int, bool) {
	switch {
	case tok == "-i":
		return 1, true
	case len(tok) == 3 && tok[:2] == "-i" && tok[2] >= '2' && tok[2] <= '9':
		return int(tok[2] - '0'), true
	}
	return 0, false
}

// ringFilesComplete reports whether argv carries exactly the n file switches
// -i, -i2 … -in, each once and each followed by an argument, and no other -i*
// file switch. Anything else file-based stays refused.
func ringFilesComplete(argv []string, n int) bool {
	seen := make(map[int]int)
	for i, a := range argv {
		k, ok := fileSwitch(a)
		if !ok {
			continue
		}
		// A staged path is never a switch: a following token that starts with
		// "-" means the file argument is missing.
		if i+1 >= len(argv) || strings.HasPrefix(argv[i+1], "-") {
			return false
		}
		seen[k]++
	}
	if len(seen) != n {
		return false
	}
	for k := 1; k <= n; k++ {
		if seen[k] != 1 {
			return false
		}
	}
	return true
}

// StagingError is a staged chart file that could not be written. Ring is the
// 1-based position of the chart in the render's own order (a multi-wheel's
// ring, a comparison's chart 1 or 2), so a caller that knows which selected
// slot sat at that ring can say which one failed. The text is the one every
// staging failure has always had.
type StagingError struct {
	Ring int
	Err  error
}

func (e *StagingError) Error() string { return fmt.Sprintf("astrolog: stage inputs: %v", e.Err) }

func (e *StagingError) Unwrap() error { return e.Err }

// StagedRing is Ring, as the one-method contract lookup.StagedRing reads, so a
// caller above astrolog can tell which ring failed without importing it.
func (e *StagingError) StagedRing() int { return e.Ring }

// writeStagedFile writes one staged chart file. It is a package seam so a test
// can fail the k-th write and prove the staging dir is removed.
var writeStagedFile = os.WriteFile

// stageCall materializes an Invocation's TempInputs and returns the runnable
// body call. With no TempInputs it is a no-op (the single-chart path): the
// invocation's own call, an empty dir, and a no-op cleanup. With TempInputs it
// writes each Content into a fresh temp dir and substitutes the staged path
// into the call at the given ArgIndex — the image path places its -Xo sink in
// the same dir so one RemoveAll cleans every temp artifact. cleanup is always
// safe to call (defer it immediately); ctx-cancellation SIGKILLs the subprocess
// but the deferred cleanup still runs in the Go caller, so temp files don't leak.
func stageCall(inv Invocation) (call BodyCall, dir string, cleanup func(), err error) {
	noop := func() {}
	call = inv.Call()
	if len(inv.TempInputs) == 0 {
		return call, "", noop, nil
	}
	d, err := os.MkdirTemp("", "sextile-r0-*")
	if err != nil {
		return BodyCall{}, "", noop, fmt.Errorf("astrolog: stage inputs: %w", err)
	}
	cleanup = func() { _ = os.RemoveAll(d) }
	for i, ti := range inv.TempInputs {
		if ti.ArgIndex < 0 || ti.ArgIndex >= len(inv.Argv) {
			cleanup()
			return BodyCall{}, "", noop, fmt.Errorf("astrolog: stage inputs: ArgIndex %d out of range (argv len %d)", ti.ArgIndex, len(inv.Argv))
		}
		p := filepath.Join(d, fmt.Sprintf("chart%d.as", i))
		if err := writeStagedFile(p, []byte(ti.Content), 0o600); err != nil {
			cleanup()
			return BodyCall{}, "", noop, &StagingError{Ring: i + 1, Err: err}
		}
		if call, err = call.substitute(ti.ArgIndex, p); err != nil {
			cleanup()
			return BodyCall{}, "", noop, fmt.Errorf("astrolog: stage inputs: %w", err)
		}
	}
	return call, d, cleanup, nil
}
