package astrolog

import (
	"errors"
	"fmt"
	"hash/fnv"
	"slices"
	"sort"
	"strconv"

	"github.com/cruiserxyz/sextile/internal/recipe"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// This file is the recipe → argv compiler. It is the ONLY place astrolog's
// switch ordering and toggle/last-wins semantics live: a recipe.ChartSpec is a
// validated, order-free description; Compile walks a fixed sequence of bands
// (Source → Frame → Object → Mode → Render) so an unsafe ordering cannot be
// expressed. buildChartArgv (compute.go) was the legacy hand-assembler this
// superseded; it was retired in the v0b cutover, and ComputeChart now lowers a
// typed recipe.ChartSpec — built by internal/lookup — through Compile directly.

// ArtifactKind classifies what an invocation produces, so the runner knows how
// to consume it — and, per the compiler invariant, never routes an image
// through the text sanity/parse pipeline.
type ArtifactKind int

const (
	TextStdout ArtifactKind = iota // parse stdout
	ImageFile                      // astrolog writes an image via -Xo (appended at run time)
)

// ParserKind names the parser bound to a text invocation's stdout. Image
// invocations carry ParseNone.
type ParserKind int

const (
	ParseNone ParserKind = iota
	ParsePositions
	ParseAspects
	ParseCrossAspects // two-chart -r0 relationship aspects (role-preserving)
	ParseRawReport    // unparsed stdout for the passive Details surface
)

// TempFileInput is chart data an Invocation needs staged to a file before it can
// run: Content is written to a temp path and that path is substituted into Argv
// at ArgIndex. It exists for `-r0` comparison charts, whose two charts are FILE
// arguments (the inline -qj form carries a single chart). The compiler emits the
// content + index; materializing the file is an execution concern, exactly like
// the `-Xo` image sink path — the compiler stays pure (no IO).
type TempFileInput struct {
	ArgIndex int
	Content  string
}

// Invocation is one concrete astrolog process: the argv to run, what it
// produces, and which parser consumes it. For an ImageFile invocation the
// runner appends `-Xo <path>` — the sink path is an execution concern, not part
// of the compiled command. TempInputs (if any) name argv positions the runner
// must fill with staged chart files before invoking.
type Invocation struct {
	Argv       []string
	Artifact   ArtifactKind
	Parser     ParserKind
	TempInputs []TempFileInput
	// call is the body call the compiler built when it emitted Argv and the
	// chooser band together: a private snapshot, so nothing done to the visible
	// Argv afterwards can reach Astrolog.
	call BodyCall
}

// newInvocation is the compiler's one way to make an Invocation: it pairs the
// argv with its band at compile time.
func newInvocation(argv []string, kind ArtifactKind, parser ParserKind, temps []TempFileInput, band Band) Invocation {
	return Invocation{Argv: argv, Artifact: kind, Parser: parser, TempInputs: temps, call: newBodyCall(argv, band)}
}

// Call is the invocation as a body call: the snapshot the compiler built. If
// the visible Argv no longer equals that snapshot it returns the zero call,
// which the seam refuses, so a mutation after compilation never reaches Astrolog.
func (inv Invocation) Call() BodyCall {
	if !slices.Equal(inv.Argv, inv.call.argv) {
		return BodyCall{}
	}
	return inv.call
}

// ExecutionPlan is the lowering of one (ChartSpec, Projection) pair. For v0
// every projection compiles to a single invocation; the slice leaves room for
// projections that need more than one later.
type ExecutionPlan struct {
	Projection  string
	Invocations []Invocation
}

// PlanBundle is several projections of ONE ChartSpec compiled together. The
// shared semantic prefix (Source+Frame+Object bands) is computed once and
// exposed both as structured tokens (NormalizedPrefix, for tests/debug) and as
// a stable PrefixFingerprint (a cache/reuse key) — never as the architectural
// center; each plan still carries its own full argv.
type PlanBundle struct {
	NormalizedPrefix  []string
	PrefixFingerprint string
	Plans             []ExecutionPlan
}

// Compile lowers one (ChartSpec, Projection) into an ExecutionPlan. It is pure
// and total: the spec is validated up front, so emission never has to ask "is
// this valid?". The same spec+projection always yields byte-identical argv.
func Compile(spec recipe.ChartSpec, p recipe.Projection) (ExecutionPlan, error) {
	if err := spec.Validate(); err != nil {
		return ExecutionPlan{}, fmt.Errorf("astrolog: Compile: %w", err)
	}
	if p == nil {
		return ExecutionPlan{}, fmt.Errorf("astrolog: Compile: nil Projection")
	}
	if err := p.Validate(); err != nil {
		return ExecutionPlan{}, fmt.Errorf("astrolog: Compile: %w", err)
	}
	if err := checkCompat(spec, p); err != nil {
		return ExecutionPlan{}, fmt.Errorf("astrolog: Compile: %w", err)
	}
	prefix, temps, band, err := emitPrefix(spec, zonesChart(p), labelsChart(p))
	if err != nil {
		return ExecutionPlan{}, err
	}
	invs, err := emitProjection(prefix, temps, band, spec.Objects, comparisonInfoOf(spec), p)
	if err != nil {
		return ExecutionPlan{}, err
	}
	if err := carryRegistry(invs, spec); err != nil {
		return ExecutionPlan{}, err
	}
	return ExecutionPlan{Projection: p.Label(), Invocations: invs}, nil
}

// carryRegistry hands the spec's object list, and the numbers its argv draws,
// to each call the compiler issued, so the parsers can name every row. It
// fails when the objects the host draws are not known (drawnObjectNumbers).
func carryRegistry(invs []Invocation, spec recipe.ChartSpec) error {
	objects, err := ArgvObjectNumbers(spec.Objects)
	if err != nil {
		return err
	}
	for i := range invs {
		invs[i].call.Registry = spec.Registry
		invs[i].call.Objects = objects
	}
	return nil
}

// isComparison reports whether the spec's source is a two-chart relationship.
func isComparison(spec recipe.ChartSpec) bool {
	_, ok := spec.Source.(recipe.Comparison)
	return ok
}

// isMultiWheel reports whether the spec's source is the N-ring root.
func isMultiWheel(spec recipe.ChartSpec) bool {
	_, ok := spec.Source.(recipe.MultiWheel)
	return ok
}

// checkCompat enforces spec×projection compatibility — a rule that needs BOTH
// halves and so belongs in neither recipe.Validate (knows only the spec) nor
// Projection.Validate (knows only the projection). A Comparison source
// supports Aspects (cross-aspects), WheelImage (bi-wheel), an Aspects
// RawReport, and — since an earlier change — a BODY raw report: `-RO <n>` over a
// relationship run is well defined, restricting the cross list to the rows
// where that object is one of the two ends. Positions and the CHART raw
// report stay rejected: both would have to answer "which chart?" — positions
// are per-chart, and the influence chart over `-r0` blends the two.
func checkCompat(spec recipe.ChartSpec, p recipe.Projection) error {
	// A MultiWheel is a picture of N charts and nothing else: Astrolog has no
	// N-way listing (-a over -r3 prints chart 1 x chart 2 only), so every text
	// projection would silently answer a different question.
	if isMultiWheel(spec) {
		if _, isWheel := p.(recipe.WheelImage); !isWheel {
			return fmt.Errorf("%s is not defined over a MultiWheel source (only the wheel image draws N charts)", p.Label())
		}
		return nil
	}
	if isComparison(spec) {
		if _, isPos := p.(recipe.Positions); isPos {
			return fmt.Errorf("Positions is not defined over a Comparison source (use Aspects for cross-aspects or WheelImage for a bi-wheel)")
		}
		if raw, ok := p.(recipe.RawReport); ok && raw.Kind() == recipe.RawReportChart {
			return fmt.Errorf("%s is not defined over a Comparison source (use an aspects or body raw report)", raw.Label())
		}
	}
	return nil
}

// CompileBundle compiles several projections of ONE spec. It validates and
// emits the shared prefix once, fingerprints it, then plans each projection
// over it — so positions+aspects+image of a chart share a single compiled
// prefix instead of the runner patching argv (the old "append -a" wart).
func CompileBundle(spec recipe.ChartSpec, ps ...recipe.Projection) (PlanBundle, error) {
	if err := spec.Validate(); err != nil {
		return PlanBundle{}, fmt.Errorf("astrolog: CompileBundle: %w", err)
	}
	// A bundle shares one prefix across projections, so it never carries the
	// chart zone pin (-zZ is the single-chart compute's alone, N23) nor a chart
	// name (its wheel must not carry one, C5a).
	prefix, temps, band, err := emitPrefix(spec, false, false)
	if err != nil {
		return PlanBundle{}, err
	}
	comp := comparisonInfoOf(spec)
	plans := make([]ExecutionPlan, 0, len(ps))
	for _, p := range ps {
		if p == nil {
			return PlanBundle{}, fmt.Errorf("astrolog: CompileBundle: nil Projection")
		}
		if err := p.Validate(); err != nil {
			return PlanBundle{}, fmt.Errorf("astrolog: CompileBundle: %w", err)
		}
		if err := checkCompat(spec, p); err != nil {
			return PlanBundle{}, fmt.Errorf("astrolog: CompileBundle: %w", err)
		}
		invs, err := emitProjection(prefix, temps, band, spec.Objects, comp, p)
		if err != nil {
			return PlanBundle{}, err
		}
		if err := carryRegistry(invs, spec); err != nil {
			return PlanBundle{}, err
		}
		plans = append(plans, ExecutionPlan{Projection: p.Label(), Invocations: invs})
	}
	return PlanBundle{
		NormalizedPrefix:  prefix,
		PrefixFingerprint: fingerprint(prefix, temps),
		Plans:             plans,
	}, nil
}

// emitPrefix builds the Source+Frame+Object bands — the argv shared by every
// projection of a spec — and any temp-file inputs the source needs staged.
//
// MomentAtPlace (single chart), canonical order:
//
//	Source: -qj <jd> -zl <lon> <lat> [-zZ <zone> <dst>] -zi <name> <place>
//	        (-zZ: Positions and Aspects of a source that names its zone, and
//	        nothing else; -zi: always, empty unless the projection prints the
//	        chart's header — labelArgv)
//	Frame:  -c <code> <zodiac pin> _Ys _sr0 :sz   (see frameBand)
//	        then the clock pins and the reform-date pin
//	Object: [-Yn] [-R <n> …]   (toggles sorted ascending by object number)
//
// LocalMomentAtPlace (single chart, raw report only):
//
//	Source: -qj <jd> -zl <lon> <lat> -zZ <zone> <dst> -zi <name> <place>
//	        (-zZ always, except inside the 1582 window, where the moment is
//	        shown in UT: the specification. No calendar fields are built in Go.)
//
// Comparison (two charts):
//
//	Source: -r0 <FILE0> <FILE1>     (the two charts staged as chart files)
//	Frame:  -c <code> <zodiac pin> _Ys _sr0 :sz   (see frameBand)
//	Object: [-Yn] [-R <n> …]        (honored — -r0 respects the object band)
func emitPrefix(spec recipe.ChartSpec, zoned, labelled bool) ([]string, []TempFileInput, Band, error) {
	frame, err := emitFrame(spec)
	if err != nil {
		return nil, nil, Band{}, err
	}
	// The clock pins ride after the frame band on every source, so the compiler
	// and the timeline spell the same display state (the specification C1).
	// The reform date rides just before them, on every chart call: a host
	// astrolog.as cannot move the rule a staged file or a search reads dates by
	// (the specification C6). The clock pins stay the last run of the frame.
	frame = append(frame, reformPin...)
	frame = append(frame, CurrentClock().Pins()...)
	switch src := spec.Source.(type) {
	case recipe.MomentAtPlace:
		var zone []string
		if zoned && src.TZ != "" {
			var err error
			if zone, err = chartZone(src); err != nil {
				return nil, nil, Band{}, err
			}
		}
		jd, err := momentArg(src.JD)
		if err != nil {
			return nil, nil, Band{}, fmt.Errorf("astrolog: Compile: %w", err)
		}
		argv := []string{
			"-qj", jd,
			"-zl", formatLon(src.Lon), formatLat(src.Lat),
		}
		argv = append(argv, zone...)
		argv = append(argv, labelArgv(src.Name, src.Place, labelled)...)
		argv = append(argv, frame...)
		obj, err := emitObjects(spec.Objects)
		if err != nil {
			return nil, nil, Band{}, err
		}
		asp := emitAspects(spec.Aspects)
		argv = append(argv, obj...)
		return append(argv, asp...), nil, newBand(obj, asp), nil
	case recipe.LocalMomentAtPlace:
		jd, err := momentArg(src.JD)
		if err != nil {
			return nil, nil, Band{}, fmt.Errorf("astrolog: Compile: %w", err)
		}
		zone, err := zoneArgs(src.JD, src.TZ)
		if err != nil {
			return nil, nil, Band{}, err
		}
		argv := []string{"-qj", jd, "-zl", formatLon(src.Lon), formatLat(src.Lat)}
		argv = append(argv, zone...)
		argv = append(argv, labelArgv(src.Name, src.Place, labelled)...)
		argv = append(argv, frame...)
		obj, err := emitObjects(spec.Objects)
		if err != nil {
			return nil, nil, Band{}, err
		}
		asp := emitAspects(spec.Aspects)
		argv = append(argv, obj...)
		return append(argv, asp...), nil, newBand(obj, asp), nil
	case recipe.Comparison:
		c0, err := stageChart(src.Primary, spec.Houses, spec.Zodiac, labelled)
		if err != nil {
			return nil, nil, Band{}, err
		}
		c1, err := stageChart(src.Secondary, spec.Houses, spec.Zodiac, labelled)
		if err != nil {
			return nil, nil, Band{}, err
		}
		// `-r0 <FILE0> <FILE1>`: the file paths are filled at execution (indices
		// 1 and 2, before the frame band). Ordering is meaningful — FILE0 is the
		// inner wheel / first aspect operand. The object band IS honored in two-
		// chart mode (verified: -r0 respects -Yn/-R), so the lens applies to the
		// relationship just as it does to a single chart.
		argv := []string{"-r0", "", ""}
		argv = append(argv, frame...)
		obj, err := emitObjects(spec.Objects)
		if err != nil {
			return nil, nil, Band{}, err
		}
		argv = append(argv, obj...)
		// An `-rt` comparison reads its second chart's objects from the
		// TRANSIT set, which `-R` never touches: a host that lists an
		// absent object there would still make Astrolog compute it.
		if src.Relation == recipe.TransitToNatal {
			argv = append(argv, emitTransitAbsentOff(CurrentAvailability())...)
		}
		// The comparison path inherits the same aspect set: -r0 honors the
		// aspect restrictions exactly as a single chart does, so nothing
		// here treats the relationship specially.
		asp := emitAspects(spec.Aspects)
		argv = append(argv, asp...)
		temps := []TempFileInput{{ArgIndex: 1, Content: c0}, {ArgIndex: 2, Content: c1}}
		return argv, temps, newBand(obj, asp), nil
	case recipe.MultiWheel:
		// `-i <F1> -i2 <F2> … -iN <FN> -rN`: N staged charts, ring 1 innermost
		// with the houses (Astrolog's legend "#1: Inner Ring + Houses"). Each
		// file path is filled at execution, at the index just after its -i
		// switch. Frame, object and aspect bands follow exactly as in the
		// two-chart case.
		var argv []string
		var temps []TempFileInput
		for i, ring := range src.Rings {
			content, err := stageChart(ring, spec.Houses, spec.Zodiac, labelled)
			if err != nil {
				return nil, nil, Band{}, err
			}
			sw := "-i"
			if i > 0 {
				sw = "-i" + strconv.Itoa(i+1)
			}
			argv = append(argv, sw, "")
			temps = append(temps, TempFileInput{ArgIndex: len(argv) - 1, Content: content})
		}
		argv = append(argv, "-r"+strconv.Itoa(len(src.Rings)))
		argv = append(argv, frame...)
		obj, err := emitObjects(spec.Objects)
		if err != nil {
			return nil, nil, Band{}, err
		}
		argv = append(argv, obj...)
		asp := emitAspects(spec.Aspects)
		argv = append(argv, asp...)
		return argv, temps, newBand(obj, asp), nil
	default:
		return nil, nil, Band{}, fmt.Errorf("astrolog: Compile: unsupported Source type %T", spec.Source)
	}
}

// stageChart renders one sub-source of a Comparison as its chart file. A
// MomentAtPlace is written in UT; a LocalMomentAtPlace carries the local clock
// and zone (the report path only, the specification).
func stageChart(src recipe.Source, houses recipe.HouseSystem, zodiac recipe.Zodiac, labelled bool) (string, error) {
	switch m := src.(type) {
	case recipe.MomentAtPlace:
		if !labelled {
			m.Name, m.Place = "", ""
		}
		return FormatChartFile(m, houses, zodiac)
	case recipe.LocalMomentAtPlace:
		if !labelled {
			m.Name, m.Place = "", ""
		}
		return FormatLocalChartFile(m, houses, zodiac)
	}
	return "", fmt.Errorf("astrolog: Compile: Comparison sub-sources must be MomentAtPlace or LocalMomentAtPlace in v1 (got %T)", src)
}

// emitFrame builds the reference-frame band shared by every source: see
// frameBand.
func emitFrame(spec recipe.ChartSpec) ([]string, error) {
	return frameBand(spec.Houses, spec.Zodiac)
}

// frameBand is the one place the reference-frame band is written, so a chart
// call, a staged chart's argv and a timeline search cannot disagree
// (the specification C1, C5):
//
//	-c <code> _s _Ys _sr0 :sz           tropical
//	-c <code> =s <offset> _Ys _sr0 :sz  sidereal (the chosen ayanamsa's offset
//	                                    from Fagan-Bradley, numeric: ayanamsa.go)
//
// Every switch is a pin, not a request. Each one names a state the host's
// astrolog.as can otherwise choose: the zodiac (`_s`/`=s`), the sidereal
// offset (the explicit `0` after `=s`, so the host's `:s` cannot pick the
// ayanamsa), the plane (`_Ys`; `=Ys` moves sidereal longitudes), the latitude
// column (`_sr0`; `=sr0` prints declination where the parser reads latitude)
// and the zodiac display format (`:sz`; `:sd` prints degrees where it reads
// signs). A bare `-s` is a toggle against whatever the host left, and is never
// emitted. `-Yp` is deliberately absent: measured irrelevant to the ephemeris
// path (TestHostAstrologAsCannotMoveTheFrame/neutralisers/=Yp-irrelevant).
func frameBand(houses recipe.HouseSystem, zodiac recipe.Zodiac) ([]string, error) {
	code, ok := houseCode(houses)
	if !ok {
		return nil, fmt.Errorf("astrolog: Compile: unknown HouseSystem %d", int(houses))
	}
	pin, ok := zodiacPin(zodiac)
	if !ok {
		return nil, fmt.Errorf("astrolog: Compile: unknown Zodiac %d", int(zodiac))
	}
	argv := append([]string{"-c", code}, pin...)
	return append(argv, "_Ys", "_sr0", ":sz"), nil
}

// emitObjects builds the object-policy band for a single chart. `-Yn` leads it
// under either profile — it selects the true-node/Lilith variants of objects 16
// and 18, which is orthogonal to which objects are drawn.
//
// A delta baseline emits `-R <n>` toggles, sorted ascending. An Explicit policy
// emits `-R0` followed by the chosen object numbers, the exact counterpart of
// the `-RA0` aspect show-list.
//
// The difference is not stylistic. `-R <n>` FLIPS an object's restriction
// against whatever state the host's astrolog.as left, so the same argv draws
// different bodies on different machines; `-R0 <n …>` restricts everything and
// then unrestricts the list, so it means the same thing everywhere. Measured:
// with an astrolog.as that restricts Chiron, `-R 11` removes Chiron from a
// chart the lens says shows it, and `-R0 … 11 …` does not. An operator who
// ticked a set gets that set.
//
// A policy with Only narrows that set to the bodies in Only and takes the `-R0`
// form, under either profile: the list is what drawnObjectNumbers answers, the
// one rule for what a chart draws (a body outside Only, already removed by the
// policy, or without an ephemeris file is not in it). Marks narrow, they never
// leave a chart with nothing: an Only that no drawn body is in is dropped, and
// the policy's own baseline goes out (withoutDrainedOnly). The timeline's band
// reads Only for the natal side alone (timelineBand): a mark narrows that
// slot's own side.
func emitObjects(o recipe.ObjectPolicy) ([]string, error) {
	av := CurrentAvailability()
	argv := emitNodeMode(o)
	if len(o.Only) > 0 {
		// Only narrows whatever the baseline resolved to, and a delta cannot
		// say "just these" (`-R <n>` flips against the host), so the
		// intersection goes out as the exact show-list, the same `-R0 <list>`
		// an Explicit policy compiles to. It is never bare because the marked
		// bodies are not drawn (they have no ephemeris file, the chart's set
		// does not list them, the lens removed them): a bare `-R0` restricts
		// every body and leaves on screen only what the host's astrolog.as
		// enables beyond them (the cusp points), an empty chart. The
		// narrowing is dropped and the policy's own baseline goes out instead.
		without, drained, err := withoutDrainedOnly(o, av)
		if err != nil {
			return nil, err
		}
		if drained {
			return emitObjects(without)
		}
		drawn, err := drawnObjects(o, av)
		if err != nil {
			return nil, err
		}
		return append(argv, emitShowList(flagMainShow, sortedMembers(drawn))...), nil
	}
	if o.Baseline == recipe.Explicit {
		// A bare `-R0` is the empty selection, and it is a real chart: the
		// operator chose no bodies. It is not the same as no override, which
		// is a delta baseline and emits no restriction at all. It is the only
		// bare list: no narrowing produces one.
		drawn, err := drawnObjects(o, av)
		if err != nil {
			return nil, err
		}
		return append(argv, emitShowList(flagMainShow, sortedMembers(drawn))...), nil
	}
	flips, err := deltaFlips(o, av)
	if err != nil {
		return nil, err
	}
	for _, n := range deltaToggles(flips, av) {
		argv = append(argv, "-R", strconv.Itoa(n))
	}
	return argv, nil
}

// withoutDrainedOnly is o without its Only narrowing when that narrowing leaves
// nothing to draw, whatever the reason: every body in it has no ephemeris file,
// or the chart's set does not draw it (the host's astrolog.as does not list it,
// the lens removed it, an Explicit selection does not hold it). drained says so.
// A mark narrows a chart, it never blanks one (operator, 2026-11-02: a panel may
// never blank), so the policy's own baseline is drawn instead of a bare list;
// the bare `-R0` is left to an Explicit policy with no selection and no
// narrowing, where the operator chose no bodies.
func withoutDrainedOnly(o recipe.ObjectPolicy, av Availability) (recipe.ObjectPolicy, bool, error) {
	if len(o.Only) == 0 {
		return o, false, nil
	}
	wanted, err := wantedObjects(o, av)
	if err != nil {
		return o, false, err
	}
	if len(withoutAbsentSet(wanted, av)) != 0 {
		return o, false, nil
	}
	o.Only = nil
	return o, true, nil
}

// DrainedObjects is every object of an Only narrowing that was dropped because it
// left nothing to draw, as numbers, ascending: the absent ones (no ephemeris
// file) and the ones the chart's set does not draw, both. nil when the narrowing
// still draws something. emitObjects falls back to the baseline for it, and the
// caller says so once, saying for each object which of the two it is.
func DrainedObjects(o recipe.ObjectPolicy) ([]int, error) {
	av := CurrentAvailability()
	without, drained, err := withoutDrainedOnly(o, av)
	if err != nil || !drained {
		return nil, err
	}
	// A baseline that draws nothing either (an Explicit policy with no
	// selection: the operator chose no bodies) has no set to fall back to, so
	// there is nothing to say.
	baseline, err := drawnObjects(without, av)
	if err != nil || len(baseline) == 0 {
		return nil, err
	}
	return o.Only.Members(), nil
}

// emitTransitAbsentOff is the `-RT <n>` flips that switch OFF each absent
// object the host's own transit set lists (Availability.DefaultTransit). A flip
// is against the host, so only a listed object is flipped: an unlisted one is
// already off, and flipping it would switch it ON.
func emitTransitAbsentOff(av Availability) []string {
	var argv []string
	for _, n := range av.DefaultTransit {
		if av.IsAbsent(n) {
			argv = append(argv, flagTransitToggle, strconv.Itoa(n))
		}
	}
	return argv
}

// withoutAbsentSet is set minus the objects with no ephemeris file, as a new
// set: the point of the availability fact is that such an object never reaches
// Astrolog's object list, so it is never computed at 0 Aries.
func withoutAbsentSet(set map[int]bool, av Availability) map[int]bool {
	out := make(map[int]bool, len(set))
	for n, in := range set {
		if in && !av.IsAbsent(n) {
			out[n] = true
		}
	}
	return out
}

// sortedMembers is the numbers of a set, ascending.
func sortedMembers(set map[int]bool) []int {
	out := make([]int, 0, len(set))
	for n, in := range set {
		if in {
			out = append(out, n)
		}
	}
	sort.Ints(out)
	return out
}

// deltaToggles is a delta baseline's `-R <n>` flips made safe for absent
// objects. A flip is against the host's astrolog.as, so: a flip that would
// switch an absent object ON is dropped, and an absent object the host lists by
// default gets a flip that switches it OFF. The host's default is the one the
// probe read (Availability.Default).
func deltaToggles(nums []int, av Availability) []int {
	if len(av.Absent) == 0 {
		return nums
	}
	hostLists := map[int]bool{}
	for _, n := range av.Default {
		hostLists[n] = true
	}
	var out []int
	flipped := map[int]bool{}
	for _, n := range nums {
		flipped[n] = true
		if av.IsAbsent(n) && !hostLists[n] {
			continue
		}
		out = append(out, n)
	}
	for _, n := range av.AbsentNumbers() {
		if hostLists[n] && !flipped[n] {
			out = append(out, n)
		}
	}
	sort.Ints(out)
	return out
}

// The four band switches. Each is written ONCE, here, and every emitter in this
// package spells it through the constant: that is what internal/invariants
// TestOneEncoderPerBand counts, and why the Details timeline cannot carry a
// second, drifting encoding of the chart's body, aspect or node-mode band.
const (
	flagTrueNode      = "-Yn"  // true node / Lilith variants of objects 16 and 18
	flagMainShow      = "-R0"  // exact show-list for the main (natal / sky) object set
	flagTransitShow   = "-RT0" // exact show-list for the transiting object set
	flagTransitToggle = "-RT"  // flips one object of the transiting set against the host
	flagAspectShow    = "-RA0" // exact show-list for aspects
)

// emitNodeMode is the `-Yn` token of a policy, or nothing.
func emitNodeMode(o recipe.ObjectPolicy) []string {
	if o.TrueNodeMode {
		return []string{flagTrueNode}
	}
	return nil
}

// emitShowList is the exact show-list form of a band: the flag (flagMainShow,
// flagTransitShow or flagAspectShow) followed by the numbers. An empty list is
// a bare flag, which restricts everything.
func emitShowList(flag string, nums []int) []string {
	argv := []string{flag}
	for _, n := range nums {
		argv = append(argv, strconv.Itoa(n))
	}
	return argv
}

// emitAspects builds the aspect-policy band. An unset policy emits nothing and
// the host astrolog.as default stands; an explicit one emits `-RA0` followed by
// the chosen registry indices, ascending and deduplicated.
//
// `-RA0 [<asp> …]` restricts every aspect and then unrestricts the listed ones,
// which is the direct expression of an arbitrary subset and the exact
// counterpart of the `-R0` show-list used for objects. Recorded runs behind
// this (the specification): `-RA0 5 4 -a`
// yields only Trine and Sextile rows; `-A 5 -RA0 13 10 -a` yields Quintile and
// Septile, so the band overrides whatever cap the host config sets; a bare
// `-RA0` yields no aspects at exit 0.
//
// It deliberately does NOT use `-A <n>` plus `-Ao <i> 0`. That also expresses a
// subset, but it says "off" by writing an aspect's ORB to zero — destroying the
// field the chooser displays and the follow-up orb slice edits — and it costs
// up to 24 argv tokens where this costs one flag and a list.
func emitAspects(a recipe.AspectPolicy) []string {
	if !a.Explicit {
		return nil
	}
	return emitShowList(flagAspectShow, a.Selected())
}

// influenceChartMode is the chart raw report's mode selector: Astrolog's
// influence chart. `-j` prints each object's positional power, aspect
// power, total, rank and percent; the sign/element/mode and house/quadrant
// tables ride along under `fInfluenceSign`.
//
// The `=` prefix is load-bearing, not decoration. Astrolog 8 defaults
// `fInfluenceSign` ON (astrolog.h: "flag fInfluenceSign; // -j0 (on by
// default)") and parses `-j0` as a TOGGLE (astrolog.cpp: `case 'j': if (ch1
// == '0') SwitchF(us.fInfluenceSign); SwitchF(us.fInfluence);`), so a bare
// `-j0` would FLIP whatever the operator's astrolog.as last set — and the
// 7.x docs that say `-j0` ADDS the sign tables describe the opposite of what
// the shipped binary does. Astrolog's switch-prefix grammar (astrolog.cpp:
// `=` sets fOr, `_` sets fAnd, `-` sets fNot) forces a switch on regardless
// of the config file, so `=j0` means "influence chart ON and its sign +
// house sections ON" on every installation. Determinism (gate 11) and
// inspectable provenance (gate 12) both depend on that: the argv says what
// it does without reading the host's astrolog.as.
const influenceChartMode = "=j0"

// aspectListMode is the aspect-list mode selector: `-aa` is `-a` with the orb
// printed as `a`/`s` (applying/separating, Astrolog's own velocity-based
// phase) instead of a +/- offset. Every aspect list Sextile reads or
// shows carries it.
const aspectListMode = "-aa"

// comparisonInfo is what emitProjection needs to know about a two-chart
// source: whether there is one, and whether its Secondary transits its
// Primary (recipe.TransitToNatal).
type comparisonInfo struct {
	active  bool
	transit bool
}

func comparisonInfoOf(spec recipe.ChartSpec) comparisonInfo {
	// A MultiWheel is "active" only so the wheel compiles to the single plain
	// image (no highlight layers); it has no transit relation.
	if isMultiWheel(spec) {
		return comparisonInfo{active: true}
	}
	c, ok := spec.Source.(recipe.Comparison)
	return comparisonInfo{active: ok, transit: ok && c.Relation == recipe.TransitToNatal}
}

// textSource lowers a TransitToNatal comparison's source switch from -r0 to
// -rt for the PHASE-BEARING text projections — the aspect list and the
// aspects and body raw reports, all three of which print `a`/`s`:
// -rt is -r0 with the natal chart's velocities zeroed, so `a`/`s` follow the
// transiting body alone (Astrolog: "so aspects show whether transits to natal
// planets are applying or separating correctly"). The wheel image keeps -r0
// on purpose: -rt also applies Astrolog's TRANSIT restrictions to the outer
// ring (dropping the transiting Moon and angles by default) and relabels the
// legend, and the shipped bi-wheel is not this switch's to change. Argv is
// a copy, so the shared prefix (and its fingerprint) is untouched.
func (c comparisonInfo) textSource(argv []string) []string {
	if c.transit && len(argv) > 0 && argv[0] == "-r0" {
		argv[0] = "-rt"
	}
	return argv
}

// emitProjection appends the Mode (+Render) tail for a projection to a COPY of
// the shared prefix and returns the projection's Invocations, carrying through
// the source's staged inputs. Copying keeps projections from aliasing each
// other's argv when planned from one prefix. comparison selects the
// role-preserving cross-aspect parser for the Aspects projection and
// suppresses the wheel highlight. objects is consulted only by the WheelImage
// highlight, whose show-list may cover only drawn bodies.
//
// Every projection compiles to exactly one invocation except a WheelImage
// with an active highlight, which compiles to the earlier change three-layer plan.
func emitProjection(prefix []string, temps []TempFileInput, band Band, objects recipe.ObjectPolicy, cmp comparisonInfo, p recipe.Projection) ([]Invocation, error) {
	argv := append([]string{}, prefix...)
	comparison := cmp.active
	switch proj := p.(type) {
	case recipe.Positions:
		// `=v0`, not the toggle `-v0`: the forced-on form cannot be turned OFF by
		// a host astrolog.as that already says `=v0`. `-Yw 0` keeps Astrolog from
		// printing a station `S` in the R column, a row the parser's pattern
		// would drop silently (the specification C13, D38).
		argv = append(argv, "=v0", "-Yw", "0")
		return []Invocation{newInvocation(argv, TextStdout, ParsePositions, temps, band)}, nil
	case recipe.Aspects:
		argv = append(cmp.textSource(argv), aspectListMode)
		parser := ParseAspects
		if comparison {
			parser = ParseCrossAspects
		}
		return []Invocation{newInvocation(argv, TextStdout, parser, temps, band)}, nil
	case recipe.RawReport:
		switch proj.Kind() {
		case recipe.RawReportChart:
			argv = append(argv, influenceChartMode)
		case recipe.RawReportBody:
			argv = append(cmp.textSource(argv), aspectListMode, "-RO", strconv.Itoa(proj.Object().Number))
		case recipe.RawReportAspects:
			argv = append(cmp.textSource(argv), aspectListMode)
		default:
			return nil, fmt.Errorf("astrolog: Compile: unsupported RawReport kind %d", int(proj.Kind()))
		}
		return []Invocation{newInvocation(argv, TextStdout, ParseRawReport, temps, band)}, nil
	case recipe.WheelImage:
		// Select the native format explicitly instead of inheriting astrolog.as'
		// graphics preference. HTTP uses SVG; desktop viewers use PNG so xdg-open
		// resolves the image to the configured raster viewer.
		formatFlag := "-XV"
		if proj.Render.Format == recipe.ImagePNG {
			formatFlag = "-Xbp"
		}
		argv = append(argv, formatFlag)
		argv = append(argv, emitRender(proj.Render)...)
		image := func(a []string) Invocation {
			return newInvocation(a, ImageFile, ParseNone, temps, band)
		}
		members, err := highlightMembers(objects, proj.Render.Highlight)
		if err != nil {
			return nil, err
		}
		// A comparison bi-wheel, an inactive highlight, and a highlight whose
		// every member is undrawn or unnumbered all compile to the single
		// plain wheel.
		if comparison || len(members) == 0 {
			return []Invocation{image(argv)}, nil
		}
		// The an earlier change three-layer composite plan: base (the plain wheel MINUS
		// the highlight members — a trailing -R flip per member, so the
		// dimmed context cannot carry a shadow twin of a body the mask
		// draws, an earlier change), foreground (show-list of the members, so Astrolog
		// draws exactly their glyphs and the aspect web among them, all at
		// its own colors), and furniture (the same render with an empty
		// show-list, for the compositor to subtract). Hiding a member from
		// the base also removes its dim aspect lines to unselected bodies —
		// Astrolog draws no line without both endpoints — which is the
		// accepted trade of the twin-free base.
		baseArgv := append([]string{}, argv...)
		for _, n := range members {
			baseArgv = append(baseArgv, "-R", strconv.Itoa(n))
		}
		foreArgv := append(append([]string{}, argv...), emitShowList(flagMainShow, members)...)
		furnArgv := append(append([]string{}, argv...), flagMainShow)
		return []Invocation{image(baseArgv), image(foreArgv), image(furnArgv)}, nil
	default:
		return nil, fmt.Errorf("astrolog: Compile: unsupported Projection type %T", p)
	}
}

// emitRender turns a RenderSpec into its -X* styling switches in a fixed order
// (legacy lazyastro order). It never emits -Xo: the output path is appended by
// the image runner, not the compiler.
func emitRender(r recipe.RenderSpec) []string {
	var sw []string
	if r.Width > 0 {
		sw = append(sw, "-Xw", strconv.Itoa(r.Width))
	}
	if r.Enhanced {
		sw = append(sw, "-Xi")
	}
	if r.AspectGlyphs {
		sw = append(sw, "-XA")
	}
	if r.ThickLines {
		sw = append(sw, "-Xx", "-Xx0")
	}
	if r.ReverseColors {
		sw = append(sw, "-Xr")
	}
	if r.NoBorder {
		sw = append(sw, "-Xu")
	}
	if r.SymbolScale > 0 {
		sw = append(sw, "-Xs", strconv.Itoa(r.SymbolScale))
	}
	if r.TextScale > 0 {
		sw = append(sw, "-XS", strconv.Itoa(r.TextScale))
	}
	if r.NoInfoText {
		sw = append(sw, "-Xt")
	}
	if r.DegreeWheel {
		sw = append(sw, "-XL")
	}
	return sw
}

// highlightMembers resolves a highlight to the foreground show-list: the
// object numbers of (Selected ∪ Focused) ∩ drawn, ascending. The failure
// posture: a member that the current lens does not draw (or that has no
// ephemeris file) is omitted — never unrestricted into the wheel, never
// emitted malformed, and never added: a highlight narrows what is drawn.
//
// "Drawn" is asked of drawnObjectNumbers, the one rule for it, under both
// profiles: without it an Explicit chart's highlight would find nothing drawn,
// collapse to the plain wheel, and do it silently, because an image renders
// either way.
func highlightMembers(objects recipe.ObjectPolicy, h recipe.HighlightSpec) ([]int, error) {
	if !h.Active() {
		return nil, nil
	}
	drawn, err := drawnObjectNumbers(objects)
	if err != nil {
		return nil, err
	}
	// Selected ∪ Focused, as a set, so a focused body that is also selected
	// contributes one show-list entry rather than two.
	wanted := recipe.NewObjectSet(h.Selected.Members()...)
	if h.HasFocus {
		wanted[h.FocusedObject] = true
	}
	var members []int
	for _, n := range wanted.Members() {
		if drawn[n] {
			members = append(members, n)
		}
	}
	return members, nil
}

// ArgvObjectNumbers is the set of objects the chart the policy compiles to
// really draws on this host, as object numbers: what the argv asks for, cut by
// the availability fact. It is the host's own default set (Availability.Default)
// with the policy's flips, or the Explicit selection, narrowed by Only: the
// objects with no glyph that the host enables (Fortune, Vertex, East Point) are in it, and
// an absent object, which the argv switches off or leaves out, is not. The body
// selector seeds its ticks from it, so a tick shows what is drawn and a change
// of one tick changes one object. It fails before the host's default set is
// known (drawnObjectNumbers).
func ArgvObjectNumbers(o recipe.ObjectPolicy) ([]int, error) {
	av := CurrentAvailability()
	if without, drained, err := withoutDrainedOnly(o, av); err != nil {
		return nil, err
	} else if drained {
		o = without
	}
	drawn, err := drawnObjects(o, av)
	if err != nil {
		return nil, err
	}
	return sortedMembers(drawn), nil
}

// WantedObjectNumbers is the policy's set before the absent cut, as numbers,
// ascending, resolved against the given availability: the objects the view
// would draw if every ephemeris file were there. The ephemeris account needs it
// (it names the ones that are not, so it lives in internal/ui, which may not
// import this package: the seam is internal/lookup), and it is the same rule as
// drawnObjectNumbers up to that cut. What the chart really draws, the chooser's
// ticks, is ArgvObjectNumbers.
func WantedObjectNumbers(o recipe.ObjectPolicy, av Availability) ([]int, error) {
	wanted, err := wantedObjects(o, av)
	if err != nil {
		return nil, err
	}
	return sortedMembers(wanted), nil
}

// errNoHostDefault is what the rule answers before the host's default object
// set is known. Nothing guesses it: the typed list that stood in for it until
// slice 8c drew objects the host did not, and left out ones it did.
var errNoHostDefault = errors.New("astrolog: which objects are drawn is not known: the host's default object set has not been read (the ephemeris probe has not answered)")

// drawnObjectNumbers is the ONE answer to "what does this policy draw on this
// host": wantedObjects with the objects that have no ephemeris file taken out.
// Every consumer asks here (the `-R0` list emitObjects sends, the numbers the
// parsers expect, the wheel highlight, the drained-selection fallback, the
// chooser's ticks, the ephemeris account) and none of them keeps a Go-side
// list, or reads the policy's Add and Remove, of its own. The one thing that
// does read them is the unnarrowed delta's `-R` flips, which deltaFlips derives
// from the same deltaSet this rule starts from, so the argv and the answer
// cannot disagree.
func drawnObjectNumbers(o recipe.ObjectPolicy) (map[int]bool, error) {
	return drawnObjects(o, CurrentAvailability())
}

// drawnObjects is drawnObjectNumbers against a given availability.
func drawnObjects(o recipe.ObjectPolicy, av Availability) (map[int]bool, error) {
	wanted, err := wantedObjects(o, av)
	if err != nil {
		return nil, err
	}
	return withoutAbsentSet(wanted, av), nil
}

// wantedObjects is the rule before the absent cut: under a delta baseline the
// host's own default set (av.Default, read from `astrolog -HO` at startup) with
// the policy's Add and Remove applied to it as set operations (deltaSet), under
// Explicit the selection; then narrowed to Only, a plain intersection that can
// only take objects away, so an object the host does not draw is not drawn
// because it is marked. The policy's typed profile is not read, because there
// is none: a number needs no table, and the host's default is the only
// baseline.
//
// av.Default is empty until the startup probe answers, and then this is an
// error, never a guess.
func wantedObjects(o recipe.ObjectPolicy, av Availability) (map[int]bool, error) {
	if len(av.Default) == 0 {
		return nil, errNoHostDefault
	}
	var on map[int]bool
	if o.Baseline == recipe.Explicit {
		on = make(map[int]bool)
		for _, n := range o.Selected() {
			on[n] = true
		}
	} else {
		on = deltaSet(o, av)
	}
	if len(o.Only) > 0 {
		for n := range on {
			if !o.Only.Has(n) {
				delete(on, n)
			}
		}
	}
	return on, nil
}

// deltaSet is the host's default set with the policy's Add and Remove applied
// as set operations: an Add is shown whether or not the host already lists it,
// a Remove is hidden whether or not it does, and neither ever does the
// opposite. (The policy cannot be validated against the host's set, so a
// Remove of an object the host does not list, or an Add of one it does, is a
// policy like any other and draws what it says.)
func deltaSet(o recipe.ObjectPolicy, av Availability) map[int]bool {
	on := make(map[int]bool, len(av.Default)+len(o.Add))
	for _, n := range av.Default {
		on[n] = true
	}
	for _, n := range o.Add.Members() {
		on[n] = true
	}
	for _, n := range o.Remove.Members() {
		delete(on, n)
	}
	return on
}

// deltaFlips is the `-R <n>` switches that take the host's default set to
// deltaSet, ascending: an object the policy names whose state differs from the
// host's. `-R` flips the host's own state, so an Add of an object the host
// already lists, or a Remove of one it does not, needs none; sending one anyway
// would do the opposite of what the policy says, and the set the argv draws
// would stop being the set drawnObjectNumbers answers.
func deltaFlips(o recipe.ObjectPolicy, av Availability) ([]int, error) {
	if len(av.Default) == 0 {
		return nil, errNoHostDefault
	}
	listed := make(map[int]bool, len(av.Default))
	for _, n := range av.Default {
		listed[n] = true
	}
	on := deltaSet(o, av)
	var out []int
	for _, n := range o.Toggles() {
		if on[n] != listed[n] {
			out = append(out, n)
		}
	}
	return out, nil
}

// fingerprint is a stable hash of the shared prefix — a cache/reuse key, not a
// security digest. FNV-1a over the NUL-joined prefix tokens, then the staged
// inputs. Folding the temp contents in is what keeps the key correct for a
// Comparison: its prefix carries EMPTY file-path slots (filled at execution),
// so two distinct comparisons would otherwise collide on an identical prefix.
func fingerprint(prefix []string, temps []TempFileInput) string {
	h := fnv.New64a()
	for _, tok := range prefix {
		_, _ = h.Write([]byte(tok))
		_, _ = h.Write([]byte{0})
	}
	for _, t := range temps {
		_, _ = h.Write([]byte(strconv.Itoa(t.ArgIndex)))
		_, _ = h.Write([]byte{0})
		_, _ = h.Write([]byte(t.Content))
		_, _ = h.Write([]byte{0})
	}
	return strconv.FormatUint(h.Sum64(), 16)
}

// houseCode maps a recipe.HouseSystem to astrolog's `-c` value. This is the
// astrolog-fact half of the house abstraction; the semantic half is in recipe.
func houseCode(h recipe.HouseSystem) (string, bool) {
	if !h.Valid() {
		return "", false
	}
	return strconv.Itoa(int(h)), true
}

// timelineBand is the chooser band of one Details timeline search: the node
// mode, the object show-lists per search side, and the aspect band, in that
// order, built from this file's own emitters so the timeline cannot encode any
// of them a second way.
//
// A TICK LISTS AND AN UNTICK HIDES, on both sides (the specification D1,
// rewritten 2026-11-04 from hide-only). The visible set is resolved from the
// policy alone (Explicit is its Selection; AstrologDefault is the main default
// set plus Add minus Remove, as Astrolog numbers) minus the objects without an
// ephemeris file. Then, per side:
//
//   - the natal side (`-R0`; the whole of a sky search) is the visible set,
//     narrowed to the policy's Only (the Space marks) unless that narrowing
//     leaves nothing to draw, for any reason (the fallback emitObjects uses:
//     never a bare `-R0` from a mark), and banded as the exact list whenever
//     it differs from the binary's main default;
//   - the transiting side of `-t` (`-RT0`) is the visible set minus what the
//     binary keeps off its own transit default while listing it on its main
//     default (Main minus Transit as read from the binary: on 8.00 the Moon and
//     the two angles, measured to flood) minus every cusp point, banded
//     whenever it differs from the binary's transit default; marks do not touch
//     it;
//   - the transiting side of `-tr` (`-RT0`) is the visible set minus the cusp
//     points, before the marks (a return is one row per cycle, so the lunar
//     return stays), banded whenever it differs from the main default, which is
//     what Astrolog's return default carries.
//
// A cusp point has no sky rows, so a sky search carries it harmlessly. An empty
// Explicit set stays the bare `-R0` and `-RT0`.
// transitMarks are the now slot's marks: they narrow the transiting side of a
// transit or return search the way a chart's own marks (o.Only) narrow its
// natal side, as Astrolog's -RT0 list, never by filtering rows (operator,
// 2026-11-04: "only if we can do it with astrolog-only"). Marks never add: an
// object the side leaves out stays out, so a mark on the transiting Moon, which
// the binary keeps off that side, narrows nothing. Marks that leave nothing on
// the side, for any reason, are ignored, as for o.Only (narrowByMarks).
//
// voids narrows a sky search's side to the Moon and the planets, the objects
// Astrolog's void-of-course rule counts (FAspectVoid, extern.h: both objects up
// to Pluto), for the month search that only marks the Moon void (skySegment).
func timelineBand(src TimelineSource, o recipe.ObjectPolicy, transitMarks recipe.ObjectSet, a recipe.AspectPolicy, d DefaultSets, voids bool) (Band, error) {
	av := CurrentAvailability()
	visible := visibleObjectNumbers(o, d.Main)
	// An object without its ephemeris file is not visible, whatever the policy
	// or the host's default says: the side is then banded without it.
	for _, n := range av.AbsentNumbers() {
		delete(visible, n)
	}
	natal := visible
	without, drained, err := withoutDrainedOnly(o, av)
	if err != nil {
		return Band{}, err
	}
	if !drained && len(without.Only) > 0 {
		natal = make(map[int]bool, len(visible))
		for _, n := range without.Only.Members() {
			if visible[n] {
				natal[n] = true
			}
		}
	}
	if voids {
		planets := make(map[int]bool, len(natal))
		for n := range natal {
			planets[n] = natal[n] && n <= voidObjectLast
		}
		natal = planets
	}
	objects := emitNodeMode(o)
	if show, banded := bandedList(natal, d.Main); banded {
		objects = append(objects, emitShowList(flagMainShow, show)...)
	}
	switch src {
	case TimelineTransit:
		transiting := make(map[int]bool, len(visible))
		for n := range visible {
			transiting[n] = !isCuspPoint(n)
		}
		for _, n := range d.Main {
			if !slices.Contains(d.Transit, n) {
				transiting[n] = false
			}
		}
		narrowByMarks(transiting, transitMarks, av)
		if show, banded := bandedList(transiting, d.Transit); banded {
			objects = append(objects, emitShowList(flagTransitShow, show)...)
		}
	case TimelineReturn:
		transiting := make(map[int]bool, len(visible))
		for n := range visible {
			transiting[n] = !isCuspPoint(n)
		}
		// Astrolog's return side by default is its main set; the cusp points
		// leave both, so an untouched lens emits no transiting band at all.
		var mainNoCusps []int
		for _, n := range d.Main {
			if !isCuspPoint(n) {
				mainNoCusps = append(mainNoCusps, n)
			}
		}
		narrowByMarks(transiting, transitMarks, av)
		if show, banded := bandedList(transiting, mainNoCusps); banded {
			objects = append(objects, emitShowList(flagTransitShow, show)...)
		}
	}
	return newBand(objects, emitAspects(a)), nil
}

// narrowByMarks keeps on a side only the marked objects, in place. Marks that
// leave nothing on the side narrow nothing: every mark is absent (the drained
// guard), or the side does not list any marked object. A mark narrows a side, it
// never blanks one, and it never adds an object the side leaves out (operator,
// 2026-11-02 and 2026-11-04).
func narrowByMarks(side map[int]bool, marks recipe.ObjectSet, av Availability) {
	live := make(map[int]bool, len(marks))
	for n := range marks {
		if marks[n] && !av.IsAbsent(n) {
			live[n] = true
		}
	}
	kept := false
	for n := range side {
		if side[n] && live[n] {
			kept = true
			break
		}
	}
	if !kept {
		return
	}
	for n := range side {
		if !live[n] {
			side[n] = false
		}
	}
}

// voidObjectLast is Astrolog's Pluto (oPlu), the last object its void-of-course
// rule counts.
const voidObjectLast = 10

// isCuspPoint is the class fact that an object is one of the house wheel's
// points (objects 22 to 33): a transiting one is a daily flood, a natal one
// is a handful of rows.
func isCuspPoint(n int) bool {
	_, ok := aspect.CuspHouse(aspect.Object{Number: n})
	return ok
}

// cycleConjunctionAspect is Astrolog's number for the conjunction, its first
// aspect (`-RA0 1` lists the same events as `-A 1`, measured 2026-10-16).
const cycleConjunctionAspect = 1

// cycleBand is the cycle search's band: the Sun and one other object on the
// main side, and the conjunction alone, built from the compiler's own
// emitters (C9). It takes an object, not a policy: the cycle's objects are the
// template's subject and the Sun, never the lens's show list. The object is
// Astrolog's own number (the Sun is 1), so any object the binary prints works.
func cycleBand(other aspect.Object) (Band, error) {
	n := other.Number
	if n <= 0 || n == sunObjectNumber {
		return Band{}, fmt.Errorf("astrolog: cycle band: object %d (%q) cannot be searched against the Sun", n, other.Name)
	}
	if av := CurrentAvailability(); av.IsAbsent(n) {
		return Band{}, fmt.Errorf("%w: %s, no ephemeris file %s", ErrBodyUnavailable, other.Name, av.Absent[n])
	}
	return newBand(emitShowList(flagMainShow, []int{sunObjectNumber, n}), emitShowList(flagAspectShow, []int{cycleConjunctionAspect})), nil
}

// visibleObjectNumbers is the policy's visible set as Astrolog object numbers,
// resolved without consulting any Go-side default list: under a delta baseline
// the starting point is mainDefault, which was read from Astrolog.
func visibleObjectNumbers(o recipe.ObjectPolicy, mainDefault []int) map[int]bool {
	visible := make(map[int]bool)
	if o.Baseline == recipe.Explicit {
		for _, n := range o.Selected() {
			visible[n] = true
		}
		return visible
	}
	for _, n := range mainDefault {
		visible[n] = true
	}
	for _, n := range o.Add.Members() {
		visible[n] = true
	}
	for _, n := range o.Remove.Members() {
		delete(visible, n)
	}
	return visible
}

// bandedList returns the members of set (true entries), ascending, and whether
// that list differs from the side's default set read from Astrolog (so a band is
// needed). An empty set differs from any non-empty default and is the bare list.
func bandedList(set map[int]bool, side []int) ([]int, bool) {
	var shown []int
	for n, in := range set {
		if in {
			shown = append(shown, n)
		}
	}
	sort.Ints(shown)
	want := uniqueInts(side)
	sort.Ints(want)
	return shown, !slices.Equal(shown, want)
}

// uniqueInts is the distinct values of s. A side's default is a set; this keeps
// the comparison by size honest if a caller hands a repeated number.
func uniqueInts(s []int) []int {
	seen := make(map[int]bool, len(s))
	var out []int
	for _, n := range s {
		if !seen[n] {
			seen[n] = true
			out = append(out, n)
		}
	}
	return out
}
