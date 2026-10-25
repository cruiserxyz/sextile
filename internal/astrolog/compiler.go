package astrolog

import (
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
	return ExecutionPlan{Projection: p.Label(), Invocations: invs}, nil
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
// A policy with Only narrows that set to the bodies in Only and always takes
// the `-R0` form, under either profile (a body outside Only, or already
// removed by the policy, is not in the list). The timeline's band does not read
// Only: it follows the lens the operator committed, not the surface's selection.
func emitObjects(o recipe.ObjectPolicy) ([]string, error) {
	argv := emitNodeMode(o)
	if len(o.Only) > 0 {
		// Only narrows whatever the baseline resolved to, and a delta cannot
		// say "just these" (`-R <n>` flips against the host), so the
		// intersection goes out as the exact show-list, the same `-R0 <list>`
		// an Explicit policy compiles to. It may be bare (`-R0`): the
		// operator selected bodies the lens or the chooser had removed, and
		// Astrolog gets the list that results.
		return append(argv, emitShowList(flagMainShow, DrawnObjectNumbers(o))...), nil
	}
	if o.Baseline == recipe.Explicit {
		// A bare `-R0` is the empty selection, and it is a real chart: the
		// operator chose no bodies. It is not the same as no override, which
		// is a delta baseline and emits no restriction at all.
		return append(argv, emitShowList(flagMainShow, o.Selected())...), nil
	}
	nums, err := toggleNumbers(o)
	if err != nil {
		return nil, err
	}
	for _, n := range nums {
		argv = append(argv, "-R", strconv.Itoa(n))
	}
	return argv, nil
}

// The four band switches. Each is written ONCE, here, and every emitter in this
// package spells it through the constant: that is what internal/invariants
// TestOneEncoderPerBand counts, and why the Details timeline cannot carry a
// second, drifting encoding of the chart's body, aspect or node-mode band.
const (
	flagTrueNode    = "-Yn"  // true node / Lilith variants of objects 16 and 18
	flagMainShow    = "-R0"  // exact show-list for the main (natal / sky) object set
	flagTransitShow = "-RT0" // exact show-list for the transiting object set
	flagAspectShow  = "-RA0" // exact show-list for aspects
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
			n, ok := bodyObjectNum[proj.Body()]
			if !ok {
				return nil, fmt.Errorf("astrolog: Compile: body %s has no astrolog object number", proj.Body())
			}
			argv = append(cmp.textSource(argv), aspectListMode, "-RO", strconv.Itoa(n))
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
		members := highlightMembers(objects, proj.Render.Highlight)
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
// object numbers of (Selected ∪ Focused) ∩ drawn ∩ numbered, ascending. The
// failure posture: a member
// Astrolog cannot number, or that the current lens does not draw, is omitted
// — never unrestricted into the wheel, never emitted malformed.
//
// "Drawn" is asked of the policy, and the two profiles answer it differently.
// A delta baseline answers in bodies, via Visible(). An Explicit policy answers
// in object NUMBERS, and Visible() cannot speak for it at all — it returns
// aspect.Body values and recipe holds no number→body mapping. That mapping
// lives here, which is why this is the layer that reconciles them: without
// this branch an Explicit chart's highlight would find nothing drawn, collapse
// to the plain wheel, and do it silently, because an image renders either way.
func highlightMembers(objects recipe.ObjectPolicy, h recipe.HighlightSpec) []int {
	if !h.Active() {
		return nil
	}
	drawn := drawnObjectNumbers(objects)
	// Selected ∪ Focused, as a set, so a focused body that is also selected
	// contributes one show-list entry rather than two.
	wanted := recipe.NewBodySet(h.Selected.Members()...)
	if h.Focused != aspect.BodyUnknown {
		wanted[h.Focused] = true
	}
	var members []int
	for _, b := range wanted.Members() {
		n, ok := bodyObjectNum[b]
		if !ok || !drawn[n] {
			continue
		}
		members = append(members, n)
	}
	sort.Ints(members)
	return members
}

// DrawnObjectNumbers is the policy's visible set as Astrolog object numbers,
// ascending. It is the exported face of drawnObjectNumbers, and it exists for
// one caller: the body selector has to seed its checklist from whatever the
// chart is drawing NOW, and the body→number map lives here, behind an
// isolation boundary `internal/ui` may not cross. The UI reaches it through
// `internal/lookup`.
func DrawnObjectNumbers(o recipe.ObjectPolicy) []int {
	drawn := drawnObjectNumbers(o)
	out := make([]int, 0, len(drawn))
	for n := range drawn {
		out = append(out, n)
	}
	sort.Ints(out)
	return out
}

// drawnObjectNumbers is the set of Astrolog object numbers the policy puts on
// the wheel. It is the one place the two VisibilityProfiles are reconciled into
// a common currency; every caller that needs "is this object drawn" asks here
// rather than reaching for Visible(), which is delta-only.
func drawnObjectNumbers(o recipe.ObjectPolicy) map[int]bool {
	drawn := make(map[int]bool)
	if o.Baseline == recipe.Explicit {
		// Only is bodies, the selection is object numbers: the narrowing is
		// done in numbers, here, where the body-to-number table lives. (A
		// delta baseline is narrowed by Visible, in bodies.)
		var only map[int]bool
		if len(o.Only) > 0 {
			only = make(map[int]bool, len(o.Only))
			for _, b := range o.Only.Members() {
				if n, ok := bodyObjectNum[b]; ok {
					only[n] = true
				}
			}
		}
		for _, n := range o.Selected() {
			if only == nil || only[n] {
				drawn[n] = true
			}
		}
		return drawn
	}
	for _, b := range o.Visible().Members() {
		if n, ok := bodyObjectNum[b]; ok {
			drawn[n] = true
		}
	}
	return drawn
}

// toggleNumbers maps the policy's toggle bodies to astrolog object numbers,
// sorted ascending so emission is deterministic (and matches legacy order).
func toggleNumbers(o recipe.ObjectPolicy) ([]int, error) {
	bodies := o.Toggles()
	nums := make([]int, 0, len(bodies))
	for _, b := range bodies {
		n, ok := bodyObjectNum[b]
		if !ok {
			return nil, fmt.Errorf("astrolog: Compile: body %s has no astrolog object number", b)
		}
		nums = append(nums, n)
	}
	sort.Ints(nums)
	return nums, nil
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

// bodyObjectNum maps a Sextile body to astrolog's internal object number. The
// classical ten coincide (Sun=1 … Pluto=10); the nodes and angles do not, so
// the table is explicit: 16 North Node, 17 South Node, 11 Chiron, 18 Lilith,
// 22 Ascendant, 31 Midheaven, 28 Descendant, 25 Nadir/Imum Coeli. Objects 22
// through 33 are the house wheel (recipe.HouseWheelObjectFirst/Last).
var bodyObjectNum = map[aspect.Body]int{
	aspect.Sun: 1, aspect.Moon: 2, aspect.Mercury: 3, aspect.Venus: 4, aspect.Mars: 5,
	aspect.Jupiter: 6, aspect.Saturn: 7, aspect.Uranus: 8, aspect.Neptune: 9, aspect.Pluto: 10,
	aspect.NorthNode: 16, aspect.SouthNode: 17, aspect.Chiron: 11, aspect.Lilith: 18,
	aspect.Ascendant: 22, aspect.Midheaven: 31, aspect.Descendant: 28, aspect.ImumCoeli: 25,
}

// timelineBand is the chooser band of one Details timeline search: the node
// mode, the object show-lists per search side, and the aspect band, in that
// order, built from this file's own emitters so the timeline cannot encode any
// of them a second way.
//
// Objects are HIDE-ONLY (the specification D1). The
// visible set is resolved from the policy alone — Explicit is its Selection;
// AstrologDefault is the main default set plus Add minus Remove, as Astrolog
// numbers — and a side is banded only when visible ∩ that side's default
// differs from that side's default. So ticking a body Astrolog leaves off a
// side (the transiting Moon) never adds rows, and an unfiltered timeline
// carries no object band at all.
//
// `-R0` is the natal/sky side and `-RT0` the transiting side of `-t`/`-tr`. A
// sky search (`-dm`) has the one side.
func timelineBand(src TimelineSource, o recipe.ObjectPolicy, a recipe.AspectPolicy, d DefaultSets) (Band, error) {
	visible, err := visibleObjectNumbers(o, d.Main)
	if err != nil {
		return Band{}, err
	}
	objects := emitNodeMode(o)
	if show, banded := bandedSide(visible, d.Main); banded {
		objects = append(objects, emitShowList(flagMainShow, show)...)
	}
	if src == TimelineTransit || src == TimelineReturn {
		if show, banded := bandedSide(visible, d.Transit); banded {
			objects = append(objects, emitShowList(flagTransitShow, show)...)
		}
	}
	return newBand(objects, emitAspects(a)), nil
}

// cycleConjunctionAspect is Astrolog's number for the conjunction, its first
// aspect (`-RA0 1` lists the same events as `-A 1`, measured 2026-10-16).
const cycleConjunctionAspect = 1

// cycleBand is the cycle search's band: the Sun and one other body on the
// main side, and the conjunction alone, built from the compiler's own
// emitters (C9). It takes bodies, not a policy: the cycle's bodies are the
// template's subject and the Sun, never the lens's show list.
func cycleBand(other aspect.Body) (Band, error) {
	n, ok := bodyObjectNum[other]
	if !ok || other == aspect.Sun {
		return Band{}, fmt.Errorf("astrolog: cycle band: body %s cannot be searched against the Sun", other)
	}
	return newBand(emitShowList(flagMainShow, []int{bodyObjectNum[aspect.Sun], n}), emitShowList(flagAspectShow, []int{cycleConjunctionAspect})), nil
}

// visibleObjectNumbers is the policy's visible set as Astrolog object numbers,
// resolved without consulting any Go-side default list: under a delta baseline
// the starting point is mainDefault, which was read from Astrolog.
func visibleObjectNumbers(o recipe.ObjectPolicy, mainDefault []int) (map[int]bool, error) {
	visible := make(map[int]bool)
	if o.Baseline == recipe.Explicit {
		for _, n := range o.Selected() {
			visible[n] = true
		}
		return visible, nil
	}
	for _, n := range mainDefault {
		visible[n] = true
	}
	for _, b := range o.Add.Members() {
		n, ok := bodyObjectNum[b]
		if !ok {
			return nil, fmt.Errorf("astrolog: timeline band: body %s has no astrolog object number", b)
		}
		visible[n] = true
	}
	for _, b := range o.Remove.Members() {
		n, ok := bodyObjectNum[b]
		if !ok {
			return nil, fmt.Errorf("astrolog: timeline band: body %s has no astrolog object number", b)
		}
		delete(visible, n)
	}
	return visible, nil
}

// bandedSide returns visible ∩ side, ascending, and whether it differs from
// side (so a band is needed). side is a default set read from Astrolog.
func bandedSide(visible map[int]bool, side []int) ([]int, bool) {
	side = uniqueInts(side)
	var shown []int
	for _, n := range side {
		if visible[n] {
			shown = append(shown, n)
		}
	}
	sort.Ints(shown)
	return shown, len(shown) != len(side)
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
